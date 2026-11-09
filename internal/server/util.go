package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/byron1st/wharf/internal/service"
)

// errorStatuses maps the errors a request can cause to their status, and to
// a code when the client must tell them apart from other errors with the same
// status. Any other error is a 500.
var errorStatuses = []struct {
	err    error
	status int
	code   string
}{
	{err: ErrFailedToDecodeTaskRequest, status: http.StatusBadRequest},
	{err: ErrTaskRequestHasTrailingData, status: http.StatusBadRequest},
	{err: ErrTaskRequestTooLarge, status: http.StatusRequestEntityTooLarge},
	{err: service.ErrEmptyTaskTitle, status: http.StatusBadRequest},
	{err: service.ErrUnknownTaskStatus, status: http.StatusBadRequest},
	{err: service.ErrEmptyRelatedTaskID, status: http.StatusBadRequest},
	{err: service.ErrTaskReferencesItself, status: http.StatusBadRequest},
	{err: service.ErrTaskNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskToUpdateNotFound, status: http.StatusNotFound},
	{err: service.ErrLocalTaskToUpdateNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskToDeleteNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskNotLocal, status: http.StatusConflict},
	{err: service.ErrUnknownTaskStatusToSet, status: http.StatusBadRequest},
	{err: service.ErrLocalTaskToSetStatusNotFound, status: http.StatusNotFound},
	{err: service.ErrEmptyTaskIDToReorder, status: http.StatusBadRequest},
	{err: service.ErrDuplicateTaskIDToReorder, status: http.StatusBadRequest},
	{err: service.ErrTodoToLinkNotFound, status: http.StatusNotFound},
	{err: service.ErrServiceDeskRequestToLinkNotFound, status: http.StatusNotFound},
	{err: service.ErrServiceDeskRequestLinkToDeleteNotFound, status: http.StatusNotFound},
	{err: ErrFailedToDecodeTodoRequest, status: http.StatusBadRequest},
	{err: ErrTodoRequestHasTrailingData, status: http.StatusBadRequest},
	{err: ErrTodoRequestTooLarge, status: http.StatusRequestEntityTooLarge},
	{err: ErrEmptyTodoRequestDate, status: http.StatusBadRequest},
	{err: ErrInvalidTodoIsDoneFilter, status: http.StatusBadRequest},
	{err: ErrTodoIDNotInteger, status: http.StatusNotFound},
	{err: service.ErrEmptyTodoDescription, status: http.StatusBadRequest},
	{err: service.ErrEmptyTodoTaskID, status: http.StatusBadRequest},
	{err: service.ErrInvalidTodoDate, status: http.StatusBadRequest},
	{err: service.ErrTodoStartsAfterEnd, status: http.StatusBadRequest},
	{err: service.ErrNegativeTodoPosition, status: http.StatusBadRequest},
	{err: ErrInvalidCompletedSinceFilter, status: http.StatusBadRequest},
	{err: ErrInvalidIncludeClosedFilter, status: http.StatusBadRequest},
	{err: ErrFailedToDecodeSettingsRequest, status: http.StatusBadRequest},
	{err: ErrSettingsRequestHasTrailingData, status: http.StatusBadRequest},
	{err: ErrSettingsRequestTooLarge, status: http.StatusRequestEntityTooLarge},
	{err: service.ErrAtlassianNotConfigured, status: http.StatusConflict, code: "atlassian_not_configured"},
	{err: service.ErrAtlassianUnauthorized, status: http.StatusBadGateway, code: "atlassian_unauthorized"},
	{err: service.ErrAtlassianUnreachable, status: http.StatusGatewayTimeout, code: "atlassian_unreachable"},
	{err: service.ErrTaskToSyncNotFound, status: http.StatusNotFound},
	{err: service.ErrTaskNotJira, status: http.StatusConflict},
	{err: service.ErrJiraIssueNotFound, status: http.StatusNotFound, code: "jira_issue_not_found"},
	{err: service.ErrInvalidAtlassianSettings, status: http.StatusBadRequest},
	{err: service.ErrInvalidAtlassianSettingsToCheck, status: http.StatusBadRequest},
	{err: service.ErrInvalidConfigFile, status: http.StatusBadRequest, code: "invalid_config_file"},
	{err: service.ErrUnsupportedOS, status: http.StatusNotImplemented, code: "unsupported_os"},
	{err: service.ErrTaskOfNewTodoNotFound, status: http.StatusBadRequest},
	{err: service.ErrTaskOfUpdatedTodoNotFound, status: http.StatusBadRequest},
	{err: service.ErrTodoToUpdateNotFound, status: http.StatusNotFound},
	{err: service.ErrTodoToDeleteNotFound, status: http.StatusNotFound},
}

// writeError logs err with msg and writes it with the status errorStatuses
// gives it, or as an internal error.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, msg string, err error) {
	requestID := r.Header.Get(requestIDHeader)

	for _, e := range errorStatuses {
		if errors.Is(err, e.err) {
			logger.InfoContext(r.Context(), msg, "request_id", requestID, "status", e.status, "error", err)
			writeJSON(w, e.status, errorResponse{Error: err.Error(), Code: e.code})

			return
		}
	}

	logger.ErrorContext(r.Context(), msg, "request_id", requestID, "error", err)
	writeInternalError(w, err)
}

// writeInternalError hides the cause from the client; it is logged instead.
func writeInternalError(w http.ResponseWriter, err error) {
	msg := "internal server error"
	if err != nil {
		msg = err.Error()
	}

	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// The status is already sent, so an encoding or write failure, such as a
	// closed connection, cannot be reported to the client.
	_ = json.NewEncoder(w).Encode(body)
}
