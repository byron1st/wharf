package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
)

var (
	ErrAtlassianNotConfigured = errors.New("service: atlassian is not configured")
	ErrAtlassianUnauthorized  = errors.New("service: atlassian rejected the credentials")
	ErrAtlassianUnreachable   = errors.New("service: atlassian is unreachable")
	ErrNoAtlassianSync        = errors.New("service: no atlassian sync recorded")
	ErrTaskToSyncNotFound     = errors.New("service: task to sync not found")
	ErrTaskNotJira            = errors.New("service: only jira tasks can be synced")
	ErrJiraIssueNotFound      = errors.New("service: jira issue not found")
	ErrJiraIssueKeyChanged    = errors.New("service: jira issue key changed")
)

// SyncAtlassian stores the Jira issues assigned to the current user and the
// Service Desk requests they created, and records the outcome. Both are
// fetched before anything is stored, so a failed fetch changes nothing. Items
// that failed to read are counted and their messages returned; a Jira issue
// whose fields failed to convert is still stored, as GetMyTasks returns it.
// Stored Jira tasks that are still open but missing from the search, such as
// ones done in Jira since the last sync, are fetched again one by one; one
// that fails to fetch keeps its stored copy and counts as a failed issue, and
// an HTTP or network failure stops the rest of the refetch. A stored open task
// this sync finds closed records the sync's time as its ClosedSyncedAt.
func (s *service) SyncAtlassian(ctx context.Context) (AtlassianSync, error) {
	// A nil slice with an error is a failed fetch; see atlassian.Client.
	tasks, taskErr := s.jira.GetMyTasks(ctx)
	if tasks == nil && taskErr != nil {
		return AtlassianSync{}, classifyAtlassianError(taskErr)
	}

	requests, requestErr := s.jira.GetOwnedRequests(ctx)
	if requests == nil && requestErr != nil {
		return AtlassianSync{}, classifyAtlassianError(requestErr)
	}

	wasOpen, err := s.db.ListServiceDeskRequests(ctx, false)
	if err != nil {
		return AtlassianSync{}, err
	}

	stored, err := s.db.ListTasks(ctx)
	if err != nil {
		return AtlassianSync{}, err
	}

	refetched, refetchErrs := s.refetchDroppedTasks(ctx, stored, tasks)

	// A copy, so stamping the tasks below leaves the client's slice alone.
	toStore := append(slices.Clone(tasks), refetched...)

	syncedAt := s.now().UTC()
	for i := range toStore {
		toStore[i].ClosedSyncedAt = &syncedAt
	}

	if err := s.db.UpsertTasks(ctx, toStore); err != nil {
		return AtlassianSync{}, err
	}

	if err := s.db.UpsertServiceDeskRequests(ctx, requests); err != nil {
		return AtlassianSync{}, err
	}

	open, closed := countRequests(wasOpen, requests)

	taskErrs, requestErrs := append(unwrapAll(taskErr), refetchErrs...), unwrapAll(requestErr)

	record := atlassiansync.Record{
		SyncedAt:        syncedAt,
		Issues:          len(toStore),
		IssueFailures:   len(taskErrs),
		OpenRequests:    open,
		ClosedRequests:  closed,
		RequestFailures: len(requestErrs),
	}
	if err := s.db.CreateAtlassianSync(ctx, record); err != nil {
		return AtlassianSync{}, err
	}

	messages := []string{}
	for _, err := range append(taskErrs, requestErrs...) {
		messages = append(messages, err.Error())
	}

	return AtlassianSync{Record: record, Errors: messages}, nil
}

// GetLastAtlassianSync returns the latest recorded sync. It fails with
// ErrNoAtlassianSync before the first one.
func (s *service) GetLastAtlassianSync(ctx context.Context) (atlassiansync.Record, error) {
	r, err := s.db.GetLastAtlassianSync(ctx)
	if errors.Is(err, db.ErrAtlassianSyncNotFound) {
		return atlassiansync.Record{}, ErrNoAtlassianSync
	}

	return r, err
}

