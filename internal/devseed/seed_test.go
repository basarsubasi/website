package devseed

import (
	"bytes"
	"context"
	"errors"
	"github.com/basarsubasi/website/internal/adapters/database"
	"image/png"
	"path/filepath"
	"testing"
	"uuid"

	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

func TestSeedIsRepeatableAndPersistsInR2(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo, err := r2.OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	count, err := Seed(ctx, repo, objects)
	if err != nil || count != 16 {
		t.Fatalf("seed: %d %v", count, err)
	}
	first, err := repo.GetImage(ctx, photoIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateImage(ctx, first.UUID, domain.PhotoInput{Name: "Edited", Color: first.Color}); err != nil {
		t.Fatal(err)
	}
	count, err = Seed(ctx, repo, objects)
	if err != nil || count != 0 {
		t.Fatalf("repeat seed: %d %v", count, err)
	}
	restarted, err := r2.OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	first, err = restarted.GetImage(ctx, photoIDs[0])
	if err != nil || first.Name != "Edited" || first.ObjectKey == "" {
		t.Fatal("edit or managed file missing after restart")
	}
	raw, _, err := objects.Read(ctx, first.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	for _, color := range []string{"bw", "color"} {
		page, err := restarted.ListImages(ctx, domain.Pagination{Page: 1, Limit: 100}, color)
		if err != nil || page.Total != 6 {
			t.Fatal("seed color coverage missing")
		}
		for _, photo := range page.Data {
			value, err := uuid.Parse(photo.UUID)
			if err != nil || value[6]>>4 != 4 {
				t.Fatal("invalid seed UUID")
			}
		}
	}
	posts, err := restarted.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 100}, true)
	if err != nil || posts.Total != 4 {
		t.Fatal("posts missing")
	}
	published, err := restarted.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 100}, false)
	if err != nil || published.Total != 3 {
		t.Fatal("draft exposed through public listing")
	}
}
func TestSeedRejectsSlugCollisionBeforeUploading(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo, err := r2.OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "seed-welcome", Title: "Mine", Body: "My writing"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(objects.Keys())
	if _, err := Seed(ctx, repo, objects); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("slug conflict ignored")
	}
	if len(objects.Keys()) != before {
		t.Fatal("seed wrote objects before rejecting collision")
	}
}

func TestSQLiteSeedAndCleanupWithR2Backups(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo, err := database.Open(ctx, filepath.Join(t.TempDir(), "website.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Initialize(ctx, backup.LoadSnapshot); err != nil {
		t.Fatal(err)
	}
	if count, err := Seed(ctx, repo, objects); err != nil || count != 16 {
		t.Fatalf("seed: %d %v", count, err)
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backup.LoadSnapshot(ctx)
	if err != nil || len(snapshot.Images) != 12 || len(snapshot.Posts) != 4 {
		t.Fatalf("backup: %+v %v", snapshot, err)
	}
	if count, err := Remove(ctx, repo); err != nil || count != 16 {
		t.Fatalf("cleanup: %d %v", count, err)
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if keys := objects.Keys(); len(keys) != 1 || keys[0] != r2.CatalogKey {
		t.Fatalf("seed objects remain: %v", keys)
	}
	if count, err := Remove(ctx, repo); err != nil || count != 0 {
		t.Fatalf("repeat cleanup: %d %v", count, err)
	}
}
