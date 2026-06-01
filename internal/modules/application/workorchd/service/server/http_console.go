package server

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server/consoleassets"
)

func registerConsoleRoutes(router engine.RouteRegistry) {
	handler := func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serveConsoleAsset(w, r)
	}
	router.AddHandler("/favicon.ico", http.MethodGet, func(_ context.Context, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/console", http.MethodGet, handler)
	router.AddHandler("/console/", http.MethodGet, handler)
	router.AddHandler("/console/**", http.MethodGet, handler)
}

func serveConsoleAsset(w http.ResponseWriter, r *http.Request) {
	staticFS, err := fs.Sub(consoleassets.Assets, "dist")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	assetPath := strings.TrimPrefix(r.URL.Path, "/console")
	switch {
	case assetPath == "" || assetPath == "/":
		serveConsoleFile(w, r, staticFS, "index.html")
	case strings.HasPrefix(assetPath, "/assets/"):
		serveConsoleFile(w, r, staticFS, strings.TrimPrefix(assetPath, "/"))
	default:
		serveConsoleFile(w, r, staticFS, "index.html")
	}
}

func serveConsoleFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS, name string) {
	content, err := fs.ReadFile(staticFS, path.Clean(name))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}
