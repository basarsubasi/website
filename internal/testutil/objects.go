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

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	emptyString   = ""
	readOperation = "read"

	etagFormat = `"%d"`

	writeOperation      = "write"
	jsonContentType     = "application/json"
	lostResponseMessage = "response lost after commit"

	incorrectLengthMessage = "incorrect content length"
	putOperation           = "put"
	publicURLPrefix        = "https://images.example.com/"

	metadataOperation = "metadata"

	deleteOperation = "delete"

	keySeparator         = "/"
	unknownBucketMessage = "unknown bucket"

	missingKeyMessage        = "bucket/key required"
	internalErrorCode        = "InternalError"
	noSuchKeyCode            = "NoSuchKey"
	preconditionFailedCode   = "PreconditionFailed"
	contentTypeHeader        = "Content-Type"
	xmlContentType           = "application/xml"
	errorXMLFormat           = "<Error><Code>%s</Code></Error>"
	getMethod                = "GET"
	etagHeader               = "ETag"
	putMethod                = "PUT"
	copySourceHeader         = "X-Amz-Copy-Source"
	metadataHeaderPrefix     = "x-amz-meta-"
	copyResultXML            = `<CopyObjectResult><ETag>"copied"</ETag><LastModified>2026-01-01T00:00:00Z</LastModified></CopyObjectResult>`
	ifMatchHeader            = "If-Match"
	ifNoneMatchHeader        = "If-None-Match"
	anyETag                  = "*"
	deleteMethod             = "DELETE"
	unsupportedMethodMessage = "unsupported method"
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
		return nil, emptyString, err
	}
	if err := m.fault(readOperation); err != nil {
		return nil, emptyString, err
	}
	item, ok := m.items[key]
	if !ok {
		return nil, emptyString, domain.ErrNotFound
	}
	return append([]byte{}, item.body...), item.etag, nil
}
func (m *Objects) save(key string, body []byte, contentType string, metadata map[string]string) string {
	m.counter++
	etag := fmt.Sprintf(etagFormat, m.counter)
	m.items[key] = object{body: append([]byte{}, body...), etag: etag, contentType: contentType, metadata: maps.Clone(metadata)}
	return etag
}
func (m *Objects) PutIf(ctx context.Context, key string, data []byte, etag string, create bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return emptyString, err
	}
	if err := m.fault(writeOperation); err != nil {
		return emptyString, err
	}
	current, exists := m.items[key]
	if (create && exists) || (!create && (!exists || current.etag != etag)) {
		return emptyString, domain.ErrConflict
	}
	newETag := m.save(key, data, jsonContentType, nil)
	if m.ambiguous {
		m.ambiguous = false
		return emptyString, fmt.Errorf(lostResponseMessage)
	}
	return newETag, nil
}
func (m *Objects) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string, metadata map[string]string) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return emptyString, err
	}
	if int64(len(data)) != size {
		return emptyString, fmt.Errorf(incorrectLengthMessage)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return emptyString, err
	}
	if err := m.fault(putOperation); err != nil {
		return emptyString, err
	}
	m.save(key, data, contentType, metadata)
	return publicURLPrefix + key, nil
}
func (m *Objects) UpdateMetadata(ctx context.Context, key, contentType string, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fault(metadataOperation); err != nil {
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
	if err := m.fault(deleteOperation); err != nil {
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
	return httptest.NewServer(s3Handler(objects))
}

// S3BucketsServer isolates objects by bucket, including catalog.json.
func S3BucketsServer(buckets map[string]*Objects) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bucket, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, keySeparator), keySeparator)
		objects, ok := buckets[bucket]
		if !ok {
			http.Error(w, unknownBucketMessage, http.StatusNotFound)
			return
		}
		s3Handler(objects).ServeHTTP(w, r)
	}))
}

func s3Handler(objects *Objects) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, keySeparator), keySeparator, 2)
		if len(parts) != 2 {
			http.Error(w, missingKeyMessage, 400)
			return
		}
		key := parts[1]
		switch r.Method {
		case getMethod:
			raw, etag, err := objects.Read(r.Context(), key)
			if err != nil {
				writeS3Error(w, err)
				return
			}
			w.Header().Set(etagHeader, etag)
			objects.mu.Lock()
			contentType := objects.items[key].contentType
			objects.mu.Unlock()
			w.Header().Set(contentTypeHeader, contentType)
			w.Write(raw)
		case putMethod:
			if source := r.Header.Get(copySourceHeader); source != emptyString {
				objects.serveCopy(w, r, key, source)
			} else {
				objects.servePut(w, r, key)
			}
		case deleteMethod:
			if err := objects.Delete(r.Context(), key); err != nil {
				writeS3Error(w, err)
				return
			}
			w.WriteHeader(204)
		default:
			http.Error(w, unsupportedMethodMessage, 405)
		}
	})
}

func writeS3Error(w http.ResponseWriter, err error) {
	status, code := 500, internalErrorCode
	if err == domain.ErrNotFound {
		status, code = 404, noSuchKeyCode
	}
	if err == domain.ErrConflict {
		status, code = 412, preconditionFailedCode
	}
	w.Header().Set(contentTypeHeader, xmlContentType)
	w.WriteHeader(status)
	fmt.Fprintf(w, errorXMLFormat, code)
}

func (objects *Objects) serveCopy(w http.ResponseWriter, r *http.Request, key, source string) {
	decoded, err := url.PathUnescape(source)
	if err != nil {
		writeS3Error(w, err)
		return
	}
	_, sourceKey, _ := strings.Cut(decoded, keySeparator)
	raw, _, err := objects.Read(r.Context(), sourceKey)
	if err != nil {
		writeS3Error(w, err)
		return
	}
	metadata := map[string]string{}
	for name := range r.Header {
		if suffix, ok := strings.CutPrefix(strings.ToLower(name), metadataHeaderPrefix); ok {
			metadata[suffix] = r.Header.Get(name)
		}
	}
	_, err = objects.Put(r.Context(), key, bytes.NewReader(raw), int64(len(raw)), r.Header.Get(contentTypeHeader), metadata)
	if err != nil {
		writeS3Error(w, err)
		return
	}
	w.Header().Set(contentTypeHeader, xmlContentType)
	fmt.Fprint(w, copyResultXML)
}

func (objects *Objects) servePut(w http.ResponseWriter, r *http.Request, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeS3Error(w, err)
		return
	}
	if r.Header.Get(ifMatchHeader) != emptyString || r.Header.Get(ifNoneMatchHeader) != emptyString {
		etag, err := objects.PutIf(r.Context(), key, body, r.Header.Get(ifMatchHeader), r.Header.Get(ifNoneMatchHeader) == anyETag)
		if err != nil {
			writeS3Error(w, err)
			return
		}
		w.Header().Set(etagHeader, etag)
		return
	}
	_, err = objects.Put(r.Context(), key, bytes.NewReader(body), int64(len(body)), r.Header.Get(contentTypeHeader), nil)
	if err != nil {
		writeS3Error(w, err)
		return
	}
	_, etag, _ := objects.Read(r.Context(), key)
	w.Header().Set(etagHeader, etag)
}
