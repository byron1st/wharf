package atlassian

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/byron1st/wharf/internal/pkg/httputil"
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

const (
	searchPath = "/rest/api/3/search/jql"

	// maxSearchPages caps the pages one GetMyTasks follows (100 pages of
	// searchMaxResults = 10,000 work items) so a server that never ends the
	// token chain cannot loop until ctx ends.
	maxSearchPages = 100
)

type Client interface {
	GetMyTasks(ctx context.Context) ([]task.Task, error)
}

type client struct {
	http      httputil.HTTPClient
	searchURL string
	header    http.Header
}

// NewClient returns a Client that queries the Jira Cloud REST API v3 at siteURL
// with Basic auth. siteURL is either https://{site}.atlassian.net or a gateway
// URL such as https://api.atlassian.com/ex/jira/{cloudId}; its path is kept.
func NewClient(httpClient httputil.HTTPClient, siteURL, email, apiToken string) Client {
	return &client{
		http:      httpClient,
		searchURL: strings.TrimRight(siteURL, "/") + searchPath,
		header: http.Header{
			"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+apiToken))},
			"Accept":        {"application/json"},
			"Content-Type":  {"application/json"},
		},
	}
}

// GetMyTasks returns the Jira work items assigned to the current user that are
// not done, following every search page. A failed page, a repeated page token,
// or more than maxSearchPages pages returns nil and the error. Items that fail
// to decode or convert are reported in the joined error alongside the tasks
// that succeeded.
func (c *client) GetMyTasks(ctx context.Context) ([]task.Task, error) {
	tasks := []task.Task{}

	var (
		errs  []error
		token string
		seen  int
	)

	for pages := 1; ; pages++ {
		page, err := c.search(ctx, token)
		if err != nil {
			return nil, err
		}

		for i, raw := range page.Issues {
			index := seen + i

			var item ViewWorkItem
			if err := json.Unmarshal(raw, &item); err != nil {
				errs = append(errs, errors.Join(fmt.Errorf("%w: index=%d", ErrFailedToParseWorkItem, index), err))
				continue
			}

			if item.Key == "" {
				errs = append(errs, fmt.Errorf("%w: index=%d", ErrMissingWorkItemKey, index))
				continue
			}

			t, err := ToTask(item)
			errs = append(errs, err)
			tasks = append(tasks, t)
		}

		seen += len(page.Issues)

		if page.IsLast || page.NextPageToken == "" {
			return tasks, errors.Join(errs...)
		}

		if page.NextPageToken == token {
			return nil, fmt.Errorf("%w: page=%d", ErrRepeatedSearchPageToken, pages)
		}

		if pages == maxSearchPages {
			return nil, fmt.Errorf("%w: max=%d", ErrTooManySearchPages, maxSearchPages)
		}

		token = page.NextPageToken
	}
}

func (c *client) search(ctx context.Context, token string) (searchResponse, error) {
	body, err := json.Marshal(searchRequest{
		JQL:           myTasksJQL,
		Fields:        searchFields(),
		MaxResults:    searchMaxResults,
		NextPageToken: token,
	})
	if err != nil {
		return searchResponse{}, errors.Join(ErrFailedToEncodeSearchRequest, err)
	}

	out, err := c.http.Post(ctx, c.searchURL, c.header, body)
	if err != nil {
		return searchResponse{}, err
	}

	var page searchResponse
	if err := json.Unmarshal(out, &page); err != nil {
		return searchResponse{}, errors.Join(ErrFailedToParseSearchOutput, err)
	}

	return page, nil
}
