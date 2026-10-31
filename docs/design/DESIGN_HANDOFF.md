# Wharf Web App — Design Handoff

Wharf SPA 웹앱 디자인을 구현용으로 정리한 문서야. 시각적인 기준은 `screens/`의 스크린샷이고, 상호작용과 데이터 규칙은 이 문서가 기준이야.

- **원본 디자인:** claude.ai Design 캔버스 "Wharf Web App" (`boards/`에 원본 파일 포함)
- **디자인 기준 시각:** 2026-10-26 (월) 14:07 — 화면의 "Today", "Sent", "N days late" 같은 표시는 모두 이 시각 기준이야.
- **스타일:** shadcn/ui New York · Tailwind · zinc 중립색 + teal primary · 라이트 테마만 디자인됨
- **폰트:** Geist, Geist Mono(키·ID·시간·경로), Noto Sans KR
- **아이콘:** lucide 계열 (shadcn 기본)

## 0. 패키지 구성

| 경로 | 내용 |
|---|---|
| `DESIGN_HANDOFF.md` | 이 문서 |
| `tokens.css` | 색·그림자·폰트 토큰 (shadcn CSS 변수 형태) |
| `screens/*.png` | 보드별·상태별 스크린샷 (1440px 폭) |
| `boards/*.dc.html`, `boards/canvas.json` | 디자인 캔버스 원본. 캔버스 전용 템플릿 형식(`{{hole}}`, `sc-for`, `sc-if`, `class Component extends DCLogic`)이라 그대로 실행되지 않아. 마크업 구조, 정확한 스타일 값, 상태 로직을 확인할 때 참고용으로만 읽어. |

### 보드 파일 대응

| 보드 | 화면 |
|---|---|
| `TodayFinal.dc.html` | Today 메인 화면 전체 (모든 Today 변형의 단일 원본). props: `initialView`, `initialTask`, `initialReq` |
| `TodayD` / `TodayTask` / `TodayReq` | TodayFinal을 각각 선택 없음 / Task 선택 / Request 선택 상태로 띄운 래퍼 |
| `TaskLocal.dc.html` | Local Task 상세 모달 (prop `initialMode: 'edit'`) · `TaskLocalEdit`은 편집 모드 래퍼 |
| `TaskJira.dc.html` | Jira Task 상세 모달 (읽기 전용, … 메뉴 열린 상태) |
| `Settings.dc.html` | 설정 모달 (prop `initialTab: 'general' \| 'atlassian'`) · `SettingsAtlassian`은 래퍼 |
| `States.dc.html` | 날짜 피커 · 리마인더 피커 명세 + 토큰 |
| `Toasts.dc.html` | 토스트 전체 명세 |

## 1. 앱 구조

뷰는 **Today 하나**뿐이야. 사이드바, 탭, 별도 Tasks/Board 뷰는 없어. 나머지는 모두 Today 위에 뜨는 모달이야.

```
Today (full width)
├─ Header: "Wharf" · [⟳ Synced 14:02] [+ New task] · 날짜 · 현재 시각(HH:MM) · ⚙ Settings
├─ Left  (flex 999 1 600px): Todos card
└─ Right (flex 1 1 340px): Requests card (접기/펼치기) → Inspector (선택 시에만)
Modals
├─ Task detail — Local (view / edit)
├─ Task detail — Jira (read-only)
└─ Settings (General / Atlassian Cloud)
```

URL 상태 제안 (디자인에는 없음, 구현 판단): `?todo=22`, `?task=WHF-003`, `?req=CIS-12345`(Inspector), `/task/WHF-003`(모달), `/settings/general|atlassian`. 새로고침과 뒤로가기가 자연스러워져.

## 2. Today

