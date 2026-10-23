BINS ?= wharf
RUN_BIN ?= $(firstword $(BINS))
BUILD_FLAGS ?=
MOCK_DIR ?= internal/pkg/mocks
SQLC_CONFIG ?= ./scripts/sqlc.yaml
SQLC_DB_FILE ?= internal/adapter/db/db.go
E2E_PKG ?= ./tests/e2e/...
EXCLUDE ?= (mocks|cmd)

.PHONY: outdated
# List outdated direct dependencies
outdated:
	@go list -u -m -f '{{if and (not .Main) (not .Indirect) .Update}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all | sed '/^$$/d'

.PHONY: gen mockgen modelgen
# Generate mocks and database models
gen: mockgen modelgen

# Generate mocks for interfaces in the project
mockgen:
	command -v mockery >/dev/null 2>&1 || go install github.com/vektra/mockery/v3@latest
	rm -rf $(MOCK_DIR) && mkdir -p $(MOCK_DIR)
	mockery
	find $(MOCK_DIR) -type f -name '*.go' -exec perl -pi -e 's/interface\{\}/any/g' {} +
	find $(MOCK_DIR) -type f -name '*.go' -exec gofmt -w {} +

# Generate database models using sqlc
modelgen:
	sqlc generate -f $(SQLC_CONFIG)
	find $$(dirname $(SQLC_DB_FILE)) -type f -name '$$(basename $(SQLC_DB_FILE))' -exec perl -pi -e 's/interface\{\}/any/g' {} +
	find $$(dirname $(SQLC_DB_FILE)) -type f -name '$$(basename $(SQLC_DB_FILE))' -exec gofmt -w {} +

.PHONY: format lint check clean-testcache test race test-e2e test-mutation test-mutation-pkg
# Run formatting and linting
check: format lint

# Format and modernize Go code
format:
	@go fmt ./...
	@go fix ./...
	@golangci-lint fmt ./...

# Tidy modules and run linters
lint:
	@go mod tidy
	@golangci-lint run --fix ./...

# Clean test cache to ensure tests run with the latest code changes
clean-testcache:
	@go clean -testcache

# Clean test caches and run tests
test: clean-testcache
	@go test ./...

# Clean test caches and run race tests
race: clean-testcache
	@go test -short -race ./...

# Clean test caches and run end-to-end tests
test-e2e: clean-testcache
	@go test $(E2E_PKG)

# Run mutation testing and show survived mutants
test-mutation:
	@command -v gremlins >/dev/null 2>&1 || go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
	@gremlins unleash -S l

# Run mutation testing for a single package, for example: make test-mutation-pkg PKG=./internal/service/foo
test-mutation-pkg:
	@test -n "$(PKG)" || (echo "usage: make test-mutation-pkg PKG=./internal/service/foo" >&2; exit 1)
	@command -v gremlins >/dev/null 2>&1 || go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
	@gremlins unleash "$(PKG)" -S l --workers=3 --timeout-coefficient=20

.PHONY: build run
# Build application binaries from ./cmd/<binary>
build:
	@for bin in $(BINS); do \
		$(BUILD_FLAGS) go build -o $$bin ./cmd/$$bin || exit 1; \
	done

# Run the default application binary
run:
	@$(BUILD_FLAGS) go run ./cmd/$(RUN_BIN)

.PHONY: deps-graph
# Show intra-module package dependency edges
deps-graph:
	@MODULE=$$(go list -m); \
	go list -f '{{$$from := .ImportPath}}{{range .Imports}}{{$$from}} -> {{.}}{{"\n"}}{{end}}' ./internal/... \
	| grep -F -- "-> $${MODULE}/"

PACKAGES := $(shell go list ./... | grep -v -E '$(EXCLUDE)')
COVERPKG := $(shell go list ./... | grep -v -E '$(EXCLUDE)' | paste -sd, -)

.PHONY: coverage
# Run package coverage over non-generated application packages
coverage:
	go test $(PACKAGES) -coverprofile=coverage.out -coverpkg=$(COVERPKG)
	go tool cover -func=coverage.out | grep total | awk '{print $$3}'
