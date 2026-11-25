package images

import (
	"context"
	"io"
	"regexp"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	objectKeyPrefix        = "images/"
	extensionSeparator     = "."
	uploadOperation        = "upload photo"
	filenamePattern        = `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.(jpg|png|gif)$`
	invalidFilenameMessage = "Invalid blog image filename"
	deleteOperation        = "delete blog image"
)

func (u *UseCase) UploadBlogImage(ctx context.Context, file io.ReadSeeker, size int64) (article.BlogImage, error) {
	if u.blogImages == nil {
		return article.BlogImage{}, domain.ErrUnavailable
	}
	info, err := media.Inspect(file, size)
	if err != nil {
		return article.BlogImage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	url, err := u.blogImages.Put(ctx, objectKeyPrefix+uuid.NewV4().String()+extensionSeparator+info.Extension, file, size, info.ContentType, nil)
	if err != nil {
		return article.BlogImage{}, &domain.StorageError{Operation: uploadOperation, Err: err}
	}
	return article.BlogImage{URL: url, Width: info.Width, Height: info.Height}, nil
}

var blogImageFilename = regexp.MustCompile(filenamePattern)

func (u *UseCase) DeleteBlogImage(ctx context.Context, filename string) error {
	if !blogImageFilename.MatchString(filename) {
		return domain.Invalid(invalidFilenameMessage)
	}
	if u.blogImages == nil {
		return domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := u.blogImages.Delete(ctx, objectKeyPrefix+filename); err != nil {
		return &domain.StorageError{Operation: deleteOperation, Err: err}
	}
	return nil
}
