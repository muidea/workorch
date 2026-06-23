# Runtime Contract Retry Batch 7 Regression Live Verification Audit

## Purpose

Batch 7 defines the final regression and live verification plan for runtime contract retry convergence. It proves that closed gates work together and that live workspace progress does not depend on workspace IDs, file names, programming languages, provider names, or template-specific rules in WorkOrch core.

This batch consumes all dimensions from `runtime-contract-retry-full-chain-audit-plan.md`.

## Scope

In scope:

- Unit regression list for every closed gate.
- E2E verification list for command, projection, retry, decision, approval, evidence, and coverage flows.
- Live verification checklist for resumed or fresh workspaces.
- CPU, memory, and IO observation during multi-workspace execution.
- Boundary search proving core remains business-neutral.
- TUI/Web projection-only verification.

Out of scope:

- Provider quota or billing diagnosis beyond bounded external-failure classification.
- Template-specific goal success criteria beyond what the template/Soul/profile declares.
- Manual acceptance of unverifiable outputs.

## Verification Packet

Use `P4-verification` from `runtime-contract-retry-full-chain-audit-plan.md`.

Required inputs:

- L0 closure plan.
- Closed Batch 3, 4, 5, and 6 closure records.
- Test command outputs.
- Selected e2e outputs.
- Bounded process stats: CPU, RSS, file IO deltas, event queue depth if available.
- Bounded live workspace summaries for current state, latest events, planned tasks, coverage, decisions/approvals, results, and artifacts.

Do not load full logs, full request bodies, full event streams, or full workspace trees into the verification packet.

## Evidence Used

| Evidence | Meaning |
| --- | --- |
| `ev:verify/ci/managed-mock/001` | CI runs boundary verification and a managed mock-provider deployed-instance target after fmt/console/vet/test/build. |
| `ev:verify/make/e2e-matrix/001` | Makefile exposes CI-safe, deployed, real-provider, TUI, projection, acceptance-slice, and loadtest verification targets. |
| `ev:verify/boundary-contract/001` | Boundary verifier fails reusable runtime layers that contain business-specific weather/software-dev/LSP fallback terms. |
| `ev:verify/unit/eventhub-lane/001` | EventHub tests cover workspace revisions and lane independence for owner writes/reads. |
| `ev:verify/unit/remote-projection-command/001` | Remote backend tests cover SSE/projection stream and unified resume/replan command metadata. |
| `ev:verify/e2e/projection-resume/001` | Deployed e2e scripts cover projection workspace switch and run resume gap confirmation. |
| `ev:verify/resource/real-runtime-load/001` | Real runtime closure verifier covers metrics preflight, scheduler/watchdog markers, governed loadtest, and resource observation. |
| `ev:verify/tui/projection-only/001` | TUI verification covers no direct client import, no fixed polling, event-driven refresh, visual workspace switch, and PTY render smoke. |
| `ev:verify/unit/runtime-regression-map/001` | Existing unit tests cover many runtime contract, retry, coverage, context, cache, result, artifact, and projection behaviors. |

## Regression Matrix

| Area | Required Verification |
| --- | --- |
| Contract enforcement | Controlled substitute scratch boundary, state-changing output gate, patch/apply safety, validation shell safety. |
| Retry correction | Violation-specific correction profile, forced narrow retry after repetition, budget escalation with targeted retry evidence. |
| Context/actionability | C4 availableTargetSnapshots, missing-snapshot classifier, snapshot compaction safety, stable prefix preservation. |
| Evidence/coverage | Accepted/rejected evidence boundary, coverage ledger movement, artifact completeness, projection completeness. |
| State loop/EventHub | Command acceptance, queued execution, async terminal events, no lost replan/resume/restart commands. |
| Projection/ViewModel | workorchd owns workspace status, planned tasks, coverage, decisions, approvals, results, artifacts, and prompt state. |
| Resource envelope | No repeated full scans or unbounded polling during steady multi-workspace execution. |

## Verification Layers

| Layer | Required Command / Artifact | Current Coverage | Batch 7 Status |
| --- | --- | --- | --- |
| Static boundary | `make verify-boundary-contract` | CI runs it; script checks reusable runtime/Web paths for domain/profile leakage. | available |
| Unit regression | `go test ./... -count=1` or targeted packages | Managed verifier runs full Go tests; test inventory covers runtime contracts, retry, coverage, context/cache, projection, and TUI backend behavior. | passed for current code closure packet |
| CI-safe deployed e2e | `make verify-ci-mvp-file` through managed verifier | CI runs service, workspace template/soul, mock LLM, and MCP stdio checks against a managed instance. | passed for current code closure packet |
| Full deployed e2e | `make verify-e2e-full` | Makefile includes broader mock/deployed/TUI/Web checks, but CI does not currently run the full set. | available; manual or separate pipeline |
| Real-provider runtime | `make verify-real-runtime-closure` | Verifier includes metrics preflight, loadtest, cache/resource observation, and real provider runtime checks. | available; requires external credentials/provider stability |
| TUI projection | `make verify-tui-agent`, `make verify-tui-workspace-switch-visual`, `make verify-tui-terminal-render` | Scripts enforce projection-only/no-polling constraints and visual/PTY behavior. | available; not part of CI-safe target |
| Resource envelope | `make loadtest-governed`, `make verify-real-runtime-closure` | Loadtest and real runtime verifier exist; CI-safe mock loadtest and real-runtime preflight have bounded output, but full real-provider resource behavior remains unverified. | partially executed |

## Audit Findings

### B7-VFY-001 CI-Safe Verification Is Managed But Narrow

- Claim: CI starts a managed WorkOrch instance and executes deterministic deployed-instance checks with mock providers.
- Expected behavior: CI should avoid real-provider false negatives while still proving boundary, build, unit, and CI-safe deployed behavior.
- Actual behavior: CI runs `make verify-boundary-contract`, then `verify-workorch-managed-full.sh` with `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file`; the managed script runs fmt-check, console-build, vet, full Go tests, build, starts the server, applies LLM config, and runs the selected target.
- Evidence: `ev:verify/ci/managed-mock/001`.
- Counter-evidence: CI does not run full deployed e2e, TUI visual/terminal checks, loadtest, real runtime closure, or real-provider verification.
- Deviation type: none; verification scope limitation.
- Owner: verification / CI.
- Closure batch: 7.
- Required code/doc/test action: Treat `verify-ci-mvp-file` as CI-safe baseline only; do not claim full runtime convergence from CI alone.

### B7-VFY-002 Full Verification Targets Exist But Are Tiered

- Claim: The repository already exposes broad verification targets for deployed, real-provider, TUI, Web, projection, governance, evidence, and resource scenarios.
- Expected behavior: Batch 7 should select the correct tier for the closure being verified instead of requiring every expensive/credentialed check in CI.
- Actual behavior: Makefile has `verify-e2e-full`, `verify-managed-full`, `verify-real-runtime-closure`, `verify-real-*`, TUI checks, projection switch, run resume gap confirmation, acceptance slice result, contract violation recovery, and governed loadtest targets.
- Evidence: `ev:verify/make/e2e-matrix/001`.
- Counter-evidence: Not all exposed targets are currently in CI; some require deployed service, interactive/PTY support, real provider credentials, or longer runtime windows.
- Deviation type: none; execution planning requirement.
- Owner: verification.
- Closure batch: 7.
- Required code/doc/test action: Gate closure records must name the exact tier run and attach bounded outputs.

### B7-VFY-003 Boundary Verification Is Present And Business-Neutral

- Claim: There is a reusable runtime boundary verifier for domain/profile leakage.
- Expected behavior: Boundary search should catch concrete weather/software-dev/LSP profile logic in reusable runtime layers while allowing tests/docs/e2e/defaults to contain concrete fixtures.
- Actual behavior: `verify-boundary-contract.sh` scans runtime/Web paths and fails on weather, software-dev bias, and LSP profile-specific terms outside allowed fixture/test locations.
- Evidence: `ev:verify/boundary-contract/001`.
- Counter-evidence: The verifier is pattern-based and cannot prove all possible semantic leakage; review still needs skill boundary judgment for new code.
- Deviation type: none.
- Owner: verification / core boundary.
- Closure batch: 7.
- Required code/doc/test action: Run this after every core/template boundary-sensitive code closure.

### B7-VFY-004 EventHub And Command Flow Have Unit Coverage

- Claim: Unit tests cover key EventHub lane and command-flow contracts that previously caused run/replan/projection issues.
- Expected behavior: Owner writes/reads and orchestrator lanes should not block unrelated scoped commands; remote resume/replan should use workspace-scoped unified commands; projection stream must request projection view.
- Actual behavior: EventHub tests cover workspace-scoped revisions and lane independence; remote backend tests cover SSE, projection stream `view=projection`, workspace-scoped status, resume metadata, request IDs, and replan intent.
- Evidence: `ev:verify/unit/eventhub-lane/001`, `ev:verify/unit/remote-projection-command/001`.
- Counter-evidence: Unit tests do not prove live multi-workspace provider execution; that belongs to e2e/loadtest.
- Deviation type: none.
- Owner: core EventHub / client backend.
- Closure batch: 7.
- Required code/doc/test action: Keep these targeted tests in the minimum regression set for run command and projection-flow changes.

### B7-VFY-005 Projection, TUI, And Resource Checks Exist But Are Not All CI-Safe

- Claim: The repository has e2e and smoke checks for projection switch, run resume gap confirmation, TUI projection-only behavior, PTY rendering, and runtime resource envelope.
- Expected behavior: Batch 7 verification should run the checks that match the closure risk: projection/TUI changes require TUI/projection checks, runtime loop changes require load/resource checks, and real-provider convergence requires real runtime closure.
- Actual behavior: Projection and resume e2e scripts exist; TUI scripts enforce no polling/direct client and visual/terminal behavior; real runtime closure verifier checks metrics, scheduler/watchdog markers, loadtest, and resource/cache observations.
- Evidence: `ev:verify/e2e/projection-resume/001`, `ev:verify/tui/projection-only/001`, `ev:verify/resource/real-runtime-load/001`.
- Counter-evidence before closure records: this audit finding initially only proved the commands existed; their pass/fail status was unknown until bounded outputs were attached below.
- Deviation type: verification execution pending.
- Owner: verification / projection / TUI / runtime.
- Closure batch: 7.
- Required code/doc/test action: After code closure, run the relevant tier and record bounded outputs in Batch 7 closure record.
- Current status: partially executed for the current closure packet. CI-safe managed deployed e2e, managed mock resource loadtest, live projection observation, and real-runtime preflight have bounded outputs in the closure records below. Full real-provider execution, positive live goal convergence, and optional TUI visual/terminal tiers remain separate verification gates.

### B7-VFY-006 Unit Regression Inventory Is Broad But Gate Mapping Is Still Required

- Claim: Existing tests cover many relevant mechanisms, but Batch 7 still needs per-gate mapping after actual code closure.
- Expected behavior: Every closed gate should cite one or more tests that fail before and pass after the change, or explicitly justify why only e2e can prove it.
- Actual behavior: Test inventory includes contract violation retry, state-changing repair contract preservation, coverage work items, rejected results, graph follow-up, patch/apply safety, prompt cache/stable prefix, snapshot safety, projection, workspace view store, and run status projection tests.
- Evidence: `ev:verify/unit/runtime-regression-map/001`.
- Counter-evidence: Inventory breadth does not prove the specific future code closure gate is covered.
- Deviation type: gate mapping pending.
- Owner: verification / target code owner.
- Closure batch: 7.
- Required code/doc/test action: Every closure record must include a gate-to-test mapping and bounded command output.
- Current status: partially closed for the current closure packet. Batch 3-6 and supplemental Batch 1 records include explicit implementation refs and bounded unit/package command outputs; Batch 7 still requires fresh bounded output for any later code closure packet and cannot be considered globally complete for future changes.

## Suggested Commands

Unit and package tests:

```bash
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/blocks/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/pkg/... -count=1
go test ./... -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|main\\.go|main_test\\.go|LSP|gateway|provider gateway|ws-[0-9a-f]" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "stateChangingOutputRequired|repairObjective|providerTimeoutStateChangingRetry|assessmentOnly|runtimeRepairFollowUp|currentProgressDigest|memoryRecall|Cache-Control|cache_control" internal/modules/blocks/llm internal/modules/kernel
rg -n "definitionChangeImpact|coverageTargetSet|repairActionPackage|processEvidenceTimeline|requiredOutput|blocked_on_decision|contextGate" internal/modules/kernel internal/modules/blocks/llm
```

Evidence reference integrity:

```bash
refs=$(mktemp)
defs=$(mktemp)
rg -o 'e[v]:[A-Za-z0-9_./-]+' docs/70-roadmap/active/runtime-contract-retry-*.md | awk -F'e[v]:' '{print "e" "v" ":" $2}' | sort -u > "$refs"
rg -n '^### e[v]:' docs/70-roadmap/active/runtime-contract-retry-evidence-index.md | sed 's/^.*### //' | sort -u > "$defs"
comm -23 "$refs" "$defs"
```

## Live Verification Checklist

For each workspace under verification, record only bounded summaries:

1. Workspace id, run id, template/profile, current state, and last event time.
2. Current Goal/DoD revision and open coverage work items.
3. Planned task list: planned, dispatched, running, completed, rejected, retrying, blocked.
4. Latest retry or decision state with violation type and target scope.
5. Accepted evidence refs and rejected evidence refs.
6. Results and artifacts with state, category, and evidence refs.
7. Whether the latest LLM request contains current Goal/DoD, current-action, available snapshots, rejected evidence, and unresolved coverage.
8. Whether TUI/Web show the same projection state without client-side business inference.
9. CPU, memory, and IO observation window.

## Closure Gates

### B7-G1 Unit Regression Coverage

- Chain node: closed batch gates.
- Deviation type: new only if a gate lacks a failing regression.
- Evidence refs: `ev:verify/unit/runtime-regression-map/001`, closure records from Batch 3 to Batch 6.
- Owner: core runtime / projection / client depending on gate.
- Expected behavior: Every code closure gate has at least one regression test or explicit documented reason why only e2e can cover it.
- Actual behavior: Broad unit inventory exists, and the current closure packet records bounded unit/package outputs for Batch 3-6 plus supplemental Batch 1. Future code closure packets still need fresh gate-to-test mapping.
- Proposed change: Add missing regression tests before declaring any future closure complete.
- Why this is not a business-specific core rule: Tests assert generic contracts and structured metadata.
- Required tests: Listed per closed gate.

### B7-G2 E2E Coverage

