---
Application: wharf
JiraTicket: DEV-21
PlanType: single-step
Timestamp: 20261027160231
Title: service-desk-requests
---

# Service Desk 요청 수집

Report: [20261027160231_DEV-21_IMPL_service-desk-requests.md](./20261027160231_DEV-21_IMPL_service-desk-requests.md)

Notion DEV-21. 본인이 Atlassian Cloud Service Desk에 올린 요청을 Jira task와 다른 테이블에 동기화한다. 공식 계약은 Jira Service Management Cloud OpenAPI의 `GET /rest/servicedeskapi/request`(`getCustomerRequests`)와 JSM REST intro의 base URL이다. 티켓 curl과 같이 Basic 인증을 쓰고, `requestStatus=ALL_REQUESTS`는 그대로 둔다. `requestOwnership=ALL_REQUESTS`는 공식 문서에서 deprecated라 `OWNED_REQUESTS`만 쓴다.

## Non-goals

- `tasks` 테이블, Jira 동기화(`GetMyTasks`, `ToTask`, ADF), config 키, `go.mod`을 바꾸지 않는다.
- 담당자 컬럼을 만들지 않고, assignee를 채우려고 Jira issue·search·bulkfetch를 호출하지 않는다. reporter 컬럼도 없다. CustomerRequestDTO에는 assignee와 updated 시각이 없다.
- 목록 GET, 필터, Wharf 쪽 페이지, Swagger 주석을 두지 않는다.
- 이번 fetch에 없는 행을 지우지 않는다. 증분 동기화, 재시도, 401/404를 500이 아닌 상태로 나누지 않는다.
- expand, SLA, 댓글, 참여자, statusCategory, issueId를 저장하지 않는다.
- 새 마이그레이션 파일을 만들지 않는다. 첫 배포 전 스키마 변경은 `scripts/migrations/0001_initial_migration.sql`만 수정한다.

## Key decisions

