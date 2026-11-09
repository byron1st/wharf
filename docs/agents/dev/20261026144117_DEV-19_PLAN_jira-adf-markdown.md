---
Application: wharf
JiraTicket: DEV-19
PlanType: single-step
Timestamp: 20261026144117
Title: jira-adf-markdown
---

# Jira sync 시 task 본문(ADF)을 Markdown 으로 변환해 저장

Report: [20261026144117_DEV-19_IMPL_jira-adf-markdown.md](./20261026144117_DEV-19_IMPL_jira-adf-markdown.md)

출처: Notion 개발 이슈 보드 DEV-19 "Jira sync API 에서 Jira task 본문을 Markdown 으로 변환해서 DB 에 저장하기". 브랜치 `feat/dev-19-jira-markdown` (base `c1a793d5e`). 사용자 지시에 따라 계획은 Claude 가 자체 승인했고, 사용자 판단이 필요한 결정은 아래 Key decisions 에서 Claude 가 정했다(PR 본문에 같은 목록을 남긴다).

이슈 요구사항 요약: `GET /tasks` 로 읽어 갈 때 본문이 일관되게 Markdown 이도록, `POST /tasks/jira` 동기화가 Jira description(ADF)을 Markdown 으로 바꿔 저장한다. Markdown 에 대응이 없는 요소는 대략 가까운 Markdown 으로 바꾼다.

## Non-goals

- local task 의 description 변환·검증. 사용자가 보낸 문자열을 그대로 저장한다(이미 Markdown 이라고 본다).
- Markdown → ADF 역변환, Jira 로 되돌려 쓰기.
- 기존 DB 행 마이그레이션. `UpsertTask` 가 description 을 덮어쓰므로 다음 동기화에서 갱신된다.
- `GET /tasks` 목록의 100 rune 절단(`toSummary`)을 Markdown 구문 단위로 맞추기.
- 첨부(media) 의 URL 해석. ADF media 노드에는 다운로드 URL 이 없다.
- acli 가 요청하는 필드(`viewFields`) 변경, 서드파티 Markdown/ADF 라이브러리 도입.
- 이 이슈 외의 task 필드(title 등) 변환.

## Key decisions

- **위치** — 변환은 `internal/adapter/atlassian` 안에서 한다. ADF 는 Jira 고유 형식이므로 adapter 가 도메인 문자열(Markdown)로 바꿔 내보내고, service·db·server 는 바뀌지 않는다. 기존 `adfNode.plainText` 를 Markdown 렌더러로 대체하고(평문 경로는 남기지 않는다), `jira.go` 의 `// TODO: Convert ADF to Markdown` 주석을 지운다. 렌더러를 별도 파일(`adf.go` 등)로 뺄지는 재량. 표준 라이브러리만 쓴다.
- **에러 없음** — 변환은 실패하지 않는다. 모르는 노드는 자식을 렌더링해 텍스트를 보존하고(기존 동작 유지), 필요한 attrs 가 없으면 텍스트만 남긴다. `ToTask` 의 에러 계약은 바뀌지 않는다.
- **출력 형식 (CommonMark + GFM 확장)**
  - 블록은 빈 줄 하나로 구분하고, 결과 앞뒤 공백은 잘라 낸다. 빈 paragraph·내용 없는 블록은 출력하지 않는다(연속 빈 줄 없음). null/빈 문서는 `""`.
  - `heading` → `#`×level(1–6, 범위 밖이면 가까운 값으로 고정).
  - text marks: `strong` → `**`, `em` → `*`, `strike` → `~~`, `code` → `` ` ``, `link` → `[text](href)`. `underline`·`textColor`·`subsup`·`backgroundColor` 등 대응 없는 mark 는 텍스트만 남긴다. 여러 mark 가 겹칠 때의 중첩 순서는 재량이되 결과가 유효한 Markdown 이어야 한다.
  - `hardBreak` → CommonMark hard line break(재량: 줄 끝 `\` 또는 공백 두 칸).
  - `bulletList` → `- `, `orderedList` → `1. ` 부터(attrs.order 가 있으면 그 번호부터) 증가, `taskList` → `- [ ] ` / `- [x] `(taskItem attrs.state `DONE`), `decisionList` → `- `. 중첩 목록은 들여쓰기로 표현하고, 한 list item 안의 여러 블록은 item 내용 들여쓰기에 맞춘다. 목록 항목 사이에는 빈 줄을 넣지 않는다(tight list).
  - `codeBlock` → fenced code block(` ``` `, attrs.language 가 있으면 info string). 내용에 backtick 연속이 있으면 더 긴 fence 를 쓴다. 내용은 그대로(마크 없음).
  - `blockquote` → 각 줄 앞 `> `. `panel` → blockquote 로(panelType 라벨은 재량, 넣는다면 `**Info:**` 류).
  - `rule` → `---`.
  - `table` → GFM 표. 첫 행을 헤더로 쓰고(헤더 행이 없어도 첫 행을 헤더로), 셀 안의 블록은 한 줄 inline 으로 펴며(블록 간 `<br>` 또는 공백, 재량) 셀 텍스트의 `|` 는 `\|` 로 이스케이프한다. 열 수가 행마다 다르면 최대 열 수에 맞춰 빈 셀을 채운다.
  - `expand`/`nestedExpand` → attrs.title 이 있으면 굵은 제목 줄(`**title**`) 다음에 내용 블록.
  - inline 특수 노드: `mention` → attrs.text, `emoji` → attrs.text(없으면 attrs.shortName), `status` → attrs.text(대응 없음; 표현은 재량, 예: `` `TEXT` `` 또는 `[TEXT]`), `date` → attrs.timestamp(epoch ms)를 UTC `YYYY-MM-DD` 로(파싱 실패 시 생략), `inlineCard` → attrs.url 을 autolink `<url>`. `blockCard`/`embedCard` → attrs.url 을 autolink 한 단독 블록.
  - `mediaSingle`/`mediaGroup`/`media` → 출력 없음(현재와 같음). 대체 텍스트(attrs.alt)를 남길지는 재량.
