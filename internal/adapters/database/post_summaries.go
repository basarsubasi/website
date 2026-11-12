package database

import (
	"context"
	"database/sql"

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
)

// Keep summaries in separate rows so public listings never read body overflow
// pages. Triggers cover edits, imports, and deletes in the same transaction.
func (r *Repository) migratePostSummaries(ctx context.Context) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, postSummariesSQL)
		if err != nil {
			return err
		}
		q := dbquery.New(tx)
		if err := q.BackfillPostSummaries(ctx); err != nil {
			return err
		}
		return q.MarkPostSummariesMigrated(ctx)
	})
}
