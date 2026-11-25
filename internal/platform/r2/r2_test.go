package r2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	galleryBucketPrefix        = "/" + bucketName + "/"
	otherBucket                = "blog"
	excessConnectionFormat     = "second bucket acquired a connection at capacity: %v"
	expectedObjectPath         = "/gallery/photos/test.png"
	wrongPathFormat            = "wrong object path: %s"
	authorizationHeader        = "Authorization"
	signingScope               = "/auto/s3/aws4_request"
	missingSignatureFailure    = "request not signed for auto region"
	putMethod                  = "PUT"
	cacheControlHeader         = "Cache-Control"
	r2TestImmutableCachePolicy = "public, max-age=31536000, immutable"
	missingImageCacheFormat    = "image cache policy missing: %q"
	copySourceHeader           = "X-Amz-Copy-Source"
	r2TestEmptyValue           = ""
	expectedCopySource         = "gallery/photos/test.png"
	wrongCopySourceFormat      = "bad copy source: %s"
	metadataDirectiveHeader    = "X-Amz-Metadata-Directive"
	replaceDirective           = "REPLACE"
	photoMetadataHeader        = "X-Amz-Meta-Photo"
	updatedMetadata            = "updated"
	contentTypeHeader          = "Content-Type"
	pngContentType             = "image/png"
	metadataReplacementFailure = "metadata replacement missing"
	xmlContentType             = "application/xml"
	copyResultXML              = `<CopyObjectResult><ETag>"test"</ETag><LastModified>2026-01-01T00:00:00Z</LastModified></CopyObjectResult>`
	photoContents              = "photo content"
	invalidBodyFormat          = "bad body or length: %q %d"
	encodedMetadata            = "encoded"
	missingMetadataFailure     = "content type / custom metadata missing"
	trailerHeader              = "X-Amz-Trailer"
	checksumTrailerFailure     = "unexpected checksum trailers"
	etagHeader                 = "ETag"
	responseETag               = `"test"`
	deleteMethod               = "DELETE"
	unexpectedRequestFormat    = "unexpected request %s"
	bucketName                 = "gallery"
	accessKeyID                = "test-key"
	secretAccessKey            = "test-secret"
	publicBaseURL              = "https://photos.example.com/"
	photoKey                   = "photos/test.png"
	photoMetadataKey           = "photo"
	expectedPhotoURL           = "https://photos.example.com/photos/test.png"
	methodSeparator            = ","
	expectedMethods            = "PUT,PUT,DELETE"

	r2TestNoStorePolicy   = "no-store"
	cachedImagePath       = "/bucket/images/test.png"
	cachePolicyFormat     = "%s cache policy = %q; want %q"
	cacheTestBucket       = "bucket"
	cacheTestCredential   = "test"
	blogImageKey          = "images/test.png"
	jsonImageKey          = "images/test.json"
	r2TestJsonContentType = "application/json"
	postVersionKey        = "posts/test/version.json"
	r2TestCatalogPartKey  = "catalog/part.json"
	originalContents      = "original"
	catalogKey            = "catalog.json"
	emptyJSONObject       = "{}"
)

func TestR2S3Requests(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		checkS3Request(t, w, r)
	}))
	defer server.Close()
	r2, err := New(Config{Endpoint: server.URL, Bucket: bucketName, AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey, PublicURL: publicBaseURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	public, err := r2.Put(ctx, photoKey, bytes.NewReader([]byte(photoContents)), 13, pngContentType, map[string]string{photoMetadataKey: encodedMetadata})
	if err != nil {
		t.Fatal(err)
	}
	if public != expectedPhotoURL {
		t.Fatal(public)
	}
	if err := r2.UpdateMetadata(ctx, photoKey, pngContentType, map[string]string{photoMetadataKey: updatedMetadata}); err != nil {
		t.Fatal(err)
	}
	if err := r2.Delete(ctx, photoKey); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, methodSeparator) != expectedMethods {
		t.Fatal(methods)
	}
}

func TestR2CachePolicyOnlyAppliesToImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := r2TestNoStorePolicy
		if r.URL.Path == cachedImagePath {
			want = r2TestImmutableCachePolicy
		}
		if got := r.Header.Get(cacheControlHeader); got != want {
			t.Errorf(cachePolicyFormat, r.URL.Path, got, want)
		}
		io.Copy(io.Discard, r.Body)
		w.Header().Set(etagHeader, responseETag)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Bucket: cacheTestBucket, AccessKeyID: cacheTestCredential, SecretAccessKey: cacheTestCredential, PublicURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ key, mime string }{
		{blogImageKey, pngContentType}, {jsonImageKey, r2TestJsonContentType},
		{postVersionKey, r2TestJsonContentType}, {r2TestCatalogPartKey, r2TestJsonContentType},
	} {
		if _, err := store.Put(context.Background(), item.key, strings.NewReader(originalContents), 8, item.mime, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.PutIf(context.Background(), catalogKey, []byte(emptyJSONObject), r2TestEmptyValue, true); err != nil {
		t.Fatal(err)
	}
}

func checkS3Request(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.URL.Path != expectedObjectPath {
		t.Errorf(wrongPathFormat, r.URL.Path)
	}
	if !strings.Contains(r.Header.Get(authorizationHeader), signingScope) {
		t.Error(missingSignatureFailure)
	}
	if r.Method == putMethod && r.Header.Get(cacheControlHeader) != r2TestImmutableCachePolicy {
		t.Errorf(missingImageCacheFormat, r.Header.Get(cacheControlHeader))
	}
	switch {
	case r.Method == putMethod && r.Header.Get(copySourceHeader) != r2TestEmptyValue:
		checkCopyRequest(t, w, r)
	case r.Method == putMethod:
		checkPutRequest(t, w, r)
	case r.Method == deleteMethod:
		w.WriteHeader(204)
	default:
		t.Errorf(unexpectedRequestFormat, r.Method)
	}
}

func checkCopyRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	source, err := url.PathUnescape(r.Header.Get(copySourceHeader))
	if err != nil || source != expectedCopySource {
		t.Errorf(wrongCopySourceFormat, source)
	}
	if r.Header.Get(metadataDirectiveHeader) != replaceDirective || r.Header.Get(photoMetadataHeader) != updatedMetadata || r.Header.Get(contentTypeHeader) != pngContentType {
		t.Error(metadataReplacementFailure)
	}
	w.Header().Set(contentTypeHeader, xmlContentType)
	io.WriteString(w, copyResultXML)
}

func checkPutRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	data, _ := io.ReadAll(r.Body)
	if !bytes.Equal(data, []byte(photoContents)) || r.ContentLength != 13 {
		t.Errorf(invalidBodyFormat, data, r.ContentLength)
	}
	if r.Header.Get(contentTypeHeader) != pngContentType || r.Header.Get(photoMetadataHeader) != encodedMetadata {
		t.Error(missingMetadataFailure)
	}
	if r.Header.Get(trailerHeader) != r2TestEmptyValue {
		t.Error(checksumTrailerFailure)
	}
	w.Header().Set(etagHeader, responseETag)
}

func TestBucketsShareOutboundConnectionLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, maxConnectionsPerHost)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, galleryBucketPrefix) {
			entered <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer endpoint.Close()
	var workers sync.WaitGroup
	defer workers.Wait()
	defer unblock()

	config := Config{Endpoint: endpoint.URL, Bucket: bucketName, AccessKeyID: accessKeyID, SecretAccessKey: cacheTestCredential, PublicURL: endpoint.URL}
	gallery, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	config.Bucket = otherBucket
	blog, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	for range maxConnectionsPerHost {
		workers.Go(func() {
			if _, _, err := gallery.Read(ctx, CatalogKey); err != nil {
				t.Error(err)
			}
		})
	}
	for range maxConnectionsPerHost {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	wait, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	_, _, err = blog.Read(wait, CatalogKey)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(excessConnectionFormat, err)
	}
	unblock()
	if _, _, err := blog.Read(ctx, CatalogKey); err != nil {
		t.Fatal(err)
	}
}
