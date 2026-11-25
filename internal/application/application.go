// Package application owns configuration, dependency wiring, and lifecycle.
package application

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	httpadapter "github.com/basarsubasi/website/internal/handler/adapter/http"
	"github.com/basarsubasi/website/internal/module/blog"
	blogsqlite "github.com/basarsubasi/website/internal/module/blog/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/gallery"
	gallerysqlite "github.com/basarsubasi/website/internal/module/gallery/adapter/sqlite"
	"github.com/basarsubasi/website/internal/platform/backup"
	"github.com/basarsubasi/website/internal/platform/database"
	"github.com/basarsubasi/website/internal/platform/r2"
)

const (
	invalidConcurrencyMessage    = "HTTP concurrency limits must be positive (or zero for defaults)"
	invalidRestoreTimeoutMessage = "RESTORE_TIMEOUT must be positive"
	missingR2ConfigMessage       = "required R2 configuration is nil"
	envDatabasePath              = "DATABASE_PATH"
	envHTTPAddr                  = "HTTP_ADDR"
	envFrontendDir               = "FRONTEND_DIR"
	envAdminPassword             = "ADMIN_PASSWORD"
	endpointOrAccountSetting     = "R2_ENDPOINT or R2_ACCOUNT_ID"
	envPhotosBucket              = "R2_PHOTOS_BUCKET"
	envAccessKeyID               = "R2_ACCESS_KEY_ID"
	envSecretAccessKey           = "R2_SECRET_ACCESS_KEY"
	envPhotosPublicURL           = "R2_PHOTOS_PUBLIC_URL"
	envBlogBucket                = "R2_BLOG_BUCKET"
	envBlogPublicURL             = "R2_BLOG_PUBLIC_URL"
	emptyString                  = ""
	missingSettingFormat         = "required configuration %s is missing"
	distinctBucketsMessage       = "R2_PHOTOS_BUCKET and R2_BLOG_BUCKET must be different buckets"
	shortPasswordMessage         = "ADMIN_PASSWORD must contain at least 16 characters"

	initializeDatabaseErrorFormat = "initialize database from R2: %w"

	startupFailureFormat = "application initialization failed: %v"

	tcpNetwork      = "tcp"
	listeningFormat = "Listening on http://%s"

	flagSetName                     = "website"
	defaultEnvFile                  = ".env"
	envFileSetting                  = "ENV_FILE"
	envFileFlag                     = "env"
	envFileHelp                     = "Environment file; empty disables file loading"
	listenAddrFlag                  = "addr"
	listenAddrHelp                  = "HTTP listen address (HTTP_ADDR)"
	frontendDirFlag                 = "frontend"
	frontendDirHelp                 = "Built frontend directory (FRONTEND_DIR)"
	databasePathFlag                = "db"
	databasePathHelp                = "SQLite database file (DATABASE_PATH)"
	unexpectedArgsMessage           = "unexpected positional arguments"
	defaultDatabasePath             = "data/website.db"
	defaultListenAddr               = "127.0.0.1:8080"
	defaultFrontendDir              = "ui/dist"
	envMaxRequests                  = "HTTP_MAX_REQUESTS"
	envMaxUploads                   = "HTTP_MAX_UPLOADS"
	envMaxConnections               = "HTTP_MAX_CONNECTIONS"
	positiveIntegerFormat           = "%s must be a positive integer"
	envRestoreTimeout               = "RESTORE_TIMEOUT"
	invalidDurationMessage          = "RESTORE_TIMEOUT must be a positive duration, for example 5m"
	envEphemeralDatabase            = "EPHEMERAL_DATABASE"
	invalidEphemeralDatabaseMessage = "EPHEMERAL_DATABASE must be a boolean"
	envTrustFlyProxy                = "TRUST_FLY_PROXY"
	invalidTrustFlyProxyMessage     = "TRUST_FLY_PROXY must be a boolean"
	envTrustedProxyCIDRs            = "TRUSTED_PROXY_CIDRS"
	cidrSeparator                   = ","
	invalidProxyCIDRsFormat         = "TRUSTED_PROXY_CIDRS must contain comma-separated IP prefixes: %w"
	envR2Endpoint                   = "R2_ENDPOINT"
	envR2AccountID                  = "R2_ACCOUNT_ID"
	httpsPrefix                     = "https://"
	r2EndpointSuffix                = ".r2.cloudflarestorage.com"

	envKeyPattern = `^[A-Za-z_][A-Za-z0-9_]*$`

	readEnvFileErrorFormat  = "read environment file: %w"
	commentPrefix           = "#"
	exportPrefix            = "export "
	assignmentSeparator     = "="
	invalidAssignmentFormat = "invalid environment assignment at line %d"
	invalidEnvValueFormat   = "invalid environment value at line %d"

	inlineCommentPrefix           = " #"
	unexpectedQuotedSuffixMessage = "unexpected text after quote"

	pendingBackupFormat = "R2 backup pending (%d queued operations): %v"
)

