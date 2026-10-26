# Wharf 도메인 개요 — Task 와 Todo (웹앱 디자인 입력용)

이 문서는 Wharf 웹앱(SPA)을 디자인하기 위해 백엔드가 다루는 핵심 개념인 **Task** 와 **Todo** 를 설명한다. 화면 구조나 시각 스타일은 정하지 않으며, 디자인이 반드시 지켜야 하는 데이터·규칙·제약만 담는다. 모든 내용은 현재 구현된 백엔드(Go HTTP API) 기준이다.

## 1. Wharf 는 무엇인가

- Wharf 는 한 사람이 자기 컴퓨터에서 실행하는 **개인용 작업 허브**다. 여러 곳에 흩어진 일(Jira 등)을 한 곳에 모으고, Wharf 안에서 직접 만든 일과 함께 관리한다.
- 로그인·권한·다중 사용자 개념은 없다. 사용자는 항상 한 명이다.
- 데이터는 로컬 SQLite 파일에 저장되고, 서버는 기본적으로 `:8080` 에서 JSON REST API 를 제공한다.
- 큰 단위의 일은 **Task**, Task 를 끝내기 위한 작은 할 일은 **Todo** 로 나눈다.

```text
Task (Jira / Local / …)        ← 일의 단위. 외부 시스템에서 가져오거나 직접 만든다.
 ├─ parent / children / linked  ← Task 끼리의 관계
 └─ Todo, Todo, Todo …          ← 그 Task 에 속한 순서 있는 체크리스트(Wharf 에만 존재)
     └─ reminders               ← Todo 별 알림 시각 목록
```

## 2. Task

### 2.1 출처(Origin)

모든 Task 는 출처(`originated`)를 가진다. 출처에 따라 Task 를 누가 소유하고 Wharf 에서 무엇을 할 수 있는지가 달라진다.

| Origin | 상태 | 생성 방법 | Wharf 에서 수정 | Wharf 에서 삭제 |
| --- | --- | --- | --- | --- |
| `jira` | 구현됨 | "Jira 동기화" 실행 시 가져옴 | 불가(읽기 전용, 409) | 가능(단, 아래 주의) |
| `local` | 구현됨 | Wharf 에서 직접 생성 | 가능 | 가능 |
| `notion` | 예정(값만 정의됨) | — | — | — |
| `gitlab` | 예정(값만 정의됨) | — | — | — |

- 디자인은 출처를 한눈에 구분할 수 있어야 하고(아이콘·배지 등), 앞으로 Notion·GitLab 출처가 추가될 수 있음을 고려해야 한다.
- 출처별로 "수정 가능 여부"가 다르므로, 편집 UI 는 `local` Task 에서만 활성화되어야 한다.

### 2.2 Task ID

- 형식은 `{origin}-{key}` 이다.
  - Jira: `jira-` + Jira 이슈 키. 예: `jira-PROJ-123`
  - Local: `local-WHF-` + 3자리 이상 순번. 예: `local-WHF-001`, `local-WHF-042`, `local-WHF-1000`
- 사용자가 ID 를 정하지 않는다. Local Task 는 생성 시 서버가 순번을 매긴다.
- 화면에서는 접두어를 뗀 키(`PROJ-123`, `WHF-001`)를 출처 배지와 함께 보여주는 방식도 가능하다.

### 2.3 Task 필드

| 필드 | 타입 | 설명 |
| --- | --- | --- |
| `id` | string | `{origin}-{key}` |
| `originated` | `jira` \| `local` \| `notion` \| `gitlab` | 출처 |
| `title` | string | 제목. 필수(공백만 있으면 안 됨) |
| `description` | string | 본문(일반 텍스트). Jira 의 서식 있는 본문은 줄바꿈만 남긴 평문으로 변환된다(목록 기호·날짜·이모지 등은 아직 빠진다) |
| `status` | `todo` \| `in-progress` \| `done` \| `canceled` | 상태. 생성 시 생략하면 `todo` |
| `start_date` | 날짜시각 \| null | 시작일(선택). RFC3339 문자열. Jira 는 날짜만 있어 `…T00:00:00Z` 로 온다 |
| `due_date` | 날짜시각 \| null | 마감일(선택). 형식은 `start_date` 와 같다 |
| `parent_id` | string \| null | 상위 Task ID(최대 1개) |
| `children_ids` | string[] | 하위 Task ID 목록 |
| `linked_ids` | string[] | 연결된 Task ID 목록(방향 없음) |

### 2.4 상태(Status)

| 값 | 의미 | Jira 에서의 매핑 |
| --- | --- | --- |
| `todo` | 시작 전 | 상태 카테고리 "To Do" |
| `in-progress` | 진행 중 | 상태 카테고리 "In Progress" |
| `done` | 완료 | 상태 카테고리 "Done" |
| `canceled` | 취소 | 상태 이름이 "Canceled" 인 경우 |

