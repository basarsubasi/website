// Testserver runs an isolated application or an in-memory S3 fixture. It never
// reads .env or uses real R2 credentials, and is not included in production.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	application "github.com/basarsubasi/website/internal"
	"github.com/basarsubasi/website/internal/testutil"
)

func main() {
	address := flag.String("addr", "127.0.0.1:18080", "listen address")
	frontend := flag.String("frontend", "frontend/dist", "built frontend directory")
	s3Only := flag.Bool("s3-only", false, "serve only the S3 fixture")
	flag.Parse()
	if err := run(*address, *frontend, *s3Only); err != nil {
		log.Fatal(err)
	}
}

func run(address, frontend string, s3Only bool) error {
	storage := testutil.S3BucketsServer(map[string]*testutil.Objects{"photos": testutil.NewObjects(), "blog": testutil.NewObjects()})
	defer storage.Close()
	if s3Only {
		return http.ListenAndServe(address, storage.Config.Handler)
	}
	dir, err := os.MkdirTemp("", "website-ui-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for key, value := range map[string]string{
		"ADMIN_PASSWORD":       "ui-test-password-only",
		"R2_ENDPOINT":          storage.URL,
		"R2_PHOTOS_BUCKET":     "photos",
		"R2_BLOG_BUCKET":       "blog",
		"R2_ACCESS_KEY_ID":     "test",
		"R2_SECRET_ACCESS_KEY": "test",
		"R2_PHOTOS_PUBLIC_URL": storage.URL + "/photos",
		"R2_BLOG_PUBLIC_URL":   storage.URL + "/blog",
		"COOKIE_SECURE":        "false",
		"TRUST_FLY_PROXY":      "false",
		"TRUSTED_PROXY_CIDRS":  "",
	} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return application.Run([]string{"-env", "", "-addr", address, "-db", dir + "/website.db", "-frontend", frontend})
}
