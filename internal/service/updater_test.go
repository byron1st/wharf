package service_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/github"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/service"
)

const archiveName = "wharf_0.2.0_darwin_arm64.tar.gz"

var release = github.Release{
	Tag:   "v0.2.0",
	URL:   "https://github.com/byron1st/wharf/releases/tag/v0.2.0",
	Notes: "notes",
}

type updaterFixture struct {
	releases  *mocks.MockReleases
	desktop   *mocks.MockDesktop
	restarted atomic.Int32
}

func newUpdaterFixture(t *testing.T) *updaterFixture {
	t.Helper()

	return &updaterFixture{releases: mocks.NewMockReleases(t), desktop: mocks.NewMockDesktop(t)}
}

func (f *updaterFixture) updater(version, goos string, enabled bool, interval time.Duration) service.Updater {
	return service.NewUpdater(service.UpdaterDeps{
		Releases: f.releases,
		System:   f.desktop,
		Version:  version,
		GOOS:     goos,
		GOARCH:   "arm64",
		Enabled:  enabled,
		Interval: interval,
		Restart:  func() { f.restarted.Add(1) },
		Logger:   slog.New(slog.DiscardHandler),
	})
}

// runOnce starts u.Run, waits for its first check to finish and stops it.
func runOnce(t *testing.T, u service.Updater, done func() bool) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})

	go func() {
		u.Run(ctx)
		close(stopped)
	}()

	require.Eventually(t, done, 5*time.Second, time.Millisecond)
	cancel()
	<-stopped
}

func checked(u service.Updater) func() bool {
	return func() bool { return !u.GetUpdateStatus(context.Background()).CheckedAt.IsZero() }
}

func TestUpdaterReportsNewerRelease(t *testing.T) {
	f := newUpdaterFixture(t)
	f.releases.EXPECT().LatestRelease(mock.Anything).Return(release, nil).Once()
	f.desktop.EXPECT().
		Notify(mock.Anything, "Wharf", "Wharf 0.2.0 is available. Open Wharf Settings to update.").
		Return(nil).
		Once()

	u := f.updater("0.1.0", "darwin", true, time.Hour)
	runOnce(t, u, checked(u))

	status := u.GetUpdateStatus(t.Context())
	require.WithinDuration(t, time.Now(), status.CheckedAt, time.Minute)
	status.CheckedAt = time.Time{}
	require.Equal(t, service.UpdateStatus{
		Supported:  true,
		Current:    "0.1.0",
		Latest:     "0.2.0",
		Available:  true,
		ReleaseURL: release.URL,
		Notes:      "notes",
	}, status)
}

func TestUpdaterComparesVersions(t *testing.T) {
	for current, available := range map[string]bool{
		"0.1.9":  true,
		"0.1.10": true,
		"0.2.0":  false,
		"0.10.0": false,
		"1.0.0":  false,
	} {
		t.Run(current, func(t *testing.T) {
			f := newUpdaterFixture(t)
			f.releases.EXPECT().LatestRelease(mock.Anything).Return(release, nil).Once()

			if available {
				f.desktop.EXPECT().Notify(mock.Anything, "Wharf", mock.Anything).Return(nil).Once()
			}

			u := f.updater(current, "darwin", true, time.Hour)
			runOnce(t, u, checked(u))

			status := u.GetUpdateStatus(t.Context())
			require.Equal(t, "0.2.0", status.Latest)
			require.Equal(t, available, status.Available)
		})
	}
}

func TestUpdaterNotifiesOncePerVersion(t *testing.T) {
	f := newUpdaterFixture(t)

	var calls atomic.Int32

	f.releases.EXPECT().LatestRelease(mock.Anything).
		RunAndReturn(func(context.Context) (github.Release, error) {
			calls.Add(1)
			return release, nil
		})
	// A failing notification is logged and not retried for the same version.
	f.desktop.EXPECT().Notify(mock.Anything, "Wharf", mock.Anything).Return(errors.New("no osascript")).Once()

	u := f.updater("0.1.0", "darwin", true, time.Millisecond)
	runOnce(t, u, func() bool { return calls.Load() >= 3 })

	require.True(t, u.GetUpdateStatus(t.Context()).Available)
}

