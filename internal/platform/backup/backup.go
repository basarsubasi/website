package backup

import (
	"context"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
)

// Target persists records in R2; public reads use the local SQLite cache.
type Target interface {
	LoadSnapshot(context.Context) (Snapshot, error)
	SaveImage(context.Context, photo.Image) error
	RemoveImage(context.Context, photo.Image) error
	SavePost(context.Context, article.Post) error
	RemovePost(context.Context, article.Post) error
	Maintain(context.Context) error
}
