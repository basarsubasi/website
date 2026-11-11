package usecase

import (
	"context"
	"github.com/basarsubasi/website/internal/domain"
	"io"
)

type Repository interface {
	Ping(context.Context) error
	ListImages(context.Context, domain.Pagination, string) (domain.Page[domain.Image], error)
	GetImage(context.Context, string) (domain.Image, error)
	CreateImage(context.Context, domain.Image) (domain.Image, error)
	UpdateImage(context.Context, string, domain.PhotoInput) (domain.Image, error)
	DeleteImage(context.Context, string) error
	ListPosts(context.Context, domain.Pagination, bool) (domain.Page[domain.Post], error)
	GetPost(context.Context, string) (domain.Post, error)
	GetPostByID(context.Context, string) (domain.Post, error)
	CreatePost(context.Context, domain.Post) (domain.Post, error)
	UpdatePost(context.Context, string, domain.PostInput) (domain.Post, error)
	DeletePost(context.Context, string) error
}
type PhotoStorage interface {
	Put(context.Context, string, io.Reader, int64, string, map[string]string) (string, error)
	UpdateMetadata(context.Context, string, string, map[string]string) error
	Delete(context.Context, string) error
}

// Backup is an asynchronous mirror; public reads never use it.
type Backup interface {
	LoadSnapshot(context.Context) (domain.Snapshot, error)
	SaveImage(context.Context, domain.Image) error
	RemoveImage(context.Context, domain.Image) error
	SavePost(context.Context, domain.Post) error
	RemovePost(context.Context, domain.Post) error
	Maintain(context.Context) error
}
