package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	store "github.com/basarsubasi/website/gen/sqlc/gallery"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/database"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	imageKind = "image"

	deleteImageOperation = "delete-image"
)

type Repository struct{ store *database.Store }

func New(store *database.Store) *Repository { return &Repository{store: store} }

func (r *Repository) GetImage(ctx context.Context, id string) (photo.Image, error) {
	return database.DecodePayload[photo.Image](store.New(r.store.DB()).GetImage(ctx, id))
}

func (r *Repository) ListImages(ctx context.Context, pagination domain.Pagination, color string) (domain.Page[photo.Image], error) {
	return database.ListPage[photo.Image](ctx, r.store.DB(), pagination, func(tx *sql.Tx) (int64, error) {
		return store.New(tx).CountImages(ctx, color)
	}, func(tx *sql.Tx) ([][]byte, error) {
		return store.New(tx).ListImages(ctx, store.ListImagesParams{Color: color, PageLimit: int64(pagination.Limit), PageOffset: int64((pagination.Page - 1) * pagination.Limit)})
	})
}

func (r *Repository) CreateImage(ctx context.Context, image photo.Image) (photo.Image, error) {
	image.CreatedAt = database.Now()
	err := r.store.Change(ctx, func(tx *sql.Tx) error {
		if err := database.InsertImage(ctx, tx, image); err != nil {
			return err
		}
		return database.Enqueue(ctx, tx, imageKind, image)
	})
	return image, err
}

func (r *Repository) UpdateImage(ctx context.Context, id string, input photo.PhotoInput) (photo.Image, error) {
	var image photo.Image
	err := r.store.Change(ctx, func(tx *sql.Tx) error {
		var err error
		image, err = database.DecodePayload[photo.Image](store.New(tx).GetImage(ctx, id))
		if err != nil {
			return err
		}
		image.Name = input.Name
		image.Country = input.Country
		image.City = input.City
		image.YearTaken = input.YearTaken
		image.Color = input.Color
		raw, err := json.Marshal(image)
		if err != nil {
			return err
		}
		if err = store.New(tx).UpdateImage(ctx, store.UpdateImageParams{Color: image.Color, Payload: raw, ID: id}); err != nil {
			return err
		}
		return database.Enqueue(ctx, tx, imageKind, image)
	})
	return image, err
}

func (r *Repository) DeleteImage(ctx context.Context, id string) error {
	return r.store.Change(ctx, func(tx *sql.Tx) error {
		image, err := database.DecodePayload[photo.Image](store.New(tx).GetImage(ctx, id))
		if err != nil {
			return err
		}
		if err = store.New(tx).DeleteImage(ctx, id); err != nil {
			return err
		}
		return database.Enqueue(ctx, tx, deleteImageOperation, image)
	})
}

func (r *Repository) MoveImage(ctx context.Context, id, direction string) error {
	return r.store.Change(ctx, func(tx *sql.Tx) error {
		q := store.New(tx)
		rows, err := q.AllImages(ctx)
		if err != nil {
			return err
		}
		images := make([]photo.Image, 0, len(rows))
		for _, raw := range rows {
			image, err := database.DecodePayload[photo.Image](raw, nil)
			if err != nil {
				return err
			}
			images = append(images, image)
		}
		changed, err := photo.MoveImage(images, id, direction)
		if err != nil {
			return err
		}
		for _, image := range changed {
			raw, err := json.Marshal(image)
			if err != nil {
				return err
			}
			if err := q.UpdateImage(ctx, store.UpdateImageParams{Color: image.Color, Payload: raw, ID: image.UUID}); err != nil {
				return err
			}
			if err := database.Enqueue(ctx, tx, imageKind, image); err != nil {
				return err
			}
		}
		return nil
	})
}
