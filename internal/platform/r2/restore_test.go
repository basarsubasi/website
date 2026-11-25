package r2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	restoreTestPostsPrefix   = "posts/"
	restoreTestEmptyValue    = ""
	restoreTestPostBody      = "body"
	incompleteSnapshotFormat = "incomplete snapshot: %d"
	serialRestoreFailure     = "restore failed to run concurrent reads"
	cancellationFormat       = "cancellation: %v"
	restoreTimeoutFailure    = "restore did not stop"
	workerCountFormat        = "active=%d peak=%d"
	postSlug                 = "post"
	partialRestoreFormat     = "partial restore: %+v %v"
)

type restoreStore struct {
	*testutil.Objects
	active  atomic.Int32
	maximum atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (s *restoreStore) Read(ctx context.Context, key string) ([]byte, string, error) {
	if strings.HasPrefix(key, restoreTestPostsPrefix) {
		active := s.active.Add(1)
		defer s.active.Add(-1)
		for old := s.maximum.Load(); active > old; old = s.maximum.Load() {
			if s.maximum.CompareAndSwap(old, active) {
				break
			}
		}
		s.started <- struct{}{}
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, restoreTestEmptyValue, ctx.Err()
		}
	}
	return s.Objects.Read(ctx, key)
}
func TestRestoreReadsAreParallelBoundedAndCancelable(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			checkRestoreWorkers(t, cancelled)
		})
	}
}

func TestRestoreRejectsMissingPostWithoutPartialSnapshot(t *testing.T) {
	objects := testutil.NewObjects()
	repo, err := OpenRepository(context.Background(), objects)
	if err != nil {
		t.Fatal(err)
	}
	post, err := repo.CreatePost(context.Background(), article.Post{ID: postSlug, Slug: postSlug, Body: restoreTestPostBody})
	if err != nil {
		t.Fatal(err)
	}
	if err = objects.Delete(context.Background(), repo.state.Posts[post.ID].ObjectKey); err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewBackup(objects).LoadSnapshot(context.Background())
	if !errors.Is(err, domain.ErrNotFound) || len(snapshot.Posts) != 0 || len(snapshot.Images) != 0 {
		t.Fatalf(partialRestoreFormat, snapshot, err)
	}
}

func restoreObjects(t *testing.T) *testutil.Objects {
	t.Helper()
	objects := testutil.NewObjects()
	repo, err := OpenRepository(context.Background(), objects)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		_, err = repo.CreatePost(context.Background(), article.Post{ID: fmt.Sprint(i), Slug: fmt.Sprint(i), Body: restoreTestPostBody, Published: int64(i % 2)})
		if err != nil {
			t.Fatal(err)
		}
	}
	return objects
}

func waitForRestoreWorkers(t *testing.T, store *restoreStore) {
	t.Helper()
	for i := 0; i < restoreWorkers; i++ {
		select {
		case <-store.started:
		case <-time.After(3 * time.Second):
			t.Fatal(serialRestoreFailure)
		}
	}
}

func checkRestoreWorkers(t *testing.T, cancelled bool) {
	t.Helper()
	objects := restoreObjects(t)
	store := &restoreStore{Objects: objects, started: make(chan struct{}, 12), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		snapshot, err := NewBackup(store).LoadSnapshot(ctx)
		if err == nil && len(snapshot.Posts) != 12 {
			err = fmt.Errorf(incompleteSnapshotFormat, len(snapshot.Posts))
		}
		done <- err
	}()
	waitForRestoreWorkers(t, store)
	if cancelled {
		cancel()
	} else {
		close(store.release)
	}
	select {
	case err := <-done:
		if cancelled && !errors.Is(err, context.Canceled) {
			t.Fatalf(cancellationFormat, err)
		}
		if !cancelled && err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal(restoreTimeoutFailure)
	}
	if store.active.Load() != 0 || store.maximum.Load() != restoreWorkers {
		t.Fatalf(workerCountFormat, store.active.Load(), store.maximum.Load())
	}
}
