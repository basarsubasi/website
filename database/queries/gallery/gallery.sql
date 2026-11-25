-- name: GetImage :one
SELECT payload FROM content WHERE kind='image' AND id=?;

-- name: CountImages :one
SELECT count(*) FROM content WHERE kind='image' AND (color=sqlc.arg(color) OR sqlc.arg(color) = '');

-- name: ListImages :many
SELECT payload FROM content WHERE kind='image' AND (color=sqlc.arg(color) OR sqlc.arg(color) = '') ORDER BY COALESCE(NULLIF(json_extract(payload, '$.order_key'), ''), created_at || '/' || id) DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateImage :exec
UPDATE content SET color=?,payload=? WHERE kind='image' AND id=?;

-- name: DeleteImage :exec
DELETE FROM content WHERE kind='image' AND id=?;

-- name: AllImages :many
SELECT payload FROM content WHERE kind='image';
