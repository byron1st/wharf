---
Application: wharf
JiraTicket: NO-JIRA
PlanType: multi-step
Timestamp: 20261109125719
Title: supabase-migration
---

# SQLite → Supabase(Postgres) 이전

개인 기기 간 동기화를 위해 로컬 SQLite 저장소를 Supabase(Postgres)로 옮긴다. Supabase 는 SQL 접속으로만 사용하며, 계정·DB 생성은 사용자가 수동으로 한다.

## 사용자 결정

- SQLite 는 완전히 대체한다(병행 지원 없음).
- 마이그레이션 전 자동 백업(`VACUUM INTO`)은 제거하고 Supabase 자체 백업에 의존한다.
- DB 비밀번호는 macOS Keychain 에 사용자가 수동 등록하고, wharf 는 시작 시 조회만 한다.
- 테스트 인프라는 testcontainers 를 쓰고, GitHub CI 에서 Docker 기반 테스트가 돌도록 설정한다.
- 기존 데이터 이전 방법은 별도 Markdown 문서로 작성한다.

## Done when

1. SQLite 제거: `rg -n -i "sqlite|modernc" -t go` 결과 없음, `go.mod` 에 `modernc.org/sqlite` 없음, `scripts/sqlc.yaml` 이 `engine: postgresql`, `make gen` 재실행 후 `git diff` 없음.
2. Postgres 스키마: 빈 Postgres(testcontainers)에 `Migrate` 가 `0001` 을 적용하고 두 번 실행해도 결과가 같다. checksum 불일치(`ErrMigrationChecksumMismatch`)·모르는 버전(`ErrUnknownAppliedMigration`) 검사는 기존 테스트대로 유지되고, 두 `Manager` 가 동시에 `Migrate` 해도 한 번만 적용된다는 새 테스트가 통과한다.
3. 테스트 인프라: Docker 가 있는 환경에서 `make test`, `make race` 통과. `internal/adapter/db` 테스트 전부가 testcontainers Postgres 위에서 돈다(`:memory:`·파일 DB 없음). Docker 데몬이 없으면 원인을 알 수 있는 메시지로 실패한다(조용히 skip 하지 않음).
4. CI: `ci.yml` 의 `go` job(ubuntu-latest)이 PR 에서 green 이고, 로그에 Postgres 컨테이너 기동과 db 패키지 `ok` 가 보인다.
5. 설정·Keychain: config 의 `db_path` 를 `db_url`(비밀번호 없는 Postgres URL)로 교체하고 `config.example.toml` 에 문서화한다. 비밀번호는 `system` 어댑터가 `/usr/bin/security find-generic-password` 로 조회한다(서드파티 의존성 없음). Keychain 항목이 없거나 `db_url` 에 비밀번호가 들어 있으면 식별 가능한 sentinel 에러로 시작이 실패한다(단위 테스트). Settings 패널은 DB 경로 대신 비밀번호 없는 접속 대상(host/db)을 표시하고 `make web-check` 가 통과한다.
6. 문서: `docs/supabase-data-migration.md` 에 Keychain 등록 명령, 기존 `wharf.db` export(sqlite3 CLI) → Supabase import(`psql \copy`), identity 시퀀스·`counters` 보정, 테이블별 row count 검증, Data API 노출 차단(RLS 또는 권한 회수)을 담는다. README·AGENTS.md 의 DB 관련 설명을 Postgres 기준으로 갱신한다.
7. 수동 시나리오(사용자 실행): macOS 에서 Keychain 등록과 `db_url` 설정 후 `make run` → 실제 Supabase 에 연결되고 마이그레이션이 적용되며, 기기 2대에서 같은 데이터가 보인다.

검증하지 않는 것: 실제 Supabase 연결과 문서의 데이터 이전 절차(사용자 실행 필요), 여러 기기의 동시 같은 행 수정 시 정합성(트랜잭션 이상의 충돌 해소는 다루지 않음).

## Out of scope

- SQLite 병행 지원, 마이그레이션 전 자동 백업(제거만 함).
- Settings UI 에서 비밀번호 입력, Linux/Windows 키체인.
- 데이터 이전 자동화 코드나 도구(문서만 작성).
- 오프라인 모드·로컬 캐시, Supabase Realtime, 기기 간 충돌 해소 로직.
- Supabase 계정·DB 생성(사용자 수동), 버전 태그·릴리스 작업.

