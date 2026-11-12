package httpadapter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/basarsubasi/website/internal/usecase"
)

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for handler")
	}
}

func TestConcurrencyRejectsWithoutQueuingAndRecovers(t *testing.T) {
	const max = 4
	entered := make(chan struct{}, max)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	handler := limitConcurrency(max, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	var workers sync.WaitGroup
	for range max {
		workers.Go(func() { handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil)) })
	}
	for range max {
		awaitSignal(t, entered)
	}
	rejected := make(chan struct{})
	go func() {
		defer close(rejected)
		for range 100 {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil))
			if res.Code != 503 || res.Header().Get("Retry-After") != "1" {
				t.Errorf("overload response: %d %v", res.Code, res.Header())
			}
		}
	}()
	awaitSignal(t, rejected)
	unblock()
	workers.Wait()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil))
	if res.Code != 204 {
		t.Fatalf("capacity not released: %d", res.Code)
	}
}

func TestConcurrencyReleasesAfterPanic(t *testing.T) {
	handler := limitConcurrency(1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			panic("handler failed")
		}
		w.WriteHeader(204)
	}))
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/panic", nil))
	}()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest("GET", "/", nil))
	if res.Code != 204 {
		t.Fatalf("panic leaked capacity: %d", res.Code)
	}
}

type blockedBody struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockedBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return 0, io.EOF
}
func (b *blockedBody) Close() error { return nil }

func TestUploadLimitBeforeBodyReadAndGeneralRequestLimit(t *testing.T) {
	c := setup(t)
	c.handler = New(usecase.New(c.repo, c.objects, c.objects), Config{Password: testPassword, FrontendDir: c.dir, MaxRequests: 2, MaxUploads: 1})
	c.login()
	body := &blockedBody{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	unblock := func() { once.Do(func() { close(body.release) }) }
	defer unblock()
	req := httptest.NewRequest("POST", "/api/admin/images", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	req.AddCookie(c.cookie)
	done := make(chan struct{})
	go func() { defer close(done); c.handler.ServeHTTP(httptest.NewRecorder(), req) }()
	awaitSignal(t, body.entered)
	// The second upload never gets as far as reading its body.
	rejectedBody := &blockedBody{entered: make(chan struct{}), release: body.release}
	rejectionDone := make(chan struct{})
	go func() {
		defer close(rejectionDone)
		req := httptest.NewRequest("POST", "/api/admin/images", rejectedBody)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		req.AddCookie(c.cookie)
		res := httptest.NewRecorder()
		c.handler.ServeHTTP(res, req)
		if res.Code != 503 {
			t.Errorf("upload overload: %d", res.Code)
		}
	}()
	awaitSignal(t, rejectionDone)
	select {
	case <-rejectedBody.entered:
		t.Fatal("rejected upload body was read")
	default:
	}
	c.request("GET", "/api/posts", nil, 200)
	// Authentication still happens before upload admission.
	res := httptest.NewRecorder()
	c.handler.ServeHTTP(res, httptest.NewRequest("POST", "/api/admin/images", nil))
	if res.Code != 401 {
		t.Fatalf("authentication bypassed: %d", res.Code)
	}
	// Occupy the remaining request slot with a login body.
	loginBody := &blockedBody{entered: make(chan struct{}), release: body.release}
	loginDone := make(chan struct{})
	go func() {
		defer close(loginDone)
		req := httptest.NewRequest("POST", "/api/auth/login", loginBody)
		req.Header.Set("Content-Type", "application/json")
		c.handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	awaitSignal(t, loginBody.entered)
	c.request("GET", "/api/posts", nil, 503)
	c.request("GET", "/api/health", nil, 503)
	unblock()
	awaitSignal(t, done)
	awaitSignal(t, loginDone)
	c.request("GET", "/api/health", nil, 200)
	c.request("POST", "/api/admin/images", nil, 400)
}