- **텍스트 이스케이프는 하지 않는다** — 일반 text 노드의 `*`, `_`, `#`, `1.` 같은 문자를 백슬래시 이스케이프하지 않는다. Jira 본문 원문 가독성을 우선하고, 원문 문자가 Markdown 으로 해석될 수 있는 것은 수용한 위험이다(표 셀의 `|` 만 예외). inline code 안의 backtick 처리(더 긴 delimiter)는 재량.

## Acceptance Contract

| ID | Observable condition | Evidence |
| --- | --- | --- |
| AC-1 | `ToTask` 가 ADF description 을 Key decisions 의 출력 형식대로 Markdown 으로 반환한다: heading 레벨, strong/em/strike/code/link mark, 대응 없는 mark 의 텍스트 보존, hardBreak, bullet/ordered(시작 번호 포함)/task(체크 상태)/중첩 목록, fenced code(언어 포함), blockquote/panel, rule, GFM 표(`|` 이스케이프), expand 제목, mention/emoji/status/date/inlineCard/blockCard, media 생략, 모르는 노드의 텍스트 보존, 블록 사이 빈 줄 하나·앞뒤 공백 없음. null·빈 문서는 `""`. | `TestToTaskDescription` 의 노드별 표 테스트 |
| AC-2 | `GetMyTasks` 가 `testdata/view_in_progress.json` 의 description 을 Markdown 으로 돌려준다(heading·mark·목록·task 체크박스·mention·inlineCard·hardBreak·code fence·rule 이 기대 문자열에 드러남). 다른 필드와 다른 fixture 결과는 바뀌지 않는다. | `client_test.go` 의 `GetMyTasks` 기대값 |
| AC-3 | `jira.go` 의 `// TODO: Convert ADF to Markdown` 주석과 평문 렌더러(`plainText`/`writeText`)가 남아 있지 않고, service·db·server 코드는 변경되지 않는다. 새 서드파티 의존성이 없다(`go.mod` 변경 없음). | diff |
| AC-4 | research `jira-adf-description` 이 구현 후 상태(Markdown 렌더러와 지원 노드)로 갱신되어 있다. | research 파일 |

검증하지 않는 위험: 원문 텍스트의 Markdown 특수문자 미이스케이프로 인한 의도치 않은 서식, 실제 Jira 에서 오는 드문 노드(`extension`, `bodiedExtension`, `layoutSection` 등)의 정확한 표현(텍스트 보존만 보장), `toSummary` 절단이 Markdown 구문 중간을 자르는 것, 렌더 결과를 실제 Markdown 렌더러로 확인하는 것(문자열 비교로만 검증).

## 참고 Research

- [jira-adf-description](../research/jira-adf-description.md) — ADF description 이 acli 출력에서 `task.Task.Description`·DB·API 응답까지 가는 경로, `adfNode` 가 현재 디코딩하는 필드와 평문 렌더러 동작, ADF 노드·mark·attrs 목록, 관련 테스트·fixture 위치.

## Authority Boundaries

- Discretion: 렌더러의 파일 배치·타입·함수 이름과 구조, `adfNode` 에 추가할 필드 모양(marks·attrs), Key decisions 에서 "재량" 으로 표시한 표현 선택(hardBreak 형식, status 표현, panel 라벨, 표 셀 블록 구분, mark 중첩 순서, media alt), 테스트 구성과 fixture 추가 여부.
- Must-ask: Key decisions·Non-goals 변경(변환 위치, 텍스트 이스케이프 정책, 블록 구분 규칙, 지원 노드 표현의 큰 변경), local task description 처리 변경, 서드파티 의존성 추가, DB 스키마 변경. (이번 실행에서는 사용자 지시에 따라 Claude 가 결정하고 PR 에 기록한다.)
- Stop conditions: Markdown 변환이 `ToTask` 의 에러 계약이나 다른 필드 동작을 바꿔야만 가능할 때, service·db·server 코드를 고쳐야 할 필요가 생길 때.
- Loop budget: 3

## TODOs

- [x] `adfNode` 가 marks 와 필요한 attrs(level, order, state, language, href, title, timestamp, shortName, url, text 등)를 디코딩하게 하고, 평문 렌더러를 Key decisions 의 출력 형식을 따르는 Markdown 렌더러로 대체한 뒤 `ToTask` 가 그것을 쓰게 한다. `jira.go` 의 TODO 주석을 지운다 (AC-1, AC-3) (design-bearing) (→ research: jira-adf-description)
- [x] `TestToTaskDescription` 을 Markdown 기대값으로 바꾸고 AC-1 의 노드·mark 별 케이스를 추가하며, `client_test.go` 의 `view_in_progress.json` 기대 description 을 갱신한다 (AC-1, AC-2) (mechanical) (→ research: jira-adf-description)
- [x] research `jira-adf-description` 과 index 를 구현 후 상태로 갱신 (AC-4) (mechanical)
