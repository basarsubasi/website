package r2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

type restoreStore struct {
	*testutil.Objects
	active  atomic.Int32
	maximum atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (s *restoreStore) Read(ctx context.Context, key string) ([]byte, string, error) {
	if strings.HasPrefix(key, "posts/") {
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
			return nil, "", ctx.Err()
		}
	}
	return s.Objects.Read(ctx, key)
}
func TestRestoreReadsAreParallelBoundedAndCancelable(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			objects := testutil.NewObjects()
			repo, err := OpenRepository(context.Background(), objects)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 12; i++ {
				_, err = repo.CreatePost(context.Background(), domain.Post{ID: fmt.Sprint(i), Slug: fmt.Sprint(i), Body: "body", Published: int64(i % 2)})
				if err != nil {
					t.Fatal(err)
				}
			}
			store := &restoreStore{Objects: objects, started: make(chan struct{}, 12), release: make(chan struct{})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				snapshot, err := NewBackup(store).LoadSnapshot(ctx)
				if err == nil && len(snapshot.Posts) != 12 {
					err = fmt.Errorf("incomplete snapshot: %d", len(snapshot.Posts))
				}
				done <- err
			}()
			for i := 0; i < restoreWorkers; i++ {
				select {
				case <-store.started:
				case <-time.After(3 * time.Second):
					t.Fatal("restore failed to run concurrent reads")
				}
			}
			if cancelled {
				cancel()
			} else {
				close(store.release)
			}
			select {
			case err := <-done:
				if cancelled && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation: %v", err)
				}
				if !cancelled && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("restore did not stop")
			}
			if store.active.Load() != 0 || store.maximum.Load() != restoreWorkers {
				t.Fatalf("active=%d peak=%d", store.active.Load(), store.maximum.Load())
			}
		})
	}
}

func TestRestoreRejectsMissingPostWithoutPartialSnapshot(t *testing.T) {
	objects := testutil.NewObjects()
	repo, err := OpenRepository(context.Background(), objects)
	if err != nil {
		t.Fatal(err)
	}
	post, err := repo.CreatePost(context.Background(), domain.Post{ID: "post", Slug: "post", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if err = objects.Delete(context.Background(), repo.state.Posts[post.ID].ObjectKey); err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewBackup(objects).LoadSnapshot(context.Background())
	if !errors.Is(err, domain.ErrNotFound) || len(snapshot.Posts) != 0 || len(snapshot.Images) != 0 {
		t.Fatalf("partial restore: %+v %v", snapshot, err)
	}
}