### Header
- 왼쪽: `Wharf` (24px / 600 / tracking -0.02em). 로고 아이콘 없음.
- 오른쪽: Sync 버튼 · New task 버튼 │ `Monday, October 26`(orange `#c2410c`, 13px/500) │ `14:07`(15px/600, tabular) │ Settings 아이콘 버튼(36×36, `⌘,`).
- **Sync 버튼 (Atlassian Cloud 동기화):** 32px 높이의 pill 모양 outline 버튼이야. 마지막 동기화 시각을 보여주는 상태 표시이자 실행 버튼이야.
  - 대기: `⟳ Synced 14:02` (gray). 누르면 동기화 시작.
  - 진행 중: 스피너 + `Syncing…` (teal 배경·테두리), 비활성. 오른쪽 아래에 로딩 토스트가 떠.
  - 완료: 로딩 토스트가 결과 토스트로 바뀌고(§6), 버튼은 새 시각(`Synced 14:07`)으로 돌아와.
  - 실패: `Sync failed` (red 배경·테두리). 다음 동기화가 성공할 때까지 유지되고, 누르면 다시 시도해.
  - Settings › Sync의 `Sync now`와 같은 동작이야.
- **New task 버튼 (로컬 Task 추가):** Sync 버튼 바로 오른쪽, 같은 pill 스타일(`+ New task`, 단축키 `N`). Wharf에 Task가 들어오는 두 경로(Jira 동기화, 로컬 추가)를 헤더 한 곳에 나란히 둔 거야.
  - 누르면 버튼이 teal로 활성화되고, 아래에 팝오버(폭 340px, 오른쪽 정렬)가 열려: `New task` + `Local` 배지, 제목 입력(포커스 상태), 안내 "Opens in the inspector so you can add a due date, description and todos next.", `Enter` create · `Esc` cancel, `Cancel` / `Create`(제목이 비면 비활성).
  - 만들면: 새 Task 그룹(예: `WHF-009`, `No due date`, `0/0`)이 Todo 목록에 추가되고 그 그룹의 Todo 입력 행이 바로 열려. 동시에 Inspector에 새 Task가 열려.
  - Todo가 아직 없는 새 Task도 Today에 보여 (만든 직후 세션 동안). ID는 저장할 때 서버가 배정해.
- 시각은 1분마다 갱신.

### Todos card (`screens/01`, `02`)
- **컬럼:** `Todo` / `Reminder` / `When`.
- **Task 그룹 행** (배경 `#fafafa`):
  - 접기 chevron, origin 점(Jira 파랑 / Local teal), 키(mono), 제목
  - 제목을 누르면 Inspector에 Task가 열려. 선택되면 행이 teal 링으로 표시되고 제목에 밑줄.
  - 마감 태그: `Due today`(orange) / `Overdue`(red) / `Due Oct 28`(gray). Task의 due로 계산.
  - 진행률: violet 바 + `2/5`
  - `+` 버튼: 그룹 바로 아래에 인라인 입력 행 `New todo for WHF-003`이 열려 (`Enter` 추가 · `Esc` 취소). 열려 있는 동안 `+`는 teal로 채워져.
- **Todo 행:**
  - 체크박스, 텍스트(누르면 Inspector에 Todo)
  - Note가 있으면 문서 아이콘, 연결된 Request가 있으면 티켓 아이콘 + 개수(indigo pill)
  - Reminder 열: 다음 리마인더 `Today 16:30` / `Tue 10:00 +1`(violet). 남은 게 없고 이미 울린 것만 있으면 `Sent 11:00`(gray).
  - When pill: `N days late`(red) / `Due today`(orange) / `By Oct 28` / `Starts today` / `No date`(gray). Todo의 end(없으면 start)로 계산.
  - 선택된 행: 배경 `#f0fdfa` + inset 1px `#99f6e4` 링 (왼쪽 보더 강조는 쓰지 않음).
- **Today에 나오는 Todo:** 디자인 데이터에서는 미완료 Todo 중 오늘 할 수 있는 것(기한 지난 것 포함)만 보여. 정확한 선정 규칙은 도메인 스펙을 따라.
- **`Completed today` 섹션:** 접기 가능. 오늘 완료한 Todo가 취소선으로 표시되고, 오른쪽에 Task 키가 보여.