- Chain node: service command, EventHub, runtime, projection, TUI/Web.
- Deviation type: new only if full-chain behavior lacks e2e coverage.
- Evidence refs: `ev:verify/ci/managed-mock/001`, `ev:verify/make/e2e-matrix/001`, `ev:verify/e2e/projection-resume/001`, `ev:verify/tui/projection-only/001`, bounded e2e outputs recorded below for the current closure packet.
- Owner: workorchd service / TUI/Web integration.
- Expected behavior: E2E covers command submission, replan/resume/restart, retry, decision/approval, projection stream, and final evidence/coverage result.
- Actual behavior: CI-safe and full e2e targets exist; CI-safe managed mock tiers have bounded passing output for this closure packet, while full real-provider and optional TUI visual/terminal tiers remain outside the CI-safe baseline.
- Proposed change: Add or update e2e scripts only after future batch gates identify new required behavior.
- Why this is not a business-specific core rule: E2E should exercise generic workspace execution flow and template-bound fixtures separately.
- Required tests: Selected CI-safe mock-provider e2e plus optional local real-provider verification.

### B7-G3 Live Verification Without Core Special Cases

- Chain node: resumed or fresh live workspace execution.
- Deviation type: new only if live success depends on core special cases.
- Evidence refs: `ev:verify/boundary-contract/001`, bounded live workspace summaries recorded below for projection/display-health.
- Owner: core/template boundary.
- Expected behavior: Live workspaces progress through declared template/Soul/profile policy while core remains business-neutral.
- Actual behavior: Boundary verifier exists; live projection/display-health observation has bounded output for the current deployed service, and the observer now has an explicit read-only goal-convergence gate. Positive live goal convergence remains unproven.
- Proposed change: Use `WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1` for live convergence closure; if future positive verification fails, map the structured failure back to a batch gate before changing code.
- Why this is not a business-specific core rule: Live workspace IDs are samples only.
- Required tests: Boundary search plus bounded live verification summaries.

### B7-G4 Resource Envelope

- Chain node: runtime loop, projection, request assembly, TUI/Web refresh.
- Deviation type: resource overuse if CPU/IO remain high during steady execution.
- Evidence refs: `ev:verify/resource/real-runtime-load/001`, bounded mock loadtest and real-runtime preflight stats recorded below.
- Owner: core runtime / projection / service clients.
- Expected behavior: Multi-workspace execution does not cause repeated full scans, unbounded polling, or growing goroutine/subscription leaks.
- Actual behavior: Resource/load verifiers exist; managed mock loadtest and real-runtime preflight have bounded passing output for this closure packet. Full real-provider resource execution remains unverified.
- Proposed change: Fix only measured hot paths from future bounded observations; do not compensate with larger timeouts or broader watchdog loops.
- Why this is not a business-specific core rule: Resource envelope applies to all templates.
- Required tests: Package tests for changed code plus live CPU/RSS/IO observation.

## Closure Records

### 2026-06-21 B7-G1 Unit / Boundary / Evidence Verification

- Closed gate: `B7-G1` for the current Batch 2-6 code closure packet.
- Scope:
  - Unit/package regression after planned-task projection, runtime contract enforcement, retry correction, and context/actionability changes.
  - Core/template boundary search for new production code.
  - Runtime retry audit evidence-reference integrity.
- Commands run:
  - `go test ./internal/modules/application/workorchd/biz/appservice/... -count=1`
  - `go test ./internal/modules/application/workorchd/service/server -count=1`

### 2026-06-22 B7-G3 Software-Dev LSP Tool Surface Closure

- Closed gate: focused `B7-G3` profile capability binding defect exposed by live `ws-4e83810542ac4d65`.
- Evidence refs:
  - `ev:code/software-dev-lsp-tool-surface/20260622-019`
  - `ev:code/bootstrap-system-defaults/20260622-020`
- Observed blocker:
  - Latest active run failed at `mcp/tool_call` with `mcp_capability_denied` because `lsp_status` was not in the `mcp-lsp-bridge` `allowedTools` whitelist.
- Expected behavior:
  - If a software-dev workspace profile exposes LSP bridge as an optional semantic capability, its default MCP server whitelist and capability catalog must include the declared profile tool surface.
  - Core runtime must keep enforcing structured MCP `allowedTools`; it must not special-case LSP or any specific server.
- Code closure:
  - Added `lsp_status`, `lsp_shutdown`, and `lsp_repair` to `mcp-lsp-bridge` default `allowedTools`.
  - Added corresponding capability catalog entries.
  - Added those capability IDs to `software-dev.contextHints.semanticCodeNavigation.defaultCapabilityIds`.
  - Added them to `software-dev-code-semantics-guidance.preferredTools`.
  - Added `lsp_status` to software-dev planning Soul capability refs; execution Soul remains wildcard-capable.
  - Started the `kernel/system` defaults module in bootstrap runtime construction so the updated defaults are upserted into selected backend repositories for embedded/test/server sessions, not only the framework main path.
  - Updated workspace template owner-port tests to expect built-in templates after system defaults startup while keeping create/read/update/delete assertions scoped to the test template.
  - Updated software-dev profile documentation and defaults tests.
- Boundary result:
  - This is template/profile capability data. No kernel/orchestrator logic was changed, and no live workspace ID, target filename, language, or tool-specific bypass was added to core.
- Required verification:
  - `go test ./internal/pkg/workorchdefaults -count=1`
  - `go test ./internal/modules/application/workorchd/biz/bootstrap -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/... -count=1`
  - `./scripts/verify-boundary-contract.sh`
  - Fresh deployed live verification after the service process actually runs the rebuilt binary and the affected workspace is explicitly replanned/resumed.
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`
  - `go test ./internal/modules/blocks/llm/biz -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
  - `go test ./... -count=1 -p 1`
  - `go test ./... -count=1`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md`: `evidence refs consistent`.
  - Boundary search:
    - `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg -g '*.go'`
    - `rg -n "sourceOutputState|contract_violation_retry|retry_exhausted|availableTargetSnapshots|available_snapshot_ignored|validationScratchDir" internal/modules/application/workorchd/biz/appservice internal/modules/kernel internal/modules/blocks/llm internal/pkg -g '*.go'`
- Result:
  - Unit/package regression passed, including default package parallelism and serial `-p 1` full repository runs.
  - Evidence refs are fully defined in `runtime-contract-retry-evidence-index.md`.
  - New production-code boundary hits are neutral runtime/projection contract fields: `validationScratchDir`, `availableTargetSnapshots`, `available_snapshot_ignored`, `contract_violation_retry`, `sourceOutputState`, and `retry_exhausted`.
  - Existing concrete terms found by broad boundary search are in tests, default template/profile fixtures, MCP server/config vocabulary, or generic `server` configuration fields; no new branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, LSP, concrete filenames, or business-specific text.
- Not closed by this record:
  - `B7-G2` deployed e2e.
  - `B7-G3` live workspace verification.
  - `B7-G4` resource-envelope observation.

### 2026-06-22 B7-G3 Stale Patch BaseHash Refresh Repair Closure

- Closed gate: focused `B7-G3` live runtime blocker exposed by `ws-7ced70e98ea50c43`.
- Evidence refs:
  - `ev:code/patch-basehash-refresh-repair/20260622-021`
- Observed blocker:
  - Latest active run blocked on a `patch/apply` step with `failureClass=patch_apply_failed`, `failureSignature` containing `patch basehash mismatch`, and escalation reason `noCoverageDeltaRounds 4/3`.
  - The patch carried a stale `baseHash` after earlier graph work had already changed the same target, but the runtime treated the mechanical patch conflict as ordinary no-coverage progress failure.
- Expected behavior:
  - A stale target snapshot conflict is a generic runtime patch-refresh problem, not a business-specific Goal/DoD failure.
  - The runtime should first allow automatic repair using the current target snapshot, while still preserving repeated-failure and repair-attempt budgets to prevent infinite loops.
- Code closure:
  - `patch/apply` / `patch_apply_failed` `baseHash mismatch` no longer consumes `noCoverageDeltaRounds`.
  - The failure still updates failure signature tracking.
  - The repair action package now treats `baseHash mismatch` as stale patch evidence, requires current-target snapshot rebuild, and forbids reusing the rejected patch hunk.
- Boundary result:
  - This is core runtime patch mechanics. The code does not branch on workspace ID, template ID, provider, language, concrete filename, tool name, or business text.
- Verification:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestUpdateConvergenceBudgetDoesNotCountPatchBaseHashMismatchAsNoDelta|TestRuntimeRepairActionPackageTreatsBaseHashMismatchAsStalePatch|TestRuntimeRepairActionPackageGroundsCurrentPatchFailure' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
  - `./scripts/verify-boundary-contract.sh`
  - `go build -o ./bin/workorch ./application/workorch`
- Not closed by this record:
  - Fresh deployed live convergence after explicit replan/resume.
  - Full real-provider resource verification.

### 2026-06-22 B7-G3 Live Patch BaseHash Refresh Verification

- Verification ref:
  - `ev:verify/live-patch-basehash-refresh/20260622-022`
- Result:
  - The rebuilt deployed process matched `./bin/workorch`.
  - Scoped `replan` of the stale `patch baseHash mismatch` decision created a new successor run.
  - The previous baseHash mismatch blocker did not recur; the run advanced into contract retry, graph execution, and state-changing file output.
  - The successor later blocked on a different shell validation failure in the workspace deliverable test file.
- Current live blocker:
  - `shell/command` validation failed with target build diagnostics in `main_test.go`: unused import, unused local variable, and assignment mismatch with the current helper signature.
  - Runtime projected `validation_build_failed`, structured target refs, current target snapshot, and scoped decision metadata.
- Status:
  - `ev:code/patch-basehash-refresh-repair/20260622-021` is live-verified for the stale baseHash path.
  - Positive live goal convergence remains open because the workspace still needs a valid follow-up repair for the target build diagnostics.

### 2026-06-21 B7-G2 CI-Safe Managed Deployed E2E

- Closed gate: `B7-G2` for the CI-safe managed deployed-instance path.
- Scope:
  - Managed workorch instance startup.
  - HTTP readiness and deployed service verification.
  - Workspace template and Soul e2e verification.
  - Mock LLM workspace execution verification.
  - MCP stdio workspace execution verification.
  - Runtime contract violation retry exhaustion decision resolution with explicit operator termination.
  - Targeted Go package regression for the changed appservice governance policy and orchestrator retry escalation context.
- Commands run:
  - `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestResolveDecisionWithSession(InfersRepairExhaustionReplanFromContinueUnderPolicy|KeepsExplicitRepairExhaustionTerminateUnderReplanPolicy|MapsRepairExhaustionReplanAndTerminate)' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_.*Governance|TestRuntimeOrchestrator_.*Localization|Test.*Repair.*Exhaust|Test.*ContractViolation' -count=1`
  - `WORKORCH_MANAGED_VERIFY_TARGET=verify-llm-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_VERIFY_KEEP_TMP=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh`
  - `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 ./scripts/verify-workorch-managed-full.sh`
- Bounded output:
  - `go test` appservice package: passed.
  - `go test` orchestrator package: passed.
  - Targeted managed verifier built console assets, ran `make vet`, built `./application/workorch`, started a managed service at `http://127.0.0.1:32870`, and `verify-llm-file` passed. Retained artifacts: `/tmp/tmp.Jd5qpEhxSw`; retained verification temp dir: `/tmp/tmp.qfEF0aalRa`.
  - CI-safe managed verifier ran `make fmt-check`, `make console-build`, `make vet`, `go test ./... -count=1`, built `./application/workorch`, started a managed service at `http://127.0.0.1:33126`, and `make verify-ci-mvp-file` passed.
  - `verify-ci-mvp-file` sub-checks passed: `verify-workorch-http.sh`, `verify-workspace-template-soul.sh`, `verify-llm-workspace-execution.sh`, and `verify-mcp-stdio-workspace-execution.sh`.
  - MCP stdio bounded output: `workspaceId=ws-cea872949bdc6233`, `runId=run-001`, `runState=completed`, `steps=3`, `toolCalls=3`, `validations=3`, `runtimeCheckpoints=26`.
  - Retained CI-safe artifacts: `/tmp/tmp.jwXoO27XoU`.
- Result:
  - The prior mock LLM contract-violation scenario no longer leaves the exhaustion ticket open or normalizes explicit `terminate` to `replan`.
  - Explicit `terminate` under runtime repair exhaustion resolves as `reject_current_path`.
  - Exhaustion tickets can still prefer `replan`, but operator-requested termination remains a valid stop path.
  - CI-safe deployed e2e now covers the managed service, workspace template/Soul, mock LLM, and MCP stdio paths for the current closure packet.
- Not closed by this record:
  - `B7-G3` live workspace verification.
  - `B7-G4` resource-envelope observation.

### 2026-06-21 B7-G1 Supplemental Batch 3 Contract Gate Regression

- Closed gate: incremental `B7-G1` verification for the Batch 3 `B3-G2` / `B3-G3` enforcement confirmation.
- Scope:
  - State-changing repair cannot satisfy the contract with failed or satisfied assessment-only output.
  - State-changing repair cannot use reason-only decision/approval blockers to bypass required executable output.
  - Existing-target `file_write` is accepted only with complete/safe snapshot and matching `baseHash`.
  - Unsafe, redacted, truncated, binary, incomplete, or stale-hash snapshots reject full `file_write`.
  - Non-substantive unified diffs are rejected at the LLM output contract entry.
