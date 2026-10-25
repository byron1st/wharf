# Research Index

| File | Application | ResearchType | Description |
|------|-------------|--------------|-------------|
| [server-service-structure](./server-service-structure.md) | wharf | Structure | internal/server·internal/service 뼈대의 TODO, db.Manager·atlassian.Client·tool.Tool 계약과 mock, GetMyTasks 부분 실패 반환 규칙, ExistsTask/CreateTasks 를 쓰는 테스트 위치, mockery·lint·에러·테스트 관례, AGENTS.md 의 internal/route 언급 위치를 정리한다. |
| [sqlite-sqlc-setup](./sqlite-sqlc-setup.md) | wharf | Structure | sqlc(SQLite 엔진) 설정 경로, Makefile modelgen 동작, Task 구조체→tasks 컬럼 매핑, sqlc SQLite 의 bulk insert 제약과 생성 코드 형태를 정리한다. |
| [task-db-relations](./task-db-relations.md) | wharf | Structure | tasks 테이블의 현재 스키마(JSON 배열 관계 컬럼), db.Manager 의 CreateTasks/ExistsTask 계약과 테스트 헬퍼, Jira adapter 가 ID·관계 ID 를 만드는 위치, sqlc SQLite 의 UPDATE…RETURNING·DROP COLUMN 생성 검증 결과와 codegen 도구 경로를 정리한다. Task ID 형식·관계 테이블 분리·local ID 카운터 작업의 출발점. |
