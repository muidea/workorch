SHELL := /usr/bin/env bash

GO ?= go
NPM ?= npm
APP := ./application/workorch
PKGS := ./...
GOFMT ?= gofmt
GOTEST_FLAGS ?=
GO_MOD_DOWNLOAD_FLAGS ?=
BUILD_OUT ?= ./bin/workorch
CLI_BUILD_OUT ?= $(BUILD_OUT)
CLI_APP := $(APP)
CONSOLE_WEB := ./web/workorch-console
CONSOLE_PACKAGE_JSON := $(CONSOLE_WEB)/package.json
CONSOLE_PACKAGE_LOCK := $(CONSOLE_WEB)/package-lock.json
CONSOLE_NODE_MODULES := $(CONSOLE_WEB)/node_modules
CONSOLE_DEPS_STAMP := $(CONSOLE_NODE_MODULES)/.workorch-deps-stamp
NPM_CI_FLAGS ?=
NPM_INSTALL_FLAGS ?=

.PHONY: help deps go-deps console-deps fmt fmt-check test test-race vet console-build build clean run run-file verify-boundary-contract verify-service verify-service-file verify-workspace-template-soul verify-workspace-template-soul-file verify-llm verify-llm-file verify-incremental-artifact-e2e verify-contract-violation-recovery-e2e verify-service-restart-recoverable-e2e verify-runtime-profile-software-dev verify-software-dev-code-agent verify-software-dev-goal-dod-e2e verify-software-requirement-orchestration verify-software-requirement-orchestration-deployed verify-skillhub verify-skillhub-file verify-mcp-stdio verify-mcp-stdio-file verify-auto-governance-ticket-approval verify-workspace-file-diff-patch verify-workspace-web-search verify-workspace-web-search-e2e verify-workspace-projection-switch verify-run-resume-gap-confirmation-e2e verify-dual-workspace-resume-replan-e2e verify-approval-decision-resolution-e2e observe-runtime-closure verify-runtime-idle-baseline verify-web-console-projection-visual verify-e2e-template-contract verify-ci-mvp verify-ci-mvp-file verify-mvp verify-mvp-file verify-real-llm verify-real-runtime-closure verify-real-mcp-stdio verify-real-vector-memory verify-real-weather verify-weather-ticket-approval verify-weather-ticket-approval-mock verify-real-mvp verify-deployed-full verify-deployed-full-file verify-e2e-full verify-managed-full verify-managed-service-smoke loadtest-governed loadtest-governed-real docker-build docker-run docker-run-file docker-verify docker-verify-file cli-build cli-test cli-clean verify-tui-agent verify-tui-workspace-switch-visual verify-tui-terminal-render verify-tui-long-output release release-check install-latest ci