var (
	envKey                      = regexp.MustCompile(envKeyPattern)
	errInvalidEphemeralDatabase = errors.New(invalidEphemeralDatabaseMessage)
	errInvalidTrustFlyProxy     = errors.New(invalidTrustFlyProxyMessage)
)

type Config struct {
	EphemeralDatabase bool
	DatabasePath      string
	Address           string
	FrontendDir       string
	AdminPassword     string
	TrustedProxies    []netip.Prefix
	TrustFlyProxy     bool
	RestoreTimeout    time.Duration
	MaxRequests       int
	MaxUploads        int
	MaxConnections    int
	PhotoStorage      *r2.Config
	BlogStorage       *r2.Config
}

func (c Config) validate() error {
	if c.MaxRequests < 0 || c.MaxUploads < 0 || c.MaxConnections < 0 {
		return fmt.Errorf(invalidConcurrencyMessage)
	}
	if c.RestoreTimeout < 0 {
		return fmt.Errorf(invalidRestoreTimeoutMessage)
	}
	if c.PhotoStorage == nil || c.BlogStorage == nil {
		return fmt.Errorf(missingR2ConfigMessage)
	}
	for _, item := range []struct{ name, value string }{
		{envDatabasePath, c.DatabasePath}, {envHTTPAddr, c.Address}, {envFrontendDir, c.FrontendDir}, {envAdminPassword, c.AdminPassword},
		{endpointOrAccountSetting, c.PhotoStorage.Endpoint}, {envPhotosBucket, c.PhotoStorage.Bucket},
		{envAccessKeyID, c.PhotoStorage.AccessKeyID}, {envSecretAccessKey, c.PhotoStorage.SecretAccessKey}, {envPhotosPublicURL, c.PhotoStorage.PublicURL},
		{envBlogBucket, c.BlogStorage.Bucket}, {envBlogPublicURL, c.BlogStorage.PublicURL},
	} {
		if strings.TrimSpace(item.value) == emptyString {
			return fmt.Errorf(missingSettingFormat, item.name)
		}
	}
	if c.PhotoStorage.Endpoint == c.BlogStorage.Endpoint && c.PhotoStorage.Bucket == c.BlogStorage.Bucket {
		return fmt.Errorf(distinctBucketsMessage)
	}
	if len(c.AdminPassword) < 16 {
		return fmt.Errorf(shortPasswordMessage)
	}
	return nil
}

type Application struct {
	Repository  *database.Store
	Backup      backup.Target
	Objects     *r2.R2
	BlogObjects *r2.R2
	Gallery     *gallery.Module
	Blog        *blog.Module
	Handler     http.Handler
	Server      *http.Server
}

