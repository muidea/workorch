SHELL := /usr/bin/env bash

GO ?= go
NPM ?= npm
APP := ./application/workorch
PKGS := ./...
GOFMT ?= gofmt
GOTEST_FLAGS ?=
# Per-package test deadline for the combined file-backed recovery matrices.
# This is harness configuration, never a workspace Runtime/repair budget.
GOTEST_TIMEOUT ?= 15m
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

.PHONY: help deps go-deps console-deps fmt fmt-check test test-race vet console-test console-build build clean run run-file verify-boundary-contract verify-runtime-positive-convergence-file verify-runtime-positive-convergence-live verify-runtime-restart-recovery-live verify-runtime-p6-scripted verify-runtime-c7-scripted verify-runtime-c7-fixture verify-runtime-bounded-terminal-live observe-runtime-liveness verify-service verify-service-file verify-workspace-template-soul verify-workspace-template-soul-file verify-llm verify-llm-file verify-contract-violation-recovery-e2e verify-service-restart-recoverable-e2e verify-runtime-profile-software-dev verify-software-dev-code-agent verify-software-dev-goal-dod-e2e verify-software-requirement-orchestration verify-software-requirement-orchestration-deployed verify-skillhub verify-skillhub-file verify-mcp-stdio verify-mcp-stdio-file verify-auto-governance-ticket-approval verify-workspace-file-diff-patch verify-workspace-web-search verify-workspace-web-search-e2e verify-workspace-projection-switch verify-run-resume-gap-confirmation-e2e verify-dual-workspace-resume-replan-e2e verify-approval-decision-resolution-e2e observe-runtime-closure verify-convergence-authority-dispatch-live verify-authority-live-e2e-closure verify-runtime-idle-baseline verify-cpu-io-observation-e2e verify-web-console-projection-visual verify-tui-web-projection-visual verify-tui-terminal-render verify-tui-long-output verify-runtime-implementation-work-readiness verify-e2e-template-contract verify-ci-mvp verify-ci-mvp-file verify-mvp verify-mvp-file verify-real-llm verify-real-runtime-closure verify-real-mcp-stdio verify-real-vector-memory verify-real-weather verify-weather-ticket-approval verify-weather-ticket-approval-mock verify-real-mvp verify-deployed-convergence-manifest-tests verify-deployed-convergence-ci-safe verify-deployed-convergence-release verify-deployed-full verify-deployed-full-file verify-e2e-full verify-managed-full verify-managed-service-smoke loadtest-governed loadtest-governed-real docker-build docker-run docker-run-file docker-verify docker-verify-file cli-build cli-test cli-clean verify-tui-agent verify-tui-workspace-switch-visual verify-tui-runtime-refresh-history-live verify-tui-terminal-render verify-tui-long-output release release-check install-latest ci

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
	@echo "  console-test - run Web console contract tests"
	@echo "  console-build - build the embedded workorch management console assets"
	@echo "  build      - build workorch into $(BUILD_OUT)"
	@echo "  clean      - remove workorch build artifacts"
	@echo "  run        - go run $(APP) as HTTP service"
	@echo "  run-file   - clean data, then run $(APP) as HTTP service with file-backed repository"
	@echo "  verify-boundary-contract - fail if generic runtime/Web code contains business-specific fallback logic"
	@echo "  verify-runtime-positive-convergence-file - deterministic Runtime positive-convergence fixtures"
	@echo "  verify-runtime-positive-convergence-live - deployed Runtime positive acceptance"
	@echo "  verify-runtime-restart-recovery-live - deployed SM-4 recovery fixture verification"
	@echo "  verify-runtime-p6-scripted - isolated P6 scripted mock-provider verification"
	@echo "  verify-runtime-c7-scripted - managed fresh-data dual-workspace Runtime C7 acceptance"
	@echo "  verify-runtime-bounded-terminal-live - bounded deployed dual-workspace terminal acceptance"
	@echo "  observe-runtime-liveness - deployed real-workspace Runtime liveness observation"
	@echo "  verify-service      - verify the deployed workorch --server HTTP service at WORKORCH_BASE_URL"
	@echo "  verify-service-file - alias of verify-service; e2e uses the deployed instance only"
	@echo "  verify-workspace-template-soul      - verify deployed Soul, Workspace Template, and template-based Workspace setup"
	@echo "  verify-workspace-template-soul-file - alias of verify-workspace-template-soul; e2e uses the deployed instance only"
	@echo "  verify-llm      - verify deployed LLM-driven execution using a local mock provider"
	@echo "  verify-llm-file - alias of verify-llm; e2e uses the deployed instance only"
	@echo "  verify-runtime-profile-software-dev - verify software-dev defaults, bootstrap and Runtime operations"
	@echo "  verify-software-dev-code-agent - verify software-dev typed operations and revision-bound replacement"
	@echo "  verify-software-dev-goal-dod-e2e - verify file-backed Goal/DoD Runtime convergence and restart"
	@echo "  verify-software-requirement-orchestration - verify software-dev activation, convergence and read-model boundaries"
	@echo "  verify-software-requirement-orchestration-deployed - compatibility alias of the current deterministic verifier"
	@echo "  verify-skillhub      - verify SkillHub discovery, selection and typed Runtime bootstrap"
	@echo "  verify-skillhub-file - alias of verify-skillhub"
	@echo "  verify-mcp-stdio      - verify deployed MCP stdio execution through workspace Runtime activation"
	@echo "  verify-mcp-stdio-file - alias of verify-mcp-stdio; e2e uses the deployed instance only"
	@echo "  verify-auto-governance-ticket-approval - verify auto governance ticket/approval closure"
	@echo "  verify-workspace-file-diff-patch - verify file/diff/patch capability closure"
	@echo "  verify-workspace-web-search - verify Web Search contract tests"
	@echo "  verify-workspace-web-search-e2e - compatibility target for deterministic Web Owner/Runtime integration"
	@echo "  verify-workspace-projection-switch - verify deployed workspace projection scope and switch refresh contract"
	@echo "  verify-run-resume-gap-confirmation-e2e - verify workspace activation and diagnostic gap compatibility"
	@echo "  verify-dual-workspace-resume-replan-e2e - legacy name; verify two workspace restart/resume intents remain isolated"
	@echo "  verify-approval-decision-resolution-e2e - verify owner resolution and Runtime continuation over typed events"
	@echo "  observe-runtime-closure - collect deployed runtime projection, SSE, heartbeat, and optional LLM cache observations"
	@echo "  verify-convergence-authority-dispatch-live - verify two-workspace convergence authority dispatch live observation"
	@echo "  verify-authority-live-e2e-closure - verify deployed owner command/readback/projection/admission report"
	@echo "  verify-runtime-idle-baseline - verify managed latest service idle CPU/IO and event IO pressure baseline"
	@echo "  verify-cpu-io-observation-e2e - verify S11.7 managed CPU/IO idle observation baseline"
	@echo "  verify-web-console-projection-visual - verify Web Console projection and heartbeat display contract"
	@echo "  verify-tui-web-projection-e2e - verify service projection, Web display, and TUI read-model rendering"
	@echo "  verify-contract-violation-recovery-e2e - verify typed rejection, bounded correction and fail-closed retry"
	@echo "  verify-service-restart-recoverable-e2e - verify file-backed aggregate recovery and idempotent re-drive"
	@echo "  verify-e2e-template-contract - verify all e2e workspace creation requests declare templateId"
	@echo "  verify-ci-mvp      - verify deployed service with CI-safe mock LLM/MCP checks"
	@echo "  verify-ci-mvp-file - alias of verify-ci-mvp; e2e uses the deployed instance only"
	@echo "  verify-mvp      - verify deployed service with local real LLM MVP checks"
	@echo "  verify-mvp-file - alias of verify-mvp; e2e uses the deployed instance only"
	@echo "  verify-real-llm - compatibility entry for the two-workspace real Runtime closure gate"
	@echo "  verify-real-runtime-closure - verify fresh real-provider Runtime authority, audit, receipts and terminal projection"
	@echo "  verify-real-mcp-stdio - compatibility entry for the managed current-contract MCP stdio gate"
	@echo "  verify-real-vector-memory - compatibility entry for Memory Owner recall/bootstrap verification"
	@echo "  verify-real-weather - verify real LLM + MCP stdio current weather query for Zhejiang Hangzhou"
	@echo "  verify-weather-ticket-approval - verify weather profile/MCP governance ownership contracts"
	@echo "  verify-weather-ticket-approval-mock - compatibility alias of weather governance ownership verification"
	@echo "  verify-real-mvp - verify a running workorch --server with real LLM, MCP stdio, and vector Memory checks"
	@echo "  verify-deployed-convergence-manifest-tests - verify manifest schema/ref/assertion fail-closed behavior"
	@echo "  verify-deployed-convergence-ci-safe - require producer-registered manifests for all CI-safe DC scenarios"
	@echo "  verify-deployed-convergence-release - fail closed on missing manifests; historical verifiers are not producers"
	@echo "  verify-deployed-full - run deployed checks, then the full producer-registered manifest gate"
	@echo "  verify-e2e-full - verify all currently tracked deployed/local e2e closure scripts"
	@echo "  verify-managed-full - fmt/vet/test/build, then start and verify a managed local workorch instance"
	@echo "  verify-managed-service-smoke - start/verify/stop a managed local workorch instance twice"
	@echo "  loadtest-governed - run governed workspace concurrency load test with deterministic mock LLM"
	@echo "  loadtest-governed-real - run governed workspace concurrency load test with configured real LLM"
	@echo "  docker-build    - build the image with embedded Web console assets"
	@echo "  docker-run      - run workorch --server with ephemeral container-local file data"
	@echo "  docker-run-file - run workorch --server container with file-backed repository"
	@echo "  docker-verify      - verify image, console, activation API and ephemeral file data"
	@echo "  docker-verify-file - additionally verify mounted file data across container replacement"
	@echo "  cli-build  - build the workorch CLI binary into $(CLI_BUILD_OUT)"
	@echo "  cli-test   - run CLI tests"
	@echo "  cli-clean  - remove CLI build artifact"
	@echo "  verify-tui-agent - run TUI agent acceptance checks"
	@echo "  verify-tui-workspace-switch-visual - record and verify TUI workspace switch view snapshots"
	@echo "  verify-tui-runtime-refresh-history-live - TC10 live: server restart/SSE reconnect/runtime-view history artifacts"
	@echo "  verify-tui-terminal-render - verify real PTY TUI render against deployed service"
	@echo "  release     - require full deployed convergence evidence, then package into $(RELEASE_OUT)/"
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

