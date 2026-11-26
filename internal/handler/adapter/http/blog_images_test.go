package httpadapter

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/shared/domain/storage"
)

const (
	videoUploadPath                 = "/api/admin/blog/videos"
	videoClipFixture                = "../../../../tests/testdata/blog-clip-av1-opus.mp4"
	clipFilename                    = "clip.mp4"
	invalidDeleteVideo              = "/api/admin/blog/videos/catalog.json"
	audioUploadPath                 = "/api/admin/blog/audio"
	audioClipFixture                = "../../../../tests/testdata/blog-clip.opus"
	audioClipFilename               = "clip.opus"
	invalidDeleteAudio              = "/api/admin/blog/audio/catalog.json"
	mediaObjectMismatch             = "stored clip differs from uploaded bytes"
	imageField                      = "image"
	uploadFilename                  = "photo.png"
	uploadPath                      = "/api/admin/blog/images"
	blogImagesTestContentTypeHeader = "Content-Type"
	blogImagesTestDeleteMethod      = "DELETE"
	missingImagePath                = "/api/admin/blog/images/00000000-0000-0000-0000-000000000000.png"
	invalidFilenamePath             = "/api/admin/blog/images/catalog.json"
	invalidImageData                = "not an image"
	dimensionsMismatchFormat        = "unexpected image dimensions: %+v"
	publicImagesURL                 = "https://images.example.com/images/"
	publicURLFailureFormat          = "unexpected URL: %s"
	bucketBaseURL                   = "https://images.example.com/"
	missingUploadFormat             = "image not stored: %v"
	adminBlogPathPrefix             = "/api/admin/blog/"
	failedDeletionFormat            = "deleted image remains: %v"
)

func TestBlogImageUpload(t *testing.T) {
	c := setup(t)
	upload := func(raw []byte, status int) *httptest.ResponseRecorder {
		return c.uploadBlogFile(t, uploadPath, imageField, uploadFilename, raw, status)
	}
	c.request(blogImagesTestDeleteMethod, missingImagePath, nil, 401)
	upload(pngBytes(t), 401)
	c.login()
	c.request(blogImagesTestDeleteMethod, invalidFilenamePath, nil, 400)
	upload([]byte(invalidImageData), 400)
	result := item[struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}](t, upload(pngBytes(t), 201))
	if result.Width != 2 || result.Height != 2 {
		t.Fatalf(dimensionsMismatchFormat, result)
	}
	if !strings.HasPrefix(result.URL, publicImagesURL) {
		t.Fatalf(publicURLFailureFormat, result.URL)
	}
	raw, _, err := c.objects.Read(context.Background(), strings.TrimPrefix(result.URL, bucketBaseURL))
	if err != nil || !bytes.Equal(raw, pngBytes(t)) {
		t.Fatalf(missingUploadFormat, err)
	}
	key := strings.TrimPrefix(result.URL, bucketBaseURL)
	c.request(blogImagesTestDeleteMethod, adminBlogPathPrefix+key, nil, 204)
	if _, _, err := c.objects.Read(context.Background(), key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf(failedDeletionFormat, err)
	}

}

func TestBlogMediaUploadAndDelete(t *testing.T) {
	for _, tc := range []struct{ endpoint, field, fixture, filename, invalidDelete string }{
		{videoUploadPath, videoFormField, videoClipFixture, clipFilename, invalidDeleteVideo},
		{audioUploadPath, audioFormField, audioClipFixture, audioClipFilename, invalidDeleteAudio},
	} {
		t.Run(tc.field, func(t *testing.T) {
			raw, err := os.ReadFile(tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			c := setup(t)
			upload := func(raw []byte, status int) *httptest.ResponseRecorder {
				return c.uploadBlogFile(t, tc.endpoint, tc.field, tc.filename, raw, status)
			}
			upload(raw, http.StatusUnauthorized)
			c.request(http.MethodDelete, tc.invalidDelete, nil, http.StatusUnauthorized)
			c.login()
			c.request(http.MethodDelete, tc.invalidDelete, nil, http.StatusBadRequest)
			upload(pngBytes(t), http.StatusBadRequest)
			result := item[struct{ URL string }](t, upload(raw, http.StatusCreated))
			key := strings.TrimPrefix(result.URL, bucketBaseURL)
			got, _, err := c.objects.Read(t.Context(), key)
			if err != nil || !bytes.Equal(raw, got) {
				t.Fatal(mediaObjectMismatch, err)
			}
			c.request(http.MethodDelete, adminBlogPathPrefix+key, nil, http.StatusNoContent)
			if _, _, err := c.objects.Read(t.Context(), key); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf(failedDeletionFormat, err)
			}
		})
	}
}

func (c *client) uploadBlogFile(t *testing.T, endpoint, field, filename string, raw []byte, status int) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, endpoint, &body)
	req.Header.Set(blogImagesTestContentTypeHeader, form.FormDataContentType())
	return c.do(req, status)
}
