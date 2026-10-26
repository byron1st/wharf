---
Application: wharf
ResearchType: Structure
Description: sqlc(SQLite 엔진) 설정 경로, Makefile modelgen 동작, tasks·task_hierarchies·task_links·counters·todos·task_todos·todo_reminders 스키마와 Task·Todo 구조체 매핑, sqlc SQLite 의 bulk insert 제약과 생성 코드 형태(UPDATE ... RETURNING, ON CONFLICT, :execrows, sqlc.slice, sqlc.embed, sqlc.narg, INSERT ... SELECT ... RETURNING, UNION)를 정리한다.
---

# sqlc + SQLite 초기 설정 현황

## 현재 코드 상태

- sqlc 설정·스키마·쿼리는 `scripts/` (`sqlc.yaml`, `schema_migrations.sql`, `migrations/`, `queries.sql`) 에 있고, 생성 코드(`db.go`, `models.go`, `queries.sql.go`) 는 손으로 쓴 `db.Manager` (`client.go`, `client_management.go`) 와 함께 `internal/adapter/db/` 에 있다.
- `cmd/wharf/main.go` 가 `db.NewManager(cfg.DBPath, scripts.Migrations())` 로 Manager 를 만들고 `Connect` → `Migrate` 후 service 에 넘긴다. `cfg.DBPath` 는 설정 파일의 `db_path` 이고, 비어 있으면 `internal/config` 가 `<os.UserConfigDir()>/wharf/wharf.db` 로 채우면서 디렉터리를 만든다.
- `go.mod` 의 직접 의존성은 `github.com/stretchr/testify`, SQLite 드라이버 `modernc.org/sqlite` (cgo 없는 순수 Go), 설정 파일 파서 `github.com/BurntSushi/toml` 이다.

## Makefile 의 sqlc 연동

- `Makefile:5` — `SQLC_CONFIG ?= ./scripts/sqlc.yaml`
- `Makefile:6` — `SQLC_DB_FILE ?= internal/adapter/db/db.go` → sqlc 출력 디렉터리는 `internal/adapter/db`, 패키지명은 `db` 로 맞춰야 한다.
- `Makefile:28-31` — `modelgen` 은 `sqlc generate -f ./scripts/sqlc.yaml` 후 `db.go` 에 한해 `interface{}`→`any` 치환과 `gofmt` 를 한다. `queries.sql.go` 는 치환 대상이 아니라 `ListTasksByIDs` 의 `[]interface{}` 가 그대로 남는다(generated 파일이라 lint 는 건너뛴다).
- sqlc.yaml 내부의 상대 경로(`schema`, `queries`, `out`)는 **sqlc.yaml 파일 위치(`scripts/`) 기준**이다. 따라서 `schema: ["schema_migrations.sql", "migrations"]`, `queries: queries.sql`, `out: ../internal/adapter/db`.
- `Makefile:8` — coverage EXCLUDE 는 `(mocks|cmd)` 로, `internal/adapter/db` 는 coverage 대상에 포함된다.

## Task 구조체 → 테이블 매핑

스키마는 `scripts/migrations/0001_initial_migration.sql` 하나다. 첫 배포 전까지는 소유자 결정에 따라 모든 스키마 변경을 새 마이그레이션 대신 0001 에 직접 반영하며(AGENTS.md 의 예외), 이미 0001 을 적용한 로컬 `wharf.db` 는 지우고 다시 만들어야 한다(`Migrate` 는 적용된 0001 을 다시 실행하지 않는다). `internal/pkg/task/task.go:5-16` 의 필드와 sqlc 가 생성하는 Go 타입 (`internal/adapter/db/models.go`):

| Task 필드 | 테이블.컬럼 | SQLite 선언 | 생성 Go 타입 |
| --- | --- | --- | --- |
| `ID string` | `tasks.id` | `TEXT PRIMARY KEY` | `string` |
| `Originated Origin` | `tasks.originated` | `TEXT NOT NULL` | `string` |
| `Title string` | `tasks.title` | `TEXT NOT NULL` | `string` |
| `Description string` | `tasks.description` | `TEXT NOT NULL` | `string` |
| `Status Status` | `tasks.status` | `TEXT NOT NULL` | `string` |
| `StartDate *time.Time` | `tasks.start_date` | `DATETIME` (nullable) | `sql.NullTime` |
| `DueDate *time.Time` | `tasks.due_date` | `DATETIME` (nullable) | `sql.NullTime` |
| `ParentID string` | `task_hierarchies(child_id = ID, parent_id)` | `child_id TEXT PRIMARY KEY` | `TaskHierarchy` |
| `ChildrenIDs []string` | `task_hierarchies(child_id, parent_id = ID)` | 같은 테이블 | `TaskHierarchy` |
| `LinkedIDs []string` | `task_links(task_id = ID, linked_id)` | `PRIMARY KEY (task_id, linked_id)` | `TaskLink` |

