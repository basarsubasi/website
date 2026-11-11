package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

const CatalogKey = "catalog.json"
const catalogVersion = 1
const refreshInterval = 5 * time.Second

type ObjectStore interface {
	usecase.PhotoStorage
	Read(context.Context, string) ([]byte, string, error)
	PutIf(context.Context, string, []byte, string, bool) (string, error)
}
type postEntry struct {
	Metadata  domain.Post `json:"metadata"`
	ObjectKey string      `json:"object_key"`
}
type catalog struct {
	Version  int                     `json:"version"`
	Revision string                  `json:"revision"`
	Images   map[string]domain.Image `json:"images"`
	Posts    map[string]postEntry    `json:"posts"`
	Garbage  []string                `json:"pending_deletes"`
}
type Repository struct {
	objects ObjectStore
	writeMu sync.Mutex
	state   catalog
	etag    string
	reads   *readCache
}

var _ usecase.Repository = (*Repository)(nil)

func OpenRepository(ctx context.Context, objects ObjectStore) (*Repository, error) {
	if objects == nil {
		return nil, domain.ErrUnavailable
	}
	repo := &Repository{objects: objects, reads: newReadCache()}
	if err := repo.load(ctx); err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("load R2 catalog: %w", err)
		}
		empty := catalog{Version: catalogVersion, Revision: uuid.NewV4().String(), Images: map[string]domain.Image{}, Posts: map[string]postEntry{}, Garbage: []string{}}
		raw, _ := json.Marshal(empty)
		etag, err := objects.PutIf(ctx, CatalogKey, raw, "", true)
		if err != nil {
			// Another instance may have initialized it, or our response may have been lost.
			if readErr := repo.load(ctx); readErr != nil {
				return nil, fmt.Errorf("initialize R2 catalog: %w", err)
			}
		} else {
			repo.state = empty
			repo.etag = etag
			repo.reads.publish(CatalogKey, &readResult{state: empty, size: len(raw)})
		}
	}
	repo.cleanup(ctx)
	return repo, nil
}
func (r *Repository) load(ctx context.Context) error {
	raw, etag, err := r.objects.Read(ctx, CatalogKey)
	if err != nil {
		return err
	}
	state, err := parseCatalog(raw, etag)
	if err != nil {
		return err
	}
	r.state = state
	r.etag = etag
	r.reads.publish(CatalogKey, &readResult{state: state, size: len(raw)})
	return nil
}
func clone(state catalog) catalog {
	next := catalog{Version: state.Version, Revision: state.Revision, Images: make(map[string]domain.Image, len(state.Images)), Posts: make(map[string]postEntry, len(state.Posts)), Garbage: append([]string{}, state.Garbage...)}
	for id, image := range state.Images {
		next.Images[id] = image
	}
	for id, post := range state.Posts {
		next.Posts[id] = post
	}
	return next
}
func (r *Repository) commit(ctx context.Context, next catalog) error {
	next.Revision = uuid.NewV4().String()
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(raw) > maxObjectJSON {
		return domain.Invalid("catalog exceeds 16 MiB capacity")
	}
	etag, err := r.objects.PutIf(ctx, CatalogKey, raw, r.etag, false)
	if err != nil {
		reconcile, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if readErr := r.load(reconcile); readErr == nil && r.state.Revision == next.Revision {
			return nil
		}
		if errors.Is(err, domain.ErrConflict) {
			return domain.ErrConflict
		}
		// A negative read cannot prove that an in-flight write won't commit later.
		return fmt.Errorf("%w: %v", domain.ErrUncertain, err)
	}
	r.state = next
	r.etag = etag
	r.reads.publish(CatalogKey, &readResult{state: next, size: len(raw)})
	return nil
}
func (r *Repository) mutate(ctx context.Context, fn func(*catalog) error) error {
	if err := r.load(ctx); err != nil {
		return err
	}
	next := clone(r.state)
	if err := fn(&next); err != nil {
		return err
	}
	if err := r.commit(ctx, next); err != nil {
		return err
	}
	r.cleanup(ctx)
	return nil
}

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
	next := clone(r.state)
	remaining := make([]string, 0, len(next.Garbage))
	for _, key := range next.Garbage {
		referenced := false
		for _, photo := range next.Images {
			if photo.ObjectKey == key {
				referenced = true
			}
		}
		for _, post := range next.Posts {
			if post.ObjectKey == key {
				referenced = true
			}
		}
		if referenced {
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
func (r *Repository) Ping(ctx context.Context) error {
	_, err := r.snapshot(ctx)
	return err
}
func (r *Repository) ListImages(ctx context.Context, p domain.Pagination, color string) (domain.Page[domain.Image], error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Page[domain.Image]{}, err
	}
	items := []domain.Image{}
	for _, photo := range state.Images {
		if color == "" || photo.Color == color {
			items = append(items, photo)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].Uuid > items[j].Uuid
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return paginate(items, p), nil
}
func paginate[T any](items []T, p domain.Pagination) domain.Page[T] {
	total := int64(len(items))
	start := (p.Page - 1) * p.Limit
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	return domain.Page[T]{Data: items[start:end], Total: total, Page: p.Page, Limit: p.Limit}
}
func (r *Repository) GetImage(ctx context.Context, id string) (domain.Image, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Image{}, err
	}
	photo, ok := state.Images[id]
	if !ok {
		return domain.Image{}, domain.ErrNotFound
	}
	return photo, nil
}
func (r *Repository) CreateImage(ctx context.Context, photo domain.Image) (domain.Image, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	photo.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
	err := r.mutate(ctx, func(next *catalog) error {
		if _, exists := next.Images[photo.Uuid]; exists {
			return domain.ErrConflict
		}
		next.Images[photo.Uuid] = photo
		return nil
	})
	return photo, err
}
func (r *Repository) UpdateImage(ctx context.Context, id string, p domain.PhotoInput) (domain.Image, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	var photo domain.Image
	err := r.mutate(ctx, func(next *catalog) error {
		var exists bool
		photo, exists = next.Images[id]
		if !exists {
			return domain.ErrNotFound
		}
		photo.Name = p.Name
		photo.Country = p.Country
		photo.City = p.City
		photo.YearTaken = p.YearTaken
		photo.Color = p.Color
		next.Images[id] = photo
		return nil
	})
	return photo, err
}
func (r *Repository) DeleteImage(ctx context.Context, id string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return r.mutate(ctx, func(next *catalog) error {
		photo, exists := next.Images[id]
		if !exists {
			return domain.ErrNotFound
		}
		delete(next.Images, id)
		next.Garbage = append(next.Garbage, photo.ObjectKey)
		return nil
	})
}
func (r *Repository) readPost(ctx context.Context, entry postEntry) (domain.Post, error) {
	result, err := r.reads.get(ctx, r.objects, entry.ObjectKey)
	if err != nil {
		return domain.Post{}, err
	}
	post := result.post
	if post.ID != entry.Metadata.ID || post.UpdatedAt != entry.Metadata.UpdatedAt {
		return domain.Post{}, fmt.Errorf("post object does not match catalog")
	}
	return post, nil
}
func (r *Repository) ListPosts(ctx context.Context, p domain.Pagination, includeDrafts bool) (domain.Page[domain.Post], error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Page[domain.Post]{}, err
	}
	items := []domain.Post{}
	for _, entry := range state.Posts {
		if includeDrafts || entry.Metadata.Published == 1 {
			items = append(items, entry.Metadata)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	page := paginate(items, p)
	if includeDrafts {
		for i, post := range page.Data {
			full, err := r.readPost(ctx, state.Posts[post.ID])
			if err != nil {
				return page, err
			}
			page.Data[i] = full
		}
	}
	return page, nil
}
func (r *Repository) GetPost(ctx context.Context, slug string) (domain.Post, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Post{}, err
	}
	for _, entry := range state.Posts {
		if entry.Metadata.Slug == slug && entry.Metadata.Published == 1 {
			return r.readPost(ctx, entry)
		}
	}
	return domain.Post{}, domain.ErrNotFound
}
func (r *Repository) GetPostByID(ctx context.Context, id string) (domain.Post, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Post{}, err
	}
	entry, ok := state.Posts[id]
	if !ok {
		return domain.Post{}, domain.ErrNotFound
	}
	return r.readPost(ctx, entry)
}
func (r *Repository) savePost(ctx context.Context, id string, input domain.PostInput, create bool, backup *domain.Post) (domain.Post, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	var post domain.Post
	var newKey string
	err := r.mutate(ctx, func(next *catalog) error {
		if backup != nil {
			_, exists := next.Posts[id]
			create = !exists
		}
		for otherID, entry := range next.Posts {
			if otherID != id && entry.Metadata.Slug == input.Slug {
				return domain.ErrConflict
			}
		}
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
		if create {
			if _, exists := next.Posts[id]; exists {
				return domain.ErrConflict
			}
			post = domain.Post{ID: id, CreatedAt: now}
		} else {
			entry, exists := next.Posts[id]
			if !exists {
				return domain.ErrNotFound
			}
			post = entry.Metadata
			next.Garbage = append(next.Garbage, entry.ObjectKey)
		}
		post.Slug = input.Slug
		post.Title = input.Title
		post.Excerpt = input.Excerpt
		post.Body = input.Body
		post.Published = input.Published
		post.UpdatedAt = now
		if backup != nil {
			post = *backup
		}
		newKey = "posts/" + id + "/" + uuid.NewV4().String() + ".json"
		raw, _ := json.Marshal(post)
		if _, err := r.objects.Put(ctx, newKey, bytes.NewReader(raw), int64(len(raw)), "application/json", map[string]string{"post-id": id}); err != nil {
			return &domain.StorageError{Operation: "write post", Err: err}
		}
		summary := post
		summary.Body = ""
		next.Posts[id] = postEntry{Metadata: summary, ObjectKey: newKey}
		return nil
	})
	if err != nil && newKey != "" && !errors.Is(err, domain.ErrUncertain) {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := r.objects.Delete(cleanup, newKey); cleanupErr != nil {
			log.Printf("Unreferenced post cleanup needed for %s: %v", newKey, cleanupErr)
		}
	}
	return post, err
}
func (r *Repository) CreatePost(ctx context.Context, p domain.Post) (domain.Post, error) {
	return r.savePost(ctx, p.ID, domain.PostInput{Slug: p.Slug, Title: p.Title, Excerpt: p.Excerpt, Body: p.Body, Published: p.Published}, true, nil)
}
func (r *Repository) UpdatePost(ctx context.Context, id string, p domain.PostInput) (domain.Post, error) {
	return r.savePost(ctx, id, p, false, nil)
}
func (r *Repository) DeletePost(ctx context.Context, id string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return r.mutate(ctx, func(next *catalog) error {
		entry, exists := next.Posts[id]
		if !exists {
			return domain.ErrNotFound
		}
		delete(next.Posts, id)
		next.Garbage = append(next.Garbage, entry.ObjectKey)
		return nil
	})
}
