package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/domain"
)

func (r *Repository) GetImage(ctx context.Context, id string) (domain.Image, error) {
	return scan[domain.Image](r.db.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='image' AND id=?", id))
}

func (r *Repository) ListImages(ctx context.Context, pagination domain.Pagination, color string) (domain.Page[domain.Image], error) {
	where := "kind='image'"
	args := []any{}
	if color != "" {
		where += " AND color=?"
		args = append(args, color)
	}
	return list[domain.Image](ctx, r.db, pagination, where, args...)
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
		image, err = scan[domain.Image](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='image' AND id=?", id))
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
		if _, err = tx.ExecContext(ctx, "UPDATE content SET color=?,payload=? WHERE kind='image' AND id=?", image.Color, raw, id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "image", image)
	})
	return image, err
}

func (r *Repository) DeleteImage(ctx context.Context, id string) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		image, err := scan[domain.Image](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='image' AND id=?", id))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM content WHERE kind='image' AND id=?", id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "delete-image", image)
	})
}
