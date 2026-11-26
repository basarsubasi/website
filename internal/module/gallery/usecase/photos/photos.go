package photos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
	"github.com/basarsubasi/website/internal/shared/domain/validation"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	allColors               = ""
	blackAndWhite           = "bw"
	colorPhoto              = "color"
	invalidColorMessage     = "color must be bw or color"
	objectKeyPrefix         = "photos/"
	extensionSeparator      = "."
	uploadOperation         = "upload photo"
	cleanupFailureFormat    = "Object cleanup needed for %s: %v"
	moveUp                  = "up"
	moveDown                = "down"
	invalidDirectionMessage = "direction must be up or down"
)

func (u *UseCase) ListImages(ctx context.Context, p pagination.Pagination, color string) (pagination.Page[photo.Image], error) {
	if err := p.Validate(); err != nil {
		return pagination.Page[photo.Image]{}, err
	}
	if color != allColors && color != blackAndWhite && color != colorPhoto {
		return pagination.Page[photo.Image]{}, validation.Invalid(invalidColorMessage)
	}
	return u.repo.ListImages(ctx, p, color)
}

func (u *UseCase) GetImage(ctx context.Context, id string) (photo.Image, error) {
	return u.repo.GetImage(ctx, id)
}

func (u *UseCase) UploadImage(ctx context.Context, p photo.PhotoInput, file io.ReadSeeker, size int64) (photo.Image, error) {
	if u.photos == nil {
		return photo.Image{}, storage.ErrUnavailable
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
	preview, err := RenderPreview(ctx, file, info)
	if err != nil {
		return photo.Image{}, err
	}
	previewKey := photo.PreviewKey(id)
	previewURL, err := u.photos.Put(ctx, previewKey, bytes.NewReader(preview), int64(len(preview)), previewContentType, nil)
	if err != nil {
		u.cleanupUpload(ctx, previewKey)
		return photo.Image{}, &storage.Error{Operation: uploadOperation, Err: err}
	}
	imageURL, err := u.photos.Put(ctx, key, file, size, info.ContentType, metadata)
	if err != nil {
		u.cleanupUpload(ctx, key, previewKey)
		return photo.Image{}, &storage.Error{Operation: uploadOperation, Err: err}
	}
	photo, err := u.repo.CreateImage(ctx, photo.Image{Width: info.Width, Height: info.Height, UUID: id, ImageURL: imageURL, PreviewURL: previewURL, ObjectKey: key, Name: p.Name, Country: p.Country, City: p.City, YearTaken: p.YearTaken, FileType: info.ContentType, Color: p.Color})
	if err != nil && !errors.Is(err, storage.ErrUncertain) {
		u.cleanupUpload(ctx, key, previewKey)
	}
	return photo, err
}

func (u *UseCase) cleanupUpload(ctx context.Context, keys ...string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	for _, key := range keys {
		if err := u.photos.Delete(cleanup, key); err != nil {
			log.Printf(cleanupFailureFormat, key, err)
		}
	}
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
		return validation.Invalid(invalidDirectionMessage)
	}
	return u.repo.MoveImage(ctx, id, direction)
}
