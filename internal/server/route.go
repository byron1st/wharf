package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

type syncResponse struct {
	Upserted int  `json:"upserted"`
	Partial  bool `json:"partial"`
}

type errorResponse struct {
	Error string `json:"error"`
}

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
			writeInternalError(w)

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

// writeInternalError hides the cause from the client; it is logged instead.
func writeInternalError(w http.ResponseWriter) {
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// The status is already sent, so an encoding or write failure, such as a
	// closed connection, cannot be reported to the client.
	_ = json.NewEncoder(w).Encode(body)
}
