package r2

import (
	"context"
	"log"
	"time"
)

// Deletion is recorded atomically with the catalog edit. Cleanup is idempotent
// and retries on subsequent writes or a reboot, rather than losing the queue.
func (r *Repository) cleanup(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if len(r.state.Garbage) == 0 {
		return
	}
	if err := r.load(ctx); err != nil {
		return
	}
	next := r.state.clone()
	remaining := make([]string, 0, len(next.Garbage))
	for _, key := range next.Garbage {
		if next.references(key) {
			continue
		}
		if err := r.objects.Delete(ctx, key); err != nil {
			log.Printf("Object cleanup pending for %s: %v", key, err)
			remaining = append(remaining, key)
		}
	}
	if len(remaining) == len(next.Garbage) {
		return
	}
	next.Garbage = remaining
	if err := r.commit(ctx, next); err != nil {
		log.Printf("Cleanup catalog update pending: %v", err)
	}
}

func (state catalog) references(key string) bool {
	for _, image := range state.Images {
		if image.ObjectKey == key {
			return true
		}
	}
	for _, post := range state.Posts {
		if post.ObjectKey == key {
			return true
		}
	}
	return false
}
