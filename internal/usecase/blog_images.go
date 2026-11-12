package usecase

import (
	"context"
	"image"
	"io"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
)

func (s *Service) UploadBlogImage(ctx context.Context, file io.ReadSeeker, size int64) (domain.BlogImage, error) {
	if s.blogImages == nil {
		return domain.BlogImage{}, domain.ErrUnavailable
	}
	if size < 1 || size > MaxPhotoSize {
		return domain.BlogImage{}, domain.ErrTooLarge
	}
	dimensions, format, err := image.DecodeConfig(file)
	if err != nil {
		return domain.BlogImage{}, domain.Invalid("Upload a valid JPEG, PNG, or GIF image")
	}
	contentType, ok := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if !ok {
		return domain.BlogImage{}, domain.Invalid("Upload a JPEG, PNG, or GIF image")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return domain.BlogImage{}, err
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	url, err := s.blogImages.Put(ctx, "images/"+uuid.NewV4().String()+"."+ext, file, size, contentType, nil)
	if err != nil {
		return domain.BlogImage{}, &domain.StorageError{Operation: "upload photo", Err: err}
	}
	return domain.BlogImage{URL: url, Width: dimensions.Width, Height: dimensions.Height}, nil
}
