package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/service"
)

// ErrInvalidIncludeClosedFilter maps to 400.
var ErrInvalidIncludeClosedFilter = errors.New("server: include_closed filter is not a bool")

// syncAtlassian waits for the whole sync; the server sets no write timeout.
func syncAtlassian(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.SyncAtlassian(r.Context())
		if err != nil {
			writeError(w, r, logger, "failed to sync atlassian", err)
			return
		}

		if len(result.Errors) > 0 {
			logger.WarnContext(
				r.Context(),
				"synced atlassian partially",
				"request_id", r.Header.Get(requestIDHeader),
				"errors", result.Errors,
			)
		}

		resp := toAtlassianSyncResponse(result.Record)
		resp.Errors = result.Errors
		writeJSON(w, http.StatusOK, resp)
	}
}

// getLastAtlassianSync answers {"last_sync": null} before the first sync.
func getLastAtlassianSync(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record, err := svc.GetLastAtlassianSync(r.Context())
		if errors.Is(err, service.ErrNoAtlassianSync) {
			writeJSON(w, http.StatusOK, lastAtlassianSyncResponse{})
			return
		}

		if err != nil {
			writeError(w, r, logger, "failed to get last atlassian sync", err)
			return
		}

		resp := toAtlassianSyncResponse(record)
		writeJSON(w, http.StatusOK, lastAtlassianSyncResponse{LastSync: &resp})
	}
}

// listServiceDeskRequests lists open requests unless include_closed is true.
func listServiceDeskRequests(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var includeClosed bool

		if v := r.URL.Query().Get("include_closed"); v != "" {
			var err error
			if includeClosed, err = strconv.ParseBool(v); err != nil {
				writeError(w, r, logger, "failed to list service desk requests",
					errors.Join(ErrInvalidIncludeClosedFilter, err))

				return
			}
		}

		requests, err := svc.ListServiceDeskRequests(r.Context(), includeClosed)
		if err != nil {
			writeError(w, r, logger, "failed to list service desk requests", err)
			return
		}

		writeJSON(w, http.StatusOK, serviceDeskRequestListResponse{Requests: toServiceDeskRequests(requests)})
	}
}

func syncJiraTask(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.SyncJiraTask(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, logger, "failed to sync jira task", err)
			return
		}

		writeJSON(w, http.StatusOK, taskSyncResponse{Task: toSummary(result.Task), Changes: result.Changes})
	}
}

func toServiceDeskRequests(requests []servicedesk.Request) []serviceDeskRequestResponse {
	responses := make([]serviceDeskRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, serviceDeskRequestResponse{
			Key:            request.Key,
			Title:          request.Title,
			Status:         request.Status,
			StatusCategory: request.StatusCategory,
			CreatedAt:      request.Created,
			StatusAt:       request.StatusAt,
		})
	}

	return responses
}

func toAtlassianSyncResponse(r atlassiansync.Record) atlassianSyncResponse {
	return atlassianSyncResponse{
		SyncedAt:        r.SyncedAt,
		Issues:          r.Issues,
		IssueFailures:   r.IssueFailures,
		OpenRequests:    r.OpenRequests,
		ClosedRequests:  r.ClosedRequests,
		RequestFailures: r.RequestFailures,
	}
}
