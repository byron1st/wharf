---
Application: wharf
JiraTicket: DEV-20
PlanType: single-step
Timestamp: 20261027131953
Title: jira-rest-api
---

# acli 를 Jira Cloud REST API 로 대체

Report: [20261027131953_DEV-20_IMPL_jira-rest-api.md](./20261027131953_DEV-20_IMPL_jira-rest-api.md)

출처: Notion 개발 이슈 보드 DEV-20 "acli 를 REST API 로 대체" (유형 Refactor). 브랜치 `refactor/dev-20-jira-rest-api` (base `develop` `75c7591`). 사용자 지시에 따라 계획은 Claude 가 자체 승인했고, 사용자 판단이 필요해 보이는 결정은 아래 Key decisions 에서 Claude 가 정했다(PR 본문에 같은 목록을 남긴다). API token 이 아직 없으므로 실제 Jira 호출 없이 Atlassian 공식 REST API v3 스펙을 기준으로 구현·검증한다.

이슈 요구사항 요약: 기존 acli 이용 코드를 Atlassian Cloud REST API 이용 코드로 대체한다. 인증용 API Token·Site 주소·이메일을 config 로 받고, `Authorization: Basic base64(email:token)` 헤더로 호출한다. HTTP 요청 공통 코드는 `httputil` 패키지에 두고 mocking 용 interface(`HTTPClient{Get, Post}`)를 만든다. Get/Post 는 status code 기반 에러 검사까지 하고 response body 를 `[]byte` 로 반환한다.

## Non-goals

- 429/503 재시도·백오프, `Retry-After` 처리. 사용자가 수동으로 한 번 실행하는 검색이라 burst 한도에 닿을 가능성이 낮다(research 참조). 에러로 돌려주기만 한다.
- OAuth 2.0(3LO), cloudId 자동 조회, `GET /rest/api/3/myself` 시작 시 인증 확인, `GET /rest/api/3/field` 로 Start date 필드 ID 탐색. `customfield_10015` 는 지금처럼 상수다.
- `bulkfetch`·단건 issue 조회 등 다른 엔드포인트, 동기화 대상 JQL·필드 변경, 증분 동기화.
- `ToTask`·ADF→Markdown 렌더러·`ViewWorkItem` 등 변환 로직 변경. service·db·server 동작 변경(`SyncJiraTasks` 의 nil/non-nil 규칙 유지).
- 토큰 저장 방식 개선(키체인 등). config 파일의 평문 값이다.
- 서드파티 HTTP 라이브러리 도입.

## Key decisions