상태 4가지는 모든 출처에서 공통이다. 화면에서 상태별 색·아이콘·그룹(예: 칸반 열)을 일관되게 쓸 수 있다.

### 2.5 Task 관계

- **Parent / Children (계층)**: Task 는 상위 Task 를 최대 하나 가진다. 하위 Task 는 여러 개일 수 있다. Jira 의 Epic → Story → Sub-task 같은 구조가 이렇게 표현된다.
- **Linked (연결)**: 방향이 없는 "관련 Task" 관계다. A 가 B 와 연결되면 B 의 상세에도 A 가 보인다.
- **출처를 넘나드는 관계가 가능하다.** 예를 들어 Local Task 가 Jira Task 들을 children 으로 묶을 수 있고, 이 관계는 Jira 동기화 후에도 유지된다.
- **관계는 Wharf 에 저장되지 않은 Task 를 가리킬 수 있다.** 예: Jira 에서 내게 할당되지 않은 상위 Epic. 이 경우 API 는 ID 만 주고 제목·상태 같은 정보는 주지 않는다. 디자인은 "ID 만 알고 내용은 없는 Task" 를 표시할 방법이 필요하다.
- 계층 순환이나 모순은 서버가 막지 않는다. 자기 자신을 참조하는 것만 거부한다.

### 2.6 Jira 동기화

- 사용자가 명시적으로 실행하는 동작이다(`POST /tasks/jira`). 자동·주기 동기화는 없다.
- **내게 할당되어 있고 아직 완료되지 않은** Jira 이슈만 가져와 저장하거나 덮어쓴다.
- 결과로 저장한 개수(`upserted`)와 일부 이슈를 가져오거나 변환하지 못했는지 여부(`partial`)를 돌려준다. `partial: true` 면 "일부 실패" 를 사용자에게 알려야 한다. 전체 실패는 500 오류다.
- 동기화는 시간이 걸릴 수 있다(이슈마다 외부 CLI 를 호출함). 진행 중임을 보여주는 UI 가 필요하다.
- 주의할 동작:
  - Jira 에서 완료되었거나 내게서 할당 해제된 이슈는 다시 가져오지 않으므로, Wharf 에는 **마지막으로 동기화된 상태 그대로 남는다**. 오래된 Jira Task 를 구분하거나 정리하는 UX 를 고려할 수 있다.
  - Jira Task 를 Wharf 에서 삭제해도, 여전히 내게 할당된 미완료 이슈라면 다음 동기화 때 다시 생긴다.
  - Jira Task 는 Wharf 에서 내용을 수정할 수 없지만, 그 Task 에 **Todo 를 붙이는 것은 가능하다**.

## 3. Todo

### 3.1 개념

- Todo 는 하나의 Task 에 속한 **작은 체크리스트 항목**이다.
- Todo 는 반드시 정확히 하나의 Task 에 속하고, Task 는 여러 Todo 를 가질 수 있다. 출처와 관계없이 모든 Task(Jira 포함)가 Todo 를 가질 수 있다.
- Todo 끼리는 계층이 없다(평평한 목록). 대신 Task 안에서 **순서(position)** 가 있다.
- Todo 는 Wharf 에만 존재한다. Jira 등 외부로 다시 쓰지 않는다.
- Task 를 삭제하면 그 Task 의 Todo 도 함께 삭제된다. Jira 동기화로는 Todo 가 사라지지 않는다.

### 3.2 Todo 필드

| 필드 | 타입 | 설명 |
| --- | --- | --- |
| `id` | number | 서버가 매기는 정수 ID |
| `task_id` | string | 소속 Task ID. 필수이며 실제로 저장된 Task 여야 한다 |
| `description` | string | 할 일 내용. 필수(공백만 있으면 안 됨) |
| `is_done` | boolean | 완료 여부. 생략하면 `false` |
| `start_date` | `"YYYY-MM-DD"` \| null | 시작일(선택) |
| `end_date` | `"YYYY-MM-DD"` \| null | 종료일(선택). `start_date` 보다 앞설 수 없다 |
| `reminders` | RFC3339 날짜시각[] | 알림 시각 목록(선택). 중복은 하나로 합쳐지고, 시간순으로 정렬되며, 항상 배열이다 |
| `position` | number | Task 안에서의 순서(0부터, 빈틈 없음) |

- Task 의 날짜는 시각을 포함한 값이고, Todo 의 `start_date`/`end_date` 는 시각 없는 **달력 날짜**다. 날짜 선택 UI 를 만들 때 이 차이를 고려해야 한다.
- `reminders` 는 시각까지 필요하다(날짜+시간 선택). 현재는 **저장과 조회만** 하며, 실제 알림 발송은 아직 없다.

