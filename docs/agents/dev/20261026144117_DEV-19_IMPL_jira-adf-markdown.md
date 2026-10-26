---
Application: wharf
JiraTicket: DEV-19
ReportType: single-step
Timestamp: 20261026144117
Title: jira-adf-markdown
ReviewBase: `git diff c1a793d5e` (working tree, untracked `internal/adapter/atlassian/adf.go` 포함)
---

# Jira sync 시 task 본문(ADF)을 Markdown 으로 변환해 저장

Plan: [20261026144117_DEV-19_PLAN_jira-adf-markdown.md](./20261026144117_DEV-19_PLAN_jira-adf-markdown.md)

See the change: `git diff c1a793d5e` (+ 새 파일 `internal/adapter/atlassian/adf.go`). Every `path:line` anchor in this report is valid against that snapshot.

## Summary

`POST /tasks/jira` 동기화가 Jira description(ADF)을 평문 대신 CommonMark + GFM Markdown 으로 저장하도록, `internal/adapter/atlassian` 의 평문 렌더러를 새 `adf.go` 의 Markdown 렌더러로 대체했다. 변환은 adapter 안에서 끝나며 service·db·server·`go.mod` 는 바뀌지 않았고, `ToTask` 의 에러 계약도 그대로다.

## Red Flags

- **RF1** `internal/adapter/atlassian/adf.go:13` - 디코딩 실패 표면 확대: `adfNode.Attrs` 에 `level`·`order`(int) 필드를 추가했으므로, Jira 가 이 attrs 를 숫자가 아닌 값으로 보내면 `json.Unmarshal` 이 실패해 해당 work item 의 view 가 에러가 된다(이전에는 string attrs 만 디코딩해 실패할 여지가 없었다). ADF 스키마상 숫자라 수용했고, `timestamp` 만 `json.RawMessage` 로 받아 문자열·숫자·잘못된 값 모두 안전하게 처리한다.

## Open Questions

- **OQ1** `internal/adapter/atlassian/adf.go:96` - 같은 `-` marker 를 쓰는 두 목록(예: fixture 의 bulletList 다음 taskList)이 빈 줄로만 이어지면 CommonMark 에서 하나의 loose list 로 합쳐진다. plan 이 marker 를 `- ` 로 고정해 그대로 두었다. 분리하려면 `<!-- -->` 삽입이나 marker 교대(`*`)가 필요하다.
- **OQ2** `internal/adapter/atlassian/adf.go:206` - 인접한 text 노드의 mark 를 합치지 않으므로 `**a****b**` 같은 경계가 나올 수 있다(서로 다른 mark 가 맞붙는 경우 해석이 렌더러마다 다를 수 있음). 문자열 비교로만 검증했다.
- **OQ3** `internal/adapter/atlassian/adf.go:96` - list item 안에서 하위 목록만 tight 하게 붙이고, paragraph 둘 이상이나 code block 은 빈 줄로 잇는다(합쳐지지 않게). 이런 item 이 있는 목록은 loose 가 된다.

## Plan Divergence

### Changed - details that differ from the plan
- None

### Added - implemented but absent from the plan
- 연속된 inline 노드를 한 paragraph 로 묶는 처리(`renderBlocks`): taskItem/decisionItem 처럼 paragraph 없이 inline 을 담는 노드와 모르는 컨테이너를 하나의 규칙으로 다루기 위해.
- taskList 안에 바로 든 taskList(ADF 의 중첩 task 표현)를 이전 항목 아래로 들여쓰는 처리.
- `timestamp` 를 `json.RawMessage` 로 받아 숫자/문자열 모두 허용(RF1 참고).

### Deferred - planned but not implemented (deferred)
- None

## TODO Fulfillment