- Commands run:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMExecutionOutputContract(AllowsStructuredStateChangingBlockers|RejectsIncompleteStateChangingBlockers|RejectsStateChangingRepairSatisfiedAssessmentOnly|AllowsSafeRepairFileWriteForExistingContextTarget|RejectsRepairFileWriteForExistingContextTargetWithoutBaseHash|RejectsUnsafeRepairFileWriteForExistingContextTarget|RejectsNonSubstantiveRepairUnifiedDiff)' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
  - `go test ./internal/modules/kernel/orchestrator/... -count=1`
  - `make verify-boundary-contract`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md`: no missing definitions.
  - `python3 -m py_compile scripts/loadtest-governed-workspace-concurrency.py`
  - `git diff --check`
- Result:
  - The Batch 3 state-changing output gate and patch snapshot gate now have explicit regression coverage at the LLM execution output contract boundary.
  - The stricter blocker rule is neutral and schema-based: decision blockers require a question; approval blockers require an action. It does not branch on workspace ID, template ID, Soul ID, provider, language, file name, or business text.
  - Boundary verification and diff whitespace checks passed for the current incremental change.
- Not closed by this record:
  - `B7-G3` live workspace positive goal-convergence verification.
  - Full real-provider resource/convergence verification.

### 2026-06-21 B7-G4 Managed Mock Resource Loadtest

- Closed gate: `B7-G4` for the deterministic mock-provider resource-envelope loadtest.
- Scope:
  - Managed workorch instance startup with `server.maxConcurrentRuns=2`.
  - Two concurrent governed workspace executions using deterministic mock LLM and transient MCP stdio helper.
  - Projection/status/snapshot/observability reads for each workspace after completion.
  - Resource-adjacent counters exposed by the loadtest: active overlap, in-flight run requests, HTTP errors, curl errors, LLM protocol errors, and timeline run-state completeness.
- Script fix made before the passing run:
  - `scripts/loadtest-governed-workspace-concurrency.py` now binds `steps` and `timeline` from `populate_result_state_summary` inside `validate_final_state`.
  - This fixes verifier false negatives where completed workspaces were reported as failed due to Python `NameError`, without weakening any workspace success, result, artifact, timeline, LLM usage, or concurrency assertion.
- Commands run:
  - `python3 -m py_compile scripts/loadtest-governed-workspace-concurrency.py`
  - `WORKORCH_MANAGED_VERIFY_TARGET=loadtest-governed WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=2 WORKORCH_LOAD_TOTAL=2 WORKORCH_LOAD_CONCURRENCY=2 WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP=1 WORKORCH_LOAD_ARTIFACT_DIR=/tmp/workorch-b7-loadtest-20260621143709 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz ./internal/services/workorch" ./scripts/verify-workorch-managed-full.sh`
- Bounded output:
  - Managed verifier ran `make fmt-check`, `make console-build`, `make vet`, selected Go package tests, built `./application/workorch`, started a managed service at `http://127.0.0.1:33586`, and `make loadtest-governed` passed.
  - Loadtest summary: `batchId=load-20260621063756`, `total=2`, `concurrency=2`, `serverMaxConcurrentRuns=2`, `workspaceSuccessTotal=2`, `workspaceFailedTotal=0`, `successRate=1.0`, `pass=true`.
  - Concurrency/resource counters: `expectedActiveRunOverlap=2`, `maxObservedActiveStageOverlap=2`, `maxObservedRunRequestsInFlight=2`, `activeRunOverlapPass=true`, `metricsSampleTotal=27`.
  - Error counters: `http4xxTotal=0`, `http5xxTotal=0`, `curlErrorTotal=0`, `llmProtocolErrorTotal=0`, `timelineMissingRunStateTotal=0`.
  - Per-workspace bounded output: both completed workspaces had `stepCount=6`, `toolCallCount=6`, `validationCount=6`, `resultCount=2`, `artifactCount=2`.
  - Retained loadtest artifacts: `/tmp/workorch-b7-loadtest-20260621143709`; retained managed artifacts: `/tmp/tmp.WoXjNZmMwM`.
- Result:
  - CI-safe mock resource envelope now proves concurrent run admission, projection/observability reads, completed workspace state, accepted results, artifact linkage, and zero transport/protocol/timeline errors for the deterministic loadtest path.
- Not closed by this record:
  - `B7-G3` live workspace verification.
  - Real-provider runtime/resource behavior from `verify-real-runtime-closure`, which still requires configured external provider credentials and provider stability.

### 2026-06-21 B7-G3 Live Projection Observation

- Closed gate: projection/display-health portion of `B7-G3`; goal-convergence portion remains open.
- Scope:
  - Existing deployed service at `http://127.0.0.1:8080`.
  - Bounded observation of live workspace projection, workspace snapshots, run heartbeat display contract, scheduler/watchdog metrics, pprof goroutine snapshot, and CPU profile availability.
  - No new run activation, no real-provider call, and no operator decision/replan mutation.
- Commands run:
  - `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-observe-20260621144244 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=5 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=20 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 ./scripts/observe-workorch-runtime-closure.sh`
  - After enhancing the observer bounded summary, rerun: `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-observe-summary-20260621145121 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=3 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=20 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 ./scripts/observe-workorch-runtime-closure.sh`
- Bounded output:
  - Service readiness: `serviceReady=true`.
  - Projection: `projectionState=ready`, `workspaceTotal=2`, `projectionPendingRebuilds=0`, `projectionScheduleDroppedDelta=0`.
  - Observed workspace ids: `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43`.
  - Display contract: `pass=true`, `snapshotCount=2`, no display contract issues.
  - Runtime metrics: `serverMaxConcurrentRuns=5`, `runRequestsInFlight=0`, `runQueueWaiting=0`, `stalledRuns=0`, `ineffectiveRetryRuns=2`, `activeRunHeartbeatAgeSecondsMax=14242`.
  - Scheduler deltas during the 5-second window: no command errors, enqueue failures, continuation failures, or stale watchdog recoveries.
  - pprof: goroutine and CPU profile checks were available; no suspicious runtime adapter/eventHub send/dispatch stack issues were reported.
  - Warning: no SSE events were captured during the observation window.
  - Retained artifacts: `/tmp/workorch-b7-live-observe-20260621144244` and `/tmp/workorch-b7-live-observe-summary-20260621145121`.
- Live workspace states:
  - Both observed workspaces were `waiting_decision` with active run `run-001-replan-001-replan-001-replan-001`.
  - Both run heartbeats were present, `activityState=waiting_decision`, `stalled=false`, and `ineffectiveRetry=true`.
  - The rerun summary includes bounded `workspaceSummaries` for the same two workspaces. Each summary includes workspace/run state, blocking ticket or approval ids, heartbeat progress markers, projection revisions, decision/approval/result/task/patch counts, compact open blocker refs, latest result/task refs, and current coverage gap fields without embedding large payloads.
  - Rerun bounded current state:
    - `ws-4e83810542ac4d65`: `workspace.state=waiting_decision`, `run.state=blocked_on_decision`, `heartbeat.ineffectiveRetry=true`, one open decision, eight results with four accepted and four rejected, seven patches, and current coverage gap `missing accepted evidence binding for required evidence kind: validation`.
    - `ws-7ced70e98ea50c43`: `workspace.state=waiting_decision`, `run.state=blocked_on_decision`, `heartbeat.ineffectiveRetry=true`, one open decision, six results with two accepted and four rejected, four patches, and current coverage gap `missing accepted evidence binding for required evidence kind: validation`.
- Result:
  - workorchd projection ownership, bounded display contract, scheduler/watchdog metrics, and profile endpoints are healthy for the observed live service.
  - The observer now emits enough bounded per-workspace state to support follow-up live convergence analysis without requiring full snapshot payloads in the audit document.
  - This observation does not prove live goal convergence because both live workspaces are waiting on decision and marked ineffective retry.
- Not closed by this record:
  - `B7-G3` live workspace positive goal-convergence verification after operator decision/replan/resume.
  - Real-provider execution success and cache/resource behavior.

### 2026-06-21 B7-G4 Real Runtime Preflight

- Closed gate: preflight portion of real-provider resource verification; provider execution remains open.
- Scope:
  - Existing deployed service at `http://127.0.0.1:8080`.
  - Readiness and scheduler/watchdog metrics only.
  - No real-provider LLM execution.
- Commands run:
  - `WORKORCH_REAL_RUNTIME_BASE_URL=http://127.0.0.1:8080 WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY=1 WORKORCH_REAL_RUNTIME_ARTIFACT_DIR=/tmp/workorch-b7-real-preflight-20260621144344 WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS=1 WORKORCH_REAL_RUNTIME_LOAD_RUN_TIMEOUT_SECONDS=900 ./scripts/verify-real-runtime-closure.sh`
- Bounded output:
  - `serviceReady=true`.
  - `runTimeoutSeconds=900`.
  - `staleDispatchedStepMinAgeSeconds=120`.
  - `maxConcurrentRuns=5`.
  - `runRequestsInFlight=0`.
  - `runQueueWaiting=0`.
  - `watchdogIntervalSeconds=30`.
  - `watchdogTicks=507`.
  - `watchdogLastErrorTimestampSeconds=0`.
  - `schedulerMetricMarkersPresent=true`.
  - `pass=true`, `errors=[]`.
- Result:
  - The deployed service exposes the scheduler/watchdog metrics required before a real-provider resource closure run.
- Not closed by this record:
  - Full `verify-real-runtime-closure` loadtest and post-run observation, which require configured external provider credentials and provider stability.

### 2026-06-21 B7-G3 Fresh Live Observation Before Supplemental Batch 3 Redeploy

- Scope:
  - Existing deployed service at `http://127.0.0.1:8080`.
  - Read-only 3-second observation using the enhanced bounded `workspaceSummaries`.
  - No new run activation, no replan/resume, and no provider calls initiated by the verifier.
- Command run:
  - `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-live-observe-20260621150205 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=3 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=20 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 ./scripts/observe-workorch-runtime-closure.sh`
- Bounded output:
  - Service readiness and projection state remained healthy: `serviceReady=true`, `projectionState=ready`, `workspaceTotal=2`, display contract `pass=true`.
  - Metrics showed `serverMaxConcurrentRuns=5`, `runRequestsInFlight=0`, `runQueueWaiting=0`, `stalledRuns=0`, `ineffectiveRetryRuns=2`, and no scheduler command/enqueue/continuation errors during the window.
  - pprof and CPU profile were available; no suspicious eventHub send/dispatch stacks or CPU hotspots were reported.
  - `ws-4e83810542ac4d65`: `workspace.state=waiting_decision`, `run.state=blocked_on_decision`, open decision reason `LLM 输出连续违反运行期合同，自动修正预算已耗尽: concrete_validation_contract_missing`, latest result rejected, current gap `missing accepted evidence binding for required evidence kind: validation`.
  - `ws-7ced70e98ea50c43`: `workspace.state=waiting_decision`, `run.state=blocked_on_decision`, open decision reason `LLM 输出连续违反运行期合同，自动修正预算已耗尽: state_changing_required_missing`, latest result rejected, current gap `missing accepted evidence binding for required evidence kind: validation`.
- Result:
  - The service/projection/resource envelope remains healthy, but live goal convergence is still not proven.
  - This observation identified a supplemental Batch 3 code gap: the deployed `ws-4e...` contract-retry input still lacked `validationScratchDir` in convergence `need_validation`. The code closure record is in Batch 3 and requires redeploy plus fresh replan/resume observation before live convergence can be claimed.

### 2026-06-22 B7-G3 Live Goal-Convergence Gate Tooling

- Closed gate: verification tooling portion of `B7-G3`; positive live convergence remains open.
- Scope:
  - `scripts/observe-workorch-runtime-closure.sh` only.
  - Read-only live observation summary and failure classification.
  - No runtime state-machine change, no repository mutation, no provider calls created by the verifier, and no workspace-specific business rule.
- Script change:
  - Added optional `WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1`.
  - Added bounded `goalConvergence` summary to `runtime-observation-summary.json`.
  - Added bounded `latestResult.summary` and `latestResult.reason` in each `workspaceSummaries` item so reviewers can identify the current failure surface without opening full snapshot payloads.
  - The gate checks only neutral projection/snapshot fields:
    - active run id is present;
    - run state is in `WORKORCH_OBSERVE_GOAL_CONVERGENCE_RUN_STATES` (default `completed`);
    - no open decision or pending approval remains;
    - heartbeat is not `stalled` and not `ineffectiveRetry`;
    - accepted result count is at least `WORKORCH_OBSERVE_MIN_GOAL_ACCEPTED_RESULTS` (default `1`);
    - artifact count is at least `WORKORCH_OBSERVE_MIN_GOAL_ARTIFACTS` (default `0`);
    - coverage has no current gap or missing evidence kinds when `WORKORCH_OBSERVE_REQUIRE_GOAL_COVERAGE_CLOSED=1`.
- Commands run:
  - `bash -n scripts/observe-workorch-runtime-closure.sh`
  - `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-goal-convergence-gate-20260622 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=1 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=2 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh`
  - `python3 -m json.tool /tmp/workorch-b7-goal-convergence-gate-20260622/runtime-observation-summary.json`
  - Supplemental bounded summary check after adding `latestResult.summary`: `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-observe-summary-20260622111724 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=1 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=2 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh`
- Bounded output:
  - `goalConvergence.required=true`.
  - `goalConvergence.pass=false`.
  - `workspaceCount=2`.
  - `ws-4e83810542ac4d65`: `runState=blocked_on_decision`, failed convergence with five structured issues: non-terminal run state, one open decision, `ineffectiveRetry=true`, latest result rejected, and coverage gap requiring validation evidence.
  - `ws-7ced70e98ea50c43`: `runState=blocked_on_decision`, failed convergence with the same five structured issue classes.
  - Process resource observation during the short window remained bounded: `cpuPercent=1.50`, `rssBytes=401055744`, `readBytesDelta=0`, `writeBytesDelta=0`.
  - Supplemental bounded summary output now exposes latest rejected result summaries without opening full snapshots:
    - `ws-4e83810542ac4d65`: latest result summary still contains the pre-scratch-contract wording `externally bounded`, while current code no longer contains that string; this is a historical request/blocker signal, not proof that the current code still lacks `validationScratchDir`.
    - `ws-7ced70e98ea50c43`: latest result summary states it lacks an actionable target snapshot for safe `text_replace` or `unified_diff`, preserving the current failure surface in the compact observation packet.
- Result:
  - Batch 7 now has an executable verification gate for positive live goal convergence instead of relying on human inspection of workspace summaries.
  - The current deployed instance still does not prove live convergence; both observed workspaces remain blocked on decision with ineffective retry and open validation coverage gaps.
  - The run also exposed that this deployed instance lacks several newer metrics markers expected by the observer, so this output should be treated as a useful live convergence failure packet, not as a full observer pass.
- Boundary result:
  - The gate is business-neutral and uses only generic run, blocker, result, heartbeat, artifact, and coverage projection fields.
  - It does not branch on workspace ID, template ID, Soul ID, provider, programming language, filename, LSP, or concrete goal text.
- Not closed by this record:
  - Positive live goal convergence.
  - Full real-provider resource/convergence verification.

### 2026-06-22 B7-G3 Observer Optional Metric Availability Refinement

- Closed gate: observer correctness for optional resource metric availability.
- Scope:
  - `scripts/observe-workorch-runtime-closure.sh`.
  - Verification semantics only; no WorkOrch runtime state-machine change and no workspace mutation.
- Reason:
  - The live goal-convergence gate correctly failed the currently deployed instance because the observed workspaces were still `blocked_on_decision`.
  - The same run also saw missing newer resource metric markers on the deployed service. Those markers are useful for newer resource-envelope checks, but their absence must not mask the primary live goal-convergence result unless the selected gate explicitly requires those metrics or sets thresholds for them.
