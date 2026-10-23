package server_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/server"
)

// syncBuffer is a log sink safe for the server goroutines and the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// start runs a Server for handler on a random port. It returns the base URL,
// the logs, a cancel that ends Run, and a wait returning Run's result.
func start(t *testing.T, handler http.Handler) (string, *syncBuffer, context.CancelFunc, func() error) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	logs := &syncBuffer{}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- server.New(handler, slog.New(slog.NewJSONHandler(logs, nil))).Run(ctx, ln) }()

	wait := sync.OnceValue(func() error { return <-done })

	t.Cleanup(func() {
		cancel()

		_ = wait()
	})

	return "http://" + ln.Addr().String(), logs, cancel, wait
}

func get(t *testing.T, url, requestID string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	return resp, err
}

func TestMiddleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	mux.HandleFunc("GET /abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })

	url, logs, _, _ := start(t, mux)

	resp, err := get(t, url+"/panic", "req-panic")
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Equal(t, "req-panic", resp.Header.Get("X-Request-ID"))

	// The server keeps serving after a panic and generates a missing ID.
	resp, err = get(t, url+"/ok", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusTeapot, resp.StatusCode)

	generated := resp.Header.Get("X-Request-ID")
	require.NotEmpty(t, generated)

	// ErrAbortHandler aborts the response instead of becoming a 500.
	_, err = get(t, url+"/abort", "req-abort")
	require.Error(t, err)

	require.Contains(t, logs.String(), `"msg":"panic","request_id":"req-panic","panic":"boom"`)
	require.Contains(
		t,
		logs.String(),
		`"msg":"request","method":"GET","path":"/panic","status":500,"request_id":"req-panic"`,
	)
	require.Contains(
		t,
		logs.String(),
		`"msg":"request","method":"GET","path":"/ok","status":418,"request_id":"`+generated+`"`,
	)
}

func TestRunWaitsForInFlightRequest(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	url, _, cancel, wait := start(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release

		_, _ = w.Write([]byte("done"))
	}))

	type result struct {
		body string
		err  error
	}

	responses := make(chan result, 1)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			responses <- result{err: err}
			return
		}

		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		responses <- result{body: string(body), err: err}
	}()

	<-entered
	cancel()

	// Shutdown closes the listener first; release the handler only after it
	// is closed so the request is in flight during shutdown.
	for {
		conn, err := net.Dial("tcp", strings.TrimPrefix(url, "http://"))
		if err != nil {
			break
		}

		_ = conn.Close()
	}

	close(release)

	got := <-responses
	require.NoError(t, got.err)
	require.Equal(t, "done", got.body)
	require.NoError(t, wait())
}

func TestMiddlewareKeepsResponseControllerAccess(t *testing.T) {
	flushErr := make(chan error, 1)

	url, _, _, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flushErr <- http.NewResponseController(w).Flush()
	}))

	_, err := get(t, url, "")
	require.NoError(t, err)
	require.NoError(t, <-flushErr)
}

func TestRunReturnsServeFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	err = server.New(http.NotFoundHandler(), slog.New(slog.DiscardHandler)).Run(t.Context(), ln)
	require.ErrorIs(t, err, server.ErrFailedToServe)
}
