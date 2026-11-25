package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	store "github.com/basarsubasi/website/gen/sqlc/storage"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	syncOperation        = "synchronize pending changes"
	uncertainWriteFormat = "%w: %v"
)

func Insert(ctx context.Context, tx *sql.Tx, kind, id, slug, color string, published int64, created string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	err = store.New(tx).InsertContent(ctx, store.InsertContentParams{Kind: kind, ID: id, Slug: slug, Color: color, Published: published, CreatedAt: created, Payload: raw})
	return dbError(err)
}

func Enqueue(ctx context.Context, tx *sql.Tx, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	err = store.New(tx).EnqueueBackup(ctx, store.EnqueueBackupParams{Operation: op, Payload: raw})
	return err
}

func (r *Store) Change(ctx context.Context, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	select {
	case r.writes <- struct{}{}:
		defer func() { <-r.writes }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if r.backup != nil {
		if err := r.FlushBackups(ctx, r.backup); err != nil {
			return &domain.StorageError{Operation: syncOperation, Err: err}
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return dbError(err)
	}
	if err := tx.Commit(); err != nil {
		return dbError(err)
	}
	if r.backup != nil {
		if err := r.FlushBackups(ctx, r.backup); err != nil {
			// The SQLite commit happened: upload cleanup must not delete the object
			// referenced by this record while its backup is pending.
			return fmt.Errorf(uncertainWriteFormat, domain.ErrUncertain, err)
		}
	}
	return nil
}