help:
	@echo "Targets:"
	@echo "  fmt        - gofmt -w + go fmt for project sources"
	@echo "  fmt-check  - fail if any Go file needs gofmt"
	@echo "  deps       - download Go modules and install Web console dependencies"
	@echo "  go-deps    - download Go modules declared by go.mod/go.sum"
	@echo "  console-deps - install Web console dependencies from package-lock.json"
	@echo "  test       - go test $(PKGS)"
	@echo "  test-race  - go test -race $(PKGS)"
	@echo "  vet        - go vet $(PKGS)"
	@echo "  console-build - build the embedded workorch management console assets"
	@echo "  build      - build workorch into $(BUILD_OUT)"
	@echo "  clean      - remove workorch build artifacts"
	@echo "  run        - go run $(APP) as HTTP service"
	@echo "  run-file   - clean data, then run $(APP) as HTTP service with file-backed repository"
	@echo "  verify-boundary-contract - fail if generic runtime/Web code contains business-specific fallback logic"
	@echo "  verify-service      - verify the deployed workorch --server HTTP service at WORKORCH_BASE_URL"
	@echo "  verify-service-file - alias of verify-service; e2e uses the deployed instance only"
	@echo "  verify-workspace-template-soul      - verify deployed Soul, Workspace Template, and template-based Workspace setup"
	@echo "  verify-workspace-template-soul-file - alias of verify-workspace-template-soul; e2e uses the deployed instance only"
	@echo "  verify-llm      - verify deployed LLM-driven execution using a local mock provider"
	@echo "  verify-llm-file - alias of verify-llm; e2e uses the deployed instance only"
	@echo "  verify-runtime-profile-software-dev - verify software-dev profile mapping for a persisted workspace/run"
	@echo "  verify-software-dev-code-agent - verify software-dev Code Agent closure contract"
	@echo "  verify-software-dev-goal-dod-e2e - verify deployed software-dev Goal/DoD auto repair and repair-escalation loop"
	@echo "  verify-software-requirement-orchestration - verify SRD-10~34 orchestration closure"
	@echo "  verify-software-requirement-orchestration-deployed - verify SRD-10~34 closure with deployed e2e enabled"
	@echo "  verify-skillhub      - verify deployed SkillHub global/workspace Skill auto-selection"
	@echo "  verify-skillhub-file - alias of verify-skillhub; e2e uses the deployed instance only"
	@echo "  verify-mcp-stdio      - verify deployed MCP stdio execution through LLM graph"
	@echo "  verify-mcp-stdio-file - alias of verify-mcp-stdio; e2e uses the deployed instance only"
	@echo "  verify-auto-governance-ticket-approval - verify auto governance ticket/approval closure"
	@echo "  verify-workspace-file-diff-patch - verify file/diff/patch capability closure"
	@echo "  verify-workspace-web-search - verify Web Search contract tests"
	@echo "  verify-workspace-web-search-e2e - verify deployed Web Search source traceability"
	@echo "  verify-workspace-projection-switch - verify deployed workspace projection scope and switch refresh contract"
	@echo "  verify-run-resume-gap-confirmation-e2e - verify deployed /run resume projection and gap baseline contract"
	@echo "  verify-dual-workspace-resume-replan-e2e - verify two workspace-scoped resume/replan commands do not block each other"
	@echo "  verify-approval-decision-resolution-e2e - verify deployed decision/approval resolution continuation"
	@echo "  observe-runtime-closure - collect deployed runtime projection, SSE, heartbeat, and optional LLM cache observations"
	@echo "  verify-runtime-idle-baseline - verify managed latest service idle CPU/IO and event IO pressure baseline"
	@echo "  verify-web-console-projection-visual - verify Web Console projection and heartbeat display contract"
	@echo "  verify-incremental-artifact-e2e - verify deployed neutral incremental artifact plan/chunk/apply lifecycle"
	@echo "  verify-contract-violation-recovery-e2e - verify deployed contract violation rejected diagnostic and bounded retry"
	@echo "  verify-service-restart-recoverable-e2e - verify managed service restart resets stale running work to manual activation"
	@echo "  verify-e2e-template-contract - verify all e2e workspace creation requests declare templateId"
	@echo "  verify-ci-mvp      - verify deployed service with CI-safe mock LLM/MCP checks"
	@echo "  verify-ci-mvp-file - alias of verify-ci-mvp; e2e uses the deployed instance only"
	@echo "  verify-mvp      - verify deployed service with local real LLM MVP checks"
	@echo "  verify-mvp-file - alias of verify-mvp; e2e uses the deployed instance only"
	@echo "  verify-real-llm - verify a running workorch --server against its configured real LLM provider"
	@echo "  verify-real-runtime-closure - verify real provider concurrency, projection observation, and optional cache baseline"
	@echo "  verify-real-mcp-stdio - verify a running workorch --server can execute MCP stdio through LLM graph"
	@echo "  verify-real-vector-memory - verify vector Memory recall on a running workorch --server"
	@echo "  verify-real-weather - verify real LLM + MCP stdio current weather query for Zhejiang Hangzhou"
	@echo "  verify-weather-ticket-approval - verify specified city weather governance with the configured real LLM"
	@echo "  verify-weather-ticket-approval-mock - CI-only deterministic mock LLM weather governance check"
	@echo "  verify-real-mvp - verify a running workorch --server with real LLM, MCP stdio, and vector Memory checks"
	@echo "  verify-deployed-full - verify deployed service with mock, SkillHub, and real LLM e2e checks"
	@echo "  verify-e2e-full - verify all currently tracked deployed/local e2e closure scripts"
	@echo "  verify-managed-full - fmt/vet/test/build, then start and verify a managed local workorch instance"
	@echo "  verify-managed-service-smoke - start/verify/stop a managed local workorch instance twice"
	@echo "  loadtest-governed - run governed workspace concurrency load test with deterministic mock LLM"
	@echo "  loadtest-governed-real - run governed workspace concurrency load test with configured real LLM"
	@echo "  docker-build    - docker build local workorch image"
	@echo "  docker-run      - run workorch --server container with in-memory repository"
	@echo "  docker-run-file - run workorch --server container with file-backed repository"
	@echo "  docker-verify      - container image packaging verification; not a deployed-instance e2e target"
	@echo "  docker-verify-file - container image packaging verification with file-backed container data"
	@echo "  cli-build  - build the workorch CLI binary into $(CLI_BUILD_OUT)"
	@echo "  cli-test   - run CLI tests"
	@echo "  cli-clean  - remove CLI build artifact"
	@echo "  verify-tui-agent - run TUI agent acceptance checks"
	@echo "  verify-tui-workspace-switch-visual - record and verify TUI workspace switch view snapshots"
	@echo "  verify-tui-terminal-render - verify real PTY TUI render against deployed service"
	@echo "  release     - build cross-platform release binaries into $(RELEASE_OUT)/"
	@echo "  release-check - fmt-check + vet + test + verify-boundary-contract + release"
	@echo "  install-latest - install latest release binary from GitHub via install script"
	@echo "  ci         - fmt-check + vet + test + build + file-backed service verification"

