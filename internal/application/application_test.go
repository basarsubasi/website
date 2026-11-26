package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blogsqlite "github.com/basarsubasi/website/internal/module/blog/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	gallerysqlite "github.com/basarsubasi/website/internal/module/gallery/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	applicationTestEnvAdminPassword   = "ADMIN_PASSWORD"
	fixturePassword                   = "test-password-for-admin"
	applicationTestEnvR2AccountID     = "R2_ACCOUNT_ID"
	fixtureAccount                    = "account"
	applicationTestEnvAccessKeyID     = "R2_ACCESS_KEY_ID"
	fixtureAccessKey                  = "key"
	applicationTestEnvSecretAccessKey = "R2_SECRET_ACCESS_KEY"
	fixtureSecret                     = "secret"
	applicationTestEnvPhotosBucket    = "R2_PHOTOS_BUCKET"
	fixturePhotosBucket               = "website"
	applicationTestEnvPhotosPublicURL = "R2_PHOTOS_PUBLIC_URL"
	fixturePhotosURL                  = "https://images.example.com"
	applicationTestEnvBlogBucket      = "R2_BLOG_BUCKET"
	fixtureBlogBucket                 = "blog"
	applicationTestEnvBlogPublicURL   = "R2_BLOG_PUBLIC_URL"
	fixtureBlogURL                    = "https://blog-assets.example.com"

	envFilename                = "config.env"
	envFileContents            = "export ADMIN_PASSWORD='a long local password'\nHTTP_ADDR=127.0.0.1:9000 # local\nTRUST_FLY_PROXY=true\nR2_ACCOUNT_ID=account\nR2_ACCESS_KEY_ID=key\nR2_SECRET_ACCESS_KEY=secret\nR2_PHOTOS_BUCKET=website\nR2_PHOTOS_PUBLIC_URL=https://images.example.com\nR2_BLOG_BUCKET=blog\nR2_BLOG_PUBLIC_URL=https://blog-assets.example.com\nFRONTEND_DIR=\"assets\\twith-tab\"\n"
	applicationTestEnvArgument = "-env"
	addrArgument               = "-addr"
	overrideAddress            = ":8080"
	falseSetting               = "false"
	expectedAssetsDir          = "assets\twith-tab"
	expectedR2Endpoint         = "https://account.r2.cloudflarestorage.com"
	precedenceFailure          = "configuration precedence failed"
	emptyValue                 = ""
	emptyPasswordFailure       = "empty password accepted"
	envMutationFailure         = "process environment changed"

	missingSettingFailure = "missing setting accepted"
	invalidSetting        = "invalid"
	invalidBooleanFailure = "invalid boolean accepted"
	invalidConfigFailure  = "invalid constructor configuration accepted"
	missingFilename       = "missing"
	missingEnvFileFailure = "missing custom env file ignored"

	applicationTestEnvR2Endpoint   = "R2_ENDPOINT"
	applicationTestEnvDatabasePath = "DATABASE_PATH"
	databaseFilename               = "website.db"
	getMethod                      = "GET"
	listImagesPath                 = "/api/images"
	emptyDataJSON                  = `"data":[]`
	startupResponseFormat          = "HTTP startup: %d %s"
	applicationTestCatalogKey      = "catalog.json"
	catalogMissingFailure          = "catalog not initialized in R2"
	offlineStartupFormat           = "existing database requires R2 during startup: %v"
	emptyDatabaseFilename          = "empty.db"
	emptyImportFailure             = "first import accepted unavailable R2"

	applicationTestEnvTrustedProxyCIDRs = "TRUSTED_PROXY_CIDRS"
	trustedCIDRList                     = "192.0.2.1/32, 2001:db8::/64"
	trustedProxiesFormat                = "trusted proxies: %+v %v"
	invalidNetwork                      = "not-a-network"
	invalidProxyFailure                 = "invalid trusted proxy configuration accepted"

	trueSetting                     = "true"
	applicationTestEnvTrustFlyProxy = "TRUST_FLY_PROXY"
	flySettingFormat                = "unexpected Fly proxy setting: %v"

	shortRestoreTimeout              = "30s"
	longRestoreTimeout               = "12m"
	zeroValue                        = "0"
	zeroDuration                     = "0s"
	negativeDuration                 = "-1m"
	applicationTestEnvRestoreTimeout = "RESTORE_TIMEOUT"
	invalidTimeoutFailure            = "invalid timeout accepted"
	defaultTimeoutText               = "5m0s"
	longTimeoutText                  = "12m0s"
	durationMismatchFormat           = "got %s want %s"

	applicationTestEnvMaxRequests    = "HTTP_MAX_REQUESTS"
	applicationTestEnvMaxUploads     = "HTTP_MAX_UPLOADS"
	applicationTestEnvMaxConnections = "HTTP_MAX_CONNECTIONS"
	positiveLimit                    = "7"
	negativeLimit                    = "-1"
	overflowingLimit                 = "999999999999999999999999"
	pathSeparator                    = "/"
	invalidLimitFailure              = "invalid limit accepted"
	limitMismatchFormat              = "limit = %d; want %d"

	applicationTestEnvEphemeralDatabase    = "EPHEMERAL_DATABASE"
	postID                                 = "post"
	draftBody                              = "draft"
	onePixelPNGBase64                      = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC"
	firstPhotoName                         = "first"
	applicationTestBlackAndWhite           = "bw"
	secondPhotoName                        = "second"
	moveUp                                 = "up"
	applicationTestResurrectedPhotoFailure = "deleted photo resurrected"
	applicationTestResurrectedPostFailure  = "deleted post resurrected"
)

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
}
func validEnv() map[string]string {
	return map[string]string{applicationTestEnvAdminPassword: fixturePassword, applicationTestEnvR2AccountID: fixtureAccount, applicationTestEnvAccessKeyID: fixtureAccessKey, applicationTestEnvSecretAccessKey: fixtureSecret, applicationTestEnvPhotosBucket: fixturePhotosBucket, applicationTestEnvPhotosPublicURL: fixturePhotosURL, applicationTestEnvBlogBucket: fixtureBlogBucket, applicationTestEnvBlogPublicURL: fixtureBlogURL}
}
func TestConfigPrecedenceAndDotEnv(t *testing.T) {
	before, beforeSet := os.LookupEnv(applicationTestEnvPhotosBucket)
	file := filepath.Join(t.TempDir(), envFilename)
	raw := envFileContents
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig([]string{applicationTestEnvArgument, file, addrArgument, overrideAddress}, lookup(map[string]string{applicationTestEnvTrustFlyProxy: falseSetting}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != overrideAddress || cfg.TrustFlyProxy || cfg.FrontendDir != expectedAssetsDir || cfg.PhotoStorage.Endpoint != expectedR2Endpoint {
		t.Fatal(precedenceFailure)
	}
	if _, err := loadConfig([]string{applicationTestEnvArgument, file}, lookup(map[string]string{applicationTestEnvAdminPassword: emptyValue})); err == nil {
		t.Fatal(emptyPasswordFailure)
	}
	after, afterSet := os.LookupEnv(applicationTestEnvPhotosBucket)
	if before != after || beforeSet != afterSet {
		t.Fatal(envMutationFailure)
	}
}
func TestMissingConfigurationFails(t *testing.T) {
	for _, key := range []string{applicationTestEnvAdminPassword, applicationTestEnvR2AccountID, applicationTestEnvAccessKeyID, applicationTestEnvSecretAccessKey, applicationTestEnvPhotosBucket, applicationTestEnvPhotosPublicURL, applicationTestEnvBlogBucket, applicationTestEnvBlogPublicURL} {
		t.Run(key, func(t *testing.T) {
			env := validEnv()
			delete(env, key)
			if _, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env)); err == nil {
				t.Fatal(missingSettingFailure)
			}
		})
	}
	env := validEnv()
	env[applicationTestEnvEphemeralDatabase] = invalidSetting
	if _, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env)); err == nil {
		t.Fatal(invalidBooleanFailure)
	}
	cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.PhotoStorage = nil }, func(c *Config) { c.BlogStorage = nil }, func(c *Config) { c.Address = emptyValue }, func(c *Config) { c.FrontendDir = emptyValue }, func(c *Config) { c.AdminPassword = emptyValue }} {
		invalid := cfg
		mutate(&invalid)
		if _, err := New(invalid); err == nil {
			t.Fatal(invalidConfigFailure)
		}
	}
	if _, err := loadConfig([]string{applicationTestEnvArgument, filepath.Join(t.TempDir(), missingFilename)}, lookup(validEnv())); err == nil {
		t.Fatal(missingEnvFileFailure)
	}
}
func TestApplicationLoadsR2CatalogBeforeServing(t *testing.T) {
	objects := testutil.NewObjects()
	blog := testutil.NewObjects()
	endpoint := testutil.S3BucketsServer(map[string]*testutil.Objects{fixturePhotosBucket: objects, fixtureBlogBucket: blog})
	defer endpoint.Close()
	env := validEnv()
	env[applicationTestEnvR2Endpoint] = endpoint.URL
	env[applicationTestEnvDatabasePath] = filepath.Join(t.TempDir(), databaseFilename)
	cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Repository.Close()
	if err := app.Repository.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	app.Handler.ServeHTTP(res, httptest.NewRequest(getMethod, listImagesPath, nil))
	if res.Code != 200 || !strings.Contains(res.Body.String(), emptyDataJSON) {
		t.Fatalf(startupResponseFormat, res.Code, res.Body.String())
	}
	if _, _, err := objects.Read(context.Background(), applicationTestCatalogKey); err != nil {
		t.Fatal(catalogMissingFailure)
	}
	endpoint.Close()
	restarted, err := New(cfg)
	if err != nil {
		t.Fatalf(offlineStartupFormat, err)
	}
	restarted.Repository.Close()
	cfg.DatabasePath = filepath.Join(t.TempDir(), emptyDatabaseFilename)
	if unexpected, err := New(cfg); err == nil {
		unexpected.Repository.Close()
		t.Fatal(emptyImportFailure)
	}
}