func TestUpdaterKeepsStatusWhenCheckFails(t *testing.T) {
	f := newUpdaterFixture(t)

	var calls atomic.Int32

	f.releases.EXPECT().LatestRelease(mock.Anything).
		RunAndReturn(func(context.Context) (github.Release, error) {
			switch calls.Add(1) {
			case 1:
				return release, nil
			case 2:
				return github.Release{}, github.ErrUnexpectedLatestReleaseStatus
			default:
				return github.Release{Tag: "nightly"}, nil
			}
		})
	f.desktop.EXPECT().Notify(mock.Anything, "Wharf", mock.Anything).Return(nil).Once()

	u := f.updater("0.1.0", "darwin", true, time.Millisecond)
	runOnce(t, u, func() bool { return calls.Load() >= 3 })

	status := u.GetUpdateStatus(t.Context())
	require.Equal(t, "0.2.0", status.Latest)
	require.True(t, status.Available)
}

func TestUpdaterUnsupportedNeverChecks(t *testing.T) {
	for name, tc := range map[string]struct {
		version string
		goos    string
		enabled bool
	}{
		"devel build":    {version: "(devel)", goos: "darwin", enabled: true},
		"pseudo-version": {version: "0.1.1-0.20261109060017-7897bbd61e4c", goos: "darwin", enabled: true},
		"dirty build":    {version: "0.1.0+dirty", goos: "darwin", enabled: true},
		"not darwin":     {version: "0.1.0", goos: "linux", enabled: true},
		"check disabled": {version: "0.1.0", goos: "darwin", enabled: false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newUpdaterFixture(t)
			u := f.updater(tc.version, tc.goos, tc.enabled, time.Millisecond)

			// Run returns at once without calling the mocks, which fail on any call.
			u.Run(t.Context())

			require.Equal(t, service.UpdateStatus{Current: tc.version}, u.GetUpdateStatus(t.Context()))
			require.ErrorIs(t, u.ApplyUpdate(t.Context()), service.ErrUpdateNotSupported)

			_, err := u.Check(t.Context())
			require.ErrorIs(t, err, service.ErrUpdateNotSupported)
		})
	}
}

func TestUpdaterCheck(t *testing.T) {
	f := newUpdaterFixture(t)
	published := time.Date(2026, 11, 10, 9, 12, 17, 0, time.UTC)
	rel := release
	rel.PublishedAt = published
	f.releases.EXPECT().LatestRelease(mock.Anything).Return(rel, nil).Once()
	f.desktop.EXPECT().Notify(mock.Anything, "Wharf", mock.Anything).Return(nil).Once()

	u := f.updater("0.1.0", "darwin", true, time.Hour)

	status, err := u.Check(t.Context())
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), status.CheckedAt, time.Minute)
	status.CheckedAt = time.Time{}
	require.Equal(t, service.UpdateStatus{
		Supported:   true,
		Current:     "0.1.0",
		Latest:      "0.2.0",
		Available:   true,
		ReleaseURL:  release.URL,
		Notes:       "notes",
		PublishedAt: published,
	}, status)
}

func TestUpdaterCheckFailureKeepsLastResult(t *testing.T) {
	f := newUpdaterFixture(t)
	f.releases.EXPECT().LatestRelease(mock.Anything).Return(release, nil).Once()
	f.releases.EXPECT().
		LatestRelease(mock.Anything).
		Return(github.Release{}, github.ErrUnexpectedLatestReleaseStatus).
		Once()
	f.releases.EXPECT().LatestRelease(mock.Anything).Return(release, nil).Once()
	f.desktop.EXPECT().Notify(mock.Anything, "Wharf", mock.Anything).Return(nil).Once()

	u := f.updater("0.1.0", "darwin", true, time.Hour)

	first, err := u.Check(t.Context())
	require.NoError(t, err)
	require.False(t, first.CheckFailed)

	failed, err := u.Check(t.Context())
	require.NoError(t, err)
	require.True(t, failed.CheckFailed)
	require.Equal(t, "0.2.0", failed.Latest)
	require.True(t, failed.Available)
	require.Equal(t, first.CheckedAt, failed.CheckedAt)
	require.True(t, u.GetUpdateStatus(t.Context()).CheckFailed)

	recovered, err := u.Check(t.Context())
	require.NoError(t, err)
	require.False(t, recovered.CheckFailed)
}

