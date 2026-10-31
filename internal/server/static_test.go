package server_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/server"
)

// webApp stands in for the built web app in every handler under test.
var webApp = fstest.MapFS{
	"index.html":       {Data: []byte("<!doctype html><title>Wharf</title>")},
	"assets/app-1.js":  {Data: []byte("console.log(1)")},
	"assets/style.css": {Data: []byte("body{}")},
}

func TestWebAppServesFiles(t *testing.T) {
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/assets/app-1.js")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "console.log(1)", rec.Body.String())
	require.Contains(t, rec.Header().Get("Content-Type"), "javascript")
}

func TestWebAppFallsBackToIndex(t *testing.T) {
	for _, path := range []string{"/", "/settings/general", "/task/WHF-003", "/assets", "/missing.js", "/tasks"} {
		rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, path)

		require.Equal(t, http.StatusOK, rec.Code, path)
		require.Equal(t, "<!doctype html><title>Wharf</title>", rec.Body.String(), path)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html", path)
	}
}

func TestWebAppRedirectsIndexToRoot(t *testing.T) {
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/index.html")

	require.Equal(t, http.StatusMovedPermanently, rec.Code)
	require.Equal(t, "./", rec.Header().Get("Location"))
}

func TestWebAppLeavesUnknownAPIRoutes(t *testing.T) {
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/api/nothing")

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"server: no such api route"}`, rec.Body.String())
}

func TestWebAppNotBuilt(t *testing.T) {
	var logs bytes.Buffer

	// Only the committed .gitkeep, as before the first web build.
	unbuilt := fstest.MapFS{".gitkeep": {}}
	handler := server.NewHandler(
		mocks.NewMockService(t),
		mocks.NewMockSettings(t),
		unbuilt,
		slog.New(slog.NewJSONHandler(&logs, nil)),
	)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "make build")
}