### 3.3 순서(position) 규칙

- 새 Todo 는 기본적으로 그 Task 목록의 맨 끝에 붙는다. `position` 을 주면 그 위치에 끼워 넣고 뒤 항목은 한 칸씩 밀린다.
- 수정할 때 `position` 을 주면 그 위치로 옮긴다. 목록 길이보다 크면 맨 끝으로 간다. 음수는 거부된다.
- `task_id` 를 바꾸면 Todo 가 다른 Task 로 **이동**한다(`position` 을 생략하면 새 Task 의 끝으로 간다).
- 이동하거나 삭제한 뒤에는 원래 Task 와 새 Task 모두 0..n-1 로 다시 번호가 매겨진다.
- 따라서 Task 안 드래그 앤 드롭 순서 변경, Task 간 Todo 이동 UI 를 모두 지원할 수 있다.

### 3.4 Todo 조회 필터

`GET /todos` 는 아래 조건을 조합(AND)해 Todo 를 거른다. 조건을 주지 않으면 전체를 돌려준다. 결과는 `task_id`, `position` 순으로 정렬된다.

| 파라미터 | 의미 | 쓰임새 예 |
| --- | --- | --- |
| `task_id=…` | 특정 Task 의 Todo 만 | Task 상세의 체크리스트 |
| `today=YYYY-MM-DD` | 시작일이 없거나 시작일이 그날 이전·당일인 Todo | "오늘 할 수 있는 일" 목록 |
| `end_date=YYYY-MM-DD` | 종료일이 정확히 그날인 Todo | "오늘 마감" 목록 |
| `is_done=true\|false` | 완료 여부 | 미완료만 보기, 완료 숨기기 |

- `today` 필터는 완료 여부와 종료일을 보지 않는다. "오늘의 미완료 Todo" 를 원하면 `today=…&is_done=false` 처럼 조합한다.
- 필터 결과에는 Task 정보가 들어 있지 않고 `task_id` 만 있다. Task 를 가로지르는 Todo 목록(예: 오늘 화면)에서 각 Todo 의 Task 제목·출처를 보여주려면 Task 목록과 합쳐서 그려야 한다.

## 4. API 요약

모든 요청·응답 본문은 JSON 이다. 오류는 `{"error": "…"}` 형식이다.

### 4.1 Task

| 메서드·경로 | 설명 | 성공 응답 |
| --- | --- | --- |
| `GET /tasks` | 전체 Task 목록(ID 순). 관계는 빠지고 `description` 은 100자에서 잘려 `…` 이 붙는다 | 200 `{"tasks":[TaskSummary…]}` |
| `GET /tasks/{id}` | Task 상세. 전체 `description`, 관계 ID, 그리고 관계 중 **저장된 Task 의 요약**을 함께 준다 | 200 `TaskDetail` |
| `POST /tasks` | Local Task 생성 | 201 `{"id":"local-WHF-001"}` |
| `PUT /tasks/{id}` | Local Task 전체 교체(아래 주의) | 204 |
| `DELETE /tasks/{id}` | Task 삭제(출처 무관). 관계와 Todo 도 함께 삭제 | 204 |
| `POST /tasks/jira` | Jira 동기화 | 200 `{"upserted":12,"partial":false}` |

- 생성·수정 본문: `{title, description, status, start_date, due_date, parent_id, children_ids, linked_ids}`.
- `PUT` 은 **전체 교체**다. 빠진 필드는 빈 값으로 처리되므로, 관계를 유지하려면 상세에서 받은 `parent_id`·`children_ids`·`linked_ids` 를 모두 다시 보내야 한다. 편집 폼은 이 값들을 숨겨진 상태로라도 유지해야 한다.
- Jira Task 에 `PUT` 하면 409(Conflict)다.

`TaskDetail` 예시:

```json
{
  "id": "local-WHF-003",
  "originated": "local",
  "title": "결제 모듈 리팩터링",
  "description": "전체 설명…",
  "status": "in-progress",
  "start_date": "2026-10-20T00:00:00Z",
  "due_date": null,
  "parent_id": "jira-PROJ-10",
  "children_ids": ["jira-PROJ-21", "local-WHF-004"],
  "linked_ids": ["jira-PROJ-99"],
  "parent": null,
  "children": [ { "id": "jira-PROJ-21", "originated": "jira", "title": "…", "description": "…", "status": "todo", "start_date": null, "due_date": null } ],
  "linked": []
}
```

위 예시에서 `parent_id` 는 있지만 `parent` 가 null 이다. 상위 Task `jira-PROJ-10` 이 Wharf 에 저장되어 있지 않다는 뜻이다. `children_ids` 는 2개인데 `children` 은 1개이므로 `local-WHF-004` 도 저장되어 있지 않은 Task 다.

