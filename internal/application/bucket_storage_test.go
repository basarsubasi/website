package application

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	galleryphoto "github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	photosBucket                             = "photos"
	blogBucket                               = "blog"
	bucketStorageTestEnvR2Endpoint           = "R2_ENDPOINT"
	bucketStorageTestEnvPhotosBucket         = "R2_PHOTOS_BUCKET"
	bucketStorageTestEnvPhotosPublicURL      = "R2_PHOTOS_PUBLIC_URL"
	photosPath                               = "/photos"
	bucketStorageTestEnvBlogPublicURL        = "R2_BLOG_PUBLIC_URL"
	blogPath                                 = "/blog"
	bucketStorageTestEnvDatabasePath         = "DATABASE_PATH"
	originalFilename                         = "original.db"
	bucketStorageTestEnvArgument             = "-env"
	emptyEnvFile                             = ""
	galleryName                              = "Gallery"
	colorPhoto                               = "color"
	missingDimensionsFailure                 = "upload dimensions missing"
	photoURLPrefix                           = "/photos/photos/"
	blogImageURLPrefix                       = "/blog/images/"
	wrongBucketsFormat                       = "wrong bucket URLs: %s %s"
	postSlug                                 = "bucket-test"
	postTitle                                = "Post"
	postKeyPrefix                            = "posts/"
	imageKeyPrefix                           = "images/"
	misplacedBlogObjectFormat                = "blog object in photos bucket: %s"
	photoKeyPrefix                           = "photos/"
	misplacedPhotoFormat                     = "gallery object in blog bucket: %s"
	missingPostFailure                       = "post missing from blog bucket"
	bucketStorageTestCatalogKey              = "catalog.json"
	missingCatalogFailure                    = "missing bucket catalog"
	blogBucketPrefix                         = "/blog/"
	missingImageFailure                      = "blog image not stored"
	restoredFilename                         = "restored.db"
	photoRestoreFailure                      = "photo restore failed"
	postRestoreFailure                       = "post restore failed"
	editedName                               = "Edited"
	bucketStorageTestBlackAndWhite           = "bw"
	wrongMetadataBucketFailure               = "photo metadata routed to wrong bucket"
	emptyFilename                            = "empty.db"
	bucketStorageTestResurrectedPhotoFailure = "deleted gallery photo restored"
	bucketStorageTestResurrectedPostFailure  = "deleted blog post restored"
	blogImageCleanupFailure                  = "gallery cleanup affected blog image"

	bucketStorageTestEnvBlogBucket = "R2_BLOG_BUCKET"
	sharedBucketFailure            = "same bucket accepted"
)

