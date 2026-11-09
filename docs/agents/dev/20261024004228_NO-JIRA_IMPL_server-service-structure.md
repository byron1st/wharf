---
Application: wharf
JiraTicket: NO-JIRA
ReportType: single-step
Timestamp: 20261024004228
Title: server-service-structure
ReviewBase: `git diff a739359` (작업 트리, 미커밋; 새 파일은 `git status` 의 untracked 포함)
---

# internal/server · internal/service 기본 구조와 `POST /tasks/jira` 시범 API

Plan: [20261024004228_NO-JIRA_PLAN_server-service-structure.md](./20261024004228_NO-JIRA_PLAN_server-service-structure.md)

See the change: `git diff a739359` + untracked 파일. Every `path:line` anchor in this report is valid against that snapshot.

## Summary
`server → service → adapter` 계층의 첫 뼈대를 만들고, Jira 의 내 task 를 DB 에 upsert 하는 `POST /tasks/jira` 와 `GET /healthz` 를 붙였다. db adapter 는 도메인 타입 `task.Task` 를 직접 받는 `UpsertTasks` 로 바꾸고 `ExistsTask` 를 제거했으며, 부분 실패는 "받은 task 는 모두 저장하고 `partial:true` + Warn 로그" 규칙을 따른다.

## Red Flags
- **RF1** `internal/server/server.go:68` - request ID 미들웨어는 들어온 `X-Request-ID` 값을 길이·문자 검사 없이 재사용한다(plan 이 "들어온 값은 재사용"으로 정함). slog JSON handler 가 escape 하므로 로그 주입은 막히지만 임의 길이 값이 로그·응답 헤더로 그대로 나간다.
- **RF2** 로컬 도구 - 이 머신의 `/usr/local/bin/golangci-lint` 는 go1.25 로 빌드돼 go 1.27.3 모듈을 거부했고 `mockery`·`sqlc` 는 없었다. `GOTOOLCHAIN=go1.27.3 go install` 로 `golangci-lint v2.15.0`, `mockery v3.8.1`, `sqlc v1.31.1` 을 `/root/go/bin` 에 설치하고 `PATH=/root/go/bin:$PATH make gen|check` 로 실행했다. 프로젝트 파일(go.mod 등)은 바뀌지 않았다.

## Open Questions
- **OQ1** `internal/server/server.go:22` - `shutdownTimeout` 30초. Jira 동기화가 항목마다 CLI 를 호출해 30초를 넘기면 `Run` 이 `ErrFailedToShutdown` 을 반환하고 main 이 exit 1 한다. 실제 동기화 소요 시간을 모르므로 값이 적절한지 확인이 필요하다.
- **OQ2** `internal/server/route.go:36` - request ID 를 context 가 아니라 request header 로 전달한다(미들웨어가 생성한 ID 는 `r.Clone` 후 header 에 넣음). route 테스트가 미들웨어 없이 header 만으로 request id 로그를 검증할 수 있어 택했지만, 이후 service 로그가 필요해지면 context 전달로 바꾸는 편이 낫다.

## Plan Divergence
### Changed - details that differ from the plan
- None
### Added - implemented but absent from the plan
- `GET /tasks/jira` 등 잘못된 method 가 405 를 반환하는지 확인하는 테스트(`internal/server/route_test.go:82`): ServeMux method pattern 동작 고정용.
- `http.Server.ReadHeaderTimeout` 10초(`internal/server/server.go:38`): WriteTimeout 은 두지 않되 slowloris 방어용 헤더 timeout 만 설정.
- 서비스 에러는 route 에서 Error 로그(request id 포함)로 남긴다(`internal/server/route.go:41`): 500 body 에 원인을 숨기므로 원인 추적용.
### Deferred - planned but not implemented (deferred)
- `docs/agents/research/sqlite-sqlc-setup.md`·`server-service-structure.md` 의 `ExistsTask`/`CreateTasks` 서술 갱신은 plan 대로 `application-research-sync` 에 남긴다.