deps: go-deps console-deps

go-deps:
	@command -v $(GO) >/dev/null 2>&1 || { echo "missing Go compiler: $(GO)"; exit 1; }
	$(GO) mod download $(GO_MOD_DOWNLOAD_FLAGS)

console-deps: $(CONSOLE_DEPS_STAMP)

$(CONSOLE_DEPS_STAMP): $(CONSOLE_PACKAGE_JSON) $(CONSOLE_PACKAGE_LOCK)
	@command -v $(NPM) >/dev/null 2>&1 || { echo "missing npm executable: $(NPM)"; exit 1; }
	@cd $(CONSOLE_WEB) && \
	if [[ -f package-lock.json ]]; then \
		$(NPM) ci $(NPM_CI_FLAGS); \
	else \
		$(NPM) install $(NPM_INSTALL_FLAGS); \
	fi
	@touch $(CONSOLE_DEPS_STAMP)

fmt: go-deps
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './.workorch-data/*' -not -path './.workorch-workspaces/*' -not -path './.claude/*' -not -path './vendor/*' -not -path './web/workorch-console/node_modules/*')"; \
	if [[ -n "$$files" ]]; then \
		$(GOFMT) -w $$files; \
	fi
	$(GO) fmt $(PKGS)

fmt-check:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './.workorch-data/*' -not -path './.workorch-workspaces/*' -not -path './.claude/*' -not -path './vendor/*' -not -path './web/workorch-console/node_modules/*')"; \
	if [[ -z "$$files" ]]; then \
		exit 0; \
	fi; \
	fmt_out="$$($(GOFMT) -l $$files)"; \
	if [[ -n "$$fmt_out" ]]; then \
		echo "Files need gofmt:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi

test: go-deps
	$(GO) test $(GOTEST_FLAGS) $(PKGS)

test-race: go-deps
	$(GO) test -race $(GOTEST_FLAGS) $(PKGS)

vet: go-deps
	$(GO) vet $(PKGS)

console-build: console-deps
	@cd $(CONSOLE_WEB) && $(NPM) run build

build: go-deps console-build
	@mkdir -p $(dir $(BUILD_OUT))
	$(GO) build -o $(BUILD_OUT) $(APP)

clean:
	rm -rf ./bin ./.workorch-data ./workorch

run: go-deps
	$(GO) run $(APP) --server -config-dir config

run-file: go-deps
	rm -rf ./.workorch-data
	$(GO) run $(APP) --server -config-dir config/file

verify-boundary-contract:
	./scripts/verify-boundary-contract.sh

verify-service:
	./scripts/verify-workorch-http.sh

verify-service-file: verify-service

verify-workspace-template-soul:
	./scripts/verify-workspace-template-soul.sh

verify-workspace-template-soul-file: verify-workspace-template-soul

verify-llm:
	WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh

verify-llm-file: verify-llm

verify-incremental-artifact-e2e:
	WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_MOCK_ADDR=127.0.0.1:18187 ./scripts/verify-incremental-artifact-e2e.sh

verify-contract-violation-recovery-e2e:
	./scripts/verify-contract-violation-recovery-e2e.sh

verify-service-restart-recoverable-e2e:
	./scripts/verify-service-restart-recoverable-e2e.sh

verify-acceptance-slice-result-e2e:
	./scripts/verify-acceptance-slice-result-e2e.sh

verify-acceptance-slice-result-deployed-e2e:
	WORKORCH_ACCEPTANCE_SLICE_VERIFY_MOCK_ADDR=127.0.0.1:18193 ./scripts/verify-acceptance-slice-result-deployed-e2e.sh

