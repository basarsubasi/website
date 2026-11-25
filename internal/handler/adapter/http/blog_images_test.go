package httpadapter

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	imageField                      = "image"
	uploadFilename                  = "photo.png"
	blogImagesTestPostMethod        = "POST"
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
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile(imageField, uploadFilename)
		if err != nil {
			t.Fatal(err)
		}
		file.Write(raw)
		form.Close()
		req := httptest.NewRequest(blogImagesTestPostMethod, uploadPath, &body)
		req.Header.Set(blogImagesTestContentTypeHeader, form.FormDataContentType())
		return c.do(req, status)
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
	if _, _, err := c.objects.Read(context.Background(), key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf(failedDeletionFormat, err)
	}

}
