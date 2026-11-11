package database_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/adapters/database"
	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
	"github.com/basarsubasi/website/internal/usecase"
)

func open(t *testing.T, path string, backup usecase.Backup) *database.Repository {
	t.Helper()
	repo, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err = repo.Initialize(context.Background(), backup.LoadSnapshot); err != nil {
		t.Fatal(err)
	}
	return repo
}
func TestSQLiteBackupRestoreAndDeletion(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	path := filepath.Join(t.TempDir(), "site.db")
	repo := open(t, path, backup)
	photo := domain.Image{UUID: uuid.NewV4().String(), ObjectKey: "photos/test.png", ImageURL: "https://photos.example.com/test.png", Color: "bw", FileType: "image/png"}
	objects.Put(ctx, photo.ObjectKey, strings.NewReader("photo"), 5, photo.FileType, nil)
	photo, err := repo.CreateImage(ctx, photo)
	if err != nil {
		t.Fatal(err)
	}
	post, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "hello", Title: "Hello", Body: "Full body", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := repo.PendingBackups(ctx); count != 2 {
		t.Fatal(count)
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), "restored.db"), r2.NewBackup(objects))
	if got, err := restored.GetPost(ctx, post.Slug); err != nil || got != post {
		t.Fatalf("post restore: %+v %v", got, err)
	}
	if got, err := restored.GetImage(ctx, photo.UUID); err != nil || got != photo {
		t.Fatalf("image restore: %+v %v", got, err)
	}
	if err = repo.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.DeleteImage(ctx, photo.UUID); err != nil {
		t.Fatal(err)
	}
	// Restart before synchronization must not resurrect the remote copies.
	repo.Close()
	repo = open(t, path, backup)
	if _, err = repo.GetPostByID(ctx, post.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("deleted post resurrected")
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if _, _, err = objects.Read(ctx, photo.ObjectKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("photo not deleted")
	}
	snapshot, err := r2.NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Images) != 0 || len(snapshot.Posts) != 0 {
		t.Fatalf("backup deletes: %+v %v", snapshot, err)
	}
}

type lostReply struct {
	usecase.Backup
	fail bool
}

func (b *lostReply) SavePost(ctx context.Context, p domain.Post) error {
	if err := b.Backup.SavePost(ctx, p); err != nil {
		return err
	}
	if b.fail {
		b.fail = false
		return errors.New("lost backup response")
	}
	return nil
}
func TestBackupFailureSurvivesRestartAndReplaysInOrder(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	path := filepath.Join(t.TempDir(), "site.db")
	repo := open(t, path, backup)
	post, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "old", Body: "Original", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdatePost(ctx, post.ID, domain.PostInput{Slug: "new", Title: "Edited", Body: "Updated", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	lost := &lostReply{Backup: backup, fail: true}
	if err = repo.SyncBackups(ctx, lost); err == nil {
		t.Fatal("lost response ignored")
	}
	if count, _ := repo.PendingBackups(ctx); count != 2 {
		t.Fatal("unacknowledged operation removed")
	}
	repo.Close()
	repo = open(t, path, backup)
	if got, err := repo.GetPost(ctx, "new"); err != nil || got != updated {
		t.Fatal("local edit lost")
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	snapshot, err := r2.NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Posts) != 1 || snapshot.Posts[0] != updated {
		t.Fatalf("ordered replay: %+v %v", snapshot, err)
	}
	if count, _ := repo.PendingBackups(ctx); count != 0 {
		t.Fatal("queue not drained")
	}
}

func TestFailedMutationDoesNotQueueBackup(t *testing.T) {
	ctx := context.Background()
	repo := open(t, filepath.Join(t.TempDir(), "site.db"), r2.NewBackup(testutil.NewObjects()))
	first, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "taken", Body: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "other", Body: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.UpdatePost(ctx, second.ID, domain.PostInput{Slug: first.Slug, Body: "Conflict"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal(err)
	}
	if count, _ := repo.PendingBackups(ctx); count != 2 {
		t.Fatal("failed edit queued")
	}
	if got, _ := repo.GetPostByID(ctx, second.ID); got != second {
		t.Fatal("failed edit committed")
	}
}

type slowBackup struct {
	usecase.Backup
	started chan struct{}
	release chan struct{}
}

func (b *slowBackup) SavePost(ctx context.Context, p domain.Post) error {
	close(b.started)
	select {
	case <-b.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return b.Backup.SavePost(ctx, p)
}
func TestSlowBackupDoesNotBlockSQLiteAndWorkersAreSerialized(t *testing.T) {
	ctx := context.Background()
	backup := r2.NewBackup(testutil.NewObjects())
	path := filepath.Join(t.TempDir(), "site.db")
	repo := open(t, path, backup)
	post, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "local", Body: "Body", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	other := open(t, path, backup)
	slow := &slowBackup{Backup: backup, started: make(chan struct{}), release: make(chan struct{})}
	defer close(slow.release)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- repo.SyncBackups(workerCtx, slow) }()
	<-slow.started
	readCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if got, err := repo.GetPost(readCtx, post.Slug); err != nil || got != post {
		t.Fatalf("backup blocked read: %v", err)
	}
	if err = other.SyncBackups(readCtx, backup); err != nil {
		t.Fatal(err)
	}
	if count, _ := repo.PendingBackups(readCtx); count != 1 {
		t.Fatal("second worker consumed first worker's job")
	}
	cancel()
	<-result
}

func TestImportFailureIsAtomicAndRetryable(t *testing.T) {
	ctx := context.Background()
	repo, err := database.Open(ctx, filepath.Join(t.TempDir(), "site.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	duplicate := domain.Post{ID: uuid.NewV4().String(), Slug: "duplicate"}
	err = repo.Initialize(ctx, func(context.Context) (domain.Snapshot, error) {
		return domain.Snapshot{Posts: []domain.Post{duplicate, duplicate}}, nil
	})
	if err == nil {
		t.Fatal("invalid import succeeded")
	}
	page, err := repo.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, true)
	if err != nil || page.Total != 0 {
		t.Fatal("partial import committed")
	}
	if err = repo.Initialize(ctx, func(context.Context) (domain.Snapshot, error) {
		return domain.Snapshot{Posts: []domain.Post{duplicate}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Initialize(ctx, func(context.Context) (domain.Snapshot, error) {
		t.Fatal("initialized DB queried remote")
		return domain.Snapshot{}, nil
	}); err != nil {
		t.Fatal(err)
	}
}
