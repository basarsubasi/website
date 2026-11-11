package devseed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

func TestRemoveOnlyFixturesAndRepeat(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo, err := r2.OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, repo, objects); err != nil {
		t.Fatal(err)
	}
	// Renaming fixtures must not make them escape cleanup.
	if _, err := repo.UpdatePost(ctx, samplePosts()[0].ID, domain.PostInput{Slug: "renamed", Title: "Edited", Body: "Edited"}); err != nil {
		t.Fatal(err)
	}
	photoID := uuid.NewV4().String()
	key := "photos/" + photoID + ".png"
	if _, err := objects.Put(ctx, key, strings.NewReader("photo"), 5, "image/png", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateImage(ctx, domain.Image{UUID: photoID, ObjectKey: key, Name: "Sample landscape 01", Color: "bw"}); err != nil {
		t.Fatal(err)
	}
	postID := uuid.NewV4().String()
	if _, err := repo.CreatePost(ctx, domain.Post{ID: postID, Slug: "seed-welcome", Title: "Mine", Body: "Keep me"}); err != nil {
		t.Fatal(err)
	}
	if count, err := Remove(ctx, repo); err != nil || count != 16 {
		t.Fatalf("remove: count=%d err=%v", count, err)
	}
	if count, err := Remove(ctx, repo); err != nil || count != 0 {
		t.Fatalf("repeat: count=%d err=%v", count, err)
	}
	restarted, err := r2.OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	photos, err := restarted.ListImages(ctx, domain.Pagination{Page: 1, Limit: 100}, "")
	if err != nil || photos.Total != 1 || photos.Data[0].UUID != photoID {
		t.Fatalf("non-fixture photo lost: %+v %v", photos, err)
	}
	posts, err := restarted.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 100}, true)
	if err != nil || posts.Total != 1 || posts.Data[0].ID != postID || posts.Data[0].Body != "Keep me" {
		t.Fatalf("non-fixture post lost: %+v %v", posts, err)
	}
	if keys := objects.Keys(); len(keys) != 3 {
		t.Fatalf("expected catalog and two non-fixture objects, got %v", keys)
	}
}

func TestRemoveCanResumeAfterFailure(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo, err := r2.OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, repo, objects); err != nil {
		t.Fatal(err)
	}
	// Simulate a previous partial cleanup followed by an interrupted catalog write.
	if err := repo.DeleteImage(ctx, photoIDs[0]); err != nil {
		t.Fatal(err)
	}
	objects.FailNext("write", domain.ErrConflict)
	if count, err := Remove(ctx, repo); count != 0 || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict: count=%d err=%v", count, err)
	}
	if count, err := Remove(ctx, repo); err != nil || count != 15 {
		t.Fatalf("resume: count=%d err=%v", count, err)
	}
	if keys := objects.Keys(); len(keys) != 1 || keys[0] != r2.CatalogKey {
		t.Fatalf("seed objects remain: %v", keys)
	}
	if count, err := Seed(ctx, repo, objects); err != nil || count != 16 {
		t.Fatalf("reseed: count=%d err=%v", count, err)
	}
}
