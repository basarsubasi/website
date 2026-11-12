package application

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

func TestSeparateBucketsUploadBackupRestoreAndDelete(t *testing.T) {
	ctx := context.Background()
	photos, blog := testutil.NewObjects(), testutil.NewObjects()
	endpoint := testutil.S3BucketsServer(map[string]*testutil.Objects{"photos": photos, "blog": blog})
	defer endpoint.Close()
	env := validEnv()
	env["R2_ENDPOINT"] = endpoint.URL
	env["R2_PHOTOS_BUCKET"] = "photos"
	env["R2_PHOTOS_PUBLIC_URL"] = endpoint.URL + "/photos"
	env["R2_BLOG_PUBLIC_URL"] = endpoint.URL + "/blog"
	env["DATABASE_PATH"] = filepath.Join(t.TempDir(), "original.db")
	cfg, err := loadConfig([]string{"-env", ""}, lookup(env))
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
	photo, err := app.Service.UploadImage(ctx, domain.PhotoInput{Name: "Gallery", Color: "color"}, bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	imageURL, err := app.Service.UploadBlogImage(ctx, bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(photo.ImageURL, endpoint.URL+"/photos/photos/") || !strings.HasPrefix(imageURL, endpoint.URL+"/blog/images/") {
		t.Fatalf("wrong bucket URLs: %s %s", photo.ImageURL, imageURL)
	}
	post, err := app.Service.SavePost(ctx, "", domain.PostInput{Slug: "bucket-test", Title: "Post", Body: imageURL, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Repository.SyncBackups(ctx, app.Backup); err != nil {
		t.Fatal(err)
	}
	for _, key := range photos.Keys() {
		if strings.HasPrefix(key, "posts/") || strings.HasPrefix(key, "images/") {
			t.Fatalf("blog object in photos bucket: %s", key)
		}
	}
	foundPost := false
	for _, key := range blog.Keys() {
		if strings.HasPrefix(key, "photos/") {
			t.Fatalf("gallery object in blog bucket: %s", key)
		}
		if strings.HasPrefix(key, "posts/") {
			foundPost = true
		}
	}
	if !foundPost {
		t.Fatal("post missing from blog bucket")
	}
	for _, store := range []*testutil.Objects{photos, blog} {
		if _, _, err := store.Read(ctx, "catalog.json"); err != nil {
			t.Fatal("missing bucket catalog", err)
		}
	}
	blogImageKey := strings.TrimPrefix(imageURL, endpoint.URL+"/blog/")
	if raw, _, err := blog.Read(ctx, blogImageKey); err != nil || !bytes.Equal(raw, data.Bytes()) {
		t.Fatal("blog image not stored", err)
	}
	cfg.DatabasePath = filepath.Join(t.TempDir(), "restored.db")
	restored, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Repository.Close()
	if got, err := restored.Service.GetImage(ctx, photo.UUID); err != nil || got != photo {
		t.Fatal("photo restore failed", err)
	}
	if got, err := restored.Service.GetPost(ctx, post.Slug); err != nil || got != post {
		t.Fatal("post restore failed", err)
	}
	if _, err := restored.Service.UpdateImage(ctx, photo.UUID, domain.PhotoInput{Name: "Edited", Color: "bw"}); err != nil {
		t.Fatal(err)
	}
	if err := restored.Repository.SyncBackups(ctx, restored.Backup); err != nil {
		t.Fatal("photo metadata routed to wrong bucket", err)
	}
	if err := restored.Service.DeleteImage(ctx, photo.UUID); err != nil {
		t.Fatal(err)
	}
	if err := restored.Service.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if err := restored.Repository.SyncBackups(ctx, restored.Backup); err != nil {
		t.Fatal(err)
	}
	cfg.DatabasePath = filepath.Join(t.TempDir(), "empty.db")
	empty, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Repository.Close()
	imagePage, err := empty.Service.ListImages(ctx, domain.Pagination{Page: 1, Limit: 10}, "")
	if err != nil || imagePage.Total != 0 {
		t.Fatal("deleted gallery photo restored", err)
	}
	postPage, err := empty.Service.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, true)
	if err != nil || postPage.Total != 0 {
		t.Fatal("deleted blog post restored", err)
	}
	if _, _, err := blog.Read(ctx, blogImageKey); err != nil {
		t.Fatal("gallery cleanup affected blog image", err)
	}
}

func TestBucketsMustBeDistinct(t *testing.T) {
	env := validEnv()
	env["R2_BLOG_BUCKET"] = env["R2_PHOTOS_BUCKET"]
	if _, err := loadConfig([]string{"-env", ""}, lookup(env)); err == nil {
		t.Fatal("same bucket accepted")
	}
}