func TestTrustedProxyConfiguration(t *testing.T) {
	env := validEnv()
	env[applicationTestEnvTrustedProxyCIDRs] = trustedCIDRList
	cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env))
	if err != nil || len(cfg.TrustedProxies) != 2 {
		t.Fatalf(trustedProxiesFormat, cfg.TrustedProxies, err)
	}
	env[applicationTestEnvTrustedProxyCIDRs] = invalidNetwork
	if _, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env)); err == nil {
		t.Fatal(invalidProxyFailure)
	}
}

func TestFlyProxyConfiguration(t *testing.T) {
	for _, value := range []string{emptyValue, falseSetting, trueSetting, invalidSetting} {
		t.Run(value, func(t *testing.T) {
			env := validEnv()
			env[applicationTestEnvTrustFlyProxy] = value
			cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env))
			if value == invalidSetting {
				if err == nil {
					t.Fatal(invalidBooleanFailure)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TrustFlyProxy != (value == trueSetting) {
				t.Fatalf(flySettingFormat, cfg.TrustFlyProxy)
			}
		})
	}
}

func TestRestoreTimeoutConfiguration(t *testing.T) {
	for _, raw := range []string{emptyValue, shortRestoreTimeout, longRestoreTimeout, zeroValue, zeroDuration, negativeDuration, invalidSetting} {
		t.Run(raw, func(t *testing.T) {
			env := validEnv()
			env[applicationTestEnvRestoreTimeout] = raw
			cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env))
			if raw == zeroValue || raw == zeroDuration || raw == negativeDuration || raw == invalidSetting {
				if err == nil {
					t.Fatal(invalidTimeoutFailure)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := defaultTimeoutText
			if raw == shortRestoreTimeout {
				want = shortRestoreTimeout
			}
			if raw == longRestoreTimeout {
				want = longTimeoutText
			}
			if cfg.RestoreTimeout.String() != want {
				t.Fatalf(durationMismatchFormat, cfg.RestoreTimeout, want)
			}
		})
	}
}