## TODO Fulfillment
### TODO 1: `CreateTasks` → `UpsertTasks([]task.Task)` 변환 구현, `ExistsTask` 제거, `make gen`, db 테스트 직접 SQL 확인 방식 - done
- 구현: `internal/adapter/db/client.go:55` `UpsertTasks` - 단일 트랜잭션, 빈 목록 no-op, 실패 시 `ErrFailedToUpsertTask` + `id=`. `client.go:87` `toUpsertTaskParams`, `:102` `toNullTime`(nil → NULL), `:110` `toJSONArray`(nil·빈 목록 → `"[]"`, 그 외 `json.Marshal`). `scripts/queries.sql` 의 `ExistsTask` 쿼리 삭제 후 `make gen` 으로 `queries.sql.go`, `mocks/db_manager.go` 재생성. `ErrFailedToCheckTaskExists`, `orEmptyJSONArray` 제거.
- 테스트: `internal/adapter/db/client_test.go:134` `TestUpsertTasks`(날짜·ID 목록 JSON·nil 날짜 NULL), `:173` `TestUpsertTasksOverwritesExistingTask`(덮어쓰기 시 날짜 NULL 로 덮임), `:197` `TestUpsertTasksStoresEmptyIDListsAsJSONArray`(빈·nil 목록), `:218` `TestUpsertTasksRollsBackOnFailure`(all-or-nothing, `ErrFailedToUpsertTask`), `:245` `TestUpsertTasksBeginTxFailure`. 존재 확인은 `:109` `requireExists(t, path, id, want)` 가 파일 DB 에 별도 연결로 SQL 실행. `client_management_test.go` 의 in-memory 사용 테스트는 `newFileManager` 로 전환.
- AC: AC-5 — 위 테스트 `make test` 통과. AC-6 — `rg -n "ExistsTask|CreateTasks" -g '*.go' -g '*.sql' .` 결과 없음(rc=1).
- 편차: none

### TODO 2: `service.Service` 와 Jira 동기화 함수, `.mockery.yml` 에 Service·Client 등록, mock 테스트 - done
- 구현: `internal/service/service.go:37` `SyncJiraTasks` - `GetMyTasks` 가 nil tasks 면 upsert 없이 그 에러 반환, 아니면 전부 upsert 하고 `SyncResult{Upserted, Partial}` 반환. upsert 실패는 그대로 반환. 로그 없음. `.mockery.yml` 에 `atlassian.Client`, `service.Service` 추가 → `internal/pkg/mocks/atlassian_client.go`, `service_service.go` 생성.
- 테스트: `internal/service/service_test.go:20` `TestSyncJiraTasks`, `:32` `TestSyncJiraTasksStoresPartialResult`, `:47` `TestSyncJiraTasksSkipsUpsertWhenJiraFails`(UpsertTasks 기대 없음 → 호출 시 mock 실패), `:61` `TestSyncJiraTasksReturnsUpsertFailure`.
- AC: AC-2, AC-3, AC-4 (service 측) — 위 테스트 통과. AC-10 — `make deps-graph` 에서 `service → server` 간선 없음.
- 편차: design-bearing 태그였지만 Key decisions 가 반환 규칙을 모두 고정하고 이름·결과 struct 는 Discretion 이라 consultant 없이 `SyncResult{Upserted int; Partial error}` 로 정했다.

### TODO 3: `route.go` 에 healthz 와 `POST /tasks/jira` handler, `MockService` + httptest 테스트 - done
- 구현: `internal/server/route.go:22` `NewHandler` - `GET /healthz`(`{"status":"ok"}`), `POST /tasks/jira`. `:36` `syncJiraTasks` - 성공 200 `{"upserted":N,"partial":bool}`, Partial 있으면 Warn 로그(request_id, upserted, error), service 에러는 Error 로그 + 500 `{"error":"internal server error"}`.
- 테스트: `internal/server/route_test.go:35` `TestHealthz`, `:42` `TestSyncJiraTasks`, `:54` `TestSyncJiraTasksPartial`(Warn + `request_id":"req-1"`), `:70` `TestSyncJiraTasksFailure`(body 에 내부 메시지 없음), `:82` `TestRoutesRejectOtherMethods`.
- AC: AC-1, AC-2, AC-3, AC-4 (route 측) — 위 테스트 통과.
- 편차: none

