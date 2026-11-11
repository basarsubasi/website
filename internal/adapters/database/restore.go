package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/basarsubasi/website/internal/domain"
)

// Initialize imports a consistent backup once. An empty but initialized database
// stays empty on restart; deleted records cannot be resurrected by stale backups.
func (r *Repository) Initialize(ctx context.Context, load func(context.Context) (domain.Snapshot, error)) error {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='initialized'").Scan(&value)
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
	return r.change(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='initialized'").Scan(&value)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		for _, p := range snapshot.Images {
			if err := insert(ctx, tx, "image", p.UUID, "", p.Color, 0, p.CreatedAt, p); err != nil {
				return err
			}
		}
		for _, p := range snapshot.Posts {
			if err := insert(ctx, tx, "post", p.ID, p.Slug, "", p.Published, p.CreatedAt, p); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('initialized','1')")
		return err
	})
}
