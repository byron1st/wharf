package server

import (
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

func getUpdateStatus(updater service.Updater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, toUpdateStatusResponse(updater.GetUpdateStatus(r.Context())))
	}
}

// checkUpdate checks for a newer release now. A failed check still answers
// 200, with check_failed set.
func checkUpdate(updater service.Updater, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := updater.Check(r.Context())
		if err != nil {
			writeError(w, r, logger, "failed to check for update", err)
			return
		}

		writeJSON(w, http.StatusOK, toUpdateStatusResponse(s))
	}
}

func toUpdateStatusResponse(s service.UpdateStatus) updateStatusResponse {
	resp := updateStatusResponse{
		Supported:   s.Supported,
		Current:     s.Current,
		Latest:      nonEmpty(s.Latest),
		Available:   s.Available,
		ReleaseURL:  nonEmpty(s.ReleaseURL),
		Notes:       s.Notes,
		CheckFailed: s.CheckFailed,
	}
	if !s.CheckedAt.IsZero() {
		resp.CheckedAt = &s.CheckedAt
	}

	if !s.PublishedAt.IsZero() {
		resp.PublishedAt = &s.PublishedAt
	}

	return resp
}

// applyUpdate answers 202 once the new executable is in place; the server
// then restarts into it and the client reconnects.
func applyUpdate(updater service.Updater, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := updater.ApplyUpdate(r.Context()); err != nil {
			writeError(w, r, logger, "failed to apply update", err)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}
