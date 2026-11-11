package database

import (
	"context"
	"database/sql"
)

// Keep summaries in separate rows so public listings never read body overflow
// pages. Triggers cover edits, imports, and deletes in the same transaction.
func (r *Repository) migratePostSummaries(ctx context.Context) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS post_summaries (
 id TEXT PRIMARY KEY,
 published INTEGER NOT NULL,
 created_at TEXT NOT NULL,
 payload BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS post_summaries_listing
 ON post_summaries(published,created_at DESC,id DESC);
CREATE TRIGGER IF NOT EXISTS post_summary_insert AFTER INSERT ON content
 WHEN NEW.kind='post' BEGIN
 INSERT INTO post_summaries VALUES(NEW.id,NEW.published,NEW.created_at,
  CAST(json_remove(CAST(NEW.payload AS TEXT),'$.body') AS BLOB));
END;
CREATE TRIGGER IF NOT EXISTS post_summary_update AFTER UPDATE ON content
 WHEN NEW.kind='post' BEGIN
 INSERT OR REPLACE INTO post_summaries VALUES(NEW.id,NEW.published,NEW.created_at,
  CAST(json_remove(CAST(NEW.payload AS TEXT),'$.body') AS BLOB));
END;
CREATE TRIGGER IF NOT EXISTS post_summary_delete AFTER DELETE ON content
 WHEN OLD.kind='post' BEGIN
 DELETE FROM post_summaries WHERE id=OLD.id;
END;
INSERT INTO post_summaries
 SELECT id,published,created_at,CAST(json_remove(CAST(payload AS TEXT),'$.body') AS BLOB)
 FROM content WHERE kind='post'
 AND NOT EXISTS (SELECT 1 FROM settings WHERE key='post_summaries_v1');
INSERT OR IGNORE INTO settings(key,value) VALUES('post_summaries_v1','1');
`)
		return err
	})
}
