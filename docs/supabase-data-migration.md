# SQLite → Supabase 데이터 이전

이전 버전 wharf가 쓰던 로컬 SQLite 파일(`wharf.db`)의 데이터를 Supabase Postgres의 `wharf` 스키마로 한 번 옮기는 절차입니다. 모든 명령은 macOS에서 데이터가 있는 기기 한 대에서만 실행합니다. 다른 기기는 마지막 단계에서 설정만 맞추면 됩니다.

## 0. 준비물

- Supabase 프로젝트와 그 안의 `wharf` 스키마. 스키마가 없어도 wharf가 처음 시작할 때 만듭니다.
- Supabase 대시보드 Connect › Session pooler의 연결 URL(포트 5432)과 DB 비밀번호. Transaction pooler(6543)는 쓰지 않습니다.
- `sqlite3` CLI(macOS 기본 포함)와 `psql`(`brew install libpq` 후 `brew link --force libpq`, 또는 `brew install postgresql@17`).
- 이 문서의 명령은 아래 변수를 쓴다고 가정합니다. 경로와 URL을 자신의 값으로 바꾸세요. URL에는 비밀번호를 넣지 않습니다.

```sh
export WHARF_DB="$HOME/Library/Application Support/wharf/wharf.db"
export DB_URL="postgresql://postgres.<project-ref>@aws-0-<region>.pooler.supabase.com:5432/postgres?sslmode=require"
export WORK="$HOME/wharf-migration"
mkdir -p "$WORK"
```

## 1. 비밀번호를 Keychain에 등록

wharf는 시작할 때 로그인 키체인에서 서비스 이름이 `wharf-db`인 일반 암호를 읽습니다. `-w` 뒤에 값을 주지 않으면 프롬프트로 입력받으므로 셸 기록에 남지 않습니다.

```sh
security add-generic-password -a "$USER" -s wharf-db -w
```

이미 있으면 `-U`를 붙여 덮어씁니다. 이 문서의 `psql` 명령도 같은 항목에서 비밀번호를 가져옵니다.

```sh
export PGPASSWORD="$(security find-generic-password -s wharf-db -w)"
```

## 2. 기존 wharf를 멈추고 원본을 보관

모든 기기에서 이전 버전 wharf를 멈춥니다. LaunchAgent로 띄웠다면 `wharf service uninstall`을, 터미널에서 띄웠다면 `Ctrl+C`를 씁니다. 그다음 원본 파일을 복사해 둡니다. 이후 단계는 원본이 아닌 복사본에서 읽습니다.

```sh
cp "$WHARF_DB" "$WORK/wharf.db"
```

## 3. 새 wharf로 스키마 만들기

설정 파일의 `db_path` 줄을 지우고 `db_url`을 추가합니다. `db_path`가 남아 있으면 새 버전이 시작을 거부합니다.

```toml
db_url = "postgresql://postgres.<project-ref>@aws-0-<region>.pooler.supabase.com:5432/postgres?sslmode=require"
```

새 버전 wharf를 한 번 실행합니다. 로그에 `server started`가 보이면 `Ctrl+C`로 멈춥니다. 이때 `wharf` 스키마에 테이블이 만들어지고 마이그레이션이 기록됩니다.

```sh
./wharf
```

## 4. SQLite에서 CSV로 내보내기

테이블마다 열 순서를 명시해 CSV로 내보냅니다. `counters`와 `schema_migrations`는 내보내지 않습니다. `counters`는 5단계에서 값만 옮기고, `schema_migrations`는 3단계에서 새로 기록됐습니다.

이전 wharf는 시각을 Go의 `time.Time.String()` 형식(`2026-10-01 00:30:00 +0000 UTC`)으로 저장했습니다. Postgres는 끝의 존 이름(`UTC`, `KST` 등)을 읽지 못하므로, `ts`가 오프셋까지만 남기고 잘라냅니다. 그 형식이 아닌 값은 그대로 둡니다.

