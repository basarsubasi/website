package assets

import (
	"context"
	"io"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	videoUploadOperation   = "upload video"
	videoDeleteOperation   = "delete blog video"
	invalidVideoFilename   = "Invalid blog video filename"
	objectKeyPrefix        = "images/"
	extensionSeparator     = "."
	uploadOperation        = "upload photo"
	invalidFilenameMessage = "Invalid blog image filename"
	deleteOperation        = "delete blog image"
)

func (u *UseCase) UploadBlogImage(ctx context.Context, file io.ReadSeeker, size int64) (article.BlogAsset, error) {
	return u.upload(ctx, file, size, objectKeyPrefix, uploadOperation, media.Inspect)
}

func (u *UseCase) UploadBlogVideo(ctx context.Context, file io.ReadSeeker, size int64) (article.BlogAsset, error) {
	return u.upload(ctx, file, size, article.VideoKeyPrefix, videoUploadOperation, media.InspectVideo)
}

func (u *UseCase) upload(ctx context.Context, file io.ReadSeeker, size int64, prefix, operation string, inspect func(io.ReadSeeker, int64) (media.FileInfo, error)) (article.BlogAsset, error) {
	if u.storage == nil {
		return article.BlogAsset{}, domain.ErrUnavailable
	}
	info, err := inspect(file, size)
	if err != nil {
		return article.BlogAsset{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	url, err := u.storage.Put(ctx, prefix+uuid.NewV4().String()+extensionSeparator+info.Extension, file, size, info.ContentType, nil)
	if err != nil {
		return article.BlogAsset{}, &domain.StorageError{Operation: operation, Err: err}
	}
	return article.BlogAsset{URL: url, Width: info.Width, Height: info.Height}, nil
}

func (u *UseCase) DeleteBlogImage(ctx context.Context, filename string) error {
	return u.delete(ctx, filename, objectKeyPrefix, article.ValidImageFilename, invalidFilenameMessage, deleteOperation)
}

func (u *UseCase) DeleteBlogVideo(ctx context.Context, filename string) error {
	return u.delete(ctx, filename, article.VideoKeyPrefix, article.ValidVideoFilename, invalidVideoFilename, videoDeleteOperation)
}

func (u *UseCase) delete(ctx context.Context, filename, prefix string, permitted func(string) bool, invalid, operation string) error {
	if !permitted(filename) {
		return domain.Invalid(invalid)
	}
	if u.storage == nil {
		return domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := u.storage.Delete(ctx, prefix+filename); err != nil {
		return &domain.StorageError{Operation: operation, Err: err}
	}
	return nil
}