.PHONY: go-test-timeout
go-test-timeout:
	@printf '%s\n' '$(GOTEST_TIMEOUT)'

test: go-deps
	$(GO) test -timeout=$(GOTEST_TIMEOUT) $(GOTEST_FLAGS) $(PKGS)

test-race: go-deps
	$(GO) test -race -timeout=$(GOTEST_TIMEOUT) $(GOTEST_FLAGS) $(PKGS)

vet: go-deps
	$(GO) vet $(PKGS)

console-test: console-deps
	@cd $(CONSOLE_WEB) && $(NPM) test

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

verify-runtime-positive-convergence-file:
	./scripts/verify-runtime-positive-convergence-file.sh

verify-runtime-positive-convergence-live:
	./scripts/verify-runtime-positive-convergence-live.sh

verify-runtime-restart-recovery-live:
	./scripts/verify-runtime-restart-recovery-live.sh

verify-runtime-p6-scripted:
	./scripts/verify-runtime-p6-scripted.sh

verify-runtime-c7-scripted:
	./scripts/verify-runtime-c7-scripted.sh

verify-runtime-c7-fixture:
	WORKORCH_LLM_VERIFY_RUN_RUNTIME_C7=1 WORKORCH_LLM_VERIFY_TIMEOUT_SECONDS=1200 ./scripts/verify-llm-workspace-execution.sh

