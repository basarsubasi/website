package r2

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/basarsubasi/website/internal/domain"
)

func (r *Repository) readPost(ctx context.Context, entry postEntry) (domain.Post, error) {
	result, err := r.reads.get(ctx, r.objects, entry.ObjectKey)
	if err != nil {
		return domain.Post{}, err
	}
	post := result.post
	if post.ID != entry.Metadata.ID || post.UpdatedAt != entry.Metadata.UpdatedAt {
		return domain.Post{}, fmt.Errorf("post object does not match catalog")
	}
	return post, nil
}

func (r *Repository) ListPosts(ctx context.Context, p domain.Pagination, includeDrafts bool) (domain.Page[domain.Post], error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Page[domain.Post]{}, err
	}
	items := []domain.Post{}
	for _, entry := range state.Posts {
		if includeDrafts || entry.Metadata.Published == 1 {
			items = append(items, entry.Metadata)
		}
	}
	slices.SortFunc(items, func(a, b domain.Post) int {
		if order := cmp.Compare(b.CreatedAt, a.CreatedAt); order != 0 {
			return order
		}
		return cmp.Compare(b.ID, a.ID)
	})
	page := paginate(items, p)
	if includeDrafts {
		for i, post := range page.Data {
			full, err := r.readPost(ctx, state.Posts[post.ID])
			if err != nil {
				return page, err
			}
			page.Data[i] = full
		}
	}
	return page, nil
}

func (r *Repository) GetPost(ctx context.Context, slug string) (domain.Post, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Post{}, err
	}
	for _, entry := range state.Posts {
		if entry.Metadata.Slug == slug && entry.Metadata.Published == 1 {
			return r.readPost(ctx, entry)
		}
	}
	return domain.Post{}, domain.ErrNotFound
}

func (r *Repository) GetPostByID(ctx context.Context, id string) (domain.Post, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Post{}, err
	}
	entry, ok := state.Posts[id]
	if !ok {
		return domain.Post{}, domain.ErrNotFound
	}
	return r.readPost(ctx, entry)
}

func (r *Repository) CreatePost(ctx context.Context, post domain.Post) (domain.Post, error) {
	return r.savePost(ctx, post.ID, post.Slug, func(_ postEntry, exists bool) (domain.Post, error) {
		if exists {
			return domain.Post{}, domain.ErrConflict
		}
		post.CreatedAt = time.Now().UTC().Format(timestampLayout)
		post.UpdatedAt = post.CreatedAt
		return post, nil
	})
}

func (r *Repository) UpdatePost(ctx context.Context, id string, input domain.PostInput) (domain.Post, error) {
	return r.savePost(ctx, id, input.Slug, func(entry postEntry, exists bool) (domain.Post, error) {
		if !exists {
			return domain.Post{}, domain.ErrNotFound
		}
		post := entry.Metadata
		post.Slug = input.Slug
		post.Title = input.Title
		post.Excerpt = input.Excerpt
		post.Body = input.Body
		post.Published = input.Published
		post.UpdatedAt = time.Now().UTC().Format(timestampLayout)
		return post, nil
	})
}

func (r *Repository) DeletePost(ctx context.Context, id string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return r.mutate(ctx, func(next *catalog) error {
		entry, exists := next.Posts[id]
		if !exists {
			return domain.ErrNotFound
		}
		delete(next.Posts, id)
		next.Garbage = append(next.Garbage, entry.ObjectKey)
		return nil
	})
}
