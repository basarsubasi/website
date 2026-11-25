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

type blogUploadForm struct {
	field             string
	maxFileSize       int64
	requiredMessage   string
	singleFileMessage string
	tooLargeMessage   string
}

func (s *Server) uploadBlogImage(w http.ResponseWriter, r *http.Request) {
	uploadBlogAsset(w, r, blogUploadForm{
		field: imageFormField, requiredMessage: imageRequiredMessage, singleFileMessage: singleImageRequiredMessage,
		maxFileSize: media.MaxSize, tooLargeMessage: blogImagesUploadTooLargeMessage,
	}, s.blog.Assets.UploadBlogImage)
}

func (s *Server) uploadBlogVideo(w http.ResponseWriter, r *http.Request) {
	uploadBlogAsset(w, r, blogUploadForm{
		field: videoFormField, requiredMessage: videoRequiredMessage, singleFileMessage: singleVideoRequiredMessage,
		maxFileSize: media.MaxVideoSize, tooLargeMessage: domain.ErrVideoTooLarge.Error(),
	}, s.blog.Assets.UploadBlogVideo)
}

func (s *Server) uploadBlogAudio(w http.ResponseWriter, r *http.Request) {
	uploadBlogAsset(w, r, blogUploadForm{
		field: audioFormField, requiredMessage: audioRequiredMessage, singleFileMessage: singleAudioRequiredMessage,
		maxFileSize: media.MaxSize, tooLargeMessage: domain.ErrAudioTooLarge.Error(),
	}, s.blog.Assets.UploadBlogAudio)
}

func uploadBlogAsset(w http.ResponseWriter, r *http.Request, form blogUploadForm, upload func(context.Context, io.ReadSeeker, int64) (article.BlogAsset, error)) {
	r.Body = http.MaxBytesReader(w, r.Body, form.maxFileSize+(64<<10))
	err := r.ParseMultipartForm(2 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, form.tooLargeMessage)
		} else {
			fail(w, http.StatusBadRequest, form.requiredMessage)
		}
		return
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File[form.field]) != 1 || len(r.MultipartForm.Value) != 0 {
		fail(w, http.StatusBadRequest, form.singleFileMessage)
		return
	}
	file, header, err := r.FormFile(form.field)
	if err != nil {
		fail(w, http.StatusBadRequest, form.requiredMessage)
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
