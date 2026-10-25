---
Application: wharf
JiraTicket: DEV-17
ReportType: single-step
Timestamp: 20261025131942
Title: task-db-relations
ReviewBase: `git diff 4043572` (working tree, untracked 파일 포함: `git status --short`)
---

# Task DB 개선: 관계 테이블 분리, `{originated}-{key}` ID, local ID 카운터

Plan: [20261025131942_DEV-17_PLAN_task-db-relations.md](./20261025131942_DEV-17_PLAN_task-db-relations.md)

See the change: `git diff 4043572` + untracked `scripts/migrations/0002_task_relations.sql`, `internal/pkg/task/id.go`, `internal/pkg/task/id_test.go`. Every `path:line` anchor in this report is valid against that snapshot.

## Summary
`tasks` 의 JSON 관계 컬럼을 `task_parents`/`task_links` 테이블로 옮기고, 모든 task ID 를 `{originated}-{key}` 로 통일했으며, local task 에 `task_counters` 기반 `local-WHF-NNN` ID 를 할당하는 `CreateLocalTask` 를 추가했다. 관계는 "각 task 가 자기 관계를 주장한다" 규칙(부모·link 는 교체, 자식 주장은 upsert 만)으로 저장해 Jira epic-story 관계가 저장 순서 때문에 지워지지 않게 하는 것이 목적이다.

## Red Flags
- **RF1** `internal/pkg/mocks/db_manager.go` - `/usr/local/bin/mockery` 가 go1.26 빌드라 go1.27 모듈을 로드하지 못해 `make gen` 이 실패했고(mocks 디렉터리를 먼저 지운 뒤 실패), 같은 v3.8.1 을 `GOTOOLCHAIN=go1.27.3` 으로 scratchpad 에 다시 빌드해 PATH 앞에 두고 `make gen` 을 실행했다. 생성 결과는 정상(`tool_tool.go` 무변경)이지만 환경의 mockery 는 여전히 go1.26 빌드다.

## Open Questions
- **OQ1** `internal/adapter/db/client.go:150` - 같은 배치에서 부모가 `ChildrenIDs` 로 자식을 주장한 뒤 그 자식이 빈 `ParentID` 로 저장되면 부모 행이 삭제된다(plan 의 "빈 부모는 삭제" + "나중 주장이 이긴다" 그대로). Jira subtask 는 항상 `parent` 를 가지므로 실제로는 문제 없다고 보지만, 다른 origin 에서 자식 쪽 parent 가 비어 오는 경우 관계가 사라질 수 있다.
- **OQ2** `scripts/migrations/0002_task_relations.sql:29` - 기존 행의 `children_ids`/`linked_ids` 가 유효한 JSON 이 아니면 `json_each` 가 실패해 migration 전체가 롤백된다. 기존 코드는 항상 `"[]"` 이상을 저장했으므로 가드는 넣지 않았다.

## Plan Divergence
### Changed - details that differ from the plan
- AC-4 의 sentinel: 관계 저장 실패는 `ErrFailedToCreateTask` 가 아니라 단계별 sentinel(`ErrFailedToSetTaskParent`, `ErrFailedToDeleteTaskParent`, `ErrFailedToSetTaskChild`, `ErrFailedToDeleteTaskLinks`, `ErrFailedToCreateTaskLink`)로 반환한다. AGENTS.md 의 "sentinel 당 origin return site 하나" 규칙 때문이며 plan 의 "`ErrFailedToCreateTask` 계열" 범위로 해석했다. 모두 `id=` 를 포함한다.
### Added - implemented but absent from the plan
- `ErrFailedToAllocateTaskNumber`(카운터 행이 없거나 UPDATE 실패) 와 `inTx` helper: `CreateTasks`/`CreateLocalTask` 가 BeginTx/Commit sentinel 을 한 origin site 에서 공유하도록.
### Deferred - planned but not implemented (deferred)
- None

## TODO Fulfillment
### TODO 1: `0002` migration 과 `scripts/queries.sql` 쿼리 작성, `make gen` - done
- 구현: `scripts/migrations/0002_task_relations.sql:1` - 관계·카운터 테이블 생성, `('local', 0)` seed, children 주장 → parent 주장 순 insert(자신의 parent 우선), link 변환(`ON CONFLICT DO NOTHING`), `id = originated || '-' || id`, 관계 컬럼 `DROP COLUMN`. `scripts/queries.sql:1` - `UpsertTask`(관계 컬럼 제거), `UpsertTaskParent`, `DeleteTaskParent`, `DeleteTaskLinks`, `CreateTaskLink`, `NextTaskNumber`(`UPDATE ... RETURNING`). `make gen` 으로 `models.go`/`queries.sql.go` 재생성.
- 테스트: `internal/adapter/db/client_test.go:399` `TestMigrateCreatesRelationTables` - 컬럼 목록·새 테이블·카운터 seed. `internal/adapter/db/client_test.go:438` `TestMigrateConvertsRelationsOfExistingTasks` - 0001 만 적용 → 직접 insert → 0002 적용 후 ID 접두·관계 이동·parent 우선·중복 link 무시.
- AC: AC-1 — 위 테스트 + `internal/adapter/db/models.go` 의 `Task` 에 관계 필드 없음. AC-2 — `TestMigrateConvertsRelationsOfExistingTasks` 통과.
- 편차: none

