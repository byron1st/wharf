package server

import (
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

func getUpdateStatus(updater service.Updater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := updater.GetUpdateStatus(r.Context())

		resp := updateStatusResponse{
			Supported:  s.Supported,
			Current:    s.Current,
			Latest:     nonEmpty(s.Latest),
			Available:  s.Available,
			ReleaseURL: nonEmpty(s.ReleaseURL),
			Notes:      s.Notes,
		}
		if !s.CheckedAt.IsZero() {
			resp.CheckedAt = &s.CheckedAt
		}

		writeJSON(w, http.StatusOK, resp)
	}
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