### 4.2 Todo

| 메서드·경로 | 설명 | 성공 응답 |
| --- | --- | --- |
| `GET /todos?task_id=&today=&end_date=&is_done=` | 필터 조회 | 200 `{"todos":[Todo…]}` |
| `POST /todos` | Todo 생성 | 201 `{"id":7}` |
| `PUT /todos/{id}` | Todo 전체 교체(이동·순서 변경 포함) | 204 |
| `DELETE /todos/{id}` | Todo 삭제 | 204 |

- 생성·수정 본문: `{task_id, description, is_done, start_date, end_date, reminders, position}`.
- 단건 조회(`GET /todos/{id}`)는 없다. Task 상세 응답에도 Todo 는 포함되지 않으므로 `GET /todos?task_id=…` 로 따로 불러온다.
- `PUT` 도 전체 교체다. 완료 체크 하나만 바꿀 때도 나머지 필드(`reminders` 포함)를 함께 보내야 한다. `position` 만 예외로, 생략하면 현재 위치를 유지한다.

`Todo` 예시:

```json
{
  "id": 7,
  "task_id": "jira-PROJ-21",
  "description": "API 스펙 리뷰 요청",
  "is_done": false,
  "start_date": "2026-10-26",
  "end_date": "2026-10-28",
  "reminders": ["2026-10-27T01:00:00Z"],
  "position": 0
}
```

### 4.3 오류와 사용자 피드백

| 상태 코드 | 언제 | UI 에서의 의미 |
| --- | --- | --- |
| 400 | 제목·내용이 공백, 알 수 없는 상태값, 잘못된 날짜 형식, 시작일 > 종료일, 음수 position, 존재하지 않는 Task 에 Todo 연결, 자기 자신을 관계로 지정 등 | 입력 검증 오류. 폼에 표시 |
| 404 | 없는 Task·Todo | 이미 삭제됨. 목록 새로고침 |
| 409 | Jira Task 수정 시도 | 읽기 전용 안내 |
| 413 | 요청 본문이 1MB 초과 | 내용이 너무 김 |
| 500 | 서버·Jira 연동 실패 | 일반 오류 안내, 재시도 |

## 5. 디자인 시 고려할 점 (도메인에서 나오는 요구)

- **출처 구분**: 모든 Task 표시에 출처를 보여준다. Jira Task 는 읽기 전용이라는 점이 드러나야 하고, 수정 진입점은 Local Task 에만 둔다. 삭제는 모든 출처에서 가능하지만 Jira Task 는 재동기화로 되살아날 수 있다.
- **두 단계 구조**: Task(큰 일)와 Todo(작은 할 일)는 위계가 다른 개념이다. Task 상세 안에 순서 있는 Todo 체크리스트를 두는 구조가 자연스럽다.
- **Task 를 가로지르는 Todo 보기**: `today`·`end_date`·`is_done` 필터는 "오늘 할 일", "오늘 마감" 같은 Task 횡단 화면을 염두에 둔 기능이다. 이 화면에서는 각 Todo 가 어느 Task 에 속하는지 함께 보여줘야 한다.
- **관계 탐색**: Task 상세에서 상위·하위·연결 Task 로 이동할 수 있어야 한다. 저장되지 않은 Task(ID 만 있음)는 이동할 수 없는 상태로 구분해 보여준다.
- **빈 값이 흔하다**: 날짜, 상위 Task, 관계, reminders, description 은 대부분 비어 있을 수 있다.
- **동기화 피드백**: Jira 동기화의 진행 중, 성공(개수), 일부 실패, 전체 실패 상태가 필요하다.
- **목록 규모와 정렬**: Task 목록은 페이지네이션·서버 정렬·검색이 없고 ID 순 전체 목록만 온다. 필터·정렬·그룹(상태별, 출처별, 마감일순 등)은 클라이언트에서 처리한다. Todo 도 페이지네이션이 없다.
- **확장 여지**: Notion·GitLab 출처, reminder 알림 발송은 아직 없지만 추가될 예정이거나 추가될 수 있다.

## 6. 현재 없는 것 (디자인에서 가정하면 안 되는 것)

- 사용자·계정·로그인·공유·협업·댓글
- Task 의 담당자, 우선순위, 라벨·태그, 첨부파일
- Jira 로의 쓰기(상태 변경 등 모든 변경은 Wharf 안에서만 일어난다)
- 자동·주기 동기화, 실시간 업데이트(푸시)
- reminder 알림 발송
- Todo 단건 조회, Todo 부분 수정(PATCH), Todo 의 하위 Todo
- 서버 측 검색·정렬·페이지네이션
