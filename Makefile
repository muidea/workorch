SHELL := /usr/bin/env bash

GO ?= go
NPM ?= npm
APP := ./application/workorchd
PKGS := ./...
GOFMT ?= gofmt
GOTEST_FLAGS ?=
BUILD_OUT ?= ./bin/workorchd
CONSOLE_WEB := ./web/workorch-console

.PHONY: help fmt fmt-check test test-race vet console-build build clean run run-file verify-boundary-contract verify-service verify-service-file verify-llm verify-llm-file verify-skillhub verify-skillhub-file verify-mcp-stdio verify-mcp-stdio-file verify-ci-mvp verify-ci-mvp-file verify-mvp verify-mvp-file verify-real-llm verify-real-mcp-stdio verify-real-vector-memory verify-real-weather verify-weather-ticket-approval verify-weather-ticket-approval-mock verify-real-mvp verify-deployed-full verify-deployed-full-file loadtest-governed loadtest-governed-real docker-build docker-run docker-run-file docker-verify docker-verify-file ci

help:
	@echo "Targets:"
	@echo "  fmt        - gofmt -w + go fmt for project sources"
	@echo "  fmt-check  - fail if any Go file needs gofmt"
	@echo "  test       - go test $(PKGS)"
	@echo "  test-race  - go test -race $(PKGS)"
	@echo "  vet        - go vet $(PKGS)"
	@echo "  console-build - build the embedded workorch management console assets"
	@echo "  build      - go build -o $(BUILD_OUT) $(APP)"
	@echo "  clean      - remove build/demo artifacts"
	@echo "  run        - go run $(APP) as HTTP service"
	@echo "  run-file   - clean data, then run $(APP) as HTTP service with file-backed repository"
	@echo "  verify-boundary-contract - fail if generic runtime/Web code contains business-specific fallback logic"
	@echo "  verify-service      - verify the deployed workorchd HTTP service at WORKORCH_BASE_URL"
	@echo "  verify-service-file - alias of verify-service; e2e uses the deployed instance only"
	@echo "  verify-llm      - verify deployed LLM-driven execution using a local mock provider"
	@echo "  verify-llm-file - alias of verify-llm; e2e uses the deployed instance only"
	@echo "  verify-skillhub      - verify deployed SkillHub global/workspace Skill auto-selection"
	@echo "  verify-skillhub-file - alias of verify-skillhub; e2e uses the deployed instance only"
	@echo "  verify-mcp-stdio      - verify deployed MCP stdio execution through LLM graph"
	@echo "  verify-mcp-stdio-file - alias of verify-mcp-stdio; e2e uses the deployed instance only"
	@echo "  verify-ci-mvp      - verify deployed service with CI-safe mock LLM/MCP checks"
	@echo "  verify-ci-mvp-file - alias of verify-ci-mvp; e2e uses the deployed instance only"
	@echo "  verify-mvp      - verify deployed service with local real LLM MVP checks"
	@echo "  verify-mvp-file - alias of verify-mvp; e2e uses the deployed instance only"
	@echo "  verify-real-llm - verify a running workorchd against its configured real LLM provider"
	@echo "  verify-real-mcp-stdio - verify a running workorchd can execute MCP stdio through LLM graph"
	@echo "  verify-real-vector-memory - verify vector Memory recall on a running workorchd"
	@echo "  verify-real-weather - verify real LLM + MCP stdio current weather query for Zhejiang Hangzhou"
	@echo "  verify-weather-ticket-approval - verify specified city weather governance with the configured real LLM"
	@echo "  verify-weather-ticket-approval-mock - CI-only deterministic mock LLM weather governance check"
	@echo "  verify-real-mvp - verify a running workorchd with real LLM, MCP stdio, and vector Memory checks"
	@echo "  verify-deployed-full - verify deployed service with mock, SkillHub, and real LLM e2e checks"
	@echo "  loadtest-governed - run governed workspace concurrency load test with deterministic mock LLM"
	@echo "  loadtest-governed-real - run governed workspace concurrency load test with configured real LLM"
	@echo "  docker-build    - docker build local workorchd image"
	@echo "  docker-run      - run workorchd container with in-memory repository"
	@echo "  docker-run-file - run workorchd container with file-backed repository"
	@echo "  docker-verify      - container image packaging verification; not a deployed-instance e2e target"
	@echo "  docker-verify-file - container image packaging verification with file-backed container data"
	@echo "  ci         - fmt-check + vet + test + build + file-backed service verification"