```sh
cd "$WORK"
export_table() { sqlite3 -header -csv wharf.db "SELECT $2 FROM $1" > "$1.csv"; }
ts() {
  echo "CASE WHEN instr($1, ' +') > 0 THEN substr($1, 1, instr($1, ' +') + 5)" \
       "WHEN instr($1, ' -') > 0 THEN substr($1, 1, instr($1, ' -') + 5) ELSE $1 END AS $1"
}
export_table tasks "id, originated, title, description, status, $(ts start_date), $(ts due_date), position"
export_table task_hierarchies "child_id, parent_id"
export_table task_links "task_id, linked_id"
export_table todos "id, description, note, is_done, $(ts completed_at), start_date, end_date"
export_table task_todos "todo_id, task_id, position"
export_table todo_reminders "todo_id, $(ts remind_at)"
export_table service_desk_requests "issue_key, title, status, status_category, $(ts created_at), $(ts status_at)"
export_table todo_service_desk_requests "todo_id, issue_key"
export_table atlassian_syncs \
  "id, $(ts synced_at), issues, issue_failures, open_requests, closed_requests, request_failures"
sqlite3 wharf.db "SELECT value FROM counters WHERE name = 'local_task'" > local_task.txt
```

나머지 값은 바꾸지 않아도 Postgres가 그대로 읽습니다.

- 오프셋이 붙은 시각(`2026-10-01 00:30:00 +0000`)은 `timestamptz`로 읽힙니다. 나노초는 마이크로초로 반올림됩니다.
- `is_done`의 `0`/`1`은 `boolean`으로 읽힙니다.
- 빈 칸은 `NULL`이 됩니다. 빈 문자열이 들어갈 수 있는 `NOT NULL` 열(`description`, `note`)은 아래 `FORCE_NOT_NULL`로 빈 문자열을 유지합니다.
- todo의 `start_date`/`end_date`는 `YYYY-MM-DD` 텍스트 그대로 옮깁니다.

## 5. Supabase로 가져오기

아래 스크립트를 `$WORK/import.sql`로 저장합니다. 이 스크립트는 하나의 트랜잭션 안에서 실행됩니다.

- 3단계에서 wharf가 이미 무언가를 썼을 수 있으므로, 데이터 테이블을 먼저 비웁니다. 그래서 실패하면 고친 뒤 다시 실행해도 됩니다.
- identity 시퀀스를 가져온 최대 ID 다음으로 맞춥니다. 그래야 새 todo와 sync 기록의 ID가 겹치지 않습니다.
- 로컬 task 번호 카운터를 옮깁니다. 그래야 다음 로컬 task가 기존 `local-WHF-{n}`과 겹치지 않습니다.

```sql
TRUNCATE wharf.tasks, wharf.task_hierarchies, wharf.task_links, wharf.todos, wharf.task_todos,
    wharf.todo_reminders, wharf.service_desk_requests, wharf.todo_service_desk_requests, wharf.atlassian_syncs;

\copy wharf.tasks FROM 'tasks.csv' WITH (FORMAT csv, HEADER true, FORCE_NOT_NULL (description))
\copy wharf.task_hierarchies FROM 'task_hierarchies.csv' WITH (FORMAT csv, HEADER true)
\copy wharf.task_links FROM 'task_links.csv' WITH (FORMAT csv, HEADER true)
\copy wharf.todos FROM 'todos.csv' WITH (FORMAT csv, HEADER true, FORCE_NOT_NULL (description, note))
\copy wharf.task_todos FROM 'task_todos.csv' WITH (FORMAT csv, HEADER true)
\copy wharf.todo_reminders FROM 'todo_reminders.csv' WITH (FORMAT csv, HEADER true)
\copy wharf.service_desk_requests FROM 'service_desk_requests.csv' WITH (FORMAT csv, HEADER true)
\copy wharf.todo_service_desk_requests FROM 'todo_service_desk_requests.csv' WITH (FORMAT csv, HEADER true)
\copy wharf.atlassian_syncs FROM 'atlassian_syncs.csv' WITH (FORMAT csv, HEADER true)

SELECT setval(pg_get_serial_sequence('wharf.todos', 'id'), COALESCE(MAX(id), 0) + 1, false) FROM wharf.todos;
SELECT setval(pg_get_serial_sequence('wharf.atlassian_syncs', 'id'), COALESCE(MAX(id), 0) + 1, false)
FROM wharf.atlassian_syncs;

UPDATE wharf.counters SET value = :local_task WHERE name = 'local_task';
```