### Requests card (`screens/04`)
- **헤더 버튼:** `Requests 4 · ● 2 changed today` + chevron. 누르면 카드가 그 자리에서 펼쳐져 (팝오버 아님).
- **행:**
  - status 점, 제목(ellipsis), `CIS-12345` + 나이 표시. 오늘 status가 바뀌었으면 `changed today`(indigo/600), 아니면 `2d in status`.
  - 오른쪽에 status pill
- 행을 누르면 Request Inspector가 열려. 목록에는 Todo 연결을 표시하지 않아.
- Done / Cancel 상태인 요청은 Today에서 빠져.

### Inspector
오른쪽 열, Requests 카드 아래에 열려. 선택은 하나만 가능해 (Todo, Task, Request 중 하나를 고르면 나머지는 해제). 열린 피커도 같이 닫혀. `×`로 닫아.

**Todo** (`screens/02`, `05`, `06`):
- **헤더:** `Todo #22` · `✓ Saved` (자동 저장 표시)
- **기본 필드:**
  - Description: Todo 텍스트를 편집하는 textarea
  - Note: plain text, Wharf에만 저장
- **Task:** 소속 Task와 `Move` 링크 → 아래 "열린 이슈" 참고.
- **Start / End:** 각 필드를 누르면 날짜 피커가 열려 (§5). End가 지났으면 필드가 빨갛게 표시되고 `Ended N days ago — pick a new end date.`가 떠.
- **Reminders:**
  - 리마인더 칩(violet)을 누르면 수정 피커가 열리고, `×`로 삭제해.
  - 이미 울린 칩은 gray에 체크 아이콘과 `Sent`가 붙고, 수정할 수 없어 (삭제만 가능).
  - `Add reminder`는 리마인더 피커를 열어 (§5).
- **Service Desk requests:**
  - 연결된 요청 칩(키, 제목, status)과 `×`(연결 해제)
  - `Link a request`를 누르면 검색 피커가 열려.
- **푸터:** `Mark as done`(완료하면 `Completed — undo`로 바뀜) · 휴지통(삭제).

**Task** (`screens/03`, `07`):
- **헤더:** `Task` · origin 배지 · 키 · `Open ↗`(Task 상세 모달)
- **Local Task:**
  - Title을 편집할 수 있어.
  - Status / Start / Due: Start·Due는 날짜 피커를 열어.
  - Description: Markdown, `Preview` / `Write` 탭
  - Relations: Parent / Children / Linked, 각각 `+`
  - 푸터: ID · `Delete task`
- **Jira Task:**
  - 맨 위에 "Managed in Jira" 안내가 있고, 필드는 모두 읽기 전용이야 (화살표 없음, 클릭 불가).
  - Description은 Jira가 준 Markdown을 렌더링만 해.
- **Relation 칩:**
  - 실선: Wharf에 있는 Task. 누르면 열려.
  - 점선 + `Not in Wharf`: ID만 아는 Task. 클릭 불가.

**Request** (`screens/04`):
- **헤더:** `Request` · `Service Desk` 배지 · 키
- **읽기 전용 정보:**
  - 안내: "Synced from Service Desk with Jira sync. It leaves Today once it's Done or Canceled."
  - Title
  - Status: 한 줄 전체 폭
  - In status since / Created
  - `2 days in Security Review · Open for 5 days`
- **Linked todos:**
  - 각 행: 체크박스(완료 토글), 텍스트, Task 키·제목, `×`(연결 해제). 헤더에 `Saved only in Wharf` 표시.
  - `Link an existing todo`: `Search todos or task keys` 검색 목록. 미완료 Todo만 나오고, 오늘 Todo가 먼저 나오며 `TODAY` 표시가 붙어.

## 3. Task detail 모달

공통: Today 위 overlay `rgba(9,9,11,0.48)`, dialog max-width 960px, max-height `calc(100vh - 80px)`, 내부 스크롤, radius 14px. 왼쪽 열은 본문(Description, Todos), 오른쪽 열은 Details와 Relations.

### Local — view (`screens/08`)
- **헤더:** `Local` 배지 · 키 · 복사 · 제목(h1) · status 배지 · `Edit` · `…` · 닫기
- **본문:** Description은 렌더링된 Markdown만 보여주고, 편집 UI는 없어.
- **Details:** 텍스트로만 표시 (`Oct 30, 2026 · in 4 days`)

