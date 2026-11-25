// Package gallery exposes photo listings and gallery edits.
package gallery

import (
	"context"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/module/gallery/usecase/photos"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	allColors           = ""
	blackAndWhite       = "bw"
	colorPhoto          = "color"
	invalidColorMessage = "color must be bw or color"
)

type Repository interface {
	photos.Repository
	ListImages(context.Context, domain.Pagination, string) (domain.Page[photo.Image], error)
	GetImage(context.Context, string) (photo.Image, error)
}

type Module struct {
	*photos.UseCase
	repo Repository
}

func New(repo Repository, files photos.Storage) *Module {
	return &Module{UseCase: photos.New(repo, files), repo: repo}
}

func (s *Module) ListImages(ctx context.Context, p domain.Pagination, color string) (domain.Page[photo.Image], error) {
	if err := p.Validate(); err != nil {
		return domain.Page[photo.Image]{}, err
	}
	if color != allColors && color != blackAndWhite && color != colorPhoto {
		return domain.Page[photo.Image]{}, domain.Invalid(invalidColorMessage)
	}
	return s.repo.ListImages(ctx, p, color)
}

func (s *Module) GetImage(ctx context.Context, id string) (photo.Image, error) {
	return s.repo.GetImage(ctx, id)
}
