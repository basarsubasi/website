package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
)

const MaxPhotoSize = 25 << 20

func PhotoMetadata(p domain.PhotoInput, id string) (map[string]string, error) {
	if p.Color != "bw" && p.Color != "color" {
		return nil, domain.Invalid("color must be bw or color")
	}
	if p.YearTaken < 0 || p.YearTaken > 9999 {
		return nil, domain.Invalid("year_taken must be between 0 and 9999")
	}
	raw, _ := json.Marshal(struct {
		UUID string `json:"uuid"`
		domain.PhotoInput
	}{id, p})
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > 1900 {
		return nil, domain.Invalid("Photo metadata is too long (maximum 1900 encoded bytes)")
	}
	return map[string]string{"photo": encoded}, nil
}
func validPage(p domain.Pagination) error {
	if p.Page < 1 || p.Page > 1000000 || p.Limit < 1 || p.Limit > 100 {
		return domain.Invalid("Invalid pagination")
	}
	return nil
}
func (s *Service) ListImages(ctx context.Context, p domain.Pagination, color string) (domain.Page[domain.Image], error) {
	if err := validPage(p); err != nil {
		return domain.Page[domain.Image]{}, err
	}
	if color != "" && color != "bw" && color != "color" {
		return domain.Page[domain.Image]{}, domain.Invalid("color must be bw or color")
	}
	return s.repo.ListImages(ctx, p, color)
}
func (s *Service) GetImage(ctx context.Context, id string) (domain.Image, error) {
	return s.repo.GetImage(ctx, id)
}
func (s *Service) UploadImage(ctx context.Context, p domain.PhotoInput, file io.ReadSeeker, size int64) (domain.Image, error) {
	if s.photos == nil {
		return domain.Image{}, domain.ErrUnavailable
	}
	id := uuid.NewV4().String()
	metadata, err := PhotoMetadata(p, id)
	if err != nil {
		return domain.Image{}, err
	}
	if size < 1 || size > MaxPhotoSize {
		return domain.Image{}, domain.ErrTooLarge
	}
	_, format, err := image.DecodeConfig(file)
	if err != nil {
		return domain.Image{}, domain.Invalid("Upload a valid JPEG, PNG, or GIF image")
	}
	contentType, ok := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if !ok {
		return domain.Image{}, domain.Invalid("Upload a JPEG, PNG, or GIF image")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return domain.Image{}, err
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	key := "photos/" + id + "." + ext
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	imageURL, err := s.photos.Put(ctx, key, file, size, contentType, metadata)
	if err != nil {
		return domain.Image{}, &domain.StorageError{Operation: "upload photo", Err: err}
	}
	photo, err := s.repo.CreateImage(ctx, domain.Image{Uuid: id, ImageUrl: imageURL, ObjectKey: key, Name: p.Name, Country: p.Country, City: p.City, YearTaken: p.YearTaken, FileType: contentType, Color: p.Color})
	if err != nil && !errors.Is(err, domain.ErrUncertain) {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := s.photos.Delete(cleanup, key); cleanupErr != nil {
			log.Printf("Object cleanup needed for %s: %v", key, cleanupErr)
		}
	}
	return photo, err
}
func (s *Service) UpdateImage(ctx context.Context, id string, p domain.PhotoInput) (domain.Image, error) {
	if _, err := PhotoMetadata(p, id); err != nil {
		return domain.Image{}, err
	}
	return s.repo.UpdateImage(ctx, id, p)
}
func (s *Service) DeleteImage(ctx context.Context, id string) error {
	// The repository deletes local metadata and durably queues remote cleanup.
	return s.repo.DeleteImage(ctx, id)
}
