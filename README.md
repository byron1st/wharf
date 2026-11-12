# Wharf

Go 백엔드 서버(`github.com/byron1st/wharf`)와 서버에 내장된 웹 앱(Vite + React)으로 구성된 로컬 도구입니다. 이 문서는 빌드 → 설정 파일 → 실행 순서로 Wharf를 띄우는 방법을 설명합니다.

## 사전 준비

- Go (`go.mod`의 `go` 버전 이상)
- Node.js와 npm (웹 앱 빌드용)
- `make`
- Postgres 데이터베이스(Supabase). 기기 여러 대가 같은 DB를 쓰며, 테이블은 `wharf` 스키마에 만들어집니다. 기존 SQLite `wharf.db`의 데이터를 옮기는 방법은 [docs/supabase-data-migration.md](docs/supabase-data-migration.md)에 있습니다.
- 테스트(`make test`, `make race`)를 돌리려면 Docker. DB 테스트가 testcontainers로 Postgres 컨테이너를 띄우며, Docker가 없으면 건너뛰지 않고 실패합니다.

## 1. 빌드

저장소 루트에서 실행합니다.

```sh
make build
```

`make build`는 먼저 웹 앱을 빌드(`npm ci` + `npm run build`, 결과는 `web/dist`)한 뒤, 이를 내장한 서버 바이너리를 저장소 루트에 `./wharf`로 생성합니다.

`PATH`에서 바로 `wharf`를 실행하려면 대신 `make install`을 사용합니다. 같은 방식으로 웹 앱을 빌드한 뒤 `go install`로 바이너리를 `$GOBIN`(설정하지 않았다면 `$GOPATH/bin`)에 설치합니다.

바이너리 없이 소스에서 바로 실행하는 `make run`도 있지만, 웹 UI를 보려면 `make web`으로 `web/dist`를 먼저 만들어 두어야 합니다.

## 2. 설정 파일

설정 파일은 TOML이며, 서버를 띄우려면 `db_url`이 필요합니다. 나머지 키는 선택 사항이고 파일이 없으면 기본값으로 읽히지만, 그 상태로는 DB에 연결할 수 없어 시작하지 않습니다. 알 수 없는 키는 거부됩니다(예전의 `db_path`도 거부됩니다).

기본 위치는 `<os.UserConfigDir()>/wharf/config.toml`입니다.

- macOS: `~/Library/Application Support/wharf/config.toml`
- Linux: `~/.config/wharf/config.toml`

다른 위치를 쓰려면 환경 변수 `WHARF_CONFIG`에 파일 경로를 지정합니다.

예시 파일을 복사해서 시작합니다(macOS 기준).

```sh
mkdir -p "$HOME/Library/Application Support/wharf"
cp config.example.toml "$HOME/Library/Application Support/wharf/config.toml"
chmod 600 "$HOME/Library/Application Support/wharf/config.toml"
```

`config.example.toml`에는 Atlassian 키가 값이 채워진 상태로 들어 있으므로, 그대로 쓰지 말고 실제 값으로 바꾸거나 해당 세 줄을 지우세요.