verify-runtime-profile-software-dev:
	./scripts/verify-workorch-runtime-profile-software-dev.sh

verify-software-dev-code-agent:
	./scripts/verify-software-dev-code-agent.sh

verify-software-dev-goal-dod-e2e:
	./scripts/verify-software-dev-goal-dod-e2e.sh

verify-software-requirement-orchestration:
	./scripts/verify-software-requirement-development-orchestration.sh

verify-software-requirement-orchestration-deployed:
	WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 ./scripts/verify-software-requirement-development-orchestration.sh

verify-skillhub:
	WORKORCH_SKILLHUB_VERIFY_MOCK_ADDR=127.0.0.1:18191 ./scripts/verify-skillhub-workspace-execution.sh

verify-skillhub-file: verify-skillhub

verify-mcp-stdio:
	WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR=127.0.0.1:18185 ./scripts/verify-mcp-stdio-workspace-execution.sh

verify-mcp-stdio-file: verify-mcp-stdio

verify-auto-governance-ticket-approval:
	./scripts/verify-auto-governance-ticket-approval.sh

verify-workspace-file-diff-patch:
	./scripts/verify-workspace-file-diff-patch.sh

verify-workspace-web-search:
	./scripts/verify-workspace-web-search.sh

verify-workspace-web-search-e2e:
	./scripts/verify-workspace-web-search-e2e.sh

verify-workspace-projection-switch:
	./scripts/verify-workorch-projection-workspace-switch.sh

verify-run-resume-gap-confirmation-e2e:
	./scripts/verify-run-resume-gap-confirmation-e2e.sh

verify-dual-workspace-resume-replan-e2e:
	./scripts/verify-dual-workspace-resume-replan-e2e.sh

verify-approval-decision-resolution-e2e:
	./scripts/verify-approval-decision-resolution-e2e.sh

observe-runtime-closure:
	./scripts/observe-workorch-runtime-closure.sh

verify-runtime-idle-baseline:
	WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure \
	WORKORCH_MANAGED_PRECREATE_WORKSPACE_COUNT=$${WORKORCH_MANAGED_PRECREATE_WORKSPACE_COUNT:-2} \
	WORKORCH_MANAGED_GO_TEST_PACKAGES="$${WORKORCH_MANAGED_GO_TEST_PACKAGES:-./internal/pkg/repository ./internal/pkg/repository/file ./internal/pkg/workorchrepo ./internal/modules/blocks/pkg/runtimeevent ./internal/modules/application/workorchd/service/server}" \
	WORKORCH_OBSERVE_DURATION_SECONDS=$${WORKORCH_OBSERVE_DURATION_SECONDS:-20} \
	WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=0 \
	WORKORCH_OBSERVE_ENABLE_PPROF_CHECK=0 \
	WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 \
	WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=1 \
	WORKORCH_OBSERVE_MAX_PENDING_REBUILDS=0 \
	WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA=0 \
	WORKORCH_OBSERVE_MAX_EVENT_METADATA_RECORDS_SCANNED_DELTA=$${WORKORCH_OBSERVE_MAX_EVENT_METADATA_RECORDS_SCANNED_DELTA:-0} \
	WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATES_DELTA=$${WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATES_DELTA:-0} \
	WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATE_BYTES_DELTA=$${WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATE_BYTES_DELTA:-0} \
	WORKORCH_OBSERVE_MAX_RUNTIME_EVENT_PUBLISH_FAILURES_DELTA=$${WORKORCH_OBSERVE_MAX_RUNTIME_EVENT_PUBLISH_FAILURES_DELTA:-0} \
	WORKORCH_OBSERVE_MAX_PROCESS_CPU_PERCENT=$${WORKORCH_OBSERVE_MAX_PROCESS_CPU_PERCENT:-20} \
	WORKORCH_OBSERVE_MAX_PROCESS_READ_BYTES_DELTA=$${WORKORCH_OBSERVE_MAX_PROCESS_READ_BYTES_DELTA:-0} \
	WORKORCH_OBSERVE_MAX_PROCESS_WRITE_BYTES_DELTA=$${WORKORCH_OBSERVE_MAX_PROCESS_WRITE_BYTES_DELTA:-0} \
	./scripts/verify-workorch-managed-full.sh

verify-web-console-projection-visual: console-build
	./scripts/verify-web-console-projection-visual.sh

verify-e2e-template-contract:
	./scripts/verify-e2e-template-contract.sh

verify-ci-mvp: verify-service verify-workspace-template-soul verify-llm verify-mcp-stdio

