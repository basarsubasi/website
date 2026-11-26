package r2_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	restoredPostID            = "restored-post"
	restoredSlug              = "restored"
	originalTitle             = "Original"
	originalBody              = "Original body"
	originalCreationTime      = "2020-01-01T00:00:00.000000000Z"
	originalUpdateTime        = "2021-01-01T00:00:00.000000000Z"
	backupTestUpdatedTitle    = "Updated"
	updatedBody               = "Updated body"
	updatedTime               = "2022-01-01T00:00:00.000000000Z"
	replayMismatchFormat      = "replay %d: got %+v, error %v; want %+v"
	backupTestPostsPrefix     = "posts/"
	replayObjectCountFormat   = "replay %d left %d post objects; want 1"
	restoredOverwriteFormat   = "create overwrote restored record: %v"
	missingID                 = "missing"
	updateCreatedRecordFormat = "update created missing record: %v"
)

func TestPostBackupReplayPreservesRecordAndCleansReplacedObjects(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	post := article.Post{
		ID: restoredPostID, Slug: restoredSlug, Title: originalTitle, Body: originalBody, Published: 1,
		CreatedAt: originalCreationTime, UpdatedAt: originalUpdateTime,
	}
	for i := 0; i < 3; i++ {
		if i == 2 {
			post.Title = backupTestUpdatedTitle
			post.Body = updatedBody
			post.UpdatedAt = updatedTime
		}
		if err := backup.SavePost(ctx, post); err != nil {
			t.Fatal(err)
		}
		got, err := open(t, objects).GetPostByID(ctx, post.ID)
		if err != nil || got != post {
			t.Fatalf(replayMismatchFormat, i, got, err, post)
		}
		count := 0
		for _, key := range objects.Keys() {
			if strings.HasPrefix(key, backupTestPostsPrefix) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf(replayObjectCountFormat, i, count)
		}
	}
	repo := open(t, objects)
	if _, err := repo.CreatePost(ctx, post); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf(restoredOverwriteFormat, err)
	}
	if _, err := repo.UpdatePost(ctx, missingID, article.PostInput{Slug: missingID}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf(updateCreatedRecordFormat, err)
	}
}
