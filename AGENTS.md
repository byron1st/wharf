# wharf

Go backend server (`github.com/byron1st/wharf`). HTTP API with sqlc-generated database access, and a Vite + React web app that the server embeds and serves.

## Key Requirements

- Minimal Dependencies: Prefer the standard library over external ones.
- Reliability: Every I/O path propagates `context.Context` and returns wrapped, identifiable errors.
- Testability: Business logic depends on interfaces so services can be tested with generated mocks.

## Core Commands

- `make format`: Format and modernize code (`go fmt`, `go fix`, `golangci-lint fmt`).
- `make lint`: Tidy modules and run `golangci-lint`.
- `make check`: Run `format` and `lint`. Run after every Go source change.
- `make gen`: Regenerate mocks (`mockgen`) and sqlc models (`modelgen`).
- `make test`: Run all tests with a clean test cache.
- `make race`: Run short tests with the race detector.
- `make test-e2e`: Run end-to-end tests under `tests/e2e`.
- `make test-mutation`: Run mutation testing over the module.
- `make test-mutation-pkg PKG=./internal/service/foo`: Run mutation testing for one package.
- `make coverage`: Report total coverage over non-generated packages.
- `make outdated`: List outdated direct dependencies.
- `make build`: Build the web app (`make web`), then binaries from `./cmd/<bin>` with it embedded.
- `make install`: Build the web app (`make web`), then `go install` binaries from `./cmd/<bin>` into `$GOBIN` (default `$GOPATH/bin`) with it embedded.
- `make web`: Install the web app dependencies (`npm ci`) and build it into `web/dist`.
- `make web-check`: Type-check, lint (`oxlint`) and test (`vitest`) the web app.
- `make web-dev`: Run the Vite dev server; it proxies `/api` to the server from `make run` (`WHARF_API` overrides the target), rewriting Origin only for its own pages so the server's cross-site check still applies.
- `make run`: Run the server (`./cmd/wharf`).

## Architecture Overview

- `cmd/wharf/`: Entry point; wires config, adapters, services, and routes only.
- `internal/config/`: Loads the TOML config file (`config.Path`, `config.Load`; a missing file is all defaults), fills defaults, and validates the optional Atlassian REST API keys (`atlassian_site_url`, `atlassian_email`, `atlassian_api_token`, all or none). `config.SaveAtlassian` rewrites only those three lines; `service.Settings` calls it from the settings API. `config.example.toml` at the repo root documents every key.
- `internal/server/`: HTTP server, middleware (including the loopback RemoteAddr/Host/Origin guard), and handlers under `/api`; translates requests/responses and calls `service.Service` and `service.Settings`.
- `internal/service/`: Business logic; depends on adapter interfaces, never on HTTP types.
- `internal/adapter/`: External I/O (database, third-party APIs, `system` for macOS desktop commands). `internal/adapter/db/` holds sqlc output (`db.go`, `models.go`, `queries.sql.go`) next to the hand-written `db.Manager` (`client.go`, `client_management.go`).
- `internal/pkg/<topic>/`: Module-private shared code. `internal/pkg/mocks/` holds generated mocks.
- `pkg/<topic>/`: Public, importable contracts (only when something must be shared outside the module).
- `scripts/`: Tooling config such as `sqlc.yaml`, SQL schema, and queries. Its Go package (`scripts.Migrations()`) embeds `schema_migrations.sql` and `migrations/` for `db.NewManager`, since `go:embed` cannot reach files outside the package directory.
- `scripts/migrations/`: SQL migrations named `{4-digit version}_{snake_case description}.sql` (e.g. `0002_add_task_index.sql`); `db.Manager.Migrate` applies them in version order and never re-applies one, so never edit or renumber an applied migration — add a new one. Exception until the first deployment: every schema change goes directly into `0001_initial_migration.sql` (owner decision), and a local `wharf.db` that already applied 0001 must be deleted and recreated. Each file runs inside a transaction, so it must not contain `BEGIN`/`COMMIT`, `VACUUM`, or `PRAGMA foreign_keys` (silently ignored inside a transaction).
- `docs/`: Project documentation.
- `tests/e2e/`: End-to-end tests against a running server.
- `web/`: The web app (Vite, React, TypeScript, Tailwind, shadcn-style components on Radix). Its Go package (`web.Dist()`) embeds the build in `web/dist`, which `server.NewHandler` serves on every non-`/api` GET path with an `index.html` fallback for client-side routes. Only `web/dist/.gitkeep` is committed, so Go builds and tests pass before a web build. Design sources are in `docs/design/`.

## Code Conventions

- Layering is one-way: `server` → `service` → `adapter`. `cmd/wharf/main.go` only wires. No catch-all `types` or `util` package.
- `context.Context` is the first parameter of anything that does I/O, calls an adapter, or can block.

### Comments

- Every exported function and method (including exported methods on unexported types) has a doc comment starting with its name.
- Unexported functions, variables, and constants get a comment only when their intent is not obvious from the name and code.

### Errors

- Expected failures are exported `Err...` sentinels in the package that owns the operation. Name them after what failed (`ErrFailedToSendRequest = errors.New("http: failed to send request")`, not `ErrRequest`).
- Sentinel messages start with the owning package name and a colon (`"tool: failed to execute cmd"`), following Effective Go, so joined error chains show which layer each line came from. Context added with `fmt.Errorf` does not repeat the prefix.
- Each sentinel has exactly one origin return site, so the error alone pinpoints where it failed. If a condition arises in two places, define two sentinels.
- Wrap only stdlib/third-party errors, as `errors.Join(ErrLocal, err)`; return errors from this module's packages as-is so chains don't grow with every layer, unless the caller holds context the error lacks (`fmt.Errorf("%w: key=%s", err, key)`). Add context with `fmt.Errorf("%w: field=%s", ErrSentinel, value)`. Compare with `errors.Is` / `errors.As`, never by string.

## Testing

- Framework: standard `testing` with `testify/require`; use `assert` only when continuing after a failed comparison adds information.
- Tests live in `<file>_test.go` in the external `<package>_test` package and exercise exported behavior only.
- Mocks are generated by `mockery` via `make gen` into `internal/pkg/mocks/`.
- Before commit: `make check && make test`. Add `make race` for concurrency-related changes, and `make web-check` when `web/` changes.
- Web app tests are `vitest` files next to the code (`*.test.ts[x]`) using Testing Library; keep date logic in `web/src/lib/dates.ts` so it stays unit-testable.

## Boundaries

- NEVER hand-edit generated code (`internal/pkg/mocks/`, sqlc output `internal/adapter/db/{db,models,queries.sql}.go`); change the source (interface, SQL) and run `make gen`.
- NEVER run raw `go test`, `go build`, or `golangci-lint` commands; use the matching `make` target so tooling stays consistent.
- NEVER commit secrets or `.env` files; read configuration through `internal/config` at startup in `cmd/wharf`. The one runtime exception is `service.Settings`, which writes the Atlassian keys with `config.SaveAtlassian` and validates a chosen file with `config.Load` before a restart; nothing else reads or writes config after startup. The only environment variable is `WHARF_CONFIG` (config file path); add new settings as TOML keys, not environment variables.
- NEVER import `internal/server` from `internal/service` or `internal/adapter`; pass plain domain types down the layers instead.
- NEVER add a third-party dependency when the standard library covers the need; justify any new module in the PR description. Web app dependencies live only in `web/package.json`.