verify-runtime-bounded-terminal-live:
	WORKORCH_E2E_MODE=bounded_terminal ./scripts/verify-runtime-positive-convergence-live.sh

observe-runtime-liveness:
	WORKORCH_E2E_MODE=liveness ./scripts/verify-runtime-positive-convergence-live.sh

verify-service:
	./scripts/verify-workorch-http.sh

verify-service-file: verify-service

# 仅本地 mock／持久 owner 合同测试；不连接部署实例或真实 provider。
.PHONY: verify-workspace-authoring-contract
verify-workspace-authoring-contract:
	python3 scripts/test-verify-authoring-mock.py
	python3 scripts/test-verify-authoring-mock-server.py
	python3 scripts/test-create-authoring-verification-workspace.py
	python3 scripts/test-verify-workspace-authoring-live.py
	go test ./internal/pkg/events -run '^TestOwnerEvent(ContractsAreCompleteAndUnique|CommandsHaveProductionSenderAndReceiver|MessageGraphsAreConcrete)$$' -count=1
	go test -race ./internal/modules/blocks/workspace/pkg/eventmapping -run '^TestAuthoring' -count=1 -timeout=3m
	go test ./internal/services/workorch -run '^TestWeb(ConsoleMutatingEndpointsStayWithinProductSurfaceContract|WorkspaceSetupRequiresTemplateAndAllowsDefaultDirectory)$$' -count=1
	go test ./internal/modules/application/workorchd/biz/appservice -run '^TestCLIDispatcherRejectsRetiredDefinitionWritesWithoutOwner$$' -count=1
	go test ./internal/modules/application/workorchd/service/server -run '^TestRetired(DefinitionWriteRoutesDoNotCallApplicationPorts|DraftRoutesHaveNoSessionSideEffects)$$' -count=1 -timeout=3m
	go test ./internal/pkg/workorchclient -run '^TestDefinitionAuthoringClient' -count=1
	go test ./internal/modules/application/workorchd/biz -run '^Test(SaveDefinitionAuthoringView|AuthoringApplicationRejectsMismatchedOwnerResults)' -count=1
	go test ./internal/modules/blocks/workspace/pkg/models -run '^Test(DefinitionAuthoringSaveReceipt|ConfirmedAuthoringAdmissionSource)' -count=1
	go test -race ./internal/modules/blocks/workspace/biz -run '^TestAuthoring(QueryAndAdmissionRejectSourceTampering|QueryRejectsCorruptArchivedSource)$$' -count=1 -timeout=3m
	go test ./internal/modules/blocks/workspace/biz -run '^Test(Authoring(SaveReceiptRejectsForgedIdentityAndSurvivesABA|SaveReceiptLimitAndConcurrentCAS|ProcessConfirmationRecovery|ProcessDecisionAdmissionRecovery|ProcessDerivationPermitRecovery|FileBudgetExhaustionSurvivesRevisions|FileSpecificationBudgetExhaustionSurvivesRevisions|SpecificationConcurrentReserveUsesOneFreshPermit|RevisionFencesHistoricalSpecificationCompletion|RevisionFencesHistoricalDerivationCompletion)|DefinitionAuthoringApplication(CreationRecoversFileOwner|TemplateDirectoryReplayAfterReopen))$$' -count=1
	go test ./internal/modules/application/workorchd/biz -run '^TestSoftwareDevRestartCompilesReviewedDefaultForFullSizeDefinition$$' -count=1
	go test -race ./internal/modules/application/workorchd/biz -run '^TestSoftwareDevRestartCompilesReviewedDefaultForFullSizeDefinition$$' -count=1 -timeout=3m
	go test ./internal/modules/application/workorchd/biz -run '^TestAuthoringHTTP(FileBackedCompilationConfirmationAndRecovery|RealBackgroundRoutineFileRecovery|ConcurrentCompileUsesOnePhysicalDerivation|CompileTasksCompeteForFreshPermit)$$' -count=1

