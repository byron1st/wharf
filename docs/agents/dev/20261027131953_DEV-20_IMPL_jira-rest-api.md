---
Application: wharf
JiraTicket: DEV-20
ReportType: single-step
Timestamp: 20261027131953
Title: jira-rest-api
ReviewBase: `git diff 75c7591` (작업 트리, 미커밋) + 새 파일 `internal/pkg/httputil/`, `internal/pkg/mocks/httputil_httpclient.go`
---

# acli 를 Jira Cloud REST API 로 대체

Plan: [20261027131953_DEV-20_PLAN_jira-rest-api.md](./20261027131953_DEV-20_PLAN_jira-rest-api.md)

See the change: `git diff 75c7591` 와 `git status --short` 의 미추적 파일. 이 보고서의 `path:line` 은 그 작업 트리 기준이다.

## Summary
Jira 동기화가 외부 acli 실행(search 후 key 별 view N+1) 대신 Jira Cloud REST API v3 `POST /rest/api/3/search/jql` 페이지 루프를 Basic 인증으로 호출하도록 바꿨다. 공통 HTTP 코드는 mock 가능한 `internal/pkg/httputil.HTTPClient{Get, Post}` 로 분리했고, 인증 정보는 config 의 필수 키 세 개로 받는다. `GetMyTasks` 의 nil/non-nil 반환 규칙은 그대로라 service·server 동작은 바뀌지 않는다.

## Red Flags
- **RF1** `internal/adapter/atlassian/client.go:104` - 페이지 수 상한이 없다: 서버가 `isLast:false` 와 토큰을 끝없이 돌려주면 루프가 멈추지 않는다. 요청마다 30초 timeout 은 있지만 `POST /tasks/jira` 요청 ctx 에는 deadline 이 없다(클라이언트 연결 종료 시 취소만). 계획상 재량 항목이라 의도적으로 생략했고 주석에 업그레이드 경로를 남겼다.
- **RF2** `internal/pkg/httputil/httputil.go:57` - 실제 Jira 호출 없이 스펙과 httptest 로만 검증했다(토큰 없음). 실제 응답의 `isLast`/`nextPageToken` 동작, 401/400 본문 형태는 미확인.

## Open Questions
- **OQ1** `internal/pkg/httputil/httputil.go:74` - 2xx 응답 본문은 크기 제한 없이 `io.ReadAll` 한다. 계획의 "응답 본문 크기 제한" 은 에러 메시지용(512바이트)으로만 해석했다. 검색 페이지가 100건 × 8필드라 실용상 문제없다고 판단.
- **OQ2** `internal/config/config.go:98` - site URL 검증 실패 메시지에 URL 원문이 들어간다. 사용자가 URL 에 userinfo(`https://user:pw@...`)를 넣으면 그대로 노출된다. 토큰 키 값은 들어가지 않는다.

## Plan Divergence
### Changed - details that differ from the plan
- `testdata/view_in_progress.json` 의 description 텍스트 `acli` → `wharf`: AC-5 의 `rg -i acli` 가 Go 테스트 기대 문자열에 걸리지 않도록. 렌더링 대상 ADF 구조는 같다.
- 항목 디코딩 실패 에러에는 `index=` 만 담는다(`key=` 없음): 디코딩이 실패하면 key 를 신뢰할 수 없고, 같은 sentinel 의 두 번째 origin 을 만들지 않기 위해서다. `index` 는 페이지를 넘어 이어지는 전역 순번이다.
### Added - implemented but absent from the plan
- `atlassian.ErrFailedToEncodeSearchRequest`: `json.Marshal` 에러 반환값을 처리하기 위한 sentinel(실제로는 발생 불가).
- `client_test.go` 에 실제 `httputil` + `httptest` 로 wire 요청(Basic auth, 경로 prefix, 바디)과 ctx 취소를 확인하는 테스트 2개.
### Deferred - planned but not implemented (deferred)
- None

## TODO Fulfillment
### TODO 1: `internal/pkg/httputil` HTTPClient + net/http 구현 + mockery 등록 + 테스트 - done
- 구현: `internal/pkg/httputil/httputil.go:33` `HTTPClient`, `:42` `NewHTTPClient`(timeout 30초), `:57` `do` - Get/Post 가 한 함수로 모여 sentinel 마다 origin 이 하나다. 헤더는 `Clone` 해서 붙이고 에러에는 넣지 않는다. 2xx 외는 `ErrUnexpectedStatusCode` + `status=` + 512바이트 `body=`. `.mockery.yml` 에 `httputil.HTTPClient` 등록.
- 테스트: `internal/pkg/httputil/httputil_test.go:49` `TestGet`, `:62` `TestPost`(메서드·경로·헤더·본문), `:82` `TestUnexpectedStatusCode`(401/503/304/본문 절단, 토큰 미노출), `:120` `TestConnectionFailure`, `:131` `TestCanceledContext`, `:143` `TestInvalidURL`, `:149` `TestTruncatedResponseBody`.
- AC: AC-1 — `make test` 의 `internal/pkg/httputil` ok
- 편차: none

