package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

var (
	ErrFailedToDecodeSettingsRequest  = errors.New("server: failed to decode settings request")
	ErrSettingsRequestHasTrailingData = errors.New("server: settings request has trailing data")
	ErrSettingsRequestTooLarge        = errors.New("server: settings request is too large")
)

// maxSettingsRequestBytes bounds a settings request body.
const maxSettingsRequestBytes = 1 << 16

// getAtlassianSettings includes the API token: the settings screen shows it on
// request, and withLocalOrigin keeps other sites from reading it.
func getAtlassianSettings(settings service.Settings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := settings.GetAtlassianSettings(r.Context())
		writeJSON(w, http.StatusOK, atlassianSettingsBody{SiteURL: a.SiteURL, Email: a.Email, APIToken: a.APIToken})
	}
}

func saveAtlassianSettings(settings service.Settings, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := decodeAtlassianSettings(w, r)
		if err != nil {
			writeError(w, r, logger, "failed to save atlassian settings", err)
			return
		}

		if err := settings.SaveAtlassianSettings(r.Context(), a); err != nil {
			writeError(w, r, logger, "failed to save atlassian settings", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// checkAtlassianSettings tests the settings in the body, saved or not.
func checkAtlassianSettings(settings service.Settings, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := decodeAtlassianSettings(w, r)
		if err != nil {
			writeError(w, r, logger, "failed to check atlassian settings", err)
			return
		}

		checkedAt, err := settings.CheckAtlassianSettings(r.Context(), a)
		if err != nil {
			writeError(w, r, logger, "failed to check atlassian settings", err)
			return
		}

		writeJSON(w, http.StatusOK, atlassianCheckResponse{CheckedAt: checkedAt})
	}
}

func getAppSettings(settings service.Settings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app := settings.GetAppSettings(r.Context())
		writeJSON(w, http.StatusOK, appSettingsResponse{
			ConfigPath:        app.ConfigPath,
			PendingConfigPath: nonEmpty(app.PendingConfigPath),
			Addr:              app.Addr,
			DBTarget:          app.DBTarget,
			Version:           app.Version,
		})
	}
}

// chooseConfigFile blocks until the user closes the file dialog, then answers
// the pending path. Choosing the current file leaves none.
func chooseConfigFile(settings service.Settings, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// A canceled dialog changes nothing and is not an error to show.
		_, err := settings.ChooseConfigFile(r.Context())
		if err != nil && !errors.Is(err, service.ErrConfigChoiceCanceled) {
			writeError(w, r, logger, "failed to choose config file", err)
			return
		}

		app := settings.GetAppSettings(r.Context())
		writeJSON(w, http.StatusOK, chosenConfigResponse{PendingConfigPath: nonEmpty(app.PendingConfigPath)})
	}
}

func clearPendingConfigFile(settings service.Settings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings.ClearPendingConfigFile(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

func openConfigFile(settings service.Settings, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := settings.OpenConfigFile(r.Context()); err != nil {
			writeError(w, r, logger, "failed to open config file", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// restartServer answers 202 before the server stops; the client reconnects.
func restartServer(settings service.Settings, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := settings.RestartServer(r.Context()); err != nil {
			writeError(w, r, logger, "failed to restart server", err)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// decodeAtlassianSettings reads an atlassianSettingsBody as decodeTask reads a
// task request.
func decodeAtlassianSettings(w http.ResponseWriter, r *http.Request) (service.AtlassianSettings, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSettingsRequestBytes))
	decoder.DisallowUnknownFields()

	var body atlassianSettingsBody

	err := decoder.Decode(&body)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return service.AtlassianSettings{}, errors.Join(ErrSettingsRequestTooLarge, err)
	}

	if err != nil {
		return service.AtlassianSettings{}, errors.Join(ErrFailedToDecodeSettingsRequest, err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return service.AtlassianSettings{}, errors.Join(ErrSettingsRequestHasTrailingData, err)
	}

	return service.AtlassianSettings{SiteURL: body.SiteURL, Email: body.Email, APIToken: body.APIToken}, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
