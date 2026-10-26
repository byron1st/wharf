---
Application: wharf
ResearchType: Flow
Description: Jira work item 의 description(ADF, Atlassian Document Format)이 `acli jira workitem view --json` 출력에서 `atlassian.ViewWorkItem` 으로 디코딩되어 `adfNode.markdown` 렌더러를 거쳐 Markdown 문자열 `task.Task.Description` 이 되고, `SyncJiraTasks` → `db.Manager.UpsertTasks` 로 `tasks.description` 에 덮어써진 뒤 `GET /tasks`·`GET /tasks/{id}` 응답(목록은 100 rune 절단)으로 나가기까지의 경로, `adfNode` 구조체가 디코딩하는 필드, ADF 노드·mark 별 Markdown 출력 규칙, 관련 테스트(fixture 포함) 위치를 정리한다.
---

# Jira description(ADF) → task.Description 경로

## 흐름

1. `client.GetMyTasks` (`internal/adapter/atlassian/client.go:37`) 가 검색으로 key 를 모은 뒤 key 마다 `viewWorkItem` (`client.go:94`) 으로 `acli jira workitem view {key} --fields ... --json` 을 실행하고 `ViewWorkItem` 으로 `json.Unmarshal` 한다.
2. `ViewWorkItem.Fields.Description` 은 `*adfNode` (`internal/adapter/atlassian/jira.go:26`, 타입 정의는 `internal/adapter/atlassian/adf.go:14`) 이다. description 이 없으면 JSON `null` → nil.
3. `ToTask` (`jira.go:56`) 가 `Description: f.Description.markdown()` (`jira.go:101`) 로 Markdown 문자열을 만든다. 변환은 에러를 내지 않는다(ToTask 의 에러는 status·날짜만).
4. `service.SyncJiraTasks` 가 tasks 를 `db.Manager.UpsertTasks` 로 저장한다. `UpsertTask` 쿼리 (`scripts/queries.sql:1`) 는 `ON CONFLICT (id) DO UPDATE SET description = excluded.description` 이므로 다음 동기화 때 기존 행의 description 이 새 변환 결과로 덮어써진다(마이그레이션 불필요).
5. 응답: `GET /tasks` 의 `toSummary` (`internal/server/route.go:214`) 는 description 을 100 rune 에서 자르고 `…` 를 붙인다. `GET /tasks/{id}` 는 전체를 준다. local task 의 description 은 사용자가 보낸 문자열 그대로 저장된다(변환 대상 아님).

## ADF → Markdown 렌더러 (`internal/adapter/atlassian/adf.go`)

