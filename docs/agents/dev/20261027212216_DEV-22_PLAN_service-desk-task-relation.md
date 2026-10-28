---
Application: wharf
JiraTicket: DEV-22
PlanType: single-step
Timestamp: 20261027212216
Title: service-desk-task-relation
---

# Service Desk 요청과 Task 의 relates-to 연결

Report: [20261027212216_DEV-22_IMPL_service-desk-task-relation.md](./20261027212216_DEV-22_IMPL_service-desk-task-relation.md)

출처: Notion 개발 이슈 보드 DEV-22 "servicedeck isuse 와 task 연동". 브랜치 `feat/dev-22-servicedesk-task-relation` (base `9f7ed5fc6`). 사용자 지시에 따라 계획은 Claude 가 자체 승인했고, 사용자 판단이 필요한 결정은 아래 Key decisions 에서 Claude 가 정했다(PR 본문에 같은 목록을 남긴다).

이슈 요구사항 요약:

- DEV-21 에서 저장하기 시작한 Service Desk 요청(`service_desk_requests`)과 task 사이에 relates-to 관계를 저장한다. 다대다이고, issue id ↔ task id 연결 테이블이면 충분하다.
- 연결을 추가·삭제하는 REST API 를 추가한다.
- `GET /tasks/{id}` 가 테이블 JOIN 으로 그 task 에 연결된 Service Desk 요청들을 함께 담아 반환한다.

## Non-goals

- `GET /tasks` 목록 응답, `PUT /tasks/{id}` 요청 본문에 연결을 넣지 않는다. 연결은 전용 API 로만 바꾼다.
- 요청 → task 역방향 조회 API, Service Desk 요청 목록·단건 GET 을 만들지 않는다.
- 연결에 관계 종류(type)·메모·생성 시각 컬럼을 두지 않는다. 관계는 relates-to 하나다.
- Jira 쪽(issue link 등)에 연결을 되돌려 쓰지 않는다. 연결은 wharf 에만 있다.
- `service_desk_requests` 스키마, `POST /service-desk/requests` 동기화 동작, config 키, `go.mod` 을 바꾸지 않는다.
- 새 마이그레이션 파일을 만들지 않는다. 배포 전 예외에 따라 `0001_initial_migration.sql` 만 고친다(AGENTS.md). 기존 로컬 `wharf.db` 재생성은 수동.

## Key decisions

- **연결 테이블** — `task_service_desk_requests (task_id TEXT NOT NULL, issue_key TEXT NOT NULL, PRIMARY KEY (task_id, issue_key))` 를 0001 에 추가한다. 방향 없는 relates-to 이므로 행 하나가 연결 하나다. "issue id" 는 `service_desk_requests` 의 PK 인 `issue_key`(Jira issue key)다. 기존 관례대로 FK 는 두지 않고 db.Manager 가 정합성을 지킨다. 역방향 조회가 없으므로 `issue_key` 인덱스는 두지 않는다(PK 가 `task_id` 조회를 덮는다).
- **API 형태** — 연결은 task 의 하위 리소스다. 본문 없이 경로로만 지정한다.
  - `PUT /tasks/{id}/service-desk-requests/{key}` → 204. 멱등이라 이미 있는 연결을 다시 PUT 해도 204 다. task 또는 요청이 저장되어 있지 않으면 404 이고 아무것도 저장되지 않는다(두 경우는 서로 다른 sentinel). 고른 이유: 다대다 연결 하나는 두 ID 로 완전히 식별되므로 POST + 본문보다 PUT 이 멱등성과 경로 대칭(DELETE)을 그대로 준다.
  - `DELETE /tasks/{id}/service-desk-requests/{key}` → 204. 그 연결이 없으면 404(기존 `DELETE /tasks/{id}`·`DELETE /todos/{id}` 와 같이 없는 대상 삭제는 404).
  - task 의 origin(local·jira)은 상관없다. 연결 테이블은 wharf 소유이고 Jira 동기화(`UpsertTasks`)가 건드리지 않는다.
