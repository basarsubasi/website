package httpadapter

import (
	"errors"
	"net/http"

	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	blogImagesUploadTooLargeMessage = "Photo must be at most 25 MiB"
	imageRequiredMessage            = "Image file is required"
	imageFormField                  = "image"
	singleImageRequiredMessage      = "Provide exactly one image file"
	blogImagesDataField             = "data"
	filenameParam                   = "filename"
)

func (s *Server) uploadBlogImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, media.MaxSize+(64<<10))
	err := r.ParseMultipartForm(2 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, blogImagesUploadTooLargeMessage)
		} else {
			fail(w, http.StatusBadRequest, imageRequiredMessage)
		}
		return
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File[imageFormField]) != 1 || len(r.MultipartForm.Value) != 0 {
		fail(w, http.StatusBadRequest, singleImageRequiredMessage)
		return
	}
	file, header, err := r.FormFile(imageFormField)
	if err != nil {
		fail(w, http.StatusBadRequest, imageRequiredMessage)
		return
	}
	defer file.Close()
	image, err := s.blog.Images.UploadBlogImage(r.Context(), file, header.Size)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusCreated, map[string]any{blogImagesDataField: image})
}

func (s *Server) deleteBlogImage(w http.ResponseWriter, r *http.Request) {
	if err := s.blog.Images.DeleteBlogImage(r.Context(), r.PathValue(filenameParam)); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}
