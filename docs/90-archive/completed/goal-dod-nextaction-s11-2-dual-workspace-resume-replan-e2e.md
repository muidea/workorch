# S11.2 Dual Workspace Resume/Replan E2E

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [Run Resume And Gap Confirmation Design](../../30-module-architecture/run-resume-gap-confirmation-design.md)
- [EventHub Driven Workorchd State Loop Design](../../30-module-architecture/eventhub-driven-workorchd-state-loop-design.md)
- [S1.6 TUI/REST Command Path Parity](./goal-dod-nextaction-s1-6-command-path-parity.md)
- [S11.1 Managed E2E Harness](./goal-dod-nextaction-s11-1-managed-e2e-harness.md)

## Contract Audit

- Scope: deployed or managed WorkOrch service handling two workspace-scoped `resume` and `replan` run commands concurrently.
- Observed state: S11.1 provides a managed `./bin/workorch` harness. `verify-run-resume-gap-confirmation-e2e.sh` validates one workspace `resume` projection and optional decision `replan`. `loadtest-governed-workspace-concurrency.py` validates concurrent workspace execution, but it does not explicitly validate paired `resume` and `replan` command projections for two workspace scopes.
- Expected state: two workspaces can submit `resume` and `replan` commands at the same time through the REST command path. Each command is accepted with its own workspace scope, request id, run id, and projection revision. A command for one workspace must not overwrite or block the other workspace projection.
- Deviation: the existing e2e set did not provide a deterministic dual-workspace resume/replan regression gate. A future regression could reintroduce unscoped command handling or shared projection state without being caught by the single-workspace script.
- Owner: e2e. This slice validates existing command -> owner -> projection behavior and does not add core runtime, template, Soul, TUI, Web, or LLM provider behavior.
- Event path: managed harness or deployed service -> create two temporary workspaces -> define Goal/DoD for each -> concurrently POST workspace `execution-activations` with `intent=resume` -> scheduler command accepted -> workspace projection `runCommand.intent=resume` -> concurrently POST run `resume` with `intent=replan` -> scheduler command accepted -> workspace projection `runCommand.intent=replan`.
- Ledger impact: none. The script only verifies command acceptance, scope, projection revision, and gap baseline presence for resume. It does not mark coverage satisfied and does not inspect business evidence.
- Stage Packet impact: none. The script does not change LLM request rendering or stage packet construction.
- Contract gap: missing dual-workspace e2e coverage for scoped run commands and projection isolation.
- Proposed change: add `scripts/verify-dual-workspace-resume-replan-e2e.sh`, expose it as `make verify-dual-workspace-resume-replan-e2e`, and include it in `verify-e2e-full`.
- Boundary check: the change is limited to e2e script, Makefile wiring, and roadmap documentation. It does not add template, language, command-output, or business-specific logic to WorkOrch core.
- Verification: shell syntax check, managed harness execution with `WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=2`, full Go tests, `git diff --check`, and boundary search over the changed e2e/doc files.

## E2E Contract

- The script must work against `WORKORCH_BASE_URL` and therefore can run under the S11.1 managed harness or against an already deployed service.
- The script creates exactly two temporary workspaces and deletes them with `purge=true` during cleanup.
- Both `resume` commands are submitted concurrently before either projection wait starts.
- Both `replan` commands are submitted concurrently against the run id projected for the corresponding workspace.
- The script validates each response has `accepted=true`, the expected `intent`, the expected `requestId`, and the expected `reason`.
- The script validates each workspace snapshot has:
  - matching `snapshot.workspace.id`;
  - projection state `ready`, `stale`, or `degraded`;
  - `snapshot.runCommand.intent` equal to the command being checked;
  - matching `requestId`;
  - positive `runCommand.revision`;
  - for `resume`, a scoped gap baseline with actionable content.
- The script must fail if workspace A's projection shows workspace B's request id or command scope.

## Acceptance

- `verify-dual-workspace-resume-replan-e2e` passes against the managed harness.
- The managed harness can run the target with `WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=2`.
- The script writes a summary JSON artifact containing the two workspace ids, run ids, request ids, and command durations.
- No production code changes are required for this slice.

## Verification

- `bash -n scripts/verify-dual-workspace-resume-replan-e2e.sh`
- `WORKORCH_MANAGED_VERIFY_TARGET=verify-dual-workspace-resume-replan-e2e WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=2 WORKORCH_MANAGED_SKIP_PREFLIGHT=1 ./scripts/verify-workorch-managed-full.sh`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" scripts/verify-dual-workspace-resume-replan-e2e.sh Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-2-dual-workspace-resume-replan-e2e.md`
