package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
	"github.com/basarsubasi/website/internal/domain"
)

func (r *Repository) GetPostByID(ctx context.Context, id string) (domain.Post, error) {
	return decodePayload[domain.Post](dbquery.New(r.db).GetPostByID(ctx, id))
}

func (r *Repository) GetPost(ctx context.Context, slug string) (domain.Post, error) {
	return decodePayload[domain.Post](dbquery.New(r.db).GetPublishedPost(ctx, slug))
}

func (r *Repository) ListPosts(ctx context.Context, pagination domain.Pagination, drafts bool) (domain.Page[domain.Post], error) {
	return listPage[domain.Post](ctx, r.db, pagination, func(q *dbquery.Queries) (int64, error) {
		if drafts {
			return q.CountPosts(ctx)
		}
		return q.CountPublishedPosts(ctx)
	}, func(q *dbquery.Queries) ([][]byte, error) {
		if drafts {
			return q.ListPosts(ctx, dbquery.ListPostsParams{PageLimit: int64(pagination.Limit), PageOffset: int64((pagination.Page - 1) * pagination.Limit)})
		}
		return q.ListPublishedPosts(ctx, dbquery.ListPublishedPostsParams{PageLimit: int64(pagination.Limit), PageOffset: int64((pagination.Page - 1) * pagination.Limit)})
	})
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
		post, err = decodePayload[domain.Post](dbquery.New(tx).GetPostByID(ctx, id))
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
		if err = dbquery.New(tx).UpdatePost(ctx, dbquery.UpdatePostParams{Slug: post.Slug, Published: post.Published, Payload: raw, ID: id}); err != nil {
			return err
		}
		return enqueue(ctx, tx, "post", post)
	})
	return post, err
}

func (r *Repository) DeletePost(ctx context.Context, id string) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		post, err := decodePayload[domain.Post](dbquery.New(tx).GetPostByID(ctx, id))
		if err != nil {
			return err
		}
		if err = dbquery.New(tx).DeletePost(ctx, id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "delete-post", post)
	})
}
