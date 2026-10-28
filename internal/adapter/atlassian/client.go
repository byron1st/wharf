package atlassian

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

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
)

type Client interface {
	GetMyTasks(ctx context.Context) ([]task.Task, error)
	GetOwnedRequests(ctx context.Context) ([]servicedesk.Request, error)
}

type client struct {
	http        httputil.HTTPClient
	searchURL   string
	requestsURL string
	header      http.Header
}

// NewClient returns a Client that queries the Jira Cloud REST API v3 at siteURL
// with Basic auth. siteURL is either https://{site}.atlassian.net or a gateway
// URL such as https://api.atlassian.com/ex/jira/{cloudId}; its path is kept.
func NewClient(httpClient httputil.HTTPClient, siteURL, email, apiToken string) Client {
	base := strings.TrimRight(siteURL, "/")

	return &client{
		http:        httpClient,
		searchURL:   base + searchPath,
		requestsURL: base + requestsPath,
		header: http.Header{
			"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+apiToken))},
			"Accept":        {"application/json"},
			"Content-Type":  {"application/json"},
		},
	}
}