verify-ci-mvp-file: verify-ci-mvp

verify-mvp: verify-service verify-real-llm verify-real-weather verify-weather-ticket-approval

verify-mvp-file: verify-mvp

verify-real-llm:
	./scripts/verify-real-llm-workspace-execution.sh

verify-real-runtime-closure:
	./scripts/verify-real-runtime-closure.sh

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

verify-deployed-full: verify-ci-mvp verify-skillhub verify-mvp verify-real-runtime-closure verify-real-mcp-stdio verify-real-vector-memory

verify-e2e-full: verify-e2e-template-contract verify-service verify-workspace-template-soul verify-llm verify-incremental-artifact-e2e verify-contract-violation-recovery-e2e verify-service-restart-recoverable-e2e verify-mcp-stdio verify-skillhub verify-auto-governance-ticket-approval verify-software-dev-code-agent verify-software-dev-goal-dod-e2e verify-software-requirement-orchestration-deployed verify-workspace-file-diff-patch verify-workspace-web-search verify-workspace-web-search-e2e verify-workspace-projection-switch verify-run-resume-gap-confirmation-e2e verify-dual-workspace-resume-replan-e2e verify-approval-decision-resolution-e2e verify-web-console-projection-visual verify-weather-ticket-approval-mock verify-tui-agent verify-tui-workspace-switch-visual verify-tui-terminal-render verify-tui-long-output

verify-managed-full:
	./scripts/verify-workorch-managed-full.sh

verify-managed-service-smoke:
	./scripts/verify-workorch-managed-harness-smoke.sh

verify-deployed-full-file: verify-deployed-full

loadtest-governed:
	./scripts/loadtest-governed-workspace-concurrency.sh

loadtest-governed-real:
	WORKORCH_LOAD_LLM_MODE=real ./scripts/loadtest-governed-workspace-concurrency.sh

docker-build:
	docker build -t workorch:local .

docker-run:
	docker run --rm -p 8080:8080 workorch:local

docker-run-file:
	mkdir -p ./.workorch-data
	chmod 0777 ./.workorch-data
	docker run --rm -p 8080:8080 \
		-v $(CURDIR)/config/container-file/application.toml:/etc/workorch/application.toml:ro \
		-v $(CURDIR)/.workorch-data:/var/lib/workorch \
		workorch:local

docker-verify:
	WORKORCH_VERIFY_PORT=18081 ./scripts/verify-workorch-container.sh

docker-verify-file:
	WORKORCH_VERIFY_PORT=18083 WORKORCH_VERIFY_BACKEND=file ./scripts/verify-workorch-container.sh

RELEASE_OUT ?= ./dist
RELEASE_TAG ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
RELEASE_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

release: go-deps console-build
	@rm -rf $(RELEASE_OUT)
	@mkdir -p $(RELEASE_OUT)
	@for platform in $(RELEASE_PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		out=$(RELEASE_OUT)/workorch-$(RELEASE_TAG)-$$os-$$arch; \
		echo "building $$platform -> $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -o $$out $(APP); \
		tar -czf $$out.tar.gz -C $(RELEASE_OUT) $$(basename $$out); \
		rm -f $$out; \
	done
	@cd $(RELEASE_OUT) && sha256sum *.tar.gz > checksums.txt
	@echo "release $(RELEASE_TAG) built into $(RELEASE_OUT)"

release-check: fmt-check vet test verify-boundary-contract
	$(MAKE) release

install-latest:
	./scripts/install-latest.sh

pre-commit: fmt fmt-check vet
	@echo "pre-commit checks passed"

cli-build: go-deps
	@mkdir -p $(dir $(CLI_BUILD_OUT))
	$(GO) build -o $(CLI_BUILD_OUT) $(CLI_APP)

cli-test:
	$(GO) test ./internal/services/workorch/... --count 1
	./scripts/verify-cli-architecture.sh

cli-clean:
	rm -f $(CLI_BUILD_OUT)

verify-cli:
	./scripts/verify-cli.sh $(CLI_BUILD_OUT)

verify-tui-agent:
	./scripts/verify-tui-agent.sh

verify-tui-workspace-switch-visual:
	./scripts/verify-tui-workspace-switch-visual.sh

verify-tui-terminal-render: cli-build
	python3 ./scripts/verify-tui-terminal-render-smoke.py

verify-tui-long-output: cli-build
	./scripts/verify-tui-long-output-e2e.sh

ci: fmt-check vet test verify-boundary-contract build cli-test
