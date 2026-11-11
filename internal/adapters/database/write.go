package database

import (
	"context"
	"database/sql"
	"encoding/json"
)

func insert(ctx context.Context, tx *sql.Tx, kind, id, slug, color string, published int64, created string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO content(kind,id,slug,color,published,created_at,payload) VALUES(?,?,?,?,?,?,?)", kind, id, slug, color, published, created, raw)
	return dbError(err)
}

func enqueue(ctx context.Context, tx *sql.Tx, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO backup_queue(operation,payload) VALUES(?,?)", op, raw)
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
