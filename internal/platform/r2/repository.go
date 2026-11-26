package r2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"uuid"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
)

const (
	loadCatalogErrorFormat = "load R2 catalog: %w"
	repositoryEmptyString  = ""
	initCatalogErrorFormat = "initialize R2 catalog: %w"
)

type ObjectStore interface {
	Put(context.Context, string, io.Reader, int64, string, map[string]string) (string, error)
	UpdateMetadata(context.Context, string, string, map[string]string) error
	Delete(context.Context, string) error
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

func OpenRepository(ctx context.Context, objects ObjectStore) (*Repository, error) {
	if objects == nil {
		return nil, storage.ErrUnavailable
	}
	repo := &Repository{objects: objects, reads: newReadCache()}
	if err := repo.load(ctx); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf(loadCatalogErrorFormat, err)
		}
		empty := catalog{
			Version:  catalogVersion,
			Revision: uuid.NewV4().String(),
			Images:   make(map[string]photo.Image),
			Posts:    make(map[string]postEntry),
			Garbage:  []string{},
		}
		raw, _ := json.Marshal(empty)
		etag, err := objects.PutIf(ctx, CatalogKey, raw, repositoryEmptyString, true)
		if err != nil {
			// Another instance may have initialized it, or our response may have been lost.
			if readErr := repo.load(ctx); readErr != nil {
				return nil, fmt.Errorf(initCatalogErrorFormat, err)
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