func TestSeparateBucketsUploadBackupRestoreAndDelete(t *testing.T) {
	ctx := context.Background()
	photos, blog := testutil.NewObjects(), testutil.NewObjects()
	endpoint := testutil.S3BucketsServer(map[string]*testutil.Objects{photosBucket: photos, blogBucket: blog})
	defer endpoint.Close()
	env := validEnv()
	env[bucketStorageTestEnvR2Endpoint] = endpoint.URL
	env[bucketStorageTestEnvPhotosBucket] = photosBucket
	env[bucketStorageTestEnvPhotosPublicURL] = endpoint.URL + photosPath
	env[bucketStorageTestEnvBlogPublicURL] = endpoint.URL + blogPath
	env[bucketStorageTestEnvDatabasePath] = filepath.Join(t.TempDir(), originalFilename)
	cfg, err := loadConfig([]string{bucketStorageTestEnvArgument, emptyEnvFile}, lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Repository.Close()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	photo, err := app.Gallery.UploadImage(ctx, galleryphoto.PhotoInput{Name: galleryName, Color: colorPhoto}, bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	blogImage, err := app.Blog.Images.UploadBlogImage(ctx, bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	imageURL := blogImage.URL
	if photo.Width != 2 || photo.Height != 2 || blogImage.Width != 2 || blogImage.Height != 2 {
		t.Fatal(missingDimensionsFailure)
	}
	if !strings.HasPrefix(photo.ImageURL, endpoint.URL+photoURLPrefix) || !strings.HasPrefix(imageURL, endpoint.URL+blogImageURLPrefix) {
		t.Fatalf(wrongBucketsFormat, photo.ImageURL, imageURL)
	}
	post, err := app.Blog.SavePost(ctx, emptyEnvFile, article.PostInput{Slug: postSlug, Title: postTitle, Body: imageURL, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Repository.SyncBackups(ctx, app.Backup); err != nil {
		t.Fatal(err)
	}
	checkBucketContents(t, ctx, photos, blog)
	blogImageKey := strings.TrimPrefix(imageURL, endpoint.URL+blogBucketPrefix)
	if raw, _, err := blog.Read(ctx, blogImageKey); err != nil || !bytes.Equal(raw, data.Bytes()) {
		t.Fatal(missingImageFailure, err)
	}
	restoreAndDelete(t, ctx, cfg, photo, post)
	checkDeletedBucketRecords(t, ctx, cfg, blog, blogImageKey)
}

func TestBucketsMustBeDistinct(t *testing.T) {
	env := validEnv()
	env[bucketStorageTestEnvBlogBucket] = env[bucketStorageTestEnvPhotosBucket]
	if _, err := loadConfig([]string{bucketStorageTestEnvArgument, emptyEnvFile}, lookup(env)); err == nil {
		t.Fatal(sharedBucketFailure)
	}
}

func checkBucketContents(t *testing.T, ctx context.Context, photos, blog *testutil.Objects) {
	t.Helper()
	for _, key := range photos.Keys() {
		if strings.HasPrefix(key, postKeyPrefix) || strings.HasPrefix(key, imageKeyPrefix) {
			t.Fatalf(misplacedBlogObjectFormat, key)
		}
	}
	foundPost := false
	for _, key := range blog.Keys() {
		if strings.HasPrefix(key, photoKeyPrefix) {
			t.Fatalf(misplacedPhotoFormat, key)
		}
		if strings.HasPrefix(key, postKeyPrefix) {
			foundPost = true
		}
	}
	if !foundPost {
		t.Fatal(missingPostFailure)
	}
	for _, store := range []*testutil.Objects{photos, blog} {
		if _, _, err := store.Read(ctx, bucketStorageTestCatalogKey); err != nil {
			t.Fatal(missingCatalogFailure, err)
		}
	}
}

func restoreAndDelete(t *testing.T, ctx context.Context, cfg Config, photo galleryphoto.Image, post article.Post) {
	t.Helper()
	cfg.DatabasePath = filepath.Join(t.TempDir(), restoredFilename)
	restored, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Repository.Close()
	if got, err := restored.Gallery.GetImage(ctx, photo.UUID); err != nil || got != photo {
		t.Fatal(photoRestoreFailure, err)
	}
	if got, err := restored.Blog.GetPost(ctx, post.Slug); err != nil || got != post {
		t.Fatal(postRestoreFailure, err)
	}
	if _, err := restored.Gallery.UpdateImage(ctx, photo.UUID, galleryphoto.PhotoInput{Name: editedName, Color: bucketStorageTestBlackAndWhite}); err != nil {
		t.Fatal(err)
	}
	if err := restored.Repository.SyncBackups(ctx, restored.Backup); err != nil {
		t.Fatal(wrongMetadataBucketFailure, err)
	}
	if err := restored.Gallery.DeleteImage(ctx, photo.UUID); err != nil {
		t.Fatal(err)
	}
	if err := restored.Blog.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if err := restored.Repository.SyncBackups(ctx, restored.Backup); err != nil {
		t.Fatal(err)
	}
}

func checkDeletedBucketRecords(t *testing.T, ctx context.Context, cfg Config, blog *testutil.Objects, blogImageKey string) {
	t.Helper()
	cfg.DatabasePath = filepath.Join(t.TempDir(), emptyFilename)
	empty, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Repository.Close()
	imagePage, err := empty.Gallery.ListImages(ctx, domain.Pagination{Page: 1, Limit: 10}, emptyEnvFile)
	if err != nil || imagePage.Total != 0 {
		t.Fatal(bucketStorageTestResurrectedPhotoFailure, err)
	}
	postPage, err := empty.Blog.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, true)
	if err != nil || postPage.Total != 0 {
		t.Fatal(bucketStorageTestResurrectedPostFailure, err)
	}
	if _, _, err := blog.Read(ctx, blogImageKey); err != nil {
		t.Fatal(blogImageCleanupFailure, err)
	}
}
