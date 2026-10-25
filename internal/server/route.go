package server

import (
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

// NewHandler returns the API routes backed by svc. It expects the request ID
// middleware of Server to run first so logs carry the request ID.
func NewHandler(svc service.Service, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("POST /tasks/jira", syncJiraTasks(svc, logger))

	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// syncJiraTasks maps every service error to 500, since none is caused by the
// request yet.
func syncJiraTasks(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)

		result, err := svc.SyncJiraTasks(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "failed to sync jira tasks", "request_id", requestID, "error", err)
			writeInternalError(w, err)

			return
		}

		if result.Partial != nil {
			logger.WarnContext(
				r.Context(),
				"synced jira tasks partially",
				"request_id", requestID,
				"upserted", result.Upserted,
				"error", result.Partial,
			)
		}

		writeJSON(w, http.StatusOK, syncResponse{Upserted: result.Upserted, Partial: result.Partial != nil})
	}
}