## Do not touch

- `service.Updater`, 릴리스 asset 계약(`release.yml`, 파일명).
- Atlassian 동기화 로직, `/api` 응답 형태(Settings 의 DB 표시 필드 하나만 예외).
- `db.Manager` 의 도메인 메서드 시그니처(`NewManager`·`Connect` 인자만 변경), service 계층 인터페이스.
- 마이그레이션 파일 명명 규칙과 checksum·다운그레이드 검사 동작.

## Key decisions

- **드라이버: `pgx/v5/stdlib` 로 `database/sql` 사용.** sqlc 의 `database/sql` 출력과 `inTx`·`*sql.Tx` 구조를 그대로 유지해 변경 범위를 최소화한다. pgx 네이티브 API 는 manager 와 생성 타입 전체를 다시 써야 한다.
- **테스트 격리: 패키지당 컨테이너 1개(`TestMain`) + 테스트마다 새 database.** 테스트별 컨테이너는 너무 느리고, 공유 DB 는 테스트끼리 간섭한다. Postgres 이미지는 Supabase 와 같은 메이저 버전으로 고정한다.
- **접속 경로: Supavisor session mode(5432) URL 권장**을 문서화한다. transaction mode(6543)는 prepared statement 와 충돌한다.

## Approach

1. 의존성: `pgx/v5`, `testcontainers-go`(+ postgres module) 추가, sqlite 제거. PR 설명에 추가 사유를 적는다.
2. 스키마: `0001_initial_migration.sql`·`schema_migrations.sql` 을 Postgres 문법으로 변경(TIMESTAMPTZ, identity, boolean, date). 첫 배포 전이므로 0001 을 직접 수정한다.
3. 쿼리: `?` → `$n`/`sqlc.arg`, `sqlc.slice` → `ANY(...)` 로 바꾸고 `make gen` 후 생성 타입 변화에 맞춰 manager 를 수정한다.
4. `db.Manager`: SQLite pragma·단일 커넥션 제약·백업 제거, `Migrate` 를 advisory lock 으로 감싼다.
5. Keychain·설정: `system` 어댑터에 비밀번호 조회 추가, `config` 의 `db_path` → `db_url`, `main.go` 에서 조회한 비밀번호를 연결 정보에 합친다. Settings 표시 필드와 웹 UI 를 바꾼다.
6. 테스트: db 테스트 헬퍼를 testcontainers 기반으로 바꾸고 동시 `Migrate` 테스트를 추가한다.
7. CI: `go` job 에 `docker info` 확인 단계 추가(ubuntu runner 는 Docker 기본 설치, 별도 서비스 불필요). 필요하면 이미지 pull 단계·타임아웃을 조정한다.
8. 문서: 데이터 이전 문서, README, AGENTS.md(아키텍처, 마이그레이션 백업 문구, 로컬 테스트에 Docker 필요)를 갱신한다.

`db_path` 키 제거로 AGENTS.md 규칙상 다음 릴리스는 MAJOR 버전업 대상이다.

## Completion condition (`/goal`)

```
SQLite→Supabase(Postgres) 이전 완료: (1) `rg -n -i "sqlite|modernc" -t go` 결과 없음, go.mod에 modernc.org/sqlite 없음, sqlc engine=postgresql, `make gen` 후 git diff 없음; (2) internal/adapter/db 테스트가 testcontainers Postgres에서 실행되고 동시 Migrate 단일 적용 테스트 포함; (3) Keychain 비밀번호 조회(system 어댑터, /usr/bin/security)와 db_url 설정, 누락 시 sentinel 에러 테스트 통과, config.example.toml 갱신; (4) Settings가 DB 경로 대신 접속 대상 표시, `make web-check` 통과; (5) docs/supabase-data-migration.md 작성(Keychain 등록, export/import, 시퀀스·counters 보정, row count 검증, Data API 차단), README·AGENTS.md 갱신; (6) ci.yml go job에 Docker 확인 단계 추가; (7) `make check && make test && make race` 통과; 기존 테스트를 약화·삭제하지 않음; 최대 150턴.
```
