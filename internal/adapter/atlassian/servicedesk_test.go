package atlassian_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
)

const (
	createdMillis  int64 = 1444290120000
	statusAtMillis int64 = 1444287660000
)

func ownedRequestsURL(start int) string {
	return siteURL + "/rest/servicedeskapi/request?limit=50&requestOwnership=OWNED_REQUESTS" +
		"&requestStatus=ALL_REQUESTS&start=" + strconv.Itoa(start)
}

func sdPage(t *testing.T, size int, isLast bool, items ...string) []byte {
	t.Helper()

	values := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		values = append(values, json.RawMessage(item))
	}

	body, err := json.Marshal(map[string]any{
		"size":       size,
		"isLastPage": isLast,
		"values":     values,
	})
	require.NoError(t, err)

	return body
}

func sdItem(t *testing.T, item map[string]any) string {
	t.Helper()

	body, err := json.Marshal(item)
	require.NoError(t, err)

	return string(body)
}

func sdRequest(key, summary, status string, created, statusAt int64) map[string]any {
	return map[string]any{
		"issueKey": key,
		"summary":  summary,
		"createdDate": map[string]any{
			"epochMillis": created,
			"friendly":    "Monday 14:42 PM",
			"iso8601":     "2015-10-08T14:42:00+0700",
		},
		"currentStatus": map[string]any{
			"status":         status,
			"statusCategory": "NEW",
			"statusDate":     map[string]any{"epochMillis": statusAt, "friendly": "Today"},
		},
		"assignee": map[string]any{"displayName": "ignored"},
		"updated":  "ignored",
	}
}

func wantRequest(key, title, status string) servicedesk.Request {
	return servicedesk.Request{
		Key:      key,
		Title:    title,
		Status:   status,
		Created:  time.UnixMilli(createdMillis).UTC(),
		StatusAt: time.UnixMilli(statusAtMillis).UTC(),
	}
}

func TestGetOwnedRequests(t *testing.T) {
	second := sdRequest("SD-2", "   ", "Open", createdMillis, statusAtMillis)
	second["requestFieldValues"] = []any{
		map[string]any{"fieldId": "description", "value": "not the title"},
		map[string]any{"fieldId": "summary", "value": map[string]any{"html": "no"}},
		map[string]any{"fieldId": "summary", "value": "  From field  "},
	}

	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).
		Return(sdPage(t, 1, false, sdItem(t, sdRequest("SD-1", " Need a mouse ", "Waiting for Support", createdMillis, statusAtMillis))), nil).
		Once()
	m.EXPECT().Get(t.Context(), ownedRequestsURL(1), header).
		Return(sdPage(t, 1, true, sdItem(t, second)), nil).
		Once()

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, []servicedesk.Request{
		wantRequest("SD-1", "Need a mouse", "Waiting for Support"),
		wantRequest("SD-2", "From field", "Open"),
	}, got)
}

func TestGetOwnedRequestsStopsPaging(t *testing.T) {
	item := sdItem(t, sdRequest("SD-1", "Mouse", "Open", createdMillis, statusAtMillis))

	tests := []struct {
		name   string
		size   int
		isLast bool
		items  []string
		next   bool
	}{
		{name: "isLast", size: 1, isLast: true, items: []string{item}},
		{name: "empty values", size: 2, isLast: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).
				Return(sdPage(t, tt.size, tt.isLast, tt.items...), nil).
				Once()

			got, err := newClient(m).GetOwnedRequests(t.Context())
			require.NoError(t, err)

			if len(tt.items) == 0 {
				require.Empty(t, got)
				return
			}

			require.Equal(t, []servicedesk.Request{wantRequest("SD-1", "Mouse", "Open")}, got)
		})
	}
}

func TestGetOwnedRequestsPageErrors(t *testing.T) {
	item := sdItem(t, sdRequest("SD-1", "Mouse", "Open", createdMillis, statusAtMillis))

	t.Run("first page http", func(t *testing.T) {
		m := mocks.NewMockHTTPClient(t)
		m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).Return(nil, context.DeadlineExceeded).Once()

		got, err := newClient(m).GetOwnedRequests(t.Context())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, got)
	})

	t.Run("later page http", func(t *testing.T) {
		m := mocks.NewMockHTTPClient(t)
		m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).Return(sdPage(t, 1, false, item), nil).Once()
		m.EXPECT().Get(t.Context(), ownedRequestsURL(1), header).Return(nil, context.DeadlineExceeded).Once()

		got, err := newClient(m).GetOwnedRequests(t.Context())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, got)
	})

	t.Run("page is not json", func(t *testing.T) {
		m := mocks.NewMockHTTPClient(t)
		m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).Return([]byte("{"), nil).Once()

		got, err := newClient(m).GetOwnedRequests(t.Context())
		require.ErrorIs(t, err, atlassian.ErrFailedToParseRequestPage)
		require.Nil(t, got)
	})
}

