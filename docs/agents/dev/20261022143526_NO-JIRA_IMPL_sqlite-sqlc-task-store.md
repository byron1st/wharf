---
Application: wharf
JiraTicket: NO-JIRA
ReportType: single-step
Timestamp: 20261022143526
Title: sqlite-sqlc-task-store
ReviewBase: `git diff 3b2ed5f` plus untracked `scripts/`, `internal/adapter/db/`, `internal/adapter/sqlite/`
---

# SQLite + sqlc 기반 tasks 저장소 초기 구성

Plan: [20261022143526_NO-JIRA_PLAN_sqlite-sqlc-task-store.md](./20261022143526_NO-JIRA_PLAN_sqlite-sqlc-task-store.md)

See the change: `git diff 3b2ed5f` (go.mod/go.sum) + `git status --short` 의 신규 디렉터리. Every `path:line` anchor in this report is valid against that snapshot.

## Summary
sqlc(SQLite 엔진) 설정과 tasks 스키마·쿼리를 추가하고, 생성 코드 위에 `task.Task` 목록을 하나의 트랜잭션으로 all-or-nothing 저장하고 ID 존재 여부를 조회하는 `internal/adapter/sqlite.TaskStore` 를 두었다. 드라이버 `modernc.org/sqlite` 는 테스트에서만 import 한다.

## Red Flags
- **RF1** `go.mod:7` - 신규 의존성 `modernc.org/sqlite v1.61.0` (+ modernc 간접 의존성 다수). 플랜 Key decisions 에서 승인된 유일한 추가 의존성이다.
- **RF2** `internal/adapter/db/db.go:13` - 생성 `db.go` 에 `interface{}` 가 그대로 남았다. `Makefile` modelgen 의 `-name '$$(basename ...)'` 가 작은따옴표 안이라 명령 치환이 되지 않아 치환 단계가 어떤 파일에도 매칭되지 않는 기존 Makefile 버그다. 이번 범위 밖이라 수정하지 않았다(lint 는 generated 파일을 건너뛰어 통과).

## Open Questions
- **OQ1** `internal/adapter/sqlite/task.go:83` - `StartDate`/`DueDate` 의 저장 후 재조회(round-trip) 형식은 검증하지 않았다. 조회 쿼리가 Non-goals 라 현재 확인 수단이 없으며, 향후 조회 쿼리 추가 시 modernc 드라이버의 DATETIME 문자열 형식·타임존 처리를 확인해야 한다.

## Plan Divergence
### Changed - details that differ from the plan
- None
### Added - implemented but absent from the plan
- `description`/`parent_id`/`children_ids`/`linked_ids` 컬럼에 DEFAULT(`''`, `'[]'`) 부여: Authority Boundaries 의 discretion 범위.
### Deferred - planned but not implemented (deferred)
- None

## TODO Fulfillment
### TODO 1: sqlc.yaml / initial_migration.sql / queries.sql 작성 및 `make modelgen` - done
- 구현: `scripts/sqlc.yaml:1` (engine sqlite, schema `migrations`, out `../internal/adapter/db`, package `db`), `scripts/migrations/initial_migration.sql:1` `tasks` 테이블 10컬럼, `scripts/queries.sql:1` `CreateTask :exec` / `ExistsTask :one`. 생성물: `internal/adapter/db/{db.go,models.go,queries.sql.go}`.
- 테스트: 생성 코드 자체는 TODO 3 의 adapter 테스트가 간접적으로 고정한다.
- AC: AC-1 — `make modelgen` 성공, `internal/adapter/db/models.go` 의 `Task` 가 ID~LinkedIds 10필드 보유
- 편차: none

### TODO 2: 별도 adapter 패키지에 트랜잭션 생성·존재 조회 구현 - done
- 구현: `internal/adapter/sqlite/task.go:32` `CreateTasks` — 빈 목록 no-op, `BeginTx` → `db.New(...).WithTx(tx)` → 단건 `CreateTask` 반복 → `Commit`, 실패 시 defer Rollback. `:59` `ExistsTask`. `:68` `toCreateTaskParams` 로 nil 시간은 `sql.NullTime{}`, nil 슬라이스는 `"[]"` 로 변환. sentinel 4개 `:16-19`, 각각 단일 origin.
- 테스트: `internal/adapter/sqlite/task_test.go` (TODO 3)
- AC: AC-2, AC-3, AC-4 — TODO 3 테스트
- 편차: none

### TODO 3: in-memory SQLite 테스트 - done
- 구현: `internal/adapter/sqlite/task_test.go:18` `newStore` — `:memory:` + `SetMaxOpenConns(1)` (연결별로 별도 DB 이므로), `initial_migration.sql` 을 읽어 적용.
- 테스트: `:44` `TestCreateTasks` (2건 생성 후 각 ID true, 미저장 ID false, nil 날짜·nil ID 슬라이스 Task 포함), `:76` `TestCreateTasksEmpty` (nil/빈 슬라이스 no-op), `:83` `TestCreateTasksRollsBackOnFailure` (중복 ID 혼합 시 `errors.Is(err, ErrFailedToCreateTask)`, 나머지 ID false).
- AC: AC-2 / AC-3 / AC-4 — `make test` 의 `ok github.com/byron1st/wharf/internal/adapter/sqlite`
- 편차: none
