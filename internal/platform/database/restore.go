package database

import (
	"context"
	"database/sql"
	"errors"

	store "github.com/basarsubasi/website/gen/sqlc/storage"
	"github.com/basarsubasi/website/internal/platform/backup"
)

const (
	imageKind = "image"

	postKind = "post"
)

// Initialize imports a consistent backup once. An empty but initialized database
// stays empty on restart; deleted records cannot be resurrected by stale backups.
func (r *Store) Initialize(ctx context.Context, load func(context.Context) (backup.Snapshot, error)) error {
	_, err := store.New(r.db).GetInitialized(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	snapshot, err := load(ctx)
	if err != nil {
		return err
	}
	return r.Change(ctx, func(tx *sql.Tx) error {
		_, err := store.New(tx).GetInitialized(ctx)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		for _, p := range snapshot.Images {
			if err := InsertImage(ctx, tx, p); err != nil {
				return err
			}
		}
		for _, p := range snapshot.Posts {
			if err := InsertPost(ctx, tx, p); err != nil {
				return err
			}
		}
		err = store.New(tx).SetInitialized(ctx)
		return err
	})
}
