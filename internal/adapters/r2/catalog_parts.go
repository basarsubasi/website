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

	"github.com/basarsubasi/website/internal/domain"
)

const shardedCatalogVersion = 2
const catalogPartSize = 4 << 20

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
		if !strings.HasPrefix(key, "catalog/") || seen[key] {
			return state, size, fmt.Errorf("invalid catalog part key")
		}
		seen[key] = true
		data, _, err := objects.Read(ctx, key)
		if err != nil {
			return state, size, fmt.Errorf("read catalog part: %w", err)
		}
		if catalogPartKey(data) != key {
			return state, size, fmt.Errorf("catalog part digest mismatch")
		}
		var part struct {
			Images map[string]domain.Image `json:"images"`
			Posts  map[string]postEntry    `json:"posts"`
		}
		if err := json.Unmarshal(data, &part); err != nil {
			return state, size, err
		}
		if part.Images == nil || part.Posts == nil {
			return state, size, fmt.Errorf("invalid catalog part")
		}
		for id, p := range part.Images {
			if _, exists := state.Images[id]; exists {
				return state, size, fmt.Errorf("duplicate image in catalog parts")
			}
			state.Images[id] = p
		}
		for id, p := range part.Posts {
			if _, exists := state.Posts[id]; exists {
				return state, size, fmt.Errorf("duplicate post in catalog parts")
			}
			state.Posts[id] = p
		}
		size += len(data)
	}
	return state, size, validateCatalog(state)
}

func catalogPartKey(raw []byte) string {
	return fmt.Sprintf("catalog/%x.json", sha256.Sum256(raw))
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
		root.Images = map[string]domain.Image{}
		root.Posts = map[string]postEntry{}
		part := catalogPart{Images: map[string]json.RawMessage{}, Posts: map[string]json.RawMessage{}}
		partSize := 128
		flush := func() error {
			if len(part.Images)+len(part.Posts) == 0 {
				return nil
			}
			data, err := json.Marshal(part)
			if err != nil {
				return err
			}
			if len(data) > maxObjectJSON {
				return domain.Invalid("catalog record exceeds 16 MiB capacity")
			}
			key := catalogPartKey(data)
			if !slices.Contains(r.state.Parts, key) {
				if _, err = r.objects.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/json", nil); err != nil {
					return err
				}
			}
			root.Parts = append(root.Parts, key)
			part = catalogPart{Images: map[string]json.RawMessage{}, Posts: map[string]json.RawMessage{}}
			partSize = 128
			return nil
		}
		add := func(id string, value any, image bool) error {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			name, _ := json.Marshal(id)
			n := len(data) + len(name) + 2
			if partSize+n > catalogPartSize {
				if err := flush(); err != nil {
					return err
				}
			}
			if image {
				part.Images[id] = data
			} else {
				part.Posts[id] = data
			}
			partSize += n
			return nil
		}
		for _, id := range slices.Sorted(maps.Keys(next.Images)) {
			if err := add(id, next.Images[id], true); err != nil {
				return nil, 0, err
			}
		}
		for _, id := range slices.Sorted(maps.Keys(next.Posts)) {
			if err := add(id, next.Posts[id], false); err != nil {
				return nil, 0, err
			}
		}
		if err := flush(); err != nil {
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
		return nil, 0, domain.Invalid("catalog manifest exceeds 16 MiB capacity")
	}
	next.Version = root.Version
	next.Parts = root.Parts
	next.Garbage = root.Garbage
	return raw, size, nil
}
