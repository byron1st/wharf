package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/byron1st/wharf/internal/adapter/github"
	"github.com/byron1st/wharf/internal/adapter/system"
)

var (
	ErrUpdateNotSupported      = errors.New("service: updates are not supported by this build")
	ErrNoUpdateAvailable       = errors.New("service: no update available")
	ErrUpdateInProgress        = errors.New("service: an update is already in progress")
	ErrInvalidLatestReleaseTag = errors.New("service: latest release tag is not a version")
	ErrReleaseChecksumNotFound = errors.New("service: release checksum not found")
	ErrReleaseChecksumMismatch = errors.New("service: release checksum mismatch")
	ErrFailedToOpenArchive     = errors.New("service: failed to open release archive")
	ErrFailedToReadArchive     = errors.New("service: failed to read release archive")
	ErrBinaryNotInArchive      = errors.New("service: binary not in release archive")
	ErrBinaryTooLarge          = errors.New("service: binary in release archive is too large")
	ErrFailedToReadBinary      = errors.New("service: failed to read binary from release archive")
)

const (
	// binaryName is the executable in a release archive and the name of the
	// archive itself.
	binaryName = "wharf"

	// checksumsName is the release asset that holds the SHA-256 of every other
	// asset, one "<hex>  <name>" line each.
	checksumsName = "checksums.txt"

	// maxBinaryBytes bounds the executable read from an archive, so a
	// malformed one cannot exhaust memory.
	maxBinaryBytes = 512 << 20
)

// versionPattern matches a release version: X.Y.Z without pre-release or
// build metadata.
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// UpdateStatus is what the updater knows about a newer release.
type UpdateStatus struct {
	// Supported is false when this build never checks: it is not a release
	// build, not darwin/arm64, or update_check is off.
	Supported bool
	// Current is the running version.
	Current string
	// Latest is the newest release version, empty until a check succeeds.
	Latest string
	// Available reports whether Latest is newer than Current.
	Available bool
	// CheckedAt is when the last check succeeded, zero until one does.
	CheckedAt time.Time
	// CheckFailed reports whether the most recent check failed; the other
	// fields keep what the last successful check found.
	CheckFailed bool
	ReleaseURL  string
	Notes       string
	// PublishedAt is when Latest was published, zero until a check succeeds.
	PublishedAt time.Time
}

type Updater interface {
	Run(ctx context.Context)
	GetUpdateStatus(ctx context.Context) UpdateStatus
	Check(ctx context.Context) (UpdateStatus, error)
	ApplyUpdate(ctx context.Context) error
}

// UpdaterDeps are what NewUpdater needs.
type UpdaterDeps struct {
	Releases github.Releases
	System   system.Desktop
	// Version is the running version, as buildinfo.Version returns it.
	Version string
	// GOOS and GOARCH select the release archive.
	GOOS   string
	GOARCH string
	// Enabled is the update_check setting.
	Enabled bool
	// Interval is the time between checks.
	Interval time.Duration
	// Restart stops the server and starts the replaced executable. It returns
	// once the stop is requested; the response is still sent.
	Restart func()
	Logger  *slog.Logger
}

type updater struct {
	releases github.Releases
	system   system.Desktop
	goos     string
	goarch   string
	interval time.Duration
	restart  func()
	logger   *slog.Logger

	// applying is held from the start of an update until it fails, so a
	// replaced executable is not replaced again before the restart.
	applying sync.Mutex

	// checking serializes the periodic and on-demand checks.
	checking sync.Mutex

	mu       sync.Mutex
	status   UpdateStatus
	notified string
}

// NewUpdater returns the Updater of a server running deps.Version.
func NewUpdater(deps UpdaterDeps) Updater {
	_, isRelease := parseVersion(deps.Version)

	return &updater{
		releases: deps.Releases,
		system:   deps.System,
		goos:     deps.GOOS,
		goarch:   deps.GOARCH,
		interval: deps.Interval,
		restart:  deps.Restart,
		logger:   deps.Logger,
		status: UpdateStatus{
			Supported: deps.Enabled && isRelease && deps.GOOS == "darwin" && deps.GOARCH == "arm64",
			Current:   deps.Version,
		},
	}
}