### TODO 2: `atlassian.client` 를 search/jql 페이지 루프로 교체 - done
- Risk / Lens: high / 반환 규칙(nil vs non-nil)과 페이지 종료 조건을 line-by-line
- 구현: `internal/adapter/atlassian/client.go:38` `NewClient(httpClient, siteURL, email, apiToken)` - URL 결합(`TrimRight("/")` + 경로, prefix 보존)과 Basic·JSON 헤더를 생성 시 한 번 만든다. `:68` `GetMyTasks` - 페이지 실패는 `nil, err`, 항목은 `json.RawMessage` 로 개별 디코딩. `:114` `search`. `jira.go:162` `searchFields()`, `searchMaxResults = 100`. `SearchItem`·`JiraSearchOption`·`JiraViewOption`·`ErrGetMyTasksCanceled`·`ErrFailedToParseViewOutput` 제거.
- 테스트: TODO 3 참조.
- AC: AC-2, AC-3 — TODO 3 테스트; AC-5 — `rg -i "acli|pkg/tool"` 결과 config 테스트의 의도된 `acli_path` 거부 케이스뿐
- 편차: design-bearing 이지만 Key decisions 가 엔드포인트·반환 규칙·헤더를 모두 고정해 남은 선택(생성자 인자 모양 = 개별 인자)은 계획이 재량으로 넘긴 것이라 consultant 없이 진행.

### TODO 3: `client_test.go` 와 fixture 를 REST 형태로 교체 - done
- 구현: `internal/adapter/atlassian/client_test.go` - `page` 헬퍼가 기존 `view_*.json` IssueBean 을 `issues[]` 에 담는다. `searchBody` 는 정확한 요청 바디 바이트, `header` 는 Basic 자격 증명을 문자열로 고정. `testdata/search*.json` 삭제.
- 테스트: `:93` `TestGetMyTasks`(3페이지, 4개 task 동일), `:151` `TestGetMyTasksStopsPaging`(isLast / 토큰 부재), `:174` `TestGetMyTasksJoinsSiteURL`(끝 `/`, `/ex/jira/{cloudId}`), `:205` `TestGetMyTasksNoWorkItems`, `:215` `TestGetMyTasksPageErrors`(첫·이후 페이지의 요청/파싱 실패 → nil), `:251` `TestGetMyTasksSkipsBadItems`(null·디코딩 불가·key 없음, index 0/1/2), `:275` `TestGetMyTasksOnlyNullWorkItems`, `:285` `TestGetMyTasksCollectsFieldErrors`, `:353` `TestGetMyTasksCanceledContext`, `:369` `TestGetMyTasksOverHTTP`.
- AC: AC-2, AC-3 — `make test` 의 `internal/adapter/atlassian` ok
- 편차: `view_in_progress.json` 텍스트 변경(Plan Divergence 참조)

### TODO 4: config 세 키·검증·sentinel, acli 해석 제거, example·테스트 갱신 - done
- 구현: `internal/config/config.go:33` `Config` 의 `AtlassianSiteURL`/`AtlassianEmail`/`AtlassianAPIToken`, `:88` 필수 키 루프(`ErrMissingConfigKey`, `key=`), `:98` site URL 검증(`ErrInvalidAtlassianSiteURL`). `ACLIPath`·`exec.LookPath`·`ErrFailedToFindACLI` 제거. `config.example.toml` 머리말과 세 키·gateway URL 설명.
- 테스트: `internal/config/config_test.go:63` `TestLoad` - 세 키 읽기, `acli_path` 거부(`:144`), 키별 누락(`:153`, 토큰 미노출), 잘못된 site URL 5종(`:178`), http 허용.
- AC: AC-4 — `make test` 의 `internal/config` ok, `config.example.toml`
- 편차: none

