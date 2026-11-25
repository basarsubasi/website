package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	legacyTitle                     = "Legacy"
	bodyPadding                     = "x"
	originalBody                    = "Original"
	createdAtValue                  = "2026-01-01"
	postKeyFormat                   = "posts/%d.json"
	catalogPartsTestJsonContentType = "application/json"
	catalogPartsTestEmptyValue      = ""

	putOperation                = "put"
	partUploadFailedMessage     = "part upload failed"
	ignoredFailureMessage       = "upload failure ignored"
	prematureRootUpdateMessage  = "root advanced before all parts were uploaded"
	unreconciledRevisionMessage = "committed revision not reconciled"
	unpartitionedCatalogMessage = "catalog not partitioned"
	legacyRestoreErrorFormat    = "legacy restore failed: %v"
	changedPostMessage          = "post changed during catalog upgrade"
	activePartDeletedMessage    = "active part deleted"
	partitionReadErrorFormat    = "partitioned catalog read: %d %v"

	emptyPartJSON              = `{"images":{},"posts":{}}`
	corruptPartAcceptedMessage = "corrupt part accepted"
)

var errPartUploadFailed = errors.New(partUploadFailedMessage)

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
		post := article.Post{ID: fmt.Sprint(i), Slug: fmt.Sprint(i), Title: legacyTitle, Excerpt: strings.Repeat(bodyPadding, 950<<10), Body: originalBody, UpdatedAt: createdAtValue}
		key := fmt.Sprintf(postKeyFormat, i)
		raw, _ := json.Marshal(post)
		if _, err = objects.Put(ctx, key, bytes.NewReader(raw), int64(len(raw)), catalogPartsTestJsonContentType, nil); err != nil {
			t.Fatal(err)
		}
		post.Body = catalogPartsTestEmptyValue
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
	objects.FailNext(putOperation, errPartUploadFailed)
	if err := repo.commit(ctx, repo.state.clone()); err == nil {
		t.Fatal(ignoredFailureMessage)
	}
	after, _, _ := objects.Read(ctx, CatalogKey)
	if !bytes.Equal(before, after) {
		t.Fatal(prematureRootUpdateMessage)
	}
	objects.AmbiguousNextCommit()
	if err := repo.commit(ctx, repo.state.clone()); err != nil {
		t.Fatal(unreconciledRevisionMessage, err)
	}
	if repo.state.Version != shardedCatalogVersion || len(repo.state.Parts) < 2 {
		t.Fatal(unpartitionedCatalogMessage)
	}
	snapshot, err := NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Posts) != 6 {
		t.Fatalf(legacyRestoreErrorFormat, err)
	}
	for _, post := range snapshot.Posts {
		if len(post.Excerpt) != 950<<10 || post.Body != originalBody {
			t.Fatal(changedPostMessage)
		}
	}
	// A cleanup-only commit must retain every referenced part.
	repo.cleanup(ctx)
	for _, key := range repo.state.Parts {
		if _, _, err := objects.Read(ctx, key); err != nil {
			t.Fatal(activePartDeletedMessage, err)
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
		t.Fatalf(partitionReadErrorFormat, page.Total, err)
	}
}

func TestCatalogPartIntegrityIsChecked(t *testing.T) {
	repo, objects := legacyLargeCatalog(t)
	if err := repo.commit(context.Background(), repo.state.clone()); err != nil {
		t.Fatal(err)
	}
	key := repo.state.Parts[0]
	raw := []byte(emptyPartJSON)
	if _, err := objects.Put(context.Background(), key, bytes.NewReader(raw), int64(len(raw)), catalogPartsTestJsonContentType, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBackup(objects).LoadSnapshot(context.Background()); err == nil {
		t.Fatal(corruptPartAcceptedMessage)
	}
}