verify-workspace-template-soul:
	./scripts/verify-workspace-template-soul.sh

verify-workspace-template-soul-file: verify-workspace-template-soul

verify-llm:
	WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh

verify-llm-file: verify-llm

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

verify-software-requirement-orchestration-deployed: verify-software-requirement-orchestration

verify-skillhub:
	WORKORCH_SKILLHUB_VERIFY_MOCK_ADDR=127.0.0.1:18191 ./scripts/verify-skillhub-workspace-execution.sh

verify-skillhub-file: verify-skillhub

verify-mcp-stdio:
	WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-mcp-stdio-workspace-execution.sh

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

verify-convergence-authority-dispatch-live:
	WORKORCH_OBSERVE_DURATION_SECONDS=$${WORKORCH_OBSERVE_DURATION_SECONDS:-60} \
	WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=$${WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT:-2} \
	WORKORCH_OBSERVE_MIN_WORKSPACE_COUNT=$${WORKORCH_OBSERVE_MIN_WORKSPACE_COUNT:-2} \
	WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=$${WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE:-1} \
	WORKORCH_OBSERVE_REQUIRE_WORKSPACE_PROGRESS=$${WORKORCH_OBSERVE_REQUIRE_WORKSPACE_PROGRESS:-1} \
	WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=$${WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE:-0} \
	WORKORCH_OBSERVE_MAX_PENDING_REBUILDS=$${WORKORCH_OBSERVE_MAX_PENDING_REBUILDS:-10} \
	WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA=$${WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA:-0} \
	./scripts/observe-workorch-runtime-closure.sh

