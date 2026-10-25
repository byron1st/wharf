---
Application: wharf
JiraTicket: DEV-17
ReportType: single-step
Timestamp: 20261025134928
Title: task-db-improvement
ReviewBase: `git diff 1945fa565a2eae322ea5c268fbbf18c0c4fc25e7` (working tree, 미커밋) + 신규 파일 `internal/pkg/task/id.go`, `internal/pkg/task/id_test.go`
---

# Task DB 개선 — 관계 테이블 분리, `{originated}-{key}` ID, local 번호 카운터

Plan: [20261025134928_DEV-17_PLAN_task-db-improvement.md](./20261025134928_DEV-17_PLAN_task-db-improvement.md)

See the change: `git diff 1945fa565a2eae322ea5c268fbbf18c0c4fc25e7` (untracked 신규 파일 포함). Every `path:line` anchor in this report is valid against that snapshot.

## Summary
tasks 의 JSON 관계 컬럼을 `task_hierarchies`·`task_links` 테이블로 옮기고, task ID 를 `{originated}-{key}` 로 통일했으며, local task 번호를 `counters` 테이블로 할당하는 `db.Manager.CreateLocalTask` 를 추가했다. 핵심은 관계 소유 규칙이다 — parent·links 는 선언한 task 가 교체하고 children 은 추가·재지정만 해서, Jira Epic 처럼 children 을 다 나열하지 않는 task 를 저장해도 Story→Epic 관계가 지워지지 않는다.

## Red Flags
- **RF1** `internal/adapter/db/client.go:119` - 구조 변경: begin/commit 을 `inTx` 헬퍼로 뽑아 `UpsertTasks`·`CreateLocalTask` 가 공유한다. `ErrFailedToBeginTx`/`ErrFailedToCommitTx` 의 return site 는 여전히 하나지만, 두 메서드 중 어느 쪽에서 났는지는 sentinel 만으로 구분되지 않는다(호출자는 자기가 부른 메서드를 안다).
- **RF2** 개발 환경 - `/root/.local/bin/mockery` (v3.8.1) 가 go1.26 으로 빌드되어 go1.27 모듈을 읽지 못해 `make gen` 이 실패했다. 같은 버전을 `GOTOOLCHAIN=go1.27.3` 으로 재설치했다. 저장소 파일은 바뀌지 않았다.

## Open Questions
- **OQ1** `docs/agents/research/task-id-and-relations.md` - 계획 단계 research 로 "현재 구조"(JSON 컬럼) 를 서술하므로 이제 stale 이다. 계획 TODO 가 갱신 대상을 sqlite-sqlc-setup·server-service-structure 로 한정해 손대지 않았다. 다음 research sync 때 정리하거나 삭제할지 판단이 필요하다.
- **OQ2** `internal/adapter/atlassian/jira.go:99` - view 출력에 key 가 비어 있으면 ID 가 `jira-` 가 된다(이전에는 빈 문자열). 검색 단계에서 빈 key 는 걸러지므로 실제로는 발생하지 않는다고 보고 처리하지 않았다.

## Plan Divergence
### Changed - details that differ from the plan
- 기존 `TestUpsertTasksStoresEmptyIDListsAsJSONArray` 는 JSON 배열 저장이 없어져 삭제하고, 빈 목록이 관계를 지우는 동작을 `TestUpsertTasksReplacesParent`·`TestUpsertTasksReplacesOnlyDeclaredLinks` 로 대신 고정했다.
### Added - implemented but absent from the plan
- `TestInitialMigrationSchema` (`internal/adapter/db/client_test.go:159`): AC-1 의 스키마 증거용.
- `TestMethodsRequireConnection` 에 `CreateLocalTask` 미연결 검사 추가 (`internal/adapter/db/client_management_test.go`).
### Deferred - planned but not implemented (deferred)
- None

## TODO Fulfillment
### TODO 1: `0001_initial_migration.sql` 새 스키마 + `queries.sql` 관계·카운터 쿼리 + `make gen` - done
- 구현: `scripts/migrations/0001_initial_migration.sql` - tasks 에서 관계 컬럼 제거, `task_hierarchies(child_id PK, parent_id)`, `task_links(PK(task_id, linked_id))`, `counters` + `('local_task', 0)` seed. FK 없음.
- 구현: `scripts/queries.sql` - `DeleteTaskParent`, `UpsertTaskHierarchy`(`ON CONFLICT(child_id) DO UPDATE`), `DeleteTaskLinks`, `CreateTaskLink`(`DO NOTHING`), `IncrementCounter`(`UPDATE ... RETURNING`, `:one` → `(int64, error)`). 생성 코드 `internal/adapter/db/{models,queries.sql}.go` 재생성.
- 테스트: `internal/adapter/db/client_test.go:159` `TestInitialMigrationSchema` - 네 테이블의 컬럼 목록과 `local_task=0` seed.
- AC: AC-1 — `TestInitialMigrationSchema` 통과, `make gen` 재실행 후 생성 파일 해시 동일.
- 편차: none

### TODO 2: `internal/pkg/task` ID 생성·local key 함수 - done
- 구현: `internal/pkg/task/id.go:10` `NewID(Origin, key)` → `{originated}-{key}`; `:16` `LocalKey(n int64)` → `WHF-%03d`.
- 테스트: `internal/pkg/task/id_test.go:11` `TestNewID`, `:16` `TestLocalKey` (1, 42, 999, 1000 → `WHF-1000`).
- AC: AC-2, AC-7 — task 패키지 테스트 통과.
- 편차: none