func New(cfg Config) (*Application, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	objects, err := r2.New(*cfg.PhotoStorage)
	if err != nil {
		return nil, err
	}
	blogObjects, err := r2.New(*cfg.BlogStorage)
	if err != nil {
		return nil, err
	}
	restoreTimeout := cfg.RestoreTimeout
	if restoreTimeout == 0 {
		restoreTimeout = 5 * time.Minute
	}
	startup, cancel := context.WithTimeout(context.Background(), restoreTimeout)
	defer cancel()
	open := database.Open
	if cfg.EphemeralDatabase {
		open = database.OpenEphemeral
	}
	repo, err := open(startup, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	backup := r2.NewSplitBackup(objects, blogObjects)
	if err = repo.Initialize(startup, backup.LoadSnapshot); err != nil {
		repo.Close()
		return nil, fmt.Errorf(initializeDatabaseErrorFormat, err)
	}
	repo.RequireBackup(backup)
	galleryModule := gallery.New(gallerysqlite.New(repo), objects)
	blogModule := blog.New(blogsqlite.New(repo), blogObjects, blogObjects.PublicOrigin())
	handler := httpadapter.New(galleryModule, blogModule, repo.Ping, httpadapter.Config{
		PhotoOrigin: objects.PublicOrigin(), BlogOrigin: blogObjects.PublicOrigin(),
		Password: cfg.AdminPassword, FrontendDir: cfg.FrontendDir,
		TrustedProxies: cfg.TrustedProxies, TrustFlyProxy: cfg.TrustFlyProxy,
		MaxRequests: cfg.MaxRequests, MaxUploads: cfg.MaxUploads,
	})
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	return &Application{Repository: repo, Backup: backup, Objects: objects, BlogObjects: blogObjects, Gallery: galleryModule, Blog: blogModule, Handler: handler, Server: server}, nil
}
func Main() {
	if err := Run(os.Args[1:]); err != nil {
		log.Fatalf(startupFailureFormat, err)
	}
}
func Run(args []string) error {
	cfg, err := loadConfig(args, os.LookupEnv)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	app, err := New(cfg)
	if err != nil {
		return err
	}
	defer app.Repository.Close()
	listener, err := net.Listen(tcpNetwork, cfg.Address)
	if err != nil {
		return err
	}
	listener = limitConnections(listener, cfg.MaxConnections)
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopBackups := app.startBackups(ctx)
	defer stopBackups()
	result := make(chan error, 1)
	go func() { result <- app.Server.Serve(listener) }()
	log.Printf(listeningFormat, cfg.Address)
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Server.Shutdown(shutdown); err != nil {
			_ = app.Server.Close()
			return err
		}
		return nil
	}
}

// Flags override process environment, which overrides .env and then defaults.
func loadConfig(args []string, lookup func(string) (string, bool)) (Config, error) {
	fs := flag.NewFlagSet(flagSetName, flag.ContinueOnError)
	envPath := defaultEnvFile
	if value, ok := lookup(envFileSetting); ok {
		envPath = value
	}
	envFile := fs.String(envFileFlag, envPath, envFileHelp)
	address := fs.String(listenAddrFlag, emptyString, listenAddrHelp)
	frontend := fs.String(frontendDirFlag, emptyString, frontendDirHelp)
	databasePath := fs.String(databasePathFlag, emptyString, databasePathHelp)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf(unexpectedArgsMessage)
	}
	values, err := readEnvFile(*envFile)
	if err != nil {
		return Config{}, err
	}
	get := func(key string) string {
		if value, ok := lookup(key); ok {
			return value
		}
		return values[key]
	}
	fallback := func(value, other string) string {
		if value != emptyString {
			return value
		}
		return other
	}
	cfg := Config{
		DatabasePath:   fallback(*databasePath, fallback(get(envDatabasePath), defaultDatabasePath)),
		Address:        fallback(*address, fallback(get(envHTTPAddr), defaultListenAddr)),
		FrontendDir:    fallback(*frontend, fallback(get(envFrontendDir), defaultFrontendDir)),
		AdminPassword:  get(envAdminPassword),
		RestoreTimeout: 5 * time.Minute,
	}
	if err := cfg.readLimits(get); err != nil {
		return Config{}, err
	}
	if err := cfg.readServerSettings(get); err != nil {
		return Config{}, err
	}
	endpoint := get(envR2Endpoint)
	if endpoint == emptyString && get(envR2AccountID) != emptyString {
		endpoint = httpsPrefix + get(envR2AccountID) + r2EndpointSuffix
	}
	cfg.PhotoStorage = &r2.Config{Endpoint: endpoint, Bucket: get(envPhotosBucket), AccessKeyID: get(envAccessKeyID), SecretAccessKey: get(envSecretAccessKey), PublicURL: get(envPhotosPublicURL)}
	cfg.BlogStorage = &r2.Config{Endpoint: endpoint, Bucket: get(envBlogBucket), AccessKeyID: get(envAccessKeyID), SecretAccessKey: get(envSecretAccessKey), PublicURL: get(envBlogPublicURL)}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg *Config) readLimits(get func(string) string) error {
	for _, limit := range []struct {
		name     string
		value    *int
		fallback int
	}{
		{envMaxRequests, &cfg.MaxRequests, httpadapter.DefaultMaxRequests},
		{envMaxUploads, &cfg.MaxUploads, httpadapter.DefaultMaxUploads},
		{envMaxConnections, &cfg.MaxConnections, defaultMaxConnections},
	} {
		*limit.value = limit.fallback
		if raw := get(limit.name); raw != emptyString {
			value, err := strconv.Atoi(raw)
			if err != nil || value <= 0 {
				return fmt.Errorf(positiveIntegerFormat, limit.name)
			}
			*limit.value = value
		}
	}
	if raw := get(envRestoreTimeout); raw != emptyString {
		duration, err := time.ParseDuration(raw)
		if err != nil || duration <= 0 {
			return fmt.Errorf(invalidDurationMessage)
		}
		cfg.RestoreTimeout = duration
	}
	return nil
}

