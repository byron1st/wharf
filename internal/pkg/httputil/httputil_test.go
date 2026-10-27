package httputil_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/httputil"
)

const secret = "Basic c2VjcmV0LXRva2Vu"

type received struct {
	method string
	path   string
	header http.Header
	body   string
}

// newServer records the last request and answers with status and body.
func newServer(t *testing.T, status int, body string) (*httptest.Server, *received) {
	t.Helper()

	got := &received{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		*got = received{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: string(b)}

		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv, got
}

func header() http.Header {
	return http.Header{"Authorization": {secret}, "Accept": {"application/json"}}
}

func TestGet(t *testing.T) {
	srv, got := newServer(t, http.StatusOK, `{"ok":true}`)

	body, err := httputil.NewHTTPClient().Get(t.Context(), srv.URL+"/a/b", header())
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(body))
	require.Equal(t, http.MethodGet, got.method)
	require.Equal(t, "/a/b", got.path)
	require.Equal(t, secret, got.header.Get("Authorization"))
	require.Equal(t, "application/json", got.header.Get("Accept"))
	require.Empty(t, got.body)
}

func TestPost(t *testing.T) {
	srv, got := newServer(t, http.StatusCreated, "created")

	body, err := httputil.NewHTTPClient().Post(t.Context(), srv.URL+"/p", header(), []byte(`{"q":1}`))
	require.NoError(t, err)
	require.Equal(t, "created", string(body))
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/p", got.path)
	require.Equal(t, secret, got.header.Get("Authorization"))
	require.JSONEq(t, `{"q":1}`, got.body)
}

func TestNilHeader(t *testing.T) {
	srv, _ := newServer(t, http.StatusOK, "ok")

	body, err := httputil.NewHTTPClient().Get(t.Context(), srv.URL, nil)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
}

func TestUnexpectedStatusCode(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "client error", status: http.StatusUnauthorized, body: "bad credentials", want: "bad credentials"},
		{name: "server error", status: http.StatusServiceUnavailable, body: "down", want: "down"},
		{name: "redirect-like 3xx", status: http.StatusNotModified, body: "", want: ""},
		{
			name:   "long body is truncated",
			status: http.StatusBadRequest,
			body:   strings.Repeat("x", 600),
			want:   strings.Repeat("x", 512),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newServer(t, tt.status, tt.body)

			for _, call := range []func() ([]byte, error){
				func() ([]byte, error) { return httputil.NewHTTPClient().Get(t.Context(), srv.URL, header()) },
				func() ([]byte, error) { return httputil.NewHTTPClient().Post(t.Context(), srv.URL, header(), nil) },
			} {
				body, err := call()
				require.ErrorIs(t, err, httputil.ErrUnexpectedStatusCode)
				require.ErrorContains(t, err, "status="+strconv.Itoa(tt.status))
				require.ErrorContains(t, err, "body="+tt.want)
				require.NotContains(t, err.Error(), strings.Repeat("x", 513))
				require.NotContains(t, err.Error(), secret)
				require.Nil(t, body)
			}
		})
	}
}

func TestConnectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	body, err := httputil.NewHTTPClient().Post(t.Context(), url, header(), []byte("{}"))
	require.ErrorIs(t, err, httputil.ErrFailedToSendRequest)
	require.NotContains(t, err.Error(), secret)
	require.Nil(t, body)
}

func TestCanceledContext(t *testing.T) {
	srv, _ := newServer(t, http.StatusOK, "ok")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	body, err := httputil.NewHTTPClient().Get(ctx, srv.URL, header())
	require.ErrorIs(t, err, httputil.ErrFailedToSendRequest)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, body)
}

func TestInvalidURL(t *testing.T) {
	body, err := httputil.NewHTTPClient().Get(t.Context(), "http://bad host/", header())
	require.ErrorIs(t, err, httputil.ErrFailedToCreateRequest)
	require.Nil(t, body)
}

func TestTruncatedResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write([]byte("ab"))
	}))
	t.Cleanup(srv.Close)

	body, err := httputil.NewHTTPClient().Get(t.Context(), srv.URL, header())
	require.ErrorIs(t, err, httputil.ErrFailedToReadResponseBody)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, body)
}

func TestErrorsRedactURLPassword(t *testing.T) {
	srv, _ := newServer(t, http.StatusUnauthorized, "denied")
	withUserinfo := strings.Replace(srv.URL, "http://", "http://user:hunter2@", 1)

	body, err := httputil.NewHTTPClient().Get(t.Context(), withUserinfo, header())
	require.ErrorIs(t, err, httputil.ErrUnexpectedStatusCode)
	require.ErrorContains(t, err, "url=")
	require.NotContains(t, err.Error(), "hunter2")
	require.Nil(t, body)

	body, err = httputil.NewHTTPClient().Get(t.Context(), "http://user:hunter2@bad host/", header())
	require.ErrorIs(t, err, httputil.ErrFailedToCreateRequest)
	require.NotContains(t, err.Error(), "hunter2")
	require.Nil(t, body)
}

func TestErrorsDropURLUsername(t *testing.T) {
	// URL.Redacted masks only the password, so a username-only userinfo
	// (e.g. an API token as the user) must be dropped entirely.
	srv, _ := newServer(t, http.StatusUnauthorized, "denied")
	withUserinfo := strings.Replace(srv.URL, "http://", "http://hunter2@", 1)

	body, err := httputil.NewHTTPClient().Get(t.Context(), withUserinfo, header())
	require.ErrorIs(t, err, httputil.ErrUnexpectedStatusCode)
	require.ErrorContains(t, err, "url="+srv.URL)
	require.NotContains(t, err.Error(), "hunter2")
	require.Nil(t, body)
}

// maxBodyBytes mirrors the package's unexported response body limit (32 MiB).
const maxBodyBytes = 32 << 20

// newStreamingServer answers with status and a body of size bytes, written in
// chunks so the test never holds the whole body.
func newStreamingServer(t *testing.T, status, size int) *httptest.Server {
	t.Helper()

	chunk := []byte(strings.Repeat("x", 64<<10))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)

		for left := size; left > 0; left -= len(chunk) {
			if _, err := w.Write(chunk[:min(left, len(chunk))]); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestResponseBodyTooLarge(t *testing.T) {
	srv := newStreamingServer(t, http.StatusOK, maxBodyBytes+1)
	withUserinfo := strings.Replace(srv.URL, "http://", "http://user:hunter2@", 1)

	body, err := httputil.NewHTTPClient().Get(t.Context(), withUserinfo, header())
	require.ErrorIs(t, err, httputil.ErrResponseBodyTooLarge)
	require.ErrorContains(t, err, "limit="+strconv.Itoa(maxBodyBytes))
	require.ErrorContains(t, err, "url=")
	require.NotContains(t, err.Error(), "hunter2")
	require.NotContains(t, err.Error(), secret)
	require.Nil(t, body)
}

func TestResponseBodyAtLimit(t *testing.T) {
	srv := newStreamingServer(t, http.StatusOK, maxBodyBytes)

	body, err := httputil.NewHTTPClient().Post(t.Context(), srv.URL, header(), nil)
	require.NoError(t, err)
	require.Len(t, body, maxBodyBytes)
}

func TestLargeErrorBodyIsTruncated(t *testing.T) {
	srv := newStreamingServer(t, http.StatusBadGateway, maxBodyBytes+1)

	body, err := httputil.NewHTTPClient().Get(t.Context(), srv.URL, header())
	require.ErrorIs(t, err, httputil.ErrUnexpectedStatusCode)
	require.NotErrorIs(t, err, httputil.ErrResponseBodyTooLarge)
	require.ErrorContains(t, err, "body="+strings.Repeat("x", 512))
	require.NotContains(t, err.Error(), strings.Repeat("x", 513))
	require.Nil(t, body)
}
