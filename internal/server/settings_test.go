package server_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
)

// serveSettings is serveBody for a handler backed by settings.
func serveSettings(t *testing.T, settings service.Settings, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequestWithContext(t.Context(), method, path, reader)
	rec := httptest.NewRecorder()

	var logs bytes.Buffer
	server.NewHandler(mocks.NewMockService(t), settings, mocks.NewMockUpdater(t), webApp, slog.New(slog.NewJSONHandler(&logs, nil))).
		ServeHTTP(rec, req)

	return rec
}

var atlassianSettings = service.AtlassianSettings{
	SiteURL:  "https://example.atlassian.net",
	Email:    "me@example.com",
	APIToken: "secret-token",
}

const atlassianSettingsJSON = `{"site_url":"https://example.atlassian.net","email":"me@example.com",` +
	`"api_token":"secret-token"}`

func TestAtlassianSettingsRoutes(t *testing.T) {
	checkedAt := time.Date(2026, 10, 26, 5, 1, 0, 0, time.UTC)

	settings := mocks.NewMockSettings(t)
	settings.EXPECT().GetAtlassianSettings(mock.Anything).Return(atlassianSettings).Once()
	settings.EXPECT().SaveAtlassianSettings(mock.Anything, atlassianSettings).Return(nil).Once()
	settings.EXPECT().CheckAtlassianSettings(mock.Anything, atlassianSettings).Return(checkedAt, nil).Once()

	rec := serveSettings(t, settings, http.MethodGet, "/api/settings/atlassian", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, atlassianSettingsJSON, rec.Body.String())

	rec = serveSettings(t, settings, http.MethodPut, "/api/settings/atlassian", atlassianSettingsJSON)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/atlassian/test", atlassianSettingsJSON)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"checked_at":"2026-10-26T05:01:00Z"}`, rec.Body.String())
}

func TestAtlassianSettingsRoutesRejectInvalidBody(t *testing.T) {
	for name, tc := range map[string]struct {
		body, want string
		status     int
	}{
		"malformed":     {`{"site_url":`, "server: failed to decode settings request", http.StatusBadRequest},
		"unknown field": {`{"token":"x"}`, "server: failed to decode settings request", http.StatusBadRequest},
		"trailing":      {`{} {}`, "server: settings request has trailing data", http.StatusBadRequest},
		"too large": {
			`{"api_token":"` + strings.Repeat("x", 1<<16) + `"}`,
			"server: settings request is too large",
			http.StatusRequestEntityTooLarge,
		},
	} {
		for _, req := range []struct{ method, path string }{
			{http.MethodPut, "/api/settings/atlassian"},
			{http.MethodPost, "/api/settings/atlassian/test"},
		} {
			t.Run(name+" "+req.method, func(t *testing.T) {
				// No expectation: the mock fails the test if it is called.
				rec := serveSettings(t, mocks.NewMockSettings(t), req.method, req.path, tc.body)

				require.Equal(t, tc.status, rec.Code)
				require.Contains(t, rec.Body.String(), tc.want)
			})
		}
	}
}

func TestAtlassianSettingsRoutesMapErrors(t *testing.T) {
	settings := mocks.NewMockSettings(t)
	settings.EXPECT().
		SaveAtlassianSettings(mock.Anything, mock.Anything).
		Return(service.ErrInvalidAtlassianSettings).
		Once()
	settings.EXPECT().
		CheckAtlassianSettings(mock.Anything, mock.Anything).
		Return(time.Time{}, service.ErrAtlassianUnauthorized).
		Once()

	rec := serveSettings(t, settings, http.MethodPut, "/api/settings/atlassian", `{}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/atlassian/test", `{}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"atlassian_unauthorized"`)
}

func TestAppSettingsRoutes(t *testing.T) {
	app := service.AppSettings{ConfigPath: "/c.toml", Addr: ":8080", DBPath: "/w.db", Version: "v1"}
	pending := app
	pending.PendingConfigPath = "/other.toml"

	settings := mocks.NewMockSettings(t)
	settings.EXPECT().GetAppSettings(mock.Anything).Return(app).Once()
	settings.EXPECT().ChooseConfigFile(mock.Anything).Return("/other.toml", nil).Once()
	settings.EXPECT().GetAppSettings(mock.Anything).Return(pending).Once()
	settings.EXPECT().ClearPendingConfigFile(mock.Anything).Once()
	settings.EXPECT().OpenConfigFile(mock.Anything).Return(nil).Once()
	settings.EXPECT().RestartServer(mock.Anything).Return(nil).Once()

	rec := serveSettings(t, settings, http.MethodGet, "/api/settings/app", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"config_path":"/c.toml","pending_config_path":null,"addr":":8080","db_path":"/w.db",
		"version":"v1"}`, rec.Body.String())

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/app/config/choose", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"pending_config_path":"/other.toml"}`, rec.Body.String())

	rec = serveSettings(t, settings, http.MethodDelete, "/api/settings/app/config/pending", "")
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/app/config/open", "")
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/app/restart", "")
	require.Equal(t, http.StatusAccepted, rec.Code)
}

func TestAppSettingsRoutesErrors(t *testing.T) {
	settings := mocks.NewMockSettings(t)
	// A canceled dialog answers the pending path unchanged.
	settings.EXPECT().ChooseConfigFile(mock.Anything).Return("", service.ErrConfigChoiceCanceled).Once()
	settings.EXPECT().GetAppSettings(mock.Anything).Return(service.AppSettings{ConfigPath: "/c.toml"}).Once()
	settings.EXPECT().ChooseConfigFile(mock.Anything).Return("", service.ErrUnsupportedOS).Once()
	settings.EXPECT().OpenConfigFile(mock.Anything).Return(errors.New("open crashed")).Once()
	settings.EXPECT().RestartServer(mock.Anything).Return(service.ErrInvalidConfigFile).Once()

	rec := serveSettings(t, settings, http.MethodPost, "/api/settings/app/config/choose", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"pending_config_path":null}`, rec.Body.String())

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/app/config/choose", "")
	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"unsupported_os"`)

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/app/config/open", "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = serveSettings(t, settings, http.MethodPost, "/api/settings/app/restart", "")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"invalid_config_file"`)
}
