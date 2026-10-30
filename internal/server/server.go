package server

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

var (
	ErrFailedToServe    = errors.New("server: failed to serve")
	ErrFailedToShutdown = errors.New("server: failed to shut down")
)

const (
	requestIDHeader   = "X-Request-ID"
	readHeaderTimeout = 10 * time.Second
	// shutdownTimeout bounds how long Run waits for in-flight requests, such
	// as a Jira sync paging through the REST search API, after its context ends.
	shutdownTimeout = 30 * time.Second
)

// Server serves an HTTP handler wrapped with the request ID, request logging,
// panic recovery, and local origin middleware.
type Server struct {
	srv *http.Server
}

// New returns a Server that serves handler and logs every request to logger.
// No WriteTimeout is set because a Jira sync request can take long.
func New(handler http.Handler, logger *slog.Logger) *Server {
	return &Server{srv: &http.Server{
		Handler:           withRequestID(withRequestLog(logger, withRecover(logger, withLocalOrigin(handler)))),
		ReadHeaderTimeout: readHeaderTimeout,
	}}
}

// Run serves on ln until ctx ends, then shuts down gracefully: it stops
// accepting connections and waits up to shutdownTimeout for in-flight requests
// before returning nil.
func (s *Server) Run(ctx context.Context, ln net.Listener) error {
	served := make(chan error, 1)

	go func() { served <- s.srv.Serve(ln) }()

	select {
	case err := <-served:
		return errors.Join(ErrFailedToServe, err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		return errors.Join(ErrFailedToShutdown, err)
	}

	return nil
}

// withRequestID reuses the incoming X-Request-ID or generates one, and sets it
// on both the request seen by later handlers and the response.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = rand.Text()
			// Clone so the incoming request is not modified.
			r = r.Clone(r.Context())
			r.Header.Set(requestIDHeader, id)
		}

		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}

func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		logger.InfoContext(
			r.Context(),
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"request_id", r.Header.Get(requestIDHeader),
		)
	})
}

// withRecover turns a panic into a 500 response. http.ErrAbortHandler is
// re-panicked so net/http aborts the response as the handler intended.
func withRecover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}

			// http.ErrAbortHandler는 핸들러가 응답을 의도적으로 중단하려고 던지는
			// panic이므로 500 응답으로 바꾸지 않고 다시 던진다. 그러면 net/http가
			// stack trace 로그 없이 연결을 끊어 핸들러의 의도대로 응답을 중단한다.
			if v == http.ErrAbortHandler {
				panic(v)
			}

			logger.ErrorContext(
				r.Context(),
				"panic",
				"request_id", r.Header.Get(requestIDHeader),
				"panic", v,
				"stack", string(debug.Stack()),
			)

			var err error

			switch pv := v.(type) {
			case error:
				err = pv
			case string:
				err = errors.New(pv)
			}

			writeInternalError(w, err)
		}()

		next.ServeHTTP(w, r)
	})
}

// withLocalOrigin answers 403 unless the request is for a loopback host name
// and, when it has an Origin, comes from that same host. wharf has no
// authentication and returns the Atlassian token, so this keeps other web
// sites from reaching it through the user's browser: a Host check stops DNS
// rebinding, and an Origin check stops cross-site requests such as a bodiless
// POST, which the browser sends without a CORS preflight.
func withLocalOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			writeJSON(
				w,
				http.StatusForbidden,
				errorResponse{Error: "server: host is not local", Code: "forbidden_host"},
			)

			return
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				writeJSON(
					w,
					http.StatusForbidden,
					errorResponse{Error: "server: cross-origin request", Code: "forbidden_origin"},
				)

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost reports whether hostport, with or without a port, names
// localhost or a loopback IP address.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}

	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))

	return ip != nil && ip.IsLoopback()
}

// statusRecorder captures the response status for the request log; it stays
// 200 when the handler writes without calling WriteHeader.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records status and forwards it.
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap returns the wrapped ResponseWriter for http.ResponseController.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
