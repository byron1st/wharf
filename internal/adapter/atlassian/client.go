package atlassian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/tool"
)

var (
	ErrFailedToParseSearchOutput = errors.New("atlassian: failed to parse search output")
	ErrMissingWorkItemKey        = errors.New("atlassian: missing work item key")
	ErrGetMyTasksCanceled        = errors.New("atlassian: get my tasks canceled")
	ErrFailedToParseViewOutput   = errors.New("atlassian: failed to parse view output")
)

type Client interface {
	GetMyTasks(ctx context.Context) ([]task.Task, error)
}

type client struct {
	tool tool.Tool
}

// NewClient returns a Client that queries Jira through the given CLI tool.
func NewClient(tool tool.Tool) Client {
	return &client{tool: tool}
}

// GetMyTasks returns the Jira work items assigned to the current user that are
// not done. Items that fail to load or convert are reported in the joined error
// alongside the tasks that succeeded.
func (c *client) GetMyTasks(ctx context.Context) ([]task.Task, error) {
	keys, errs, err := c.searchKeys(ctx)
	if err != nil {
		return nil, err
	}

	tasks := make([]task.Task, 0, len(keys))

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(append(errs, ErrGetMyTasksCanceled, err)...)
		}

		item, err := c.viewWorkItem(ctx, key)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		t, err := ToTask(item)
		errs = append(errs, err)
		tasks = append(tasks, t)
	}

	return tasks, errors.Join(errs...)
}

// searchKeys returns the found keys and the errors of items without a key.
// The last error is set only when the search itself failed.
func (c *client) searchKeys(ctx context.Context) ([]string, []error, error) {
	out, err := c.tool.Run(ctx, JiraSearchOption())
	if err != nil {
		return nil, nil, err
	}

	var items []SearchItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, nil, errors.Join(ErrFailedToParseSearchOutput, err)
	}

	keys := make([]string, 0, len(items))

	var errs []error

	for i, item := range items {
		if item.Key == "" {
			errs = append(errs, fmt.Errorf("%w: index=%d", ErrMissingWorkItemKey, i))
			continue
		}

		keys = append(keys, item.Key)
	}

	return keys, errs, nil
}

func (c *client) viewWorkItem(ctx context.Context, key string) (ViewWorkItem, error) {
	out, err := c.tool.Run(ctx, JiraViewOption(key))
	if err != nil {
		return ViewWorkItem{}, fmt.Errorf("%w: key=%s", err, key)
	}

	var item ViewWorkItem
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		return ViewWorkItem{}, errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToParseViewOutput, key), err)
	}

	return item, nil
}
