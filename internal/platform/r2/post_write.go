package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	postKeyPrefix            = "posts/"
	keySeparator             = "/"
	jsonSuffix               = ".json"
	postWriteJsonContentType = "application/json"
	postIDMetadataKey        = "post-id"
	writePostOperation       = "write post"
	postWriteEmptyString     = ""
	postCleanupErrorFormat   = "Unreferenced post cleanup needed for %s: %v"
)

// savePost serializes a post edit with the catalog update. prepare decides how
// to handle an existing entry; persistence and failed-write cleanup are shared.
func (r *Repository) savePost(ctx context.Context, id, slug string, prepare func(postEntry, bool) (article.Post, error)) (article.Post, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	var post article.Post
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
		newKey = postKeyPrefix + id + keySeparator + uuid.NewV4().String() + jsonSuffix
		if _, err := r.objects.Put(ctx, newKey, bytes.NewReader(raw), int64(len(raw)), postWriteJsonContentType, map[string]string{postIDMetadataKey: id}); err != nil {
			return &domain.StorageError{Operation: writePostOperation, Err: err}
		}
		summary := post
		summary.Body = postWriteEmptyString
		next.Posts[id] = postEntry{Metadata: summary, ObjectKey: newKey}
		return nil
	})
	// An uncertain commit may still reference the new object. Only remove it
	// when we know the catalog did not commit.
	if err != nil && newKey != postWriteEmptyString && !errors.Is(err, domain.ErrUncertain) {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := r.objects.Delete(cleanup, newKey); cleanupErr != nil {
			log.Printf(postCleanupErrorFormat, newKey, cleanupErr)
		}
	}
	return post, err
}
