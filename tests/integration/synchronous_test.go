package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/module/gallery/usecase/photos"
	"github.com/basarsubasi/website/internal/platform/backup"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	synchronousTestDatabaseFilename = "site.db"
	postID                          = "post"
	savedBody                       = "saved"
	earlyReturnFormat               = "returned before R2: %v"
	blockedReadFailure              = "read blocked"
	synchronousTestRestoredFilename = "restored.db"

	originalBody               = "original"
	uncertainWriteFormat       = "expected uncertain write, got %v"
	synchronousTestUpdatedBody = "updated"

	flushedCountFormat = "only flushed %d"
	lastPostID         = "last"
	skippedLockFormat  = "skipped held lock: %v"

	onePixelPNGBase64            = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC"
	metadataOperation            = "metadata"
	r2UnavailableMessage         = "R2 unavailable"
	synchronousTestBlackAndWhite = "bw"
	prematureCleanupFailure      = "upload incorrectly cleaned up"
)

var errR2Unavailable = errors.New(r2UnavailableMessage)

func TestSynchronousWriteWaitsForR2WithoutBlockingReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo := open(t, filepath.Join(t.TempDir(), synchronousTestDatabaseFilename), backup)
	slow := &slowBackup{Target: backup, started: make(chan struct{}), release: make(chan struct{})}
	repo.RequireBackup(slow)
	result := make(chan error, 1)
	go func() {
		_, err := repo.blog.CreatePost(ctx, article.Post{ID: postID, Slug: postID, Body: savedBody, Published: 1})
		result <- err
	}()
	select {
	case <-slow.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-result:
		t.Fatalf(earlyReturnFormat, err)
	default:
	}
	if _, err := repo.blog.GetPost(ctx, postID); err != nil {
		t.Fatal(blockedReadFailure, err)
	}
	close(slow.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), synchronousTestRestoredFilename), r2.NewBackup(objects))
	if got, err := restored.blog.GetPost(ctx, postID); err != nil || got.Body != savedBody {
		t.Fatal(got, err)
	}
}

func TestSynchronousFailureRetainsQueueAndRetry(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo := open(t, filepath.Join(t.TempDir(), synchronousTestDatabaseFilename), backup)
	lost := &lostReply{Target: backup, fail: true}
	repo.RequireBackup(lost)
	if _, err := repo.blog.CreatePost(ctx, article.Post{ID: postID, Slug: postID, Body: originalBody}); !errors.Is(err, domain.ErrUncertain) {
		t.Fatalf(uncertainWriteFormat, err)
	}
	if count, err := repo.PendingBackups(ctx); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	lost.fail = false
	if _, err := repo.blog.UpdatePost(ctx, postID, article.PostInput{Slug: postID, Body: synchronousTestUpdatedBody}); err != nil {
		t.Fatal(err)
	}
	if count, err := repo.PendingBackups(ctx); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	restored := open(t, filepath.Join(t.TempDir(), synchronousTestRestoredFilename), r2.NewBackup(objects))
	if got, err := restored.blog.GetPostByID(ctx, postID); err != nil || got.Body != synchronousTestUpdatedBody {
		t.Fatal(got, err)
	}
}

type countBackup struct {
	backup.Target
	posts int
}

func (b *countBackup) SavePost(context.Context, article.Post) error { b.posts++; return nil }

func TestFlushDrainsMoreThanOneBatchAndWaitsForLock(t *testing.T) {
	ctx := context.Background()
	backup := r2.NewBackup(testutil.NewObjects())
	repo := open(t, filepath.Join(t.TempDir(), synchronousTestDatabaseFilename), backup)
	for i := 0; i < 105; i++ {
		id := fmt.Sprint(i)
		if _, err := repo.blog.CreatePost(ctx, article.Post{ID: id, Slug: id}); err != nil {
			t.Fatal(err)
		}
	}
	count := &countBackup{Target: backup}
	if err := repo.FlushBackups(ctx, count); err != nil {
		t.Fatal(err)
	}
	if count.posts != 105 {
		t.Fatalf(flushedCountFormat, count.posts)
	}
	if _, err := repo.blog.CreatePost(ctx, article.Post{ID: lastPostID, Slug: lastPostID}); err != nil {
		t.Fatal(err)
	}
	slow := &slowBackup{Target: backup, started: make(chan struct{}), release: make(chan struct{})}
	workerCtx, stop := context.WithCancel(ctx)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- repo.SyncBackups(workerCtx, slow) }()
	<-slow.started
	waitCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	if err := repo.FlushBackups(waitCtx, backup); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(skippedLockFormat, err)
	}
	close(slow.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := repo.FlushBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
}

func TestUnconfirmedPhotoBackupDoesNotDeleteUploadedObject(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo := open(t, filepath.Join(t.TempDir(), synchronousTestDatabaseFilename), backup)
	repo.RequireBackup(backup)
	service := photos.New(repo.gallery, objects)
	raw, _ := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	objects.FailNext(metadataOperation, errR2Unavailable)
	photo, err := service.UploadImage(ctx, photo.PhotoInput{Color: synchronousTestBlackAndWhite}, bytes.NewReader(raw), int64(len(raw)))
	if !errors.Is(err, domain.ErrUncertain) {
		t.Fatal(err)
	}
	if _, _, err := objects.Read(ctx, photo.ObjectKey); err != nil {
		t.Fatal(prematureCleanupFailure, err)
	}
	if count, err := repo.PendingBackups(ctx); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := repo.FlushBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), synchronousTestRestoredFilename), r2.NewBackup(objects))
	if got, err := restored.gallery.GetImage(ctx, photo.UUID); err != nil || got != photo {
		t.Fatal(got, err)
	}
}