- Script change:
  - Base readiness, projection, scheduler, and heartbeat markers remain required.
  - `eventRecordIOPressure`, `runtimeEventPublishFailures`, and `watchdogRegistration` metric marker groups now report bounded availability in `resourceEnvelope.optionalMetricAvailability`.
  - Missing optional groups are warnings by default.
  - Missing optional groups become errors only when the selected gate enables the corresponding threshold or explicit requirement, for example `WORKORCH_OBSERVE_MAX_EVENT_METADATA_RECORDS_SCANNED_DELTA`, `WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATES_DELTA`, `WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATE_BYTES_DELTA`, `WORKORCH_OBSERVE_MAX_RUNTIME_EVENT_PUBLISH_FAILURES_DELTA`, or `WORKORCH_OBSERVE_REQUIRE_WATCHDOG_REGISTRATION_METRIC=1`.
- Commands run:
  - `bash -n scripts/observe-workorch-runtime-closure.sh`
- Boundary result:
  - The refinement is business-neutral and provider-neutral. It only changes observer gate classification for metric availability.
  - It does not branch on workspace ID, template ID, Soul ID, provider, programming language, filename, LSP, or concrete goal text.
- Not closed by this record:
  - Positive live goal convergence.
  - Full real-provider resource/convergence verification.

### 2026-06-21 B7-G1 Supplemental Batch 1 Delivery Policy Regression

- Closed gate: Batch 1 EventHub delivery policy regression.
- Scope:
  - Event contract policy only.
  - No call-site behavior migration, no live service mutation, and no provider calls.
- Commands run:
  - `go test ./internal/pkg/events -count=1`
  - `go test ./internal/modules/kernel/run/biz -count=1`
  - `go test ./internal/modules/kernel/workspace/biz -count=1`
  - `go test ./internal/modules/kernel/approval/biz -count=1`
  - `go test ./internal/modules/kernel/decision/biz -count=1`
  - `go test ./internal/modules/kernel/governance/biz -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/pkg/events`
  - `ok github.com/muidea/workorch/internal/modules/kernel/run/biz`
  - `ok github.com/muidea/workorch/internal/modules/kernel/workspace/biz`
  - `ok github.com/muidea/workorch/internal/modules/kernel/approval/biz`
  - `ok github.com/muidea/workorch/internal/modules/kernel/decision/biz`
  - `ok github.com/muidea/workorch/internal/modules/kernel/governance/biz`
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
- Regression evidence:
  - `TestEventHubDeliveryModeForTopicClassifiesCommandsAndNotifications` proves read/write/facade/runtime-execute topics are synchronous, runtime terminal notify results and projection notifications are async-preferred, facade result topics keep response semantics, and unknown topics keep conservative synchronous delivery.
  - Existing EventHub tests still prove lane selection and `PublishAsync` uses magicCommon `Post`.
  - `TestPublishByContractUsesAsyncForNotifyWhenAvailable`, `TestPublishByContractFallsBackToSyncWhenAsyncUnavailable`, and `TestPublishByContractUsesSyncForCommandTopics` prove the shared publisher applies the delivery policy without requiring every hub to implement async delivery.
  - `TestService_PublishesRunNotificationsThroughAsyncHub`, `TestGoalDoDManager_PublishesWorkspaceNotificationsThroughAsyncHub`, `TestGoalDoDManager_PublishesDefinitionNotificationsThroughAsyncHub`, `TestService_PublishesApprovalNotificationsThroughAsyncHub`, `TestService_PublishesDecisionNotificationsThroughAsyncHub`, `TestPublishGovernanceEventUsesAsyncHubForNotifications`, and `TestEventSubsystemPublishesNotificationsThroughAsyncHub` prove run/workspace/approval/decision/governance/orchestrator event-subsystem notifications use contract-aware async publication when the hub supports it.
- Result:
  - Batch 1 now has a canonical, test-covered delivery policy and shared publisher that future module owners can use when choosing synchronous or asynchronous event delivery.
  - Run/workspace/approval/decision/governance/orchestrator event-subsystem notifications are migrated to the shared contract-aware publisher; command/result response paths remain synchronous.
  - This does not automatically migrate every existing notify publisher; each additional migration must separately prove idempotency, ordering, and replay safety for the owner topic.
- Not closed by this record:
  - Live goal convergence.
  - Full real-provider resource verification.

### 2026-06-22 B7-G1 Supplemental Batch 6 Result Event Naming Regression

- Closed gate: incremental `B7-G1` verification for the Batch 6 `B6-G1` result event naming clarity closure.
- Scope:
  - Internal orchestrator result event publisher naming only.
  - No repository state change, no payload shape change, no event topic compatibility change, no provider calls, and no live service mutation.
- Commands run:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_(RecordsRejectedResult|RejectedResult|DispatchNextClosesRunningRunWithRejectedResult|DuplicateCompletedRuntimeResultDoesNotRepeatResultArtifacts|CompleteRunRequiresAcceptedResultForDoDGatedRun|ResultArtifacts)|TestResultSubsystemRecordsAcceptedSliceResultWithoutFinalCompletion' -count=1 -timeout=180s`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestRunSnapshotFromModelsProjectsBlockingDecisionStepAndCoverage|TestRunSnapshotFromModelsProjectsContractViolationWithoutRuntimeProgress|TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection|TestWorkspaceSnapshotFromModelsProjectsArtifactContinuations|TestProjectWorkspaceSnapshotDetailCompactsContextAndRunDerivedCollections' -count=1 -timeout=180s`
  - `go test ./internal/pkg/events -run 'Test.*Result.*|TestWorkorchEvent.*Result' -count=1 -timeout=120s`
  - `make verify-boundary-contract`
  - `git diff --check -- internal/modules/kernel/orchestrator/biz/runtime_events.go internal/modules/kernel/orchestrator/biz/runtime_result_artifacts.go internal/modules/kernel/orchestrator/biz/runtime_result_recording.go docs/70-roadmap/active/runtime-contract-retry-batch6-evidence-coverage-projection-audit.md`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - `ok github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection`
  - `ok github.com/muidea/workorch/internal/pkg/events`
  - `boundary contract verification passed`
- Regression evidence:
  - The internal helper name is now `publishResultEvent`, matching the state-based event publisher behavior.
  - `resultEventTopic` remains the compatibility authority: accepted results publish `/result/accepted`, rejected results publish `/result/rejected`, and superseded results publish `/result/superseded`.
  - Existing rejected-result tests still prove rejected results are persisted and projected as rejected, not counted as accepted final completion, and emitted through rejected result event semantics.
- Boundary result:
  - The change is a core event naming clarity change only.
  - No branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, filenames, LSP, or business-specific text.
- Not closed by this record:
  - Live goal convergence.
  - Full real-provider resource verification.

### 2026-06-22 B7-G1 Supplemental TUI Port Boundary Regression

- Closed gate: incremental `B7-G1` verification for the historical TUI agent
  boundary audit.
- Scope:
  - TUI / CLI / agent port boundary only.
  - No runtime state-machine change, no repository state change, no REST payload
    shape change, no provider calls, and no live service mutation.
- Commands run:
  - `go test ./internal/pkg/workorchport ./internal/services/workorch/service/tui ./internal/services/workorch/biz/backends ./internal/services/workorch/biz/agent ./internal/services/workorch/service/cli -count=1 -timeout=180s`
  - `rg -n "internal/modules/|internal/pkg/workorchclient|biz/backends|internal/pkg/repository|framework/application|EventHub|event\\.Hub|internal/pkg/events" internal/services/workorch/service/tui internal/services/workorch/service/cli internal/services/workorch/biz/agent internal/pkg/workorchport -g '*.go'`
  - `make verify-boundary-contract`
  - `git diff --check -- internal/pkg/workorchport/backend.go internal/pkg/workorchport/event_payload_keys.go internal/pkg/workorchport/governance.go internal/pkg/workorchport/governance_test.go internal/pkg/workorchport/runtime_block.go internal/pkg/workorchport/runtime_block_test.go internal/services/workorch/biz/agent/agent.go internal/services/workorch/service/tui/commands.go internal/services/workorch/service/tui/model.go internal/services/workorch/service/tui/model_test.go internal/services/workorch/service/tui/view.go internal/services/workorch/service/tui/workspace_state.go docs/60-verification/audits/tui-agent-requirements-review-2026-05-03.md`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/pkg/workorchport`
  - `ok github.com/muidea/workorch/internal/services/workorch/service/tui`
  - `ok github.com/muidea/workorch/internal/services/workorch/biz/backends`
  - `ok github.com/muidea/workorch/internal/services/workorch/biz/agent`
  - `ok github.com/muidea/workorch/internal/services/workorch/service/cli`
  - Code boundary search returned no TUI/CLI/agent/port imports of internal
    module, event payload package, backend implementation, REST client,
    repository, framework, or EventHub packages.
  - `boundary contract verification passed`
- Regression evidence:
  - TUI production code now obtains decision resolution constants and
    approval/decision semantic normalization from `workorchport`.
  - TUI production code now obtains runtime block kind canonicalization from
    `workorchport` instead of importing the LLM block package.
  - TUI and agent event payload parsing now uses `workorchport` payload key
    constants instead of importing the event payload package.
  - `workorchport` has direct semantic normalization regression tests for
    replan, explicit reject-current-path, approval, and rejection inputs.
  - `workorchport` has runtime block kind canonicalization tests for workspace
    plan, legacy patch request, patch request, and custom trimmed kinds.
  - Existing TUI tests still cover decision replan after workspace switch,
    stale blocker replacement, server-projected ticket preference, approval
    semantic parsing, prompt updates, and projection-only refresh behavior.
- Boundary result:
  - CLI/TUI/agent code remains behind the WorkOrch port/backend boundary for
    decision/governance semantics.
  - The change is business-neutral: no branch depends on workspace template ID,
    Soul ID, live workspace ID, provider, programming language, filenames, LSP,
    or business-specific text.
- Not closed by this record:
  - Live goal convergence.
  - Full real-provider resource verification.

### 2026-06-22 B7-G1 Supplemental Batch 5 Live Regression Verification

- Closed gate: incremental `B7-G1` verification for the Batch 5 live
  regression supplemental closure.
- Scope:
  - Runtime repair input and contract violation classifier only.
  - No prompt stable-prefix change, no provider-specific cache field, no
    repository state migration, no REST/TUI payload shape change, and no live
    service mutation.
- Related closure record:
  - `docs/70-roadmap/active/runtime-contract-retry-batch5-context-actionability-audit.md`
    section `2026-06-22 B5 Live Regression Supplemental Closure`.
- Commands run:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(ApplyRuntimeRepairStateChangingActionPackageProjectionMaterializesAvailableTargetSnapshots|ContractViolationClassifiesAvailableSnapshotIgnored|ContractViolationClassifiesAvailableSnapshotIgnoredFromNestedChineseOutput|ContractViolationDoesNotClassifyMissingSnapshotWithoutAvailableSnapshot)$' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*(AvailableTargetSnapshots|AvailableSnapshot|ContractViolation.*Snapshot|StateChangingActionPackageProjection)' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*(Scratch|Validation|Contract|Convergence)' -count=1 -timeout=240s`
  - `go test ./internal/modules/blocks/llm/biz -run 'TestCurrentActionIncludesAvailableTargetSnapshots|TestCurrentActionMarksTruncatedAvailableSnapshotUnsafe|TestProviderCachePlanKeepsStablePrefixForAvailableTargetSnapshots|TestContractViolationRecoveryRendersInCurrentActionOnly' -count=1 -timeout=180s`
  - `make verify-boundary-contract`
  - `git diff --check -- internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_input.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_repair_input_test.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go docs/70-roadmap/active/runtime-contract-retry-batch5-context-actionability-audit.md`
  - `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|ws-[0-9a-f]" internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_input.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - `ok github.com/muidea/workorch/internal/modules/blocks/llm/biz`
  - `boundary contract verification passed`
  - Boundary search returned no production core occurrences for the searched
    business/template/language/workspace markers in the changed files.
- Regression evidence:
  - State-changing repair input now materializes
    `availableTargetSnapshots` from nested `failureTargetRepairContext` /
    `patchRepairContext` without copying full target content into the top-level
    summary.
  - `repairActionPackage.targetSnapshots`, `currentTargetSnapshots`, and
    current-snapshot direct patch requirements are synchronized when complete
    safe target context already exists.
  - Missing-current-target-snapshot claims in generic Chinese wording are
    classified as `available_snapshot_ignored` when complete safe snapshots are
    present.
  - Existing prompt/cache tests still prove available target snapshot fields
    are rendered in volatile current-action context and do not change the C0
    stable prefix.
- Read-only deployed observation:
  - Command:
    `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-observe-current-20260622113416 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=1 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=2 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh`
  - Result: observer exited `1` because both observed workspaces were still
    `blocked_on_decision` with open decisions, ineffective retry markers,
    rejected latest results, and open validation coverage gaps.
  - Interpretation: the deployed service projection was still on old events
    (`updatedAt=2026-06-21T02:50:14Z`) and surfaced the same historical
    controlled-substitute and available-snapshot blockers. This is a useful
    live convergence failure packet, but it does not prove the local Batch 5
    supplemental code closure has or has not converged after redeploy.
- Boundary result:
  - The change remains core-neutral and compares only structured runtime
    snapshot availability with generic missing-current-target claims.
  - No branch depends on workspace template ID, Soul ID, live workspace ID,
    provider, programming language, filenames, LSP, or business-specific text.
- Not closed by this record:
  - Positive live goal convergence after redeploy.
  - Full real-provider resource verification.

### 2026-06-22 B7-G2 Appservice Session Lifecycle And Managed CI-Safe Verification

- Closed gate: appservice/bootstrap lifecycle regression that prevented
  deterministic full managed verification.
- Scope:
  - `appservice` short-lived runtime session ownership.
  - Bootstrap test lifecycle cleanup for explicitly created runtime instances.
  - No runtime state-machine change, no prompt/context change, no repository
    schema change, no REST/TUI payload shape change, and no template/Soul
    policy change.
- Root cause:
  - `withBackendSession` and `withEnvSession` created `RuntimeSession`
    instances without owning their shutdown path. Most call paths closed the
    same session indirectly through nested `WithSession` helpers, but read-only
    and mixed call paths could leave file backend runtimes alive long enough to
    race `t.TempDir()` cleanup or cross-test package teardown.
  - `TestNewWithBackendRegistersOrchestratorCommandHandlers` constructed a
    file-backed bootstrap runtime directly and did not explicitly shut it down.
- Code closure:
  - `withBackendSession` and `withEnvSession` now use named return values and
    `defer session.CloseOnReturn(ctx, &err)`, making the helper that creates a
    short-lived session also own its teardown.
  - The bootstrap runtime registration test now shuts down its directly
    constructed runtime before TempDir cleanup.
  - Shared backend sessions are unchanged: `withSharedBackendSession` continues
    to borrow from the shared pool and the pool is closed only through the
    explicit shared-session cleanup path.
- Commands run:
  - `go test ./internal/modules/application/workorchd/biz/appservice -run TestRecoverStuckRunningRunsWithDuplicateRunIDsUsesWorkspaceScope -count=20 -timeout=120s`
  - `go test ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout=180s`
  - `go test ./internal/modules/application/workorchd/biz/bootstrap -count=1 -timeout=120s`
  - `go test ./internal/modules/application/workorchd/service/server -count=5 -timeout=180s`
  - `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 ./scripts/verify-workorch-managed-full.sh`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice 0.895s` for the 20-count duplicate-run recovery target.
  - `ok github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice 36.434s`
  - `ok github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap 0.227s`
  - `ok github.com/muidea/workorch/internal/modules/application/workorchd/service/server 59.718s`
  - Managed verification:
    - `make fmt-check` passed.
    - `make console-build` passed.
    - `make vet` passed.
    - `go test -count=1 ./...` passed.
    - `go build -o /home/rangh/aispace/workorch/bin/workorch ./application/workorch` passed.
    - Managed service became ready at `http://127.0.0.1:35426`.
    - `workorch HTTP verification passed`.
    - `workspace template and soul e2e verification passed`.
    - `workorch LLM workspace execution verification passed`.
    - `workorch MCP stdio verification passed`, with `runState=completed`,
      `steps=3`, `toolCalls=3`, `validations=3`, and
      `runtimeCheckpoints=26`.
    - `managed WorkOrch verification passed for http://127.0.0.1:35426 target=verify-ci-mvp-file`.
    - Artifacts retained at `/tmp/tmp.9Cxe0ADDRp`.
