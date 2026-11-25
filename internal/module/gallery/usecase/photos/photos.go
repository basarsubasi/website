package photos

import (
	"context"
	"errors"
	"io"
	"log"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	objectKeyPrefix         = "photos/"
	extensionSeparator      = "."
	uploadOperation         = "upload photo"
	cleanupFailureFormat    = "Object cleanup needed for %s: %v"
	moveUp                  = "up"
	moveDown                = "down"
	invalidDirectionMessage = "direction must be up or down"
)

func (u *UseCase) UploadImage(ctx context.Context, p photo.PhotoInput, file io.ReadSeeker, size int64) (photo.Image, error) {
	if u.photos == nil {
		return photo.Image{}, domain.ErrUnavailable
	}
	id := uuid.NewV4().String()
	metadata, err := photo.PhotoMetadata(p, id)
	if err != nil {
		return photo.Image{}, err
	}
	info, err := media.Inspect(file, size)
	if err != nil {
		return photo.Image{}, err
	}
	key := objectKeyPrefix + id + extensionSeparator + info.Extension
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	imageURL, err := u.photos.Put(ctx, key, file, size, info.ContentType, metadata)
	if err != nil {
		return photo.Image{}, &domain.StorageError{Operation: uploadOperation, Err: err}
	}
	photo, err := u.repo.CreateImage(ctx, photo.Image{Width: info.Width, Height: info.Height, UUID: id, ImageURL: imageURL, ObjectKey: key, Name: p.Name, Country: p.Country, City: p.City, YearTaken: p.YearTaken, FileType: info.ContentType, Color: p.Color})
	if err != nil && !errors.Is(err, domain.ErrUncertain) {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if cleanupErr := u.photos.Delete(cleanup, key); cleanupErr != nil {
			log.Printf(cleanupFailureFormat, key, cleanupErr)
		}
	}
	return photo, err
}

func (u *UseCase) UpdateImage(ctx context.Context, id string, p photo.PhotoInput) (photo.Image, error) {
	if _, err := photo.PhotoMetadata(p, id); err != nil {
		return photo.Image{}, err
	}
	return u.repo.UpdateImage(ctx, id, p)
}

func (u *UseCase) DeleteImage(ctx context.Context, id string) error {
	// The repository deletes local metadata and durably queues remote cleanup.
	return u.repo.DeleteImage(ctx, id)
}

func (u *UseCase) MoveImage(ctx context.Context, id, direction string) error {
	if direction != moveUp && direction != moveDown {
		return domain.Invalid(invalidDirectionMessage)
	}
	return u.repo.MoveImage(ctx, id, direction)
}
