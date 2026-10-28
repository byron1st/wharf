---
Application: wharf
JiraTicket: DEV-22
ReportType: single-step
Timestamp: 20261027212216
Title: service-desk-task-relation
ReviewBase: `git diff 9f7ed5fc6` (working tree, uncommitted)
---

# Service Desk 요청과 Task 의 relates-to 연결

Plan: [20261027212216_DEV-22_PLAN_service-desk-task-relation.md](./20261027212216_DEV-22_PLAN_service-desk-task-relation.md)

See the change: `git diff 9f7ed5fc6`. Every `path:line` anchor in this report is valid against that snapshot.

## Summary

DEV-21 이 저장하는 Service Desk 요청과 task 사이에 다대다 relates-to 연결을 wharf 에만 저장하고, 연결을 추가·삭제하는 `PUT`/`DELETE /tasks/{id}/service-desk-requests/{key}` 와 `GET /tasks/{id}` 의 `service_desk_requests` 임베딩을 추가했다. 연결 추가는 쓰기 우선 트랜잭션 규칙(`busyTimeoutPragma`)을 지키도록 존재 확인을 INSERT 한 문장에 넣고, 영향 행이 0 일 때만 읽어서 원인을 가른다.

## Red Flags

None

## Open Questions

- **OQ1** `docs/design/wharf-domain-for-design.md:148` - 웹 앱 설계용 도메인 문서의 API 표는 DEV-21 의 `POST /service-desk/requests` 부터 이미 반영되어 있지 않고, 이번 두 라우트와 `service_desk_requests` 필드도 반영하지 않았다(계획의 갱신 대상이 research 문서뿐이라). 갱신 여부는 사용자 판단.
- **OQ2** `internal/adapter/db/client.go:354` - task 와 요청이 둘 다 없으면 task 없음(`ErrTaskToLinkNotFound`)을 먼저 보고한다. 응답 상태는 둘 다 404 라 클라이언트 동작 차이는 메시지뿐이다.

## Plan Divergence

### Changed - details that differ from the plan

None

### Added - implemented but absent from the plan

- `ExistsServiceDeskRequest` 쿼리: 연결 추가가 0 행일 때 요청 존재를 가르는 데 필요했다(task 쪽은 기존 `ExistsTask` 재사용).
- `TestMethodsRequireConnection` 에 새 db 메서드 3개의 `ErrNotConnected` 확인 추가: 기존 테스트가 모든 비관리 메서드를 덮는 관례를 유지.

### Deferred - planned but not implemented (deferred)

None

## TODO Fulfillment

### TODO 1: 0001 에 `task_service_desk_requests` 추가, queries.sql 에 연결 추가·삭제·JOIN 조회·일괄 삭제 쿼리, `make gen` - done

- 구현: `scripts/migrations/0001_initial_migration.sql:79` `task_service_desk_requests` - `(task_id, issue_key)` PK, FK·추가 인덱스 없음. `scripts/queries.sql:179` `CreateTaskServiceDeskRequest` (`INSERT ... SELECT ... WHERE EXISTS(task) AND EXISTS(request) ON CONFLICT DO NOTHING`, `:execrows`), `:188` `ExistsServiceDeskRequest`, `:191` `DeleteTaskServiceDeskRequest`, `:194` `DeleteTaskServiceDeskRequestsOfTask`, `:197` `ListTaskServiceDeskRequests` (inner JOIN, `issue_key` 순). 생성 코드는 `make gen` 결과.
- 테스트: `internal/adapter/db/client_test.go:159` `TestInitialMigrationSchema` - 새 테이블의 컬럼과 PK 순서(`task_id,issue_key`), 기존 테이블 컬럼 불변. 기존 `TestInitialMigrationIndexes` 가 인덱스 추가 없음을 고정.
- AC: AC-1 — `TestInitialMigrationSchema` 통과, 스키마 diff 는 0001 에만 있음(`git diff --stat scripts/migrations`).
- 편차: none

### TODO 2: `db.Manager` 연결 추가·삭제·요청 목록, `DeleteTask` cascade, db 테스트 - done

