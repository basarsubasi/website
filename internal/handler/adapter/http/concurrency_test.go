package httpadapter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const (
	handlerTimeoutMessage = "timed out waiting for handler"

	concurrencyTestGetMethod        = "GET"
	concurrencyTestRootPath         = "/"
	concurrencyTestRetryAfterHeader = "Retry-After"
	retryDelaySeconds               = "1"
	overloadResponseFormat          = "overload response: %d %v"
	capacityReleaseFormat           = "capacity not released: %d"

	panicPath            = "/panic"
	panicMessage         = "handler failed"
	missingPanicMessage  = "expected panic"
	leakedCapacityFormat = "panic leaked capacity: %d"

	concurrencyTestPostMethod        = "POST"
	uploadPhotoPath                  = "/api/admin/images"
	concurrencyTestContentTypeHeader = "Content-Type"
	multipartContentType             = "multipart/form-data; boundary=test"
	uploadOverloadFormat             = "upload overload: %d"
	rejectedBodyReadMessage          = "rejected upload body was read"
	listPostsPath                    = "/api/posts"
	authBypassFormat                 = "authentication bypassed: %d"
	concurrencyTestLoginPath         = "/api/auth/login"
	concurrencyTestJsonContentType   = "application/json"
	concurrencyTestHealthPath        = "/api/health"
)

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal(handlerTimeoutMessage)
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
		workers.Go(func() {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(concurrencyTestGetMethod, concurrencyTestRootPath, nil))
		})
	}
	for range max {
		awaitSignal(t, entered)
	}
	rejected := make(chan struct{})
	go func() {
		defer close(rejected)
		for range 100 {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(concurrencyTestGetMethod, concurrencyTestRootPath, nil))
			if res.Code != 503 || res.Header().Get(concurrencyTestRetryAfterHeader) != retryDelaySeconds {
				t.Errorf(overloadResponseFormat, res.Code, res.Header())
			}
		}
	}()
	awaitSignal(t, rejected)
	unblock()
	workers.Wait()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(concurrencyTestGetMethod, concurrencyTestRootPath, nil))
	if res.Code != 204 {
		t.Fatalf(capacityReleaseFormat, res.Code)
	}
}

func TestConcurrencyReleasesAfterPanic(t *testing.T) {
	handler := limitConcurrency(1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == panicPath {
			panic(panicMessage)
		}
		w.WriteHeader(204)
	}))
	func() {
		defer func() {
			if recover() == nil {
				t.Error(missingPanicMessage)
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(concurrencyTestGetMethod, panicPath, nil))
	}()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(concurrencyTestGetMethod, concurrencyTestRootPath, nil))
	if res.Code != 204 {
		t.Fatalf(leakedCapacityFormat, res.Code)
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
	c.configureHandler(Config{Password: testPassword, FrontendDir: c.dir, MaxRequests: 2, MaxUploads: 1})
	c.login()
	body := &blockedBody{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	unblock := func() { once.Do(func() { close(body.release) }) }
	defer unblock()
	req := httptest.NewRequest(concurrencyTestPostMethod, uploadPhotoPath, body)
	req.Header.Set(concurrencyTestContentTypeHeader, multipartContentType)
	req.AddCookie(c.cookie)
	done := make(chan struct{})
	go func() { defer close(done); c.handler.ServeHTTP(httptest.NewRecorder(), req) }()
	awaitSignal(t, body.entered)
	// The second upload never gets as far as reading its body.
	rejectedBody := &blockedBody{entered: make(chan struct{}), release: body.release}
	rejectionDone := make(chan struct{})
	go func() {
		defer close(rejectionDone)
		req := httptest.NewRequest(concurrencyTestPostMethod, uploadPhotoPath, rejectedBody)
		req.Header.Set(concurrencyTestContentTypeHeader, multipartContentType)
		req.AddCookie(c.cookie)
		res := httptest.NewRecorder()
		c.handler.ServeHTTP(res, req)
		if res.Code != 503 {
			t.Errorf(uploadOverloadFormat, res.Code)
		}
	}()
	awaitSignal(t, rejectionDone)
	select {
	case <-rejectedBody.entered:
		t.Fatal(rejectedBodyReadMessage)
	default:
	}
	c.request(concurrencyTestGetMethod, listPostsPath, nil, 200)
	// Authentication still happens before upload admission.
	res := httptest.NewRecorder()
	c.handler.ServeHTTP(res, httptest.NewRequest(concurrencyTestPostMethod, uploadPhotoPath, nil))
	if res.Code != 401 {
		t.Fatalf(authBypassFormat, res.Code)
	}
	// Occupy the remaining request slot with a login body.
	loginBody := &blockedBody{entered: make(chan struct{}), release: body.release}
	loginDone := make(chan struct{})
	go func() {
		defer close(loginDone)
		req := httptest.NewRequest(concurrencyTestPostMethod, concurrencyTestLoginPath, loginBody)
		req.Header.Set(concurrencyTestContentTypeHeader, concurrencyTestJsonContentType)
		c.handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	awaitSignal(t, loginBody.entered)
	c.request(concurrencyTestGetMethod, listPostsPath, nil, 503)
	c.request(concurrencyTestGetMethod, concurrencyTestHealthPath, nil, 503)
	unblock()
	awaitSignal(t, done)
	awaitSignal(t, loginDone)
	c.request(concurrencyTestGetMethod, concurrencyTestHealthPath, nil, 200)
	c.request(concurrencyTestPostMethod, uploadPhotoPath, nil, 400)
}
