// Package atlassiansync holds the outcome of syncing Jira issues and Service
// Desk requests together.
package atlassiansync

import "time"

// Record is a sync that stored what Atlassian returned, completely or partly.
type Record struct {
	SyncedAt time.Time
	// Issues is how many Jira issues were stored; IssueFailures is how many
	// failed to read or convert. A failed issue may still be among Issues.
	Issues        int
	IssueFailures int
	// OpenRequests is how many stored requests are not closed, and
	// ClosedRequests how many of the requests open before the sync it closed.
	OpenRequests   int
	ClosedRequests int
	// RequestFailures is how many requests failed to read and were skipped.
	RequestFailures int
}
