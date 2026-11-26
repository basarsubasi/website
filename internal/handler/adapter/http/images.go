package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	imagesUploadTooLargeMessage = "Photo must be at most 25 MiB"
	expectedMultipartMessage    = "Expected multipart form with photo and metadata fields"
	photoFormField              = "photo"
	metadataFormField           = "metadata"
	invalidPartCountMessage     = "Provide exactly one photo file and one metadata JSON field"
	invalidMetadataMessage      = "Invalid photo metadata JSON"
	singleMetadataObjectMessage = "Expected one metadata JSON object"
	photoRequiredMessage        = "Photo file is required"
	imagesDataField             = "data"
	uuidParam                   = "uuid"
	colorParam                  = "color"
)

func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, media.MaxSize+(64<<10))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, imagesUploadTooLargeMessage)
		} else {
			fail(w, http.StatusBadRequest, expectedMultipartMessage)
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File[photoFormField]) != 1 || len(r.MultipartForm.Value) != 1 || len(r.MultipartForm.Value[metadataFormField]) != 1 {
		fail(w, http.StatusBadRequest, invalidPartCountMessage)
		return
	}
	var p photo.PhotoInput
	d := json.NewDecoder(strings.NewReader(r.MultipartForm.Value[metadataFormField][0]))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		fail(w, http.StatusBadRequest, invalidMetadataMessage)
		return
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, http.StatusBadRequest, singleMetadataObjectMessage)
		return
	}
	file, header, err := r.FormFile(photoFormField)
	if err != nil {
		fail(w, http.StatusBadRequest, photoRequiredMessage)
		return
	}
	defer file.Close()
	photo, err := s.gallery.Photos.UploadImage(r.Context(), p, file, header.Size)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusCreated, map[string]any{imagesDataField: photo})
}
func (s *Server) saveImage(w http.ResponseWriter, r *http.Request) {
	var p photo.PhotoInput
	if !decode(w, r, &p) {
		return
	}
	photo, err := s.gallery.Photos.UpdateImage(r.Context(), r.PathValue(uuidParam), p)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{imagesDataField: photo})
}
func (s *Server) deleteImage(w http.ResponseWriter, r *http.Request) {
	if err := s.gallery.Photos.DeleteImage(r.Context(), r.PathValue(uuidParam)); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}

func (s *Server) listImages(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := parsePagination(w, r, 9)
	if !ok {
		return
	}
	data, err := s.gallery.Photos.ListImages(r.Context(), pagination.Pagination{Page: page, Limit: limit}, r.URL.Query().Get(colorParam))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, data)
}

func (s *Server) getImage(w http.ResponseWriter, r *http.Request) {
	data, err := s.gallery.Photos.GetImage(r.Context(), r.PathValue(uuidParam))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{imagesDataField: data})
}

func (s *Server) moveImage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Direction string `json:"direction"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := s.gallery.Photos.MoveImage(r.Context(), r.PathValue(uuidParam), input.Direction); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}