- `task.Task.ID` 는 `{originated}-{key}` 다 (`task.NewID`, `internal/pkg/task/id.go:10`). Jira 는 `jira-WHF-1`, local 은 `local-WHF-001` (`task.LocalKey`, `id.go:16`, `%03d` 이고 999 를 넘으면 자리수가 는다).
- `task_hierarchies` 는 `child_id` 가 PK 라 task 의 parent 는 하나다. 관계는 저장되지 않은 task 를 가리킬 수 있어서 `tasks(id)` FK 는 없다.
- `task_links` 는 선언한 task 기준의 방향 있는 행이다. 양방향 조회는 `ListTaskLinks` 가 `task_id = ? UNION linked_id = ?` 로 읽는다. 조회 인덱스 `task_hierarchies(parent_id)`, `task_links(linked_id)` 가 0001 에 있다.
- `counters (name TEXT PK, value INTEGER)` 는 이름 붙은 번호 카운터이고 0001 이 `('local_task', 0)` 을 seed 한다. 행이 없으면 `IncrementCounter` 가 `sql.ErrNoRows` 를 돌려준다.
- `Origin`/`Status` 는 `string` 기반 타입이고 상수는 untyped string 이다 (`internal/pkg/task/origin.go`, `internal/pkg/task/status.go`). 저장 시 `string(...)` 변환이면 된다.
- tasks 컬럼 변환은 db 패키지의 `toUpsertTaskParams` (`internal/adapter/db/client.go:337`) 가, 읽은 행 변환은 `fromTask` (`:358`, NULL 날짜 → nil) 가, 관계 행 쓰기는 `storeTask`/`storeRelations` (`client.go:289`/`:299`) 가 한다. 날짜는 nil 이면 NULL 인 `sql.NullTime` 이다. 그래서 `db.Manager.UpsertTasks` 는 sqlc 모델이 아니라 `[]task.Task` 를 받는다.

## Todo 구조체 → 테이블 매핑 (DEV-18)

