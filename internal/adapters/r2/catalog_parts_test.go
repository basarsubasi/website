package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

func legacyLargeCatalog(t *testing.T) (*Repository, *testutil.Objects) {
	t.Helper()
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo, err := OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	state := repo.state.clone()
	for i := 0; i < 6; i++ {
		post := domain.Post{ID: fmt.Sprint(i), Slug: fmt.Sprint(i), Title: "Legacy", Excerpt: strings.Repeat("x", 950<<10), Body: "Original", UpdatedAt: "2026-01-01"}
		key := fmt.Sprintf("posts/%d.json", i)
		raw, _ := json.Marshal(post)
		if _, err = objects.Put(ctx, key, bytes.NewReader(raw), int64(len(raw)), "application/json", nil); err != nil {
			t.Fatal(err)
		}
		post.Body = ""
		state.Posts[post.ID] = postEntry{Metadata: post, ObjectKey: key}
	}
	raw, _ := json.Marshal(state)
	if _, err = objects.PutIf(ctx, CatalogKey, raw, repo.etag, false); err != nil {
		t.Fatal(err)
	}
	if err = repo.load(ctx); err != nil {
		t.Fatal(err)
	}
	return repo, objects
}

func TestCatalogUpgradeFailureAndAmbiguousCommit(t *testing.T) {
	ctx := context.Background()
	repo, objects := legacyLargeCatalog(t)
	before, _, _ := objects.Read(ctx, CatalogKey)
	objects.FailNext("put", errors.New("part upload failed"))
	if err := repo.commit(ctx, repo.state.clone()); err == nil {
		t.Fatal("upload failure ignored")
	}
	after, _, _ := objects.Read(ctx, CatalogKey)
	if !bytes.Equal(before, after) {
		t.Fatal("root advanced before all parts were uploaded")
	}
	objects.AmbiguousNextCommit()
	if err := repo.commit(ctx, repo.state.clone()); err != nil {
		t.Fatal("committed revision not reconciled", err)
	}
	if repo.state.Version != shardedCatalogVersion || len(repo.state.Parts) < 2 {
		t.Fatal("catalog not partitioned")
	}
	snapshot, err := NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Posts) != 6 {
		t.Fatalf("legacy restore failed: %v", err)
	}
	for _, post := range snapshot.Posts {
		if len(post.Excerpt) != 950<<10 || post.Body != "Original" {
			t.Fatal("post changed during catalog upgrade")
		}
	}
	// A cleanup-only commit must retain every referenced part.
	repo.cleanup(ctx)
	for _, key := range repo.state.Parts {
		if _, _, err := objects.Read(ctx, key); err != nil {
			t.Fatal("active part deleted", err)
		}
	}
	// Cached public reads must also understand the new manifest.
	reopened, err := OpenRepository(ctx, objects)
	if err != nil {
		t.Fatal(err)
	}
	expireCatalog(reopened)
	page, err := reopened.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, true)
	if err != nil || page.Total != 6 {
		t.Fatalf("partitioned catalog read: %d %v", page.Total, err)
	}
}

func TestCatalogPartIntegrityIsChecked(t *testing.T) {
	repo, objects := legacyLargeCatalog(t)
	if err := repo.commit(context.Background(), repo.state.clone()); err != nil {
		t.Fatal(err)
	}
	key := repo.state.Parts[0]
	raw := []byte(`{"images":{},"posts":{}}`)
	if _, err := objects.Put(context.Background(), key, bytes.NewReader(raw), int64(len(raw)), "application/json", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBackup(objects).LoadSnapshot(context.Background()); err == nil {
		t.Fatal("corrupt part accepted")
	}
}