- **존재 확인과 쓰기 우선 규칙** — 연결 추가는 task·요청 존재 확인과 쓰기를 한 트랜잭션에서 하고, 트랜잭션의 첫 문장은 쓰기여야 한다(`busyTimeoutPragma` 주석, DEV-18 `CreateTodo` 와 같은 방식). 예: `INSERT ... SELECT ... WHERE EXISTS(task) AND EXISTS(request) ON CONFLICT DO NOTHING` 의 영향 행 수가 0 일 때만 존재를 읽어 원인(task 없음 / 요청 없음 / 이미 연결됨)을 가른다. 구체적인 쿼리 형태는 구현 재량.
- **삭제 cascade** — `db.Manager.DeleteTask` 가 같은 트랜잭션에서 그 task 의 연결 행도 지운다. Service Desk 요청은 동기화가 지우지 않으므로 요청 쪽 cascade 는 없다.
- **GET 임베딩** — `GET /tasks/{id}` 응답에 `service_desk_requests` 배열을 추가한다. 연결 테이블과 `service_desk_requests` 를 JOIN 하는 한 쿼리로 읽고, `issue_key` 오름차순이며 연결이 없으면 `[]` 다. 원소는 `{"key","title","status","created_at","status_at"}`(시각은 기존 task 날짜와 같은 RFC3339 JSON). 저장 구조상 요청 없는 연결은 생기지 않지만 JOIN 은 inner join 이라 그런 행은 빠진다. 기존 응답 필드는 그대로다.
- **계층 배치** — DEV-16/18 과 같다. db.Manager 에 연결 추가·삭제와 task 의 요청 목록(JOIN) 메서드를 추가하고, service 의 `TaskDetail` 에 `[]servicedesk.Request` 필드를 더해 `GetTask` 가 채운다. `task.Task` 는 바꾸지 않는다. service 가 db 의 not-found 를 자기 sentinel 로 바꿔 server 가 adapter 를 import 하지 않게 한다. 기존 `Service`·`db.Manager` 인터페이스에 메서드를 추가하고 mock 은 `make gen` 으로 다시 만든다.
- **에러 → 상태 코드** — 기존 `errorStatuses` 표에 새 service sentinel 을 추가한다: 연결 추가의 task 없음·요청 없음 404, 삭제할 연결 없음 404, 그 외 500. 4xx 본문은 기존과 같은 `{"error":"..."}`.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | 0001 적용 후 `task_service_desk_requests` 테이블이 `(task_id, issue_key)` PK 로 있고, `tasks`·`service_desk_requests` 스키마는 바뀌지 않았다. 스키마 변경은 0001 에만 있다. | db 테스트, 0001 SQL diff |
| AC-2 | `PUT /tasks/{id}/service-desk-requests/{key}` 가 저장된 task(local·jira)와 저장된 요청을 연결하고 204 를 반환한다. 같은 PUT 반복은 204 이고 행은 하나다. 한 task 에 여러 요청, 한 요청에 여러 task 를 연결할 수 있다. task 가 없거나 요청이 없으면 404 이고 행이 생기지 않는다. | db 테스트(직접 SQL 로 행 확인), service 테스트, server 테스트 |
| AC-3 | `DELETE /tasks/{id}/service-desk-requests/{key}` 가 그 연결 행만 지우고 204 를 반환한다. 연결이 없으면 404. 같은 요청의 다른 task 연결과 요청 행 자체는 남는다. | db 테스트, server 테스트 |
| AC-4 | `GET /tasks/{id}` 응답에 `service_desk_requests` 가 JOIN 으로 읽은 요청들(`key`, `title`, `status`, `created_at`, `status_at`)을 `key` 오름차순으로 담고, 연결이 없으면 `[]` 다. 기존 필드(`parent`, `children`, `linked` 등)는 그대로다. | db JOIN 테스트, service 테스트, server 응답 테스트 |
| AC-5 | `DELETE /tasks/{id}` 가 그 task 의 연결 행도 지우고 다른 task 의 연결과 요청 행은 남긴다. `UpsertTasks`(Jira 동기화)와 `UpsertServiceDeskRequests` 후에도 연결은 남는다. | db 테스트 |
| AC-6 | 기존 task·todo·동기화 API 동작과 기존 테스트가 그대로 통과한다. `MockService`·`MockManager` 가 새 메서드로 재생성되어 있고 `make gen` 후 diff 가 없다. 새 모듈·config 키가 없다. | 기존 테스트, `make gen`, `go.mod` |

