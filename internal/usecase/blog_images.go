package usecase

import (
	"context"
	"image"
	"io"
	"regexp"
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

var blogImageFilename = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.(jpg|png|gif)$`)

func (s *Service) DeleteBlogImage(ctx context.Context, filename string) error {
	if !blogImageFilename.MatchString(filename) {
		return domain.Invalid("Invalid blog image filename")
	}
	if s.blogImages == nil {
		return domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.blogImages.Delete(ctx, "images/"+filename); err != nil {
		return &domain.StorageError{Operation: "delete blog image", Err: err}
	}
	return nil
}
