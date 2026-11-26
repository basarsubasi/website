// Package gallery exposes photo listings and gallery edits.
package gallery

import (
	"github.com/basarsubasi/website/internal/module/gallery/usecase/photos"
)

type Module struct {
	Photos *photos.UseCase
}

func New(repo photos.Repository, files photos.Storage) *Module {
	return &Module{Photos: photos.New(repo, files)}
}
