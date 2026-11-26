package photos

import (
	"context"
	"io"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
)

type Repository interface {
	ListImages(context.Context, domain.Pagination, string) (domain.Page[photo.Image], error)
	GetImage(context.Context, string) (photo.Image, error)
	CreateImage(context.Context, photo.Image) (photo.Image, error)
	UpdateImage(context.Context, string, photo.PhotoInput) (photo.Image, error)
	DeleteImage(context.Context, string) error
	MoveImage(context.Context, string, string) error
}

type Storage interface {
	Put(context.Context, string, io.Reader, int64, string, map[string]string) (string, error)
	Delete(context.Context, string) error
}

type UseCase struct {
	repo   Repository
	photos Storage
}

func New(repo Repository, photos Storage) *UseCase {
	return &UseCase{repo: repo, photos: photos}
}
