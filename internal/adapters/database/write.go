package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
	"github.com/basarsubasi/website/internal/domain"
)

func insert(ctx context.Context, tx *sql.Tx, kind, id, slug, color string, published int64, created string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	err = dbquery.New(tx).InsertContent(ctx, dbquery.InsertContentParams{Kind: kind, ID: id, Slug: slug, Color: color, Published: published, CreatedAt: created, Payload: raw})
	return dbError(err)
}

func enqueue(ctx context.Context, tx *sql.Tx, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	err = dbquery.New(tx).EnqueueBackup(ctx, dbquery.EnqueueBackupParams{Operation: op, Payload: raw})
	return err
}

func (r *Repository) change(ctx context.Context, fn func(*sql.Tx) error) error {
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
			return &domain.StorageError{Operation: "synchronize pending changes", Err: err}
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
			return fmt.Errorf("%w: %v", domain.ErrUncertain, err)
		}
	}
	return nil
}
