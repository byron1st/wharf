package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/service"
)

var (
	ErrFailedToDecodeTaskRequest  = errors.New("server: failed to decode task request")
	ErrTaskRequestHasTrailingData = errors.New("server: task request has trailing data")
	ErrTaskRequestTooLarge        = errors.New("server: task request is too large")
)

const (
	// maxTaskRequestBytes bounds a task request body; a larger one fails
	// with ErrTaskRequestTooLarge.
	maxTaskRequestBytes = 1 << 20
	// summaryDescriptionRunes is how much of a description a summary keeps.
	summaryDescriptionRunes = 100
)

// NewHandler returns the API routes, all under /api, backed by svc and
// settings. It expects the request ID middleware of Server to run first so
// logs carry the request ID.
func NewHandler(svc service.Service, settings service.Settings, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/sync/atlassian", syncAtlassian(svc, logger))
	mux.HandleFunc("GET /api/sync/atlassian", getLastAtlassianSync(svc, logger))
	mux.HandleFunc("GET /api/service-desk/requests", listServiceDeskRequests(svc, logger))
	mux.HandleFunc("GET /api/tasks", listTasks(svc, logger))
	mux.HandleFunc("POST /api/tasks", createTask(svc, logger))
	mux.HandleFunc("GET /api/tasks/{id}", getTask(svc, logger))
	mux.HandleFunc("PUT /api/tasks/{id}", updateTask(svc, logger))
	mux.HandleFunc("DELETE /api/tasks/{id}", deleteTask(svc, logger))
	mux.HandleFunc("POST /api/tasks/{id}/sync", syncJiraTask(svc, logger))
	mux.HandleFunc("GET /api/todos", listTodos(svc, logger))
	mux.HandleFunc("POST /api/todos", createTodo(svc, logger))
	mux.HandleFunc("PUT /api/todos/{id}", updateTodo(svc, logger))
	mux.HandleFunc("DELETE /api/todos/{id}", deleteTodo(svc, logger))
	mux.HandleFunc("PUT /api/todos/{id}/service-desk-requests/{key}", linkServiceDeskRequest(svc, logger))
	mux.HandleFunc("DELETE /api/todos/{id}/service-desk-requests/{key}", unlinkServiceDeskRequest(svc, logger))
	mux.HandleFunc("GET /api/settings/atlassian", getAtlassianSettings(settings))
	mux.HandleFunc("PUT /api/settings/atlassian", saveAtlassianSettings(settings, logger))
	mux.HandleFunc("POST /api/settings/atlassian/test", checkAtlassianSettings(settings, logger))
	mux.HandleFunc("GET /api/settings/app", getAppSettings(settings))
	mux.HandleFunc("POST /api/settings/app/config/choose", chooseConfigFile(settings, logger))
	mux.HandleFunc("DELETE /api/settings/app/config/pending", clearPendingConfigFile(settings))
	mux.HandleFunc("POST /api/settings/app/config/open", openConfigFile(settings, logger))
	mux.HandleFunc("POST /api/settings/app/restart", restartServer(settings, logger))

	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func listTasks(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := svc.ListTasks(r.Context())
		if err != nil {
			writeError(w, r, logger, "failed to list tasks", err)
			return
		}

		resp := taskListResponse{Tasks: make([]taskListItem, 0, len(tasks))}
		for _, t := range tasks {
			resp.Tasks = append(resp.Tasks, taskListItem{
				taskSummary:   toSummary(t.Task),
				TodoCount:     t.Todos.Todos,
				DoneTodoCount: t.Todos.Done,
			})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func getTask(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := svc.GetTask(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, logger, "failed to get task", err)
			return
		}

		t := detail.Task
		resp := taskDetailResponse{
			taskSummary: toSummary(t),
			ChildrenIDs: nonNil(t.ChildrenIDs),
			LinkedIDs:   nonNil(t.LinkedIDs),
			Children:    toSummaries(detail.Children),
			Linked:      toSummaries(detail.Linked),
		}
		// The detail carries the full description that toSummary cut.
		resp.taskSummary.Description = t.Description

		if t.ParentID != "" {
			resp.ParentID = &t.ParentID
		}

		if detail.Parent != nil {
			parent := toSummary(*detail.Parent)
			resp.Parent = &parent
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func createTask(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := decodeTask(w, r)
		if err != nil {
			writeError(w, r, logger, "failed to create task", err)
			return
		}

		id, err := svc.CreateTask(r.Context(), t)
		if err != nil {
			writeError(w, r, logger, "failed to create task", err)
			return
		}

		w.Header().Set("Location", "/api/tasks/"+id)
		writeJSON(w, http.StatusCreated, createTaskResponse{ID: id})
	}
}

func updateTask(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := decodeTask(w, r)
		if err != nil {
			writeError(w, r, logger, "failed to update task", err)
			return
		}

		t.ID = r.PathValue("id")

		if err := svc.UpdateTask(r.Context(), t); err != nil {
			writeError(w, r, logger, "failed to update task", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteTask(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteTask(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, r, logger, "failed to delete task", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// decodeTask reads a taskRequest body, rejecting unknown fields such as id
// and originated, and anything but whitespace after the object.
func decodeTask(w http.ResponseWriter, r *http.Request) (task.Task, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTaskRequestBytes))
	decoder.DisallowUnknownFields()

	var req taskRequest

	err := decoder.Decode(&req)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return task.Task{}, errors.Join(ErrTaskRequestTooLarge, err)
	}

	if err != nil {
		return task.Task{}, errors.Join(ErrFailedToDecodeTaskRequest, err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return task.Task{}, errors.Join(ErrTaskRequestHasTrailingData, err)
	}

	return task.Task{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		StartDate:   req.StartDate,
		DueDate:     req.DueDate,
		ParentID:    req.ParentID,
		ChildrenIDs: req.ChildrenIDs,
		LinkedIDs:   req.LinkedIDs,
	}, nil
}

func toSummaries(tasks []task.Task) []taskSummary {
	summaries := make([]taskSummary, 0, len(tasks))
	for _, t := range tasks {
		summaries = append(summaries, toSummary(t))
	}

	return summaries
}

// toSummary cuts a description longer than summaryDescriptionRunes runes to
// that many runes followed by an ellipsis.
func toSummary(t task.Task) taskSummary {
	description := t.Description
	if runes := []rune(description); len(runes) > summaryDescriptionRunes {
		description = string(runes[:summaryDescriptionRunes]) + "…"
	}

	return taskSummary{
		ID:          t.ID,
		Originated:  t.Originated,
		Title:       t.Title,
		Description: description,
		Status:      t.Status,
		StartDate:   t.StartDate,
		DueDate:     t.DueDate,
	}
}

func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}

	return ids
}
