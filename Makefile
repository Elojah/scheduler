GO                 = go
GOFMT              = gofmt
# Pure Go, SQLite included: the binary cross-compiles, such as GOOS=linux GOARCH=arm64 make build.
CGO_ENABLED       ?= 0
BIN                = bin/scheduler
VERSION           ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
CONFIG            ?= config/scheduler/local.json

# For CI
ifneq ($(wildcard ./bin/golangci-lint),)
	GOLINT = $(CURDIR)/bin/golangci-lint
else
	GOLINT = golangci-lint
endif

V                 = 0
Q                 = $(if $(filter 1,$V),,@)
M                 = $(shell printf "\033[0;35m▶\033[0m")

.PHONY: all
all: check

# Tidy
.PHONY: tidy
tidy: ## Run go mod tidy
	$(info $(M) running go mod tidy) @
	$Q $(GO) mod tidy

# Check
.PHONY: check
check: lint test ## Run lint and tests

# Lint
.PHONY: lint
lint: ## Run golangci-lint
	$(info $(M) running $(GOLINT))
	$Q $(GOLINT) run

# Test
.PHONY: test
# -cpu 1,4 varies interleavings, -shuffle catches tests that depend on each other.
test: ## Run tests with race detector, shuffled, with GOMAXPROCS 1 and 4
	$(info $(M) running go test) @
	$Q $(GO) test -cover -race -shuffle=on -cpu 1,4 -v ./...

# Build
.PHONY: build
build: ## Build bin/scheduler, without cgo
	$(info $(M) building $(BIN)) @
	$Q CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -ldflags '-X main.version=$(VERSION)' -o $(BIN) ./cmd/scheduler

# Run
.PHONY: run
# A broker: docker run --rm -p 1883:1883 eclipse-mosquitto mosquitto -c /mosquitto-no-auth.conf
run: ## Run the scheduler with CONFIG (config/scheduler/local.json), on the broker at localhost:1883
	$Q $(GO) run ./cmd/scheduler $(CONFIG)

# Generate (mocks)
.PHONY: generate
generate: ## Generate mocks using moq
	$(info $(M) running go generate for mocks) @
	$Q find ./internal -name '*_moq.go' -delete
	$Q $(GO) generate ./internal/...

## Helpers

.PHONY: fmt
fmt: ## Format code
	$(info $(M) running $(GOFMT)) @
	$Q $(GOFMT) -l -w .

.PHONY: doc
doc: ## Print package documentation
	$(info $(M) running go doc) @
	$Q $(GO) doc -all ./internal/schedule

.PHONY: help
help: ## Print this
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
