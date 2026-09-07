# Project.
APP_NAME := rinha-fraud-detector
GO       := go
GOBIN    := $(shell go_bin=$$($(GO) env GOBIN); echo "$${go_bin:-$$($(GO) env GOPATH)/bin}")
BIN_DIR  := bin
DATA_DIR := data
RESOURCES_DIR := resources

# Tools.
GOLANGCI_LINT  := $(shell test -x "$(GOBIN)/golangci-lint" && echo "$(GOBIN)/golangci-lint" || echo golangci-lint)
DOCKER_COMPOSE := docker compose

# Upstream sources.
REFERENCES_URL := https://raw.githubusercontent.com/zanfranceschi/rinha-de-backend-2026/main/resources/references.json.gz
TEST_DATA_URL  := https://raw.githubusercontent.com/zanfranceschi/rinha-de-backend-2026/main/test/test-data.json

# Make.
.DEFAULT_GOAL := help

.PHONY: \
	build \
	ci \
	clean \
	docker-build \
	docker-down \
	docker-logs \
	docker-up \
	fmt \
	help \
	index \
	lint \
	load-data \
	load-smoke \
	load-test \
	run \
	stress-test \
	test \
	test-e2e \
	update-dataset \
	vet

# Help.
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "App:"
	@echo "  build           Build the api and indexbuilder binaries into ./bin"
	@echo "  run             Run the api binary locally [requires: make index]"
	@echo "  index           Build data/index.bin from the vendored dataset (resources/)"
	@echo "  update-dataset  Refresh the vendored reference dataset from upstream"
	@echo ""
	@echo "Quality:"
	@echo "  fmt             Format all Go source"
	@echo "  vet             Run go vet"
	@echo "  lint            Run golangci-lint"
	@echo "  test            Run unit tests with race detector and coverage"
	@echo "  test-e2e        Run E2E tests against a running docker compose stack"
	@echo "  ci              Run the full local quality gate (fmt, vet, lint, test)"
	@echo ""
	@echo "Infra (Docker):"
	@echo "  docker-build    Build the api Docker image"
	@echo "  docker-up       Start the full stack (HAProxy + 2 API replicas) on :9999"
	@echo "  docker-down     Stop and remove the stack"
	@echo "  docker-logs     Tail logs from every service (or one: SERVICE=<name>, e.g. SERVICE=api1)"
	@echo ""
	@echo "Load testing (official k6 scripts, see loadtest/):"
	@echo "  load-smoke      Run the official k6 smoke test [requires: make docker-up]"
	@echo "  load-test       Run the official k6 full load test (alias: stress-test)"
	@echo "  load-data       Refresh the vendored k6 test payloads from upstream"
	@echo ""
	@echo "Other:"
	@echo "  clean           Remove build artifacts and downloaded/generated data"

# Build.
build:
	@echo "Building binaries..."
	$(GO) build -o $(BIN_DIR)/api ./cmd/api
	$(GO) build -o $(BIN_DIR)/indexbuilder ./cmd/indexbuilder

run: build
	@echo "Running $(APP_NAME)..."
	INDEX_PATH=$(DATA_DIR)/index.bin ./$(BIN_DIR)/api

# Dataset & index. The dataset itself (resources/references.json.gz) is
# vendored and committed — no download needed to build the index.
index: build
	@echo "Building k-d tree index..."
	mkdir -p $(DATA_DIR)
	./$(BIN_DIR)/indexbuilder -input $(RESOURCES_DIR)/references.json.gz -output $(DATA_DIR)/index.bin

update-dataset:
	@echo "Refreshing vendored reference dataset..."
	curl -fsSL -o $(RESOURCES_DIR)/references.json.gz $(REFERENCES_URL)

# Quality.
fmt:
	@echo "Formatting..."
	gofmt -l -w .

vet:
	@echo "Vetting..."
	$(GO) vet ./...

lint:
	@echo "Linting..."
	$(GOLANGCI_LINT) run ./...

test:
	@echo "Running unit tests..."
	$(GO) test -race -cover ./...

test-e2e:
	@echo "Running E2E tests..."
	$(GO) test -tags=e2e ./e2e/...

ci: fmt vet lint test

# Docker.
docker-build:
	@echo "Building Docker image..."
	$(DOCKER_COMPOSE) build

docker-up:
	@echo "Starting stack..."
	$(DOCKER_COMPOSE) up -d --build

docker-down:
	@echo "Stopping stack..."
	$(DOCKER_COMPOSE) down

docker-logs:
	$(DOCKER_COMPOSE) logs -f $(SERVICE)

# Load testing (official k6 scripts, see ./loadtest/).
load-smoke:
	@echo "Running official k6 smoke test..."
	cd loadtest && $(DOCKER_COMPOSE) --profile smoke up --abort-on-container-exit

load-test:
	@echo "Running official k6 load test..."
	mkdir -p loadtest/test
	cd loadtest && $(DOCKER_COMPOSE) --profile test up --abort-on-container-exit
	@echo "results written to loadtest/test/results.json"

stress-test: load-test

load-data:
	@echo "Refreshing vendored k6 test payloads..."
	mkdir -p loadtest/fixtures
	curl -fsSL -o loadtest/fixtures/test-data.json $(TEST_DATA_URL)

# Module.
clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR) $(DATA_DIR) loadtest/test/results.json