func (cfg *Config) readServerSettings(get func(string) string) error {
	for _, setting := range []struct {
		name    string
		invalid error
		dst     *bool
	}{
		{envEphemeralDatabase, errInvalidEphemeralDatabase, &cfg.EphemeralDatabase},
		{envTrustFlyProxy, errInvalidTrustFlyProxy, &cfg.TrustFlyProxy},
	} {
		raw := get(setting.name)
		if raw == emptyString {
			continue
		}
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return setting.invalid
		}
		*setting.dst = value
	}
	if raw := strings.TrimSpace(get(envTrustedProxyCIDRs)); raw != emptyString {
		for _, value := range strings.Split(raw, cidrSeparator) {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
			if err != nil {
				return fmt.Errorf(invalidProxyCIDRsFormat, err)
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, prefix.Masked())
		}
	}
	return nil
}

func readEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	if path == emptyString {
		return values, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && path == defaultEnvFile {
		return values, nil
	}
	if err != nil {
		return nil, fmt.Errorf(readEnvFileErrorFormat, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == emptyString || strings.HasPrefix(text, commentPrefix) {
			continue
		}
		text = strings.TrimPrefix(text, exportPrefix)
		key, value, ok := strings.Cut(text, assignmentSeparator)
		key = strings.TrimSpace(key)
		if !ok || !envKey.MatchString(key) {
			return nil, fmt.Errorf(invalidAssignmentFormat, line)
		}
		value, err := envValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf(invalidEnvValueFormat, line)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
func envValue(value string) (string, error) {
	if value == emptyString {
		return emptyString, nil
	}
	if value[0] != '\'' && value[0] != '"' {
		before, _, _ := strings.Cut(value, inlineCommentPrefix)
		return strings.TrimSpace(before), nil
	}
	return quotedEnvValue(value)
}

func quotedEnvValue(value string) (string, error) {
	quote := value[0]
	for i := 1; i < len(value); i++ {
		if quote == '"' && value[i] == '\\' {
			i++
			continue
		}
		if value[i] != quote {
			continue
		}
		tail := strings.TrimSpace(value[i+1:])
		if tail != emptyString && !strings.HasPrefix(tail, commentPrefix) {
			return emptyString, fmt.Errorf(unexpectedQuotedSuffixMessage)
		}
		if quote == '\'' {
			return value[1:i], nil
		}
		return strconv.Unquote(value[:i+1])
	}
	return emptyString, io.ErrUnexpectedEOF
}

// startBackups retries failed writes and performs cleanup; successful writes
// already wait for R2 in the repository.
func (app *Application) startBackups(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		var maintenance time.Time
		for {
			pending, err := app.Repository.PendingBackups(ctx)
			if err == nil && (pending > 0 || time.Since(maintenance) >= time.Minute) {
				attempt, stop := context.WithTimeout(ctx, 2*time.Minute)
				err = app.Repository.SyncBackups(attempt, app.Backup)
				stop()
				maintenance = time.Now()
			}
			if err != nil && ctx.Err() == nil {
				log.Printf(pendingBackupFormat, pending, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}
