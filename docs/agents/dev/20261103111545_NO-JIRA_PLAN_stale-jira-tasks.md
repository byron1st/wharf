# PLAN: 동기화에서 빠진 Jira Task 재조회와 Today 의 완료 Task 숨김

## 배경

전체 Atlassian 동기화(`SyncAtlassian`)는 `assignee = currentUser() AND statusCategory != done` JQL 로 이슈를 받는다. 이미 저장된 Task 가 Jira 에서 Done 이 되면 결과에서 빠지고, `UpsertTasks` 는 받은 것만 쓰므로 DB 에는 마지막 상태(예: `in-progress`)가 그대로 남는다. Today 가 모든 Task 를 보여주도록 바뀐 뒤로 이 Task 들이 열린 상태로 계속 보인다.

## Done when

- **A-1** 전체 동기화 때, DB 에 `originated = jira` 이고 상태가 `done`·`canceled` 가 아닌데 이번 검색 결과에 없는 Task 를 `GetTask` 로 하나씩 다시 받아 검색 결과와 함께 저장한다. 증거: `internal/service/sync_test.go` 에 "검색 결과에 없는 열린 Jira Task 는 GetTask 결과(done)로 UpsertTasks 에 포함된다" 테스트가 통과한다.
- **A-2** 재조회 대상이 아닌 Task(local 출처, 이미 `done`/`canceled`, 검색 결과에 있는 것)는 `GetTask` 를 부르지 않는다. 증거: 같은 테스트에서 mock 에 해당 호출 기대가 없고 `AssertExpectations` 가 통과한다.
- **A-3** 재조회 하나가 실패하면(404 포함) 그 Task 는 저장된 그대로 두고, 실패를 `IssueFailures` 수와 `errors` 메시지에 더한다. 나머지는 정상 저장되고 동기화는 성공으로 기록된다. 증거: 실패 케이스 서비스 테스트 통과.
- **A-4** 모든 받기(검색, Service Desk, 재조회)가 끝난 뒤에만 저장한다는 기존 규칙을 지킨다. 검색이나 Service Desk 받기가 실패하면 재조회도 저장도 하지 않는다. 증거: 기존 `TestSyncAtlassianChangesNothingWhenAFetchFails` 가 수정 없이 통과한다.
- **C-1** Today 는 `done`·`canceled` Task 를 숨긴다. 단, 오늘 할 Todo(미완료, 시작일 없음 또는 오늘 이전)가 남아 있는 Task 는 계속 보인다. 증거: 새 vitest 테스트(필터 함수 단위 테스트) 통과, 그리고 로컬 실행에서 사람이 확인하는 시나리오(Jira 에서 Done 처리 → 동기화 → Today 에서 사라짐).
- 저장소 게이트: `make check`, `make test`, `make web-check` 통과. 기존 테스트를 약화하거나 지우지 않는다.

검증하지 않는 것: 실제 Jira Cloud 대상 자동 테스트는 없다. 로컬 실행 시나리오는 사용자의 Jira 데이터에 Done 으로 바꿀 이슈가 있을 때만 사람이 확인할 수 있다.

## Out of scope

- 담당자가 바뀐 이슈 처리. 재조회해도 상태가 열려 있으면 그대로 남고, 매 동기화마다 다시 재조회된다.
- 재조회 병렬화, 호출 수 제한.
- `atlassian_syncs` 기록·API 응답 형식 변경(재조회 건수 필드 추가 등).
- Jira 에서 삭제된 이슈의 로컬 Task 자동 삭제.

## Do not touch

- JQL(`myTasksJQL`)과 `GetMyTasks`, `GetTask` 의 동작.
- DB 스키마, `db.Manager` 인터페이스, HTTP API 형식.
- 개별 Task 동기화(`SyncJiraTask`)의 동작.

## Key decision

- 재조회 실패는 동기화 전체를 실패시키지 않고 항목 실패로 센다. 검색은 이미 성공했으므로, 일부 이슈 하나 때문에 나머지 갱신을 버리지 않는다. 404 도 같은 방식으로 보고해 사용자가 로컬 Task 를 직접 지울 수 있게 한다.

## 접근

- `service.SyncAtlassian`: 두 받기가 끝난 뒤 `db.ListTasks` 로 재조회 대상을 고르고 `jira.GetTask` 로 받아 검색 결과 뒤에 붙인 다음 한 번에 `UpsertTasks` 한다. 실패는 기존 `taskErrs` 에 합친다. `Issues` 는 지금처럼 검색 결과 수로 둔다.
- 웹: Today 필터를 `web/src/lib/` 의 순수 함수로 두고 `TodayPage` 가 쓴다.
