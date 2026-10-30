---
Application: wharf
JiraTicket: DEV-26
PlanType: single-step
Timestamp: 20261030123500
Title: frontend-api
---

# 웹앱(SPA)에 필요한 백엔드 API

`docs/design` 웹앱이 쓰는 API 를 먼저 프론트엔드 관점에서만 나열하고(1절), 그 목록을 현재 서버와 비교해 만들 것·바꿀 것·지울 것을 정한 뒤(2절), 백엔드 구현 계획을 세운다(3절). 데이터 구조 변경은 DEV-25 에서 끝났다.

## 1. 프론트엔드가 필요로 하는 API (백엔드 무관)

화면은 Today 하나와 그 위의 모달(Task 상세, Settings)이다. 화면별로 필요한 읽기·쓰기를 적는다.

### Today 로딩
- 모든 Task 요약: 키·출처·제목·상태·시작·마감, 그리고 그룹 행 진행률 `2/5` 를 위한 todo 전체 수와 완료 수.
- 미완료 Todo 전체: Today 목록(오늘 할 수 있는 것, 기한 지난 것 포함)은 클라이언트가 로컬 날짜로 고른다. 같은 목록이 Request Inspector 의 `Link an existing todo` 피커(미완료만, 오늘 것 먼저)에도 쓰인다. Todo 마다 note·reminders·연결된 요청이 있어야 행 아이콘과 개수를 그린다.
- 오늘 완료한 Todo: `Completed today` 섹션. 로컬 자정 이후 완료한 것.
- 열린 Service Desk 요청: Done·Cancel 이 아닌 것. key·제목·상태·생성 시각·상태 변경 시각(`changed today`, `2d in status`).
- 서버 생존 확인: `Can't reach the Wharf server` 토스트의 Retry·자동 닫힘.

### Todo (Inspector, Task 모달의 Todos)
- 생성(Task 지정, 끝에 추가), 전체 수정(description·note·완료·start/end·reminders·순서·Task 이동), 삭제.
- 요청 연결·해제.
- Task 의 Todo 목록(Task 모달).
- 특정 요청에 연결된 Todo 목록(Request Inspector `Linked todos`, 완료한 것 포함).

### Task (Inspector, Task 모달)
- 상세(전체 description, 관계와 저장된 관계 대상 요약).
- Local Task 전체 수정(제목·상태·시작·마감·description·관계), 삭제(Jira 는 `Remove from Wharf`).
- Jira Task 한 건 다시 가져오기(`Sync from Jira`): 결과가 바뀐 필드 / 변경 없음 / Jira 에 없음.
- `Open in Jira`: Jira site URL.

### Settings › Atlassian Cloud
- 현재 연결 설정(URL, Email, API key — 보기/숨기기 토글이 있어 값이 필요).
- 저장(`Save`), 저장 전 값으로 연결 테스트(Jira 와 Service Desk 접근 확인, 확인 시각).
- `Sync now`: Jira 와 Service Desk 를 한 번에 동기화하고 결과 개수(갱신한 이슈, 열린 요청, 이번에 닫힌 요청, 실패 수와 메시지)를 받는다. 실패는 인증 실패·연결 실패를 구분해야 토스트가 다르다.
- 마지막 동기화(시각, 완료/부분, 이슈 수, 열린 요청 수).

### Settings › General (App Server)
- 사용 중인 config 경로, 포트(주소), DB 경로, 버전, 재시작 대기 중인 새 config 경로.
- `Choose…`: 서버가 macOS 파일 선택창을 띄워 경로를 받는다. `Open`: 서버가 config 파일을 연다. `Undo`: 고른 경로 취소. `Restart server`.

### 화면 URL
- 디자인 핸드오프가 제안한 SPA 경로(`/task/WHF-003`, `/settings/general`)가 API 경로와 겹치지 않아야 한다.

## 2. 현재 서버와의 비교

| 필요 | 현재 | 결정 |
| --- | --- | --- |
| API 경로 | 루트(`/tasks`, `/todos`…) | 모두 `/api` 아래로 옮긴다. SPA 경로(`/settings/…`, `/task/…`)와 정적 파일 서빙(3단계)이 겹치지 않게 한다. |
| Task 목록 + todo 수 | `GET /tasks` (수 없음) | `todo_count`·`done_todo_count` 를 더한다. |
| 미완료 Todo, Task 별 Todo | `GET /todos?task_id=&is_done=` | 유지. |
| 오늘 완료한 Todo | 없음 | `completed_since=RFC3339` 필터(`completed_at >= 값`). |
| 요청에 연결된 Todo | 없음 | `service_desk_request=KEY` 필터. |
| `today`·`end_date` 필터 | 있음 | 제거. 미완료 전체를 받아 클라이언트가 로컬 날짜로 고르므로 쓰지 않는다. |
| 열린 요청 목록 | 없음 | `GET /api/service-desk/requests?open=true` (`status_category != DONE`). |
| 통합 동기화 + 결과 + 마지막 기록 | `POST /tasks/jira`, `POST /service-desk/requests` 따로, 기록 없음 | `POST /api/sync/atlassian` 하나로 합치고 결과를 `atlassian_syncs` 에 남긴다. `GET /api/sync/atlassian` 이 마지막 기록. 기존 두 엔드포인트는 제거. |
| Jira Task 한 건 동기화 | 없음 | `POST /api/tasks/{id}/sync`. |
| Atlassian 설정 조회·저장·테스트 | config 파일에서 시작 시 한 번 읽음(필수 키) | `GET`/`PUT /api/settings/atlassian`, `POST /api/settings/atlassian/test`. 저장은 config 파일의 세 키만 바꾸고 실행 중인 클라이언트에 바로 반영한다. 키는 선택이 되고, 설정 전 동기화는 409. |
| App Server 설정 | 없음 | `GET /api/settings/app`, `POST /api/settings/app/config/choose`, `DELETE /api/settings/app/config/pending`, `POST /api/settings/app/config/open`, `POST /api/settings/app/restart`. |
| Local Task 생성 | `POST /tasks` | 유지. 디자인에 생성 진입점이 없지만, 지우면 Local Task 를 만들 방법이 사라진다(열린 질문으로 보고). |
| Task 상세·수정·삭제, Todo CRUD, 요청 연결 | 있음 | 경로만 `/api` 로. |
| 생존 확인 | `GET /healthz` | `GET /api/healthz`. |

