---
Application: wharf
JiraTicket: DEV-17
PlanType: single-step
Timestamp: 20261025131942
Title: task-db-relations
---

# Task DB 개선: 관계 테이블 분리, `{originated}-{key}` ID, local ID 카운터

Notion 개발 이슈 보드 DEV-17. `tasks` 의 JSON 배열 관계 컬럼을 별도 테이블로 옮기고, task ID 를 출처 접두어가 붙은 형식으로 바꾸며, local task 에 증가 번호 ID 를 할당하는 경로를 만든다.

## Non-goals

- task 조회 쿼리·API(`GET /tasks` 등), task 삭제. 별도 이슈("Task 관련 API 들 추가")에서 다룬다.
- `CreateTasks` → `UpsertTasks` 이름 변경과 `ExistsTask` 제거. `20261024004228_NO-JIRA_PLAN_server-service-structure` 계획이 맡는다. 이번에는 `CreateTasks` 의 입력 타입만 바꾼다.
- service·server·`cmd/wharf` wiring.
- origin·status·ID 접두어 검증. 호출자가 만든 ID 를 그대로 저장한다.
- 관계 테이블의 FK·cascade, 동기화에서 빠진(완료·삭제된) task 의 관계 정리.
- Notion·GitLab adapter.
- 이미 적용된 `0001_initial_migration.sql` 수정.

## Key decisions

- **ID 형식**: 모든 task ID 는 `{originated}-{key}`. 조합 규칙은 도메인 패키지 `internal/pkg/task` 에 한 곳으로 둔다.
  - Jira: `jira-BLC-000`. `atlassian.ToTask` 가 `ID`, `ParentID`, `ChildrenIDs`, `LinkedIDs` 모두에 접두어를 붙인다(관계 대상도 같은 출처라고 본다). CLI 호출과 에러 메시지의 `key=` 는 원래 Jira key 를 유지한다.
  - local: key 는 `WHF-{번호}`, 번호는 3자리 zero-padding(`%03d`, 999 초과 시 자릿수가 늘어남). 전체 ID 는 규칙대로 `local-WHF-001`. (사용자 확인 대신 선택: 이슈 본문의 `WHF-001` 을 key 로 해석해 모든 origin 에 같은 규칙을 적용. 전체 ID 를 `WHF-001` 로 두는 대안은 origin 별 예외가 생겨 기각.)
- **parent-children**: `task_parents(child_id TEXT PRIMARY KEY, parent_id TEXT NOT NULL)` 한 테이블. task 하나는 부모를 최대 하나 가진다.
  - task X 를 저장할 때 X 의 `ParentID` 가 있으면 `(X → ParentID)` upsert, 비어 있으면 X 의 부모 행 삭제. X 의 `ChildrenIDs` 각각 `(c → X)` upsert.
  - 부모 쪽 목록에 없다는 이유로 자식 행을 지우지 않는다. Jira 에서 epic 의 `subtasks` 에는 story 가 없지만 story 의 `parent` 는 epic 이라, 지우면 epic-story 관계가 사라진다. 같은 배치에서 주장이 엇갈리면 나중 것이 이긴다.
  - 기각: `(parent_id, child_id)` 다대다 테이블 + "X 저장 시 X 가 걸린 행 전부 교체" — 저장 순서에 따라 epic-story 관계가 지워진다.
- **linked**: `task_links(task_id, linked_id, PRIMARY KEY(task_id, linked_id))`. 행은 그 task 의 주장이다. X 저장 시 `task_id = X` 행을 모두 지우고 `LinkedIDs` 로 다시 넣는다. 같은 대상 중복(Jira 는 한 이슈와 여러 종류의 link 가능)은 무시한다. 대칭 정규화(min/max 쌍)는 한쪽 주장 철회를 표현할 수 없어 기각.
- **FK 없음**: 관계 대상(내게 할당되지 않은 epic, 완료된 subtask 등)이 `tasks` 에 없을 수 있다.
- **local 카운터**: 이슈 제안대로 카운터 테이블 `task_counters(origin TEXT PRIMARY KEY, value INTEGER NOT NULL)` 에 `('local', 0)` 을 seed 하고 `UPDATE ... SET value = value + 1 ... RETURNING value` 로 번호를 얻는다. 트랜잭션이 쓰기로 시작하므로 busy_timeout 주의사항(`client_management.go`)과 맞고, 번호는 재사용되지 않는다. 기각: `MAX(번호)+1`(삭제 후 재사용, 읽고-쓰기 트랜잭션), 별도 AUTOINCREMENT 테이블(번호만 쌓이는 테이블이 생기고 의도가 덜 드러남).
- **Manager API**:
  - `CreateTasks(ctx, []task.Task) error` — 입력을 도메인 타입으로 바꾸고 task 행과 관계 행을 한 트랜잭션에 저장한다. 기존 계약(all-or-nothing, 기존 ID 덮어쓰기, 빈 목록 no-op, 실패 시 `ErrFailedToCreateTask` + `id=`)은 유지. `*time.Time` ↔ `sql.NullTime` 변환은 db 패키지가 맡는다.
  - local task 생성 메서드 추가(이름은 재량, 예: `CreateLocalTask(ctx, task.Task) (string, error)`). 한 트랜잭션에서 번호 할당 → ID·`Originated=local` 설정 → task·관계 저장 후 할당된 ID 를 반환. 실패하면 번호 증가도 롤백된다. 입력의 `ID`·`Originated` 는 무시한다.