### TODO 3: `atlassian.ToTask` 의 `jira-` 접두어 - done
- 구현: `internal/adapter/atlassian/jira.go:77,84,93,99` - task·parent·children·linked ID 를 `task.NewID(task.OriginJira, key)` 로. CLI 인자와 에러의 `key=` 는 raw key 그대로.
- 테스트: `internal/adapter/atlassian/jira_test.go` `TestToTask*`, `client_test.go` `TestGetMyTasks*` 기대 ID 를 `jira-WHF-N` 으로 갱신. `viewOption("WHF-1")` 과 `key=WHF-1` 기대값은 그대로 두어 raw key 유지를 고정.
- AC: AC-2 — atlassian 패키지 테스트 통과.
- 편차: none

### TODO 4: `UpsertTasks` 관계 테이블 쓰기 + sentinel + db 테스트 - done
- Risk / Lens: high / line-by-line (관계 소유 규칙)
- 구현: `internal/adapter/db/client.go:69` `UpsertTasks` → `inTx` 안에서 task 마다 `storeTask` (`:140`): task upsert → `child_id=T` 삭제 → parent 삽입 → children upsert(재지정) → `task_id=T` 링크 삭제 → 링크 삽입(중복 무시). sentinel 6개 추가(`client.go:16-27`), 각 return site 하나.
- 테스트: `internal/adapter/db/client_test.go` `TestUpsertTasks`(:203), `TestUpsertTasksReplacesParent`(:257), `TestUpsertTasksAddsAndMovesChildren`(:274), `TestUpsertTasksKeepsStoryUnderEpicWithoutChildren`(:291), `TestUpsertTasksReplacesOnlyDeclaredLinks`(:305), `TestUpsertTasksStoresDuplicateRelationsOnce`(:324), `TestUpsertTasksRollsBackOnFailure`(:335, 쓰기 지점 6곳마다 trigger 로 실패 유도 후 sentinel 과 tasks·관계 전체 롤백 확인).
- AC: AC-3, AC-4, AC-5, AC-6 — 위 테스트 통과.
- 편차: begin/commit 을 `inTx` 로 추출(RF1). design-bearing 이지만 Key decisions 가 의미를 모두 정해 둬서 consultant 없이 진행했다.

### TODO 5: `db.Manager.CreateLocalTask` + mock + db 테스트 - done
- 구현: `internal/adapter/db/client.go:93` `CreateLocalTask` - `inTx` 안에서 `IncrementCounter("local_task")` → `task.NewID(OriginLocal, task.LocalKey(n))` → `storeTask`. 입력 `ID`/`Originated` 무시. 저장은 기존 `UpsertTask` 재사용(Discretion: plain INSERT 대신). `make gen` 으로 `internal/pkg/mocks/db_manager.go` 갱신.
- 테스트: `client_test.go` `TestCreateLocalTask`(:422, 001→002, originated=local, 관계 저장), `TestCreateLocalTaskPastThreeDigits`(:442, 999→`local-WHF-1000`), `TestCreateLocalTaskFailureKeepsNumber`(:456, 실패 후 다음 호출이 같은 번호), `TestCreateLocalTaskCounterFailure`(:481, 카운터 행 없음 → `ErrFailedToIncrementLocalTaskCounter`+`sql.ErrNoRows`); `client_management_test.go` `TestMethodsRequireConnection`.
- AC: AC-7, AC-8 — 위 테스트 통과, `MockManager.CreateLocalTask` 생성, service/server 테스트 무변경 통과.
- 편차: none

### TODO 6: research 문서 갱신 - done
- 구현: `docs/agents/research/sqlite-sqlc-setup.md` (테이블 매핑 표, ID 형식, `IncrementCounter`·ON CONFLICT 생성 형태, `inTx`), `docs/agents/research/server-service-structure.md` (db.Manager 계약·sentinel·소유 규칙·CreateLocalTask, 테스트 헬퍼 `relations`, ToTask ID 규칙, `task.NewID`/`LocalKey`). 두 문서의 Description 이 바뀌어 `docs/agents/research/index.md` 행도 갱신.
- 테스트: 해당 없음(문서).
- AC: AC-1 — 문서가 새 스키마를 서술.
- 편차: none

## Manual Verification
- [ ] 기존 로컬 `wharf.db` 를 지우고 서버를 다시 띄워 0001 이 새 스키마로 적용되는지 확인(이미 적용된 0001 은 재적용되지 않는다).

## Fix
- 출처: dev-loop Round 0 리뷰의 [NORMAL] 비차단 항목(senior-generalist-reviewer). 사용자 위임에 따라 Claude 가 Fix 로 결정.
- 결함: `ToTask` 가 key 없는 parent 를 받으면 `ParentID` 가 이전의 `""` 대신 `"jira-"` 가 되어 `task_hierarchies` 에 가짜 parent 행이 생긴다. subtasks·issuelinks 는 빈 key 를 건너뛰는데 parent 만 빠져 있었다.
- 수정: `internal/adapter/atlassian/jira.go:76` 조건을 `f.Parent != nil && f.Parent.Key != ""` 로 바꿨다. 회귀 테스트 `TestToTaskRelations` 의 "parent without a key is skipped" 케이스 추가.
- 검증: `make check` 0 issues, `make test` 전체 통과.
- 문서: 구현 후 낡은 `docs/agents/research/task-id-and-relations.md` 를 새 구조 기준으로 갱신하고 index 행·정렬을 맞췄다(OQ1 해소).