### Local — edit (`screens/09`)
- **헤더:** `Editing` 배지, 제목 입력칸, `Cancel` / `Save changes ⌘↵`
- **Description:** `Preview` / `Write` 탭. Write에서는 툴바(H, B, I, code, link, list, task list, code block), monospace textarea, 푸터 `Markdown **bold** · \`code\` · - [ ] task · \`\`\` block` · `Saved`.
- **Details:** Status는 select, Start·Due는 날짜 피커 필드.
- **Relations:** 추가와 수정은 편집 모드에서만 가능.

### Todos (view·edit 공통 — Todo는 모드와 상관없이 항상 편집 가능)
- **헤더:** `Todos 2 of 5` + 진행 바 · `Hide completed` 스위치
- **행:**
  - 드래그 핸들(순서 변경), 체크박스, 텍스트
  - 메타 칩: 기간, 리마인더(울린 건 gray + 체크), Note, Request 개수
  - `…` 버튼: 행 아래에 인라인 편집 폼을 열어 (`screens/10`)
- **인라인 편집 폼:**
  - Start date / End date (날짜 피커)
  - Reminders (칩 + `Add reminder`, 리마인더 피커)
  - Note (textarea)
  - Service Desk requests (칩 + `Link a request` + 검색 피커)
  - 푸터: `Delete` / `Cancel` / `Save`
  - Task 선택 필드는 없어.
- 마지막 행 아래에 `Add a todo — press Enter` 입력칸

### Jira (`screens/11`)
- **헤더:** `Jira` 배지 · 키 · `Read-only` 배지 · 제목 · status 배지 · `…` · 닫기
- **`…` 메뉴:** 항목은 딱 두 개야.
  - `Open in Jira` (`…/browse/SEC-142`, 새 브라우저 탭)
  - `Sync from Jira` (이 이슈 하나만 다시 가져옴 → §6 토스트)
- **본문:**
  - "Managed in Jira" 배너
  - Description: `Markdown from Jira`, 렌더링만
  - Todos: `Editable — stays in Wharf`. Local Todos와 똑같이 동작해.
- **Details / Relations:** 읽기 전용. Relations 아래에 "To group this issue with your own work, add it to a local task's children or links." 안내가 있어.

## 4. Settings 모달 (`screens/12–14`)

- **크기와 진입:** max-width 920px, height `min(800px, calc(100vh - 112px))`. 헤더의 ⚙(`⌘,`)로 열고 `Esc`로 닫아.
- **왼쪽 내비 (210px, `#fafafa`):**
  - `General`, `Atlassian Cloud`. 선택된 항목은 `#e3f4f1` 배경과 teal 글자.
  - 구분선 아래 `GitLab`, `Notion`, `Local Directories`: gray에 `Soon` 배지, 클릭 불가.
  - 맨 아래 `Wharf · [VERSION]`
- **General › App Server:**
  - **Config file:**
    - 경로 표시(mono, `#fafafa`, 파일 아이콘), `Choose…`(폴더 아이콘), `Open`(외부 링크 아이콘)
    - 안내: "A .toml file. Port and database below are read from it — edit the file to change them."
  - **`🔒 From config.toml · read-only` 목록:**
    - Port: `8080 · http://localhost:8080`
    - Database: 경로 + `SQLite` 태그
    - 둘 다 수정할 수 없어.
  - **config 파일을 바꿨을 때:** amber 배너 "The server is still using the previous file. Restart it to load the new config." + `Undo` / `Restart server`. config 경로 필드 테두리도 amber가 돼.
  - Save 버튼은 없어. 경로를 고르면 바로 반영돼.
- **Atlassian Cloud › Connection:**
  - URL (`https://[your-site].atlassian.net`), Email, API Key(password 입력 + 보기/숨기기 토글)
  - 연결 테스트 행: 성공하면 green 배경에 `Connected — Jira and Service Desk are reachable. Checked 14:01.`, 테스트 중이면 스피너 + `Testing connection…`
  - `Discard` / `Save`
  - 섹션은 카드로 감싸지 않아. Connection과 Sync 사이에 얇은 구분선만 있어.
