// Package blog wires post publishing and media uploads.
package blog

import (
	"github.com/basarsubasi/website/internal/module/blog/usecase/assets"
	"github.com/basarsubasi/website/internal/module/blog/usecase/posts"
	"github.com/basarsubasi/website/internal/shared/media"
)

type Module struct {
	*posts.UseCase
	Assets *assets.UseCase
}

func New(repo posts.Repository, storage assets.Storage, assetOrigin media.Origin) *Module {
	return &Module{UseCase: posts.New(repo, assetOrigin), Assets: assets.New(storage)}
}
