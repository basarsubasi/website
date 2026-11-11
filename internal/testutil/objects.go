// Package testutil provides an in-memory object service for adapter tests.
package testutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"github.com/basarsubasi/website/internal/domain"
)

type object struct {
	body              []byte
	etag, contentType string
	metadata          map[string]string
}
type Objects struct {
	mu        sync.Mutex
	items     map[string]object
	counter   int
	faults    map[string]error
	ambiguous bool
}

func NewObjects() *Objects { return &Objects{items: map[string]object{}, faults: map[string]error{}} }
func (m *Objects) FailNext(op string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.faults[op] = err
}
func (m *Objects) AmbiguousNextCommit()  { m.mu.Lock(); defer m.mu.Unlock(); m.ambiguous = true }
func (m *Objects) fault(op string) error { err := m.faults[op]; delete(m.faults, op); return err }
func (m *Objects) Read(ctx context.Context, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if err := m.fault("read"); err != nil {
		return nil, "", err
	}
	item, ok := m.items[key]
	if !ok {
		return nil, "", domain.ErrNotFound
	}
	return append([]byte{}, item.body...), item.etag, nil
}
func (m *Objects) save(key string, body []byte, contentType string, metadata map[string]string) string {
	m.counter++
	etag := fmt.Sprintf(`"%d"`, m.counter)
	m.items[key] = object{body: append([]byte{}, body...), etag: etag, contentType: contentType, metadata: maps.Clone(metadata)}
	return etag
}
func (m *Objects) PutIf(ctx context.Context, key string, data []byte, etag string, create bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := m.fault("write"); err != nil {
		return "", err
	}
	current, exists := m.items[key]
	if (create && exists) || (!create && (!exists || current.etag != etag)) {
		return "", domain.ErrConflict
	}
	newETag := m.save(key, data, "application/json", nil)
	if m.ambiguous {
		m.ambiguous = false
		return "", fmt.Errorf("response lost after commit")
	}
	return newETag, nil
}
func (m *Objects) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string, metadata map[string]string) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	if int64(len(data)) != size {
		return "", fmt.Errorf("incorrect content length")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := m.fault("put"); err != nil {
		return "", err
	}
	m.save(key, data, contentType, metadata)
	return "https://images.example.com/" + key, nil
}
func (m *Objects) UpdateMetadata(ctx context.Context, key, contentType string, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fault("metadata"); err != nil {
		return err
	}
	item, exists := m.items[key]
	if !exists {
		return domain.ErrNotFound
	}
	item.metadata = maps.Clone(metadata)
	item.contentType = contentType
	m.items[key] = item
	return nil
}
func (m *Objects) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.fault("delete"); err != nil {
		return err
	}
	delete(m.items, key)
	return nil
}
func (m *Objects) Metadata(key string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.items[key].metadata)
}
func (m *Objects) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := []string{}
	for key := range m.items {
		keys = append(keys, key)
	}
	return keys
}

// S3Server implements the GET/PUT/DELETE subset used by the real AWS adapter.
func S3Server(objects *Objects) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		if len(parts) != 2 {
			http.Error(w, "bucket/key required", 400)
			return
		}
		key := parts[1]
		fail := func(err error) {
			status, code := 500, "InternalError"
			if err == domain.ErrNotFound {
				status, code = 404, "NoSuchKey"
			}
			if err == domain.ErrConflict {
				status, code = 412, "PreconditionFailed"
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(status)
			fmt.Fprintf(w, "<Error><Code>%s</Code></Error>", code)
		}
		switch r.Method {
		case "GET":
			raw, etag, err := objects.Read(r.Context(), key)
			if err != nil {
				fail(err)
				return
			}
			w.Header().Set("ETag", etag)
			objects.mu.Lock()
			contentType := objects.items[key].contentType
			objects.mu.Unlock()
			w.Header().Set("Content-Type", contentType)
			w.Write(raw)
		case "PUT":
			if source := r.Header.Get("X-Amz-Copy-Source"); source != "" {
				decoded, err := url.PathUnescape(source)
				if err != nil {
					fail(err)
					return
				}
				_, sourceKey, _ := strings.Cut(decoded, "/")
				raw, _, err := objects.Read(r.Context(), sourceKey)
				if err != nil {
					fail(err)
					return
				}
				metadata := map[string]string{}
				for name := range r.Header {
					if suffix, ok := strings.CutPrefix(strings.ToLower(name), "x-amz-meta-"); ok {
						metadata[suffix] = r.Header.Get(name)
					}
				}
				_, err = objects.Put(r.Context(), key, bytes.NewReader(raw), int64(len(raw)), r.Header.Get("Content-Type"), metadata)
				if err != nil {
					fail(err)
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w, `<CopyObjectResult><ETag>"copied"</ETag><LastModified>2026-01-01T00:00:00Z</LastModified></CopyObjectResult>`)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				fail(err)
				return
			}
			if r.Header.Get("If-Match") != "" || r.Header.Get("If-None-Match") != "" {
				etag, err := objects.PutIf(r.Context(), key, body, r.Header.Get("If-Match"), r.Header.Get("If-None-Match") == "*")
				if err != nil {
					fail(err)
					return
				}
				w.Header().Set("ETag", etag)
				return
			}
			_, err = objects.Put(r.Context(), key, strings.NewReader(string(body)), int64(len(body)), r.Header.Get("Content-Type"), nil)
			if err != nil {
				fail(err)
				return
			}
			_, etag, _ := objects.Read(r.Context(), key)
			w.Header().Set("ETag", etag)
		case "DELETE":
			if err := objects.Delete(r.Context(), key); err != nil {
				fail(err)
				return
			}
			w.WriteHeader(204)
		default:
			http.Error(w, "unsupported method", 405)
		}
	}))
}
