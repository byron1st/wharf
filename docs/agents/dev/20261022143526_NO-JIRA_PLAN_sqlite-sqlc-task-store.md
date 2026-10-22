---
Application: wharf
JiraTicket: NO-JIRA
PlanType: single-step
Timestamp: 20261022143526
Title: sqlite-sqlc-task-store
---

Report: [20261022143526_NO-JIRA_IMPL_sqlite-sqlc-task-store.md](./20261022143526_NO-JIRA_IMPL_sqlite-sqlc-task-store.md)

# SQLite + sqlc 기반 tasks 저장소 초기 구성

## Non-goals

- 런타임 마이그레이션 실행기(migration runner) 도입, migration 버전 관리 도구(goose, golang-migrate 등) 도입.
- `cmd/wharf/main.go` 에서 DB 연결·wiring. DB 파일 경로 설정(env var) 추가.
- Task 조회(전체 필드 select)·수정·삭제·upsert 쿼리. 이번 쿼리는 생성과 ID 존재 여부 조회 두 가지뿐이다.
- `ChildrenIDs`/`LinkedIDs` 를 위한 별도 관계 테이블, 외래 키, `status`/`originated` CHECK 제약.
- service 계층 인터페이스·mockery 설정 추가.

## Key decisions

- **설정 파일 배치**: `scripts/sqlc.yaml` (engine `sqlite`, `schema: migrations`, `queries: queries.sql`, `out: ../internal/adapter/db`, package `db`), `scripts/queries.sql`, `scripts/migrations/initial_migration.sql`. 출력 경로는 `Makefile` 의 `SQLC_DB_FILE` 과 일치시킨다.
- **여러 건 생성 = 단건 INSERT + 트랜잭션** (사용자 결정). sqlc SQLite 엔진은 `:copyfrom`·가변 multi-row `VALUES` 를 지원하지 않는다. `queries.sql` 에는 단건 `CreateTask :exec` 만 두고, 수작업 adapter 가 하나의 트랜잭션 안에서 반복 호출해 all-or-nothing 으로 만든다. `json_each` 단일 문장 방식은 파라미터가 `interface{}` 로 생성되어 기각.
- **존재 여부 조회**: `SELECT EXISTS (...)` + `:one` 으로 `bool` 을 반환하는 `ExistsTask`.
- **컬럼 표현**: `ChildrenIDs`/`LinkedIDs` 는 JSON 배열 문자열(`TEXT NOT NULL`, nil 은 `[]`)로 저장. `StartDate`/`DueDate` 는 nullable `DATETIME`(→ `sql.NullTime`). `ParentID` 등 문자열은 `NOT NULL`, 빈 문자열이 "없음".
- **수작업 코드 위치**: 생성 코드 디렉터리(`internal/adapter/db`)와 분리된 별도 adapter 패키지(예: `internal/adapter/sqlite`)에 `task.Task` 도메인 타입을 받는 저장소를 둔다. 이 패키지가 `task.Task` ↔ 생성 params 변환, 트랜잭션, sentinel 에러를 담당한다.
- **드라이버**: `modernc.org/sqlite` (pure Go, cgo 불필요). 표준 라이브러리에 SQLite 드라이버가 없어 추가가 불가피하다. 이번 범위에서는 테스트(in-memory DB)에서만 import 한다.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | `scripts/sqlc.yaml`, `scripts/queries.sql`, `scripts/migrations/initial_migration.sql` 이 존재하고, sqlc.yaml 의 schema 가 `migrations` 디렉터리를 가리킨다. `make modelgen` 이 `internal/adapter/db/` 에 `db.go`, `models.go`, `queries.sql.go` 를 생성하며 `Task` 모델이 Task 구조체의 10개 필드를 모두 컬럼으로 가진다. | 파일 내용, `make modelgen` 성공 및 생성 파일 |
| AC-2 | 여러 Task 를 한 번에 생성하면 모두 저장되고, 각 ID 로 존재 여부 조회 시 `true`, 저장하지 않은 ID 는 `false` 를 반환한다. 빈 목록은 에러 없이 no-op 이다. | in-memory SQLite 에 initial_migration.sql 을 적용한 adapter 테스트 |
| AC-3 | 여러 건 생성 중 하나라도 실패하면(예: 기존 ID 와 중복) 해당 호출의 어떤 Task 도 저장되지 않는다 (all-or-nothing). 반환 에러는 adapter 패키지의 sentinel 로 `errors.Is` 식별 가능하다. | 중복 ID 를 섞은 테스트에서 나머지 ID 의 존재 여부가 `false` 이고 `errors.Is` 통과 |
| AC-4 | `StartDate`/`DueDate` 가 nil 인 Task, `ChildrenIDs`/`LinkedIDs` 가 nil 인 Task 도 생성에 성공한다. | 해당 케이스를 포함한 테스트 |

## 참고 Research

- [sqlite-sqlc-setup](../research/sqlite-sqlc-setup.md) — Makefile modelgen 경로, Task→컬럼 매핑, sqlc SQLite bulk insert 제약과 생성 코드 형태(prototype 검증).

## Authority Boundaries

- Discretion: adapter 패키지명·타입명·함수 시그니처, sentinel 이름, 테이블 컬럼의 DEFAULT 값, sqlc.yaml 의 부가 옵션(`emit_*`, `rename`), 테스트에서 마이그레이션 SQL 을 읽어 적용하는 방식.
- Must-ask: Key decisions 의 방식 변경(예: json_each, 관계 테이블), `modernc.org/sqlite` 외의 의존성 추가, `cmd/wharf/main.go` wiring, Non-goals 에 적힌 쿼리 추가.
- Stop conditions: `modernc.org/sqlite` 가 현재 Go 버전(`go 1.27.3`)과 호환되지 않거나 `make modelgen` 이 sqlc 설치·버전 문제로 실패할 때.
- Loop budget: 3

## TODOs

- [x] `scripts/sqlc.yaml`, `scripts/migrations/initial_migration.sql`(tasks 테이블), `scripts/queries.sql`(`CreateTask :exec`, `ExistsTask :one`)을 작성하고 `make modelgen` 으로 `internal/adapter/db` 를 생성 (AC-1) (mechanical) (→ research: sqlite-sqlc-setup)
- [x] 별도 adapter 패키지에 `task.Task` 목록을 하나의 트랜잭션으로 생성하는 기능과 ID 존재 여부 조회 기능을 구현 (AC-2, AC-3, AC-4) (mechanical) (→ research: sqlite-sqlc-setup)
- [x] `modernc.org/sqlite` in-memory DB 에 initial_migration.sql 을 적용해 생성·존재 조회·롤백·nil 필드 케이스를 검증하는 테스트 작성 (AC-2, AC-3, AC-4) (mechanical)