- Risk / Lens: line-by-line (쓰기 우선 트랜잭션 규칙)
- 구현: `internal/adapter/db/client.go:354` `LinkServiceDeskRequest` - 첫 문장이 INSERT, 0 행일 때만 `ExistsTask`→`ExistsServiceDeskRequest` 로 task 없음/요청 없음/이미 연결됨(no-op)을 가름. `:405` `UnlinkServiceDeskRequest` - 단일 DELETE `:execrows`, 0 이면 `ErrServiceDeskRequestLinkToDeleteNotFound`. `:427` `ListTaskServiceDeskRequests` - JOIN 결과를 `servicedesk.Request` 로, 항상 non-nil. `:297` `DeleteTask` 가 같은 트랜잭션에서 연결 행 삭제.
- 테스트: `internal/adapter/db/servicedesk_test.go:156` `TestLinkServiceDeskRequest` (local·jira task, 다대다, 반복 PUT 시 행 하나), `:170` `TestLinkServiceDeskRequestNotFound` (각 sentinel, 행 없음), `:190` `TestUnlinkServiceDeskRequest` (그 행만 삭제, 요청·task·다른 연결 유지, 재삭제 not-found), `:212` `TestListTaskServiceDeskRequests` (key 순·필드·빈 slice non-nil), `:239` `TestDeleteTaskDeletesItsServiceDeskLinks` (`UpsertTasks`·`UpsertServiceDeskRequests` 후 연결 유지, DeleteTask cascade 범위). 모두 `serviceDeskLinks` 헬퍼로 직접 SQL 확인.
- AC: AC-1, AC-2, AC-3, AC-4, AC-5 — 위 db 테스트 `make test` 통과.
- 편차: none

### TODO 3: `service.Service` 연결 추가·삭제, `TaskDetail`·`GetTask` 요청 목록, sentinel 변환, MockManager 테스트 - done

- 구현: `internal/service/service.go:221` `LinkServiceDeskRequest`, `:237` `UnlinkServiceDeskRequest` - db sentinel 을 `ErrTaskToLinkNotFound`·`ErrServiceDeskRequestToLinkNotFound`·`ErrServiceDeskRequestLinkToDeleteNotFound` 로 변환. `:105` `GetTask` 가 `ListTaskServiceDeskRequests` 결과를 `TaskDetail.ServiceDeskRequests` (`internal/service/model.go:33`) 에 담음.
- 테스트: `internal/service/servicedesk_test.go:134` `TestLinkServiceDeskRequest`, `:143` `TestLinkServiceDeskRequestFailures` (sentinel 분리·맥락·기타 에러 전달), `:172` `TestUnlinkServiceDeskRequest`; `internal/service/service_test.go:86`/`:118`/`:137` `TestGetTask*` (요청 목록 포함), `:174` `TestGetTaskServiceDeskRequestsFailure`.
- AC: AC-2, AC-3, AC-4 — service 테스트 통과.
- 편차: none

### TODO 4: server 두 라우트, `service_desk_requests` 매핑, 에러 → 상태 코드, MockService 테스트 - done

- 구현: `internal/server/route.go:40-41` 라우트, `:222` `linkServiceDeskRequest`, `:234` `unlinkServiceDeskRequest` (204, 본문 없음), `:129` `getTask` 의 `serviceDeskRequestResponse` 매핑(항상 배열). `internal/server/model.go:59` `serviceDeskRequestResponse`. `internal/server/util.go:30-32` 404 매핑 3개.
- 테스트: `internal/server/servicedesk_test.go:60` `TestLinkServiceDeskRequestRoute` (반복 PUT 204), `:73` `TestUnlinkServiceDeskRequestRoute`, `:83` `TestServiceDeskRequestLinkRoutesMapServiceErrors` (404×3, 500, `{"error":...}`); `internal/server/route_test.go:160` `TestGetTask` (원소 필드·RFC3339·순서), `:214` `TestGetTaskWithoutRelations` (`[]`).
- AC: AC-2, AC-3, AC-4, AC-6 — server 테스트와 기존 테스트 전체 `make test` 통과.
- 편차: none

### TODO 5: research 4개와 index 갱신 - done

- 구현: `docs/agents/research/service-desk-requests.md` (DEV-22 절 추가), `task-id-and-relations.md` (DeleteTask cascade·쓰기 우선 규칙·연결 테이블 절), `sqlite-sqlc-setup.md` (스키마·쿼리·`INSERT ... SELECT ... ON CONFLICT :execrows` 형태), `server-service-structure.md` (service·route·db 계약), `index.md` Description 4행 동기화.
- 테스트: 문서 변경이라 없음.
- AC: AC-6 — `make gen` 후 diff 없음, `go.mod`/`go.sum`/`config.example.toml` 무변경.
- 편차: none

## Manual Verification

- [ ] 기존 로컬 `wharf.db` 가 0001 을 이미 적용했다면 지우고 다시 만든 뒤, `PUT /tasks/{id}/service-desk-requests/{key}` → `GET /tasks/{id}` 의 `service_desk_requests` 를 실제 동기화 데이터로 확인.
