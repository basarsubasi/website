package httpadapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/basarsubasi/website/internal/adapters/database"
	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
	"github.com/basarsubasi/website/internal/usecase"
)

const testPassword = "test-password-for-admin"

type client struct {
	t       *testing.T
	handler http.Handler
	objects *testutil.Objects
	cookie  *http.Cookie
	dir     string
	dbPath  string
	repo    *database.Repository
	backup  *r2.Backup
}

func setup(t *testing.T) *client {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("Vue application"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, objects: testutil.NewObjects(), dir: dir, dbPath: filepath.Join(t.TempDir(), "website.db")}
	c.restart()
	t.Cleanup(func() { c.repo.Close() })
	return c
}
func (c *client) restart() {
	c.t.Helper()
	if c.repo != nil {
		c.repo.Close()
	}
	repo, err := database.Open(context.Background(), c.dbPath)
	if err != nil {
		c.t.Fatal(err)
	}
	c.repo = repo
	c.backup = r2.NewBackup(c.objects)
	if err := repo.Initialize(context.Background(), c.backup.LoadSnapshot); err != nil {
		c.t.Fatal(err)
	}
	c.handler = New(usecase.New(repo, c.objects), Config{Password: testPassword, FrontendDir: c.dir})
}
func (c *client) do(req *http.Request, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	res := httptest.NewRecorder()
	c.handler.ServeHTTP(res, req)
	if res.Code != status {
		c.t.Fatalf("%s %s: got %d want %d: %s", req.Method, req.URL, res.Code, status, res.Body.String())
	}
	return res
}
func (c *client) request(method, path string, body any, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, status)
}
func (c *client) login() {
	res := c.request("POST", "/api/auth/login", map[string]string{"password": testPassword}, 200)
	cookies := res.Result().Cookies()
	if len(cookies) == 0 {
		c.t.Fatal("missing session")
	}
	c.cookie = cookies[0]
	assertV4(c.t, c.cookie.Value)
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		c.t.Fatal("unprotected cookie")
	}
}
func item[T any](t *testing.T, res *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}
func page[T any](t *testing.T, res *httptest.ResponseRecorder) domain.Page[T] {
	t.Helper()
	var body domain.Page[T]
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data == nil {
		t.Fatal("null list")
	}
	return body
}
func assertV4(t *testing.T, id string) {
	t.Helper()
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("not UUIDv4: %s", id)
	}
}
func pngBytes(t *testing.T) []byte {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}
func (c *client) upload(input any, raw []byte, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadata, _ := json.Marshal(input)
	writer.WriteField("metadata", string(metadata))
	file, _ := writer.CreateFormFile("photo", "../../untrusted.jpg")
	file.Write(raw)
	writer.Close()
	req := httptest.NewRequest("POST", "/api/admin/images", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return c.do(req, status)
}
func TestPhotoAPIAndReboot(t *testing.T) {
	c := setup(t)
	input := domain.PhotoInput{Name: "Boğaz", City: "İstanbul", Country: "Türkiye", YearTaken: 2025, Color: "bw"}
	c.upload(input, pngBytes(t), 401)
	c.login()
	photo := item[domain.Image](t, c.upload(input, pngBytes(t), 201))
	assertV4(t, photo.UUID)
	if photo.FileType != "image/png" {
		t.Fatal("trusted filename instead of file format")
	}
	metadata, _ := base64.RawURLEncoding.DecodeString(c.objects.Metadata(photo.ObjectKey)["photo"])
	var fields map[string]any
	json.Unmarshal(metadata, &fields)
	if fields["city"] != "İstanbul" || fields["color"] != "bw" {
		t.Fatal("metadata lost")
	}
	input.Color = "color"
	c.upload(input, pngBytes(t), 201)
	result := page[domain.Image](t, c.request("GET", "/api/images?color=bw&limit=1", nil, 200))
	if result.Total != 1 || len(result.Data) != 1 {
		t.Fatal("filter/pagination failed")
	}
	input.Name = "Updated"
	c.request("PUT", "/api/admin/images/"+photo.UUID, input, 200)
	c.restart()
	got := item[domain.Image](t, c.request("GET", "/api/images/"+photo.UUID, nil, 200))
	if got.Name != "Updated" {
		t.Fatal("data lost on reboot")
	}
	c.request("DELETE", "/api/admin/images/"+photo.UUID, nil, 401)
	c.login()
	c.request("DELETE", "/api/admin/images/"+photo.UUID, nil, 204)
	c.request("GET", "/api/images/"+photo.UUID, nil, 404)
	if err := c.repo.SyncBackups(context.Background(), c.backup); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.objects.Read(context.Background(), photo.ObjectKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("object retained after delete")
	}
}
func TestBlogAPIAndDraftVisibility(t *testing.T) {
	c := setup(t)
	c.login()
	input := domain.PostInput{Slug: "first", Title: "First", Body: "Hello\n\nWorld", Published: 0}
	post := item[domain.Post](t, c.request("POST", "/api/admin/posts", input, 201))
	assertV4(t, post.ID)
	c.request("GET", "/api/posts/first", nil, 404)
	if page[domain.Post](t, c.request("GET", "/api/posts?include_drafts=1", nil, 200)).Total != 0 {
		t.Fatal("draft exposed through public API")
	}
	admin := page[domain.Post](t, c.request("GET", "/api/admin/posts", nil, 200))
	if admin.Data[0].Body != input.Body {
		t.Fatal("admin missing post body")
	}
	c.request("POST", "/api/admin/posts", input, 409)
	input.Published = 1
	c.request("PUT", "/api/admin/posts/"+post.ID, input, 200)
	c.restart()
	if item[domain.Post](t, c.request("GET", "/api/posts/first", nil, 200)).Body != input.Body {
		t.Fatal("body lost after restart")
	}
	c.login()
	c.request("DELETE", "/api/admin/posts/"+post.ID, nil, 204)
	c.request("GET", "/api/posts/first", nil, 404)
	c.request("POST", "/api/auth/logout", nil, 204)
	c.request("GET", "/api/admin/posts", nil, 401)
}
func TestValidationAndCrossOrigin(t *testing.T) {
	c := setup(t)
	c.request("POST", "/api/auth/login", map[string]string{"password": "wrong"}, 401)
	c.login()
	c.upload(domain.PhotoInput{Color: "color"}, []byte("not an image"), 400)
	c.upload(domain.PhotoInput{Color: "invalid"}, pngBytes(t), 400)
	c.request("GET", "/api/images?color=invalid", nil, 400)
	c.request("GET", "/api/posts?page=-1", nil, 400)
	c.request("POST", "/api/admin/posts", domain.PostInput{Slug: "Bad Slug", Title: "Title", Body: "Body"}, 400)
	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	c.do(req, 403)
	for _, path := range []string{"/", "/gallery", "/bw-photos", "/color-photos", "/blog/post", "/image/id", "/admin"} {
		c.request("GET", path, nil, 200)
	}
	c.request("GET", "/missing.js", nil, 404)
	c.request("GET", "/api/missing", nil, 404)
}

func TestPublicReadsUseSQLiteDuringR2Outage(t *testing.T) {
	c := setup(t)
	c.login()
	photo := item[domain.Image](t, c.upload(domain.PhotoInput{Color: "bw"}, pngBytes(t), 201))
	c.request("POST", "/api/admin/posts", domain.PostInput{Slug: "local", Title: "Local", Body: "Body", Published: 1}, 201)
	offline := errors.New("R2 offline")
	c.objects.FailNext("read", offline)
	c.objects.FailNext("metadata", offline)
	for i := 0; i < 20; i++ {
		c.request("GET", "/api/health", nil, 200)
		c.request("GET", "/api/posts/local", nil, 200)
		c.request("GET", "/api/posts/missing", nil, 404)
		c.request("GET", "/api/posts", nil, 200)
		c.request("GET", "/api/images", nil, 200)
		c.request("GET", "/api/images/"+photo.UUID, nil, 200)
	}
	c.request("PUT", "/api/admin/images/"+photo.UUID, domain.PhotoInput{Name: "Offline edit", Color: "color"}, 200)
	if _, _, err := c.objects.Read(context.Background(), r2.CatalogKey); !errors.Is(err, offline) {
		t.Fatal("public read unexpectedly touched R2")
	}
}
