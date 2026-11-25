package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const (
	noResponseBody     = ""
	assetName          = "gallery.css"
	assetBody          = ".gallery {}"
	assetLink          = "linked.css"
	assetDirectory     = "assets"
	missingAsset       = "/missing.css"
	draftRoute         = "/blog/draft"
	fileResponseFormat = "%s %s: got HTTP %d and body %q"
	indexCacheFailure  = "SPA response missing its cache policy"
)

func TestFrontendServesOnlyFilesInBuildDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{indexFilename: frontendHTML, assetName: assetBody} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, assetDirectory), 0700); err != nil {
		t.Fatal(err)
	}
	otherFile := filepath.Join(t.TempDir(), assetName)
	if err := os.WriteFile(otherFile, []byte(assetBody), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otherFile, filepath.Join(dir, assetLink)); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{FrontendDir: dir}}
	for _, tc := range []struct {
		method, path string
		status       int
		body         string
	}{
		{http.MethodGet, homePath + assetName, http.StatusOK, assetBody},
		{http.MethodHead, homePath + assetName, http.StatusOK, noResponseBody},
		{http.MethodGet, draftRoute, http.StatusOK, frontendHTML},
		{http.MethodGet, homePath + assetDirectory, http.StatusNotFound, noResponseBody},
		{http.MethodGet, homePath + assetLink, http.StatusNotFound, noResponseBody},
		{http.MethodGet, missingAsset, http.StatusNotFound, noResponseBody},
		{http.MethodGet, homePath + indexFilename, http.StatusMovedPermanently, noResponseBody},
	} {
		w := httptest.NewRecorder()
		s.frontend(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || (tc.status == http.StatusOK && w.Body.String() != tc.body) {
			t.Errorf(fileResponseFormat, tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.path == draftRoute && w.Header().Get(frontendCacheControlHeader) != noCache {
			t.Error(indexCacheFailure)
		}
	}
}
