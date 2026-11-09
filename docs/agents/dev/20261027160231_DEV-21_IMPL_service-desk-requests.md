---
Application: wharf
JiraTicket: DEV-21
ReportType: single-step
Timestamp: 20261027160231
Title: service-desk-requests
ReviewBase: git diff e23652d dbb2009
---

# Service Desk 요청 수집

Plan: [20261027160231_DEV-21_PLAN_service-desk-requests.md](./20261027160231_DEV-21_PLAN_service-desk-requests.md)

See the change: `git diff e23652d dbb2009`. Every `path:line` anchor in this report is valid against that snapshot.

## Summary

본인이 Service Desk에 올린 요청을 Jira task와 다른 테이블에 동기화한다. `GET /rest/servicedeskapi/request`를 기존 Atlassian 클라이언트와 Basic 헤더로 페이지를 따라 읽고, `issue_key`·제목·상태·`created_at`·`status_at`만 `POST /service-desk/requests`로 저장한다. 담당자 컬럼과 assignee용 추가 API는 없다.

## Red Flags

None

## Open Questions

None

## Plan Divergence

### Changed - details that differ from the plan

- None

### Added - implemented but absent from the plan

- `docs/agents/research/index.md`에서 `jira-adf-description`·`jira-rest-search` 설명을 각 파일 frontmatter와 같게 고치고, Flow를 Structure보다 앞에 두었다. 계획의 기능 범위는 아니고, research 인덱스를 현재 설명과 맞추는 정리이다.

### Deferred - planned but not implemented (deferred)

- None

## TODO Fulfillment

### TODO 1: `atlassian.Client`에서 owned Service Desk 요청을 조회해 `servicedesk.Request`로 매핑한다 - done

- 구현: `internal/pkg/servicedesk/request.go:8` `Request` — Key, Title, Status, Created, StatusAt. `internal/adapter/atlassian/client.go:37` `GetOwnedRequests`와 `client.go:56` `requestsURL`(TrimRight 한 site URL + `/rest/servicedeskapi/request`, 같은 Basic 헤더). `internal/adapter/atlassian/servicedesk.go:73` `GetOwnedRequests`가 `OWNED_REQUESTS`·`ALL_REQUESTS`·`limit=50`·`start`로 페이지를 따른다. 정상 종료(`isLastPage`, values 빈 목록, `size <= 0`)는 모은 항목을 비-nil 슬라이스로 반환한다. HTTP 실패, 페이지 JSON 실패, 100페이지 초과는 nil. `servicedesk.go:149` `toRequest`는 key·title을 TrimSpace하고, status는 공백이 아니면 원문, epochMillis 0은 탈락, 시각은 UTC `time.UnixMilli`이다. 제목은 summary가 비어 있을 때만 fieldId `summary`의 JSON 문자열을 쓴다.
- 테스트: `internal/adapter/atlassian/servicedesk_test.go:88` `TestGetOwnedRequests` — 제목 trim, 비문자열 summary 필드를 건너뛴 뒤 문자열 제목. `servicedesk_test.go:112` `TestGetOwnedRequestsStopsPaging`. `servicedesk_test.go:213` `TestGetOwnedRequestsJoinsSiteURL` — trailing slash와 `/ex/jira/{cloudId}` 유지, 쿼리 순서. `servicedesk_test.go:297` `TestGetOwnedRequestsOverHTTP` — GET, path, raw query, Authorization이 기존 테스트 헤더와 같고 토큰 원문이 헤더에 없음.
- AC: AC-1 — 위 테스트가 `make test`에서 통과. AC-2의 페이지 실패·상한·취소·항목 탈락은 TODO 4.
- 편차: none. 페이지 상한은 재량으로 `maxRequestPages = 100` (`servicedesk.go:36`). sentinel은 `ErrFailedToBuildRequestURL`부터 `ErrTooManyRequestPages` (`servicedesk.go:16`). 탈락 에러는 `index=` 또는 `key=`.

### TODO 2: `service_desk_requests`를 `0001_initial_migration.sql`에 넣고 sqlc upsert를 `db.Manager`에 연결한다 - done

- 구현: `scripts/migrations/0001_initial_migration.sql:68` `service_desk_requests` — issue_key PK, title, status, created_at, status_at, 전부 NOT NULL. `scripts/queries.sql:167` `UpsertServiceDeskRequest`가 네 컬럼을 ON CONFLICT로 덮어쓴다. `internal/adapter/db/client.go:303` `UpsertServiceDeskRequests`는 연결 확인 후 빈 목록은 no-op, 그 외는 한 트랜잭션. 없는 행은 지우지 않는다. 생성 모델은 `internal/adapter/db/models.go`의 `ServiceDeskRequest`.
- 테스트: `internal/adapter/db/servicedesk_test.go:15` `TestUpsertServiceDeskRequests` — SD-1 네 필드 덮어쓰기, SD-2 유지, nil upsert 후 행 유지, tasks의 WHF-1과 행 수 1 유지. `internal/adapter/db/client_test.go:197` `TestInitialMigrationSchema`가 다섯 컬럼을 확인한다.
- AC: AC-3 — 위 테스트와 0001만 수정(`scripts/migrations/`에 0001만 있음). `make test` 통과.
- 편차: none. sentinel은 `ErrFailedToUpsertServiceDeskRequest` (`internal/adapter/db/client.go:46`), 실패 시 `key=` (`client.go:322`).