- 도메인 타입은 `internal/pkg/servicedesk.Request`다. `atlassian.Client`에 조회를 추가하고, 같은 Basic 헤더와 `strings.TrimRight(siteURL, "/") + "/rest/servicedeskapi/request"`를 쓴다. site URL의 경로 prefix(`/ex/jira/{cloudId}` 포함)는 유지된다. JSM intro는 Basic을 `https://{site}.atlassian.net/rest/servicedeskapi/`, scoped·3LO 게이트웨이를 `https://api.atlassian.com/ex/jira/{cloudId}/rest/servicedeskapi/`로 적는다.
- 테이블 이름은 `service_desk_requests`다. 컬럼은 `issue_key` (PK), `title`, `status`, `created_at`, `status_at`이고 모두 NOT NULL이다. `created_at`은 `createdDate.epochMillis`, `status_at`은 `currentStatus.statusDate.epochMillis`(상태가 된 시각)를 UTC `time.Time`으로 저장한다. friendly·iso8601 문자열은 저장하지 않는다. epochMillis가 없거나 0이면 그 항목은 탈락한다.
- 제목은 `summary`를 TrimSpace 한 값이 비어 있지 않으면 그 값이다. 비어 있을 때만 `requestFieldValues`에서 `fieldId`가 `summary`이고 `value`가 JSON 문자열인 항목을 TrimSpace 한다. 그것도 비거나 문자열이 아니면 그 항목은 탈락한다. `status`는 `currentStatus.status` 원문이고, 비어 있으면 탈락한다. `issueKey`가 비어 있으면 탈락한다.
- 쿼리는 `requestOwnership=OWNED_REQUESTS`, `requestStatus=ALL_REQUESTS`, `limit=50`, `start`다. expand는 없다.
- 다음 `start`는 이전 `start + size`다. `isLastPage`이거나 `values`가 비어 있거나 `size`가 0이면 정상 종료하고, 그때까지 모은 항목을 upsert한다. HTTP 실패, 페이지 본문이 JSON이 아님, 페이지 상한 초과는 `nil`과 에러이며 upsert하지 않는다. 상한의 숫자만 재량이다. 모르는 JSON 필드는 페이지를 실패시키지 않는다. 항목 하나의 디코딩 실패는 그 항목만 탈락한다.
- upsert는 `issue_key` 기준이고 한 트랜잭션이다. 이번 결과에 없는 행은 남긴다. 빈 목록은 no-op이다.
- 서비스 메서드는 `SyncJiraTasks`와 같이 `nil` 슬라이스면 저장하지 않는다. 성공한 슬라이스(빈 슬라이스 포함)는 upsert한 뒤 `SyncResult`를 돌린다. 탈락한 항목은 저장하지 않고 `Partial`에 넣으며, 에러는 index 또는 issue key로 그 항목을 가리킨다. Jira의 "변환에 실패한 항목도 저장"은 이 경로에 적용하지 않는다. `SyncResult` 구조체 주석이 그 문장을 공통 계약처럼 적고 있으면, 그 문장은 `SyncJiraTasks` 설명으로 옮긴다.
- 라우트는 `POST /service-desk/requests`다. 본문은 없다. 성공 JSON은 `POST /tasks/jira`와 같이 `{"upserted","partial"}`다. 서비스 에러는 500이고 본문은 `{"error": err.Error()}`다.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | 조회가 위 쿼리·Basic 인증·URL 결합으로 페이지를 따르고, 저장 대상은 `issue_key`·제목·상태·`created_at`·`status_at`만 위 규칙으로 채워진다. 게이트웨이 prefix가 유지된다. | atlassian client 테스트 (`MockHTTPClient`로 URL·헤더·쿼리 검증) |
| AC-2 | 페이지 HTTP 실패·페이지 JSON 파싱 실패·상한 초과, 그리고 취소된 ctx(`errors.Is(err, context.Canceled)`)는 `nil`이라 서비스가 upsert를 호출하지 않는다. 항목 탈락은 나머지만 저장하고 `Partial`이 그 항목을 index 또는 issue key로 가리킨다. 성공한 빈 목록은 upserted 0, partial false, 기존 행 유지. 항목이 전부 탈락하면 upserted 0, partial true, DB는 바뀌지 않는다. | client 테스트와 service 테스트 |
| AC-3 | `service_desk_requests`는 `tasks`와 별도다. 같은 `issue_key`를 다시 저장하면 네 컬럼이 덮어써진다. 이번 결과에 없는 행은 남는다. 스키마 변경은 `0001_initial_migration.sql`에만 있다. | db 테스트, 0001 SQL |
| AC-4 | `POST /service-desk/requests`가 `{"upserted":N,"partial":bool}`를 돌린다. 서비스 에러는 500이고 본문은 `{"error": err.Error()}`다. `POST /tasks/jira`의 상태·JSON은 그대로다. | server route 테스트 |
| AC-5 | 새 모듈, 새 config 키, assignee를 채우는 두 번째 API가 없다. | diff, `go.mod` |

검증하지 않는 위험: 실제 JSM 호출, 서버가 강제하는 limit 상한, scoped 토큰 게이트웨이의 실제 동작.

## 참고 Research

- [service-desk-requests](../research/service-desk-requests.md) — `getCustomerRequests`의 공식 필드·페이지 계약과, 지금 Jira 동기화가 꽂혀 있는 client·service·`POST /tasks/jira`·0001 스키마 위치.

## Authority Boundaries

- Discretion: 메서드·sentinel 이름, 페이지 상한 숫자, 테스트 구성, 탈락 에러의 `index=`/`key=` 표기.
- Must-ask: 컬럼 추가, 다른 엔드포인트, GET 목록, tasks와 합치기, 없는 행 삭제, config 키, 서드파티 의존성.
- Stop conditions: 공식 스펙의 CustomerRequestDTO에 assignee 또는 updated 시각이 있을 때. 공식 문서가 게이트웨이에 `/rest/servicedeskapi`가 없다고 할 때.
- Loop budget: 3

## TODOs

- [x] `atlassian.Client`에서 owned Service Desk 요청을 조회해 `servicedesk.Request`로 매핑한다 (AC-1, AC-2) (mechanical) (→ research: service-desk-requests)
- [x] `service_desk_requests`를 `0001_initial_migration.sql`에 넣고 sqlc upsert를 `db.Manager`에 연결한다 (AC-3) (mechanical) (→ research: service-desk-requests)
- [x] 서비스 동기화 메서드와 `POST /service-desk/requests`를 추가하고 mock을 `make gen`으로 다시 만든다 (AC-2, AC-4, AC-5) (mechanical) (→ research: service-desk-requests)
- [x] AC-1부터 AC-4의 client·service·db·route 테스트를 고정한다 (AC-1, AC-2, AC-3, AC-4) (mechanical) (→ research: service-desk-requests)
