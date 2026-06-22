package platform

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebUIServesShellAndAssets(t *testing.T) {
	root := t.TempDir()
	writeWebUITestFile(t, filepath.Join(root, "index.html"), "<!doctype html><title>NexusPaaS</title>")
	writeWebUITestFile(t, filepath.Join(root, "assets", "app.js"), "console.log('nexuspaas')")

	app := NewApp(Config{RequireAuth: true, WebUIDir: root})

	redirect := httptest.NewRecorder()
	app.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/ui", nil))
	if redirect.Code != http.StatusPermanentRedirect {
		t.Fatalf("GET /ui = %d, want %d", redirect.Code, http.StatusPermanentRedirect)
	}
	if location := redirect.Header().Get("Location"); location != "/ui/" {
		t.Fatalf("GET /ui Location = %q, want /ui/", location)
	}

	index := httptest.NewRecorder()
	app.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("GET /ui/ = %d, want 200", index.Code)
	}
	if body := index.Body.String(); !strings.Contains(body, "NexusPaaS") {
		t.Fatalf("GET /ui/ body = %q, want GUI shell", body)
	}

	asset := httptest.NewRecorder()
	app.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/ui/assets/app.js", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("GET /ui/assets/app.js = %d, want 200", asset.Code)
	}
	if body := asset.Body.String(); !strings.Contains(body, "nexuspaas") {
		t.Fatalf("GET /ui/assets/app.js body = %q, want asset content", body)
	}
}

func TestWebUIFallbackAndMissingAssets(t *testing.T) {
	root := t.TempDir()
	writeWebUITestFile(t, filepath.Join(root, "index.html"), "<!doctype html><main>Operations</main>")
	app := NewApp(Config{RequireAuth: false, WebUIDir: root})

	fallback := httptest.NewRecorder()
	app.ServeHTTP(fallback, httptest.NewRequest(http.MethodGet, "/ui/deep/link", nil))
	if fallback.Code != http.StatusOK {
		t.Fatalf("GET /ui/deep/link = %d, want 200", fallback.Code)
	}
	if body := fallback.Body.String(); !strings.Contains(body, "Operations") {
		t.Fatalf("GET /ui/deep/link body = %q, want index fallback", body)
	}

	missing := NewApp(Config{RequireAuth: false, WebUIDir: filepath.Join(root, "missing")})
	rec := httptest.NewRecorder()
	missing.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /ui/ without index = %d, want 404", rec.Code)
	}
}

func writeWebUITestFile(t *testing.T, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("mkdir test web ui dir: %v", err)
	}
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatalf("write test web ui file: %v", err)
	}
}
