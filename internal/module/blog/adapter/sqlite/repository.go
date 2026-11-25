package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	store "github.com/basarsubasi/website/gen/sqlc/blog"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/platform/database"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	postKind            = "post"
	emptyColor          = ""
	deletePostOperation = "delete-post"
)

type Repository struct{ store *database.Store }

func New(store *database.Store) *Repository { return &Repository{store: store} }

func (r *Repository) GetPostByID(ctx context.Context, id string) (article.Post, error) {
	return database.DecodePayload[article.Post](store.New(r.store.DB()).GetPostByID(ctx, id))
}

func (r *Repository) GetPost(ctx context.Context, slug string) (article.Post, error) {
	return database.DecodePayload[article.Post](store.New(r.store.DB()).GetPublishedPost(ctx, slug))
}

func (r *Repository) ListPosts(ctx context.Context, pagination domain.Pagination, drafts bool) (domain.Page[article.Post], error) {
	return database.ListPage[article.Post](ctx, r.store.DB(), pagination, func(tx *sql.Tx) (int64, error) {
		if drafts {
			return store.New(tx).CountPosts(ctx)
		}
		return store.New(tx).CountPublishedPosts(ctx)
	}, func(tx *sql.Tx) ([][]byte, error) {
		if drafts {
			return store.New(tx).ListPosts(ctx, store.ListPostsParams{PageLimit: int64(pagination.Limit), PageOffset: int64((pagination.Page - 1) * pagination.Limit)})
		}
		return store.New(tx).ListPublishedPosts(ctx, store.ListPublishedPostsParams{PageLimit: int64(pagination.Limit), PageOffset: int64((pagination.Page - 1) * pagination.Limit)})
	})
}

func (r *Repository) CreatePost(ctx context.Context, post article.Post) (article.Post, error) {
	post.CreatedAt = database.Now()
	post.UpdatedAt = post.CreatedAt
	err := r.store.Change(ctx, func(tx *sql.Tx) error {
		if err := database.Insert(ctx, tx, postKind, post.ID, post.Slug, emptyColor, post.Published, post.CreatedAt, post); err != nil {
			return err
		}
		return database.Enqueue(ctx, tx, postKind, post)
	})
	return post, err
}

func (r *Repository) UpdatePost(ctx context.Context, id string, input article.PostInput) (article.Post, error) {
	var post article.Post
	err := r.store.Change(ctx, func(tx *sql.Tx) error {
		var err error
		post, err = database.DecodePayload[article.Post](store.New(tx).GetPostByID(ctx, id))
		if err != nil {
			return err
		}
		post.Slug = input.Slug
		post.Title = input.Title
		post.Excerpt = input.Excerpt
		post.Body = input.Body
		post.Published = input.Published
		post.UpdatedAt = database.Now()
		raw, err := json.Marshal(post)
		if err != nil {
			return err
		}
		if err = store.New(tx).UpdatePost(ctx, store.UpdatePostParams{Slug: post.Slug, Published: post.Published, Payload: raw, ID: id}); err != nil {
			return err
		}
		return database.Enqueue(ctx, tx, postKind, post)
	})
	return post, err
}

func (r *Repository) DeletePost(ctx context.Context, id string) error {
	return r.store.Change(ctx, func(tx *sql.Tx) error {
		post, err := database.DecodePayload[article.Post](store.New(tx).GetPostByID(ctx, id))
		if err != nil {
			return err
		}
		if err = store.New(tx).DeletePost(ctx, id); err != nil {
			return err
		}
		return database.Enqueue(ctx, tx, deletePostOperation, post)
	})
}
