---
Application: wharf
JiraTicket: NO-JIRA
PlanType: single-step
Timestamp: 20261024004228
Title: server-service-structure
---

# internal/server · internal/service 기본 구조와 `POST /tasks/jira` 시범 API

Report: [20261024004228_NO-JIRA_IMPL_server-service-structure.md](./20261024004228_NO-JIRA_IMPL_server-service-structure.md)

## Non-goals

- `GET /healthz`, `POST /tasks/jira` 외의 endpoint. task 조회 API·조회 쿼리도 만들지 않는다.
- 범용 JSON request body 디코딩·검사 헬퍼. body 가 있는 첫 endpoint 에서 함께 도입한다.
- Jira 에서 완료되거나 사라진 task 를 DB 에서 정리하는 일. 이번에는 upsert 만 한다.
- 동기화 스케줄링, 백그라운드 실행, 동시 실행 방지.
- `atlassian` adapter 의 동작 변경, `Status`/`Origin` 값 검증.
- `tests/e2e`, healthz 의 DB 확인, 서드파티 router/logger/uuid 의존성.

## Key decisions

- **계층**: `server` → `service.Service` → `atlassian.Client`, `db.Manager`. server 는 `internal/adapter/*` 를 import 하지 않고 service 는 `net/http` 를 import 하지 않는다. `.mockery.yml` 에 `service.Service` (`service_service.go` / `MockService`), `atlassian.Client` (`atlassian_client.go` / `MockClient`) 를 추가하고 `make gen`.
- **db adapter** (사용자 결정):
  - `CreateTasks` 를 `UpsertTasks(ctx, []task.Task) error` 로 바꾼다. `task.Task` → sqlc params 변환(`*time.Time` → `sql.NullTime`, `[]string` → JSON 배열 문자열, nil·빈 목록 → `"[]"`)은 db 패키지가 맡는다.
  - 단일 트랜잭션 all-or-nothing, 기존 ID 덮어쓰기, 빈 목록 no-op 은 유지한다. sentinel 은 `ErrFailedToCreateTask` → `ErrFailedToUpsertTask`.
  - `ExistsTask` 는 Manager 메서드, `scripts/queries.sql` 쿼리, 생성 코드, mock 에서 모두 제거한다(`ErrFailedToCheckTaskExists` 포함). 기존 테스트의 존재 확인은 파일 DB 에 별도 연결로 직접 SQL 을 실행하는 방식(`read` 헬퍼와 같은 방식)으로 바꾼다.
- **service** — `POST /tasks/jira` 와 1:1 로 대응하는 동기화 함수(예: `SyncJiraTasks`). 부분 실패는 사용자 결정에 따라 "성공분만 저장":
  - `GetMyTasks` 가 nil tasks 를 반환하면(검색 실패·취소, `internal/adapter/atlassian/client.go:39`, `:46`) 저장하지 않고 에러를 반환한다.
  - tasks 가 non-nil 이면 에러가 있어도 반환된 tasks 를 모두 upsert 한다(필드 변환에 일부 실패한 항목 포함). 결과로 upsert 개수와 받은 joined error 를 함께 돌려준다.
  - `UpsertTasks` 실패는 그대로 반환한다.
  - service 는 로그를 남기지 않는다. 로그(request id 포함)는 route 가 남긴다.
- **route.go** — route 정의를 서버 객체 생성과 분리한다.
  - 라우트를 등록한 `http.Handler` 를 반환하는 생성자. 표준 `ServeMux` method pattern 으로 `GET /healthz`, `POST /tasks/jira` 를 등록한다.
  - 성공 시 200 `{"upserted": N, "partial": bool}`. 부분 실패의 joined error 는 Warn 로그로 남긴다.
  - service 에러는 500, body 는 내부 메시지 없는 일반 문구. 상태 매핑 표는 500 이 아닌 첫 사례가 생길 때 만든다.
  - request body 가 없는 endpoint 라 body 검사는 하지 않는다.
- **server.go**:
  - `*http.Server` 를 감싸는 `Server`.
  - 미들웨어 순서는 바깥부터 request-id → request-logger → recover. recover 는 `http.ErrAbortHandler` 를 다시 panic 한다. request ID 는 `X-Request-ID` 헤더로, 들어온 값은 재사용하고 응답 헤더로도 돌려준다.
  - `Run(ctx, ln net.Listener) error`: ctx 가 끝나면 정해진 timeout 안에서 graceful shutdown 하고 nil 을 반환한다. listener 를 받는 것은 테스트에서 `:0` 을 쓰기 위해서다.
  - 동기화 요청은 CLI 를 항목마다 호출해 오래 걸릴 수 있으므로 짧은 `WriteTimeout` 은 두지 않는다.
  - sentinel 은 `"server: ..."` 접두어.
- **main.go**:
  - env: `WHARF_ADDR` (`:8080`), `WHARF_DB_PATH` (`wharf.db`), `WHARF_ACLI_PATH`. `tool.NewTool` 은 PATH 를 탐색하지 않으므로(`internal/pkg/tool/tool.go:41`) `WHARF_ACLI_PATH` 가 없으면 `exec.LookPath("acli")` 로 찾는다.
  - 로거는 `slog` JSON handler (stdout, Info).
  - 순서: signal context → DB `Connect`/`Migrate` → tool → atlassian client → service → handler → server → `net.Listen` → `Run` → `Close`. 실패한 에러는 각각 로그로 남기고 exit 1.
