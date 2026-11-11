package r2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/basarsubasi/website/internal/domain"
)

const restoreWorkers = 8

// Restore bypasses the public read cache: each object is needed once, and
// cancellation must stop its network request rather than a shared waiter.
func (r *Repository) restorePosts(ctx context.Context) ([]domain.Post, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	entries := make([]postEntry, 0, len(r.state.Posts))
	for _, entry := range r.state.Posts {
		entries = append(entries, entry)
	}
	posts := make([]domain.Post, len(entries))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for i := 0; i < min(restoreWorkers, len(entries)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				entry := entries[index]
				attempt, stop := context.WithTimeout(ctx, readTimeout)
				raw, _, err := r.objects.Read(attempt, entry.ObjectKey)
				stop()
				if err == nil {
					err = json.Unmarshal(raw, &posts[index])
				}
				if err == nil && (posts[index].ID != entry.Metadata.ID || posts[index].UpdatedAt != entry.Metadata.UpdatedAt) {
					err = fmt.Errorf("post object does not match catalog")
				}
				if err != nil {
					cancel(fmt.Errorf("restore post %s: %w", entry.Metadata.ID, err))
					return
				}
			}
		}()
	}
dispatch:
	for index := range entries {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return posts, nil
}
