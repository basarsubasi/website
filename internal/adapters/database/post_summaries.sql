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