- **Atlassian Cloud › Sync:**
  - Last sync 카드: `Today, 14:02` · `✓ Complete` · `6 issues · 4 open requests imported or updated` + `Sync now`
  - `What a sync does`:
    1. 나에게 할당된 미완료 Jira 이슈를 가져오거나 갱신
    2. Service Desk 요청 갱신. Done·Canceled는 Today에서 빠짐
    3. 읽기만 함. Wharf의 Todo·Note·링크는 Jira에 쓰지 않음
    4. Jira에서 해결됐거나 할당이 빠진 이슈는 다시 가져오지 않아서, 직접 지울 때까지 마지막 동기화 상태로 남음

## 5. 피커 (`screens/05–07`, `10`, `15`)

공통 팝오버: 폭 300px, padding 12, radius 10, `--shadow-popover`, 트리거 바로 아래 6px. 바깥을 누르거나 `Esc`를 누르면 닫혀. shadcn `Popover` + `Calendar`(react-day-picker)로 구현하면 돼.

### 날짜 피커 — Start / End / Due
- **열기와 저장:** 필드마다 팝오버가 하나씩 열려. 열려 있는 필드는 teal 테두리에 3px 링이 생겨. 날짜를 누르면 바로 저장되고 닫혀 (Save 버튼 없음).
- **구성:**
  1. 제목 (`End date` 등)
  2. 빠른 선택 칩: `Today` · `Tomorrow` · 이번 `Fri` · `Next Mon`. 현재 값과 같은 칩은 teal로 표시돼.
  3. 월 달력: 주는 월요일 시작, 이전·다음 달로 이동 가능
  4. 푸터: `Clear` + 짝 날짜 안내 (`Starts Mon, Oct 26` / `Due Fri, Oct 30`)
- **날짜 칸 상태:**
  - 선택: teal 채움
  - 짝 날짜: teal 테두리 (End를 고를 땐 Start, Start를 고를 땐 End·Due)
  - 두 날짜 사이: `#e6f4f1` 띠
  - 오늘: orange 테두리 `#fdba74`, 글자 `#c2410c`
  - 비활성: `#d4d4d8`
- **순서 강제:** End·Due는 Start보다 앞설 수 없고, Start는 End·Due보다 뒤일 수 없어. 그런 날짜는 비활성이라 에러 메시지가 필요 없어.
- **Clear:** 값이 없으면 비활성이야.

### 리마인더 피커
- **새로 추가 (`Add reminder`):**
  - 프리셋 메뉴를 한 번 누르면 바로 추가되고 닫혀.
    - `In 1 hour` (Today 15:07)
    - `This evening` (Today 18:00)
    - `Tomorrow morning` (Tue 09:00)
    - `Morning of end date` (Thu 29 · 09:00): Todo에 End가 있고 미래일 때만
  - 그 아래 `Pick a time`:
    - 월 달력: violet 선택, 지난 날짜는 비활성
    - `Time` 입력칸: HH:MM 숫자 입력, 24시간제, mono, 72px
    - 오른쪽에 요약 `Wed, Oct 28 · 10:00`
    - `Add reminder` 버튼
  - 기본값은 내일 09:00.
- **수정 (예정된 리마인더 칩 클릭):** 같은 폼(달력, Time)이 열리고, 푸터는 `Delete` · `Cancel` · `Save`.
- **검증 (버튼 비활성 + 빨간 테두리 + 메시지):**
  - 형식 오류: `Use 24-hour time, like 09:30.`
  - 이미 지난 시각: `That time has already passed. It's 14:07 now.`
- **울린 리마인더:** gray, 체크 아이콘, `Sent`. 수정할 수 없고 삭제만 가능해.
- **색:** 리마인더는 어디서나 violet이야 (선택된 날짜, 칩, 종 아이콘).

## 6. 토스트 (`screens/16`)

