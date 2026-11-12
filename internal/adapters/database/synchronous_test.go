package database_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
	"github.com/basarsubasi/website/internal/usecase"
)

func TestSynchronousWriteWaitsForR2WithoutBlockingReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo := open(t, filepath.Join(t.TempDir(), "site.db"), backup)
	slow := &slowBackup{Backup: backup, started: make(chan struct{}), release: make(chan struct{})}
	repo.RequireBackup(slow)
	result := make(chan error, 1)
	go func() {
		_, err := repo.CreatePost(ctx, domain.Post{ID: "post", Slug: "post", Body: "saved", Published: 1})
		result <- err
	}()
	select {
	case <-slow.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-result:
		t.Fatalf("returned before R2: %v", err)
	default:
	}
	if _, err := repo.GetPost(ctx, "post"); err != nil {
		t.Fatal("read blocked", err)
	}
	close(slow.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), "restored.db"), r2.NewBackup(objects))
	if got, err := restored.GetPost(ctx, "post"); err != nil || got.Body != "saved" {
		t.Fatal(got, err)
	}
}

func TestSynchronousFailureRetainsQueueAndRetry(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo := open(t, filepath.Join(t.TempDir(), "site.db"), backup)
	lost := &lostReply{Backup: backup, fail: true}
	repo.RequireBackup(lost)
	if _, err := repo.CreatePost(ctx, domain.Post{ID: "post", Slug: "post", Body: "original"}); !errors.Is(err, domain.ErrUncertain) {
		t.Fatalf("expected uncertain write, got %v", err)
	}
	if count, err := repo.PendingBackups(ctx); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	lost.fail = false
	if _, err := repo.UpdatePost(ctx, "post", domain.PostInput{Slug: "post", Body: "updated"}); err != nil {
		t.Fatal(err)
	}
	if count, err := repo.PendingBackups(ctx); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	restored := open(t, filepath.Join(t.TempDir(), "restored.db"), r2.NewBackup(objects))
	if got, err := restored.GetPostByID(ctx, "post"); err != nil || got.Body != "updated" {
		t.Fatal(got, err)
	}
}

type countBackup struct {
	usecase.Backup
	posts int
}

func (b *countBackup) SavePost(context.Context, domain.Post) error { b.posts++; return nil }

func TestFlushDrainsMoreThanOneBatchAndWaitsForLock(t *testing.T) {
	ctx := context.Background()
	backup := r2.NewBackup(testutil.NewObjects())
	repo := open(t, filepath.Join(t.TempDir(), "site.db"), backup)
	for i := 0; i < 105; i++ {
		id := fmt.Sprint(i)
		if _, err := repo.CreatePost(ctx, domain.Post{ID: id, Slug: id}); err != nil {
			t.Fatal(err)
		}
	}
	count := &countBackup{Backup: backup}
	if err := repo.FlushBackups(ctx, count); err != nil {
		t.Fatal(err)
	}
	if count.posts != 105 {
		t.Fatalf("only flushed %d", count.posts)
	}
	if _, err := repo.CreatePost(ctx, domain.Post{ID: "last", Slug: "last"}); err != nil {
		t.Fatal(err)
	}
	slow := &slowBackup{Backup: backup, started: make(chan struct{}), release: make(chan struct{})}
	workerCtx, stop := context.WithCancel(ctx)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- repo.SyncBackups(workerCtx, slow) }()
	<-slow.started
	waitCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	if err := repo.FlushBackups(waitCtx, backup); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("skipped held lock: %v", err)
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
	repo := open(t, filepath.Join(t.TempDir(), "site.db"), backup)
	repo.RequireBackup(backup)
	service := usecase.New(repo, objects, objects)
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC")
	objects.FailNext("metadata", errors.New("R2 unavailable"))
	photo, err := service.UploadImage(ctx, domain.PhotoInput{Color: "bw"}, bytes.NewReader(raw), int64(len(raw)))
	if !errors.Is(err, domain.ErrUncertain) {
		t.Fatal(err)
	}
	if _, _, err := objects.Read(ctx, photo.ObjectKey); err != nil {
		t.Fatal("upload incorrectly cleaned up", err)
	}
	if count, err := repo.PendingBackups(ctx); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := repo.FlushBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), "restored.db"), r2.NewBackup(objects))
	if got, err := restored.GetImage(ctx, photo.UUID); err != nil || got != photo {
		t.Fatal(got, err)
	}
}