### TODO 1: `adfNode` 가 marks·attrs 를 디코딩하고 평문 렌더러를 Markdown 렌더러로 대체, `ToTask` 가 사용, TODO 주석 삭제 - done
- Risk / Lens: line-by-line (`renderList`·`applyMarks` 의 문자열 조립)
- 구현: `internal/adapter/atlassian/adf.go:13` `adfNode`/`adfMark` - marks(`href`)와 attrs(level, order, state, language, title, shortName, timestamp, text, url) 디코딩. `adf.go:51` `markdown` → `renderBlocks`(`:61`)/`joinBlocks`(`:96`)/`renderBlock`(`:114`)/`renderInline`(`:161`)/`applyMarks`(`:206`)/`renderCodeBlock`(`:254`)/`renderList`(`:274`)/`renderTable`(`:323`)/`renderCell`(`:371`) 로 Key decisions 의 출력 형식 구현. `internal/adapter/atlassian/jira.go:101` `ToTask` 가 `markdown()` 사용, `plainText`/`writeText` 와 `// TODO: Convert ADF to Markdown` 주석 삭제.
- 테스트: `internal/adapter/atlassian/jira_test.go:204` `TestToTaskDescription` (TODO 2 에서 작성)
- AC: AC-1 — `make test` 통과(`TestToTaskDescription` 21 케이스), AC-3 — diff(아래 Evidence)
- 편차: 재량 선택 — hardBreak 는 줄 끝 `\`, status 는 `[TEXT]`, panel 라벨 없음(blockquote 만), 표 셀 블록·hardBreak 구분은 `<br>`, mark 중첩은 link > strike > strong > em > code, media alt 는 버림, 모르는 inline 노드는 `text`+자식 렌더. design-bearing 이지만 Key decisions 가 출력 형식을 확정했고 남은 선택은 모두 Authority Boundaries 의 Discretion 목록 안이라 consultant 없이 진행.

### TODO 2: `TestToTaskDescription` 을 Markdown 기대값으로 바꾸고 노드·mark 별 케이스 추가, `client_test.go` 기대 description 갱신 - done
- 구현: `internal/adapter/atlassian/jira_test.go:204` 표 테스트 교체, `internal/adapter/atlassian/client_test.go:88` 기대 description 을 Markdown 으로 갱신(fixture 는 변경 없음).
- 테스트: `jira_test.go:204` `TestToTaskDescription` - null/빈 문서, heading 레벨 고정, 각 mark·중첩 순서·code backtick, 대응 없는 mark, hardBreak, bullet/ordered(order 9 시작·기본 1)/중첩·다중 paragraph item, task 체크·중첩 taskList, decision, code fence(언어·긴 fence·빈 내용 생략), blockquote/panel, rule, GFM 표(`|` 이스케이프·`<br>`·빈 셀 채움), expand 제목, mention/emoji/status/date(문자열·숫자·잘못된 값)/inlineCard, block/embedCard, 모르는 블록·inline 노드, media·빈 블록 생략. `client_test.go:67` `TestGetMyTasks` - fixture 의 heading·code/strong·목록·task 체크박스·mention·inlineCard autolink·hardBreak·code fence·rule.
- AC: AC-1, AC-2 — `make test` 통과
- 편차: none

### TODO 3: research `jira-adf-description` 과 index 를 구현 후 상태로 갱신 - done
- 구현: `docs/agents/research/jira-adf-description.md` - "현재 ADF 처리" 절을 "ADF → Markdown 렌더러" 절로 교체(디코딩 필드, 구조, 노드·mark 별 규칙, 알려진 한계), 흐름·테스트 위치 anchor 갱신. `docs/agents/research/index.md` 설명 갱신.
- 테스트: 해당 없음(문서)
- AC: AC-4 — research 파일
- 편차: none

## Key Decisions

- 사용자 사전 위임에 따라 Claude 가 정한 must-ask 성격 항목: 없음. Key decisions·Non-goals 를 벗어난 결정은 하지 않았다. PR 본문에는 TODO 1 편차 줄의 재량 선택 목록을 남긴다.

## Test Hardening (test-dev, light mode)

- Scope: diff vs c1a793d5e plus working tree; `internal/adapter/atlassian/adf.go`, `jira.go`.
- Phase 1: adf.go coverage already ~99% (only `renderTable` 95.7%, the `cols == 0` return is unreachable through ToTask output paths hit). Added one case to `TestToTaskDescription` in `jira_test.go` (multi-block list items, ordered `order` start, code block inside item).
- Phase 2: not applicable (no tests/e2e). Phase 3: out of scope (caller).
- Verification: `make check` 0 issues; `make test` green.

### Findings
- TEST-001 (suspected defect, adf.go:96-112 / renderBlocks): two adjacent top-level lists (e.g. bulletList then taskList, as in testdata/view_in_progress.json) are joined by a blank line and both use `-`, so CommonMark parses them as one loose list. Observed `"- a\n\n- b"`; expected two distinct lists. Surfaced in phase 1; no red test left.
- TEST-002 (suspected defect, adf.go:206-242 applyMarks): adjacent text nodes with the same mark are not merged. Observed `"**a****b**"`; expected `"**ab**"`. Phase 1; no test left.
- TEST-003 (minor, adf.go:323-367 renderTable): a tableRow with no cells renders an empty `|  |` row. Phase 1; no test left.
- Note: list items with multiple blocks render loose (`"- a\n\n  b"`); valid CommonMark, now pinned by a test.

## Fix

### Fix 1 — 2026-10-26 14:55 — 인접한 형제 목록의 marker 교대
- **Finding**: TEST-001
- **Root cause**: 형제 블록을 빈 줄로 잇는 `renderBlocks`/`joinBlocks` 경로에서 같은 계열의 목록이 연달아 오면 모두 `-`(또는 `N.`) marker 를 써서, CommonMark 가 bulletList 다음 taskList 같은 두 목록을 하나의 loose list 로 합쳤다.
- **Change**: 형제 블록이 조립되는 유일한 지점인 `renderBlocks` 에서 직전 블록이 같은 계열(bullet 계열 bulletList/taskList/decisionList, 또는 orderedList) 목록이면 `renderList(n, alt)` 로 대체 marker(`* ` / `N) `)를 쓰게 하고 연속되면 번갈아 쓰도록 했다. top level·blockquote/panel·expand·list item 내용 모두 이 경로를 지나므로 한 곳 수정으로 충분하며, 그 외에는 기본 marker 를 유지한다.
- **Regression test**: `TestToTaskDescription` 에 bullet+task+bullet, ordered+ordered, 목록 사이에 paragraph 가 있는 경우, blockquote 안 인접 목록 케이스를 추가했고 `client_test.go` 의 `view_in_progress.json` 기대값을 `* [x] Parse ADF description` / `* [ ] Map statuses` 로 갱신했다. 모두 통과.
- **Files changed**:
  - `internal/adapter/atlassian/adf.go:40`
  - `internal/adapter/atlassian/adf.go:67`
  - `internal/adapter/atlassian/adf.go:302`
  - `internal/adapter/atlassian/jira_test.go:325`
  - `internal/adapter/atlassian/client_test.go:92`
  - `docs/agents/research/jira-adf-description.md:22`
- **Verification**: `make check` ✅ (0 issues), `make test` ✅. `make race`/`make test-e2e` 는 동시성·서버 동작과 무관한 순수 렌더러 변경이라 생략.
- **Notes**: taskList 안에 바로 든 하위 목록 두 개가 연달아 오는 경우(`renderList` 의 nested 경로)는 같은 항목 아래에 줄바꿈으로만 이어져 하나의 하위 목록으로 합쳐질 수 있으나 이번 범위에서 다루지 않았다.

### Fix 2 — 2026-10-26 14:57 — 같은 mark 를 가진 인접 text 노드 병합
- **Finding**: TEST-002
- **Root cause**: `renderInline` 이 text 노드마다 `applyMarks` 를 따로 적용해, 같은 mark 를 가진 인접 노드가 `**a****b**` 처럼 delimiter 를 맞붙여 출력했고 CommonMark 는 이를 닫기+다시 열기로 안정적으로 파싱하지 않는다.
- **Change**: `renderInline` 이 노드를 순회하기 전에 `mergeText` 로 mark 집합이 같은(순서 무관, `adfMark` 값 비교라 link 는 href 까지 일치해야 함) 연속 `text` 노드의 텍스트를 하나로 합치게 했다. inline 렌더링의 유일한 진입점에서 입력만 정규화하므로 `applyMarks` 등 나머지 로직은 그대로다.
- **Regression test**: `TestToTaskDescription` 에 "adjacent text with the same marks is merged"(순서가 다른 strong+em, 같은 href link → `***ab*** [cd](https://example.com)`)와 "adjacent text with different marks stays separate"(strong 다음 strong+em, href 가 다른 두 link) 케이스를 추가했다. 수정 전 앞 케이스가 `***a******b*** [c](...)[d](...)` 로 실패함을 확인했고 수정 후 통과.
- **Files changed**:
  - `internal/adapter/atlassian/adf.go:6`
  - `internal/adapter/atlassian/adf.go:193`
  - `internal/adapter/atlassian/adf.go:232`
  - `internal/adapter/atlassian/jira_test.go:246`
  - `docs/agents/research/jira-adf-description.md:22`
  - `docs/agents/research/jira-adf-description.md:25`
- **Verification**: `make check` ✅ (0 issues), `make test` ✅. `make race`/`make test-e2e` 는 동시성·서버 동작과 무관한 순수 렌더러 변경이라 생략.
- **Notes**: mark 가 다른 인접 노드(예: strong 다음 strong+em)는 여전히 `**a*****b***` 처럼 delimiter 가 맞붙으며, 이를 공통 mark 로 묶어 중첩하는 것은 이번 결정 범위 밖이다. research 문서의 다른 `adf.go:N` 줄 번호(`renderInline` 등)는 이번 변경 이전부터 어긋나 있어 그대로 두었다.

### Fix 3 — 2026-10-26 14:58 — 셀 없는 table 행 제거
- **Finding**: TEST-003
- **Root cause**: `renderTable` 이 셀이 없는 `tableRow` 도 rows 에 그대로 모아, 빈 행이 `|  |` 로 출력되고 첫 행이 비면 빈 헤더가 되었다.
- **Change**: `renderTable` 의 행 수집 루프에서 `row.Content` 가 비어 있으면 건너뛰게 해, 남은 첫 행이 헤더가 되도록 했다. 행이 모이는 유일한 지점에서 입력만 거르므로 열 수 계산·출력 로직은 그대로다.
- **Regression test**: `TestToTaskDescription` 에 "table drops rows without cells"(빈 `content` 첫 행과 `content` 가 없는 중간 행 → `| Name |\n| --- |\n| a |`) 케이스를 추가했다. 수정 전 `|  |\n| --- |\n| Name |\n|  |\n| a |` 로 실패함을 확인했고 수정 후 통과.
- **Files changed**:
  - `internal/adapter/atlassian/adf.go:404`
  - `internal/adapter/atlassian/jira_test.go:440`
  - `docs/agents/research/jira-adf-description.md:12`
  - `docs/agents/research/jira-adf-description.md:20`
  - `docs/agents/research/jira-adf-description.md:21`
  - `docs/agents/research/jira-adf-description.md:22`
- **Verification**: `make check` ✅ (0 issues), `make test` ✅. `make race`/`make test-e2e` 는 동시성·서버 동작과 무관한 순수 렌더러 변경이라 생략.
- **Notes**: research 문서의 `adf.go:N` 참조를 현재 줄 번호로 모두 바로잡았다(`adfNode` 14, `markdown` 56, `renderBlocks` 68, `joinBlocks` 116, `renderBlock` 134, `renderInline` 190; `mergeText` 235·`applyMarks` 271·`renderList` 339 는 원래 맞음). Fix 2 Notes 에 남겼던 줄 번호 불일치는 이로써 해소되었다.

## Test Hardening — Round 1 (reduced re-entry after TEST-001/002/003)

- Scope: `internal/adapter/atlassian/adf.go` (Fix 1-3) and its tests in `jira_test.go` / `client_test.go`. Phase 2 skipped (no `tests/e2e`), Phase 3 out of scope (caller).
- Phase 1: all `adf.go` functions at 100% statement coverage except `renderTable` (96%): the `cols == 0` early return was untested. Added `TestToTaskDescription` case "table with only cellless rows renders nothing between paragraphs" (`jira_test.go`); `renderTable` now 100%.
- Verification: `make check` 0 issues, `make test` green, `make coverage` 93.7% total.
- Findings: none (no TEST-004+).

## 루프 종료 후 리뷰 반영 (사용자 위임)

- 반영: 1 이 아닌 번호로 시작하는 ordered list 는 CommonMark 에서 문단을 끊지 못하므로 list item 안에서도 앞에 빈 줄을 두게 했다(`mdBlock.list` 를 그런 목록에는 세우지 않음). `renderList` 의 목록 타입 나열을 `listFamily` 로 바꿨다. `mergeText` 의 줄바꿈된 조건문을 읽기 쉽게 정리했다. `make check`·`make test` 통과.
- 유지(PR 기록): 텍스트 미이스케이프(plan Key decisions 의 수용 위험), link href·autolink URL 미검증, 표 셀 안 목록·코드블록의 구조 손실, heading 안 hardBreak, 서로 다른 mark 가 맞닿은 구분자, `level`/`order` 정수 디코딩(ADF 스키마상 숫자), 테스트를 `jira_test.go` 에 둔 것(`markdown()` 이 unexported 라 `ToTask` 로만 검증), family 문자열 상수화, `renderList` 분리.

## PR 리뷰 대응 (byron1st/wharf#6, Grok Bot: Wharf Verifier)

- Blocker 1: link href·inlineCard·blockCard·embedCard URL 을 `safeURL`(`http`/`https`/`mailto` 만, `<`·`>`·개행 거부)로 거른다. 안전하지 않은 link 는 텍스트만, 카드는 URL 을 code span 으로 보여 따라가지 못하게 한다. 공백·괄호가 있는 href 는 `<href>` 형태로 넣는다. text·mention·emoji·status·expand 제목의 `<` 를 `\<` 로 이스케이프한다(code span 제외). research 에 "description 은 신뢰할 수 없는 Markdown 이므로 소비자가 sanitize 해야 함" 을 적었다.
- Should fix 2: `level`·`order` 를 `json.RawMessage` 로 받고 `attrInt` 가 `2`·`2.0`·`"2"` 를 읽으며 실패 시 기본값 1 로 떨어진다.
- Should fix 3: status 는 `\[TEXT\]` 로 써서 뒤의 `(...)` 와 링크가 되지 않는다.
- Should fix 4: `canEmphasize` 가 CommonMark left/right-flanking 조건을 앞뒤 문자로 확인해, 성립하지 않는 강조 delimiter(`**API(v2)**는`)는 생략한다. 테스트와 research 에 고정했다.
- Nit 5: codeBlock info string 은 ASCII 영숫자·`_+#.-` 만 남긴다. Nit 6: `mdBlock` doc comment 를 다시 감쌌다.
- `make check` 0 issues, `make test` 통과. 회귀 테스트 6건 추가.
- 재리뷰 Blocker(Blocker 1번 우회): 본문 text 에 직접 적힌 `[x](javascript:...)` 가 링크가 되던 문제. `escapeHTML` 을 `escapeText` 로 바꿔 `\`(먼저)·`<`·`[`·`]` 를 이스케이프한다(text·mention·emoji·status·expand 제목, code span 제외). 회귀 테스트 "link syntax typed into text is escaped" 추가. `make check`·`make test` 통과.
- 재리뷰 Blocker(code span 개행 탈출): code mark·안전하지 않은 card URL 의 code span 에 빈 줄이 있으면 code span 이 문단 경계에서 끊겨 내용이 raw HTML 로 새던 문제. `codeSpan` 이 줄바꿈을 공백으로 바꾼다. 백슬래시로 `\<` 를 무효화하던 우회는 0aaaa59 의 `escapeText` 로 이미 막혔고 회귀 테스트를 추가했다.
- 재리뷰 Nit: em+strong 은 감싸기 전 core 로 한 번 판정(strike 는 감싼 뒤 판정), ordered list 시작 번호를 0–999999999 로 고정. 회귀 테스트 4건 추가, `make check`·`make test` 통과.