- **위치와 쌓임:** 화면 오른쪽 아래, 최신이 맨 앞. 최대 3개까지 겹쳐 쌓이고, 마우스를 올리면 펼쳐져 (Sonner 방식, shadcn `sonner` 사용).
- **수명:** success와 info는 4초 뒤 자동으로 닫혀. warning과 error는 직접 닫거나 Retry할 때까지 남아.
- **로딩 → 결과:** 로딩 토스트는 같은 자리에서 결과 토스트로 바뀌어 (`toast.promise` 또는 같은 id로 갱신).
- **카드:** 폭 최대 380px, radius 10. warn은 amber 배경, error는 red 배경. 액션 버튼 28px.

| 그룹 | 상태 | 제목 / 설명 | 액션 |
|---|---|---|---|
| Atlassian Cloud sync (헤더 ⟳ · Settings › Sync now) | 진행 중 | Syncing Atlassian Cloud… / Fetching Jira issues and Service Desk requests. (아래에 진행 바) | — |
| | 성공 | Atlassian Cloud synced `14:02` / 6 issues updated · 4 open requests · 2 requests closed | — |
| | 부분 성공 | Partly synced / 5 of 6 issues updated. SEC-171 couldn't be read. | Sync again |
| | 인증 실패 | Atlassian Cloud sync failed / 401 Unauthorized. Check the email and API key. Nothing was changed. | Open settings · Retry |
| | 네트워크 실패 | Can't reach Atlassian Cloud / [your-site].atlassian.net didn't respond. Nothing was changed. | Retry |
| Single issue (Jira 모달 … › Sync from Jira) | 진행 중 | Syncing SEC-142… | — |
| | 성공 | SEC-142 synced / Status changed to In Progress. Description updated. | — |
| | 변경 없음 | SEC-142 is up to date | — |
| | 찾을 수 없음 | SEC-142 wasn't found in Jira / … Wharf keeps the last synced copy. | Open in Jira |
| API errors | 서버 응답 없음 | Can't reach the Wharf server / localhost:8080 isn't responding… (서버가 응답하면 자동으로 닫힘) | Retry |
| | 저장 실패 | Couldn't save the todo / Your change was undone. 500 … (낙관적 업데이트 되돌림) | Retry |
| | 항목 없음 | That task no longer exists / It was removed elsewhere. The list has been refreshed. | — |

## 7. shadcn 컴포넌트 대응

| UI | shadcn / 라이브러리 |
|---|---|
| 카드, 배지, 버튼, 입력, textarea, switch, separator | `Card`, `Badge`, `Button`(default / outline / ghost / destructive), `Input`, `Textarea`, `Switch`, `Separator` |
| Task 상세, Settings | `Dialog` (Settings 내비는 직접 구현하거나 세로 `Tabs`) |
| 날짜·리마인더 피커 | `Popover` + `Calendar` (`weekStartsOn: 1`) |
| Jira `…` 메뉴, Status select | `DropdownMenu`, `Select` |
| Request·Todo 검색 피커 | `Command`(cmdk) 인라인 |
| Markdown Preview/Write | `Tabs` + `react-markdown` (GFM task list) |
| Todo 순서 변경 | `@dnd-kit/sortable` |
| 토스트 | `sonner` |
| 아이콘 | `lucide-react` |

## 8. 데이터와 백엔드 함의

디자인을 진행하면서 생긴 요구사항이야. 기존 도메인 스펙에 없는 것 위주로 정리했어.

- **Todo**
  - `note` (plain text, Wharf 전용)
  - `start_date`, `end_date` (날짜만)
  - `reminders[]`: `remind_at`(로컬 datetime) + 발송 여부(`sent_at` 등) → `Sent` 표시와 수정 불가 판단에 필요
  - `position` (Task 안 정렬)
- **Service Desk request**
  - 필드: `{ issue_key, title, status, created_at, status_at }`. status는 Open / Pending / In Progress / Security Review / Done / Cancel.
  - Today에는 Done·Cancel이 아닌 것만 보여.
  - "changed today" = `status_at`이 오늘.
