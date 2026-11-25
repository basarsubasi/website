package posts

import (
	"context"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

type Repository interface {
	ListPosts(context.Context, domain.Pagination, bool) (domain.Page[article.Post], error)
	GetPost(context.Context, string) (article.Post, error)
	CreatePost(context.Context, article.Post) (article.Post, error)
	UpdatePost(context.Context, string, article.PostInput) (article.Post, error)
	DeletePost(context.Context, string) error
}

type UseCase struct {
	repo        Repository
	assetOrigin media.Origin
}

func New(repo Repository, assetOrigin media.Origin) *UseCase {
	return &UseCase{repo: repo, assetOrigin: assetOrigin}
}
