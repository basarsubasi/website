// Package migrations applies the embedded SQLite migrations in filename order.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
)

const (
	upMigrationGlob           = "*.up.sql"
	createMigrationTableSQL   = "CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)"
	recordMigrationSQL        = "INSERT OR IGNORE INTO schema_migrations (name) VALUES (?)"
	applyMigrationErrorFormat = "apply migration %s: %w"
)

//go:embed *.up.sql
var files embed.FS

func Apply(ctx context.Context, db *sql.DB) error {
	names, err := fs.Glob(files, upMigrationGlob)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, createMigrationTableSQL); err != nil {
		return err
	}
	for _, name := range names {
		result, err := tx.ExecContext(ctx, recordMigrationSQL, name)
		if err != nil {
			return err
		}
		added, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if added == 0 {
			continue
		}
		script, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf(applyMigrationErrorFormat, name, err)
		}
	}
	return tx.Commit()
}