### TODO 2: `internal/pkg/task` 에 `{origin}-{key}` 조합과 local key 규칙 추가 - done
- 구현: `internal/pkg/task/id.go:7` `NewID`, `internal/pkg/task/id.go:13` `LocalKey`(`WHF-%03d`).
- 테스트: `internal/pkg/task/id_test.go:11` `TestNewID`, `internal/pkg/task/id_test.go:16` `TestLocalKey` - 3자리 padding 과 999 초과 시 자릿수 증가.
- AC: AC-5, AC-6 의 ID 규칙 — 위 테스트 통과.
- 편차: none

### TODO 3: `CreateTasks` 를 `[]task.Task` 입력으로, 관계 저장, local task 생성 메서드, mock 갱신 - done
- Risk / Lens: high / line-by-line (관계 저장 순서 규칙과 트랜잭션 경계)
- 구현: `internal/adapter/db/client.go:67` `CreateTasks` - 입력을 `[]task.Task` 로, 각 task 를 `storeTask` 로 한 트랜잭션에 저장. `internal/adapter/db/client.go:140` `storeTask` - task upsert → parent upsert/삭제 → `(child → X)` upsert → link 전체 삭제 후 재삽입. `internal/adapter/db/client.go:92` `CreateLocalTask` - 카운터 UPDATE 로 트랜잭션을 쓰기로 시작, ID·Originated 덮어쓰고 저장, 실패 시 번호도 롤백. `internal/adapter/db/client.go:118` `inTx`, `internal/adapter/db/client.go:184` `nullTime`. 제거: `orEmptyJSONArray`. mock 은 `make gen` 으로 재생성.
- 테스트: `internal/adapter/db/client_test.go:168` `TestCreateTasks`(필드·NULL 날짜·관계·중복 link), `:229` `TestCreateTasksReplacesClaimedRelations`(부모·link 교체/삭제, 빠진 자식 유지), `:259` `TestCreateTasksLaterClaimWins`, `:307` `TestCreateTasksRollsBackOnRelationFailure`(trigger 로 link insert 실패 → 전부 롤백), `:345` `TestCreateLocalTask`(001→실패→002→재연결→003), `:388` `TestCreateLocalTaskWithoutCounter`, `:472` `TestCreateTasksBeginTxFailure`, `client_management_test.go` `TestMethodsRequireConnection`(CreateLocalTask 포함).
- AC: AC-3, AC-4, AC-5 — 위 테스트 통과(`make test`).
- 편차: (design-bearing) 이었지만 plan 의 Key decisions 가 시그니처(`CreateTasks(ctx, []task.Task)`)·저장 규칙·카운터 방식을 이미 확정했고 남은 것은 이름(재량)뿐이라 consultant 없이 진행. 메서드 이름은 plan 예시대로 `CreateLocalTask(ctx, task.Task) (string, error)`. sentinel 분리는 Plan Divergence/Changed 참고.

### TODO 4: db 테스트를 새 스키마·입력 타입에 맞게 수정, AC-1~AC-5 테스트 추가 - done
- 구현: `internal/adapter/db/client_test.go:81` `readTask`/`open`/`read`/`parentOf`/`linksOf` 헬퍼, `newTask` 가 `task.Task` 반환. `client_management_test.go` 의 `CreateTasks` 호출을 `[]task.Task`·`jira-` ID 로 갱신.
- 테스트: TODO 1·3 에 나열한 테스트. JSON 배열 저장 계약이 사라져 `TestCreateTasksStoresEmptyIDListsAsJSONArray` 는 `TestCreateTasksReplacesClaimedRelations` 로 대체.
- AC: AC-1~AC-5 — `make test` 통과.
- 편차: `TestCreateTasks` 는 자식(`jira-WHF-2`)을 부모보다 먼저 저장한다. 반대 순서면 자식의 빈 `ParentID` 가 나중 주장으로 부모 행을 지운다(OQ1).

### TODO 5: `atlassian.ToTask` 의 ID·관계 ID 에 `jira-` 접두 적용, 테스트 기대값 갱신 - done
- 구현: `internal/adapter/atlassian/jira.go:76`, `:83`, `:92`, `:98` - `task.NewID(task.OriginJira, key)`. 에러의 `key=` 와 CLI 호출은 원래 key 유지.
- 테스트: `internal/adapter/atlassian/jira_test.go` `TestToTask*`, `internal/adapter/atlassian/client_test.go` `TestGetMyTasks*` - ID·관계 기대값이 `jira-WHF-*`, `key=WHF-1` 검사 유지.
- AC: AC-6 — `make test` 의 atlassian 패키지 통과.
- 편차: none

## Manual Verification
- [ ] `docs/agents/research/sqlite-sqlc-setup.md` 의 Task → 컬럼 매핑 표와 `task-db-relations.md` 는 이제 stale 이다. `application-research-sync` 로 갱신 필요.

## Test-dev (Round 0)
- Scope: uncommitted (13 files). Phase 1: existing tests already cover AC-1..AC-6; added `TestCreateTasksRelationStepFailures` in `internal/adapter/db/client_test.go` (trigger-injected failures for ErrFailedToSetTaskParent, ErrFailedToDeleteTaskParent, ErrFailedToSetTaskChild, ErrFailedToDeleteTaskLinks, with rollback and `id=`/`child=` checks). Phase 2: skipped, no `tests/e2e`. Phase 3: out of scope (caller).
- Verification: `make check` 0 issues, `make test` green. Findings: none.