### TODO 5: main wiring, `internal/pkg/tool` 제거, `make gen` - done
- 구현: `cmd/wharf/main.go:61` `atlassian.NewClient(httputil.NewHTTPClient(), cfg.AtlassianSiteURL, ...)`. `internal/pkg/tool/` 삭제, `make gen` 으로 `tool_tool.go` 삭제·`httputil_httpclient.go` 생성.
- 테스트: `make build` 성공(빌드 산출물은 삭제).
- AC: AC-5 — `go.mod` diff 없음, `rg -i "acli|pkg/tool" --glob '!docs/**'` 는 `config_test.go` 의 `acli_path` 거부 테스트만
- 편차: none

### TODO 6: 문서·research 갱신 - done
- 구현: `AGENTS.md` `internal/config/` 항목, `internal/server/server.go` `shutdownTimeout` 주석, `docs/design/wharf-domain-for-design.md` 2.6, research `server-service-structure`(의존 그래프·wiring·config·atlassian/httputil 계약·mock), `jira-adf-description`, `task-id-and-relations`, `jira-rest-search`(구현 결과 섹션 추가, 이전 흐름을 `75c7591` 기준으로 표시), `index.md`.
- 테스트: 해당 없음(문서)
- AC: AC-6 — diff
- 편차: none

## Manual Verification
- [ ] 실제 API token 발급 후 `config.toml` 에 세 키를 넣고 `make run` → `POST /tasks/jira` 가 내게 할당된 미완료 이슈를 저장하는지(`upserted`, `partial:false`) 확인
- [ ] scoped token 이면 `atlassian_site_url = "https://api.atlassian.com/ex/jira/{cloudId}"` 로 같은 호출이 성공하는지 확인
- [ ] 잘못된 토큰으로 500 응답과 로그에 `status=401` 이 남고 토큰 값이 없는지 확인

## Fix

### TEST-001: site URL 의 userinfo 가 에러 메시지로 노출 - fixed
- 원인: `internal/config/config.go` 의 `ErrInvalidAtlassianSiteURL` 메시지가 원본 `atlassian_site_url` 을 그대로 넣고, `url.Parse` 의 `*url.Error` 도 원본 URL 을 인용한다. `internal/pkg/httputil/httputil.go` 도 `url=` 에 원본 URL 을, 요청 생성 실패 시 `*url.Error` 를 그대로 넣는다.
- 수정: config 는 userinfo 가 있는 site URL 을 거부(`u.User != nil`, Basic 인증은 email/token 키로만)하고 메시지에 `u.Redacted()` 를 쓰며 parse 에러는 `errors.Unwrap` 으로 원인만 남긴다. httputil 은 `url=` 에 `req.URL.Redacted()` 를 쓰고, 요청 생성 실패 시 `*url.Error` 를 `.Err` 로 풀어 원본 URL 을 버린다(`do` 의 인자 `url` 은 `net/url` 과 겹쳐 `rawURL` 로 변경). 전송 실패는 net/http 가 이미 비밀번호를 제거한다.
- 테스트: `internal/config/config_test.go` `TestLoadInvalidSiteURLDoesNotLeakUserinfo`(기존 red → green), `TestLoadRejectsSiteURLWithUserinfo`(https + userinfo 거부·미노출) 추가. `internal/pkg/httputil/httputil_test.go` `TestErrorsRedactURLPassword`(non-2xx 와 요청 생성 실패에서 비밀번호 미노출) 추가. 세 테스트 모두 수정 전 실패 확인.
- 검증: `make check` 0 issues, `make test` 전체 ok.

### TEST-002: `GetMyTasks` 페이지 루프에 상한이 없음 - fixed
- 원인: `internal/adapter/atlassian/client.go` `GetMyTasks` 가 `isLast:false` 와 비어 있지 않은 `nextPageToken` 이 오는 한 무조건 다음 페이지를 요청해, 같은 토큰을 반복하거나 토큰 체인을 끝내지 않는 서버/게이트웨이에서는 ctx 가 끝날 때까지(sync 요청 ctx 는 deadline 이 없을 수 있음) 루프하며 tasks/errs 를 무한히 쌓는다.
- 수정: 상수 `maxSearchPages = 100`(× `searchMaxResults` 100 = 10,000 work item) 을 두고, 응답 토큰이 방금 보낸 토큰과 같으면 `ErrRepeatedSearchPageToken`(`page=N`), 100 번째 페이지에도 다음 토큰이 있으면 `ErrTooManySearchPages`(`max=100`) 로 nil 과 에러를 반환한다(“페이지 실패 = 전체 실패” 와 일관). sentinel 은 각각 단일 반환 지점, `atlassian: ` prefix. 상한을 추가하는 방법을 설명하던 기존 주석은 낡아 삭제했고 `GetMyTasks` doc 주석에 두 종료 조건을 추가했다.
- 테스트: `internal/adapter/atlassian/client_test.go` 에 `TestGetMyTasksRepeatedPageToken`(두 번째 페이지가 같은 토큰 반환), `TestGetMyTasksPageLimit`(100 페이지 모두 다음 토큰 반환 → 101 번째 요청 없이 에러), `TestGetMyTasksLastPageAtLimit`(100 번째 페이지가 마지막이면 정상 성공, off-by-one 경계) 추가. 앞의 두 테스트는 수정 전 실패 확인.
- 검증: `make check` 0 issues, `make test` 전체 ok.

