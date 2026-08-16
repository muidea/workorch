# Runtime Contract Drift Closure

Status: completed

Type: implementation-closure-plan

Last Verified: 2026-08-16

## Closed Problem

The documentation refresh found executable content that still described removed production surfaces even though component
README and core contracts were already single-owner/Runtime-only. This was implementation-content drift, not a documentation
authority exception.

## Completed Scope

### CD-1 Default Template And Soul Guidance

- Removed provider-facing execution-graph, next-action, execution-context, repair-package and automatic-replan authority.
- Replaced it with typed semantic-plan, operations, sealed binding, owner receipt, evidence and assessment guidance.
- Removed the template stage `entry`/`nextAction` schema and dead graph/artifact LLM-output settings.
- Added default-resource assertions that reject retired authority vocabulary.
- Verified stable provider prefix behavior after the embedded-resource change.

Affected SM: SM-2 semantic definition/focus inputs and SM-3 provider document boundary. No transition was added.

### CD-2 Verification And Example Surfaces

- Removed direct Run write-route registration; Run HTTP endpoints are read-only and workspace `execution-activations` is the
  sole normal execution mutation ingress.
- Replaced graph/context/replan verifiers with current semantic seal, dispatch, receipt, evidence, assessment, governance and
  terminal-delivery tests.
- Added `scripts/lib/go-test-must-run.sh` so focused verifiers reject Go's successful `[no tests to run]` result.
- Removed two unreferenced obsolete verifiers and corrected stale package/test paths in active Make targets.
- Kept `nextAction` only in explicitly display-only compatibility checks.

Affected SM: SM-1 ingress/restart, SM-2 evidence/terminal, SM-3 candidate/dispatch and SM-4 recovery/delivery.

### CD-3 Container Packaging

- Docker builder now copies embedded console assets and separates `go mod download` into a cacheable layer.
- Docker build accepts an explicit `GOPROXY`; Make/container verification also accept an explicit build-network override.
- The container verifier now creates Workspace, Goal and DoD through current APIs, submits a typed restart activation, checks
  removed Run mutations return `404`, and verifies both ephemeral and mounted file-backed persistence.
- Clean image build and both storage modes passed on 2026-08-16.

This did not alter a Runtime state machine or owner boundary.

### CD-4 Recurrence Guards

`scripts/verify-boundary-contract.sh` now rejects:

- forbidden Runtime authority surfaces in runtime contracts, Orchestrator and activation ports;
- retired provider keys in embedded defaults and active scripts;
- `GapNextAction` leakage into Runtime/owner authority and unclassified script use;
- direct Run POST routes or a missing execution-activation ingress;
- retired graph/artifact output configuration fields;
- nonexistent literal Go package paths in scripts/Make;
- focused closure scripts that do not fail closed on empty test selection, or positive-convergence selector prefixes that no
  longer resolve to current tests;
- Governance/application code that directly creates, resumes or drives Runtime work.

Read-only compatibility projection remains permitted only outside owner/reducer authority.

## Exit Evidence

- `make release-check`
- `make console-build`
- `./scripts/verify-boundary-contract.sh`
- `./scripts/verify-runtime-positive-convergence-file.sh`
- `./scripts/verify-workspace-file-diff-patch.sh`
- `./scripts/verify-auto-governance-ticket-approval.sh`
- `./scripts/verify-acceptance-slice-result-deployed-e2e.sh`
- clean Docker image build
- container verification in `ephemeral` and `persistent` storage modes

Detailed evidence is in
[Runtime Contract Drift Code Closure Record](../records/runtime-contract-drift-code-closure-record-2026-08-16.md).

## Non-Goals

This closure does not close real-provider positive convergence or Workspace reasoning-effort deployed verification. Those
remain separate active queues and cannot be inferred from deterministic/container evidence.
