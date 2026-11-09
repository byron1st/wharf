package github_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/github"
)

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/releases/latest", r.URL.Path)
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))

		_, _ = w.Write(
			[]byte(`{"tag_name":"v0.2.0","html_url":"https://github.com/r/v0.2.0","body":"notes","draft":false}`),
		)
	}))
	defer srv.Close()

	rel, err := github.NewClientWith(srv.URL, srv.URL).LatestRelease(t.Context())
	require.NoError(t, err)
	require.Equal(t, github.Release{Tag: "v0.2.0", URL: "https://github.com/r/v0.2.0", Notes: "notes"}, rel)
}

func TestLatestReleaseFailures(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		_, err := github.NewClientWith(srv.URL, srv.URL).LatestRelease(t.Context())
		require.ErrorIs(t, err, github.ErrUnexpectedLatestReleaseStatus)
		require.ErrorContains(t, err, "status=404")
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{`))
		}))
		defer srv.Close()

		_, err := github.NewClientWith(srv.URL, srv.URL).LatestRelease(t.Context())
		require.ErrorIs(t, err, github.ErrFailedToDecodeLatestRelease)
	})

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()

		_, err := github.NewClientWith(srv.URL, srv.URL).LatestRelease(t.Context())
		require.ErrorIs(t, err, github.ErrFailedToRequestLatestRelease)
	})
}

func TestDownload(t *testing.T) {
	mux := http.NewServeMux()
	// GitHub answers a release download with a redirect to its storage host.
	mux.HandleFunc("/v0.2.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/storage/checksums.txt", http.StatusFound)
	})
	mux.HandleFunc("/storage/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("abc  wharf.tar.gz\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := github.NewClientWith(srv.URL, srv.URL)

	data, err := c.Download(t.Context(), "v0.2.0", "checksums.txt")
	require.NoError(t, err)
	require.Equal(t, "abc  wharf.tar.gz\n", string(data))

	_, err = c.Download(t.Context(), "v0.2.0", "missing.tar.gz")
	require.ErrorIs(t, err, github.ErrUnexpectedDownloadStatus)
	require.ErrorContains(t, err, "status=404")
}
