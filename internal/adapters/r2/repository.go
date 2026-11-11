package r2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

type ObjectStore interface {
	usecase.PhotoStorage
	Read(context.Context, string) ([]byte, string, error)
	PutIf(context.Context, string, []byte, string, bool) (string, error)
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
		empty := catalog{
			Version:  catalogVersion,
			Revision: uuid.NewV4().String(),
			Images:   make(map[string]domain.Image),
			Posts:    make(map[string]postEntry),
			Garbage:  []string{},
		}
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

func (r *Repository) Ping(ctx context.Context) error {
	_, err := r.snapshot(ctx)
	return err
}
