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
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/basarsubasi/website/internal/adapters/database"
	httpadapter "github.com/basarsubasi/website/internal/adapters/http"
	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/devseed"
	"github.com/basarsubasi/website/internal/usecase"
)

type Config struct {
	DatabasePath   string
	Address        string
	FrontendDir    string
	AdminPassword  string
	SecureCookies  bool
	TrustedProxies []netip.Prefix
	TrustFlyProxy  bool
	Storage        *r2.Config
}

func (c Config) validate() error {
	if c.Storage == nil {
		return fmt.Errorf("required R2 configuration is nil")
	}
	for _, item := range []struct{ name, value string }{
		{"DATABASE_PATH", c.DatabasePath}, {"HTTP_ADDR", c.Address}, {"FRONTEND_DIR", c.FrontendDir}, {"ADMIN_PASSWORD", c.AdminPassword},
		{"R2_ENDPOINT or R2_ACCOUNT_ID", c.Storage.Endpoint}, {"R2_BUCKET", c.Storage.Bucket},
		{"R2_ACCESS_KEY_ID", c.Storage.AccessKeyID}, {"R2_SECRET_ACCESS_KEY", c.Storage.SecretAccessKey}, {"R2_PUBLIC_URL", c.Storage.PublicURL},
	} {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("required configuration %s is missing", item.name)
		}
	}
	if len(c.AdminPassword) < 16 {
		return fmt.Errorf("ADMIN_PASSWORD must contain at least 16 characters")
	}
	return nil
}

type Application struct {
	Repository *database.Repository
	Backup     usecase.Backup
	Objects    *r2.R2
	Service    *usecase.Service
	Handler    http.Handler
	Server     *http.Server
}

func New(cfg Config) (*Application, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	objects, err := r2.New(*cfg.Storage)
	if err != nil {
		return nil, err
	}
	startup, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	repo, err := database.Open(startup, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	backup := r2.NewBackup(objects)
	if err = repo.Initialize(startup, backup.LoadSnapshot); err != nil {
		repo.Close()
		return nil, fmt.Errorf("initialize database from R2: %w", err)
	}
	service := usecase.New(repo, objects)
	handler := httpadapter.New(service, httpadapter.Config{Password: cfg.AdminPassword, FrontendDir: cfg.FrontendDir, SecureCookies: cfg.SecureCookies, TrustedProxies: cfg.TrustedProxies, TrustFlyProxy: cfg.TrustFlyProxy})
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	return &Application{Repository: repo, Backup: backup, Objects: objects, Service: service, Handler: handler, Server: server}, nil
}
func Main() {
	if err := Run(os.Args[1:]); err != nil {
		log.Fatalf("application initialization failed: %v", err)
	}
}
func SeedMain() {
	if err := RunSeed(os.Args[1:]); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
}
func UnseedMain() {
	if err := RunUnseed(os.Args[1:]); err != nil {
		log.Fatalf("seed cleanup failed: %v", err)
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopBackups := app.startBackups(ctx)
	defer stopBackups()
	result := make(chan error, 1)
	go func() { result <- app.Server.ListenAndServe() }()
	log.Printf("Listening on http://%s", cfg.Address)
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
func RunSeed(args []string) error {
	return runFixtures(args, false)
}
func RunUnseed(args []string) error {
	return runFixtures(args, true)
}
func runFixtures(args []string, remove bool) error {
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var count int
	if remove {
		count, err = devseed.Remove(ctx, app.Repository)
		log.Printf("Removed %d seed records from SQLite; R2 cleanup queued", count)
	} else {
		count, err = devseed.Seed(ctx, app.Repository, app.Objects)
		log.Printf("Added %d sample records to SQLite; R2 backups queued", count)
	}
	if err != nil {
		return err
	}
	backupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := app.Repository.SyncBackups(backupCtx, app.Backup); err != nil {
		return fmt.Errorf("local changes saved; R2 backup remains queued: %w", err)
	}
	return nil
}

// Flags override process environment, which overrides .env and then defaults.
func loadConfig(args []string, lookup func(string) (string, bool)) (Config, error) {
	fs := flag.NewFlagSet("website", flag.ContinueOnError)
	envPath := ".env"
	if value, ok := lookup("ENV_FILE"); ok {
		envPath = value
	}
	envFile := fs.String("env", envPath, "Environment file; empty disables file loading")
	address := fs.String("addr", "", "HTTP listen address (HTTP_ADDR)")
	frontend := fs.String("frontend", "", "Built frontend directory (FRONTEND_DIR)")
	databasePath := fs.String("db", "", "SQLite database file (DATABASE_PATH)")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments")
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
		if value != "" {
			return value
		}
		return other
	}
	cfg := Config{DatabasePath: fallback(*databasePath, fallback(get("DATABASE_PATH"), "data/website.db")), Address: fallback(*address, fallback(get("HTTP_ADDR"), "127.0.0.1:8080")), FrontendDir: fallback(*frontend, fallback(get("FRONTEND_DIR"), "frontend/dist")), AdminPassword: get("ADMIN_PASSWORD")}
	if raw := get("COOKIE_SECURE"); raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be a boolean")
		}
		cfg.SecureCookies = secure
	}
	if raw := get("TRUST_FLY_PROXY"); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("TRUST_FLY_PROXY must be a boolean")
		}
		cfg.TrustFlyProxy = enabled
	}
	if raw := strings.TrimSpace(get("TRUSTED_PROXY_CIDRS")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
			if err != nil {
				return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS must contain comma-separated IP prefixes: %w", err)
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, prefix.Masked())
		}
	}
	endpoint := get("R2_ENDPOINT")
	if endpoint == "" && get("R2_ACCOUNT_ID") != "" {
		endpoint = "https://" + get("R2_ACCOUNT_ID") + ".r2.cloudflarestorage.com"
	}
	cfg.Storage = &r2.Config{Endpoint: endpoint, Bucket: get("R2_BUCKET"), AccessKeyID: get("R2_ACCESS_KEY_ID"), SecretAccessKey: get("R2_SECRET_ACCESS_KEY"), PublicURL: get("R2_PUBLIC_URL")}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func readEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	if path == "" {
		return values, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && path == ".env" {
		return values, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read environment file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		text = strings.TrimPrefix(text, "export ")
		key, value, ok := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		if !ok || !envKey.MatchString(key) {
			return nil, fmt.Errorf("invalid environment assignment at line %d", line)
		}
		value, err := envValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid environment value at line %d", line)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
func envValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] != '\'' && value[0] != '"' {
		if before, _, ok := strings.Cut(value, " #"); ok {
			value = before
		}
		return strings.TrimSpace(value), nil
	}
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
		if tail != "" && !strings.HasPrefix(tail, "#") {
			return "", fmt.Errorf("unexpected text after quote")
		}
		if quote == '\'' {
			return value[1:i], nil
		}
		return strconv.Unquote(value[:i+1])
	}
	return "", io.ErrUnexpectedEOF
}

// startBackups owns the worker lifecycle; reads and edits never wait for R2.
func (app *Application) startBackups(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
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
				log.Printf("R2 backup pending: %v", err)
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
