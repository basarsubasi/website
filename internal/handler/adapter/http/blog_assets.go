package httpadapter

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	blogImagesUploadTooLargeMessage = "Photo must be at most 25 MiB"
	imageRequiredMessage            = "Image file is required"
	imageFormField                  = "image"
	audioFormField                  = "audio"
	audioRequiredMessage            = "Audio file is required"
	singleAudioRequiredMessage      = "Provide exactly one audio file"
	videoFormField                  = "video"
	videoRequiredMessage            = "Video file is required"
	singleVideoRequiredMessage      = "Provide exactly one video file"
	singleImageRequiredMessage      = "Provide exactly one image file"
	blogAssetDataField              = "data"
	filenameParam                   = "filename"
)

func (s *Server) uploadBlogImage(w http.ResponseWriter, r *http.Request) {
	s.uploadBlogAsset(w, r, imageFormField, imageRequiredMessage, singleImageRequiredMessage, blogImagesUploadTooLargeMessage, media.MaxSize, s.blog.Assets.UploadBlogImage)
}

func (s *Server) uploadBlogVideo(w http.ResponseWriter, r *http.Request) {
	s.uploadBlogAsset(w, r, videoFormField, videoRequiredMessage, singleVideoRequiredMessage, domain.ErrVideoTooLarge.Error(), media.MaxVideoSize, s.blog.Assets.UploadBlogVideo)
}

func (s *Server) uploadBlogAudio(w http.ResponseWriter, r *http.Request) {
	s.uploadBlogAsset(w, r, audioFormField, audioRequiredMessage, singleAudioRequiredMessage, domain.ErrAudioTooLarge.Error(), media.MaxSize, s.blog.Assets.UploadBlogAudio)
}

func (s *Server) uploadBlogAsset(w http.ResponseWriter, r *http.Request, field, required, single, tooLargeMessage string, maxFileSize int64, upload func(context.Context, io.ReadSeeker, int64) (article.BlogAsset, error)) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize+(64<<10))
	err := r.ParseMultipartForm(2 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, tooLargeMessage)
		} else {
			fail(w, http.StatusBadRequest, required)
		}
		return
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File[field]) != 1 || len(r.MultipartForm.Value) != 0 {
		fail(w, http.StatusBadRequest, single)
		return
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		fail(w, http.StatusBadRequest, required)
		return
	}
	defer file.Close()
	asset, err := upload(r.Context(), file, header.Size)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusCreated, map[string]any{blogAssetDataField: asset})
}

func (s *Server) deleteBlogImage(w http.ResponseWriter, r *http.Request) {
	if err := s.blog.Assets.DeleteBlogImage(r.Context(), r.PathValue(filenameParam)); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}

func (s *Server) deleteBlogAudio(w http.ResponseWriter, r *http.Request) {
	if err := s.blog.Assets.DeleteBlogAudio(r.Context(), r.PathValue(filenameParam)); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}

func (s *Server) deleteBlogVideo(w http.ResponseWriter, r *http.Request) {
	if err := s.blog.Assets.DeleteBlogVideo(r.Context(), r.PathValue(filenameParam)); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}
