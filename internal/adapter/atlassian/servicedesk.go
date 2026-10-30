package atlassian

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/byron1st/wharf/internal/pkg/servicedesk"
)

var (
	ErrFailedToBuildRequestURL  = errors.New("atlassian: failed to build service desk request url")
	ErrFailedToParseRequestPage = errors.New("atlassian: failed to parse service desk request page")
	ErrFailedToParseRequest     = errors.New("atlassian: failed to parse service desk request")
	ErrMissingRequestKey        = errors.New("atlassian: missing service desk request key")
	ErrMissingRequestTitle      = errors.New("atlassian: missing service desk request title")
	ErrMissingRequestStatus     = errors.New("atlassian: missing service desk request status")
	ErrMissingRequestCreated    = errors.New("atlassian: missing service desk request created date")
	ErrMissingRequestStatusDate = errors.New("atlassian: missing service desk request status date")
	ErrTooManyRequestPages      = errors.New("atlassian: service desk request list exceeded the page limit")
)

const (
	requestsPath = "/rest/servicedeskapi/request"

	requestPageLimit = 50

	// maxRequestPages caps one GetOwnedRequests follow (100 pages of
	// requestPageLimit = 5,000 requests) so a server that never sets
	// isLastPage cannot loop until ctx ends.
	maxRequestPages = 100

	summaryFieldID = "summary"

	undefinedStatusCategory = "UNDEFINED"
)

type requestPage struct {
	IsLastPage bool              `json:"isLastPage"`
	Values     []json.RawMessage `json:"values"`
}

type customerRequest struct {
	IssueKey           string                      `json:"issueKey"`
	Summary            string                      `json:"summary"`
	CreatedDate        dateDTO                     `json:"createdDate"`
	CurrentStatus      customerRequestStatus       `json:"currentStatus"`
	RequestFieldValues []customerRequestFieldValue `json:"requestFieldValues"`
}

type customerRequestStatus struct {
	Status         string  `json:"status"`
	StatusCategory string  `json:"statusCategory"`
	StatusDate     dateDTO `json:"statusDate"`
}

type customerRequestFieldValue struct {
	FieldID string          `json:"fieldId"`
	Value   json.RawMessage `json:"value"`
}

type dateDTO struct {
	EpochMillis int64 `json:"epochMillis"`
}

// GetOwnedRequests returns Service Desk requests created by the current user,
// open and closed, following every page. A failed page, a page that is not
// JSON, or more than maxRequestPages pages returns nil and the error. Items
// that fail to decode or map are omitted and reported in the joined error.
// A repeated issue key keeps the first occurrence.
func (c *client) GetOwnedRequests(ctx context.Context) ([]servicedesk.Request, error) {
	var (
		reqs     []servicedesk.Request
		errs     []error
		seen     int
		seenKeys = make(map[string]struct{})
	)

	for pages, start := 1, 0; ; pages++ {
		page, err := c.fetchRequestPage(ctx, start)
		if err != nil {
			return nil, err
		}

		for i, raw := range page.Values {
			req, err := toRequest(raw, seen+i)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if _, ok := seenKeys[req.Key]; ok {
				continue
			}

			seenKeys[req.Key] = struct{}{}
			reqs = append(reqs, req)
		}

		seen += len(page.Values)

		if page.IsLastPage || len(page.Values) == 0 {
			if reqs == nil {
				reqs = []servicedesk.Request{}
			}

			return reqs, errors.Join(errs...)
		}

		if pages == maxRequestPages {
			return nil, fmt.Errorf("%w: max=%d", ErrTooManyRequestPages, maxRequestPages)
		}

		start += len(page.Values)
	}
}

func (c *client) fetchRequestPage(ctx context.Context, start int) (requestPage, error) {
	rawURL, err := ownedRequestsURL(c.requestsURL, start)
	if err != nil {
		return requestPage{}, err
	}

	out, err := c.http.Get(ctx, rawURL, c.header)
	if err != nil {
		return requestPage{}, err
	}

	var page requestPage
	if err := json.Unmarshal(out, &page); err != nil {
		return requestPage{}, errors.Join(ErrFailedToParseRequestPage, err)
	}

	return page, nil
}

func ownedRequestsURL(rawBase string, start int) (string, error) {
	u, err := url.Parse(rawBase)
	if err != nil {
		return "", errors.Join(ErrFailedToBuildRequestURL, err)
	}

	query := u.Query()
	query.Set("requestOwnership", "OWNED_REQUESTS")
	query.Set("requestStatus", "ALL_REQUESTS")
	query.Set("limit", strconv.Itoa(requestPageLimit))
	query.Set("start", strconv.Itoa(start))
	u.RawQuery = query.Encode()

	return u.String(), nil
}

func toRequest(raw json.RawMessage, index int) (servicedesk.Request, error) {
	var item customerRequest
	if err := json.Unmarshal(raw, &item); err != nil {
		return servicedesk.Request{}, errors.Join(
			fmt.Errorf("%w: index=%d", ErrFailedToParseRequest, index),
			err,
		)
	}

	key := strings.TrimSpace(item.IssueKey)
	if key == "" {
		return servicedesk.Request{}, fmt.Errorf("%w: index=%d", ErrMissingRequestKey, index)
	}

	title, ok := requestTitle(item)
	if !ok {
		return servicedesk.Request{}, fmt.Errorf("%w: key=%s", ErrMissingRequestTitle, key)
	}

	// status is stored as returned. Only a blank name is missing.
	status := item.CurrentStatus.Status
	if strings.TrimSpace(status) == "" {
		return servicedesk.Request{}, fmt.Errorf("%w: key=%s", ErrMissingRequestStatus, key)
	}

	// A blank category is UNDEFINED, so the request still syncs and counts as
	// not closed.
	category := cmp.Or(strings.TrimSpace(item.CurrentStatus.StatusCategory), undefinedStatusCategory)

	if item.CreatedDate.EpochMillis == 0 {
		return servicedesk.Request{}, fmt.Errorf("%w: key=%s", ErrMissingRequestCreated, key)
	}

	if item.CurrentStatus.StatusDate.EpochMillis == 0 {
		return servicedesk.Request{}, fmt.Errorf("%w: key=%s", ErrMissingRequestStatusDate, key)
	}

	return servicedesk.Request{
		Key:            key,
		Title:          title,
		Status:         status,
		StatusCategory: category,
		Created:        time.UnixMilli(item.CreatedDate.EpochMillis).UTC(),
		StatusAt:       time.UnixMilli(item.CurrentStatus.StatusDate.EpochMillis).UTC(),
	}, nil
}

func requestTitle(item customerRequest) (string, bool) {
	if title := strings.TrimSpace(item.Summary); title != "" {
		return title, true
	}

	for _, field := range item.RequestFieldValues {
		if field.FieldID != summaryFieldID {
			continue
		}

		var title string
		if err := json.Unmarshal(field.Value, &title); err != nil {
			continue
		}

		if title = strings.TrimSpace(title); title != "" {
			return title, true
		}
	}

	return "", false
}
