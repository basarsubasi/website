CREATE TABLE IF NOT EXISTS content (
 kind TEXT NOT NULL CHECK(kind IN ('image','post')),
 id TEXT NOT NULL PRIMARY KEY,
 slug TEXT NOT NULL DEFAULT '',
 color TEXT NOT NULL DEFAULT '',
 published INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL,
 payload BLOB NOT NULL
 );
 CREATE UNIQUE INDEX IF NOT EXISTS content_post_slug ON content(slug) WHERE kind='post';
 CREATE INDEX IF NOT EXISTS content_listing ON content(kind,created_at DESC,id DESC);
 CREATE TABLE IF NOT EXISTS backup_queue (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 operation TEXT NOT NULL,
 payload BLOB NOT NULL
 );
 CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY,value TEXT NOT NULL);