| Todo 필드 | 테이블.컬럼 | SQLite 선언 | 생성 Go 타입 |
| --- | --- | --- | --- |
| `ID int64` | `todos.id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | `int64` |
| `Description string` | `todos.description` | `TEXT NOT NULL` | `string` |
| `IsDone bool` | `todos.is_done` | `BOOLEAN NOT NULL DEFAULT 0` | `bool` |
| `StartDate`/`EndDate string` | `todos.start_date`/`end_date` | `TEXT` (nullable, `YYYY-MM-DD`) | `sql.NullString` |
| `TaskID string`, `Position *int` | `task_todos(todo_id PK, task_id, position)` | 인덱스 `(task_id, position)`, unique 아님 | `TaskTodo` |
| `Reminders []time.Time` | `todo_reminders(todo_id, remind_at)` | `PRIMARY KEY (todo_id, remind_at)`, `DATETIME` | `TodoReminder` |

- 날짜를 TEXT 로 두어 사전순 비교가 날짜 비교이고 시간대 변환이 없다. 빈 문자열 ↔ NULL 변환은 `toNullString` (`internal/adapter/db/todo.go`). position 은 task 안에서 0..n-1 이며 한 번의 `UPDATE position = position ± 1` 로 밀고 당기므로 unique 제약을 두지 않았다. FK 는 없다.
- reminder 는 service 가 UTC 로 정규화해 넘기므로 저장 텍스트가 시각 순으로 정렬된다(`ListTodoReminders` 의 `ORDER BY remind_at`). `ListTodoReminders` 는 todo ID 목록(`sqlc.slice`) 대신 `ListTodos` 와 같은 `sqlc.narg` 필터로 JOIN 해 읽는다. ID 목록을 바인드하면 todo 수가 SQLite 바인드 변수 한도(`SQLITE_MAX_VARIABLE_NUMBER`, 기본 32766)를 넘을 때 `too many SQL variables` 로 실패하기 때문이다. 두 쿼리의 WHERE 조건은 같게 유지해야 하고, 생성 struct 가 같아 `ListTodoRemindersParams(params)` 로 변환한다.

## sqlc SQLite 엔진 제약

- `:copyfrom` 은 PostgreSQL(pgx)·MySQL 전용이다 (sqlc 공식 문서 howto/insert). SQLite 에서는 bulk insert 를 위한 생성 코드가 없다.
- 가변 길이 multi-row `VALUES` 도 생성할 수 없다. `json_each(?)` 기반 단일 문장은 생성은 되지만 파라미터가 `interface{}` 로 나와 타입 안전성이 없다 (prototype 확인, 채택하지 않음).
- `SELECT EXISTS (...)` + `:one` 은 `bool` 을 반환하는 메서드로 생성된다. 현재는 마이그레이션 적용 여부 확인용 `ExistsSchemaMigration(ctx, version string) (bool, error)` 만 이 형태다.
- `UPDATE counters SET value = value + 1 WHERE name = ? RETURNING value` + `:one` 은 `IncrementCounter(ctx, name string) (int64, error)` 로 생성된다.
- `:execrows` 는 `(int64, error)` 로 영향 행 수를 돌려준다(`UpdateLocalTask`, `DeleteTask`). 쓰기로 시작하는 트랜잭션에서 존재 여부를 판별하는 데 쓴다.
- `sqlc.slice(ids)` 는 `IN (/*SLICE:ids*/?)` 를 실행 시 `?,?,...` 로 바꾸고, 빈 slice 면 `NULL` 로 바꿔 0행이 된다(`ListTasksByIDs`).
- `sqlc.narg(x)` 는 `x IS NULL` 쪽이 먼저 나오면 타입 추론이 안 돼 `interface{}` 가 된다. 컬럼 비교를 먼저 쓰면(`todos.end_date = sqlc.narg(end_date) OR sqlc.narg(end_date) IS NULL`) `sql.NullString`/`sql.NullBool` 로 생성된다(`ListTodos`).
- `INSERT INTO ... SELECT ... WHERE EXISTS (...) RETURNING id` + `:one` 은 조건이 거짓이면 `sql.ErrNoRows` 를 돌려준다(`CreateTodo`). `DELETE ... RETURNING a, b` + `:one` 은 행 struct(`DeleteTaskTodoRow`)를 돌려준다.
- `sqlc.embed(tasks)` 는 행 struct 에 `Task Task` 필드로 들어간다. `LEFT JOIN` 한 nullable 컬럼은 `sql.NullString` 이다(`GetTaskRow`).
- 같은 `sqlc.arg(x)` 를 여러 번 쓰면 `?1` 로 번호 매겨진 하나의 파라미터가 된다. `UNION` 쿼리에서 `ORDER BY 1` 이나 테이블을 한정하지 않은 컬럼은 sqlc 가 "column reference is ambiguous" 로 거부해 `AS id` 별칭·`task_links.` 한정이 필요하다(`ListTaskLinks`).
- `ON CONFLICT (...) DO UPDATE` / `DO NOTHING` 도 그대로 생성된다: `UpsertTaskHierarchy` (child 의 parent 재지정), `CreateTaskLink` (중복 링크 무시).
- 생성 `db.go` 는 `DBTX` 인터페이스, `New(db DBTX) *Queries`, `(*Queries).WithTx(tx *sql.Tx) *Queries` 를 제공한다. 여러 건 저장은 `*sql.DB.BeginTx` → `WithTx` → task 마다 `UpsertTask` (`INSERT ... ON CONFLICT (id) DO UPDATE`) 와 관계 쿼리(`DeleteTaskParent`, `UpsertTaskHierarchy`, `DeleteTaskLinks`, `CreateTaskLink`) 반복 → `Commit` 으로 구성되며, `db.Manager` 의 쓰기 메서드(`UpsertTasks`·`CreateLocalTask`·`UpdateLocalTask`·`DeleteTask`)가 공유하는 `inTx` (`client.go:268`) 가 begin/commit 을 맡는다.
