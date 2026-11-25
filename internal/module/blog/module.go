// Package blog wires post publishing and image uploads.
package blog

import (
	"github.com/basarsubasi/website/internal/module/blog/usecase/images"
	"github.com/basarsubasi/website/internal/module/blog/usecase/posts"
)

type Module struct {
	*posts.UseCase
	Images *images.UseCase
}

func New(repo posts.Repository, storage images.Storage) *Module {
	return &Module{UseCase: posts.New(repo), Images: images.New(storage)}
}
