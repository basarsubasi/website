package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/domain"
)

func (r *Repository) GetPostByID(ctx context.Context, id string) (domain.Post, error) {
	return scan[domain.Post](r.db.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND id=?", id))
}

func (r *Repository) GetPost(ctx context.Context, slug string) (domain.Post, error) {
	return scan[domain.Post](r.db.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND slug=? AND published=1", slug))
}

func (r *Repository) ListPosts(ctx context.Context, pagination domain.Pagination, drafts bool) (domain.Page[domain.Post], error) {
	if !drafts {
		return listFrom[domain.Post](ctx, r.db, pagination, "post_summaries", "published=1")
	}
	return list[domain.Post](ctx, r.db, pagination, "kind='post'")
}

func (r *Repository) CreatePost(ctx context.Context, post domain.Post) (domain.Post, error) {
	post.CreatedAt = now()
	post.UpdatedAt = post.CreatedAt
	err := r.change(ctx, func(tx *sql.Tx) error {
		if err := insert(ctx, tx, "post", post.ID, post.Slug, "", post.Published, post.CreatedAt, post); err != nil {
			return err
		}
		return enqueue(ctx, tx, "post", post)
	})
	return post, err
}

func (r *Repository) UpdatePost(ctx context.Context, id string, input domain.PostInput) (domain.Post, error) {
	var post domain.Post
	err := r.change(ctx, func(tx *sql.Tx) error {
		var err error
		post, err = scan[domain.Post](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND id=?", id))
		if err != nil {
			return err
		}
		post.Slug = input.Slug
		post.Title = input.Title
		post.Excerpt = input.Excerpt
		post.Body = input.Body
		post.Published = input.Published
		post.UpdatedAt = now()
		raw, err := json.Marshal(post)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE content SET slug=?,published=?,payload=? WHERE kind='post' AND id=?", post.Slug, post.Published, raw, id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "post", post)
	})
	return post, err
}

func (r *Repository) DeletePost(ctx context.Context, id string) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		post, err := scan[domain.Post](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND id=?", id))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM content WHERE kind='post' AND id=?", id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "delete-post", post)
	})
}
