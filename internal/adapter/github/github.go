// Package github reads the releases of wharf's GitHub repository.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrFailedToCreateReleaseRequest  = errors.New("github: failed to create latest release request")
	ErrFailedToRequestLatestRelease  = errors.New("github: failed to request latest release")
	ErrUnexpectedLatestReleaseStatus = errors.New("github: unexpected latest release status")
	ErrFailedToDecodeLatestRelease   = errors.New("github: failed to decode latest release")
	ErrFailedToCreateDownloadRequest = errors.New("github: failed to create download request")
	ErrFailedToDownloadAsset         = errors.New("github: failed to download release asset")
	ErrUnexpectedDownloadStatus      = errors.New("github: unexpected release asset status")
	ErrFailedToReadAsset             = errors.New("github: failed to read release asset")
	ErrAssetTooLarge                 = errors.New("github: release asset is too large")
)

const (
	apiBaseURL      = "https://api.github.com/repos/byron1st/wharf"
	downloadBaseURL = "https://github.com/byron1st/wharf/releases/download"

	// requestTimeout bounds one request, a download included, on top of ctx.
	requestTimeout = 5 * time.Minute

	// maxReleaseBytes bounds the latest release JSON, notes included.
	maxReleaseBytes = 1 << 20

	// maxAssetBytes bounds a downloaded asset; a release archive is tens of MiB.
	maxAssetBytes = 256 << 20
)

// Release is a published GitHub release.
type Release struct {
	// Tag is the release tag, such as "v0.2.0".
	Tag string
	// URL is the release page.
	URL string
	// Notes is the release body in Markdown.
	Notes string
	// PublishedAt is when the release was published.
	PublishedAt time.Time
}

type Releases interface {
	LatestRelease(ctx context.Context) (Release, error)
	Download(ctx context.Context, tag, name string) ([]byte, error)
}

type client struct {
	http         *http.Client
	apiBase      string
	downloadBase string
	token        string
}

// NewClient returns Releases for github.com/byron1st/wharf. A non-empty token
// authenticates the latest release request, which lifts the API rate limit
// from per IP address to per user. Downloads never carry it, so it does not
// follow their redirect to the storage host.
func NewClient(token string) Releases {
	return newClient(apiBaseURL, downloadBaseURL, token)
}

func newClient(apiBase, downloadBase, token string) Releases {
	return &client{
		http:         &http.Client{Timeout: requestTimeout},
		apiBase:      apiBase,
		downloadBase: downloadBase,
		token:        token,
	}
}

// LatestRelease returns the newest release that is neither a draft nor a
// pre-release.
func (c *client) LatestRelease(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/releases/latest", nil)
	if err != nil {
		return Release{}, errors.Join(ErrFailedToCreateReleaseRequest, err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Release{}, errors.Join(ErrFailedToRequestLatestRelease, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("%w: status=%d", ErrUnexpectedLatestReleaseStatus, resp.StatusCode)
	}

	var body struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBytes)).Decode(&body); err != nil {
		return Release{}, errors.Join(ErrFailedToDecodeLatestRelease, err)
	}

	return Release{Tag: body.TagName, URL: body.HTMLURL, Notes: body.Body, PublishedAt: body.PublishedAt}, nil
}

// Download returns the asset name of the release tagged tag.
func (c *client) Download(ctx context.Context, tag, name string) ([]byte, error) {
	url := strings.Join([]string{c.downloadBase, tag, name}, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("%w: url=%s", ErrFailedToCreateDownloadRequest, url), err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("%w: url=%s", ErrFailedToDownloadAsset, url), err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: url=%s, status=%d", ErrUnexpectedDownloadStatus, url, resp.StatusCode)
	}

	// One byte past the limit tells an oversized asset from one exactly at it.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("%w: url=%s", ErrFailedToReadAsset, url), err)
	}

	if len(data) > maxAssetBytes {
		return nil, fmt.Errorf("%w: url=%s, limit=%d", ErrAssetTooLarge, url, maxAssetBytes)
	}

	return data, nil
}
