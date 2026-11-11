package r2_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

func TestPostBackupReplayPreservesRecordAndCleansReplacedObjects(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	post := domain.Post{
		ID: "restored-post", Slug: "restored", Title: "Original", Body: "Original body", Published: 1,
		CreatedAt: "2020-01-01T00:00:00.000000000Z", UpdatedAt: "2021-01-01T00:00:00.000000000Z",
	}
	for i := 0; i < 3; i++ {
		if i == 2 {
			post.Title = "Updated"
			post.Body = "Updated body"
			post.UpdatedAt = "2022-01-01T00:00:00.000000000Z"
		}
		if err := backup.SavePost(ctx, post); err != nil {
			t.Fatal(err)
		}
		got, err := open(t, objects).GetPostByID(ctx, post.ID)
		if err != nil || got != post {
			t.Fatalf("replay %d: got %+v, error %v; want %+v", i, got, err, post)
		}
		count := 0
		for _, key := range objects.Keys() {
			if strings.HasPrefix(key, "posts/") {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("replay %d left %d post objects; want 1", i, count)
		}
	}
	repo := open(t, objects)
	if _, err := repo.CreatePost(ctx, post); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("create overwrote restored record: %v", err)
	}
	if _, err := repo.UpdatePost(ctx, "missing", domain.PostInput{Slug: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update created missing record: %v", err)
	}
}
