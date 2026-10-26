---
Application: wharf
ResearchType: Flow
Description: Jira work item 의 description(ADF, Atlassian Document Format)이 `acli jira workitem view --json` 출력에서 `atlassian.ViewWorkItem` 으로 디코딩되어 `adfNode` 렌더러를 거쳐 `task.Task.Description` 문자열이 되고, `SyncJiraTasks` → `db.Manager.UpsertTasks` 로 `tasks.description` 에 덮어써진 뒤 `GET /tasks`·`GET /tasks/{id}` 응답(목록은 100 rune 절단)으로 나가기까지의 경로, `adfNode` 구조체가 디코딩하는 필드, 지원·미지원 ADF 노드, 관련 테스트(fixture 포함) 위치를 정리한다.
---

# Jira description(ADF) → task.Description 경로

## 흐름

1. `client.GetMyTasks` (`internal/adapter/atlassian/client.go:37`) 가 검색으로 key 를 모은 뒤 key 마다 `viewWorkItem` (`client.go:94`) 으로 `acli jira workitem view {key} --fields ... --json` 을 실행하고 `ViewWorkItem` 으로 `json.Unmarshal` 한다.
2. `ViewWorkItem.Fields.Description` 은 `*adfNode` (`internal/adapter/atlassian/jira.go:27`) 이다. description 이 없으면 JSON `null` → nil.
3. `ToTask` (`jira.go:56`) 가 `Description: f.Description.plainText()` (`jira.go:102`) 로 문자열을 만든다. 변환은 에러를 내지 않는다(ToTask 의 에러는 status·날짜만).
4. `service.SyncJiraTasks` 가 tasks 를 `db.Manager.UpsertTasks` 로 저장한다. `UpsertTask` 쿼리 (`scripts/queries.sql:1`) 는 `ON CONFLICT (id) DO UPDATE SET description = excluded.description` 이므로 다음 동기화 때 기존 행의 description 이 새 변환 결과로 덮어써진다(마이그레이션 불필요).
5. 응답: `GET /tasks` 의 `toSummary` (`internal/server/route.go:214`) 는 description 을 100 rune 에서 자르고 `…` 를 붙인다. `GET /tasks/{id}` 는 전체를 준다. local task 의 description 은 사용자가 보낸 문자열 그대로 저장된다(변환 대상 아님).

## 현재 ADF 처리 (`jira.go:199` 이하)

- `jira.go:199` 에 `// TODO: Convert ADF to Markdown` 주석이 있다(date·status·emoji 노드 렌더, 목록·task 마커 유지가 남은 일로 적혀 있다).
- `adfNode` (`jira.go:208`) 가 디코딩하는 필드는 `type`, `text`, `attrs.text`, `attrs.url`, `content` 뿐이다. `marks`(strong·em·code·link 의 `attrs.href` 등)와 `attrs.level`·`attrs.state`·`attrs.order`·`attrs.language`·`attrs.timestamp`·`attrs.shortName` 은 디코딩하지 않는다. 필요한 필드는 구조체에 추가해야 한다.
- `plainText`/`writeText` (`jira.go:219`, `jira.go:230`): `text` 는 그대로, `hardBreak` 는 `\n`, `mention` 은 `attrs.text`, `inlineCard` 는 `attrs.url`. 자식을 재귀 방문한 뒤 `paragraph`·`heading`·`codeBlock`·`taskItem` 끝에 `\n`. 마지막에 `strings.TrimSpace`. 즉 marks·목록 마커·heading 레벨·code fence·표 구조는 모두 버려지고, 모르는 노드는 자식 텍스트만 남는다. `rule`·`media` 는 텍스트가 없다.
- ADF 노드 종류 참고(공식 문서 기준): block — `doc`, `paragraph`, `heading`(attrs.level 1–6), `bulletList`/`orderedList`(attrs.order)/`listItem`, `taskList`/`taskItem`(attrs.state `TODO`|`DONE`), `decisionList`/`decisionItem`, `codeBlock`(attrs.language), `blockquote`, `panel`(attrs.panelType), `rule`, `table`/`tableRow`/`tableHeader`/`tableCell`, `expand`/`nestedExpand`(attrs.title), `mediaSingle`/`mediaGroup`/`media`, `blockCard`/`embedCard`(attrs.url). inline — `text`(marks: `strong`, `em`, `strike`, `code`, `link`(attrs.href), `underline`, `textColor`, `subsup` 등), `hardBreak`, `mention`(attrs.text), `emoji`(attrs.shortName, attrs.text), `date`(attrs.timestamp, epoch ms 문자열), `status`(attrs.text), `inlineCard`(attrs.url). `taskItem` 은 paragraph 없이 inline 을 바로 담는다.

## 테스트 위치

- `internal/adapter/atlassian/jira_test.go:204` `TestToTaskDescription` — `descriptionItem` 헬퍼(`jira_test.go:24`)로 ADF 조각을 감싸 `ToTask` 의 `Description` 을 비교하는 표 테스트. 현재 기대값은 평문이다.
- `internal/adapter/atlassian/client_test.go:88` — `testdata/view_in_progress.json` 의 description(heading, code·strong mark, bulletList, taskList, mention, inlineCard, hardBreak, codeBlock, rule, mediaSingle, 빈 paragraph)을 변환한 기대 문자열. `view_done.json`·`view_canceled.json` 은 단일 paragraph.
- 변환기는 unexported 이고 테스트는 외부 `atlassian_test` 패키지에서 `ToTask` 를 통해서만 검증한다(AGENTS.md 테스트 관례).
