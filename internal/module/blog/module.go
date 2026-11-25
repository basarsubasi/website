// Package blog wires post publishing and image uploads.
package blog

import (
	"github.com/basarsubasi/website/internal/module/blog/usecase/images"
	"github.com/basarsubasi/website/internal/module/blog/usecase/posts"
	"github.com/basarsubasi/website/internal/shared/media"
)

type Module struct {
	*posts.UseCase
	Images *images.UseCase
}

func New(repo posts.Repository, storage images.Storage, imageOrigin media.Origin) *Module {
	return &Module{UseCase: posts.New(repo, imageOrigin), Images: images.New(storage)}
}
