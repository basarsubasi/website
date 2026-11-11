package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxPhotoSize+(64<<10))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, 413, "Photo must be at most 25 MiB")
		} else {
			fail(w, 400, "Expected multipart form with photo and metadata fields")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["photo"]) != 1 || len(r.MultipartForm.Value) != 1 || len(r.MultipartForm.Value["metadata"]) != 1 {
		fail(w, 400, "Provide exactly one photo file and one metadata JSON field")
		return
	}
	var p domain.PhotoInput
	d := json.NewDecoder(strings.NewReader(r.MultipartForm.Value["metadata"][0]))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		fail(w, 400, "Invalid photo metadata JSON")
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "Expected one metadata JSON object")
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		fail(w, 400, "Photo file is required")
		return
	}
	defer file.Close()
	photo, err := s.service.UploadImage(r.Context(), p, file, header.Size)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, 201, map[string]any{"data": photo})
}
func (s *Server) saveImage(w http.ResponseWriter, r *http.Request) {
	var p domain.PhotoInput
	if !decode(w, r, &p) {
		return
	}
	photo, err := s.service.UpdateImage(r.Context(), r.PathValue("uuid"), p)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, 200, map[string]any{"data": photo})
}
func (s *Server) deleteImage(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteImage(r.Context(), r.PathValue("uuid")); err != nil {
		appError(w, err)
		return
	}
	reply(w, 204, nil)
}