검증하지 않는 위험: 동시 요청 간 경합(단일 연결 + busy_timeout 에 맡김), 실제 JSM 데이터로 만든 연결, 기존 로컬 `wharf.db` 재생성(수동).

## 참고 Research

- [server-service-structure](../research/server-service-structure.md) — route/server/service 구조, `getTask` 응답 매핑·`writeError`·`errorStatuses`, `db.Manager` 계약과 `inTx`, mockery·에러·테스트 관례, db 테스트의 직접 SQL 확인 방식.
- [sqlite-sqlc-setup](../research/sqlite-sqlc-setup.md) — sqlc 설정·`make modelgen`, 스키마와 매핑, sqlc SQLite 생성 형태(`:execrows`, `INSERT ... SELECT ... RETURNING`, `sqlc.embed`)와 제약.
- [task-id-and-relations](../research/task-id-and-relations.md) — 관계 테이블의 쓰기 소유 규칙, SQLite 쓰기 우선 트랜잭션 제약, `DeleteTask` 의 현재 삭제 범위(관계·todo cascade).
- [service-desk-requests](../research/service-desk-requests.md) — `service_desk_requests` 스키마(`issue_key` PK)와 `servicedesk.Request` 매핑, 동기화가 행을 지우지 않는다는 규칙.

## Authority Boundaries

- Discretion: db·service·server 의 메서드·타입·sentinel·헬퍼 이름과 시그니처, 쿼리 구성(존재 확인 방식, JOIN 쿼리 형태), 응답 원소 타입 이름, route 를 별도 파일로 둘지, 테스트 구성.
- Must-ask: Key decisions·Non-goals 변경(API 경로·메서드·상태 코드, 존재 확인 규칙, cascade, 응답 필드 이름), 새 마이그레이션 파일, `task.Task`·`servicedesk.Request` 필드 변경, 서드파티 의존성 추가. (이번 실행에서는 사용자 지시에 따라 Claude 가 결정하고 PR 에 기록한다.)
- Stop conditions: sqlc 가 필요한 쿼리를 생성하지 못해 생성 코드 밖의 손으로 쓴 SQL 이 필요할 때, 연결 추가를 쓰기 우선 규칙을 지키며 구현할 수 없을 때.
- Loop budget: 3

## TODOs

- [x] 0001 에 `task_service_desk_requests` 를 추가하고 `scripts/queries.sql` 에 연결 추가(존재 확인 포함)·삭제·task 의 요청 JOIN 조회·task 연결 일괄 삭제 쿼리를 추가한 뒤 `make gen` (AC-1, AC-2, AC-3, AC-4, AC-5) (mechanical) (→ research: sqlite-sqlc-setup)
- [x] `db.Manager` 에 연결 추가·삭제·task 의 요청 목록 메서드를 추가하고 `DeleteTask` 가 연결 행도 지우게 한 뒤 db 테스트 작성 (AC-1, AC-2, AC-3, AC-4, AC-5) (mechanical) (→ research: task-id-and-relations)
- [x] `service.Service` 에 연결 추가·삭제를 추가하고 `TaskDetail`·`GetTask` 가 요청 목록을 담게 하며 not-found 를 sentinel 로 변환, `make gen` 후 MockManager 기반 테스트 작성 (AC-2, AC-3, AC-4) (mechanical) (→ research: server-service-structure)
- [x] server 에 두 라우트와 `GET /tasks/{id}` 의 `service_desk_requests` 매핑, 에러 → 상태 코드 매핑을 추가하고 MockService 기반 테스트 작성 (AC-2, AC-3, AC-4, AC-6) (mechanical) (→ research: server-service-structure)
- [x] research(`server-service-structure`, `sqlite-sqlc-setup`, `task-id-and-relations`, `service-desk-requests`)와 index 를 구현 후 상태로 갱신 (AC-6) (mechanical)
