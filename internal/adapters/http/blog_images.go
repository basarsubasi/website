package httpadapter

import (
	"errors"
	"github.com/basarsubasi/website/internal/usecase"
	"net/http"
)

func (s *Server) uploadBlogImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxPhotoSize+(64<<10))
	err := r.ParseMultipartForm(2 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, "Photo must be at most 25 MiB")
		} else {
			fail(w, http.StatusBadRequest, "Image file is required")
		}
		return
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 || len(r.MultipartForm.Value) != 0 {
		fail(w, http.StatusBadRequest, "Provide exactly one image file")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		fail(w, http.StatusBadRequest, "Image file is required")
		return
	}
	defer file.Close()
	image, err := s.service.UploadBlogImage(r.Context(), file, header.Size)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusCreated, map[string]any{"data": image})
}

func (s *Server) deleteBlogImage(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteBlogImage(r.Context(), r.PathValue("filename")); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}