- **Request ↔ Todo 연결:** 다대다 연결 테이블. Wharf에만 저장하고 Jira에는 쓰지 않아.
- **Task relations:** parent / children / linked. Wharf에 없는 대상도 ID로 저장할 수 있어야 해 (`jira-SEC-110`처럼, `Not in Wharf`로 표시).
- **Jira**
  - "Open in Jira" 링크용 site URL (`{site}/browse/{key}`)
  - 단일 이슈 동기화 API
  - 전체 동기화 API: 결과 카운트(updated issues, open requests, closed requests, failures + 실패한 키)와 진행 상태
- **Atlassian 연결 설정:** URL, email, API token 저장. 연결 테스트 API는 Jira와 Service Desk에 접근 가능한지 확인해.
- **App Server 설정 API**
  - 현재 config 경로, port, DB 경로 조회
  - `Choose…`: 브라우저 `<input type=file>`로는 절대 경로를 얻을 수 없어서, Go 서버가 macOS 파일 선택창을 띄우고 경로를 돌려줘야 해 (예: `osascript -e 'POSIX path of (choose file of type {"toml"})'`).
  - `Open`: 서버가 `open <path>` 실행
  - `Restart server`
- **자동 저장:** Inspector는 `✓ Saved`를 보여주는 자동 저장이야. 낙관적으로 업데이트하고, 실패하면 되돌린 뒤 토스트를 띄워.

## 9. 열린 이슈와 임시 값

- **[결정 필요] Inspector의 Todo `Move` 링크:** "다른 Task로 옮기기" 다이얼로그는 필요 없다고 해서 지웠는데, Inspector의 `Move` 링크는 아직 남아 있어. 기능을 뺄지 정해야 해.
- **[결정 필요] Jira Task Inspector의 `Remove from Wharf` 버튼:** 확인 다이얼로그는 지웠는데 버튼은 남아 있어. 확인 없이 바로 지울지, 버튼을 없앨지 정해야 해.
- **임시 값:**
  - `[VERSION]`, `[your-site]`
  - 기본 config 경로 `~/.config/wharf/config.toml`
  - DB 경로 `~/Library/Application Support/Wharf/wharf.db`
  - 포트 `8080`
  - 토스트 예시의 에러 코드와 이슈 키
- **디자인 범위 밖:** 다크 테마, 좁은 화면(레이아웃은 flex-wrap으로 아래로 쌓이는 정도만 고려), 빈 상태(Todo가 하나도 없을 때)는 디자인하지 않았어.

## 10. 스크린샷 목록

| 파일 | 상태 |
|---|---|
| `01-today-nothing-selected` | 선택 없음, Requests 펼침 |
| `02-today-todo-selected` | Todo #22 Inspector (울린 리마인더 `Sent`) |
| `03-today-task-selected` | Local Task WHF-003 Inspector |
| `04-today-request-selected` | Request CIS-12345 Inspector + Todo 연결 피커 |
| `05-today-todo-end-date-picker` | Todo End 날짜 피커 |
| `06-today-todo-add-reminder` | 리마인더 추가 피커 |
| `07-today-task-due-picker` | Task Due 날짜 피커 |
| `08-task-local-view` | Local Task 모달 view |
| `09-task-local-edit` | Local Task 모달 edit |
| `10-task-local-todo-reminder-edit` | Todo 인라인 편집 + 리마인더 수정 피커 |
| `11-task-jira-with-menu` | Jira Task 모달 + … 메뉴 |
| `12-settings-general` | Settings › General |
| `13-settings-general-config-changed` | config 파일 변경 후 재시작 안내 |
| `14-settings-atlassian` | Settings › Atlassian Cloud |
| `15-pickers-sheet` | 피커 명세 시트 |
| `16-toasts-sheet` | 토스트 명세 시트 (헤더 Sync 버튼 상태 포함) |
| `17-today-sync-running` | 헤더 Sync 진행 중 + 로딩 토스트 |
| `18-today-new-local-task` | 헤더 New task 팝오버 |
| `19-today-local-task-created` | 새 Task 생성 직후 (Todo 입력 행 + Task Inspector) |