func TestUpdaterCanceledCheckIsNotAFailure(t *testing.T) {
	f := newUpdaterFixture(t)

	ctx, cancel := context.WithCancel(t.Context())
	f.releases.EXPECT().LatestRelease(mock.Anything).
		RunAndReturn(func(ctx context.Context) (github.Release, error) {
			cancel()
			return github.Release{}, ctx.Err()
		}).
		Once()

	u := f.updater("0.1.0", "darwin", true, time.Hour)

	status, err := u.Check(ctx)
	require.NoError(t, err)
	require.False(t, status.CheckFailed)
}

func TestUpdaterCheckFailureBeforeAnySuccess(t *testing.T) {
	f := newUpdaterFixture(t)
	f.releases.EXPECT().
		LatestRelease(mock.Anything).
		Return(github.Release{}, github.ErrUnexpectedLatestReleaseStatus).
		Once()

	u := f.updater("0.1.0", "darwin", true, time.Hour)
	runOnce(t, u, func() bool { return u.GetUpdateStatus(context.Background()).CheckFailed })

	status := u.GetUpdateStatus(t.Context())
	require.True(t, status.CheckedAt.IsZero())
	require.Empty(t, status.Latest)
}

// archive returns a .tar.gz holding files, in order.
func archive(t *testing.T, files map[string]string, order ...string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, name := range order {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(files[name]))}))
		_, err := tw.Write([]byte(files[name]))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}

func sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// availableUpdater returns an updater on 0.1.0 that has seen release.
func availableUpdater(t *testing.T, f *updaterFixture) service.Updater {
	t.Helper()

	f.releases.EXPECT().LatestRelease(mock.Anything).Return(release, nil).Once()
	f.desktop.EXPECT().Notify(mock.Anything, "Wharf", mock.Anything).Return(nil).Once()

	u := f.updater("0.1.0", "darwin", true, time.Hour)
	runOnce(t, u, checked(u))

	return u
}

func TestApplyUpdate(t *testing.T) {
	f := newUpdaterFixture(t)
	u := availableUpdater(t, f)

	data := archive(t, map[string]string{"README.md": "readme", "wharf": "new binary"}, "README.md", "wharf")
	sums := "0000  other.tar.gz\n" + sum(data) + "  " + archiveName + "\n"

	f.releases.EXPECT().Download(mock.Anything, "v0.2.0", archiveName).Return(data, nil).Once()
	f.releases.EXPECT().Download(mock.Anything, "v0.2.0", "checksums.txt").Return([]byte(sums), nil).Once()
	f.desktop.EXPECT().ReplaceExecutable(mock.Anything, []byte("new binary")).Return(nil).Once()

	require.NoError(t, u.ApplyUpdate(t.Context()))
	require.Equal(t, int32(1), f.restarted.Load())

	// The executable is replaced, so a second apply must wait for the restart.
	require.ErrorIs(t, u.ApplyUpdate(t.Context()), service.ErrUpdateInProgress)
}