- Boundary result:
  - The lifecycle closure is core-neutral. It changes ownership of runtime
    resources created by appservice helper entrypoints; it does not branch on
    workspace template ID, Soul ID, live workspace ID, provider, programming
    language, filenames, LSP, or business-specific text.
- Not closed by this record:
  - Positive live goal convergence with real provider execution.
  - Full real-provider resource verification.

### 2026-06-22 B7-G1 Supplemental Batch 1 Web ACK ViewModel Regression

- Closed gate: incremental `B7-G1` verification for the Batch 1 accepted
  scheduler command state-model boundary.
- Scope:
  - Web workspace detail ViewModel only.
  - No backend state-machine change, no EventHub delivery change, no repository
    state change, no provider call, and no live service mutation.
- Related closure record:
  - `docs/70-roadmap/active/runtime-contract-retry-batch1-state-model-audit.md`
    gate `B1-G2`.
- Code closure:
  - Web `WorkspaceSnapshot` now declares `runCommand?: RunCommandStatus`.
  - Workspace detail renders `runCommand` as an independent `调度 ACK`
    metadata section.
  - Actual workspace/run state display still comes from owner projection
    fields: `workspace.state`, `activeRunId`, run list, run heartbeat, and
    runtime execution fields.
- Commands run:
  - `npm run build` in `web/workorch-console`.
  - `go test ./internal/services/workorch/service/tui ./internal/pkg/workorchclient ./internal/pkg/workorchport -count=1 -timeout=240s`
  - `go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchDWorkspaceRunCommand|TestDecisionResolveEndpointProjectsReplanRunCommand|TestWorkorchDSchedulerWorkspaceRunCommand' -count=1 -timeout=240s`
  - `make verify-boundary-contract`
  - `git diff --check`
- Bounded output:
  - Frontend TypeScript build and Vite build passed.
  - `ok github.com/muidea/workorch/internal/services/workorch/service/tui`
  - `ok github.com/muidea/workorch/internal/pkg/workorchclient`
  - `ok github.com/muidea/workorch/internal/pkg/workorchport`
  - `ok github.com/muidea/workorch/internal/modules/application/workorchd/service/server`
  - `boundary contract verification passed`
- Regression evidence:
  - Existing TUI regression `TestModelWorkspaceContextDoesNotTreatAcceptedRunCommandAsRunning`
    still proves accepted scheduler command metadata alone does not create a
    live run state or keep stale submitting UI state.
  - Existing server scheduler command tests still prove accepted command
    metadata is projected with workspace/run/request/source/decision scope.
  - Web now consumes the same metadata as display-only ACK data rather than
    silently dropping it or forcing clients to infer command state from run
    state.
- Boundary result:
  - The change is projection/ViewModel plumbing only.
  - It does not branch on workspace template ID, Soul ID, live workspace ID,
    provider, programming language, filenames, LSP, or business-specific text.
- Not closed by this record:
  - Positive live goal convergence.
  - Full real-provider resource verification.

### 2026-06-22 B7-G3 Live Replan Observation And Evidence-Acquisition Boundary Closure

- Closed gate: incremental `B7-G1` regression for the live `B7-G3`
  evidence-acquisition failure surface.
- Scope:
  - Read-only observation of the already deployed service at
    `http://127.0.0.1:8080`.
  - Explicit operator-style `replan` resolution of the two open live decisions
    to verify the governance-continuation main path.
  - Core-neutral prompt/current-action contract clarification for bounded
    evidence acquisition.
  - No weakening of runtime contract gates and no workspace/template-specific
    branch.
- Live verification before replan:
  - Command:
    `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-current-20260622140116 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=2 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=5 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh`
  - Bounded output: `goalConvergence.pass=false`; both workspaces were
    `blocked_on_decision`, had one open decision, `ineffectiveRetry=true`,
    latest rejected result, and an open validation coverage gap.
- Replan command verification:
  - `POST /api/v1/workorch/decisions/{ticketId}/resolve?workspaceId=...`
    with `resolutionType=replan` succeeded for
    `ws-4e83810542ac4d65` and `ws-7ced70e98ea50c43`.
  - Projection moved both workspaces to `running` with active run
    `run-001-replan-001-replan-001-replan-001-replan-001`.
  - Scheduler metric `workorch_scheduler_commands_total{kind="workspace_run",result="accepted"}`
    increased from `2` to `6`.
- Live verification after replan:
  - Command:
    `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-after-replan-20260622140246 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=10 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=5 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 ./scripts/observe-workorch-runtime-closure.sh`
  - Bounded output: both workspaces were `running`, had no open decisions, and
    had fresh `/run/llm_stream/progress`; CPU was `1.97%`, RSS
    `544681984`, read delta `0`, write delta `4096`.
  - Follow-up command:
    `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-progress2-20260622140727 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=5 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=5 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 ./scripts/observe-workorch-runtime-closure.sh`
  - Bounded output: both workspaces returned to `blocked_on_decision` after
    new LLM output, with fresh rejected results:
    - `ws-4e...`: `evidence acquisition shell/command node
      "prepare_multilang_definition_fixtures" must not write deliverable files`;
      the model used an evidence-acquisition shell node to initialize or mutate
      deliverable files instead of using a state-changing patch contract.
    - `ws-7c...`: `assessment-only step requested a decision for work that can
      continue through runtime repair or replan`; the model still attempted a
      continuable decision escape instead of producing executable repair or a
      failed assessment.
  - Resource envelope stayed bounded during the failed live run: CPU `1.97%`,
    RSS `521150464`, read delta `0`, write delta `0`; no pprof EventHub send or
    recursive dispatch issue was reported.
- Code closure:
  - `runtime_repair_action_package.go` now emits explicit neutral
    `requiredOutput` fields for evidence acquisition:
    `deliverableWriteForbidden=true`,
    `scratchOnlyForGeneratedFixtures=true`, `deliverableWriteRule`, and
    `scratchFixtureRule`.
  - `prompt_messages.go` and `http_provider.go` now tell the model that
    evidence acquisition, controlled-substitute, and supplemental validation
    shell/tool nodes must not create, initialize, modify, or delete workspace
    deliverable files. Generated fixtures, probes, and temporary configs must
    stay outside the deliverable tree, such as runtime scratch/temp paths.
  - Runtime rejection remains strict: workspace deliverable changes still
    require `patch/apply` or another explicitly accepted state-changing kind.
- Commands run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_grounding_test.go internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/prompt_messages_test.go internal/modules/blocks/llm/biz/http_provider.go`
  - `go test ./internal/modules/blocks/llm/biz -run 'TestBuildProviderPromptMessagesUses(StateChangingRepairAction|ConcreteValidationRepairAction)|TestContractViolationRecoveryRendersInCurrentActionOnly|TestCurrentActionIncludesAvailableTargetSnapshots' -count=1 -timeout=180s`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeRepairActionPackageUsesEvidenceAcquisitionWithoutConcreteTarget|TestRuntimeRepairActionPackageRequestsShapeEvidenceBeforeRepeatingShapePatch|Test.*EvidenceAcquisition|Test.*ConcreteValidation' -count=1 -timeout=240s`
  - `./scripts/verify-boundary-contract.sh`
  - `git diff --check -- internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_grounding_test.go internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/prompt_messages_test.go internal/modules/blocks/llm/biz/http_provider.go`
  - Boundary spot-check:
    `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|ws-[0-9a-f]" internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/http_provider.go`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/blocks/llm/biz`
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - `boundary contract verification passed`
  - `git diff --check` passed.
  - Boundary spot-check returned only pre-existing generic SSH/server and MCP
    server terminology; no new branch depends on workspace template ID, Soul
    ID, live workspace ID, provider, programming language, filenames, LSP, or
    business-specific text.
- Not closed by this record:
  - Positive live goal convergence after redeploying this prompt/current-action
    contract clarification and re-running replan/resume.
  - Full real-provider resource/convergence verification.

## 2026-06-22 B7-G2 Local Managed CI-Safe Reverification

- Scope:
  - Reverify the current local code closure packet with a managed temporary
    WorkOrch instance before any new live convergence claim.
  - This is a CI-safe mock/deployed verification tier. It does not close live
    goal convergence or real-provider resource verification.
- Evidence:
  - `ev:verify/managed-ci-safe/20260622-001`
- Command:
  - `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh`
- Bounded output:
  - `make fmt-check` passed.
  - Console build passed.
  - `go vet ./...` passed.
  - Targeted LLM and orchestrator package tests passed.
  - `go build -o bin/workorch ./application/workorch` passed.
  - Managed service became ready at `http://127.0.0.1:34370`.
  - `verify-workorch-http`, workspace template/Soul e2e, mock LLM workspace
    execution, and MCP stdio workspace execution passed.
  - Managed artifacts retained at `/tmp/tmp.CwFeuHCFwK`.
- Closure effect:
  - Refreshes the bounded CI-safe verification evidence for the current local
    closure packet after the evidence-acquisition deliverable-write boundary
    change.
  - Does not prove positive live goal convergence because the verification uses
    local mock providers and bounded CI-safe e2e flows.
  - Does not prove full real-provider resource/convergence behavior because no
    external real provider execution was run.
- Not closed by this record:
  - Positive live goal convergence after redeploy.
  - Full real-provider resource/convergence verification.

## 2026-06-22 B7-G3 Current Deployed Live Convergence Recheck

- Scope:
  - Recheck the currently running `http://127.0.0.1:8080` service with the
    positive goal-convergence observer gate enabled.
  - This record is failure evidence for the deployed runtime state. It does not
    prove whether the local code closure packet has been redeployed.
- Evidence:
  - `ev:verify/live-convergence/20260622-001`
- Command:
  - `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-convergence-20260622142251 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=8 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=5 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh`
- Bounded output:
  - Service readiness and projection were healthy: `serviceReady=true`,
    `projectionState=ready`, `workspaceTotal=2`.
  - `ws-4e83810542ac4d65` failed convergence:
    `runState=blocked_on_decision`, one open decision,
    `ineffectiveRetry=true`, latest result rejected, and the current coverage
    gap required validation evidence.
  - `ws-7ced70e98ea50c43` failed convergence with the same structural issues:
    `runState=blocked_on_decision`, one open decision,
    `ineffectiveRetry=true`, latest result rejected, and the current coverage
    gap required validation evidence.
  - The observer still reported a bounded resource envelope during the sample:
    CPU `1.32%`, RSS `469172224`, read delta `0`, write delta `4096`.
- Closure effect:
  - Confirms `B7-G3` remains open for the current deployed service.
  - Does not create a new code closure by itself; it restates the live
    convergence failure that must be cleared by redeploying the local closure
    packet and re-running the observer.
- Not closed by this record:
  - Positive live goal convergence.
  - Full real-provider resource/convergence verification.

## 2026-06-22 B7-G3 Latest Replan Failure And Context-Contract Closure

- Scope:
  - Recheck the latest deployed service after resolving both open decisions with
    `resolutionType=replan`.
  - Close the two newly observed generic contract/context gaps without adding
    workspace-, language-, provider-, or file-name-specific behavior to core.
- Evidence:
  - `ev:verify/live-convergence/20260622-002`
- Live observation:
  - Both decision resolves succeeded and both workspaces entered a fresh
    running run with `/run/llm_stream/progress`.
  - Both workspaces later returned to `blocked_on_decision`.
  - `ws-4e...` failed because the evidence-acquisition graph still wrote
    generated fixture state in the deliverable tree. The request contained
    `scratchOnlyForGeneratedFixtures=true`, but no concrete
    `validationScratchDir` was mirrored into the evidence-acquisition contract.
  - `ws-7c...` failed because the request carried conflicting structured
    focus data: `coverageFocus` pointed to `cwi-005`, while
    `repairActionPackage.coverageTargetSet` exposed earlier coverage items.
    The LLM produced a cwi-005 validation target and runtime rejected it as
    unrelated to the stale failure target scope.
- Code closure:
  - `runtime_validation_scratch.go` now exposes a generic
    `ensureValidationScratchContract` helper. Concrete validation still uses
    the same strict scratch contract, and evidence acquisition can now mirror
    `validationScratchDir` into input, `repairActionPackage`, and
    `requiredOutput`.
  - `runtime_repair_action_package.go` now uses the scratch helper for
    evidence-acquisition contracts and makes `scratchFixtureRule` explicitly
    point to `validationScratchDir`.
  - `coverageTargetSetFromRepairInput` now prioritizes the current
    `coverageFocus` by matching `coverageWorkItemIds` and `targetRefs` against
    `coverageWorkItemContext` items before filling the rest of the target set.
    This keeps the target relevance gate strict while removing contradictory
    focus/actionPackage inputs.