fmt:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*' -not -path './web/workorch-console/node_modules/*')"; \
	if [[ -n "$$files" ]]; then \
		$(GOFMT) -w $$files; \
	fi
	$(GO) fmt $(PKGS)

fmt-check:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*' -not -path './web/workorch-console/node_modules/*')"; \
	if [[ -z "$$files" ]]; then \
		exit 0; \
	fi; \
	fmt_out="$$($(GOFMT) -l $$files)"; \
	if [[ -n "$$fmt_out" ]]; then \
		echo "Files need gofmt:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi

test:
	$(GO) test $(GOTEST_FLAGS) $(PKGS)

test-race:
	$(GO) test -race $(GOTEST_FLAGS) $(PKGS)

vet:
	$(GO) vet $(PKGS)

console-build:
	@cd $(CONSOLE_WEB) && \
	if [[ ! -d node_modules ]]; then $(NPM) install; fi && \
	$(NPM) run build

build: console-build
	@mkdir -p $(dir $(BUILD_OUT))
	$(GO) build -o $(BUILD_OUT) $(APP)

clean:
	rm -rf ./bin ./.workorch-data ./workorchd

run:
	$(GO) run $(APP) -config-dir config

run-file:
	rm -rf ./.workorch-data
	$(GO) run $(APP) -config-dir config/file

verify-boundary-contract:
	./scripts/verify-boundary-contract.sh

verify-service:
	./scripts/verify-workorchd-http.sh

verify-service-file: verify-service

verify-llm:
	WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh

verify-llm-file: verify-llm

verify-skillhub:
	WORKORCH_SKILLHUB_VERIFY_MOCK_ADDR=127.0.0.1:18191 ./scripts/verify-skillhub-workspace-execution.sh

verify-skillhub-file: verify-skillhub

verify-mcp-stdio:
	WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR=127.0.0.1:18185 ./scripts/verify-mcp-stdio-workspace-execution.sh

verify-mcp-stdio-file: verify-mcp-stdio

verify-ci-mvp: verify-service verify-llm verify-mcp-stdio

verify-ci-mvp-file: verify-ci-mvp

verify-mvp: verify-service verify-real-llm verify-real-weather verify-weather-ticket-approval

verify-mvp-file: verify-mvp

verify-real-llm:
	./scripts/verify-real-llm-workspace-execution.sh

verify-real-mcp-stdio:
	./scripts/verify-real-mcp-stdio-workspace-execution.sh

verify-real-vector-memory:
	./scripts/verify-real-vector-memory-recall.sh

verify-real-weather:
	./scripts/verify-real-weather-query.sh

verify-weather-ticket-approval:
	./scripts/verify-weather-ticket-approval-query.sh

verify-weather-ticket-approval-mock:
	WORKORCH_WEATHER_GOVERNANCE_LLM_MODE=mock ./scripts/verify-weather-ticket-approval-query.sh

verify-real-mvp: verify-real-llm verify-real-mcp-stdio verify-real-vector-memory

verify-deployed-full: verify-ci-mvp verify-skillhub verify-mvp verify-real-mcp-stdio verify-real-vector-memory

verify-deployed-full-file: verify-deployed-full

loadtest-governed:
	./scripts/loadtest-governed-workspace-concurrency.sh

loadtest-governed-real:
	WORKORCH_LOAD_LLM_MODE=real ./scripts/loadtest-governed-workspace-concurrency.sh

docker-build:
	docker build -t workorchd:local .

docker-run:
	docker run --rm -p 8080:8080 workorchd:local

docker-run-file:
	mkdir -p ./.workorch-data
	chmod 0777 ./.workorch-data
	docker run --rm -p 8080:8080 \
		-v $(CURDIR)/config/container-file/application.toml:/etc/workorch/application.toml:ro \
		-v $(CURDIR)/.workorch-data:/var/lib/workorch \
		workorchd:local

docker-verify:
	WORKORCH_VERIFY_PORT=18081 ./scripts/verify-workorchd-container.sh

docker-verify-file:
	WORKORCH_VERIFY_PORT=18083 WORKORCH_VERIFY_BACKEND=file ./scripts/verify-workorchd-container.sh

ci: fmt-check vet test verify-boundary-contract build
