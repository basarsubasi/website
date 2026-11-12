package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return dbError(err)
	}
	return dbError(tx.Commit())
}
