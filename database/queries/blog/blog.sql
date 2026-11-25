-- name: GetPostByID :one
SELECT payload FROM content WHERE kind='post' AND id=?;

-- name: GetPublishedPost :one
SELECT payload FROM content WHERE kind='post' AND slug=? AND published=1;

-- name: CountPosts :one
SELECT count(*) FROM content WHERE kind='post';

-- name: ListPosts :many
SELECT payload FROM content WHERE kind='post' ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountPublishedPosts :one
SELECT count(*) FROM post_summaries WHERE published=1;

-- name: ListPublishedPosts :many
SELECT payload FROM post_summaries WHERE published=1 ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdatePost :exec
UPDATE content SET slug=?,published=?,payload=? WHERE kind='post' AND id=?;

-- name: DeletePost :exec
DELETE FROM content WHERE kind='post' AND id=?;