실행합니다. 오류가 하나라도 나면 전체가 롤백됩니다.

```sh
cd "$WORK"
psql "$DB_URL" -v ON_ERROR_STOP=1 -v local_task="$(cat local_task.txt)" --single-transaction -f import.sql
```

## 6. 행 수로 검증

양쪽에서 같은 쿼리로 테이블별 행 수와 카운터를 뽑아 비교합니다. `diff`가 아무것도 출력하지 않으면 일치합니다.

```sh
cd "$WORK"
COUNTS="SELECT 'tasks', COUNT(*) FROM tasks
UNION ALL SELECT 'task_hierarchies', COUNT(*) FROM task_hierarchies
UNION ALL SELECT 'task_links', COUNT(*) FROM task_links
UNION ALL SELECT 'todos', COUNT(*) FROM todos
UNION ALL SELECT 'task_todos', COUNT(*) FROM task_todos
UNION ALL SELECT 'todo_reminders', COUNT(*) FROM todo_reminders
UNION ALL SELECT 'service_desk_requests', COUNT(*) FROM service_desk_requests
UNION ALL SELECT 'todo_service_desk_requests', COUNT(*) FROM todo_service_desk_requests
UNION ALL SELECT 'atlassian_syncs', COUNT(*) FROM atlassian_syncs
UNION ALL SELECT 'counter:local_task', value FROM counters WHERE name = 'local_task'"
sqlite3 -separator '|' wharf.db "$COUNTS" > counts.sqlite.txt
psql "$DB_URL" -Atq -c "SET search_path TO wharf" -c "$COUNTS" > counts.postgres.txt
diff counts.sqlite.txt counts.postgres.txt
```

행 수가 같으면 wharf를 실행해 Today 화면과 todo 몇 개를 열어 보고, 날짜와 완료 여부가 예전과 같은지 눈으로 확인합니다.

## 7. Data API에서 숨기기

wharf는 SQL로만 접근하므로 Supabase Data API(PostgREST)로 `wharf` 스키마가 보이면 안 됩니다.

1. 대시보드 Project Settings › Data API(또는 API)의 Exposed schemas에 `wharf`가 없는지 확인합니다. 기본값은 `public`, `graphql_public`뿐입니다.
2. SQL Editor에서 아래를 실행해 API 역할의 권한을 회수하고, 앞으로 만들어질 테이블에도 적용되게 합니다.

```sql
REVOKE ALL ON SCHEMA wharf FROM anon, authenticated;
REVOKE ALL ON ALL TABLES IN SCHEMA wharf FROM anon, authenticated;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA wharf FROM anon, authenticated;
ALTER DEFAULT PRIVILEGES IN SCHEMA wharf REVOKE ALL ON TABLES FROM anon, authenticated;
ALTER DEFAULT PRIVILEGES IN SCHEMA wharf REVOKE ALL ON SEQUENCES FROM anon, authenticated;
```

3. Security Advisor의 RLS 경고까지 없애려면, 정책 없이 RLS를 켭니다. wharf가 접속하는 `postgres` 역할은 테이블 소유자라서 영향을 받지 않습니다. 새 마이그레이션이 테이블을 추가하면 그 테이블에도 다시 실행합니다.

```sql
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = 'wharf' LOOP
        EXECUTE format('ALTER TABLE wharf.%I ENABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
```

## 8. 다른 기기 설정

다른 기기마다 새 버전 wharf를 설치하고, 1단계의 `security add-generic-password`를 실행하고, 설정 파일의 `db_path`를 지우고 같은 `db_url`을 넣은 뒤 시작합니다. Settings 패널의 Database 항목에 Supabase 호스트와 DB 이름이 보이면 됩니다.

## 되돌리기

원본 `wharf.db`와 이전 버전 바이너리는 그대로 남아 있습니다. 문제가 생기면 모든 기기에서 새 wharf를 멈추고, 이전 바이너리와 `db_path` 설정으로 돌아가면 됩니다. 이전 이후 Supabase에만 쓴 데이터는 SQLite로 돌아가지 않습니다.
