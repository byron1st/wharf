package service

import (
	"context"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
)

type Service interface {
	SyncJiraTasks(ctx context.Context) (SyncResult, error)
}

type service struct {
	jira atlassian.Client
	db   db.Manager
}

// NewService returns a Service that syncs tasks from jira into db.
func NewService(jira atlassian.Client, db db.Manager) Service {
	return &service{jira: jira, db: db}
}

// SyncJiraTasks upserts the Jira tasks assigned to the current user. When
// Jira returns no tasks with an error, such as a failed search, nothing is
// stored and the error is returned. Otherwise every returned task is stored
// and the Jira error, if any, is reported in SyncResult.Partial.
func (s *service) SyncJiraTasks(ctx context.Context) (SyncResult, error) {
	tasks, err := s.jira.GetMyTasks(ctx)
	if tasks == nil {
		return SyncResult{}, err
	}

	if err := s.db.UpsertTasks(ctx, tasks); err != nil {
		return SyncResult{}, err
	}

	return SyncResult{Upserted: len(tasks), Partial: err}, nil
}
