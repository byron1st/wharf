---
Application: wharf
JiraTicket: DEV-17
PlanType: single-step
Timestamp: 20261025134928
Title: task-db-improvement
---

# Task DB 개선 — 관계 테이블 분리, `{originated}-{key}` ID, local 번호 카운터

Report: [20261025134928_DEV-17_IMPL_task-db-improvement.md](./20261025134928_DEV-17_IMPL_task-db-improvement.md)

출처: Notion 개발 이슈 보드 DEV-17 "Task DB 개선". 사용자 지시에 따라 계획은 자동 승인되었고, 사용자에게 물어야 할 결정은 아래 Key decisions 에서 Claude 가 정했다(PR 본문에 같은 목록을 남긴다).

## Non-goals

- task 조회·생성·수정·삭제 HTTP API, service 메서드. local task 생성 API 는 다음 이슈("Task 관련 API 들 추가")에서 만든다.
- 새 마이그레이션 파일. 배포된 DB 가 없으므로 `scripts/migrations/0001_initial_migration.sql` 을 직접 고친다(사용자 지시). 기존 로컬 `wharf.db` 에 이미 적용된 0001 은 다시 적용되지 않으므로 파일을 지우고 다시 만들어야 한다 — 이를 처리하는 코드는 만들지 않는다.
- 관계 테이블의 조회 쿼리·조회용 인덱스. 읽는 쿼리가 생길 때(다음 이슈) 함께 추가한다.
- Jira 에서 사라진 관계(동기화되지 않는 task 쪽에 남은 오래된 관계) 정리.
- `task.Task` 도메인 구조체의 필드 변경, `Status`/`Origin` 검증, `atlassian.Client` 의 CLI 호출 방식 변경.

## Key decisions

- **ID 형식** — `task.Task.ID` 는 `{originated}-{key}` 다. 이를 만드는 작은 함수를 `internal/pkg/task` 에 둔다(atlassian 과 db 두 곳에서 쓴다).
  - Jira: `ToTask` 가 task 자신, parent, children, linked ID 모두에 `jira-` 접두어를 붙인다(관계 ID 가 저장된 task ID 와 맞물려야 하므로). CLI 호출과 에러 메시지의 `key=` 는 raw key 를 유지한다.
  - local (Claude 결정): key 는 `WHF-001` 형식이고 ID 는 규칙대로 `local-WHF-001` 이다. 번호는 최소 3자리 0 채움(`%03d`), 999 를 넘으면 `WHF-1000` 처럼 자리수가 늘어난다. 이슈 본문의 "WHF-001 과 같이 할당"은 key 로 해석했다 — 모든 ID 가 같은 규칙으로 `{originated}-` 를 가지면 origin 판별이 일관되기 때문이다.
- **local 번호 카운터** (Claude 결정, 이슈의 제안 그대로 채택) — `counters (name TEXT PK, value INTEGER)` 테이블을 두고 0001 에서 `('local_task', 0)` 을 seed 한다. 할당은 `UPDATE counters SET value = value + 1 WHERE name = ? RETURNING value` 한 문장.
  - 거절한 대안: `SELECT MAX(...)+1` — 읽기로 시작하는 트랜잭션이라 이 저장소의 SQLITE_BUSY 회피 규칙(트랜잭션은 쓰기로 시작)에 어긋나고 마지막 task 삭제 시 번호가 재사용된다. `AUTOINCREMENT` 더미 테이블 — 같은 효과를 내지만 의도가 드러나지 않고 행이 계속 쌓인다.
- **local 할당 진입점** (Claude 결정) — `db.Manager` 에 `CreateLocalTask(ctx, task.Task) (string, error)` 를 추가한다. 한 트랜잭션에서 카운터 증가 → ID 생성 → task 와 관계 저장을 하고 생성된 ID 를 돌려준다(입력의 `ID`, `Originated` 는 무시하고 `local` 로 저장). 호출하는 service/API 는 다음 이슈에서 붙인다. 카운터와 저장을 같은 트랜잭션에 둬서 실패 시 번호가 소모되지 않는다.
- **parent-children 테이블** (Claude 결정) — `task_hierarchies (child_id TEXT PK, parent_id TEXT NOT NULL)`. child_id 를 PK 로 해서 task 의 parent 는 하나다(Jira 와 같다). task T 를 저장할 때:
  - T 가 선언한 parent 는 T 소유다: `child_id = T` 행을 지우고 `ParentID` 가 있으면 `(ParentID, T)` 를 넣는다.
  - T 가 선언한 children 은 추가·재지정만 한다: 각 child 에 대해 `(T, child)` 를 넣고 child 가 이미 다른 parent 에 있으면 `ON CONFLICT(child_id)` 로 parent 를 T 로 바꾼다. T 의 다른 children 행은 지우지 않는다.
  - 이유: Jira 의 `subtasks` 는 sub-task 유형만 담아서 Epic 은 Story 를 children 으로 선언하지 않는다. "T 의 children 을 통째로 교체"하면 Story(parent=Epic) 저장 후 Epic 저장 시 관계가 지워진다(→ research).
