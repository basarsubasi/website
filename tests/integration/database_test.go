package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	blogsqlite "github.com/basarsubasi/website/internal/module/blog/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	gallerysqlite "github.com/basarsubasi/website/internal/module/gallery/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/backup"
	"github.com/basarsubasi/website/internal/platform/database"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	databaseTestDatabaseFilename = "site.db"
	photoKey                     = "photos/test.png"
	photoURL                     = "https://photos.example.com/test.png"
	databaseTestBlackAndWhite    = "bw"
	databaseTestPngContentType   = "image/png"
	databaseTestPhotoContents    = "photo"
	postSlug                     = "hello"
	postTitle                    = "Hello"
	fullBody                     = "Full body"
	databaseTestRestoredFilename = "restored.db"
	postRestoreFormat            = "post restore: %+v %v"
	photoRestoreFormat           = "image restore: %+v %v"
	resurrectedPostFailure       = "deleted post resurrected"
	retainedPhotoFailure         = "photo not deleted"
	backupDeleteFormat           = "backup deletes: %+v %v"

	lostBackupResponseMessage = "lost backup response"

	oldSlug                    = "old"
	originalTitle              = "Original"
	newSlug                    = "new"
	editedTitle                = "Edited"
	databaseTestUpdatedBody    = "Updated"
	ignoredLostResponseFailure = "lost response ignored"
	removedOperationFailure    = "unacknowledged operation removed"
	lostEditFailure            = "local edit lost"
	replayMismatchFormat       = "ordered replay: %+v %v"
	pendingQueueFailure        = "queue not drained"

	takenSlug                  = "taken"
	firstTitle                 = "First"
	otherSlug                  = "other"
	secondTitle                = "Second"
	conflictTitle              = "Conflict"
	failedEditQueuedFailure    = "failed edit queued"
	failedEditCommittedFailure = "failed edit committed"

	localSlug              = "local"
	postBody               = "Body"
	blockedReadFormat      = "backup blocked read: %v"
	duplicateWorkerFailure = "second worker consumed first worker's job"

	duplicateSlug        = "duplicate"
	invalidImportFailure = "invalid import succeeded"
	partialImportFailure = "partial import committed"
	remoteAccessFailure  = "initialized DB queried remote"

	bodyPadding          = "x"
	legacyTitle          = "Legacy"
	blockedQueueFormat   = "queue blocked: %d"
	restoreCountFormat   = "restore count: %d error: %v"
	legacyDataFailure    = "legacy data lost"
	blockedDeletesFormat = "deletes blocked: %v"
	catalogPrefix        = "catalog/"
	postsPrefix          = "posts/"
	staleObjectFormat    = "retired object not cleaned: %s"
)

var errLostBackupResponse = errors.New(lostBackupResponseMessage)

type testStore struct {
	*database.Store
	gallery *gallerysqlite.Repository
	blog    *blogsqlite.Repository
}

func open(t *testing.T, path string, backup backup.Target) *testStore {
	t.Helper()
	repo, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err = repo.Initialize(context.Background(), backup.LoadSnapshot); err != nil {
		t.Fatal(err)
	}
	return &testStore{Store: repo, gallery: gallerysqlite.New(repo), blog: blogsqlite.New(repo)}
}
func TestSQLiteBackupRestoreAndDeletion(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	path := filepath.Join(t.TempDir(), databaseTestDatabaseFilename)
	repo := open(t, path, backup)
	photo := photo.Image{UUID: uuid.NewV4().String(), ObjectKey: photoKey, ImageURL: photoURL, Color: databaseTestBlackAndWhite, FileType: databaseTestPngContentType}
	objects.Put(ctx, photo.ObjectKey, strings.NewReader(databaseTestPhotoContents), 5, photo.FileType, nil)
	photo, err := repo.gallery.CreateImage(ctx, photo)
	if err != nil {
		t.Fatal(err)
	}
	post, err := repo.blog.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: postSlug, Title: postTitle, Body: fullBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := repo.PendingBackups(ctx); count != 2 {
		t.Fatal(count)
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), databaseTestRestoredFilename), r2.NewBackup(objects))
	if got, err := restored.blog.GetPost(ctx, post.Slug); err != nil || got != post {
		t.Fatalf(postRestoreFormat, got, err)
	}
	if got, err := restored.gallery.GetImage(ctx, photo.UUID); err != nil || got != photo {
		t.Fatalf(photoRestoreFormat, got, err)
	}
	if err = repo.blog.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.gallery.DeleteImage(ctx, photo.UUID); err != nil {
		t.Fatal(err)
	}
	// Restart before synchronization must not resurrect the remote copies.
	repo.Close()
	repo = open(t, path, backup)
	if _, err = repo.blog.GetPostByID(ctx, post.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(resurrectedPostFailure)
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if _, _, err = objects.Read(ctx, photo.ObjectKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(retainedPhotoFailure)
	}
	snapshot, err := r2.NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Images) != 0 || len(snapshot.Posts) != 0 {
		t.Fatalf(backupDeleteFormat, snapshot, err)
	}
}

type lostReply struct {
	backup.Target
	fail bool
}

