package r2

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestR2S3Requests(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.URL.Path != "/gallery/photos/test.png" {
			t.Errorf("wrong object path: %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "/auto/s3/aws4_request") {
			t.Error("request not signed for auto region")
		}
		if r.Method == "PUT" && r.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("image cache policy missing: %q", r.Header.Get("Cache-Control"))
		}
		switch {
		case r.Method == "PUT" && r.Header.Get("X-Amz-Copy-Source") != "":
			source, err := url.PathUnescape(r.Header.Get("X-Amz-Copy-Source"))
			if err != nil || source != "gallery/photos/test.png" {
				t.Errorf("bad copy source: %s", source)
			}
			if r.Header.Get("X-Amz-Metadata-Directive") != "REPLACE" || r.Header.Get("X-Amz-Meta-Photo") != "updated" || r.Header.Get("Content-Type") != "image/png" {
				t.Error("metadata replacement missing")
			}
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, `<CopyObjectResult><ETag>"test"</ETag><LastModified>2026-01-01T00:00:00Z</LastModified></CopyObjectResult>`)
		case r.Method == "PUT":
			data, _ := io.ReadAll(r.Body)
			if !bytes.Equal(data, []byte("photo content")) || r.ContentLength != 13 {
				t.Errorf("bad body or length: %q %d", data, r.ContentLength)
			}
			if r.Header.Get("Content-Type") != "image/png" || r.Header.Get("X-Amz-Meta-Photo") != "encoded" {
				t.Error("content type / custom metadata missing")
			}
			if r.Header.Get("X-Amz-Trailer") != "" {
				t.Error("unexpected checksum trailers")
			}
			w.Header().Set("ETag", `"test"`)
		case r.Method == "DELETE":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected request %s", r.Method)
		}
	}))
	defer server.Close()
	r2, err := New(Config{Endpoint: server.URL, Bucket: "gallery", AccessKeyID: "test-key", SecretAccessKey: "test-secret", PublicURL: "https://photos.example.com/"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	public, err := r2.Put(ctx, "photos/test.png", bytes.NewReader([]byte("photo content")), 13, "image/png", map[string]string{"photo": "encoded"})
	if err != nil {
		t.Fatal(err)
	}
	if public != "https://photos.example.com/photos/test.png" {
		t.Fatal(public)
	}
	if err := r2.UpdateMetadata(ctx, "photos/test.png", "image/png", map[string]string{"photo": "updated"}); err != nil {
		t.Fatal(err)
	}
	if err := r2.Delete(ctx, "photos/test.png"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "PUT,PUT,DELETE" {
		t.Fatal(methods)
	}
}

func TestR2CachePolicyOnlyAppliesToImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "no-store"
		if r.URL.Path == "/bucket/images/test.png" {
			want = "public, max-age=31536000, immutable"
		}
		if got := r.Header.Get("Cache-Control"); got != want {
			t.Errorf("%s cache policy = %q; want %q", r.URL.Path, got, want)
		}
		io.Copy(io.Discard, r.Body)
		w.Header().Set("ETag", `"test"`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Bucket: "bucket", AccessKeyID: "test", SecretAccessKey: "test", PublicURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ key, mime string }{
		{"images/test.png", "image/png"}, {"images/test.json", "application/json"},
		{"posts/test/version.json", "application/json"}, {"catalog/part.json", "application/json"},
	} {
		if _, err := store.Put(context.Background(), item.key, strings.NewReader("original"), 8, item.mime, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.PutIf(context.Background(), "catalog.json", []byte("{}"), "", true); err != nil {
		t.Fatal(err)
	}
}