### TEST-003: 응답 본문을 크기 제한 없이 읽음 - fixed
- 원인: `internal/pkg/httputil/httputil.go` `do` 가 status 와 관계없이 `io.ReadAll(resp.Body)` 로 본문 전체를 읽고, non-2xx 는 다 읽은 뒤에야 512 바이트로 자른다. 오동작하는 endpoint 가 요청 timeout(30s) 안에 거대한 본문을 보내면 메모리를 소진할 수 있다.
- 수정: 고정 상수 `maxResponseBodyBytes = 32 << 20`(32 MiB, ADF description 포함 100 issue search 페이지는 수 MiB 이하라 여유 있음; 설정 키 아님) 을 두고 `io.LimitReader(resp.Body, limit+1)` 로 읽는다. 2xx 는 limit 을 넘으면 새 sentinel `ErrResponseBodyTooLarge`(`httputil: response body too large`, `method=`, redacted `url=`, `limit=`, 단일 반환 지점) 를 반환한다. non-2xx 는 limit 을 `maxErrorBodyBytes`(512) 로 두어 에러 메시지에 쓰는 만큼만 읽는다. 본문을 끝까지 읽지 않게 되어 defer close 주석을 갱신했다.
- 테스트: `internal/pkg/httputil/httputil_test.go` 에 청크로 스트리밍하는 `newStreamingServer` 와 `TestResponseBodyTooLarge`(limit+1 → sentinel, `limit=33554432`, 비밀번호·토큰 미노출), `TestResponseBodyAtLimit`(정확히 limit 은 성공, off-by-one 경계), `TestLargeErrorBodyIsTruncated`(limit+1 크기 502 → `ErrUnexpectedStatusCode` 이고 too-large 아님, 본문 512 바이트로 절단) 추가. 테스트는 패키지의 비공개 상수를 같은 값의 테스트 상수로 미러링한다(테스트용 export 없음).
- 검증: `make check` 0 issues, `make test` 전체 ok.

### TEST-004: username-only userinfo 가 에러 메시지로 노출 - fixed
- 원인: `url.URL.Redacted()` 는 비밀번호만 `xxxxx` 로 가리고 username 은 그대로 둔다. `internal/config/config.go` 의 `ErrInvalidAtlassianSiteURL` 메시지와 `internal/pkg/httputil/httputil.go` 의 세 `url=` 필드가 `Redacted()` 를 써서, username 자리에 토큰을 넣은 URL(`https://hunter2@example.atlassian.net`)은 토큰이 노출된다.
- 수정: 두 패키지에 각각 비공개 helper `withoutUserinfo(u *url.URL) string`(URL 복사본의 `User = nil` 후 `String()`; config 쪽은 parse 실패 시 nil `u` 에 대해 `""`) 을 두고 `Redacted()` 호출을 모두 교체했다. 패키지 간 공유는 하지 않았다(config → httputil 의존을 새로 만들지 않기 위해). 관련 주석을 갱신했다.
- 테스트: `internal/config/config_test.go` `TestLoadInvalidSiteURLVariantsDoNotLeakUserinfo/username_only`(기존 red → green, 수정 안 함). `internal/pkg/httputil/httputil_test.go` 에 `TestErrorsDropURLUsername`(username-only URL 의 401 → `url=` 이 userinfo 없는 서버 URL 이고 `hunter2` 미노출) 추가, 수정 전 실패 확인.
- 검증: `make check` 0 issues, `make test` 전체 ok.
- 참고(미수정): `ErrFailedToSendRequest` 경로는 net/http 의 `*url.Error` 를 그대로 join 하는데, net/http 도 비밀번호만 `***` 로 가려 username-only userinfo 는 전송 실패 메시지에 남는다. 별도 finding 후보.
