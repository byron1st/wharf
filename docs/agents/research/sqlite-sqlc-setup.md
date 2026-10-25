---
Application: wharf
ResearchType: Structure
Description: sqlc(SQLite 엔진) 설정 경로, Makefile modelgen 동작, Task 구조체→tasks 컬럼 매핑, sqlc SQLite 의 bulk insert 제약과 생성 코드 형태를 정리한다.
---

# sqlc + SQLite 초기 설정 현황

## 현재 코드 상태

- sqlc 설정·스키마·쿼리는 `scripts/` (`sqlc.yaml`, `schema_migrations.sql`, `migrations/`, `queries.sql`) 에 있고, 생성 코드(`db.go`, `models.go`, `queries.sql.go`) 는 손으로 쓴 `db.Manager` (`client.go`, `client_management.go`) 와 함께 `internal/adapter/db/` 에 있다.
- `cmd/wharf/main.go` 가 `db.NewManager(cfg.DBPath, scripts.Migrations())` 로 Manager 를 만들고 `Connect` → `Migrate` 후 service 에 넘긴다. `cfg.DBPath` 는 설정 파일의 `db_path` 이고, 비어 있으면 `internal/config` 가 `<os.UserConfigDir()>/wharf/wharf.db` 로 채우면서 디렉터리를 만든다.
- `go.mod` 의 직접 의존성은 `github.com/stretchr/testify`, SQLite 드라이버 `modernc.org/sqlite` (cgo 없는 순수 Go), 설정 파일 파서 `github.com/BurntSushi/toml` 이다.

## Makefile 의 sqlc 연동

- `Makefile:5` — `SQLC_CONFIG ?= ./scripts/sqlc.yaml`
- `Makefile:6` — `SQLC_DB_FILE ?= internal/adapter/db/db.go` → sqlc 출력 디렉터리는 `internal/adapter/db`, 패키지명은 `db` 로 맞춰야 한다.
- `Makefile:28-31` — `modelgen` 은 `sqlc generate -f ./scripts/sqlc.yaml` 후 `db.go` 에 한해 `interface{}`→`any` 치환과 `gofmt` 를 한다.
- sqlc.yaml 내부의 상대 경로(`schema`, `queries`, `out`)는 **sqlc.yaml 파일 위치(`scripts/`) 기준**이다. 따라서 `schema: ["schema_migrations.sql", "migrations"]`, `queries: queries.sql`, `out: ../internal/adapter/db`.
- `Makefile:8` — coverage EXCLUDE 는 `(mocks|cmd)` 로, `internal/adapter/db` 는 coverage 대상에 포함된다.

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
- 이 변환은 db 패키지의 `toUpsertTaskParams` (`internal/adapter/db/client.go:87`) 가 한다. 날짜는 nil 이면 NULL 인 `sql.NullTime`, ID 목록은 `json.Marshal` 한 문자열이며 nil·빈 목록은 `"[]"` 로 저장한다. 그래서 `db.Manager.UpsertTasks` 는 sqlc 모델이 아니라 `[]task.Task` 를 받는다.
- 생성 struct 필드명은 `ChildrenIds`, `LinkedIds` 처럼 sqlc 기본 규칙을 따른다 (rename 은 필요 시 sqlc.yaml `rename` 으로).

## sqlc SQLite 엔진 제약

- `:copyfrom` 은 PostgreSQL(pgx)·MySQL 전용이다 (sqlc 공식 문서 howto/insert). SQLite 에서는 bulk insert 를 위한 생성 코드가 없다.
- 가변 길이 multi-row `VALUES` 도 생성할 수 없다. `json_each(?)` 기반 단일 문장은 생성은 되지만 파라미터가 `interface{}` 로 나와 타입 안전성이 없다 (prototype 확인, 채택하지 않음).
- `SELECT EXISTS (...)` + `:one` 은 `bool` 을 반환하는 메서드로 생성된다. 현재는 마이그레이션 적용 여부 확인용 `ExistsSchemaMigration(ctx, version string) (bool, error)` (`internal/adapter/db/queries.sql.go:26`) 만 이 형태이고, task 를 읽는 쿼리는 없다.
- 생성 `db.go` 는 `DBTX` 인터페이스, `New(db DBTX) *Queries`, `(*Queries).WithTx(tx *sql.Tx) *Queries` 를 제공한다. 여러 건 저장은 `*sql.DB.BeginTx` → `WithTx` → 단건 `UpsertTask` (`INSERT ... ON CONFLICT (id) DO UPDATE`) 반복 → `Commit` 으로 구성되며, `db.Manager.UpsertTasks` 가 이 방식이다.