### TODO 4: `server.go` 미들웨어 세 개와 `Run`(graceful shutdown) - done
- Risk / Lens: line-by-line (동시성·shutdown)
- 구현: `internal/server/server.go:35` `New` - request-id → request-log → recover 순서로 감싸고 WriteTimeout 없음. `:45` `Run` - Serve 를 goroutine 으로, ctx 종료 시 `context.WithoutCancel` 기반 30초 timeout 으로 `Shutdown` 후 nil. Serve 조기 실패는 `ErrFailedToServe`, Shutdown 실패는 `ErrFailedToShutdown`. `:68` request ID 는 `crypto/rand.Text()` 생성. `:101` recover 는 `http.ErrAbortHandler` 재panic.
- 테스트: `internal/server/server_test.go:83` `TestMiddleware`(panic → 500 후 다음 요청 정상, X-Request-ID 재사용·생성, 요청 로그 method/path/status/request_id, ErrAbortHandler 는 연결 중단), `:121` `TestRunWaitsForInFlightRequest`(취소 후 listener 닫힘 확인 → 처리 중 요청 완료, `Run` nil).
- AC: AC-7, AC-8 — 위 테스트 `make test`, `make race` 통과.
- 편차: none

### TODO 5: `cmd/wharf/main.go` wiring 및 실행 확인 - done
- 구현: `cmd/wharf/main.go:24` `run` - signal ctx(SIGINT/SIGTERM) → DB Connect/Migrate → `WHARF_ACLI_PATH` 또는 `exec.LookPath("acli")` → `tool.NewTool` → atlassian client → service → handler → server → `net.Listen(WHARF_ADDR)` → `Run` → defer `Close`. 각 실패는 Error 로그 후 exit 1.
- 테스트: 자동 테스트 없음(wiring 만). 아래 수동 실행으로 확인.
- AC: AC-9 — 아래 Manual Verification 기록. AC-10 — `make deps-graph` 출력에 `internal/server -> internal/adapter/*` 없음.
- 편차: none

### TODO 6: AGENTS.md 의 `internal/route` → `internal/server` - done
- 구현: `AGENTS.md:30`, `:42`, `:69` 수정.
- 테스트: 해당 없음.
- AC: AC-11 — `rg -n "internal/route" AGENTS.md` 결과 없음(rc=1).
- 편차: none

## Manual Verification
AC-9 실행 기록 (이 머신에는 `acli` 가 없음):
1. `WHARF_DB_PATH=<scratch>/run1.db WHARF_ADDR=127.0.0.1:18080 make run` → 시작 실패, exit 2(make 가 go run 의 exit status 1 을 보고). 로그: `{"level":"ERROR","msg":"failed to find acli; set WHARF_ACLI_PATH","error":"exec: \"acli\": executable file not found in $PATH"}`.
2. 미인증 acli 를 흉내 내는 stub(`#!/bin/sh` 로 stderr 에 `acli stub: not authenticated` 출력 후 exit 1)을 scratchpad 에 만들고 `WHARF_ACLI_PATH=<stub> ... make run` 으로 백그라운드 실행.
   - `curl -i /healthz` → `HTTP/1.1 200 OK`, `X-Request-Id: EX2XMD4MI3IADJ56JU56YQFKPV`, body `{"status":"ok"}`.
   - `curl -i -X POST -H 'X-Request-ID: ac9-manual' /tasks/jira` → `HTTP/1.1 500 Internal Server Error`, `X-Request-Id: ac9-manual`, body `{"error":"internal server error"}`. 서버 로그: `"msg":"failed to sync jira tasks","request_id":"ac9-manual","error":"tool: failed to execute cmd: stderr=acli stub: not authenticated\n\nexit status 1"`.
   - wharf 프로세스에 `kill -INT <pid>` → 로그 `"msg":"server stopped"`, `make run` exit 0. (프로세스 그룹 전체에 SIGINT 를 보내면 make 자체가 중단돼 130 을 반환하므로 wharf 바이너리에만 보냈다.)
- [ ] 인증된 실제 acli 환경에서 `POST /tasks/jira` 가 200 을 반환하는지 사용자가 확인 (읽기 전용: `acli jira workitem search/view` 만 호출).

## Test-dev (Round 0)

- Scope: working tree vs a739359. Unit coverage was already 96.1% total; remaining gaps in `internal/server` were `statusRecorder.Unwrap` and the `Run` serve-failure branch.
- Added to `internal/server/server_test.go`: `TestMiddlewareKeepsResponseControllerAccess` (http.ResponseController reaches the real writer through the middleware) and `TestRunReturnsServeFailure` (`ErrFailedToServe` on a closed listener).
- E2E: N/A (no `tests/e2e`; plan non-goal). Mutation: out of scope (caller).
- `make check`, `make test`, `make race` green. Findings: none.
