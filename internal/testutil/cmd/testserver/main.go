// Testserver runs an isolated application or an in-memory S3 fixture. It never
// reads .env or uses real R2 credentials, and is not included in production.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/basarsubasi/website/internal/application"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	addressFlag        = "addr"
	defaultAddress     = "127.0.0.1:18080"
	addressHelp        = "listen address"
	frontendFlag       = "frontend"
	defaultFrontendDir = "ui/dist"
	frontendHelp       = "built frontend directory"
	s3OnlyFlag         = "s3-only"
	s3OnlyHelp         = "serve only the S3 fixture"

	photosBucket         = "photos"
	blogBucket           = "blog"
	emptyString          = ""
	tempDirPrefix        = "website-ui-"
	envAdminPassword     = "ADMIN_PASSWORD"
	testAdminPassword    = "ui-test-password-only"
	envR2Endpoint        = "R2_ENDPOINT"
	envPhotosBucket      = "R2_PHOTOS_BUCKET"
	envBlogBucket        = "R2_BLOG_BUCKET"
	envAccessKeyID       = "R2_ACCESS_KEY_ID"
	testCredential       = "test"
	envSecretAccessKey   = "R2_SECRET_ACCESS_KEY"
	envPhotosPublicURL   = "R2_PHOTOS_PUBLIC_URL"
	photosPath           = "/photos"
	envBlogPublicURL     = "R2_BLOG_PUBLIC_URL"
	blogPath             = "/blog"
	falseSetting         = "false"
	envTrustFlyProxy     = "TRUST_FLY_PROXY"
	envTrustedProxyCIDRs = "TRUSTED_PROXY_CIDRS"
	envArgument          = "-env"
	addrArgument         = "-addr"
	dbArgument           = "-db"
	databaseSuffix       = "/website.db"
	frontendArgument     = "-frontend"
)

func main() {
	address := flag.String(addressFlag, defaultAddress, addressHelp)
	frontend := flag.String(frontendFlag, defaultFrontendDir, frontendHelp)
	s3Only := flag.Bool(s3OnlyFlag, false, s3OnlyHelp)
	flag.Parse()
	if err := run(*address, *frontend, *s3Only); err != nil {
		log.Fatal(err)
	}
}

func run(address, frontend string, s3Only bool) error {
	storage := testutil.S3BucketsServer(map[string]*testutil.Objects{photosBucket: testutil.NewObjects(), blogBucket: testutil.NewObjects()})
	defer storage.Close()
	if s3Only {
		return http.ListenAndServe(address, storage.Config.Handler)
	}
	dir, err := os.MkdirTemp(emptyString, tempDirPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for key, value := range map[string]string{
		envAdminPassword:     testAdminPassword,
		envR2Endpoint:        storage.URL,
		envPhotosBucket:      photosBucket,
		envBlogBucket:        blogBucket,
		envAccessKeyID:       testCredential,
		envSecretAccessKey:   testCredential,
		envPhotosPublicURL:   storage.URL + photosPath,
		envBlogPublicURL:     storage.URL + blogPath,
		envTrustFlyProxy:     falseSetting,
		envTrustedProxyCIDRs: emptyString,
	} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return application.Run([]string{envArgument, emptyString, addrArgument, address, dbArgument, dir + databaseSuffix, frontendArgument, frontend})
}
