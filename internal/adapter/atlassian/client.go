package atlassian

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
)

var (
	ErrFailedToEncodeSearchRequest = errors.New("atlassian: failed to encode search request")
	ErrFailedToParseSearchOutput   = errors.New("atlassian: failed to parse search output")
	ErrFailedToParseWorkItem       = errors.New("atlassian: failed to parse work item")
	ErrMissingWorkItemKey          = errors.New("atlassian: missing work item key")
	ErrRepeatedSearchPageToken     = errors.New("atlassian: search returned the page token it was sent")
	ErrTooManySearchPages          = errors.New("atlassian: search exceeded the page limit")
	ErrNotConfigured               = errors.New("atlassian: site is not configured")
	ErrMissingSiteURLToCheck       = errors.New("atlassian: site url to check is empty")
	ErrFailedToReachJira           = errors.New("atlassian: failed to reach jira")
	ErrFailedToReachServiceDesk    = errors.New("atlassian: failed to reach service desk")
)

// myselfPath is the cheapest authenticated Jira call, used by Check.
const myselfPath = "/rest/api/3/myself"

// Client reaches Jira and Service Desk. GetMyTasks and GetOwnedRequests
// return nil only with an error, when nothing could be read; a non-nil slice,
// possibly empty, comes with the joined errors of the items that failed.
type Client interface {
	GetMyTasks(ctx context.Context) ([]task.Task, error)
	GetOwnedRequests(ctx context.Context) ([]servicedesk.Request, error)
	GetTask(ctx context.Context, key string) (task.Task, error)
	Configure(siteURL, email, apiToken string)
	Check(ctx context.Context, siteURL, email, apiToken string) error
}

type client struct {
	http httputil.HTTPClient

	mu  sync.RWMutex
	api api
}

// api is where and how a client reaches one Atlassian site.
type api struct {
	base   string
	header http.Header
}

// NewClient returns a Client that queries the Jira Cloud REST API v3 at siteURL
// with Basic auth. siteURL is either https://{site}.atlassian.net or a gateway
// URL such as https://api.atlassian.com/ex/jira/{cloudId}; its path is kept.
// An empty siteURL leaves the client unconfigured until Configure.
func NewClient(httpClient httputil.HTTPClient, siteURL, email, apiToken string) Client {
	return &client{http: httpClient, api: newAPI(siteURL, email, apiToken)}
}

// Configure replaces the site and credentials used by later calls. Calls in
// flight keep the ones they started with.
func (c *client) Configure(siteURL, email, apiToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.api = newAPI(siteURL, email, apiToken)
}

// Check reports whether both Jira and Service Desk accept siteURL, email, and
// apiToken, without changing the configured ones.
func (c *client) Check(ctx context.Context, siteURL, email, apiToken string) error {
	a := newAPI(siteURL, email, apiToken)
	if a.base == "" {
		return ErrMissingSiteURLToCheck
	}

	if _, err := c.http.Get(ctx, a.base+myselfPath, a.header); err != nil {
		return errors.Join(ErrFailedToReachJira, err)
	}

	if _, err := c.http.Get(ctx, a.base+requestsPath+"?limit=1", a.header); err != nil {
		return errors.Join(ErrFailedToReachServiceDesk, err)
	}

	return nil
}

// current returns the configured api, or ErrNotConfigured before a site is set.
func (c *client) current() (api, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.api.base == "" {
		return api{}, ErrNotConfigured
	}

	return c.api, nil
}

func newAPI(siteURL, email, apiToken string) api {
	base := strings.TrimRight(siteURL, "/")
	if base == "" {
		return api{}
	}

	return api{
		base: base,
		header: http.Header{
			"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+apiToken))},
			"Accept":        {"application/json"},
			"Content-Type":  {"application/json"},
		},
	}
}