- **AGENTS.md**: `internal/route` 경로 언급 세 곳(Architecture Overview, layering 줄, Boundaries)을 `internal/server` 로 고친다.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | `GET /healthz` 가 200 을 반환한다. | httptest 기반 route 테스트 |
| AC-2 | `GetMyTasks` 가 에러 없이 끝나면 반환된 task 가 모두 upsert 되고(기존 ID 덮어쓰기), 응답은 200 `{"upserted":N,"partial":false}` 이다. | `MockClient`/`MockManager` service 테스트, route 테스트 |
| AC-3 | tasks 와 에러가 함께 반환되면 반환된 task 를 모두 upsert 하고 200 `"partial":true` 를 반환하며, Warn 로그에 request id 가 남는다. | service 테스트, route 테스트(로그 버퍼 검사) |
| AC-4 | nil tasks 와 에러가 반환되거나 `UpsertTasks` 가 실패하면 500 이고 body 에 내부 메시지가 없다. 앞의 경우 `UpsertTasks` 는 호출되지 않는다. | service 테스트(mock 호출 여부), route 테스트 |
| AC-5 | `UpsertTasks([]task.Task)` 가 nil 날짜를 NULL 로, nil·빈 ID 목록을 `"[]"` 로, 값이 있는 목록을 JSON 배열로 저장한다. 하나라도 실패하면 아무것도 저장하지 않고 `errors.Is(err, db.ErrFailedToUpsertTask)` 다. | in-memory/파일 DB 테스트 |
| AC-6 | Go 코드와 `scripts/queries.sql` 에 `ExistsTask`, `CreateTasks` 가 남아 있지 않다(생성 코드·mock 포함). | `rg "ExistsTask\|CreateTasks" -g '*.go' -g '*.sql'` 결과 없음 |
| AC-7 | handler 에서 panic 이 나도 500 을 응답하고 다음 요청을 계속 처리한다. 모든 응답에 `X-Request-ID` 가 있고 들어온 값이면 같은 값이다. 요청마다 method, path, status, request id 가 로그에 남는다(panic 요청 포함). | 미들웨어 테스트 |
| AC-8 | ctx 를 취소해도 처리 중인 요청은 완료되고 `Run` 은 nil 을 반환한다. | `:0` listener 테스트 |
| AC-9 | `make run` 후 `curl /healthz` 200, `curl -X POST /tasks/jira` 는 acli 인증 상태면 200, 아니면 500 이다. SIGINT 로 exit 0 이다. | 구현 보고서에 curl 출력과 exit code 첨부 |
| AC-10 | `make deps-graph` 에 `server → adapter`, `service → server` 간선이 없다. | deps-graph 출력 |
| AC-11 | AGENTS.md 에 `internal/route` 경로 언급이 없다. | `rg "internal/route" AGENTS.md` 결과 없음 |

검증하지 않는 위험: 실제 Jira 를 상대로 한 검증은 수동(AC-9) 뿐이다. 부분 실패의 항목 수는 응답 `partial` 만으로 알 수 없고 로그로만 확인한다.

## 참고 Research

- [server-service-structure](../research/server-service-structure.md) — 뼈대 TODO, `db.Manager`/`atlassian.Client`/mock 계약, `GetMyTasks` 부분 실패 반환 규칙, `tool.NewTool` 의 경로 처리, `ExistsTask` 를 쓰는 테스트 목록, mockery 템플릿, lint·에러·테스트 관례, AGENTS.md 수정 위치.
- [sqlite-sqlc-setup](../research/sqlite-sqlc-setup.md) — sqlc SQLite 설정, Task → 컬럼 매핑, `make modelgen` 동작. 이 문서의 `ExistsTask` 서술은 구현 후 stale 이 되므로 구현 뒤 `application-research-sync` 로 갱신한다.

## Authority Boundaries

- Discretion: 타입·생성자·함수·결과 struct 이름과 정확한 파라미터 목록, healthz body, request ID 생성 방식(표준 라이브러리 안에서), shutdown timeout 값, 파일 분할, 테스트 헬퍼 구성.
- Must-ask: Key decisions 또는 Non-goals 변경, `atlassian` adapter 수정, 서드파티 의존성 추가, 이미 적용된 migration 수정, 실제 Jira 에 쓰기를 하는 동작.
- Stop conditions: `make gen` 이 도구 문제로 실패할 때, `GetMyTasks` 의 "nil 이면 전체 실패" 규칙이 코드와 다를 때.
- Loop budget: 3

## TODOs

- [x] `CreateTasks` → `UpsertTasks([]task.Task)` 로 바꾸고 변환을 구현. `ExistsTask` 메서드와 쿼리를 지우고 `make gen`. 영향받는 db 테스트를 직접 SQL 확인 방식으로 고침 (AC-5, AC-6) (mechanical) (→ research: sqlite-sqlc-setup, server-service-structure)
- [x] `service.Service` 와 Jira 동기화 함수를 부분 실패 규칙대로 구현. `.mockery.yml` 에 Service·Client 를 등록하고 `make gen` 후 mock 테스트 작성 (AC-2, AC-3, AC-4, AC-10) (design-bearing) (→ research: server-service-structure)
- [x] `route.go` 에 healthz 와 `POST /tasks/jira` handler 를 구현하고 `MockService` + httptest 로 테스트 (AC-1, AC-2, AC-3, AC-4) (mechanical) (→ research: server-service-structure)
- [x] `server.go` 에 미들웨어 세 개와 `Run`(graceful shutdown) 을 구현하고 테스트 (AC-7, AC-8) (mechanical) (→ research: server-service-structure)
- [x] `cmd/wharf/main.go` 에서 env, DB, tool, atlassian client, service, handler, server 를 wiring 하고 실행으로 확인 (AC-9, AC-10) (mechanical)
- [x] AGENTS.md 의 `internal/route` 경로 언급을 `internal/server` 로 수정 (AC-11) (mechanical)