- **패키지 위치는 `internal/pkg/httputil`** — 이슈 본문은 `pkg/httputil` 이라고 적었지만 AGENTS.md 가 `pkg/` 를 모듈 밖 공유용으로만 허용하므로, 기존 `internal/pkg/{tool,task,todo}` 처럼 모듈 내부 공유 코드 위치에 둔다.
- **`httputil.HTTPClient` 는 Atlassian 을 모른다** — 인터페이스는 `Get(ctx, url, header)` / `Post(ctx, url, header, body)` 형태로 `context.Context` 가 첫 인자이고, 요청 헤더를 호출마다 받는다(정확한 시그니처·이름은 재량, 단 Get/Post 둘 다 이슈 요구대로 둔다). 인증 헤더는 atlassian adapter 가 만든다. 2xx 가 아니면 status code 를 담은 sentinel 에러(예: `ErrUnexpectedStatusCode`, 메시지에 `status=`, 응답 본문은 길이를 제한해 포함)를 돌려준다. 전송 실패·본문 읽기 실패도 각각 sentinel. 에러 메시지에 요청 헤더 값(토큰)을 넣지 않는다. 표준 `net/http` 만 쓰고, ctx 와 별개로 고정된 요청 timeout 을 둔다(값은 재량, 설정 키로 만들지 않는다). 생성자는 인터페이스를 반환하고 `.mockery.yml` 에 등록해 `make gen` 으로 `MockHTTPClient` 를 만든다.
- **검색은 `POST /rest/api/3/search/jql` 하나** — 폐기 중인 `/rest/api/3/search` 는 쓰지 않는다. 요청 바디는 `jql`(기존 `myTasksJQL`), `fields`(기존 `viewFields` 를 배열로), `maxResults`(값 재량, 100 이하 권장), 두 번째 페이지부터 `nextPageToken`. 응답의 `isLast` 가 true 이거나 `nextPageToken` 이 비면 멈춘다. 검색 결과에 모든 필드가 오므로 key 별 view 호출(N+1)은 없어진다. 요청 헤더는 `Authorization: Basic base64(email:token)`, `Accept`·`Content-Type: application/json`.
- **base URL 은 설정값 그대로 결합** — `{site_url}` 뒤에 `/rest/api/3/search/jql` 을 경로 결합한다(끝 `/` 유무와 관계없이, 기존 경로 prefix 보존). 그래서 `https://{site}.atlassian.net` 과 scoped token 용 `https://api.atlassian.com/ex/jira/{cloudId}` 둘 다 같은 키로 동작한다.
- **`GetMyTasks` 반환 규칙** — 어떤 페이지든 요청 실패·응답 파싱 실패면 `nil, err`(현재 "검색 실패 = 전체 실패" 와 같음, 앞 페이지 결과도 버린다). 페이지가 모두 성공하면 non-nil 슬라이스를 돌려주고, 항목 단위 문제는 그 항목만 빼거나(key 없음 → `ErrMissingWorkItemKey` + `index=`, 항목 JSON 디코딩 실패 → 항목용 sentinel + 식별 가능한 `index=`/`key=`) `ToTask` 필드 에러처럼 task 를 넣은 채 에러를 join 한다. 한 항목의 잘못된 JSON 이 페이지 전체를 실패시키지 않도록 항목을 개별 디코딩한다. ctx 는 HTTP 요청까지 전달되며, 취소되면 `nil` 과 `errors.Is(err, context.Canceled)` 인 에러다. key 별 루프가 없어지므로 `ErrGetMyTasksCanceled`·`ErrFailedToParseViewOutput` 은 쓰이지 않으면 제거한다.
- **config 키** — `atlassian_site_url`, `atlassian_email`, `atlassian_api_token` 세 키를 추가하고 모두 필수다(acli 가 없으면 기동이 실패하던 것과 같은 수준). 빠진 키는 그 키 이름을 담은 sentinel 로 `Load` 가 실패하고, site URL 은 `http`/`https` scheme 과 host 가 있는 절대 URL 이어야 한다(아니면 별도 sentinel). 에러 메시지에 토큰 값을 넣지 않는다. `acli_path` 키와 `ACLIPath` 필드, `exec.LookPath`, `ErrFailedToFindACLI` 는 제거한다 — 남은 설정 파일의 `acli_path` 는 이제 기존 `ErrUnknownConfigKeys` 로 거부된다(배포 전이라 수용). `config.example.toml` 머리말의 "Every key is optional" 은 고쳐 쓴다. 환경변수는 추가하지 않는다.
- **acli 흔적 제거** — `internal/pkg/tool` 패키지(테스트 포함), `internal/pkg/mocks/tool_tool.go`(`make gen` 으로 재생성되어 사라짐), `.mockery.yml` 의 `tool.Tool` 항목, `JiraSearchOption`/`JiraViewOption`, acli search 출력 fixture(`testdata/search*.json`)를 지운다. `tool` 은 acli 전용 사용처만 있어 이 변경으로 고아가 된다. `view_*.json` IssueBean fixture 는 REST 응답 페이지 fixture 의 재료로 재사용해도 된다(형식이 같다).
- **`ViewWorkItem` 등 기존 이름은 유지** — 변환 계층을 건드리지 않기 위해서다(이름 변경은 non-goal).
- **main wiring** — `httputil` 클라이언트를 만들어 `atlassian.NewClient` 에 site URL·email·token 과 함께 넘긴다. 생성자 인자 모양(개별 인자 vs 작은 설정 struct)은 재량.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | `httputil` 의 Get/Post 가 ctx 를 붙인 요청을 주어진 헤더(Post 는 본문 포함)로 보내고 2xx 면 응답 본문 `[]byte` 를 돌려준다. 2xx 가 아니면 status code 가 메시지에 드러나는 sentinel 에러이고(응답 본문 일부 포함), 요청 헤더 값은 에러에 나타나지 않는다. 연결 실패는 전송 실패 sentinel, 취소된 ctx 는 `errors.Is(err, context.Canceled)`. | `internal/pkg/httputil` 의 `httptest.Server` 기반 테스트 |
| AC-2 | `GetMyTasks` 가 `POST {site_url}/rest/api/3/search/jql` 를 `Authorization: Basic base64(email:token)`·JSON 헤더와 `jql`·`fields`(기존 8개 필드)·`maxResults` 바디로 호출하고, `nextPageToken` 으로 다음 페이지를 이어 `isLast`/토큰 부재에서 멈춘다. 결과 task 는 기존 `TestGetMyTasks` 의 4개 task(Markdown description·날짜·관계 포함)와 같다. site URL 의 끝 `/` 와 `/ex/jira/{cloudId}` 같은 경로 prefix 가 보존된다. | `internal/adapter/atlassian/client_test.go` (`MockHTTPClient` 로 URL·헤더·바디 JSON 검증, 2페이지 이상 시나리오) |
| AC-3 | 실패 규칙: 첫 페이지 또는 이후 페이지의 요청 실패·파싱 실패 → `nil` + 에러(원인 sentinel 로 `errors.Is` 가능). key 없는 항목·디코딩 불가 항목·`ToTask` 필드 에러 → 다른 task 는 유지되고 에러에 `index=`/`key=` 가 드러난다. 결과 없음 → 빈 non-nil 슬라이스, 에러 없음. 취소된 ctx → `nil` + `context.Canceled`. | `client_test.go` 의 실패 시나리오 테스트 |
| AC-4 | `config.Load` 가 세 키를 읽어 `Config` 에 담는다. 키가 하나라도 없으면 빠진 키 이름을 담은 sentinel, site URL 이 http(s) 절대 URL 이 아니면 별도 sentinel 로 실패한다. 실패 메시지에 토큰 값이 없다. `acli_path` 는 unknown key 로 거부된다. `config.example.toml` 이 세 키와 scoped token gateway URL 사용법을 설명한다. | `internal/config/config_test.go`, `config.example.toml` |
| AC-5 | Go 코드·`.mockery.yml`·mock 에 acli·`internal/pkg/tool` 참조가 없고, `cmd/wharf/main.go` 가 `httputil` 클라이언트로 `atlassian.NewClient` 를 만든다. `go.mod` 에 새 의존성이 없다. | `rg -i "acli|pkg/tool"` (docs/agents/dev 이력 제외), diff |
| AC-6 | acli 를 전제로 한 서술이 REST 기준으로 갱신되어 있다: `AGENTS.md` 의 `internal/config/` 항목, `internal/server/server.go` 의 `shutdownTimeout` 주석, `docs/design/wharf-domain-for-design.md` 의 동기화 소요 서술, research `server-service-structure`·`jira-adf-description`·`task-id-and-relations`·`jira-rest-search` 와 index. | diff |