### TODO 3: 서비스 동기화 메서드와 `POST /service-desk/requests`를 추가하고 mock을 `make gen`으로 다시 만든다 - done

- 구현: `internal/service/service.go:76` `SyncServiceDeskRequests` — nil이면 upsert하지 않고 에러를 반환하고, 그 외(빈 슬라이스 포함)는 upsert한 뒤 `SyncResult{Upserted: len, Partial}`를 반환한다. 탈락 항목은 클라이언트가 빼므로 저장되지 않는다. `internal/service/model.go:5` `SyncResult` 주석에서 "실패한 항목도 저장"을 뺐고, 그 문장은 `service.go:54` `SyncJiraTasks`에만 있다. `internal/server/route.go:34` `POST /service-desk/requests`, `route.go:82` `syncServiceDeskRequests`는 `POST /tasks/jira`와 같은 `syncResponse`와 `writeInternalError`를 쓴다. mock은 `make gen`으로 `internal/pkg/mocks/`에 다시 만들었다.
- 테스트: `internal/service/servicedesk_test.go:54` `TestSyncServiceDeskRequestsSkipsUpsertWhenFetchFails` — nil이면 `UpsertServiceDeskRequests` 기대가 없다. `servicedesk_test.go:67` `TestSyncServiceDeskRequestsEmptyKeepsStoredRows` — 빈 슬라이스는 upserted 0, partial nil. `internal/server/servicedesk_test.go:15` `TestSyncServiceDeskRequestsRoute`.
- AC: AC-2의 nil·빈 목록·전부 탈락은 TODO 4 테스트와 함께. AC-4는 라우트 테스트. AC-5 — `go.mod`/`go.sum` diff 없음, config 파일 diff 없음, 조회는 `GetOwnedRequests` 한 경로뿐.
- 편차: none

### TODO 4: AC-1부터 AC-4의 client·service·db·route 테스트를 고정한다 - done

- 구현: 테스트만 추가. 프로덕션 동작은 TODO 1–3.
- 테스트: `internal/adapter/atlassian/servicedesk_test.go:147` `TestGetOwnedRequestsPageErrors` — 첫·이후 페이지 HTTP와 페이지 JSON 실패가 nil. `servicedesk_test.go:179` `TestGetOwnedRequestsPageLimit` — 100페이지 후에도 다음이 있으면 nil과 `ErrTooManyRequestPages`. `servicedesk_test.go:196` `TestGetOwnedRequestsLastPageAtLimit` — 100번째가 마지막이면 100건. `servicedesk_test.go:245` `TestGetOwnedRequestsSkipsBadItems` — 정상 1건만 반환하고 Partial에 `index=0`, `index=1`, `key=SD-3`, `key=SD-6`. `servicedesk_test.go:283` `TestGetOwnedRequestsCanceledContext` — `errors.Is(err, context.Canceled)`와 nil. `internal/service/servicedesk_test.go:39` `TestSyncServiceDeskRequestsStoresPartialResult` — 탈락 에러가 있어도 남은 슬라이스는 저장되고 Partial에 실린다. `servicedesk_test.go:79` `TestSyncServiceDeskRequestsAllDropped` — upserted 0, partial true, 빈 슬라이스 upsert. `servicedesk_test.go:108` `TestSyncServiceDeskRequestsCanceled`. `internal/server/servicedesk_test.go:43` `TestSyncServiceDeskRequestsRouteFailure` — 500 `{"error":"secret detail"}`. `POST /tasks/jira` 핸들러 본문(`route.go:54`)은 수정하지 않았다.
- AC: AC-1 `TestGetOwnedRequests`·`TestGetOwnedRequestsJoinsSiteURL`·`TestGetOwnedRequestsOverHTTP`. AC-2 페이지 실패·상한·취소·항목 탈락·빈 목록·전부 탈락 테스트와 `TestUpsertServiceDeskRequests`의 nil upsert(기존 SD-2 유지). AC-3 `TestUpsertServiceDeskRequests`·`TestInitialMigrationSchema`. AC-4 `TestSyncServiceDeskRequestsRoute`·`TestSyncServiceDeskRequestsRouteFailure`. `make test` 전 패키지 통과.
- 편차: none. 항목 디코딩 실패 픽스처는 페이지 JSON이 유효하도록 숫자 `1`이다. 잘못된 토큰은 페이지 파싱 실패가 되어 항목 탈락과 구분되지 않는다.

## Key Decisions

- 페이지 상한은 Jira `maxSearchPages`와 같이 100이다. 100번째 페이지를 읽고도 끝나지 않으면 그때까지 모은 항목을 버리고 nil을 반환한다.
- status는 TrimSpace가 비어 있지 않으면 앞뒤 공백을 포함한 원문이다. key와 title만 TrimSpace 결과를 저장한다.

## Manual Verification

- [ ] 실제 JSM `GET /rest/servicedeskapi/request`와 scoped 게이트웨이 URL은 호출하지 않았다. 배포 전에 본인 요청이 `service_desk_requests`에 기대 컬럼만으로 들어가는지 확인한다.