- **linked 테이블** (Claude 결정) — `task_links (task_id, linked_id, PK(task_id, linked_id))`, 선언한 task 기준의 방향 있는 행. task T 저장 시 `task_id = T` 행을 지우고 T 의 `LinkedIDs` 를 넣는다. 양방향 조회는 읽기 쿼리를 만들 때 합집합으로 한다. 이유: Jira task 를 다시 동기화할 때 local task 가 그 Jira task 로 선언한 링크를 지우지 않기 위해서이며, 기존 `linked_ids` 컬럼의 "선언한 task 가 덮어쓴다" 의미와 같다.
- **중복과 FK** — 같은 상대로의 중복 선언(Jira 의 다중 link type, 중복 subtask)은 PK 충돌 대신 무시한다. 관계는 DB 에 없는 task 를 가리킬 수 있으므로 `tasks(id)` FK 는 두지 않는다. 자기 자신을 가리키는 관계는 막지 않는다(Jira 가 만들지 않음).
- **tasks 테이블** — `parent_id`, `children_ids`, `linked_ids` 컬럼을 없앤다. `originated` 컬럼은 유지한다(ID 접두어와 같지만 필터용 컬럼으로 남긴다).
- **UpsertTasks 의미 유지** — 여전히 한 트랜잭션 all-or-nothing, 빈 목록 no-op, 같은 ID 덮어쓰기. 관계 쓰기도 같은 트랜잭션에서 하며 실패 시 전부 롤백된다. 실패 원인별 sentinel 을 추가한다(관례: 하나의 return site 당 하나).

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | 0001 마이그레이션 적용 후 `tasks` 에 `parent_id`/`children_ids`/`linked_ids` 컬럼이 없고, `task_hierarchies`, `task_links`, `counters`(`local_task`=0 seed) 테이블이 있다. sqlc 생성 코드가 이 스키마로 재생성되어 있다. | db 테스트(파일 DB 직접 SQL), `make gen` 후 diff 없음 |
| AC-2 | `ToTask` 결과의 ID·ParentID·ChildrenIDs·LinkedIDs 가 모두 `jira-{key}` 이고, CLI view 호출 인자와 에러 메시지의 `key=` 는 raw key 다. | `jira_test.go`, `client_test.go` |
| AC-3 | `UpsertTasks` 가 parent 를 `task_hierarchies(child=T)` 한 행으로 저장하고, 다시 저장하면 이전 parent 행을 교체(빈 ParentID 면 삭제)한다. children 은 `(T, child)` 로 저장되며 child 가 다른 parent 에 있으면 T 로 재지정되고, T 의 children 목록에 없는 기존 children 행은 남는다. | db 테스트 |
| AC-4 | Story(parent=Epic) 를 저장한 뒤 children 이 비어 있는 Epic 을 저장해도 Epic→Story 관계가 남는다. | db 테스트 |
| AC-5 | `UpsertTasks` 가 linked 를 `task_links(task_id=T)` 로 저장하고, 다시 저장하면 T 가 선언한 행만 교체한다(다른 task 가 T 로 선언한 행은 유지). 같은 ID 가 중복된 LinkedIDs/ChildrenIDs 도 에러 없이 한 행으로 저장된다. | db 테스트 |
| AC-6 | 관계 쓰기를 포함해 어느 하나라도 실패하면 `UpsertTasks` 는 아무것도 저장하지 않고 원인별 sentinel 로 `errors.Is` 판별된다. | db 테스트(실패 유도) |
| AC-7 | `CreateLocalTask` 를 연속 호출하면 `local-WHF-001`, `local-WHF-002` … 를 돌려주고, task 는 `originated=local` 로 관계와 함께 저장된다. 저장이 실패하면 카운터도 증가하지 않는다(다음 호출이 같은 번호). `WHF-1000` 처럼 3자리를 넘는 번호도 형식이 맞다. | db 테스트, task 패키지 테스트 |
| AC-8 | `MockManager` 에 `CreateLocalTask` 가 있고 기존 service/server 테스트가 그대로 통과한다. | `make gen`, `make test` |