- Commands run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_grounding_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeRepairActionPackageUsesEvidenceAcquisitionWithoutConcreteTarget|TestCoverageTargetSetPrioritizesCurrentCoverageFocus|TestRepairActionPackageAnchorsPrimaryFailureAcrossTargetAdmissionWrapperFailure|Test.*ConcreteValidation|Test.*EvidenceAcquisition' -count=1 -timeout=240s`
  - `./scripts/verify-boundary-contract.sh`
  - `git diff --check -- internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_grounding_test.go`
  - Evidence reference integrity check for `runtime-contract-retry-*.md`.
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - `boundary contract verification passed`
  - `git diff --check` passed.
  - `evidence refs consistent`
- Not closed by this record:
  - Positive live goal convergence after redeploying this context-contract
    closure and re-running replan/resume.
  - Full real-provider resource/convergence verification.

## 2026-06-22 B7-G3 Runtime Capability And Non-Substantive Patch Closure

- Scope:
  - Close the next local code defects found while auditing
    `ev:verify/live-convergence/20260622-002` and the follow-up bounded
    request/output samples.
  - Preserve WorkOrch core neutrality: no workspace ID, template ID, language,
    provider, MCP server name, or target file name is encoded in production
    logic.
- Evidence:
  - `ev:code/context-contract/20260622-001`
  - `ev:verify/managed-ci-safe/20260622-002`
- Observed defects:
  - A follow-up LLM request could carry an old or empty
    `executionContextPackage.selectedContext` and omit the current run's
    declared `mcpServers`, `mcpCapabilities`, and `capabilityIds`, even though
    those capabilities still existed in `run.ExecutionContext`.
  - The executionGraph contract checked Soul-level allowed MCP refs but did not
    also reject a graph MCP `serverId` absent from the current runtime-declared
    MCP capability context.
  - A state-changing patch can carry structured quality metadata showing the
    patch is intentionally non-substantive or zero-work. That output is
    diagnostic only and must not be accepted as a substantive state change.
  - Full package testing also exposed duplicate `coverageTargetSet` entries for
    the same requirement when both current coverage work-item focus and legacy
    missing DoD item data were present.
- Code closure:
  - `execution_context_package.go` now merges current runtime capability
    context into stable `selectedContext`, with current declarative capability
    fields overriding stale selected capability data while preserving selected
    memory context.
  - `selectedContextPackage` now includes `capabilityIds` alongside compact
    MCP server and capability catalogs.
  - `runtime_capability_contract.go` now validates MCP graph nodes against
    runtime-declared MCP server IDs collected from run execution context,
    selected context, runtime capabilities, and capability IDs. Empty declared
    capability context remains non-blocking; present context rejects unknown
    server IDs.
  - `runtime_patch_requests.go` now rejects non-substantive patch requests only
    when the patch payload explicitly declares structured quality metadata such
    as `nonSubstantive`, `zeroWork`, or `substantive=false`. Natural-language
    placeholder/deferral interpretation belongs in template/Soul/policy
    guidance; core remains a neutral metadata interpreter.
  - `coverageTargetSetFromRepairInput` now dedupes targets by normalized
    requirement identity before kind/source-scope, so the current work item
    focus wins and legacy DoD/coverage fallbacks cannot duplicate the same
    target.
- Commands run:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRunExecutionContextPackageMergesCurrentRuntimeCapabilityContextIntoStableSelectedContext|TestValidateLLMExecutionOutputContractRejectsGraphMCPServerOutsideDeclaredRuntimeCapabilities|TestRuntimePatchRequestRejectsDeclaredNonSubstantivePatch|TestValidateLLMExecutionOutputContractRejectsGraphMCPServerOutsideCurrentSoulRefs|TestValidateLLMExecutionOutputContractRejectsNonSubstantiveRepairUnifiedDiff' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestGraphExpansionLimitFollowUpRestoresCoverageValidationConvergenceContract|TestGraphFollowUpRestoresCoverageValidationConvergenceContract' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s`
  - `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh`
  - `./scripts/verify-boundary-contract.sh`
  - `git diff --check -- internal/modules/kernel/orchestrator/biz/execution_context_package.go internal/modules/kernel/orchestrator/biz/runtime_capability_contract.go internal/modules/kernel/orchestrator/biz/runtime_patch_requests.go internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/execution_context_package_test.go internal/modules/kernel/orchestrator/biz/runtime_contract_optimization_test.go internal/modules/kernel/orchestrator/biz/runtime_patch_integration_test.go`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - Managed verification passed for `http://127.0.0.1:34552` target
    `verify-ci-mvp-file`; artifacts retained at `/tmp/tmp.WoVjdTAXve`.
  - Managed MCP stdio e2e completed run `run-001` with `steps=3`,
    `toolCalls=3`, `validations=3`, and `runtimeCheckpoints=26`.
  - `boundary contract verification passed`
  - `git diff --check` passed.
- Not closed by this record:
  - Positive live goal convergence after redeploying this closure and resolving
    or resuming the affected workspaces.
  - Full real-provider resource/convergence verification.

## 2026-06-22 B7-G3 Live Continuation Observation After Context Closure

- Scope:
  - Re-check the deployed `http://127.0.0.1:8080` service after the local
    context-contract closure and latest redeploy.
  - Determine whether the two active workspaces are blocked by workorchd
    projection/EventHub/runtime resource defects, or are currently waiting on
    external LLM provider calls.
- Evidence:
  - `ev:verify/live-convergence/20260622-003`
- Observed state:
  - `ws-4e83810542ac4d65` and `ws-7ced70e98ea50c43` were both `running`.
  - Neither workspace had an open decision or pending approval.
  - SSE produced `/run/llm_stream/progress` and projection-change events for
    both workspaces during the observation window.
  - The current active steps were LLM provider-call waits, so no new evidence
    supports classifying the state as EventHub dispatch loss, TUI projection
    staleness, or scheduler command loss.
  - Resource envelope remained bounded: CPU averaged `5.23%`, RSS was
    `426364928`, read delta was `0`, write delta was `65536`, pprof reported
    no synchronous runtime send stack, and runtime event publish failure
    deltas were `0`.
- Result:
  - No additional code closure is justified by this observation alone.
  - The next valid Batch 7 gate is to wait for provider responses and then
    inspect the resulting terminal output, retry, decision, or completion
    state.
  - Positive live goal convergence remains open because both runs were still
    running and coverage gaps were still present.
- Boundary note:
  - This record intentionally does not add provider, workspace-title, language,
    MCP server, or deliverable-file rules to core. Live workspace IDs are only
    evidence scope.

## 2026-06-22 B7-G3 Profile Evidence State-Change Focus Closure

- Scope:
  - Close the live follow-up regression observed after provider output returned:
    a repair focused on supplemental profile evidence still inherited a stale
    primary state-change focus and therefore rendered `requiredOutput` without
    the profile-required write evidence kind.
  - Preserve core neutrality by using structured coverage/evidence fields, not
    workspace title, deliverable name, language, file name, or provider data.
- Evidence:
  - `ev:verify/live-convergence/20260622-003`
  - `ev:code/profile-evidence-state-change/20260622-004`
- Observed defect:
  - A live repair target had a primary state-change item with structured
    `requiredEvidenceKinds=[validation,state_change]` and
    `missingEvidenceKinds=[validation]`, while the current focus required
    supplemental state-change evidence.
  - The primary target was still treated as needing another primary
    state-changing output, which polluted `requiredOutput.acceptedKinds` with
    generic primary state-changing kinds and omitted the current supplemental
    profile evidence kind.
  - The LLM returned `git/write` add/commit/log nodes for the supplemental
    evidence target, but the runtime rejected the graph as missing patch/apply
    or accepted auditable state-changing operation.
- Code closure:
  - `coverageTargetProfileStateChangeEvidenceSatisfied` and
    `coverageTargetClosedForPrimaryStateChange` now treat an explicitly
    structured target as primary-state-change closed when it requires
    `state_change` but `missingEvidenceKinds` no longer includes
    `state_change`.
  - The rule intentionally does not use the default validation-only
    `coverageMissingEvidenceKinds` fallback as proof of state-change closure;
    it requires explicit `requiredEvidenceKinds` and `missingEvidenceKinds`.
  - Mixed target repair now derives the current supplemental profile evidence
    contract instead of reopening an unrelated primary state-changing patch.
  - Execution contract validation now has an explicit regression proving
    `git/write` is accepted only when `requiredOutput.acceptedKinds` and
    `acceptedOperations` allow the operation.
- Commands run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_convergence.go internal/modules/kernel/orchestrator/biz/runtime_convergence_test.go internal/modules/kernel/orchestrator/biz/runtime_execution_contract_integration_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestBuildConvergenceContractDoesNotReopenPrimaryStateChangeWhenOnlyValidationMissing|TestBuildConvergenceContractUsesProfileRequiredStateChangingEvidenceForCoverageTarget|TestValidateLLMExecutionOutputContractAllowsAcceptedGitWriteStateChangingGraph|TestValidateLLMExecutionOutputContractAllowsDeclaredStateChangingShellGraph|TestValidateLLMExecutionOutputContractRejectsUndeclaredStateChangingShellGraph' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
- Not closed by this record:
  - The currently deployed service has not yet been rebuilt with this change.
  - Positive live goal convergence and full real-provider resource verification
    still require redeploy plus fresh bounded observation.

## 2026-06-22 B7-G3 Shell Bash-Feature Validation Compatibility Closure

- Closed gate: incremental `B7-G1` regression for a validation runtime output
  mismatch observed while auditing the still-open live `B7-G3` convergence
  path.
- Evidence:
  - `ev:code/shell-bash-feature/20260622-005`
- Observed defect:
  - A validation shell step can request `command=sh` with `args=["-c", ...]`
    while the script uses bash-only pipeline status syntax such as
    `${PIPESTATUS[0]}`.
  - When executed by POSIX `sh`, the validation command can emit `Bad
    substitution` and fail even if the underlying validation command already
    produced useful output. That creates false validation failure evidence and
    slows convergence.
- Code closure:
  - `shell_runtime_adapter.go` now normalizes `sh -c` scripts that use
    `${PIPESTATUS[...]}` to bash, alongside the existing `pipefail`
    normalization.
  - The normalization is syntax/feature based and remains core-neutral: it does
    not inspect programming language, test framework, workspace template,
    provider, file name, or command marker semantics.
  - Existing command policy checks run after normalization against the actual
    executable and script args.
- Commands run:
  - `gofmt -w internal/modules/blocks/shell/biz/shell_runtime_adapter.go internal/modules/blocks/shell/biz/shell_runtime_adapter_test.go`
  - `go test ./internal/modules/blocks/shell/biz -run 'TestRuntimeAdapter_ExecuteRuntimeStepNormalizesShPipeStatusScriptToBash|TestRuntimeAdapter_ExecuteRuntimeStepNormalizesShPipefailScriptToBash' -count=1`
  - `go test ./internal/modules/blocks/shell/biz -count=1`
  - `./scripts/verify-boundary-contract.sh`
- Not closed by this record:
  - The latest deployed service has not yet been rebuilt with this shell block
    fix.
  - Positive live goal convergence and full real-provider resource verification
    still require redeploy plus fresh bounded observation.

## 2026-06-22 B7-G2 Current Packet Managed CI-Safe Reverification

- Closed gate: current local closure packet verification for CI-safe deployed
  paths.
- Evidence:
  - `ev:verify/managed-ci-safe/20260622-006`
- Verification scope:
  - Formatting, console build, `go vet ./...`, targeted Go tests for shell,
    LLM, and orchestrator packages, workorch binary build, managed workorch
    service startup, HTTP health/API checks, workspace template/Soul e2e, mock
    LLM workspace execution, and MCP stdio workspace execution.
  - The verifier manages its own temporary service instance and does not depend
    on the user-deployed `127.0.0.1:8080` service.
- Bounded output:
  - Managed service: `http://127.0.0.1:35188`.
  - `verify-ci-mvp-file` passed.
  - MCP stdio workspace completed `run-001` with `steps=3`, `toolCalls=3`,
    `validations=3`, and `runtimeCheckpoints=26`.
  - Artifacts retained at `/tmp/tmp.vQ618dTrHL`.
- Not closed by this record:
  - CI-safe mock verification does not prove real-provider live goal
    convergence.
  - Full real-provider resource verification still requires configured
    external credentials and bounded run output.

## 2026-06-22 B7-G3 Post-Managed Live Convergence Observation

- Gate checked: `B7-G3` live goal-convergence observation after the current
  local packet had passed managed CI-safe verification.
- Evidence:
  - `ev:verify/live-convergence/20260622-007`
- Bounded output:
  - Service and projection were ready with two observed workspaces.
  - Both workspaces were still `blocked_on_decision` on `run-002`, each with
    one open decision, `ineffectiveRetry=true`, a latest rejected result, and
    coverage still missing `validation,state_change`.
  - pprof was available and found no synchronous runtime send stack.
  - Runtime event publish failure deltas were zero.
  - Resource envelope remained bounded during the 45s sample: CPU `1.42%`,
    RSS `350994432`, read delta `0`, write delta `4096`.
  - No SSE events were captured because both active runs were waiting on
    decisions rather than running new work.
- Interpretation:
  - This observation confirms live goal convergence remains open.
  - It does not prove a new code defect in the current local packet because the
    latest workspace results were historical decision-blocked states, not new
    post-fix execution output.
  - The next live gate must explicitly resolve/replan/resume the open
    decisions and then observe the resulting fresh runs.
- Not closed by this record:
  - Positive live goal convergence.
  - Full real-provider resource verification.

## 2026-06-22 B7-G3 Fresh Replan Budget Reset Closure

- Closed gate: live replan continuation state-machine defect found after
  explicit replan/resume.
- Evidence:
  - `ev:verify/live-convergence/20260622-008`
  - `ev:code/replan-budget-reset/20260622-009`
- Observed defect:
  - Both historical open decisions were explicitly resolved with
    `resolutionType=replan`, creating fresh `run-002-replan-001` successors.
  - `ws-4e...` resumed live execution and emitted LLM stream progress, so the
    replan command path, projection, and runtime dispatch path were active.
  - `ws-7c...` produced a new validation failure with actionable diagnostics,
    but the newly created replan successor carried the predecessor's already
    escalated convergence budget (`repairAttempts 4/4`,
    validation/no-delta counters exhausted).
  - Because the successor inherited the exhausted budget, the first fresh
    post-replan validation failure immediately opened a repair-exhausted
    decision instead of allowing the new replan attempt to use its own retry
    budget.
