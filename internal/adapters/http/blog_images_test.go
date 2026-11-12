package httpadapter

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBlogImageUpload(t *testing.T) {
	c := setup(t)
	upload := func(raw []byte, status int) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("image", "photo.png")
		if err != nil {
			t.Fatal(err)
		}
		file.Write(raw)
		form.Close()
		req := httptest.NewRequest("POST", "/api/admin/blog/images", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		return c.do(req, status)
	}
	upload(pngBytes(t), 401)
	c.login()
	upload([]byte("not an image"), 400)
	result := item[struct {
		URL string `json:"url"`
	}](t, upload(pngBytes(t), 201))
	if !strings.HasPrefix(result.URL, "https://images.example.com/images/") {
		t.Fatalf("unexpected URL: %s", result.URL)
	}
	raw, _, err := c.objects.Read(context.Background(), strings.TrimPrefix(result.URL, "https://images.example.com/"))
	if err != nil || !bytes.Equal(raw, pngBytes(t)) {
		t.Fatalf("image not stored: %v", err)
	}
}
