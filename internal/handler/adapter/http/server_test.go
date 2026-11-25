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

	"github.com/basarsubasi/website/internal/module/blog"
	blogsqlite "github.com/basarsubasi/website/internal/module/blog/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery"
	gallerysqlite "github.com/basarsubasi/website/internal/module/gallery/adapter/sqlite"
	galleryphoto "github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/database"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	serverTestIndexFilename = "index.html"
	frontendHTML            = "Vue application"
	databaseFilename        = "website.db"

	requestStatusFormat = "%s %s: got %d want %d: %s"

	serverTestContentTypeHeader = "Content-Type"
	serverTestJsonContentType   = "application/json"

	serverTestPostMethod  = "POST"
	serverTestLoginPath   = "/api/auth/login"
	passwordField         = "password"
	missingSessionFailure = "missing session"
	insecureCookieFailure = "unprotected cookie"

	nullListFailure = "null list"

	uuidV4Pattern     = `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	invalidUUIDFormat = "not UUIDv4: %s"

	metadataField     = "metadata"
	photoField        = "photo"
	untrustedFilename = "../../untrusted.jpg"
	adminImagesPath   = "/api/admin/images"

	originalPhotoName      = "Boğaz"
	originalCity           = "İstanbul"
	originalCountry        = "Türkiye"
	blackAndWhite          = "bw"
	pngContentType         = "image/png"
	trustedFilenameFailure = "trusted filename instead of file format"
	cityField              = "city"
	colorValue             = "color"
	missingMetadataFailure = "metadata lost"
	serverTestGetMethod    = "GET"
	filteredPhotosPath     = "/api/images?color=bw&limit=1"
	paginationFailure      = "filter/pagination failed"
	updatedPhotoName       = "Updated"
	putMethod              = "PUT"
	adminImagePathPrefix   = "/api/admin/images/"
	publicImagePathPrefix  = "/api/images/"
	restartDataFailure     = "data lost on reboot"
	serverTestDeleteMethod = "DELETE"
	retainedObjectFailure  = "object retained after delete"

	firstSlug                = "first"
	firstTitle               = "First"
	postBody                 = "Hello\n\nWorld"
	serverTestAdminPostsPath = "/api/admin/posts"
	firstPostPath            = "/api/posts/first"
	includeDraftsPath        = "/api/posts?include_drafts=1"
	exposedDraftFailure      = "draft exposed through public API"
	missingBodyFailure       = "admin missing post body"
	adminPostPathPrefix      = "/api/admin/posts/"
	lostBodyFailure          = "body lost after restart"
	logoutPath               = "/api/auth/logout"

	wrongPassword       = "wrong"
	invalidImageBytes   = "not an image"
	invalidValue        = "invalid"
	invalidColorPath    = "/api/images?color=invalid"
	invalidPagePath     = "/api/posts?page=-1"
	invalidSlug         = "Bad Slug"
	postTitle           = "Title"
	anotherPostBody     = "Body"
	originHeader        = "Origin"
	foreignOrigin       = "https://evil.example"
	serverTestRootPath  = "/"
	galleryPath         = "/photos"
	monochromePath      = "/bw-photos"
	colorPath           = "/color-photos"
	blogPostPath        = "/blog/post"
	photoPath           = "/photo/id"
	serverTestAdminPath = "/admin"
	missingAssetPath    = "/missing.js"
	missingAPIPath      = "/api/missing"

	localSlug               = "local"
	localTitle              = "Local"
	r2OfflineMessage        = "R2 offline"
	readOperation           = "read"
	serverTestHealthPath    = "/api/health"
	localPostPath           = "/api/posts/local"
	missingPostPath         = "/api/posts/missing"
	postsPath               = "/api/posts"
	galleryAPIPath          = "/api/images"
	offlineEditTitle        = "Offline edit"
	unexpectedR2ReadFailure = "public read unexpectedly touched R2"

	missingMovePath  = "/api/admin/images/missing/move"
	directionField   = "direction"
	moveUp           = "up"
	invalidDirection = "sideways"
)

var errR2Offline = errors.New(r2OfflineMessage)

const testPassword = "test-password-for-admin"

type client struct {
	t       *testing.T
	handler http.Handler
	objects *testutil.Objects
	cookie  *http.Cookie
	dir     string
	dbPath  string
	repo    *database.Store
	backup  *r2.Backup
}

func setup(t *testing.T) *client {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, serverTestIndexFilename), []byte(frontendHTML), 0600); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, objects: testutil.NewObjects(), dir: dir, dbPath: filepath.Join(t.TempDir(), databaseFilename)}
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
	c.handler = New(gallery.New(gallerysqlite.New(repo), c.objects), blog.New(blogsqlite.New(repo), c.objects), repo.Ping, Config{Password: testPassword, FrontendDir: c.dir})
}
func (c *client) do(req *http.Request, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	res := httptest.NewRecorder()
	c.handler.ServeHTTP(res, req)
	if res.Code != status {
		c.t.Fatalf(requestStatusFormat, req.Method, req.URL, res.Code, status, res.Body.String())
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
	req.Header.Set(serverTestContentTypeHeader, serverTestJsonContentType)
	return c.do(req, status)
}
func (c *client) login() {
	res := c.request(serverTestPostMethod, serverTestLoginPath, map[string]string{passwordField: testPassword}, 200)
	cookies := res.Result().Cookies()
	if len(cookies) == 0 {
		c.t.Fatal(missingSessionFailure)
	}
	c.cookie = cookies[0]
	assertV4(c.t, c.cookie.Value)
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		c.t.Fatal(insecureCookieFailure)
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
		t.Fatal(nullListFailure)
	}
	return body
}
func assertV4(t *testing.T, id string) {
	t.Helper()
	if !regexp.MustCompile(uuidV4Pattern).MatchString(id) {
		t.Fatalf(invalidUUIDFormat, id)
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
	writer.WriteField(metadataField, string(metadata))
	file, _ := writer.CreateFormFile(photoField, untrustedFilename)
	file.Write(raw)
	writer.Close()
	req := httptest.NewRequest(serverTestPostMethod, adminImagesPath, &body)
	req.Header.Set(serverTestContentTypeHeader, writer.FormDataContentType())
	return c.do(req, status)
}
func TestPhotoAPIAndReboot(t *testing.T) {
	c := setup(t)
	input := galleryphoto.PhotoInput{Name: originalPhotoName, City: originalCity, Country: originalCountry, YearTaken: 2025, Color: blackAndWhite}
	c.upload(input, pngBytes(t), 401)
	c.login()
	photo := item[galleryphoto.Image](t, c.upload(input, pngBytes(t), 201))
	assertV4(t, photo.UUID)
	if photo.FileType != pngContentType {
		t.Fatal(trustedFilenameFailure)
	}
	metadata, _ := base64.RawURLEncoding.DecodeString(c.objects.Metadata(photo.ObjectKey)[photoField])
	var fields map[string]any
	json.Unmarshal(metadata, &fields)
	if fields[cityField] != originalCity || fields[colorValue] != blackAndWhite {
		t.Fatal(missingMetadataFailure)
	}
	input.Color = colorValue
	c.upload(input, pngBytes(t), 201)
	result := page[galleryphoto.Image](t, c.request(serverTestGetMethod, filteredPhotosPath, nil, 200))
	if result.Total != 1 || len(result.Data) != 1 {
		t.Fatal(paginationFailure)
	}
	input.Name = updatedPhotoName
	c.request(putMethod, adminImagePathPrefix+photo.UUID, input, 200)
	c.restart()
	got := item[galleryphoto.Image](t, c.request(serverTestGetMethod, publicImagePathPrefix+photo.UUID, nil, 200))
	if got.Name != updatedPhotoName {
		t.Fatal(restartDataFailure)
	}
	c.request(serverTestDeleteMethod, adminImagePathPrefix+photo.UUID, nil, 401)
	c.login()
	c.request(serverTestDeleteMethod, adminImagePathPrefix+photo.UUID, nil, 204)
	c.request(serverTestGetMethod, publicImagePathPrefix+photo.UUID, nil, 404)
	if err := c.repo.SyncBackups(context.Background(), c.backup); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.objects.Read(context.Background(), photo.ObjectKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(retainedObjectFailure)
	}
}
func TestBlogAPIAndDraftVisibility(t *testing.T) {
	c := setup(t)
	c.login()
	input := article.PostInput{Slug: firstSlug, Title: firstTitle, Body: postBody, Published: 0}
	post := item[article.Post](t, c.request(serverTestPostMethod, serverTestAdminPostsPath, input, 201))
	assertV4(t, post.ID)
	c.request(serverTestGetMethod, firstPostPath, nil, 404)
	if page[article.Post](t, c.request(serverTestGetMethod, includeDraftsPath, nil, 200)).Total != 0 {
		t.Fatal(exposedDraftFailure)
	}
	admin := page[article.Post](t, c.request(serverTestGetMethod, serverTestAdminPostsPath, nil, 200))
	if admin.Data[0].Body != input.Body {
		t.Fatal(missingBodyFailure)
	}
	c.request(serverTestPostMethod, serverTestAdminPostsPath, input, 409)
	input.Published = 1
	c.request(putMethod, adminPostPathPrefix+post.ID, input, 200)
	c.restart()
	if item[article.Post](t, c.request(serverTestGetMethod, firstPostPath, nil, 200)).Body != input.Body {
		t.Fatal(lostBodyFailure)
	}
	c.login()
	c.request(serverTestDeleteMethod, adminPostPathPrefix+post.ID, nil, 204)
	c.request(serverTestGetMethod, firstPostPath, nil, 404)
	c.request(serverTestPostMethod, logoutPath, nil, 204)
	c.request(serverTestGetMethod, serverTestAdminPostsPath, nil, 401)
}
func TestValidationAndCrossOrigin(t *testing.T) {
	c := setup(t)
	c.request(serverTestPostMethod, serverTestLoginPath, map[string]string{passwordField: wrongPassword}, 401)
	c.login()
	c.upload(galleryphoto.PhotoInput{Color: colorValue}, []byte(invalidImageBytes), 400)
	c.upload(galleryphoto.PhotoInput{Color: invalidValue}, pngBytes(t), 400)
	c.request(serverTestGetMethod, invalidColorPath, nil, 400)
	c.request(serverTestGetMethod, invalidPagePath, nil, 400)
	c.request(serverTestPostMethod, serverTestAdminPostsPath, article.PostInput{Slug: invalidSlug, Title: postTitle, Body: anotherPostBody}, 400)
	req := httptest.NewRequest(serverTestPostMethod, logoutPath, nil)
	req.Header.Set(originHeader, foreignOrigin)
	c.do(req, 403)
	for _, path := range []string{serverTestRootPath, galleryPath, monochromePath, colorPath, blogPostPath, photoPath, serverTestAdminPath} {
		c.request(serverTestGetMethod, path, nil, 200)
	}
	c.request(serverTestGetMethod, missingAssetPath, nil, 404)
	c.request(serverTestGetMethod, missingAPIPath, nil, 404)
}

func TestPublicReadsUseSQLiteDuringR2Outage(t *testing.T) {
	c := setup(t)
	c.login()
	photo := item[galleryphoto.Image](t, c.upload(galleryphoto.PhotoInput{Color: blackAndWhite}, pngBytes(t), 201))
	c.request(serverTestPostMethod, serverTestAdminPostsPath, article.PostInput{Slug: localSlug, Title: localTitle, Body: anotherPostBody, Published: 1}, 201)
	c.objects.FailNext(readOperation, errR2Offline)
	c.objects.FailNext(metadataField, errR2Offline)
	for i := 0; i < 20; i++ {
		c.request(serverTestGetMethod, serverTestHealthPath, nil, 200)
		c.request(serverTestGetMethod, localPostPath, nil, 200)
		c.request(serverTestGetMethod, missingPostPath, nil, 404)
		c.request(serverTestGetMethod, postsPath, nil, 200)
		c.request(serverTestGetMethod, galleryAPIPath, nil, 200)
		c.request(serverTestGetMethod, publicImagePathPrefix+photo.UUID, nil, 200)
	}
	c.request(putMethod, adminImagePathPrefix+photo.UUID, galleryphoto.PhotoInput{Name: offlineEditTitle, Color: colorValue}, 200)
	if _, _, err := c.objects.Read(context.Background(), r2.CatalogKey); !errors.Is(err, errR2Offline) {
		t.Fatal(unexpectedR2ReadFailure)
	}
}

func TestImageMoveRequiresAdminAndValidDirection(t *testing.T) {
	c := setup(t)
	c.request(serverTestPostMethod, missingMovePath, map[string]string{directionField: moveUp}, http.StatusUnauthorized)
	c.login()
	c.request(serverTestPostMethod, missingMovePath, map[string]string{directionField: invalidDirection}, http.StatusBadRequest)
	c.request(serverTestPostMethod, missingMovePath, map[string]string{directionField: moveUp}, http.StatusNotFound)
}
