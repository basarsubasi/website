package application

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/testutil"
)

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
}
func validEnv() map[string]string {
	return map[string]string{"ADMIN_PASSWORD": "test-password-for-admin", "R2_ACCOUNT_ID": "account", "R2_ACCESS_KEY_ID": "key", "R2_SECRET_ACCESS_KEY": "secret", "R2_PHOTOS_BUCKET": "website", "R2_PHOTOS_PUBLIC_URL": "https://images.example.com", "R2_BLOG_BUCKET": "blog", "R2_BLOG_PUBLIC_URL": "https://blog-assets.example.com"}
}
func TestConfigPrecedenceAndDotEnv(t *testing.T) {
	before, beforeSet := os.LookupEnv("R2_PHOTOS_BUCKET")
	file := filepath.Join(t.TempDir(), "config.env")
	raw := "export ADMIN_PASSWORD='a long local password'\nHTTP_ADDR=127.0.0.1:9000 # local\nCOOKIE_SECURE=true\nR2_ACCOUNT_ID=account\nR2_ACCESS_KEY_ID=key\nR2_SECRET_ACCESS_KEY=secret\nR2_PHOTOS_BUCKET=website\nR2_PHOTOS_PUBLIC_URL=https://images.example.com\nR2_BLOG_BUCKET=blog\nR2_BLOG_PUBLIC_URL=https://blog-assets.example.com\nFRONTEND_DIR=\"assets\\twith-tab\"\n"
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig([]string{"-env", file, "-addr", ":8080"}, lookup(map[string]string{"COOKIE_SECURE": "false"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != ":8080" || cfg.SecureCookies || cfg.FrontendDir != "assets\twith-tab" || cfg.PhotoStorage.Endpoint != "https://account.r2.cloudflarestorage.com" {
		t.Fatal("configuration precedence failed")
	}
	if _, err := loadConfig([]string{"-env", file}, lookup(map[string]string{"ADMIN_PASSWORD": ""})); err == nil {
		t.Fatal("empty password accepted")
	}
	after, afterSet := os.LookupEnv("R2_PHOTOS_BUCKET")
	if before != after || beforeSet != afterSet {
		t.Fatal("process environment changed")
	}
}
func TestMissingConfigurationFails(t *testing.T) {
	for _, key := range []string{"ADMIN_PASSWORD", "R2_ACCOUNT_ID", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "R2_PHOTOS_BUCKET", "R2_PHOTOS_PUBLIC_URL", "R2_BLOG_BUCKET", "R2_BLOG_PUBLIC_URL"} {
		t.Run(key, func(t *testing.T) {
			env := validEnv()
			delete(env, key)
			if _, err := loadConfig([]string{"-env", ""}, lookup(env)); err == nil {
				t.Fatal("missing setting accepted")
			}
		})
	}
	env := validEnv()
	env["COOKIE_SECURE"] = "invalid"
	if _, err := loadConfig([]string{"-env", ""}, lookup(env)); err == nil {
		t.Fatal("invalid boolean accepted")
	}
	cfg, err := loadConfig([]string{"-env", ""}, lookup(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.PhotoStorage = nil }, func(c *Config) { c.BlogStorage = nil }, func(c *Config) { c.Address = "" }, func(c *Config) { c.FrontendDir = "" }, func(c *Config) { c.AdminPassword = "" }} {
		invalid := cfg
		mutate(&invalid)
		if _, err := New(invalid); err == nil {
			t.Fatal("invalid constructor configuration accepted")
		}
	}
	if _, err := loadConfig([]string{"-env", filepath.Join(t.TempDir(), "missing")}, lookup(validEnv())); err == nil {
		t.Fatal("missing custom env file ignored")
	}
}
func TestApplicationLoadsR2CatalogBeforeServing(t *testing.T) {
	objects := testutil.NewObjects()
	blog := testutil.NewObjects()
	endpoint := testutil.S3BucketsServer(map[string]*testutil.Objects{"website": objects, "blog": blog})
	defer endpoint.Close()
	env := validEnv()
	env["R2_ENDPOINT"] = endpoint.URL
	env["DATABASE_PATH"] = filepath.Join(t.TempDir(), "website.db")
	cfg, err := loadConfig([]string{"-env", ""}, lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Repository.Close()
	if err := app.Service.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	app.Handler.ServeHTTP(res, httptest.NewRequest("GET", "/api/images", nil))
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"data":[]`) {
		t.Fatalf("HTTP startup: %d %s", res.Code, res.Body.String())
	}
	if _, _, err := objects.Read(context.Background(), "catalog.json"); err != nil {
		t.Fatal("catalog not initialized in R2")
	}
	endpoint.Close()
	restarted, err := New(cfg)
	if err != nil {
		t.Fatalf("existing database requires R2 during startup: %v", err)
	}
	restarted.Repository.Close()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "empty.db")
	if unexpected, err := New(cfg); err == nil {
		unexpected.Repository.Close()
		t.Fatal("first import accepted unavailable R2")
	}
}

func TestTrustedProxyConfiguration(t *testing.T) {
	env := validEnv()
	env["TRUSTED_PROXY_CIDRS"] = "192.0.2.1/32, 2001:db8::/64"
	cfg, err := loadConfig([]string{"-env", ""}, lookup(env))
	if err != nil || len(cfg.TrustedProxies) != 2 {
		t.Fatalf("trusted proxies: %+v %v", cfg.TrustedProxies, err)
	}
	env["TRUSTED_PROXY_CIDRS"] = "not-a-network"
	if _, err := loadConfig([]string{"-env", ""}, lookup(env)); err == nil {
		t.Fatal("invalid trusted proxy configuration accepted")
	}
}

func TestFlyProxyConfiguration(t *testing.T) {
	for _, value := range []string{"", "false", "true", "invalid"} {
		t.Run(value, func(t *testing.T) {
			env := validEnv()
			env["TRUST_FLY_PROXY"] = value
			cfg, err := loadConfig([]string{"-env", ""}, lookup(env))
			if value == "invalid" {
				if err == nil {
					t.Fatal("invalid boolean accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TrustFlyProxy != (value == "true") {
				t.Fatalf("unexpected Fly proxy setting: %v", cfg.TrustFlyProxy)
			}
		})
	}
}

func TestRestoreTimeoutConfiguration(t *testing.T) {
	for _, raw := range []string{"", "30s", "12m", "0", "0s", "-1m", "invalid"} {
		t.Run(raw, func(t *testing.T) {
			env := validEnv()
			env["RESTORE_TIMEOUT"] = raw
			cfg, err := loadConfig([]string{"-env", ""}, lookup(env))
			if raw == "0" || raw == "0s" || raw == "-1m" || raw == "invalid" {
				if err == nil {
					t.Fatal("invalid timeout accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "5m0s"
			if raw == "30s" {
				want = "30s"
			}
			if raw == "12m" {
				want = "12m0s"
			}
			if cfg.RestoreTimeout.String() != want {
				t.Fatalf("got %s want %s", cfg.RestoreTimeout, want)
			}
		})
	}
}

func TestConcurrencyLimitConfiguration(t *testing.T) {
	for _, name := range []string{"HTTP_MAX_REQUESTS", "HTTP_MAX_UPLOADS", "HTTP_MAX_CONNECTIONS"} {
		for _, raw := range []string{"", "7", "0", "-1", "invalid", "999999999999999999999999"} {
			t.Run(name+"/"+raw, func(t *testing.T) {
				env := validEnv()
				env[name] = raw
				cfg, err := loadConfig([]string{"-env", ""}, lookup(env))
				if raw != "" && raw != "7" {
					if err == nil {
						t.Fatal("invalid limit accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				got := map[string]int{"HTTP_MAX_REQUESTS": cfg.MaxRequests, "HTTP_MAX_UPLOADS": cfg.MaxUploads, "HTTP_MAX_CONNECTIONS": cfg.MaxConnections}[name]
				want := map[string]int{"HTTP_MAX_REQUESTS": 32, "HTTP_MAX_UPLOADS": 2, "HTTP_MAX_CONNECTIONS": 128}[name]
				if raw == "7" {
					want = 7
				}
				if got != want {
					t.Fatalf("limit = %d; want %d", got, want)
				}
			})
		}
	}
}
