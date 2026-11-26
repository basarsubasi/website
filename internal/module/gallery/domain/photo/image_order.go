package photo

import (
	"cmp"
	"slices"

	"github.com/basarsubasi/website/internal/shared/domain/storage"
	"github.com/basarsubasi/website/internal/shared/domain/validation"
)

const (
	emptyOrderKey           = ""
	orderKeySeparator       = "/"
	moveUp                  = "up"
	moveDown                = "down"
	invalidDirectionMessage = "direction must be up or down"
)

// GalleryOrderKey preserves the original newest-first order for unmoved photos.
func (image Image) GalleryOrderKey() string {
	if image.OrderKey != emptyOrderKey {
		return image.OrderKey
	}
	return image.CreatedAt + orderKeySeparator + image.UUID
}

// MoveImage swaps two adjacent slots, leaving upload dates and metadata intact.
// Only the changed photos need to be persisted.
func MoveImage(images []Image, id, direction string) ([]Image, error) {
	if direction != moveUp && direction != moveDown {
		return nil, validation.Invalid(invalidDirectionMessage)
	}
	slices.SortFunc(images, func(a, b Image) int { return cmp.Compare(b.GalleryOrderKey(), a.GalleryOrderKey()) })
	for i := range images {
		if images[i].UUID != id {
			continue
		}
		neighbor := i - 1
		if direction == moveDown {
			neighbor = i + 1
		}
		if neighbor < 0 || neighbor >= len(images) {
			return nil, nil
		}
		a, b := images[i], images[neighbor]
		a.OrderKey, b.OrderKey = b.GalleryOrderKey(), a.GalleryOrderKey()
		return []Image{a, b}, nil
	}
	return nil, storage.ErrNotFound
}
