---
Application: wharf
ResearchType: Structure
Description: sqlc(SQLite 엔진) 설정 경로, Makefile modelgen 동작, Task 구조체→tasks 컬럼 매핑, sqlc SQLite 의 bulk insert 제약과 생성 코드 형태를 정리한다.
---

# sqlc + SQLite 초기 설정 현황

## 현재 코드 상태

- DB 관련 코드·설정은 아직 없다. `scripts/`, `internal/adapter/db/` 디렉터리 모두 존재하지 않는다.
- `cmd/wharf/main.go` 는 `Hello world` 출력만 한다 (wiring 없음).
- `go.mod` 의 직접 의존성은 `github.com/stretchr/testify` 뿐이다. SQLite 드라이버는 없다.

## Makefile 의 sqlc 연동

- `Makefile:5` — `SQLC_CONFIG ?= ./scripts/sqlc.yaml`
- `Makefile:6` — `SQLC_DB_FILE ?= internal/adapter/db/db.go` → sqlc 출력 디렉터리는 `internal/adapter/db`, 패키지명은 `db` 로 맞춰야 한다.
- `Makefile:30-33` — `modelgen` 은 `sqlc generate -f ./scripts/sqlc.yaml` 후 `db.go` 에 한해 `interface{}`→`any` 치환과 `gofmt` 를 한다.
- sqlc.yaml 내부의 상대 경로(`schema`, `queries`, `out`)는 **sqlc.yaml 파일 위치(`scripts/`) 기준**이다. 따라서 `schema: migrations`, `queries: queries.sql`, `out: ../internal/adapter/db`.
- `Makefile:10` — coverage EXCLUDE 는 `(mocks|docs|cmd)` 로, `internal/adapter/db` 는 coverage 대상에 포함된다.

## Task 구조체 → tasks 컬럼 매핑

`internal/pkg/task/task.go:5-16` 의 필드와 sqlc 가 생성하는 Go 타입 (sqlc v1.31.1 로 /tmp 에서 prototype 검증):

| Task 필드 | 컬럼 | SQLite 선언 | 생성 Go 타입 |
| --- | --- | --- | --- |
| `ID string` | `id` | `TEXT PRIMARY KEY` | `string` |
| `Originated Origin` | `originated` | `TEXT NOT NULL` | `string` |
| `Title string` | `title` | `TEXT NOT NULL` | `string` |
| `Description string` | `description` | `TEXT NOT NULL` | `string` |
| `Status Status` | `status` | `TEXT NOT NULL` | `string` |
| `StartDate *time.Time` | `start_date` | `DATETIME` (nullable) | `sql.NullTime` |
| `DueDate *time.Time` | `due_date` | `DATETIME` (nullable) | `sql.NullTime` |
| `ParentID string` | `parent_id` | `TEXT NOT NULL` | `string` |
| `ChildrenIDs []string` | `children_ids` | `TEXT NOT NULL` (JSON 배열) | `string` |
| `LinkedIDs []string` | `linked_ids` | `TEXT NOT NULL` (JSON 배열) | `string` |

- `Origin`/`Status` 는 `string` 기반 타입이고 상수는 untyped string 이다 (`internal/pkg/task/origin.go`, `internal/pkg/task/status.go`). 저장 시 `string(...)` 변환이면 된다.
- 생성 struct 필드명은 `ChildrenIds`, `LinkedIds` 처럼 sqlc 기본 규칙을 따른다 (rename 은 필요 시 sqlc.yaml `rename` 으로).

## sqlc SQLite 엔진 제약

- `:copyfrom` 은 PostgreSQL(pgx)·MySQL 전용이다 (sqlc 공식 문서 howto/insert). SQLite 에서는 bulk insert 를 위한 생성 코드가 없다.
- 가변 길이 multi-row `VALUES` 도 생성할 수 없다. `json_each(?)` 기반 단일 문장은 생성은 되지만 파라미터가 `interface{}` 로 나와 타입 안전성이 없다 (prototype 확인, 채택하지 않음).
- `SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?)` + `:one` 은 `func (q *Queries) ExistsTask(ctx, id string) (bool, error)` 로 생성된다 (prototype 확인).
- 생성 `db.go` 는 `DBTX` 인터페이스, `New(db DBTX) *Queries`, `(*Queries).WithTx(tx *sql.Tx) *Queries` 를 제공한다. 여러 건 생성은 `*sql.DB.BeginTx` → `WithTx` → 단건 insert 반복 → `Commit` 으로 구성할 수 있다.
