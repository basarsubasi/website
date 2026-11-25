package backup

import (
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
)

// Snapshot is the metadata and post content restored from an R2 backup.
type Snapshot struct {
	Images []photo.Image
	Posts  []article.Post
}
