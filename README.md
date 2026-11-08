# Wharf

Go 백엔드 서버(`github.com/byron1st/wharf`)와 서버에 내장된 웹 앱(Vite + React)으로 구성된 로컬 도구입니다. 이 문서는 빌드 → 설정 파일 → 실행 순서로 Wharf를 띄우는 방법을 설명합니다.

## 사전 준비

- Go (`go.mod`의 `go` 버전 이상)
- Node.js와 npm (웹 앱 빌드용)
- `make`

## 1. 빌드

저장소 루트에서 실행합니다.

```sh
make build
```

`make build`는 먼저 웹 앱을 빌드(`npm ci` + `npm run build`, 결과는 `web/dist`)한 뒤, 이를 내장한 서버 바이너리를 저장소 루트에 `./wharf`로 생성합니다.

`PATH`에서 바로 `wharf`를 실행하려면 대신 `make install`을 사용합니다. 같은 방식으로 웹 앱을 빌드한 뒤 `go install`로 바이너리를 `$GOBIN`(설정하지 않았다면 `$GOPATH/bin`)에 설치합니다.

바이너리 없이 소스에서 바로 실행하는 `make run`도 있지만, 웹 UI를 보려면 `make web`으로 `web/dist`를 먼저 만들어 두어야 합니다.

## 2. 설정 파일

설정 파일은 TOML이며, 모든 키가 선택 사항입니다. 파일이 없으면 모두 기본값으로 동작합니다. 알 수 없는 키는 거부됩니다.

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
| `db_path` | `<UserConfigDir>/wharf/wharf.db` | SQLite 파일 경로. 디렉터리가 없으면 만들어집니다. |
| `atlassian_site_url` | 없음 | Jira Cloud REST API 기본 URL(https만). Scoped API 토큰이면 `https://api.atlassian.com/ex/jira/{cloudId}`. |
| `atlassian_email` | 없음 | Basic 인증에 쓸 Atlassian 계정 이메일. |
| `atlassian_api_token` | 없음 | [API 토큰](https://id.atlassian.com/manage-profile/security/api-tokens). 평문으로 저장되므로 파일 권한을 `600`으로 두세요. |

`atlassian_*` 세 키는 모두 설정하거나 모두 비워 두어야 합니다. 서버를 띄운 뒤 웹 앱의 Settings › Atlassian Cloud에서 저장해도 되며, 이때 해당 세 줄만 다시 쓰이고 주석과 다른 키는 유지됩니다.

테스트 삼아 띄워보는 용도라면 설정 파일 없이도 시작할 수 있습니다.

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

- 첫 실행 때 DB 마이그레이션이 자동으로 적용됩니다.
- 소스에서 바로 실행하려면 `make run`을 쓰세요(`go run ./cmd/wharf`).
- 프런트엔드를 개발할 때는 `make run`으로 서버를 띄운 상태에서 `make web-dev`로 Vite 개발 서버를 함께 실행하세요.
- 아직 첫 배포 전이라 스키마 변경이 `0001_initial_migration.sql`에 직접 반영됩니다. 이전 버전으로 만든 로컬 `wharf.db`에서 오류가 나면 파일을 삭제하고 다시 실행하세요.
