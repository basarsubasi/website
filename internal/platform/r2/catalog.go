package r2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	uncertainWriteFormat     = "%w: %v"
	invalidCatalogJSONFormat = "invalid catalog JSON: %w"
	catalogEmptyString       = ""
	invalidCatalogMessage    = "invalid catalog version or structure"
	invalidPartsMessage      = "invalid catalog parts"
	blackAndWhite            = "bw"
	colorPhoto               = "color"
	invalidImageEntryMessage = "invalid image entry in catalog"
	invalidPostEntryMessage  = "invalid post entry in catalog"
)

// Fixed precision keeps timestamp strings in chronological sort order.
const timestampLayout = "2006-01-02T15:04:05.000000000Z"

const CatalogKey = "catalog.json"

const catalogVersion = 1

type postEntry struct {
	Metadata  article.Post `json:"metadata"`
	ObjectKey string       `json:"object_key"`
}

type catalog struct {
	Version  int                    `json:"version"`
	Revision string                 `json:"revision"`
	Images   map[string]photo.Image `json:"images"`
	Posts    map[string]postEntry   `json:"posts"`
	Garbage  []string               `json:"pending_deletes"`
	Parts    []string               `json:"parts,omitempty"`
}

func (r *Repository) load(ctx context.Context) error {
	raw, etag, err := r.objects.Read(ctx, CatalogKey)
	if err != nil {
		return err
	}
	state, size, err := readCatalog(ctx, r.objects, raw, etag)
	if err != nil {
		return err
	}
	r.state = state
	r.etag = etag
	r.reads.publish(CatalogKey, &readResult{state: state, size: size})
	return nil
}

func (state catalog) clone() catalog {
	return catalog{
		Version:  state.Version,
		Revision: state.Revision,
		Images:   maps.Clone(state.Images),
		Posts:    maps.Clone(state.Posts),
		Garbage:  append([]string{}, state.Garbage...),
		Parts:    append([]string{}, state.Parts...),
	}
}

func (r *Repository) commit(ctx context.Context, next catalog) error {
	next.Revision = uuid.NewV4().String()
	raw, size, err := r.encodeCatalog(ctx, &next)
	if err != nil {
		return err
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
		return fmt.Errorf(uncertainWriteFormat, domain.ErrUncertain, err)
	}
	r.state = next
	r.etag = etag
	r.reads.publish(CatalogKey, &readResult{state: next, size: size})
	return nil
}

// mutate requires writeMu to be held by the caller. Published catalog maps
// are immutable; edits are applied to a private copy before committing.
func (r *Repository) mutate(ctx context.Context, fn func(*catalog) error) error {
	if err := r.load(ctx); err != nil {
		return err
	}
	next := r.state.clone()
	if err := fn(&next); err != nil {
		return err
	}
	if err := r.commit(ctx, next); err != nil {
		return err
	}
	r.cleanup(ctx)
	return nil
}

func parseCatalog(raw []byte, etag string) (catalog, error) {
	var state catalog
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf(invalidCatalogJSONFormat, err)
	}
	if (state.Version != catalogVersion && state.Version != shardedCatalogVersion) || state.Revision == catalogEmptyString || state.Images == nil || state.Posts == nil || etag == catalogEmptyString {
		return state, fmt.Errorf(invalidCatalogMessage)
	}
	if (state.Version == catalogVersion && len(state.Parts) != 0) ||
		(state.Version == shardedCatalogVersion && (len(state.Parts) == 0 || len(state.Images) != 0 || len(state.Posts) != 0)) {
		return state, fmt.Errorf(invalidPartsMessage)
	}
	return state, validateCatalog(state)
}

func validateCatalog(state catalog) error {
	slugs := map[string]bool{}
	for id, photo := range state.Images {
		if id != photo.UUID || photo.ObjectKey == catalogEmptyString || (photo.Color != blackAndWhite && photo.Color != colorPhoto) {
			return fmt.Errorf(invalidImageEntryMessage)
		}
	}
	for id, post := range state.Posts {
		if id != post.Metadata.ID || post.ObjectKey == catalogEmptyString || post.Metadata.Slug == catalogEmptyString || slugs[post.Metadata.Slug] {
			return fmt.Errorf(invalidPostEntryMessage)
		}
		slugs[post.Metadata.Slug] = true
	}
	return nil
}
