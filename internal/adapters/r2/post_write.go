package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
)

// savePost serializes a post edit with the catalog update. prepare decides how
// to handle an existing entry; persistence and failed-write cleanup are shared.
func (r *Repository) savePost(ctx context.Context, id, slug string, prepare func(postEntry, bool) (domain.Post, error)) (domain.Post, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	var post domain.Post
	var newKey string
	err := r.mutate(ctx, func(next *catalog) error {
		for otherID, entry := range next.Posts {
			if otherID != id && entry.Metadata.Slug == slug {
				return domain.ErrConflict
			}
		}
		entry, exists := next.Posts[id]
		var err error
		post, err = prepare(entry, exists)
		if err != nil {
			return err
		}
		if exists {
			next.Garbage = append(next.Garbage, entry.ObjectKey)
		}
		raw, err := json.Marshal(post)
		if err != nil {
			return err
		}
		newKey = "posts/" + id + "/" + uuid.NewV4().String() + ".json"
		if _, err := r.objects.Put(ctx, newKey, bytes.NewReader(raw), int64(len(raw)), "application/json", map[string]string{"post-id": id}); err != nil {
			return &domain.StorageError{Operation: "write post", Err: err}
		}
		summary := post
		summary.Body = ""
		next.Posts[id] = postEntry{Metadata: summary, ObjectKey: newKey}
		return nil
	})
	// An uncertain commit may still reference the new object. Only remove it
	// when we know the catalog did not commit.
	if err != nil && newKey != "" && !errors.Is(err, domain.ErrUncertain) {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := r.objects.Delete(cleanup, newKey); cleanupErr != nil {
			log.Printf("Unreferenced post cleanup needed for %s: %v", newKey, cleanupErr)
		}
	}
	return post, err
}