- Code closure:
  - Replan successor creation now resets runtime convergence counters,
    repeated failure signature state, no-delta state, and escalated decision
    reasons.
  - The reset preserves generic budget limits and current focus/slice scope so
    the successor keeps policy configuration without inheriting exhausted
    predecessor runtime state.
  - The change is core-neutral: it operates only on structured convergence
    budget fields and does not inspect workspace title, template, language,
    command output marker, file name, provider, or MCP server.
- Commands run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go internal/modules/kernel/orchestrator/biz/runtime_run_state.go internal/modules/kernel/orchestrator/biz/runtime_repair_resume_integration_test.go internal/modules/kernel/orchestrator/biz/runtime_convergence_budget_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestResetConvergenceBudgetForReplanSuccessorClearsRuntimeCounters|TestRuntimeOrchestrator_RunningResumeReplansFromResolvedDecisionOnFailedNonTerminalStep|TestRuntimeOrchestrator_BlockerAutoResumePolicyMatrix|TestUpdateConvergenceBudget' -count=1`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
- Not closed by this record:
  - Positive live goal convergence after redeploy.
  - Full real-provider resource verification.
  - High event-record IO deltas observed during the 90s live sample; no
    threshold was configured for that run, so it remains a follow-up resource
    gate rather than this state-machine closure.

## 2026-06-22 B7-G2 Current Packet Managed CI-Safe Reverification After Budget Reset

- Closed gate: current local budget-reset closure packet verification for
  CI-safe deployed paths.
- Evidence:
  - `ev:verify/managed-ci-safe/20260622-010`
- Verification scope:
  - Formatting, console build, `go vet ./...`, targeted Go tests for shell,
    LLM, and orchestrator packages, workorch binary build, managed workorch
    service startup, HTTP health/API checks, workspace template/Soul e2e, mock
    LLM workspace execution, and MCP stdio workspace execution.
  - The verifier managed its own temporary service instance and did not depend
    on the user-deployed `127.0.0.1:8080` service.
- Bounded output:
  - Managed service: `http://127.0.0.1:32668`.
  - `verify-ci-mvp-file` passed.
  - MCP stdio workspace completed `run-001` with `steps=3`, `toolCalls=3`,
    `validations=3`, and `runtimeCheckpoints=26`.
  - Artifacts retained at `/tmp/tmp.Ynb8y5h8uD`.
- Not closed by this record:
  - Positive live goal convergence on the user-deployed service.
  - Full real-provider resource verification.

## 2026-06-22 B7-G3 Runtime Validation Scratch Preparation Closure

- Closed gate: focused runtime dispatch defect exposed by fresh live
  post-redeploy execution.
- Evidence:
  - `ev:verify/live-convergence/20260622-011`
  - `ev:code/runtime-validation-scratch-prepare/20260622-012`
- Observed defect:
  - After redeploy and explicit `replan` for both open decisions, both
    workspaces created fresh `run-002-replan-001-replan-001` successors.
  - `ws-4e...` failed a shell validation step before useful validation could
    run because the command attempted to `cd` into the structured
    runtime-provided `validationScratchDir`, but runtime dispatch had not
    prepared that directory.
  - The failing step carried `validationScratchDir` in structured
    `repairActionPackage.requiredOutput`; the code closure does not parse shell
    command text and does not depend on workspace title, target language,
    template ID, provider, MCP server, or command name.
- Code closure:
  - Runtime dispatch now prepares `validationScratchDir` before sending a
    runtime request, including request-only/event-driven runtime block paths.
  - Scratch creation is restricted to the runtime-owned
    `os.TempDir()/workorch-validation-scratch` root. Sibling or arbitrary
    absolute paths are rejected before side effects.
  - The prepared path is mirrored back into the runtime request input so blocks
    receive the same structured scratch contract that the LLM saw.
- Commands run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go internal/modules/kernel/orchestrator/biz/runtime_validation_diagnostic_test.go internal/modules/kernel/orchestrator/biz/runtime_mainline_integration_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestEnsureRuntimeValidationScratchDirCreatesOnlyControlledRoot|TestRuntimeOrchestrator_DispatchPreparesValidationScratchBeforeRuntimeRequest|TestRuntimeOrchestrator_DispatchStepRequestsRuntimeExecutionWithoutWaitingForResult|TestResetConvergenceBudgetForReplanSuccessorClearsRuntimeCounters' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s`
  - `./scripts/verify-boundary-contract.sh`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - `boundary contract verification passed`
- Not closed by this record:
  - Positive live goal convergence after redeploy of this scratch preparation
    closure.
  - Full real-provider resource verification.
  - Event-record hydrate/write pressure observed in
    `ev:verify/live-convergence/20260622-011`; no threshold was configured in
    that observation, so it remains a resource gate.

## 2026-06-22 B7-G4 Run Progress Lifecycle Preservation Closure

- Closed gate: focused run lifecycle overwrite defect exposed by the latest
  post-redeploy live execution.
- Evidence:
  - `ev:verify/live-convergence/20260622-013`
  - `ev:code/run-progress-lifecycle-preserve/20260622-014`
- Observed defect:
  - `ws-4e...` created an open repair-exhausted decision and heartbeat
    projected `waiting_decision`, but the owner run file still showed
    `State=running` with an empty `BlockingTicketID`.
  - The owner workspace file was already `waiting_decision`, so the mismatch
    was not a TUI-only issue.
  - The likely live path is parallel graph execution: one step blocks the run,
    while another step with a stale run snapshot later persists coverage or
    reconciliation progress and overwrites lifecycle fields.
- Code closure:
  - Non-lifecycle run progress writes now use a scoped owner read before
    update.
  - If the owner run is already blocked or terminal, the progress write
    preserves `State`, `BlockingTicketID`, `BlockingApprovalID`, and
    `FailureReason` while still persisting updated execution-context progress.
  - Lifecycle transitions remain in the existing state-machine paths.
- Commands run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_run_state.go internal/modules/kernel/orchestrator/biz/runtime_run_state_subsystem_test.go internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go internal/modules/kernel/orchestrator/biz/runtime_repair.go internal/modules/kernel/orchestrator/biz/runtime_graph.go internal/modules/kernel/orchestrator/biz/runtime_result_recording.go internal/modules/kernel/orchestrator/biz/runtime_artifact_assemble.go internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestratorPersistRunProgressUpdatePreservesBlockedDecisionLifecycle|TestRuntimeOrchestrator_RuntimeRepairExhaustion|TestRuntimeOrchestrator_ResumeRunningFailedDoDCoverageRepairExhaustionBlocksRun' -count=1 -timeout=120s`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s`
  - `./scripts/verify-boundary-contract.sh`
  - `git diff --check`
- Bounded output:
  - `ok github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz`
  - `boundary contract verification passed`
- Not closed by this record:
  - Positive live goal convergence after redeploy of this lifecycle preservation
    closure.
  - Full real-provider resource verification.
  - CPU/RSS and event-record hydrate/write pressure still need bounded live
    observation after redeploy.

## 2026-06-22 B7-G4 Workspace Snapshot LLM Usage Fan-Out Closure

- Closed gate: focused resource-envelope code closure for workspace snapshot
  read-model assembly.
- Evidence:
  - `ev:code/workspace-snapshot-llm-usage-fanout/20260622-026`
- Observed risk:
  - The live service remained CPU-bounded, but cumulative event-record payload
    hydration counters were high during multi-run live execution.
  - Code audit found that workspace snapshot assembly loaded bounded
    workspace-scoped LLM usage and then loaded run-scoped LLM usage for every
    historical run in the workspace.
  - That fan-out is unnecessary for the high-frequency workspace
    projection/detail path because the selected projection run is the only run
    that needs run-scoped fallback usage in the default ViewModel.
- Code closure:
  - `loadWorkspaceSnapshotModels` now selects the projection run before loading
    run-derived read-model items.
  - `loadRunDerivedItems` keeps existing artifact continuation and patch reads,
    but limits run-scoped LLM usage event hydration to the selected projection
    run.
  - Workspace-scoped latest LLM usage remains loaded once, and explicit run
    observability/detail endpoints still own historical run drill-down.
- Boundary result:
  - The closure is projection/readmodel IO infrastructure.
  - It does not branch on workspace template ID, Soul ID, live workspace ID,
    provider, programming language, filenames, LSP, or business-specific text.
- Verification:
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -run 'TestWorkspaceSnapshot(BuildsSnapshotFromReaders|MergesRunScopedLLMUsageEvents|LoadsRunScopedLLMUsageOnlyForProjectionRun|UsesCachedRunSnapshotForProjectionRun)' -count=1 -timeout=180s`
  - `go test ./internal/modules/application/workorchd/biz/appservice/... -count=1 -timeout=300s`
  - `./scripts/verify-boundary-contract.sh`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md`: `evidence refs consistent`.
  - `git diff --check`
- Result:
  - Focused readmodelports regression and appservice package regression passed.
  - Boundary contract verification passed; the change is projection/readmodel IO and did not add business-specific branching to core.
  - Evidence references remained complete.
- Not closed by this record:
  - Positive live goal convergence.
  - Full real-provider resource verification.
  - A fresh post-redeploy resource observation with explicit event IO delta
    thresholds.

## 2026-06-23 B7-G3 Coverage Assessment Required Output Correction Closure

- Closed gate: focused live convergence correction defect for contract
  violation retries.
- Evidence:
  - `ev:code/coverage-assessment-required-output-correction/20260623-027`
- Observed blocker:
  - Latest deployed observation had both live workspaces in
    `waiting_decision` on `dod_assessment_missing_evidence_refs`.
  - The failed steps' `expectedContract.repairActionPackage.requiredOutput`
    required evidence acquisition or state-changing output, but the correction
    profile narrowed the retry to `traceable_coverage_assessment`.
  - This created a contract mismatch: the retry feedback could tell the LLM to
    rewrite an assessment when the current stage still required an executable
    evidence graph or state-changing output.
- Code closure:
  - Coverage / DoD-assessment contract violations now inspect the structured
    `repairActionPackage.requiredOutput` before selecting a correction profile.
  - If the current required output demands state-changing work, the retry keeps
    `state_changing_output`.
  - If it demands concrete validation evidence, the retry keeps
    `bounded_validation_evidence`.
  - If it demands concrete failure evidence acquisition, the retry keeps
    `bounded_failure_evidence`.
  - Pure assessment-stage coverage violations still use
    `traceable_coverage_assessment`.
- Boundary result:
  - The closure is based only on structured runtime contract fields. It does
    not branch on workspace ID, template ID, Soul ID, provider, language,
    filenames, tool names, or business text.