verify-authority-live-e2e-closure:
	./scripts/verify-authority-live-e2e-closure.sh

verify-runtime-idle-baseline:
	WORKORCH_MANAGED_VERIFY_TARGET=observe-runtime-closure \
	WORKORCH_MANAGED_PRECREATE_WORKSPACE_COUNT=$${WORKORCH_MANAGED_PRECREATE_WORKSPACE_COUNT:-2} \
	WORKORCH_MANAGED_GO_TEST_PACKAGES="$${WORKORCH_MANAGED_GO_TEST_PACKAGES:-./internal/pkg/repository ./internal/pkg/repository/file ./internal/pkg/workorchrepo ./internal/modules/blocks/eventstream/... ./internal/modules/application/workorchd/service/server}" \
	WORKORCH_OBSERVE_DURATION_SECONDS=$${WORKORCH_OBSERVE_DURATION_SECONDS:-20} \
	WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=0 \
	WORKORCH_OBSERVE_ENABLE_PPROF_CHECK=0 \
	WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 \
	WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=1 \
	WORKORCH_OBSERVE_MAX_PENDING_REBUILDS=0 \
	WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA=0 \
	WORKORCH_OBSERVE_MAX_PROCESS_CPU_PERCENT=$${WORKORCH_OBSERVE_MAX_PROCESS_CPU_PERCENT:-20} \
	WORKORCH_OBSERVE_MAX_PROCESS_READ_BYTES_DELTA=$${WORKORCH_OBSERVE_MAX_PROCESS_READ_BYTES_DELTA:-0} \
	WORKORCH_OBSERVE_MAX_PROCESS_WRITE_BYTES_DELTA=$${WORKORCH_OBSERVE_MAX_PROCESS_WRITE_BYTES_DELTA:-65536} \
	./scripts/verify-workorch-managed-full.sh

verify-cpu-io-observation-e2e: verify-runtime-idle-baseline

verify-web-console-projection-visual: console-build
	./scripts/verify-web-console-projection-visual.sh

verify-e2e-template-contract:
	./scripts/verify-e2e-template-contract.sh

verify-runtime-implementation-work-readiness:
	go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/biz -run 'Test(RuntimeImplementationWork|ImplementationWorkScope)' -count=1

verify-ci-mvp: verify-runtime-implementation-work-readiness verify-service verify-workspace-template-soul verify-llm verify-mcp-stdio

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