- **migration**: 새 `0002_*.sql` 하나로 관계 테이블·카운터 테이블 생성, 기존 행 변환(관계 JSON → 관계 테이블, 관계 ID·task ID 에 `originated || '-'` 접두), `tasks` 의 `parent_id`/`children_ids`/`linked_ids` 컬럼 삭제(`ALTER TABLE ... DROP COLUMN`, sqlc 반영 확인됨). 기존 children 변환이 parent 주장과 충돌하면 task 자신의 parent 주장을 우선한다.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | 새 DB 에 migration 을 적용하면 `tasks` 에 `parent_id`·`children_ids`·`linked_ids` 컬럼이 없고, `task_parents`·`task_links`·`task_counters` 가 있으며 `task_counters` 에 `('local', 0)` 이 있다. 생성 모델 `db.Task` 에도 관계 필드가 없다. | 파일 DB 테스트(`pragma_table_info`/직접 조회), `internal/adapter/db/models.go` |
| AC-2 | 0001 만 적용된 DB 의 기존 행(부모·자식·link 가 있는 행 포함)에 0002 를 적용하면 task ID 가 `{originated}-{원래 id}` 가 되고 관계가 접두어 붙은 ID 로 관계 테이블에 옮겨진다. | migration 테스트(0001 적용 → 직접 insert → 0002 적용 → 조회) |
| AC-3 | `CreateTasks([]task.Task)` 가 task 필드(nil 날짜는 NULL)와 부모 행, `(child → X)` 행, link 행을 저장한다. 다시 저장하면 X 의 부모·link 가 새 값으로 바뀌고(빈 부모·빈 link 는 삭제), X 의 `ChildrenIDs` 에 없는 기존 자식 행은 남는다. 중복 link 는 실패하지 않는다. | in-memory/파일 DB 테스트 |
| AC-4 | 관계 행 저장 중 하나라도 실패하면 그 호출의 task·관계 변경이 전부 롤백되고 `errors.Is(err, db.ErrFailedToCreateTask)` 계열 sentinel 과 `id=` 가 에러에 있다. | trigger migration 으로 관계 insert 를 실패시키는 테스트 |
| AC-5 | local task 생성이 `local-WHF-001`, `local-WHF-002` 순으로 ID 를 반환하고 `originated = 'local'` 로 저장한다. 재연결 후에도 번호가 이어지고, 저장이 실패한 호출은 번호를 소비하지 않는다. | 파일 DB 테스트(재연결, 실패 후 다음 ID 확인) |
| AC-6 | `atlassian.ToTask` 가 `ID`·`ParentID`·`ChildrenIDs`·`LinkedIDs` 를 `jira-{key}` 로 반환하고, 에러 메시지의 `key=` 는 원래 key 다. | `jira_test.go`, `client_test.go` 기대값 |

검증하지 않는 위험: 실제 운영 DB 는 아직 없어(`cmd/wharf` 미구현) 0002 의 데이터 변환은 테스트 데이터로만 확인한다. 동기화에서 빠진 task 가 남긴 오래된 관계 행은 정리하지 않는다.

## 참고 Research

- [task-db-relations](../research/task-db-relations.md) — 현재 tasks 스키마와 sqlc 생성 모델, `CreateTasks`/`ExistsTask` 계약과 트랜잭션 주의점, 테스트 헬퍼와 이를 쓰는 테스트 목록, Jira adapter 의 ID 생성 위치와 epic-story/중복 link 특성, sqlc 의 RETURNING·DROP COLUMN 생성 검증, codegen 도구 위치.
- [sqlite-sqlc-setup](../research/sqlite-sqlc-setup.md) — sqlc 설정 경로와 `make modelgen` 동작. Task → 컬럼 매핑 표는 이 작업 후 stale 이 된다.

## Authority Boundaries

- Discretion: 테이블·쿼리·메서드·sentinel 이름, 쿼리 분할, ID 조합 helper 의 이름과 시그니처, migration 파일 이름(4자리 version 규칙 안에서), 테스트 헬퍼 구성, 기존 테스트를 새 입력 타입에 맞게 고치는 방식.
- Must-ask: Key decisions·Non-goals 변경, 서드파티 의존성 추가, `0001` 수정, `ExistsTask` 제거·`CreateTasks` 이름 변경.
- Stop conditions: modernc SQLite 가 0002 의 `DROP COLUMN`/`RETURNING`/`json_each` 를 실행하지 못하고 테이블 재생성으로도 우회가 안 될 때, `make gen` 이 도구 문제로 실패할 때.
- Loop budget: 3

## TODOs

- [ ] `0002` migration 과 `scripts/queries.sql` 쿼리(task upsert, 부모 upsert/삭제, link 삭제/추가, 카운터 증가)를 작성하고 `make gen` (AC-1, AC-2) (mechanical) (→ research: task-db-relations)
- [ ] `internal/pkg/task` 에 `{origin}-{key}` 조합과 local key(`WHF-%03d`) 규칙을 추가 (AC-5, AC-6) (mechanical)
- [ ] `CreateTasks` 를 `[]task.Task` 입력으로 바꾸고 관계 테이블 저장을 Key decisions 의 규칙대로 구현, local task 생성 메서드 추가, `make gen` 으로 mock 갱신 (AC-3, AC-4, AC-5) (design-bearing) (→ research: task-db-relations)
- [ ] db 테스트를 새 스키마·입력 타입에 맞게 고치고 AC-1~AC-5 테스트 추가 (AC-1, AC-2, AC-3, AC-4, AC-5) (mechanical) (→ research: task-db-relations)
- [ ] `atlassian.ToTask` 의 ID·관계 ID 에 `jira-` 접두를 적용하고 adapter 테스트 기대값 갱신 (AC-6) (mechanical) (→ research: task-db-relations)