- Verification:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolation(CorrectionProfilesCoverPrimaryRules|CoverageAssessmentPreservesCurrentRequiredOutputShape|RecoveryActionRetriesWithinBudget|RuntimeRepairInputIncludesContractViolationRecovery)' -count=1 -timeout=240s`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s`
  - `go test ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s`
  - `./scripts/verify-boundary-contract.sh`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md`: `evidence refs consistent`.
  - `go build -o ./bin/workorch ./application/workorch`
  - `git diff --check`
- Result:
  - Targeted and package regressions passed.
  - Boundary contract verification passed; the change is driven by structured runtime contract fields only.
  - Evidence references remained complete, diff whitespace was clean, and `./bin/workorch` rebuilt successfully.
- Not closed by this record:
  - Fresh post-redeploy positive live convergence after explicit replan/resume.
  - Full real-provider resource verification.

## Batch 7 Decision

Batch 7 verification inventory is audit-complete for the current evidence packet. Unit, boundary, evidence-reference verification, targeted mock LLM deployed e2e, full CI-safe managed deployed e2e, managed mock resource loadtest, live projection observation, and real-runtime preflight have now been executed for the current code closure packet. Live goal convergence and full real-provider resource verification remain pending.

The audited result is:

- CI-safe verification is managed and deterministic, but intentionally narrow.
- Full e2e, real-provider, TUI visual/terminal, and resource/load verification targets exist; CI-safe mock deployed, mock loadtest, live projection observation, and real-runtime preflight tiers have been executed, while real-provider execution and live goal-convergence tiers remain pending.
- Boundary verification is present and should remain mandatory for core runtime/template boundary changes.
- Unit regression inventory is broad, and the current code closure packet has explicit bounded unit outputs.
- CI-safe managed deployed verification passed for HTTP, workspace template/Soul, mock LLM, and MCP stdio paths; targeted mock LLM verification also passed for the contract retry exhaustion decision path.
- The current local closure packet reran CI-safe managed verification in
  `ev:verify/managed-ci-safe/20260622-006`, including shell, LLM, orchestrator
  targeted package tests before managed service startup.
- Post-managed live convergence observation in
  `ev:verify/live-convergence/20260622-007` still failed because both live
  workspaces were waiting on historical open decisions; projection, pprof,
  EventHub publish failure counters, and resource envelope were healthy.
- Fresh explicit replan observation in
  `ev:verify/live-convergence/20260622-008` proved the command path can create
  fresh successor runs, and exposed a core state-machine defect: replan
  successors inherited exhausted convergence budgets. The local closure in
  `ev:code/replan-budget-reset/20260622-009` resets runtime budget counters
  for replan successors while preserving policy limits and scope.
- Current packet managed CI-safe verification in
  `ev:verify/managed-ci-safe/20260622-010` passed after the replan budget reset
  closure.
- Post-redeploy live convergence observation in
  `ev:verify/live-convergence/20260622-011` proved fresh replan successors can
  run, but exposed an additional core runtime dispatch defect: structured
  validation scratch directories were contracted but not prepared before
  runtime execution. The local closure in
  `ev:code/runtime-validation-scratch-prepare/20260622-012` prepares only
  runtime-owned scratch directories before runtime request publication and
  rejects arbitrary scratch roots.
- Latest post-redeploy live extraction in
  `ev:verify/live-convergence/20260622-013` exposed a parallel graph progress
  overwrite defect: `ws-4e...` had an open decision and workspace
  `waiting_decision`, while the owner run had been overwritten back to
  `running`. The local closure in
  `ev:code/run-progress-lifecycle-preserve/20260622-014` makes non-lifecycle
  progress writes preserve owner blocked/terminal lifecycle fields.
- The next redeploy after lifecycle preservation confirmed the forward-path
  overwrite fix was not enough to repair already-corrupted historical owner
  state: `ws-4e...` still had workspace owner `waiting_decision`, heartbeat
  `activityState=waiting_decision`, and one open decision for the active run,
  while run owner remained `running` with no `BlockingTicketID`. The local
  closure in
  `ev:code/startup-governance-blocker-reconcile/20260622-015` adds
  business-neutral startup reconciliation for unique open decision tickets and
  pending approvals on the active run.
- Post-redeploy verification in
  `ev:verify/live-startup-governance-reconcile/20260622-016` confirmed the
  startup reconciliation path on the deployed service: `ws-4e...` is now
  projected as `waiting_decision`, its active run owner is
  `blocked_on_decision` with the selected open ticket, and snapshot top-level
  `decisions` includes the ticket detail. This closes the historical
  owner-state inconsistency exposed by `ev:verify/live-convergence/20260622-013`.
- The same live check exposed a separate global governance read-model defect:
  workspace snapshots contained scoped open decisions, but global
  `GET /api/v1/workorch/decisions` returned HTTP 409. The local closure in
  `ev:code/global-decision-page-scoped-resolution/20260622-017` scopes
  resolution reads and internal resolution map keys by workspace plus ticket
  ID, so historical duplicate ticket IDs across workspaces no longer break
  the global DecisionQueue projection. This requires redeploy before the live
  endpoint can be marked verified.
- Follow-up code audit found the same scope problem in governance pagination
  cursors: global decision and approval pages used only bare ticket/approval
  IDs as cursor positions. `ev:code/global-governance-page-scoped-cursor/20260622-018`
  changes the page cursor to an opaque workspace-scoped value so duplicate
  governance IDs across workspaces cannot cause repeated or skipped pages.
  The deployed service observed during this audit was still running an older
  binary than the scoped-resolution source edits, so live endpoint verification
  remains pending after rebuild/restart.
- Managed mock resource loadtest passed for two concurrent governed workspaces and zero transport/protocol/timeline errors.
- Live projection observation passed for display contract, scheduler/watchdog metrics, and pprof availability, but both observed workspaces remain waiting on decision with ineffective retry markers.
- Live replan observation proved governance-continuation command submission can
  move both blocked workspaces back to running, but the post-replan live LLM
  outputs still failed contract convergence. The latest code closures clarify
  the evidence-acquisition deliverable-write boundary, preserve runtime MCP
  capability context in LLM inputs, reject undeclared MCP server IDs, reject
  structurally declared non-substantive state changes without parsing
  natural-language placeholder/deferral intent in core, and dedupe coverage
  targets by requirement identity. They require redeploy plus fresh bounded
  observation before live convergence can be claimed.
- The latest bounded live continuation observation after redeploy showed both
  active runs still running with current LLM provider-call progress, no open
  governance blockers, no projection staleness, no EventHub synchronous-send
  stack, and bounded CPU/IO. It does not close goal convergence, but it narrows
  the next audit point to provider-returned runtime output rather than
  scheduler/TUI/projection plumbing.
- After provider output returned, a focused code closure corrected the
  structured coverage/evidence state-change contract so supplemental profile
  evidence targets such as runtime-declared write operations can be accepted
  without reopening already-satisfied primary state-change work.
- A focused shell block closure now normalizes bash-only pipeline status syntax
  in `sh -c` validation scripts to bash execution, avoiding false `Bad
  substitution` validation failures without adding language or test-framework
  rules to core.
- Real-runtime preflight passed for service readiness and scheduler/watchdog metrics; real-provider execution remains unverified.
- Post-redeploy live snapshots showed both active workspaces blocked on
  `repair_focus_mismatch` after fresh replan successors. The runtime contract
  had already narrowed the step to structured evidence acquisition for the
  current coverage focus, but provider outputs omitted repeated
  `coverageFocusId` / `coverageWorkItemIds` metadata on `shell/command` or
  `mcp/tool_call` evidence nodes. The local closure in
  `ev:code/focus-bound-evidence-graph-normalization/20260623-028` normalizes
  only non-`patch/apply` evidence nodes from the structured current focus when
  state-changing output is not required. `patch/apply` remains strict and must
  declare focus binding itself. Redeploy plus explicit replan/resume is still
  required to verify live convergence past this blocker.
- The next redeploy/replan verified that the focus-bound evidence normalization
  moved both workspaces past the previous blocker. It then exposed a separate
  convergence-budget accounting defect: completed LLM steps that only returned
  a pending `executionGraph` consumed `noCoverageDeltaRounds` before their
  graph nodes could complete, and a mechanical stale `patch/apply` `oldText not
  found` conflict consumed the same budget instead of staying on the patch
  rebuild path. The local closure in
  `ev:code/convergence-budget-graph-pending-and-stale-patch/20260623-029`
  keeps LLM rounds and failure signatures visible while preventing pending
  graph plans and stale patch conflicts from prematurely exhausting no-delta
  budget.
- The same live verification exposed a correction-contract wording defect for
  verification-stage retries: runtime correctly rejects continuable decision
  blockers, but the retry correction contract still listed bare
  `blocked_on_decision` as an allowed output. The local closure in
  `ev:code/contract-correction-governance-blocker-shape/20260623-030`
  preserves real governance blockers by requiring a declared external or
  governance dependency, and explicitly forbids decision requests for
  continuable candidate choices.
- After redeploy with the current binary and explicit scoped `replan`, both
  live workspaces created fresh successor runs and produced real patch and
  validation activity. The new run then exposed a convergence-budget accounting
  defect: successful intermediate graph nodes that cannot independently move
  coverage, such as read and patch nodes without `validationEvidence`, consumed
  `noCoverageDeltaRounds` before downstream validation/result recording could
  close coverage. The local closure in
  `ev:code/convergence-budget-intermediate-graph-nodes/20260623-031` skips
  no-delta accounting for completed non-validation graph nodes while preserving
  accounting for validation evidence nodes and failed steps.
- The next redeploy plus scoped `replan` proved the governance continuation
  path still creates fresh successor runs for both live workspaces. `ws-4e...`
  then exposed a contract consistency defect rather than a scheduler defect:
  the runtime validator enforced `coverageFocusBindingRequired`, but rebuilt
  `repairActionPackage.requiredOutput` no longer exposed the matching
  `mustBindCoverageFocus`, `coverageFocusId`, `coverageWorkItemIds`, or binding
  rule. The local closure in
  `ev:code/focus-contract-required-output-preservation/20260623-032` reapplies
  focus binding after requiredOutput rebuilds and recognizes focus tokens in
  neutral tool-contract fields. `ws-7c...` remains blocked on a distinct
  concrete-validation safety issue where provider output uses background
  service launch plus process termination; that requires separate bounded
  service validation harness design or template guidance and is not closed by
  `032`.
- The subsequent redeploy/replan proved the focus requiredOutput preservation
  is active: `ws-4e...` produced real patch activity instead of another
  focus-mismatch failure. The new blocker is a separate graph-node contract
  propagation defect: the first repair graph node satisfied the original
  primary diagnostic, but the second same-scope graph node was still evaluated
  against the stale primary diagnostic in isolation and escalated to decision.
  The local closure in
  `ev:code/graph-node-primary-diagnostic-dependency-evidence/20260623-033`
  allows later same-scope graph-node patches only when completed dependency
  graph evidence already directly satisfies the primary diagnostic contract.
- The next redeploy verified that the `033` blocker did not recur:
  `ws-4e...` created a fresh successor run and moved through additional patch
  and validation activity. It then exposed a distinct resource-shape context
  gap: the actionable shape diagnostic was inherited through repair input and
  `repairActionPackage`, while the failed step output only contained wrapper
  failure metadata. The local closure in
  `ev:code/shape-alignment-context-from-inherited-diagnostics/20260623-034`
  makes resource-shape alignment use inherited structured diagnostics and
  inherited target scope as neutral input evidence, so retry inputs can include
  same-directory resource-shape snapshots instead of forcing the provider to
  guess names or attributes.
- After redeploying `034`, scoped `replan` moved `ws-4e...` into a fresh
  successor run and produced real file changes, so the shape-alignment blocker
  was passed. The next blocker was a stale text replacement where the failed
  patch's `oldText` was no longer present but its `newText` was already present
  in the current target. Existing repair input code knew how to turn that
  condition into `currentTargetAlreadySatisfiesFailedPatch` and a validation
  contract, but recovery action selection escalated to repair-exhausted /
  duplicate-failure decision before that input could be built. The local
  closure in
  `ev:code/already-satisfied-stale-patch-bypasses-exhaustion/20260623-035`
  makes those governance gates yield to the mechanical already-satisfied patch
  signal, preserving the existing validation path.
- The next live snapshot proved `035` moved the run past the already-satisfied
  stale text replacement. It then exposed a different stale patch path:
  `patch/apply` failed with `conflict: patch baseHash mismatch` because the
  same target had already changed after the failed patch's base hash was
  captured. Current target evidence existed and was sufficient to rebuild the
  patch from a fresh `contentHash`, but recovery action selection escalated to
  `runtime_repair_exhausted` before the existing patch repair context could be
  used. The local closure in
  `ev:code/stale-basehash-patch-refresh-bypasses-exhaustion/20260623-036`
  adds a narrow refreshable stale-patch signal for readable current targets and
  lets it bypass exhaustion/duplicate gates into runtime repair. Missing-target
  or non-refreshable stale hash cases still escalate at budget.
- After `036`, the successor run advanced through real graph and MCP
  validation activity until graph follow-up expansion reached its limit. The
  next contract retry still carried the active `coverageFocus`, but the final
  `repairActionPackage.requiredOutput` had lost `mustBindCoverageFocus`,
  `coverageFocusId`, and `coverageWorkItemIds`, so provider output was rejected
  with `repair_focus_mismatch`. The local closure in
  `ev:code/final-repair-output-decision-refresh-preserves-focus/20260623-037`
  reapplies the coverage-focus contract during the final output-decision
  refresh after convergence/output-mode projections rebuild `requiredOutput`.
- After `037`, the redeployed successor run advanced through real patch,
  build, git, and MCP validation activity and no longer repeated the focus
  mismatch. The next open decision was a concrete-validation graph rejection:
  a scratch-bounded heredoc validation script contained a non-shell comparison
  token (`>=`) inside fixture text, and the generic shell safety scanner
  treated the `>` byte as a write redirection outside `validationScratchDir`.
  The local closure in
  `ev:code/validation-shell-redirection-comparison-token/20260623-038` keeps
  shell validation strict for real redirections while ignoring neutral
  comparison/arrow tokens such as `>=`, `=>`, `->`, and `!>` when scanning
  command text. The closure is core-neutral: it changes only generic command
  parsing and does not add any template, language, workspace, or target-file
  rule.
- After redeploying `038`, live `ws-4e...` verified that the previous scratch
  validation scanner blocker did not recur: multiple scratch setup and
  path-conversion validation nodes completed. The successor run then exposed a
  separate coverage-ledger binding gap. A focus-bound graph completed
  state-changing `git/write` add/commit nodes, but those nodes did not carry
  `coverageFocusId` / `coverageWorkItemIds`, so the coverage work item lacked
  the completed state-change evidence refs and the next assessment escalated
  to `dod_assessment_missing_evidence_refs`. The local closure in
  `ev:code/focus-bound-state-change-write-evidence/20260623-039` makes
  coverage focus normalization and validation use the generic
  `stepKindCanProduceStateChangingEvidence` rule, so any state-changing
  evidence-producing node such as `*/write` must bind to the current focus or
  be normalized when the step is eligible. The closure is core-neutral: it does
  not add workspace IDs, template IDs, language rules, file names, provider
  names, or target-project behavior.
- A redeploy of `039` proved the scoped replan path itself worked: the open
  decision resolved into a fresh successor run and entered LLM repair. That
  run immediately exposed the remaining normalization boundary. The repair
  graph had a focus-bound primary `patch/apply`, but dependent non-patch
  action/evidence nodes such as `mcp/tool_call` and `git/write` still lacked
  explicit focus fields. Because strict state-changing repair disabled all
  normalization, the graph failed with `repair_focus_mismatch` before those
  state-change evidence refs could be recorded. The local closure in
  `ev:code/strict-repair-nonpatch-focus-normalization/20260623-040` keeps
  `patch/apply` strict in state-changing repair, while allowing non-patch
  state-changing or validation action nodes to inherit the unambiguous current
  coverage focus. This preserves the guard against silently redirecting a
  patch to the wrong target and still lets the coverage ledger bind follow-up
  write/tool evidence to the active work item.
- After redeploying `040`, the fresh successor run no longer failed with
  `repair_focus_mismatch`. It advanced through real patch and validation
  nodes and `cwi-039` gained new accepted state-change and validation evidence
  refs. The next blocker was a different target validation failure, but the
  convergence budget still escalated on `repairAttempts 4/4` because the
  focus-level repair counter had not been reset after accepted coverage
  progress. The local closure in
  `ev:code/convergence-budget-positive-delta-resets-repair-window/20260623-041`
  resets repair-attempt, validation-only retry, and repeated-failure counters
  only when a completed step produces accepted coverage delta. Failed steps
  that merely add rejected diagnostic evidence keep their repair counters, so
  the budget still blocks non-progressing retry loops while allowing a
  genuinely progressing focus to continue repairing the next concrete
  validation failure.
- After redeploying `041`, the scoped successor run proved positive coverage
  progress now resets the local repair window: it advanced through additional
  patch and validation nodes instead of immediately reusing the stale exhausted
  budget. The next decision exposed a projection consistency defect rather
  than a target-project rule. The active `coverageFocus` pointed at an
  `evidence_ready` coverage work item, but the compact
  `coverageWorkItemContext` match came from `ledgerSummary`, where by-kind
  evidence ledgers are intentionally omitted to control LLM context size.
  Recomputing `missingEvidenceKinds` from that compact item reopened the item
  as missing `validation` and `state_change`, which forced more
  state-changing repair and consumed `llmRounds` instead of moving toward
  assessment. The local closure in
  `ev:code/evidence-ready-compact-ledger-does-not-reopen-missing-evidence/20260623-042`
  keeps complete by-kind ledgers authoritative when present, but treats
  compact `evidence_ready` / `satisfied` work-item summaries with no by-kind
  ledger as already evidence-complete. This is core-neutral: it preserves
  strict missing-evidence calculation for `open`, `in_progress`, and `failed`
  items and does not add workspace IDs, template IDs, language rules, provider
  names, commands, or target-file rules.
- Batch 7 cannot mark runtime convergence complete until live workspace goal convergence and real-provider resource tiers have bounded pass/fail output; optional full deployed or TUI tiers must be run when a later closure touches those paths.
- The next live observation must trigger or accompany an explicit
  replan/resume of the open decisions; observing unchanged decision-blocked
  runs is useful stale-state evidence but cannot prove fresh runtime
  convergence.

Do not treat the presence of a Makefile target or test inventory as proof of closure. Remaining Batch 7 closure requires bounded live goal-convergence summaries and real-provider resource observations for the specific gates being verified; broader deployed/TUI/real-provider targets require actual bounded output when selected.
