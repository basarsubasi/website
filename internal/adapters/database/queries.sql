-- name: GetPostByID :one
SELECT payload FROM content WHERE kind='post' AND id=?;

-- name: GetPublishedPost :one
SELECT payload FROM content WHERE kind='post' AND slug=? AND published=1;

-- name: GetImage :one
SELECT payload FROM content WHERE kind='image' AND id=?;

-- name: CountPosts :one
SELECT count(*) FROM content WHERE kind='post';

-- name: ListPosts :many
SELECT payload FROM content WHERE kind='post' ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountPublishedPosts :one
SELECT count(*) FROM post_summaries WHERE published=1;

-- name: ListPublishedPosts :many
SELECT payload FROM post_summaries WHERE published=1 ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountImages :one
SELECT count(*) FROM content WHERE kind='image' AND (color=sqlc.arg(color) OR sqlc.arg(color) = '');

-- name: ListImages :many
SELECT payload FROM content WHERE kind='image' AND (color=sqlc.arg(color) OR sqlc.arg(color) = '') ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: InsertContent :exec
INSERT INTO content(kind,id,slug,color,published,created_at,payload) VALUES(?,?,?,?,?,?,?);

-- name: UpdatePost :exec
UPDATE content SET slug=?,published=?,payload=? WHERE kind='post' AND id=?;

-- name: UpdateImage :exec
UPDATE content SET color=?,payload=? WHERE kind='image' AND id=?;

-- name: DeletePost :exec
DELETE FROM content WHERE kind='post' AND id=?;

-- name: DeleteImage :exec
DELETE FROM content WHERE kind='image' AND id=?;

-- name: EnqueueBackup :exec
INSERT INTO backup_queue(operation,payload) VALUES(?,?);

-- name: CountBackups :one
SELECT count(*) FROM backup_queue;

-- name: NextBackup :one
SELECT sequence,operation,payload FROM backup_queue ORDER BY sequence LIMIT 1;

-- name: DeleteBackup :exec
DELETE FROM backup_queue WHERE sequence=?;

-- name: GetInitialized :one
SELECT value FROM settings WHERE key='initialized';

-- name: SetInitialized :exec
INSERT INTO settings(key,value) VALUES('initialized','1');

-- name: BackfillPostSummaries :exec
INSERT INTO post_summaries
 SELECT id,published,created_at,CAST(json_remove(CAST(payload AS TEXT),'$.body') AS BLOB)
 FROM content WHERE kind='post'
 AND NOT EXISTS (SELECT 1 FROM settings WHERE key='post_summaries_v1');

-- name: MarkPostSummariesMigrated :exec
INSERT OR IGNORE INTO settings(key,value) VALUES('post_summaries_v1','1');