// Run checks for a newer release now and then every interval until ctx ends.
// It returns at once when updates are not supported. A failed check is logged
// and keeps the last known status.
func (u *updater) Run(ctx context.Context) {
	if !u.status.Supported {
		return
	}

	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()

	for {
		u.check(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Check checks for a newer release now and returns the resulting status. A
// failed check is logged and reported by the status's CheckFailed, not as an
// error. It fails with ErrUpdateNotSupported when updates are not supported.
func (u *updater) Check(ctx context.Context) (UpdateStatus, error) {
	if !u.status.Supported {
		return UpdateStatus{}, ErrUpdateNotSupported
	}

	u.check(ctx)

	return u.GetUpdateStatus(ctx), nil
}

func (u *updater) check(ctx context.Context) {
	u.checking.Lock()
	defer u.checking.Unlock()

	rel, err := u.releases.LatestRelease(ctx)
	if err == nil {
		err = u.record(ctx, rel)
	}

	if err != nil {
		u.logger.WarnContext(ctx, "failed to check for update", "error", err)

		u.mu.Lock()
		u.status.CheckFailed = true
		u.mu.Unlock()
	}
}

// record stores rel as the latest release and shows a notification the first
// time a newer version is seen.
func (u *updater) record(ctx context.Context, rel github.Release) error {
	latest := strings.TrimPrefix(rel.Tag, "v")

	latestVersion, ok := parseVersion(latest)
	if !ok {
		return fmt.Errorf("%w: tag=%s", ErrInvalidLatestReleaseTag, rel.Tag)
	}

	currentVersion, _ := parseVersion(u.status.Current)

	u.mu.Lock()
	u.status.Latest = latest
	u.status.Available = compareVersions(latestVersion, currentVersion) > 0
	u.status.CheckedAt = time.Now()
	u.status.CheckFailed = false
	u.status.ReleaseURL = rel.URL
	u.status.Notes = rel.Notes
	u.status.PublishedAt = rel.PublishedAt
	notify := u.status.Available && u.notified != latest

	if notify {
		u.notified = latest
	}

	u.mu.Unlock()

	if notify {
		// The notification only points at the web app, which shows the update
		// anyway, so failing to show it is not worth failing the check.
		msg := fmt.Sprintf("Wharf %s is available. Open Wharf to upgrade.", latest)
		if err := u.system.Notify(ctx, "Wharf", msg); err != nil {
			u.logger.WarnContext(ctx, "failed to notify about update", "version", latest, "error", err)
		}
	}

	return nil
}

// GetUpdateStatus returns what the last check found.
func (u *updater) GetUpdateStatus(context.Context) UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.status
}

// ApplyUpdate downloads the latest release archive and checksums.txt,
// verifies the archive's SHA-256, replaces the running executable with the
// archive's wharf, keeping the previous one as wharf.old, and restarts the
// server. On any failure the executable is left as it was.
func (u *updater) ApplyUpdate(ctx context.Context) error {
	status := u.GetUpdateStatus(ctx)
	if !status.Supported {
		return ErrUpdateNotSupported
	}

	if !status.Available {
		return ErrNoUpdateAvailable
	}

	if !u.applying.TryLock() {
		return ErrUpdateInProgress
	}

	if err := u.replace(ctx, status.Latest); err != nil {
		u.applying.Unlock()

		return err
	}

	u.restart()

	return nil
}

func (u *updater) replace(ctx context.Context, version string) error {
	tag := "v" + version
	name := fmt.Sprintf("%s_%s_%s_%s.tar.gz", binaryName, version, u.goos, u.goarch)

	// The small checksums file goes first, so a release without one fails
	// before the archive is downloaded.
	sums, err := u.releases.Download(ctx, tag, checksumsName)
	if err != nil {
		return err
	}

	archive, err := u.releases.Download(ctx, tag, name)
	if err != nil {
		return err
	}

	if err := verifyChecksum(sums, name, archive); err != nil {
		return err
	}

	binary, err := extractBinary(archive)
	if err != nil {
		return err
	}

	return u.system.ReplaceExecutable(ctx, binary)
}

// verifyChecksum checks data against the line for name in sums, which is in
// sha256sum's output format.
func verifyChecksum(sums []byte, name string, data []byte) error {
	for line := range strings.Lines(string(sums)) {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}

		sum := sha256.Sum256(data)
		if !strings.EqualFold(fields[0], hex.EncodeToString(sum[:])) {
			return fmt.Errorf("%w: asset=%s", ErrReleaseChecksumMismatch, name)
		}

		return nil
	}

	return fmt.Errorf("%w: asset=%s", ErrReleaseChecksumNotFound, name)
}

// extractBinary returns the wharf file at the root of a .tar.gz archive.
func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.Join(ErrFailedToOpenArchive, err)
	}

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, ErrBinaryNotInArchive
		}

		if err != nil {
			return nil, errors.Join(ErrFailedToReadArchive, err)
		}

		if hdr.Typeflag != tar.TypeReg || path.Clean(hdr.Name) != binaryName {
			continue
		}

		// The size is checked up front so an oversized entry is not read at all.
		if hdr.Size > maxBinaryBytes {
			return nil, fmt.Errorf("%w: size=%d, limit=%d", ErrBinaryTooLarge, hdr.Size, maxBinaryBytes)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, errors.Join(ErrFailedToReadBinary, err)
		}

		return data, nil
	}
}

// parseVersion splits an X.Y.Z version into its numbers; ok is false for
// anything else, such as "(devel)" or a pseudo-version.
func parseVersion(v string) (numbers [3]int, ok bool) {
	m := versionPattern.FindStringSubmatch(v)
	if m == nil {
		return numbers, false
	}

	for i := range numbers {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return numbers, false
		}

		numbers[i] = n
	}

	return numbers, true
}

func compareVersions(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}

	return 0
}