| 키 | 기본값 | 설명 |
| --- | --- | --- |
| `addr` | `127.0.0.1:8080` | 서버가 listen할 TCP 주소. 요청은 이 머신에서 오고 loopback 호스트(`localhost`, `127.0.0.1`, `[::1]`)를 가리켜야 하며, 그렇지 않으면 403입니다. |
| `db_url` | 없음(필수) | 비밀번호를 뺀 Postgres 연결 URL(`postgres://` 또는 `postgresql://`). Supabase에서는 Connect › Session pooler URL(포트 5432)을 씁니다. Transaction pooler(6543)는 wharf가 쓰는 세션 단위 advisory lock과 맞지 않습니다. 비밀번호가 들어 있으면 시작하지 않습니다. |
| `atlassian_site_url` | 없음 | Jira Cloud REST API 기본 URL(https만). Scoped API 토큰이면 `https://api.atlassian.com/ex/jira/{cloudId}`. |
| `atlassian_email` | 없음 | Basic 인증에 쓸 Atlassian 계정 이메일. |
| `atlassian_api_token` | 없음 | [API 토큰](https://id.atlassian.com/manage-profile/security/api-tokens). 평문으로 저장되므로 파일 권한을 `600`으로 두세요. |
| `update_check` | `true` | GitHub Releases에서 새 버전을 확인할지 여부. 릴리스 빌드(`X.Y.Z`)를 macOS(arm64)에서 실행할 때만 확인합니다. |
| `update_check_interval` | `"24h"` | 확인 주기(`"6h"`, `"48h"` 같은 Go duration). `"1h"`보다 짧으면 시작하지 않습니다. |

`atlassian_*` 세 키는 모두 설정하거나 모두 비워 두어야 합니다. 서버를 띄운 뒤 웹 앱의 Settings › Atlassian Cloud에서 저장해도 되며, 이때 해당 세 줄만 다시 쓰이고 주석과 다른 키는 유지됩니다.

### DB 비밀번호 (macOS Keychain)

DB 비밀번호는 설정 파일에 두지 않고, 시작할 때 로그인 키체인에서 서비스 이름이 `wharf-db`인 일반 암호 항목을 읽습니다. 한 번만 등록하면 됩니다. `-w` 뒤에 값을 주지 않으면 프롬프트로 입력받으므로 셸 기록에 남지 않습니다.

```sh
security add-generic-password -a "$USER" -s wharf-db -w
```

비밀번호를 바꾸려면 `-U`를 붙여 다시 실행합니다. 항목이 없거나 비어 있으면 서버가 시작하지 않습니다.

## 3. 실행

빌드한 바이너리를 실행합니다.

```sh
./wharf
```

다른 설정 파일을 쓰려면:

```sh
WHARF_CONFIG=/path/to/config.toml ./wharf
```

정상적으로 시작되면 JSON 로그로 `server started`와 listen 주소가 출력됩니다.

```json
{"level":"INFO","msg":"server started","addr":"127.0.0.1:8080"}
```

브라우저에서 <http://localhost:8080>을 열면 웹 앱이 보이고, API는 `/api` 아래에 있습니다. 종료는 `Ctrl+C`(SIGINT/SIGTERM)입니다.

### 참고

- 시작할 때마다 DB 마이그레이션이 자동으로 적용됩니다.
- DB가 원격이므로 네트워크 없이는 시작하지 않고 동작하지도 않습니다.
- 소스에서 바로 실행하려면 `make run`을 쓰세요(`go run ./cmd/wharf`).
- 프런트엔드를 개발할 때는 `make run`으로 서버를 띄운 상태에서 `make web-dev`로 Vite 개발 서버를 함께 실행하세요.

## 4. 백그라운드 실행 (macOS)

터미널을 닫아도 계속 돌도록 launchd LaunchAgent로 등록합니다. 실행할 바이너리 자체로 명령을 실행하세요.

```sh
wharf service install
```

- `~/Library/LaunchAgents/com.byron1st.wharf.plist`를 쓰고 바로 실행합니다. 로그인할 때 자동으로 시작되고, 비정상 종료하면 launchd가 다시 띄웁니다.
- plist에는 명령을 실행한 바이너리의 절대 경로가 들어갑니다. 바이너리를 옮겼다면 다시 실행하세요. 이미 등록돼 있으면 다시 읽어 들입니다.
- `WHARF_CONFIG`가 설정된 셸에서 실행하면 그 경로(절대 경로로 바꿔서)를 plist에 넣습니다. 설정하지 않았으면 기본 위치를 씁니다.
- 로그(JSON)는 `~/Library/Logs/wharf/wharf.log`에 쌓입니다. 자동으로 정리되지 않습니다.

해제:

```sh
wharf service uninstall
```

주의:

- LaunchAgent가 떠 있는 동안 터미널에서 `./wharf`를 또 실행하면 같은 포트를 쓰므로 `failed to listen` 오류로 종료됩니다. 반대로 수동 실행 중에 등록하면 LaunchAgent 쪽이 포트를 얻지 못해 10초 간격으로 재시작을 반복하니, 먼저 수동 실행을 끄세요.
- Settings에서 다른 설정 파일을 골라 재시작하면 그 경로는 지금 프로세스에만 적용됩니다. 재부팅하거나 launchd가 다시 띄우면 plist의 경로로 돌아가므로, 계속 쓰려면 `WHARF_CONFIG=<경로> wharf service install`로 다시 등록하세요.

## 자동 업데이트

릴리스 빌드를 macOS(arm64)에서 실행하면, 서버가 시작할 때와 `update_check_interval`마다 GitHub의 최신 릴리스(`/releases/latest`, draft와 pre-release 제외)를 확인합니다.

- 새 버전을 처음 발견하면 macOS 알림을 한 번 띄웁니다. Settings › General의 Version에서 현재 버전, 마지막 확인 시각과 결과를 볼 수 있고, **Check for updates**로 바로 확인할 수 있습니다. 확인이 실패하면 그 사실이 표시되고, 원인은 서버 로그에 남습니다.
- 새 버전이 있으면 Version에 나타나는 **Update and restart**를 누릅니다. 서버가 릴리스 archive와 `checksums.txt`를 받아 SHA-256을 검증한 뒤, archive 안의 `wharf`로 실행 중인 바이너리를 교체하고 재시작합니다. 웹 앱은 서버가 돌아오면 새로고침됩니다.
- 교체는 같은 디렉터리 안에서 rename으로 하므로, 바이너리가 있는 디렉터리에 쓰기 권한이 있어야 합니다(`~/.local/bin` 등). 검증이나 교체가 실패하면 바이너리는 그대로입니다.
- 이전 바이너리는 `wharf.old`로 남습니다. 새 버전에 문제가 있으면 서버를 멈추고 되돌립니다. 새 버전이 DB 마이그레이션을 적용했다면 `DB 마이그레이션`의 백업 파일도 함께 되돌려야 이전 버전이 시작됩니다.

  ```sh
  wharf service uninstall
  mv /path/to/wharf.old /path/to/wharf
  wharf service install
  ```

- GitHub API는 인증 없이 호출하면 IP당 시간당 60회로 제한되어, 회사 망처럼 여러 사람이 공인 IP를 공유하면 확인이 403으로 실패할 수 있습니다. 로그인 키체인에 서비스 이름이 `wharf-github`인 GitHub 토큰을 등록하면 확인 요청에 그 토큰을 써서 사용자당 한도(시간당 5,000회)를 적용받습니다. 선택 사항이며, 없으면 인증 없이 확인합니다.

  ```sh
  security add-generic-password -a "$USER" -s wharf-github -w
  ```

  - 토큰은 fine-grained personal access token에 Repository access를 "Public repositories (read-only)"로 두고 다른 권한은 주지 않으면 됩니다(classic이라면 scope 없이).
  - 서버가 시작할 때 한 번 읽으므로, 등록하거나 바꾼 뒤에는 재시작합니다(`launchctl kickstart -k gui/$(id -u)/com.byron1st.wharf`). 읽지 못하면 경고 로그만 남기고 인증 없이 확인합니다.
  - 토큰은 최신 릴리스 조회에만 쓰고, 릴리스 파일 다운로드에는 보내지 않습니다.
  - 토큰이 만료되거나 폐기되면 확인이 계속 실패하고 로그에 `github: token rejected`가 남습니다. 새 토큰으로 바꾸거나(`security add-generic-password -U -a "$USER" -s wharf-github -w`) 항목을 지운 뒤 재시작하세요.
- `make build`/`make install`로 만든 개발 빌드(`(devel)`, pseudo-version, `+dirty`)는 확인하지 않습니다. `update_check = false`로 끌 수도 있습니다.
- v0.1.0에는 업데이트 기능이 없으므로, 이 기능이 들어간 첫 버전은 직접 설치해야 합니다.

## DB 마이그레이션

시작할 때마다 `scripts/migrations/`의 SQL 파일 중 아직 적용하지 않은 것을 버전 순서대로 적용하고, 각 파일의 SHA-256을 `schema_migrations`에 기록합니다.

- 스키마를 바꿀 때는 기존 파일을 고치지 말고 다음 번호의 파일(예: `0002_add_task_index.sql`)을 추가합니다. 적용된 파일 내용이 바뀌면 서버가 시작을 거부합니다.
- 모든 테이블은 `wharf` 스키마에 있습니다. 스키마가 없으면 만들고, 연결마다 `search_path`를 `wharf`로 맞춥니다.
- 여러 기기가 동시에 시작해도 advisory lock으로 한 번에 하나씩만 마이그레이션합니다.
- 마이그레이션 전 자동 백업은 하지 않습니다. Supabase의 백업(대시보드 Database › Backups)이나 `pg_dump`에 맡깁니다.
- DB가 이 바이너리보다 새 버전으로 마이그레이션되어 있으면(다운그레이드) 서버가 시작을 거부합니다. 모든 기기의 wharf를 같은 버전으로 맞추세요.

## 버전과 릴리스

버전은 [Semantic Versioning](https://semver.org) `X.Y.Z`이며, `main`의 git 태그 `vX.Y.Z`가 유일한 원천입니다. `make build`/`make install`(`go build`/`go install`)이 태그를 바이너리에 기록하고, 웹 앱 Settings 하단에 `Wharf · X.Y.Z`로 표시됩니다. 태그 이후 커밋에서 빌드하면 `X.Y.(Z+1)-0.<시각>-<커밋>`, 커밋하지 않은 변경이 있으면 `+dirty`가 붙습니다.

- MAJOR: 호환되지 않는 변경(설정 키 제거, 데이터를 버리는 마이그레이션 등)
- MINOR: 기능 추가(새 마이그레이션 포함)
- PATCH: 버그 수정

릴리스 절차:

1. `develop`을 `main`으로 PR 병합합니다.
2. `main`에서 태그를 만들어 push합니다.

   ```sh
   git switch main && git pull
   git tag v0.1.0
   git push origin v0.1.0
   ```

3. `release` 워크플로가 macOS(arm64) 바이너리를 빌드해 `wharf_0.1.0_darwin_arm64.tar.gz`와 그 SHA-256을 담은 `checksums.txt`를 GitHub Release에 첨부합니다. 이미 설치된 Wharf는 이 두 파일로 업데이트합니다.

`go install github.com/byron1st/wharf/cmd/wharf@vX.Y.Z`로 설치하면 웹 앱이 포함되지 않으므로, 릴리스 파일이나 `make install`을 사용하세요.

## CI

`main`·`develop`으로의 push와 PR에서 `ci` 워크플로가 `make check`(변경이 생기면 실패), `make test`, `make build`, `make web-check`를 실행합니다. `make test`의 DB 테스트는 GitHub 호스티드 Ubuntu 러너에 기본 설치된 Docker로 Postgres 컨테이너를 띄우며, 워크플로가 먼저 `docker info`로 Docker가 쓸 수 있는지 확인합니다.