검증하지 않는 위험: 실제 Jira Cloud 에 대한 호출(토큰 없음 — 스펙과 기존 IssueBean fixture 로만 검증), 실제 응답의 페이지 크기·`isLast` 동작, 사이트별 Start date 필드 ID 차이, 429 rate limit 상황, scoped token gateway 경로의 실제 동작.

## 참고 Research

- [jira-rest-search](../research/jira-rest-search.md) — 현재 acli search→view 흐름과 반환 규칙, `POST /rest/api/3/search/jql` 요청/응답 스펙·페이지네이션, Basic 인증과 gateway URL, rate limit, acli 출력이 REST IssueBean 과 같다는 근거, 바뀌는 파일 표.
- [server-service-structure](../research/server-service-structure.md) — config 로딩 규칙·sentinel 관례, main wiring 순서, `atlassian.Client` 반환 규칙과 `SyncJiraTasks` 의 처리, mockery 설정, lint·에러·테스트 관례.

## Authority Boundaries

- Discretion: `httputil`·`atlassian` 의 시그니처·타입·sentinel 이름, 응답 본문 크기 제한·요청 timeout 값, `maxResults` 값, 페이지 응답 구조체 모양, 무한 페이지 방지 장치 여부, fixture 구성(새 파일 vs 기존 IssueBean 재사용), 테스트 구성, 문서 문구.
- Must-ask: Key decisions·Non-goals 변경(엔드포인트 선택, 실패 규칙, config 키 이름·필수 여부, 패키지 위치), 재시도 도입, 서드파티 의존성 추가, service·db·server 동작 변경. (이번 실행에서는 사용자 지시에 따라 Claude 가 결정하고 PR 에 기록한다.)
- Stop conditions: 공식 스펙이 research 의 계약과 다르게 확인될 때(엔드포인트·필드·페이지네이션), `SyncJiraTasks` 의 nil/non-nil 규칙을 바꿔야만 구현이 가능할 때.
- Loop budget: 3

## TODOs

- [x] `internal/pkg/httputil` 에 `HTTPClient` 인터페이스(Get/Post)와 `net/http` 구현을 만들고, status code 검사·sentinel 에러·timeout 을 Key decisions 대로 갖춘 뒤 `.mockery.yml` 에 등록하고 테스트를 쓴다 (AC-1) (mechanical) (→ research: jira-rest-search)
- [x] `atlassian.client` 를 `HTTPClient` 기반 `POST /rest/api/3/search/jql` 페이지 루프로 바꾸고, Basic 인증 헤더·URL 결합·항목 개별 디코딩·반환 규칙을 구현한다. CLI 옵션 함수와 쓰이지 않게 된 sentinel 을 지운다 (AC-2, AC-3, AC-5) (design-bearing) (→ research: jira-rest-search)
- [x] `client_test.go` 와 fixture 를 REST 응답 형태로 바꿔 AC-2·AC-3 시나리오를 검증한다 (AC-2, AC-3) (mechanical) (→ research: jira-rest-search)
- [x] `config` 에 세 키·검증·sentinel 을 넣고 acli 해석을 제거하며, `config.example.toml`·config 테스트를 갱신한다 (AC-4) (mechanical) (→ research: server-service-structure)
- [x] `cmd/wharf/main.go` wiring 을 바꾸고 `internal/pkg/tool` 패키지와 `.mockery.yml` 항목을 제거한 뒤 `make gen` 으로 mock 을 재생성한다 (AC-5) (mechanical)
- [x] `AGENTS.md`·`server.go` 주석·design 문서·research 파일과 index 를 REST 기준으로 갱신한다 (AC-6) (mechanical)
