package server_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
)

func serveUpdate(t *testing.T, updater service.Updater, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	var logs bytes.Buffer

	rec := httptest.NewRecorder()
	server.NewHandler(mocks.NewMockService(t), mocks.NewMockSettings(t), updater, webApp,
		slog.New(slog.NewJSONHandler(&logs, nil))).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, nil))

	return rec
}

func TestGetUpdateStatus(t *testing.T) {
	updater := mocks.NewMockUpdater(t)
	updater.EXPECT().GetUpdateStatus(mock.Anything).Return(service.UpdateStatus{
		Supported:   true,
		Current:     "0.1.0",
		Latest:      "0.2.0",
		Available:   true,
		CheckedAt:   time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC),
		ReleaseURL:  "https://github.com/byron1st/wharf/releases/tag/v0.2.0",
		Notes:       "notes",
		CheckFailed: true,
		PublishedAt: time.Date(2026, 11, 8, 9, 12, 17, 0, time.UTC),
	}).Once()

	rec := serveUpdate(t, updater, http.MethodGet, "/api/update")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"supported": true,
		"current": "0.1.0",
		"latest": "0.2.0",
		"available": true,
		"checked_at": "2026-11-09T07:00:00Z",
		"release_url": "https://github.com/byron1st/wharf/releases/tag/v0.2.0",
		"notes": "notes",
		"check_failed": true,
		"published_at": "2026-11-08T09:12:17Z"
	}`, rec.Body.String())
}

func TestGetUpdateStatusBeforeCheck(t *testing.T) {
	updater := mocks.NewMockUpdater(t)
	updater.EXPECT().GetUpdateStatus(mock.Anything).Return(service.UpdateStatus{Current: "(devel)"}).Once()

	rec := serveUpdate(t, updater, http.MethodGet, "/api/update")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"supported": false,
		"current": "(devel)",
		"latest": null,
		"available": false,
		"checked_at": null,
		"release_url": null,
		"notes": "",
		"check_failed": false,
		"published_at": null
	}`, rec.Body.String())
}

func TestCheckUpdate(t *testing.T) {
	updater := mocks.NewMockUpdater(t)
	updater.EXPECT().Check(mock.Anything).Return(service.UpdateStatus{
		Supported:   true,
		Current:     "0.2.0",
		Latest:      "0.2.0",
		CheckedAt:   time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC),
		CheckFailed: true,
	}, nil).Once()

	rec := serveUpdate(t, updater, http.MethodPost, "/api/update/check")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"supported": true,
		"current": "0.2.0",
		"latest": "0.2.0",
		"available": false,
		"checked_at": "2026-11-09T07:00:00Z",
		"release_url": null,
		"notes": "",
		"check_failed": true,
		"published_at": null
	}`, rec.Body.String())
}

func TestCheckUpdateNotSupported(t *testing.T) {
	updater := mocks.NewMockUpdater(t)
	updater.EXPECT().Check(mock.Anything).Return(service.UpdateStatus{}, service.ErrUpdateNotSupported).Once()

	rec := serveUpdate(t, updater, http.MethodPost, "/api/update/check")
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "update_not_supported")
}

func TestApplyUpdate(t *testing.T) {
	updater := mocks.NewMockUpdater(t)
	updater.EXPECT().ApplyUpdate(mock.Anything).Return(nil).Once()

	rec := serveUpdate(t, updater, http.MethodPost, "/api/update/apply")
	require.Equal(t, http.StatusAccepted, rec.Code)
}

func TestApplyUpdateMapsErrors(t *testing.T) {
	for err, want := range map[error]struct {
		status int
		code   string
	}{
		service.ErrUpdateNotSupported:      {http.StatusConflict, "update_not_supported"},
		service.ErrNoUpdateAvailable:       {http.StatusConflict, "no_update_available"},
		service.ErrUpdateInProgress:        {http.StatusConflict, "update_in_progress"},
		service.ErrReleaseChecksumNotFound: {http.StatusBadGateway, "invalid_release"},
		service.ErrReleaseChecksumMismatch: {http.StatusBadGateway, "invalid_release"},
		service.ErrFailedToOpenArchive:     {http.StatusBadGateway, "invalid_release"},
		service.ErrFailedToReadArchive:     {http.StatusBadGateway, "invalid_release"},
		service.ErrBinaryNotInArchive:      {http.StatusBadGateway, "invalid_release"},
		service.ErrBinaryTooLarge:          {http.StatusBadGateway, "invalid_release"},
		service.ErrFailedToReadBinary:      {http.StatusBadGateway, "invalid_release"},
		errors.New("download failed"):      {http.StatusInternalServerError, ""},
	} {
		t.Run(err.Error(), func(t *testing.T) {
			updater := mocks.NewMockUpdater(t)
			updater.EXPECT().ApplyUpdate(mock.Anything).Return(err).Once()

			rec := serveUpdate(t, updater, http.MethodPost, "/api/update/apply")
			require.Equal(t, want.status, rec.Code)

			if want.code != "" {
				require.Contains(t, rec.Body.String(), `"code":"`+want.code+`"`)
			}
		})
	}
}