func TestGetOwnedRequestsPageLimit(t *testing.T) {
	const maxPages = 100

	item := sdItem(t, sdRequest("SD-1", "Mouse", "Open", createdMillis, statusAtMillis))
	m := mocks.NewMockHTTPClient(t)

	for start := range maxPages {
		m.EXPECT().Get(t.Context(), ownedRequestsURL(start), header).
			Return(sdPage(t, 1, false, item), nil).
			Once()
	}

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.ErrorIs(t, err, atlassian.ErrTooManyRequestPages)
	require.Nil(t, got)
}

func TestGetOwnedRequestsLastPageAtLimit(t *testing.T) {
	const maxPages = 100

	m := mocks.NewMockHTTPClient(t)

	for start := range maxPages {
		item := sdItem(t, sdRequest(fmt.Sprintf("SD-%d", start+1), "Mouse", "Open", createdMillis, statusAtMillis))
		m.EXPECT().Get(t.Context(), ownedRequestsURL(start), header).
			Return(sdPage(t, 1, start == maxPages-1, item), nil).
			Once()
	}

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.NoError(t, err)
	require.Len(t, got, maxPages)
}

func TestGetOwnedRequestsJoinsSiteURL(t *testing.T) {
	query := "?limit=50&requestOwnership=OWNED_REQUESTS&requestStatus=ALL_REQUESTS&start=0"
	tests := []struct {
		name    string
		siteURL string
		want    string
	}{
		{name: "trailing slash", siteURL: siteURL + "/", want: siteURL + "/rest/servicedeskapi/request" + query},
		{
			name:    "gateway path prefix",
			siteURL: "https://api.atlassian.com/ex/jira/abc-123",
			want:    "https://api.atlassian.com/ex/jira/abc-123/rest/servicedeskapi/request" + query,
		},
		{
			name:    "gateway path prefix with trailing slash",
			siteURL: "https://api.atlassian.com/ex/jira/abc-123/",
			want:    "https://api.atlassian.com/ex/jira/abc-123/rest/servicedeskapi/request" + query,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Get(t.Context(), tt.want, header).Return(sdPage(t, 0, true), nil).Once()

			got, err := atlassian.NewClient(m, tt.siteURL, email, apiToken).GetOwnedRequests(t.Context())
			require.NoError(t, err)
			require.Empty(t, got)
		})
	}
}

func TestGetOwnedRequestsSkipsBadItems(t *testing.T) {
	good := sdRequest("SD-9", "Kept", "Open", createdMillis, statusAtMillis)
	missingCreated := sdRequest("SD-4", "Title", "Open", 0, statusAtMillis)
	missingStatusAt := sdRequest("SD-5", "Title", "Open", createdMillis, 0)
	blankStatus := sdRequest("SD-6", "Title", "   ", createdMillis, statusAtMillis)
	objectTitle := sdRequest("SD-7", "", "Open", createdMillis, statusAtMillis)
	objectTitle["requestFieldValues"] = []any{
		map[string]any{"fieldId": "summary", "value": map[string]any{"text": "no"}},
	}

	items := []string{
		sdItem(t, map[string]any{"summary": "no key"}),
		"1",
		sdItem(t, sdRequest("SD-3", "", "Open", createdMillis, statusAtMillis)),
		sdItem(t, missingCreated),
		sdItem(t, missingStatusAt),
		sdItem(t, blankStatus),
		sdItem(t, objectTitle),
		sdItem(t, good),
	}

	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).Return(sdPage(t, len(items), true, items...), nil).Once()

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.Equal(t, []servicedesk.Request{wantRequest("SD-9", "Kept", "Open")}, got)
	require.ErrorIs(t, err, atlassian.ErrMissingRequestKey)
	require.ErrorIs(t, err, atlassian.ErrFailedToParseRequest)
	require.ErrorIs(t, err, atlassian.ErrMissingRequestTitle)
	require.ErrorIs(t, err, atlassian.ErrMissingRequestCreated)
	require.ErrorIs(t, err, atlassian.ErrMissingRequestStatusDate)
	require.ErrorIs(t, err, atlassian.ErrMissingRequestStatus)
	require.ErrorContains(t, err, "index=0")
	require.ErrorContains(t, err, "index=1")
	require.ErrorContains(t, err, "key=SD-3")
	require.ErrorContains(t, err, "key=SD-6")
}

func TestGetOwnedRequestsCanceledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(sdPage(t, 0, true))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := atlassian.NewClient(httputil.NewHTTPClient(), srv.URL, email, apiToken).GetOwnedRequests(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
}

func TestGetOwnedRequestsOverHTTP(t *testing.T) {
	var got *http.Request

	body := sdPage(t, 1, true, sdItem(t, sdRequest("SD-1", "Mouse", "Open", createdMillis, statusAtMillis)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(t.Context())
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	requests, err := atlassian.NewClient(httputil.NewHTTPClient(), srv.URL, email, apiToken).
		GetOwnedRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, []servicedesk.Request{wantRequest("SD-1", "Mouse", "Open")}, requests)
	require.Equal(t, http.MethodGet, got.Method)
	require.Equal(t, "/rest/servicedeskapi/request", got.URL.Path)
	require.Equal(t, "limit=50&requestOwnership=OWNED_REQUESTS&requestStatus=ALL_REQUESTS&start=0", got.URL.RawQuery)
	require.Equal(t, header.Get("Authorization"), got.Header.Get("Authorization"))
	require.NotContains(t, fmt.Sprint(got.Header), apiToken)
}

func TestGetOwnedRequestsAdvancesByPageSize(t *testing.T) {
	first := sdItem(t, sdRequest("SD-1", "Mouse", "Open", createdMillis, statusAtMillis))
	second := sdItem(t, sdRequest("SD-2", "Keyboard", "Open", createdMillis, statusAtMillis))
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).Return(sdPage(t, 1, false, first), nil).Once()
	m.EXPECT().Get(t.Context(), ownedRequestsURL(1), header).Return(sdPage(t, 1, true, second), nil).Once()

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, []servicedesk.Request{
		wantRequest("SD-1", "Mouse", "Open"),
		wantRequest("SD-2", "Keyboard", "Open"),
	}, got)
}

func TestGetOwnedRequestsDedupesIssueKey(t *testing.T) {
	first := sdItem(t, sdRequest("SD-1", "Mouse", "Open", createdMillis, statusAtMillis))
	dup := sdItem(t, sdRequest("SD-1", "Later title", "Closed", createdMillis, statusAtMillis))
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).Return(sdPage(t, 1, false, first), nil).Once()
	m.EXPECT().Get(t.Context(), ownedRequestsURL(1), header).Return(sdPage(t, 1, true, dup), nil).Once()

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, []servicedesk.Request{wantRequest("SD-1", "Mouse", "Open")}, got)
}

func TestGetOwnedRequestsNormalizesFields(t *testing.T) {
	item := sdRequest("  SD-1  ", "   ", "  Open  ", -1, statusAtMillis)
	item["requestFieldValues"] = []any{
		map[string]any{"fieldId": "summary", "value": "   "},
		map[string]any{"fieldId": "summary", "value": "  Kept  "},
	}

	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Get(t.Context(), ownedRequestsURL(0), header).
		Return(sdPage(t, 1, true, sdItem(t, item)), nil).
		Once()

	got, err := newClient(m).GetOwnedRequests(t.Context())
	require.NoError(t, err)

	want := wantRequest("SD-1", "Kept", "  Open  ")
	want.Created = time.UnixMilli(-1).UTC()
	require.Equal(t, []servicedesk.Request{want}, got)
}

func TestGetOwnedRequestsRejectsBadURL(t *testing.T) {
	got, err := atlassian.NewClient(mocks.NewMockHTTPClient(t), "http://a b", email, apiToken).
		GetOwnedRequests(t.Context())
	require.ErrorIs(t, err, atlassian.ErrFailedToBuildRequestURL)
	require.Nil(t, got)
}
