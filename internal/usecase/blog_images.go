package usecase

import (
	"context"
	"image"
	"io"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
)

func (s *Service) UploadBlogImage(ctx context.Context, file io.ReadSeeker, size int64) (string, error) {
	if s.photos == nil {
		return "", domain.ErrUnavailable
	}
	if size < 1 || size > MaxPhotoSize {
		return "", domain.ErrTooLarge
	}
	_, format, err := image.DecodeConfig(file)
	if err != nil {
		return "", domain.Invalid("Upload a valid JPEG, PNG, or GIF image")
	}
	contentType, ok := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if !ok {
		return "", domain.Invalid("Upload a JPEG, PNG, or GIF image")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	url, err := s.photos.Put(ctx, "blog/images/"+uuid.NewV4().String()+"."+ext, file, size, contentType, nil)
	if err != nil {
		return "", &domain.StorageError{Operation: "upload photo", Err: err}
	}
	return url, nil
}
