package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"writers.studio/internal/config"
)

func TestStaticHandlerCachesHashedAssetsAndKeepsSPAFallbackHTMLOnly(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(staticDir, "index.html"),
		[]byte("<!doctype html><title>Writer</title>"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(staticDir, "assets", "app-123.js"),
		[]byte("window.writer=true"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{config: config.Config{StaticDir: staticDir}}).staticHandler()

	t.Run("serves a client route through a revalidated index", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/projects/123/studio/456", nil)
		request.Header.Set("Accept", "text/html,application/xhtml+xml")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Writer") {
			t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
		if cache := response.Header().Get("Cache-Control"); cache != "no-cache" {
			t.Fatalf("index cache policy=%q", cache)
		}
	})

	t.Run("does not turn a missing script into index HTML", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/assets/removed-hash.js", nil)
		request.Header.Set("Accept", "*/*")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Fatalf("missing asset status=%d body=%q", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "<!doctype html>") {
			t.Fatal("missing asset was replaced with SPA HTML")
		}
	})

	t.Run("serves hashed assets as immutable", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/assets/app-123.js", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK || response.Body.String() != "window.writer=true" {
			t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
		if cache := response.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
			t.Fatalf("asset cache policy=%q", cache)
		}
	})
}
