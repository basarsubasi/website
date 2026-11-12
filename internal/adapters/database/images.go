package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
	"github.com/basarsubasi/website/internal/domain"
)

func (r *Repository) GetImage(ctx context.Context, id string) (domain.Image, error) {
	return decodePayload[domain.Image](dbquery.New(r.db).GetImage(ctx, id))
}

func (r *Repository) ListImages(ctx context.Context, pagination domain.Pagination, color string) (domain.Page[domain.Image], error) {
	return listPage[domain.Image](ctx, r.db, pagination, func(q *dbquery.Queries) (int64, error) {
		return q.CountImages(ctx, color)
	}, func(q *dbquery.Queries) ([][]byte, error) {
		return q.ListImages(ctx, dbquery.ListImagesParams{Color: color, PageLimit: int64(pagination.Limit), PageOffset: int64((pagination.Page - 1) * pagination.Limit)})
	})
}

func (r *Repository) CreateImage(ctx context.Context, image domain.Image) (domain.Image, error) {
	image.CreatedAt = now()
	err := r.change(ctx, func(tx *sql.Tx) error {
		if err := insert(ctx, tx, "image", image.UUID, "", image.Color, 0, image.CreatedAt, image); err != nil {
			return err
		}
		return enqueue(ctx, tx, "image", image)
	})
	return image, err
}

func (r *Repository) UpdateImage(ctx context.Context, id string, input domain.PhotoInput) (domain.Image, error) {
	var image domain.Image
	err := r.change(ctx, func(tx *sql.Tx) error {
		var err error
		image, err = decodePayload[domain.Image](dbquery.New(tx).GetImage(ctx, id))
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
		if err = dbquery.New(tx).UpdateImage(ctx, dbquery.UpdateImageParams{Color: image.Color, Payload: raw, ID: id}); err != nil {
			return err
		}
		return enqueue(ctx, tx, "image", image)
	})
	return image, err
}

func (r *Repository) DeleteImage(ctx context.Context, id string) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		image, err := decodePayload[domain.Image](dbquery.New(tx).GetImage(ctx, id))
		if err != nil {
			return err
		}
		if err = dbquery.New(tx).DeleteImage(ctx, id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "delete-image", image)
	})
}
