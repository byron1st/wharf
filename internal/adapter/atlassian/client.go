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
	ErrUnknownStatusCategory     = errors.New("atlassian: unknown status category")
	ErrFailedToParseStartDate    = errors.New("atlassian: failed to parse start date")
	ErrFailedToParseDueDate      = errors.New("atlassian: failed to parse due date")
)

type Client interface {
	GetMyTasks(ctx context.Context) ([]task.Task, error)
}

type client struct {
	tool tool.Tool
}

func NewClient(tool tool.Tool) Client {
	return &client{tool: tool}
}

func (c *client) GetMyTasks(ctx context.Context) ([]task.Task, error) {
	keys, searchErr := c.searchKeys(ctx)
	if len(keys) == 0 && searchErr != nil {
		return nil, searchErr
	}

	tasks := make([]task.Task, 0, len(keys))
	errs := []error{searchErr}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(ErrGetMyTasksCanceled, err)
		}

		item, err := c.viewWorkItem(ctx, key)
		if err != nil {
			return nil, errors.Join(append(errs, err)...)
		}

		t, err := ToTask(item)
		errs = append(errs, err)
		tasks = append(tasks, t)
	}

	return tasks, errors.Join(errs...)
}

func (c *client) searchKeys(ctx context.Context) ([]string, error) {
	out, err := c.tool.Run(ctx, JiraSearchOption)
	if err != nil {
		return nil, err
	}

	var items []SearchItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, errors.Join(ErrFailedToParseSearchOutput, err)
	}

	keys := make([]string, 0, len(items))

	for i, item := range items {
		if item.Key == "" {
			err = errors.Join(err, fmt.Errorf("%w: index=%d", ErrMissingWorkItemKey, i))
			continue
		}

		keys = append(keys, item.Key)
	}

	return keys, err
}

func (c *client) viewWorkItem(ctx context.Context, key string) (ViewWorkItem, error) {
	out, err := c.tool.Run(ctx, JiraWorkOption(key))
	if err != nil {
		return ViewWorkItem{}, fmt.Errorf("%w: key=%s", err, key)
	}

	var item ViewWorkItem
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		return ViewWorkItem{}, errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToParseViewOutput, key), err)
	}

	return item, nil
}
