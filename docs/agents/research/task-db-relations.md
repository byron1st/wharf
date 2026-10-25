---
Application: wharf
ResearchType: Structure
Description: tasks 테이블의 현재 스키마(JSON 배열 관계 컬럼), db.Manager 의 CreateTasks/ExistsTask 계약과 테스트 헬퍼, Jira adapter 가 ID·관계 ID 를 만드는 위치, sqlc SQLite 의 UPDATE…RETURNING·DROP COLUMN 생성 검증 결과와 codegen 도구 경로를 정리한다. Task ID 형식·관계 테이블 분리·local ID 카운터 작업의 출발점.
---

# Task DB 구조와 ID 생성 위치

> DEV-17 구현 후 상태: `0002_task_relations.sql` 이 관계 컬럼을 `task_parents`·`task_links` 로 옮기고 `task_counters` 를 추가했다. `CreateTasks` 는 `[]task.Task` 를 받고 `CreateLocalTask` 가 `local-WHF-NNN` 을 할당한다. Jira ID 는 `task.NewID` 로 `jira-{key}` 가 된다(`internal/pkg/task/id.go`). 아래 "현재 스키마"·"ID 가 만들어지는 곳" 절은 구현 전 상태 기록이다.

## 현재 스키마

- `scripts/migrations/0001_initial_migration.sql:1-12` — `tasks` 단일 테이블. `parent_id TEXT NOT NULL DEFAULT ''`, `children_ids`/`linked_ids` 는 JSON 배열 문자열(`DEFAULT '[]'`). 관계 테이블·카운터 테이블은 없다.
- `scripts/schema_migrations.sql` 은 migration 기록 테이블. `scripts/sqlc.yaml` 은 `schema: ["schema_migrations.sql", "migrations"]` 로 migration 파일 전체를 순서대로 읽어 최종 스키마를 만든다. 따라서 `ALTER TABLE ... DROP COLUMN` 으로 컬럼을 지우면 생성 `Task` 모델에서도 빠진다(아래 검증).
- `internal/adapter/db/models.go` 의 생성 `Task` 는 `ParentID`, `ChildrenIds`, `LinkedIds` 를 string 으로 가진다.
- FK 는 없고 `PRAGMA foreign_keys` 도 켜지 않는다(AGENTS.md: migration 안에서 무시됨).

## db.Manager 계약 (`internal/adapter/db/client.go`)

- `Manager` 인터페이스(`client.go:20-31`): `CreateTasks(ctx, []Task) error`, `ExistsTask(ctx, id) (bool, error)`, `Connect`/`Migrate`/`Close`.
- `CreateTasks`(`client.go:51-82`): 연결 확인 → 빈 목록 no-op → `BeginTx` → `queries.WithTx(tx)` 로 task 마다 `UpsertTask` → `Commit`. 실패 시 `fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToCreateTask, err), task.ID)`. 빈 JSON 은 `orEmptyJSONArray` 로 `"[]"` 저장.
- 입력 타입이 sqlc 모델 `db.Task` 라서 `UpsertTaskParams(task)` 타입 변환을 쓴다(`client.go:72`). 관계 컬럼이 빠지면 이 변환이 불가능하므로 입력 타입을 도메인 `task.Task`(`internal/pkg/task/task.go:5-16`) 로 바꿔야 한다.
- 트랜잭션 주의(`client_management.go:42-48`): busy_timeout 은 "읽고 나서 쓰는" DEFERRED 트랜잭션을 돕지 못하므로 트랜잭션은 쓰기로 시작한다. local ID 할당 트랜잭션도 카운터 UPDATE(쓰기)로 시작해야 한다.
- `Connect` 는 `SetMaxOpenConns(1)` (`client_management.go:65-67`).
- 사용처: `CreateTasks`/`ExistsTask` 를 호출하는 프로덕션 코드는 없다(`internal/service/service.go`, `cmd/wharf/main.go` 는 TODO/Hello world). 사용처는 테스트와 생성 mock(`internal/pkg/mocks/db_manager.go`) 뿐이다.

## 테스트 헬퍼 (`internal/adapter/db/client_test.go`)

