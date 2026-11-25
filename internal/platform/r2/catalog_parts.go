package r2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	partKeyPrefix               = "catalog/"
	invalidPartKeyMessage       = "invalid catalog part key"
	readPartErrorFormat         = "read catalog part: %w"
	digestMismatchMessage       = "catalog part digest mismatch"
	invalidPartMessage          = "invalid catalog part"
	duplicateImageMessage       = "duplicate image in catalog parts"
	duplicatePostMessage        = "duplicate post in catalog parts"
	partKeyFormat               = "catalog/%x.json"
	oversizedRecordMessage      = "catalog record exceeds 16 MiB capacity"
	catalogPartsJsonContentType = "application/json"
	oversizedManifestMessage    = "catalog manifest exceeds 16 MiB capacity"
)

const shardedCatalogVersion = 2
const catalogPartSize = 4 << 20
const partOverhead = 128

// The root remains a small, conditionally written commit point. Immutable parts
// contain bounded groups of records; old version-1 catalogs remain readable.
type catalogPart struct {
	Images map[string]json.RawMessage `json:"images"`
	Posts  map[string]json.RawMessage `json:"posts"`
}

func readCatalog(ctx context.Context, objects ObjectStore, raw []byte, etag string) (catalog, int, error) {
	state, err := parseCatalog(raw, etag)
	size := len(raw)
	if err != nil {
		return state, size, err
	}
	seen := make(map[string]bool)
	for _, key := range state.Parts {
		if !strings.HasPrefix(key, partKeyPrefix) || seen[key] {
			return state, size, fmt.Errorf(invalidPartKeyMessage)
		}
		seen[key] = true
		data, _, err := objects.Read(ctx, key)
		if err != nil {
			return state, size, fmt.Errorf(readPartErrorFormat, err)
		}
		if err := mergeCatalogPart(&state, key, data); err != nil {
			return state, size, err
		}
		size += len(data)
	}
	return state, size, validateCatalog(state)
}

func mergeCatalogPart(state *catalog, key string, data []byte) error {
	if catalogPartKey(data) != key {
		return fmt.Errorf(digestMismatchMessage)
	}
	var part struct {
		Images map[string]photo.Image `json:"images"`
		Posts  map[string]postEntry   `json:"posts"`
	}
	if err := json.Unmarshal(data, &part); err != nil {
		return err
	}
	if part.Images == nil || part.Posts == nil {
		return fmt.Errorf(invalidPartMessage)
	}
	for id, p := range part.Images {
		if _, exists := state.Images[id]; exists {
			return fmt.Errorf(duplicateImageMessage)
		}
		state.Images[id] = p
	}
	for id, p := range part.Posts {
		if _, exists := state.Posts[id]; exists {
			return fmt.Errorf(duplicatePostMessage)
		}
		state.Posts[id] = p
	}
	return nil
}

func catalogPartKey(raw []byte) string {
	return fmt.Sprintf(partKeyFormat, sha256.Sum256(raw))
}

func (r *Repository) encodeCatalog(ctx context.Context, next *catalog) ([]byte, int, error) {
	root := *next
	root.Parts = nil
	root.Version = catalogVersion
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, 0, err
	}
	size := len(raw)
	if size > catalogPartSize && len(next.Images)+len(next.Posts) > 0 {
		root.Version = shardedCatalogVersion
		root.Images = map[string]photo.Image{}
		root.Posts = map[string]postEntry{}
		root.Parts, err = r.writeCatalogParts(ctx, next)
		if err != nil {
			return nil, 0, err
		}
	}
	// Retired parts use the same durable, reference-checked cleanup as post files.
	// Unchanged parts are reused, including during a cleanup-only commit.
	for _, key := range r.state.Parts {
		if !slices.Contains(root.Parts, key) && !slices.Contains(root.Garbage, key) {
			root.Garbage = append(root.Garbage, key)
		}
	}
	raw, err = json.Marshal(root)
	if err != nil {
		return nil, 0, err
	}
	if len(raw) > maxObjectJSON {
		return nil, 0, domain.Invalid(oversizedManifestMessage)
	}
	next.Version = root.Version
	next.Parts = root.Parts
	next.Garbage = root.Garbage
	return raw, size, nil
}

type catalogPartWriter struct {
	repo *Repository
	part catalogPart
	size int
	keys []string
}

func (w *catalogPartWriter) reset() {
	w.part = catalogPart{Images: make(map[string]json.RawMessage), Posts: make(map[string]json.RawMessage)}
	w.size = partOverhead
}

func (r *Repository) writeCatalogParts(ctx context.Context, next *catalog) ([]string, error) {
	w := catalogPartWriter{repo: r}
	w.reset()
	for _, id := range slices.Sorted(maps.Keys(next.Images)) {
		if err := w.add(ctx, id, next.Images[id], true); err != nil {
			return nil, err
		}
	}
	for _, id := range slices.Sorted(maps.Keys(next.Posts)) {
		if err := w.add(ctx, id, next.Posts[id], false); err != nil {
			return nil, err
		}
	}
	if err := w.flush(ctx); err != nil {
		return nil, err
	}
	return w.keys, nil
}

func (w *catalogPartWriter) add(ctx context.Context, id string, value any, image bool) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	name, _ := json.Marshal(id)
	size := len(data) + len(name) + 2
	if w.size+size > catalogPartSize {
		if err := w.flush(ctx); err != nil {
			return err
		}
	}
	if image {
		w.part.Images[id] = data
	} else {
		w.part.Posts[id] = data
	}
	w.size += size
	return nil
}

func (w *catalogPartWriter) flush(ctx context.Context) error {
	if len(w.part.Images)+len(w.part.Posts) == 0 {
		return nil
	}
	data, err := json.Marshal(w.part)
	if err != nil {
		return err
	}
	if len(data) > maxObjectJSON {
		return domain.Invalid(oversizedRecordMessage)
	}
	key := catalogPartKey(data)
	if !slices.Contains(w.repo.state.Parts, key) {
		if _, err := w.repo.objects.Put(ctx, key, bytes.NewReader(data), int64(len(data)), catalogPartsJsonContentType, nil); err != nil {
			return err
		}
	}
	w.keys = append(w.keys, key)
	w.reset()
	return nil
}
