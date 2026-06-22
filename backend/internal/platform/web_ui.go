package platform

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const webUIPrefix = "/ui/"

func (a *App) registerWebUI() {
	a.Mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, webUIPrefix, http.StatusPermanentRedirect)
	})
	a.Mux.HandleFunc("GET /ui/{path...}", a.serveWebUI)
}

func (a *App) serveWebUI(w http.ResponseWriter, r *http.Request) {
	filename, ok := webUIFile(a.Config.WebUIDir, r.PathValue("path"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filename)
}

func webUIFile(root, requested string) (string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false
	}

	rel := strings.TrimPrefix(path.Clean("/"+requested), "/")
	if rel == "" || rel == "." {
		rel = "index.html"
	}
	if filename, ok := existingWebUIFile(root, rel); ok {
		return filename, true
	}
	return existingWebUIFile(root, "index.html")
}

func existingWebUIFile(root, name string) (string, bool) {
	filename := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Stat(filename)
	if err != nil || info.IsDir() {
		return "", false
	}
	return filename, true
}