## Key decisions

- **에러 구분**: 응답 본문 `{"error": "..."}` 에 선택 필드 `code` 를 더한다. Atlassian 이 401/403 을 주면 502 + `atlassian_unauthorized`, 요청을 보내지 못하면 504 + `atlassian_unreachable`, 연결 설정 전이면 409 + `atlassian_not_configured`, 한 건 동기화에서 Jira 404 면 404 + `jira_issue_not_found`. 상태 코드를 오류 메시지 문자열에서 꺼내지 않도록 `httputil` 이 상태 코드를 담은 오류 타입을 돌려준다(`errors.Is(err, ErrUnexpectedStatusCode)` 는 그대로).
- **통합 동기화**: Jira 이슈와 Service Desk 요청을 둘 다 받은 뒤에만 저장한다. 어느 쪽이든 받기가 실패하면 아무것도 바꾸지 않고 오류를 돌려준다(`Nothing was changed`). `closed_requests` 는 저장 전에 열려 있던 요청 중 이번에 DONE 카테고리로 받은 수다. 실패한 항목은 `failures`(수)와 `errors`(메시지)로 준다. 성공(부분 포함)만 기록한다.
- **한 건 동기화**: Jira Task 만(`409` 그 외). `GET /rest/api/3/issue/{key}` 로 받아 `ToTask` 로 바꾸고, 변환 오류면 저장하지 않고 502. 저장 전 값과 비교해 바뀐 필드(`title`·`description`·`status`·`start_date`·`due_date`)를 돌려준다.
- **Atlassian 설정 저장**: config 파일 전체를 다시 쓰지 않고 `atlassian_*` 세 줄만 바꾸거나 덧붙여 주석과 다른 키를 지킨다. 같은 디렉터리의 임시 파일에 쓴 뒤 rename 하고 권한은 0600. 저장 후 다시 `config.Load` 로 검증한다. 실행 중인 `atlassian.Client` 는 `Configure` 로 자격 증명을 바꾼다(RWMutex).
- **config 파일 없음**: 첫 실행에 config 파일이 없으면 기본값으로 시작한다. 웹앱에서 Atlassian 을 저장하면 파일이 생긴다.
- **API key 반환**: 디자인의 보기 토글 때문에 `GET /api/settings/atlassian` 이 토큰을 준다. 그래서 아래 로컬 가드를 함께 넣는다.
- **로컬 가드**: Host 가 `localhost`·`127.0.0.1`·`[::1]` 이 아니면 403(DNS rebinding), `Origin` 이 있고 Host 와 다르면 403(다른 사이트의 CSRF: 본문 없는 POST 로 재시작·동기화·파일 선택창을 띄우는 것). 같은 출처의 SPA·curl 은 그대로 된다.
- **App Server**: 파일 선택은 `osascript -e 'POSIX path of (choose file of type {"toml"})'`, 열기는 `open <path>`. macOS 가 아니면 501. 고른 경로는 메모리의 "재시작 대기" 값이고, 재시작은 응답(202)을 보낸 뒤 서버를 정상 종료하고 같은 실행 파일을 `WHARF_CONFIG=<경로>` 로 exec 한다. 재시작 전에 새 config 를 `config.Load` 로 검증해 잘못된 파일로 서버가 죽지 않게 한다. 다음에 직접 실행할 때는 다시 `WHARF_CONFIG`/기본 경로를 쓴다(경로를 따로 저장하지 않는다).
- **버전**: `debug.ReadBuildInfo` 의 모듈 버전(없으면 `(devel)`).

## Non-goals
- 자동·주기 동기화, 진행률 스트리밍(동기화는 요청 하나가 끝날 때까지 기다린다).
- SPA 정적 서빙(3단계).
- `docs/wharf-domain-for-design.md` 갱신(Backlog DEV-24).
- reminder 발송.

## Acceptance Contract
- AC-1: 1절의 모든 화면 요구가 2절 표의 API 로 충족되고, 각 API 가 server·service·db/adapter 테스트로 검증된다.
- AC-2: 제거한 API(`POST /tasks/jira`, `POST /service-desk/requests`, `today`·`end_date` 필터, `/api` 밖의 경로)는 404 다.
- AC-3: Atlassian 실패가 `code` 로 구분된다(unauthorized·unreachable·not_configured·jira_issue_not_found).
- AC-4: 로컬 가드가 다른 Host·다른 Origin 을 403 으로 막고 같은 출처는 통과시킨다.
- AC-5: `make check`·`make test`·`make build` 통과, `make gen` 재실행 diff 없음.