- 표준 라이브러리만 쓰고, 출력은 CommonMark + GFM(표·task list) 이다. 평문 렌더러(`plainText`/`writeText`)와 `jira.go` 의 TODO 주석은 없어졌다.
- `adfNode` (`adf.go:14`) 가 디코딩하는 필드: `type`, `text`, `marks`(`adfMark`: `type`, `attrs.href`), `content`, attrs 의 `text`·`url`·`level`·`order`(`*int`, 없으면 1부터)·`state`·`language`·`title`·`shortName`·`timestamp`. `timestamp` 는 `json.RawMessage` 로 받아 문자열·숫자 모두 허용하고, 파싱 실패 시 date 만 생략한다(디코딩 실패로 work item 전체가 실패하지 않게). `level`·`order` 는 `int` 라서 숫자가 아니면 `json.Unmarshal` 이 실패한다(ADF 스키마상 숫자).
- 구조: `markdown` (`adf.go:56`) → `renderBlocks` (`adf.go:68`, 연속 inline 노드를 한 paragraph 로 묶고 빈 블록을 버림) → `renderBlock` (`adf.go:134`) / `renderInline` (`adf.go:190`). 블록은 `joinBlocks` (`adf.go:116`) 가 빈 줄 하나로 잇고, list item 안에서는 하위 목록만 바로 다음 줄에 붙여 tight list 를 유지한다. 결과 전체는 `strings.TrimSpace`.
- 블록: `heading` → `#`×level(1–6 으로 고정, 없으면 1), 목록(`renderList`, `adf.go:339`) — `bulletList`/`decisionList` `- `, `orderedList` `N. `(attrs.order 부터), `taskList` `- [x] `/`- [ ] `(state `DONE`), 이어지는 줄과 하위 목록은 marker 폭만큼 들여쓰고 taskList 안에 바로 든 taskList 는 이전 항목 아래로 들여쓴다. 같은 계열(bullet 계열 = bulletList/taskList/decisionList, 또는 orderedList)의 목록이 형제 블록으로 바로 이어지면(`renderBlocks`, `adf.go:68`) 두 번째 목록은 대체 marker(`* ` / `N) `)를 쓰고 그 다음은 다시 기본 marker 로 번갈아, CommonMark 가 별개 목록으로 파싱한다. 사이에 다른 블록이 있으면 기본 marker 를 유지한다. `codeBlock` → backtick fence(내용의 최장 backtick 연속보다 길게, 최소 3, info string = language, 빈 내용이면 생략). `blockquote`·`panel` → 줄마다 `> `(panelType 라벨 없음). `rule` → `---`. `table` → GFM 표(셀 없는 행은 버리고 남은 첫 행이 헤더, 셀 블록·hardBreak 는 `<br>`, 나머지 개행은 공백, `|` → `\|`, 최대 열 수로 빈 셀 채움). `expand`/`nestedExpand` → `**title**` 줄 + 내용. `blockCard`/`embedCard` → `<url>` 단독 블록. `mediaSingle`/`mediaGroup`/`media` → 출력 없음(alt 도 버림).
- inline: `text` 는 이스케이프하지 않는다. 연속된 `text` 노드는 mark 집합(순서 무관, link 는 href 까지)이 같으면 `mergeText` (`adf.go:235`) 가 하나로 합쳐 `**a****b**` 대신 `**ab**` 로 렌더링한다. marks 는 `applyMarks` (`adf.go:271`) 가 link > strike(`~~`) > strong(`**`) > em(`*`) > code 순으로 감싸고, 앞뒤 공백은 delimiter 밖에 둔다. code 는 내부 backtick 보다 긴 delimiter. `underline`·`textColor`·`subsup` 등과 href 없는 link 는 텍스트만. `hardBreak` → 줄 끝 `\` + 개행(paragraph 끝의 hardBreak 는 버림). `mention` → attrs.text, `emoji` → attrs.text 또는 shortName, `status` → `[TEXT]`, `date` → UTC `YYYY-MM-DD`, `inlineCard` → `<url>`.
- 모르는 노드: 블록 위치면 자식 블록을, inline 위치면 `text` 와 자식 inline 을 렌더링해 텍스트를 보존한다(`extension`·`layoutSection` 등).
- 알려진 한계: list item 안에 paragraph 가 둘 이상이거나 code block 이 있으면 그 목록은 loose 가 된다.

## 테스트 위치

- `internal/adapter/atlassian/jira_test.go:204` `TestToTaskDescription` — `descriptionItem` 헬퍼(`jira_test.go:24`)로 ADF 조각을 감싸 `ToTask` 의 `Description` 을 비교하는 표 테스트. 노드·mark 별 Markdown 기대값(heading·mark 중첩·목록·code fence·quote·rule·표·expand·inline 노드·card·모르는 노드·빈 블록)을 담는다.
- `internal/adapter/atlassian/client_test.go:88` — `testdata/view_in_progress.json` 의 description(heading, code·strong mark, bulletList, taskList, mention, inlineCard, hardBreak, codeBlock, rule, mediaSingle, 빈 paragraph)을 변환한 Markdown 기대 문자열. `view_done.json`·`view_canceled.json` 은 단일 paragraph.
- 변환기는 unexported 이고 테스트는 외부 `atlassian_test` 패키지에서 `ToTask` 를 통해서만 검증한다(AGENTS.md 테스트 관례).
