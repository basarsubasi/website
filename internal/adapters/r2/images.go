package r2

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/basarsubasi/website/internal/domain"
)

func (r *Repository) ListImages(ctx context.Context, p domain.Pagination, color string) (domain.Page[domain.Image], error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Page[domain.Image]{}, err
	}
	items := []domain.Image{}
	for _, photo := range state.Images {
		if color == "" || photo.Color == color {
			items = append(items, photo)
		}
	}
	slices.SortFunc(items, func(a, b domain.Image) int {
		if order := cmp.Compare(b.CreatedAt, a.CreatedAt); order != 0 {
			return order
		}
		return cmp.Compare(b.UUID, a.UUID)
	})
	return paginate(items, p), nil
}

func (r *Repository) GetImage(ctx context.Context, id string) (domain.Image, error) {
	state, err := r.snapshot(ctx)
	if err != nil {
		return domain.Image{}, err
	}
	photo, ok := state.Images[id]
	if !ok {
		return domain.Image{}, domain.ErrNotFound
	}
	return photo, nil
}

func (r *Repository) CreateImage(ctx context.Context, photo domain.Image) (domain.Image, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	photo.CreatedAt = time.Now().UTC().Format(timestampLayout)
	err := r.mutate(ctx, func(next *catalog) error {
		if _, exists := next.Images[photo.UUID]; exists {
			return domain.ErrConflict
		}
		next.Images[photo.UUID] = photo
		return nil
	})
	return photo, err
}

func (r *Repository) UpdateImage(ctx context.Context, id string, p domain.PhotoInput) (domain.Image, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	var photo domain.Image
	err := r.mutate(ctx, func(next *catalog) error {
		var exists bool
		photo, exists = next.Images[id]
		if !exists {
			return domain.ErrNotFound
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
		photo, exists := next.Images[id]
		if !exists {
			return domain.ErrNotFound
		}
		delete(next.Images, id)
		next.Garbage = append(next.Garbage, photo.ObjectKey)
		return nil
	})
}
