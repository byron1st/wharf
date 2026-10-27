// Package servicedesk holds Service Desk customer requests as stored by wharf.
// They are a separate collection from Jira tasks.
package servicedesk

import "time"

// Request is a Service Desk customer request created by the current user.
type Request struct {
	Key      string
	Title    string
	Status   string
	Created  time.Time
	StatusAt time.Time
}