// SyncJiraTask fetches the Jira task id again and stores it as a sync would,
// reporting which fields changed. It fails with ErrTaskToSyncNotFound when no
// task has id, with ErrTaskNotJira for other origins, and with
// ErrJiraIssueNotFound when Jira no longer has it, which keeps the stored copy.
func (s *service) SyncJiraTask(ctx context.Context, id string) (TaskSync, error) {
	current, err := s.db.GetTask(ctx, id)
	if errors.Is(err, db.ErrTaskNotFound) {
		return TaskSync{}, fmt.Errorf("%w: id=%s", ErrTaskToSyncNotFound, id)
	}

	if err != nil {
		return TaskSync{}, err
	}

	if current.Originated != task.OriginJira {
		return TaskSync{}, fmt.Errorf("%w: id=%s, originated=%s", ErrTaskNotJira, id, current.Originated)
	}

	key := strings.TrimPrefix(id, task.NewID(task.OriginJira, ""))

	fresh, err := s.jira.GetTask(ctx, key)
	if errors.Is(err, atlassian.ErrWorkItemNotFound) {
		return TaskSync{}, fmt.Errorf("%w: key=%s", ErrJiraIssueNotFound, key)
	}

	if err != nil {
		return TaskSync{}, classifyAtlassianError(err)
	}

	if err := s.db.UpsertTasks(ctx, []task.Task{fresh}); err != nil {
		return TaskSync{}, err
	}

	fresh.ParentID, fresh.ChildrenIDs, fresh.LinkedIDs = "", nil, nil

	return TaskSync{Task: fresh, Changes: changedFields(current, fresh)}, nil
}

// ListServiceDeskRequests returns the stored requests in issue key order:
// only open ones unless includeClosed.
func (s *service) ListServiceDeskRequests(ctx context.Context, includeClosed bool) ([]servicedesk.Request, error) {
	return s.db.ListServiceDeskRequests(ctx, includeClosed)
}

// classifyAtlassianError joins err with the sentinel the server shows the
// user: not configured, credentials rejected (401 or 403), or unreachable.
// Other errors are returned as is.
func classifyAtlassianError(err error) error {
	if errors.Is(err, atlassian.ErrNotConfigured) {
		return errors.Join(ErrAtlassianNotConfigured, err)
	}

	if statusErr, ok := errors.AsType[*httputil.StatusError](err); ok &&
		(statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden) {
		return errors.Join(ErrAtlassianUnauthorized, err)
	}

	if errors.Is(err, httputil.ErrFailedToSendRequest) {
		return errors.Join(ErrAtlassianUnreachable, err)
	}

	return err
}

// refetchDroppedTasks fetches again each stored Jira task that is neither done
// nor canceled but is missing from fetched. A task that fails to fetch, or
// comes back under another key because the issue moved, is left out, so its
// stored copy is kept, and its error is returned. An HTTP or network failure
// other than a missing issue would likely repeat for every remaining task, so
// it stops the refetch and is returned once.
func (s *service) refetchDroppedTasks(ctx context.Context, stored, fetched []task.Task) ([]task.Task, []error) {
	seen := make(map[string]bool, len(fetched))
	for _, t := range fetched {
		seen[t.ID] = true
	}

	var (
		refetched []task.Task
		errs      []error
	)

	for _, t := range stored {
		if t.Originated != task.OriginJira || t.Status == task.StatusDone || t.Status == task.StatusCanceled ||
			seen[t.ID] {
			continue
		}

		key := strings.TrimPrefix(t.ID, task.NewID(task.OriginJira, ""))

		fresh, err := s.jira.GetTask(ctx, key)
		if _, ok := errors.AsType[*httputil.StatusError](err); ok || errors.Is(err, httputil.ErrFailedToSendRequest) {
			errs = append(errs, fmt.Errorf("%w: id=%s", classifyAtlassianError(err), t.ID))
			break
		}

		if err != nil {
			errs = append(errs, fmt.Errorf("%w: id=%s", err, t.ID))
			continue
		}

		if fresh.ID != t.ID {
			errs = append(errs, fmt.Errorf("%w: id=%s, new_id=%s", ErrJiraIssueKeyChanged, t.ID, fresh.ID))
			continue
		}

		refetched = append(refetched, fresh)
	}

	return refetched, errs
}

// countRequests returns how many of requests are open, and how many of them
// were in wasOpen and are closed now.
func countRequests(wasOpen, requests []servicedesk.Request) (open, closed int) {
	keys := make(map[string]bool, len(wasOpen))
	for _, r := range wasOpen {
		keys[r.Key] = true
	}

	for _, r := range requests {
		switch {
		case r.StatusCategory != servicedesk.StatusCategoryDone:
			open++
		case keys[r.Key]:
			closed++
		}
	}

	return open, closed
}

// unwrapAll returns the errors joined in err, or err alone, or none for nil.
func unwrapAll(err error) []error {
	if err == nil {
		return nil
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return joined.Unwrap()
	}

	return []error{err}
}

func changedFields(before, after task.Task) []string {
	changes := []string{}

	for _, f := range []struct {
		name    string
		changed bool
	}{
		{"title", before.Title != after.Title},
		{"description", before.Description != after.Description},
		{"status", before.Status != after.Status},
		{"start_date", !sameDate(before.StartDate, after.StartDate)},
		{"due_date", !sameDate(before.DueDate, after.DueDate)},
	} {
		if f.changed {
			changes = append(changes, f.name)
		}
	}

	return changes
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Equal(*b)
}