func TestApplyUpdateLeavesExecutableOnFailure(t *testing.T) {
	good := archive(t, map[string]string{"wharf": "new binary"}, "wharf")
	noBinary := archive(t, map[string]string{"bin/wharf": "nested", "wharf.txt": "x"}, "bin/wharf", "wharf.txt")
	downloadErr := errors.New("download failed")

	for name, tc := range map[string]struct {
		archive     []byte
		archiveErr  error
		sums        string
		sumsErr     error
		want        error
		skipArchive bool
	}{
		"checksums download fails": {sumsErr: downloadErr, want: downloadErr, skipArchive: true},
		"archive download fails":   {archiveErr: downloadErr, sums: sum(good) + "  " + archiveName + "\n", want: downloadErr},
		"checksum missing":         {archive: good, sums: sum(good) + "  other.tar.gz\n", want: service.ErrReleaseChecksumNotFound},
		"checksum mismatch": {
			archive: good, sums: sum([]byte("tampered")) + "  " + archiveName + "\n", want: service.ErrReleaseChecksumMismatch,
		},
		"not gzip": {
			archive: []byte("plain"), sums: sum([]byte("plain")) + "  " + archiveName + "\n", want: service.ErrFailedToOpenArchive,
		},
		"binary missing": {
			archive: noBinary, sums: sum(noBinary) + "  " + archiveName + "\n", want: service.ErrBinaryNotInArchive,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newUpdaterFixture(t)
			u := availableUpdater(t, f)

			f.releases.EXPECT().
				Download(mock.Anything, "v0.2.0", "checksums.txt").
				Return([]byte(tc.sums), tc.sumsErr).
				Once()

			// A failed checksums download stops before the archive.
			if !tc.skipArchive {
				f.releases.EXPECT().
					Download(mock.Anything, "v0.2.0", archiveName).
					Return(tc.archive, tc.archiveErr).
					Once()
			}

			// ReplaceExecutable is not expected: the mock fails if it is called.
			require.ErrorIs(t, u.ApplyUpdate(t.Context()), tc.want)
			require.Zero(t, f.restarted.Load())
		})
	}
}

func TestApplyUpdateCanRetryAfterFailure(t *testing.T) {
	f := newUpdaterFixture(t)
	u := availableUpdater(t, f)

	data := archive(t, map[string]string{"wharf": "new binary"}, "wharf")
	sums := sum(data) + "  " + archiveName + "\n"
	replaceErr := errors.New("read-only dir")

	f.releases.EXPECT().Download(mock.Anything, "v0.2.0", archiveName).Return(data, nil).Twice()
	f.releases.EXPECT().Download(mock.Anything, "v0.2.0", "checksums.txt").Return([]byte(sums), nil).Twice()
	f.desktop.EXPECT().ReplaceExecutable(mock.Anything, []byte("new binary")).Return(replaceErr).Once()
	f.desktop.EXPECT().ReplaceExecutable(mock.Anything, []byte("new binary")).Return(nil).Once()

	require.ErrorIs(t, u.ApplyUpdate(t.Context()), replaceErr)
	require.NoError(t, u.ApplyUpdate(t.Context()))
	require.Equal(t, int32(1), f.restarted.Load())
}

func TestApplyUpdateWithoutUpdate(t *testing.T) {
	f := newUpdaterFixture(t)
	u := f.updater("0.1.0", "darwin", true, time.Hour)

	// Before any check there is nothing to apply.
	require.ErrorIs(t, u.ApplyUpdate(t.Context()), service.ErrNoUpdateAvailable)
}

func TestApplyUpdateIsExclusive(t *testing.T) {
	f := newUpdaterFixture(t)
	u := availableUpdater(t, f)

	data := archive(t, map[string]string{"wharf": "new binary"}, "wharf")
	sums := sum(data) + "  " + archiveName + "\n"
	downloading := make(chan struct{})
	proceed := make(chan struct{})

	f.releases.EXPECT().Download(mock.Anything, "v0.2.0", archiveName).
		RunAndReturn(func(context.Context, string, string) ([]byte, error) {
			close(downloading)
			<-proceed

			return data, nil
		}).Once()
	f.releases.EXPECT().Download(mock.Anything, "v0.2.0", "checksums.txt").Return([]byte(sums), nil).Once()
	f.desktop.EXPECT().ReplaceExecutable(mock.Anything, []byte("new binary")).Return(nil).Once()

	first := make(chan error, 1)

	go func() { first <- u.ApplyUpdate(context.Background()) }()

	<-downloading
	require.ErrorIs(t, u.ApplyUpdate(t.Context()), service.ErrUpdateInProgress)
	close(proceed)
	require.NoError(t, <-first)
	require.Equal(t, int32(1), f.restarted.Load())
}