func (b *lostReply) SavePost(ctx context.Context, p article.Post) error {
	if err := b.Target.SavePost(ctx, p); err != nil {
		return err
	}
	if b.fail {
		b.fail = false
		return errLostBackupResponse
	}
	return nil
}
func TestBackupFailureSurvivesRestartAndReplaysInOrder(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	path := filepath.Join(t.TempDir(), databaseTestDatabaseFilename)
	repo := open(t, path, backup)
	post, err := repo.blog.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: oldSlug, Body: originalTitle, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.blog.UpdatePost(ctx, post.ID, article.PostInput{Slug: newSlug, Title: editedTitle, Body: databaseTestUpdatedBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	lost := &lostReply{Target: backup, fail: true}
	if err = repo.SyncBackups(ctx, lost); err == nil {
		t.Fatal(ignoredLostResponseFailure)
	}
	if count, _ := repo.PendingBackups(ctx); count != 2 {
		t.Fatal(removedOperationFailure)
	}
	repo.Close()
	repo = open(t, path, backup)
	if got, err := repo.blog.GetPost(ctx, newSlug); err != nil || got != updated {
		t.Fatal(lostEditFailure)
	}
	if err = repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	snapshot, err := r2.NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Posts) != 1 || snapshot.Posts[0] != updated {
		t.Fatalf(replayMismatchFormat, snapshot, err)
	}
	if count, _ := repo.PendingBackups(ctx); count != 0 {
		t.Fatal(pendingQueueFailure)
	}
}

func TestFailedMutationDoesNotQueueBackup(t *testing.T) {
	ctx := context.Background()
	repo := open(t, filepath.Join(t.TempDir(), databaseTestDatabaseFilename), r2.NewBackup(testutil.NewObjects()))
	first, err := repo.blog.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: takenSlug, Body: firstTitle})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.blog.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: otherSlug, Body: secondTitle})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.blog.UpdatePost(ctx, second.ID, article.PostInput{Slug: first.Slug, Body: conflictTitle}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal(err)
	}
	if count, _ := repo.PendingBackups(ctx); count != 2 {
		t.Fatal(failedEditQueuedFailure)
	}
	if got, _ := repo.blog.GetPostByID(ctx, second.ID); got != second {
		t.Fatal(failedEditCommittedFailure)
	}
}

type slowBackup struct {
	backup.Target
	started chan struct{}
	release chan struct{}
}

func (b *slowBackup) SavePost(ctx context.Context, p article.Post) error {
	close(b.started)
	select {
	case <-b.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return b.Target.SavePost(ctx, p)
}
func TestSlowBackupDoesNotBlockSQLiteAndWorkersAreSerialized(t *testing.T) {
	ctx := context.Background()
	backup := r2.NewBackup(testutil.NewObjects())
	path := filepath.Join(t.TempDir(), databaseTestDatabaseFilename)
	repo := open(t, path, backup)
	post, err := repo.blog.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: localSlug, Body: postBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	other := open(t, path, backup)
	slow := &slowBackup{Target: backup, started: make(chan struct{}), release: make(chan struct{})}
	defer close(slow.release)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- repo.SyncBackups(workerCtx, slow) }()
	<-slow.started
	readCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if got, err := repo.blog.GetPost(readCtx, post.Slug); err != nil || got != post {
		t.Fatalf(blockedReadFormat, err)
	}
	if err = other.SyncBackups(readCtx, backup); err != nil {
		t.Fatal(err)
	}
	if count, _ := repo.PendingBackups(readCtx); count != 1 {
		t.Fatal(duplicateWorkerFailure)
	}
	cancel()
	<-result
}

func TestImportFailureIsAtomicAndRetryable(t *testing.T) {
	ctx := context.Background()
	repo, err := database.Open(ctx, filepath.Join(t.TempDir(), databaseTestDatabaseFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	singleBlog := blogsqlite.New(repo)
	duplicate := article.Post{ID: uuid.NewV4().String(), Slug: duplicateSlug}
	err = repo.Initialize(ctx, func(context.Context) (backup.Snapshot, error) {
		return backup.Snapshot{Posts: []article.Post{duplicate, duplicate}}, nil
	})
	if err == nil {
		t.Fatal(invalidImportFailure)
	}
	page, err := singleBlog.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, true)
	if err != nil || page.Total != 0 {
		t.Fatal(partialImportFailure)
	}
	if err = repo.Initialize(ctx, func(context.Context) (backup.Snapshot, error) {
		return backup.Snapshot{Posts: []article.Post{duplicate}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Initialize(ctx, func(context.Context) (backup.Snapshot, error) {
		t.Fatal(remoteAccessFailure)
		return backup.Snapshot{}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLargeLegacyExcerptsDoNotBlockBackupQueue(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	repo := open(t, filepath.Join(t.TempDir(), databaseTestDatabaseFilename), backup)
	excerpt := strings.Repeat(bodyPadding, 950<<10)
	ids := []string{}
	for i := 0; i < 18; i++ {
		id := uuid.NewV4().String()
		post, err := repo.blog.CreatePost(ctx, article.Post{ID: id, Slug: id, Title: legacyTitle, Excerpt: excerpt, Body: fullBody, Published: 1})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, post.ID)
	}
	if err := repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if pending, _ := repo.PendingBackups(ctx); pending != 0 {
		t.Fatalf(blockedQueueFormat, pending)
	}
	restored := open(t, filepath.Join(t.TempDir(), databaseTestRestoredFilename), r2.NewBackup(objects))
	page, err := restored.blog.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 100}, true)
	if err != nil || len(page.Data) != 18 {
		t.Fatalf(restoreCountFormat, len(page.Data), err)
	}
	for _, p := range page.Data {
		if p.Excerpt != excerpt || p.Body != fullBody {
			t.Fatal(legacyDataFailure)
		}
	}
	for _, id := range ids {
		if err := repo.blog.DeletePost(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	snapshot, err := r2.NewBackup(objects).LoadSnapshot(ctx)
	if err != nil || len(snapshot.Posts) != 0 {
		t.Fatalf(blockedDeletesFormat, err)
	}
	for _, key := range objects.Keys() {
		if strings.HasPrefix(key, catalogPrefix) || strings.HasPrefix(key, postsPrefix) {
			t.Fatalf(staleObjectFormat, key)
		}
	}
}