- `newManager`(in-memory + 프로젝트 migration), `connect`, `newFileManager`(파일 DB, 경로 반환), `projectMigrationsWith`(프로젝트 migration + 추가 파일), `withMigrations`(`client_management_test.go:20-31`, schema_migrations.sql + 지정 파일만).
- `read`(`client_test.go:86-100`) 는 별도 연결로 `SELECT title, status, parent_id, children_ids, linked_ids FROM tasks` 를 직접 실행한다. 컬럼이 사라지므로 관계 테이블 조회로 바꿔야 한다.
- `newTask`(`client_test.go:110-112`) 는 `db.Task` 를 만든다. `client_management_test.go` 의 `TestMigrateKeepsDataAcrossReconnect`, `TestConnectWaitsForLockHeldByAnotherConnection`, `TestMethodsRequireConnection`, `TestConnectTwiceKeepsExistingConnection`, `TestConnectAfterClose` 도 `CreateTasks`/`newTask` 를 쓴다.
- `TestCreateTasksRollsBackOnFailure` 는 trigger migration(`9999_reject_task.sql`)으로 중간 실패를 만든다. 관계 테이블에도 같은 방식이 통한다.
- 0001 만 적용된 DB 에 행을 넣은 뒤 0002 를 적용하는 테스트는, `scripts.Migrations()` 에서 0001 만 복사한 MapFS 로 migrate → 직접 SQL insert → 같은 MapFS 에 0002 를 추가해 다시 `Migrate` 하는 방식으로 만들 수 있다(`TestMigrateRollsBackFailedMigration` 처럼 MapFS 를 수정 후 재호출).

## ID 가 만들어지는 곳

- `internal/adapter/atlassian/jira.go:57-111` `ToTask` — `ID: item.Key`, `ParentID = f.Parent.Key`, `ChildrenIDs` 는 subtasks key, `LinkedIDs` 는 issuelinks 의 inward/outward key. 모두 접두어 없는 Jira key 다.
- Jira 의 `parent` 필드는 subtask 의 부모뿐 아니라 epic → story 관계도 담는다. 반면 `subtasks` 에는 sub-task 만 있다. 즉 epic 의 `subtasks` 에는 story 가 없지만 story 의 `parent` 는 epic 이다. 부모 쪽 목록에 없다는 이유로 관계를 지우면 epic-story 관계가 사라진다.
- Jira 는 같은 이슈와 여러 종류의 link 를 가질 수 있어 `LinkedIDs` 에 같은 key 가 중복될 수 있다.
- 에러 메시지의 `key=` 와 CLI 호출(`client.go` `viewWorkItem`, `JiraViewOption`)은 원래 Jira key 를 써야 한다.
- 관련 테스트: `internal/adapter/atlassian/jira_test.go` (`TestToTask*`), `internal/adapter/atlassian/client_test.go` (`GetMyTasks` 결과 task 의 ID·관계 ID 기대값).
- `internal/pkg/task/origin.go` 의 `Origin` 상수(`jira`, `notion`, `gitlab`, `local`)는 untyped string 이다.

## sqlc 검증 (scratchpad prototype, sqlc v1.31.1)

- `UPDATE task_counters SET value = value + 1 WHERE origin = ? RETURNING value;` + `:one` → `func (q *Queries) NextTaskNumber(ctx, origin string) (int64, error)` 로 생성된다.
- `INSERT ... ON CONFLICT (child_id) DO UPDATE SET ...`, `INSERT ... ON CONFLICT DO NOTHING` 모두 생성된다.
- migration 의 `ALTER TABLE tasks DROP COLUMN parent_id;` 를 sqlc 가 반영해 생성 `Task` 에서 컬럼이 빠진다. 같은 migration 안의 `json_each`, `INSERT ... SELECT`, `UPDATE` 도 파싱 에러 없이 통과한다.
- 런타임(modernc.org/sqlite v1.61.0)은 SQLite 3.35+ 이므로 `RETURNING`, `DROP COLUMN` 을 지원한다(실행은 구현 시 테스트로 확인).

## Codegen 도구

- 이 cloud 세션에서는 `sqlc`(v1.31.1, 생성 코드 헤더와 같은 버전), `mockery` v3, Go 1.27.3 으로 빌드한 `golangci-lint` v2.15.0 을 `go install` 후 `/usr/local/bin` 에 복사해 두었다. 기본 설치된 golangci-lint v2.5.0 은 go1.25 빌드라 `go 1.27.3` 모듈을 lint 하지 못한다.
- `make gen` 은 mock 디렉터리를 지우고 다시 만든다(`Makefile` `mockgen`).
