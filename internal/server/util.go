package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

// errorStatuses maps the errors a request can cause to their status. Any other
// error is a 500.
var errorStatuses = []struct {
	err    error
	status int
}{
	{err: ErrFailedToDecodeTaskRequest, status: http.StatusBadRequest},
	{err: ErrTaskRequestHasTrailingData, status: http.StatusBadRequest},
	{err: service.ErrEmptyTaskTitle, status: http.StatusBadRequest},
	{err: service.ErrUnknownTaskStatus, status: http.StatusBadRequest},
	{err: service.ErrTaskReferencesItself, status: http.StatusBadRequest},
	{err: service.ErrTaskNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskToUpdateNotFound, status: http.StatusNotFound},
	{err: service.ErrLocalTaskToUpdateNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskToDeleteNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskNotLocal, status: http.StatusConflict},
}

// writeError logs err with msg and writes it with the status errorStatuses
// gives it, or as an internal error.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, msg string, err error) {
	requestID := r.Header.Get(requestIDHeader)

	for _, e := range errorStatuses {
		if errors.Is(err, e.err) {
			logger.InfoContext(r.Context(), msg, "request_id", requestID, "status", e.status, "error", err)
			writeJSON(w, e.status, errorResponse{Error: err.Error()})

			return
		}
	}

	logger.ErrorContext(r.Context(), msg, "request_id", requestID, "error", err)
	writeInternalError(w, err)
}

// writeInternalError hides the cause from the client; it is logged instead.
func writeInternalError(w http.ResponseWriter, err error) {
	msg := "internal server error"
	if err != nil {
		msg = err.Error()
	}

	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// The status is already sent, so an encoding or write failure, such as a
	// closed connection, cannot be reported to the client.
	_ = json.NewEncoder(w).Encode(body)
}
