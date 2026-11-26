package r2

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
)

const allColors = ""

func (r *Repository) ListImages(ctx context.Context, p pagination.Pagination, color string) (pagination.Page[photo.Image], error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return pagination.Page[photo.Image]{}, err
	}
	items := []photo.Image{}
	for _, photo := range state.Images {
		if color == allColors || photo.Color == color {
			items = append(items, photo)
		}
	}
	slices.SortFunc(items, func(a, b photo.Image) int {
		return cmp.Compare(b.GalleryOrderKey(), a.GalleryOrderKey())
	})
	return paginate(items, p), nil
}

func (r *Repository) GetImage(ctx context.Context, id string) (photo.Image, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return photo.Image{}, err
	}
	found, ok := state.Images[id]
	if !ok {
		return photo.Image{}, storage.ErrNotFound
	}
	return found, nil
}

func (r *Repository) CreateImage(ctx context.Context, photo photo.Image) (photo.Image, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	photo.CreatedAt = time.Now().UTC().Format(timestampLayout)
	err := r.mutate(ctx, func(next *catalog) error {
		if _, exists := next.Images[photo.UUID]; exists {
			return storage.ErrConflict
		}
		next.Images[photo.UUID] = photo
		return nil
	})
	return photo, err
}

func (r *Repository) UpdateImage(ctx context.Context, id string, p photo.PhotoInput) (photo.Image, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	var photo photo.Image
	err := r.mutate(ctx, func(next *catalog) error {
		var exists bool
		photo, exists = next.Images[id]
		if !exists {
			return storage.ErrNotFound
		}
		photo.Name = p.Name
		photo.Country = p.Country
		photo.City = p.City
		photo.YearTaken = p.YearTaken
		photo.Color = p.Color
		next.Images[id] = photo
		return nil
	})
	return photo, err
}

func (r *Repository) DeleteImage(ctx context.Context, id string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return r.mutate(ctx, func(next *catalog) error {
		image, exists := next.Images[id]
		if !exists {
			return storage.ErrNotFound
		}
		delete(next.Images, id)
		next.Garbage = append(next.Garbage, image.ObjectKey)
		if image.PreviewURL != r2EmptyString {
			next.Garbage = append(next.Garbage, photo.PreviewKey(id))
		}
		return nil
	})
}

func (r *Repository) MoveImage(ctx context.Context, id, direction string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return r.mutate(ctx, func(next *catalog) error {
		images := make([]photo.Image, 0, len(next.Images))
		for _, image := range next.Images {
			images = append(images, image)
		}
		changed, err := photo.MoveImage(images, id, direction)
		if err != nil {
			return err
		}
		for _, image := range changed {
			next.Images[image.UUID] = image
		}
		return nil
	})
}
