# Research Index

| File | Application | ResearchType | Description |
|------|-------------|--------------|-------------|
| [server-service-structure](./server-service-structure.md) | wharf | Structure | internal/server(route·미들웨어·Run)·internal/service(SyncJiraTasks) 의 구조와 main wiring, internal/config 의 TOML 설정 로딩(경로·기본값·acli 해석), db.Manager(관계 테이블 쓰기 규칙·CreateLocalTask 포함)·atlassian.Client·tool.Tool 계약과 mock, GetMyTasks 부분 실패 반환 규칙과 그 처리, db 테스트의 저장 확인 방식, mockery·lint·에러·테스트 관례, AGENTS.md 의 계층 서술 위치를 정리한다. |
| [sqlite-sqlc-setup](./sqlite-sqlc-setup.md) | wharf | Structure | sqlc(SQLite 엔진) 설정 경로, Makefile modelgen 동작, tasks·task_hierarchies·task_links·counters 스키마와 Task 구조체 매핑, sqlc SQLite 의 bulk insert 제약과 생성 코드 형태(UPDATE ... RETURNING, ON CONFLICT)를 정리한다. |
| [task-id-and-relations](./task-id-and-relations.md) | wharf | Structure | Task ID `{originated}-{key}` 가 만들어지는 곳(task.NewID·atlassian ToTask·db CreateLocalTask)과 local key(WHF-001) 카운터, parent/children(task_hierarchies)·linked(task_links) 관계 테이블의 쓰기 소유 규칙과 그 근거(Jira subtasks/issuelinks 특성, cross-origin 관계)·중복·FK 제약, SQLite 쓰기 우선 트랜잭션 제약, ID 와 관계 테이블에 의존하는 테스트 위치를 정리한다. |