func TestConcurrencyLimitConfiguration(t *testing.T) {
	for _, name := range []string{applicationTestEnvMaxRequests, applicationTestEnvMaxUploads, applicationTestEnvMaxConnections} {
		for _, raw := range []string{emptyValue, positiveLimit, zeroValue, negativeLimit, invalidSetting, overflowingLimit} {
			t.Run(name+pathSeparator+raw, func(t *testing.T) {
				checkConcurrencyLimit(t, name, raw)
			})
		}
	}
}

func checkConcurrencyLimit(t *testing.T, name, raw string) {
	t.Helper()
	env := validEnv()
	env[name] = raw
	cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env))
	if raw != emptyValue && raw != positiveLimit {
		if err == nil {
			t.Fatal(invalidLimitFailure)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{applicationTestEnvMaxRequests: cfg.MaxRequests, applicationTestEnvMaxUploads: cfg.MaxUploads, applicationTestEnvMaxConnections: cfg.MaxConnections}[name]
	want := map[string]int{applicationTestEnvMaxRequests: 32, applicationTestEnvMaxUploads: 2, applicationTestEnvMaxConnections: 128}[name]
	if raw == positiveLimit {
		want = 7
	}
	if got != want {
		t.Fatalf(limitMismatchFormat, got, want)
	}
}

func TestEphemeralApplicationRestoresAcknowledgedWrites(t *testing.T) {
	ctx := context.Background()
	photos, blog := testutil.NewObjects(), testutil.NewObjects()
	endpoint := testutil.S3BucketsServer(map[string]*testutil.Objects{fixturePhotosBucket: photos, fixtureBlogBucket: blog})
	defer endpoint.Close()
	env := validEnv()
	env[applicationTestEnvR2Endpoint] = endpoint.URL
	env[applicationTestEnvDatabasePath] = filepath.Join(t.TempDir(), databaseFilename)
	env[applicationTestEnvEphemeralDatabase] = trueSetting
	cfg, err := loadConfig([]string{applicationTestEnvArgument, emptyValue}, lookup(env))
	if err != nil || !cfg.EphemeralDatabase {
		t.Fatal(cfg, err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Repository.Close()
	post, err := blogsqlite.New(app.Repository).CreatePost(ctx, article.Post{ID: postID, Slug: postID, Body: draftBody})
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	first, err := app.Gallery.Photos.UploadImage(ctx, photo.PhotoInput{Name: firstPhotoName, Color: applicationTestBlackAndWhite}, bytes.NewReader(png), int64(len(png)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.Gallery.Photos.UploadImage(ctx, photo.PhotoInput{Name: secondPhotoName, Color: applicationTestBlackAndWhite}, bytes.NewReader(png), int64(len(png)))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Gallery.Photos.MoveImage(ctx, first.UUID, moveUp); err != nil {
		t.Fatal(err)
	}
	// Reopen with the same configuration while the old SQLite still exists:
	// ephemeral mode must restore from R2, not reuse a local file.
	restored, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Repository.Close()
	if got, err := blogsqlite.New(restored.Repository).GetPostByID(ctx, post.ID); err != nil || got != post {
		t.Fatal(got, err)
	}
	page, err := gallerysqlite.New(restored.Repository).ListImages(ctx, domain.Pagination{Page: 1, Limit: 9}, emptyValue)
	if err != nil || len(page.Data) != 2 || page.Data[0].UUID != first.UUID {
		t.Fatal(page, err)
	}
	if err := app.Gallery.Photos.DeleteImage(ctx, second.UUID); err != nil {
		t.Fatal(err)
	}
	if err := app.Blog.Posts.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	again, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Repository.Close()
	if _, err := gallerysqlite.New(again.Repository).GetImage(ctx, second.UUID); err == nil {
		t.Fatal(applicationTestResurrectedPhotoFailure)
	}
	if _, err := blogsqlite.New(again.Repository).GetPostByID(ctx, post.ID); err == nil {
		t.Fatal(applicationTestResurrectedPostFailure)
	}
}