검증하지 않는 위험: 동기화되지 않는 task 쪽에 남은 오래된 관계(예: Jira 에서 subtask 가 다른 parent 로 옮겨졌는데 그 subtask 가 동기화 대상이 아님)는 정리되지 않는다. 다른 프로세스와의 동시 `CreateLocalTask` 경합은 busy_timeout 에 맡기고 테스트하지 않는다. 기존 로컬 `wharf.db` 는 수동으로 지워야 한다.

## 참고 Research

- [task-id-and-relations](../research/task-id-and-relations.md) — ID 생성 위치(`ToTask`), 관계 저장 현재 구조, 관계 테이블의 쓰기 의미를 정한 근거(Jira subtasks/issuelinks 특성, cross-origin 관계), 카운터와 SQLite 트랜잭션 제약, ID 에 의존하는 테스트 위치.
- [sqlite-sqlc-setup](../research/sqlite-sqlc-setup.md) — sqlc 설정과 `make modelgen` 동작. Task → 컬럼 매핑 표는 구현 후 stale 이 되므로 갱신 대상이다.
- [server-service-structure](../research/server-service-structure.md) — `db.Manager` 계약, db 테스트의 직접 SQL 확인 방식, mockery·에러·테스트 관례.

## Authority Boundaries

- Discretion: sentinel·쿼리·헬퍼·테스트 헬퍼 이름, 관계 쓰기 헬퍼 구성(UpsertTasks/CreateLocalTask 공유 방식), local task 저장 시 plain INSERT 와 upsert 중 선택, ID 생성 함수의 이름과 시그니처, 카운터 이름 문자열.
- Must-ask: Key decisions·Non-goals 변경(관계 쓰기 의미, ID 형식, 카운터 방식), 새 마이그레이션 파일 추가, `task.Task` 필드 변경, 서드파티 의존성 추가, HTTP API 추가. (이번 실행에서는 사용자 지시에 따라 Claude 가 결정하고 PR 에 기록한다.)
- Stop conditions: sqlc 가 계획한 쿼리(`UPDATE ... RETURNING`, `ON CONFLICT ... DO NOTHING/UPDATE`)를 생성하지 못할 때, 관계 쓰기 의미가 기존 테스트의 의도와 정면으로 충돌할 때.
- Loop budget: 3

## TODOs

- [x] `0001_initial_migration.sql` 을 새 스키마(tasks 관계 컬럼 제거, `task_hierarchies`, `task_links`, `counters` + seed)로 고치고 `scripts/queries.sql` 에 관계·카운터 쿼리를 추가한 뒤 `make gen` (AC-1) (mechanical) (→ research: task-id-and-relations)
- [x] `internal/pkg/task` 에 `{originated}-{key}` ID 생성과 local key(`WHF-%03d`) 형식 함수를 추가하고 테스트 (AC-2, AC-7) (mechanical)
- [x] `atlassian.ToTask` 가 task·parent·children·linked ID 에 `jira-` 접두어를 쓰도록 바꾸고 atlassian 테스트 기대값 갱신 (AC-2) (mechanical) (→ research: task-id-and-relations)
- [x] `UpsertTasks` 가 Key decisions 의 소유 규칙대로 관계 테이블에 쓰도록 바꾸고, 원인별 sentinel 과 db 테스트(교체·재지정·Epic/Story 순서·중복·롤백) 작성 (AC-3, AC-4, AC-5, AC-6) (design-bearing) (→ research: task-id-and-relations)
- [x] `db.Manager.CreateLocalTask` 를 카운터 증가와 저장을 한 트랜잭션으로 구현하고 `make gen` 으로 mock 갱신, db 테스트 작성 (AC-7, AC-8) (mechanical) (→ research: task-id-and-relations)
- [x] `docs/agents/research/sqlite-sqlc-setup.md`·`server-service-structure.md` 의 tasks 컬럼·`UpsertTasks`·`Manager` 서술을 새 구조로 갱신 (AC-1) (mechanical)