DEPLOYED_CONVERGENCE_MANIFEST_ROOT ?= .ci-artifacts/deployed-convergence
DEPLOYED_CONVERGENCE_REPORT ?= .ci-artifacts/deployed-convergence-aggregate.json
DEPLOYED_CONVERGENCE_PROFILE ?= full
DEPLOYED_CONVERGENCE_PROFILE_ID ?=

verify-deployed-convergence-manifest-tests:
	python3 ./scripts/test_verify_deployed_convergence_manifest.py
	python3 ./scripts/test_release_convergence_gate.py

verify-deployed-convergence-ci-safe:
	python3 ./scripts/verify_deployed_convergence_manifest.py \
		--manifest-root "$(DEPLOYED_CONVERGENCE_MANIFEST_ROOT)" \
		--profile ci-safe \
		--output "$(DEPLOYED_CONVERGENCE_REPORT)"

verify-deployed-convergence-release:
	python3 ./scripts/verify_deployed_convergence_manifest.py \
		--manifest-root "$(DEPLOYED_CONVERGENCE_MANIFEST_ROOT)" \
		--profile "$(DEPLOYED_CONVERGENCE_PROFILE)" \
		$(if $(strip $(DEPLOYED_CONVERGENCE_PROFILE_ID)),--profile-id "$(DEPLOYED_CONVERGENCE_PROFILE_ID)") \
		--output "$(DEPLOYED_CONVERGENCE_REPORT)"

verify-deployed-full: verify-ci-mvp verify-skillhub verify-mvp verify-real-runtime-closure verify-real-mcp-stdio verify-real-vector-memory
	$(MAKE) verify-deployed-convergence-release

verify-tui-web-projection-e2e: verify-workspace-projection-switch verify-web-console-projection-visual verify-tui-agent verify-tui-workspace-switch-visual verify-tui-terminal-render verify-tui-long-output

verify-e2e-full: verify-e2e-template-contract verify-service verify-workspace-template-soul verify-llm verify-contract-violation-recovery-e2e verify-service-restart-recoverable-e2e verify-mcp-stdio verify-skillhub verify-auto-governance-ticket-approval verify-software-requirement-orchestration verify-workspace-file-diff-patch verify-workspace-web-search verify-workspace-web-search-e2e verify-run-resume-gap-confirmation-e2e verify-dual-workspace-resume-replan-e2e verify-approval-decision-resolution-e2e verify-tui-web-projection-e2e verify-cpu-io-observation-e2e verify-weather-ticket-approval-mock

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
	docker build --network="$${WORKORCH_DOCKER_BUILD_NETWORK:-default}" --build-arg GOPROXY="$${WORKORCH_DOCKER_GOPROXY:-$$(go env GOPROXY)}" -t workorch:local .

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
	WORKORCH_VERIFY_PORT=18081 WORKORCH_VERIFY_STORAGE_MODE=ephemeral ./scripts/verify-workorch-container.sh

docker-verify-file:
	WORKORCH_VERIFY_PORT=18083 WORKORCH_VERIFY_STORAGE_MODE=persistent ./scripts/verify-workorch-container.sh

RELEASE_OUT ?= ./dist
RELEASE_TAG ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
RELEASE_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

release: override DEPLOYED_CONVERGENCE_PROFILE := full
release: verify-deployed-convergence-release
	$(MAKE) go-deps console-build
	@rm -rf $(RELEASE_OUT)
	@mkdir -p $(RELEASE_OUT)
	@set -e; \
	for platform in $(RELEASE_PLATFORMS); do \
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

release-check: fmt-check vet test verify-boundary-contract cli-test console-test verify-deployed-convergence-manifest-tests
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

verify-tui-runtime-refresh-history-live:
	./scripts/verify-tui-runtime-refresh-history-live.sh

verify-tui-terminal-render: cli-build
	python3 ./scripts/verify-tui-terminal-render-smoke.py

verify-tui-long-output: cli-build
	./scripts/verify-tui-long-output-e2e.sh

ci: fmt-check vet test verify-boundary-contract cli-test console-test verify-deployed-convergence-manifest-tests build
