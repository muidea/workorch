# S11.1 Managed E2E Harness

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S8.5 SSE Reconnect And Last Event ID](./goal-dod-nextaction-s8-5-sse-reconnect-last-event-id.md)
- [S9.6 Service Restart Running Recoverable](./goal-dod-nextaction-s9-6-service-restart-running-recoverable.md)
- [S10.6 Non-Software Template Isolation](./goal-dod-nextaction-s10-6-non-software-template-isolation.md)

## Contract Audit

- Scope: managed local WorkOrch process lifecycle for e2e verification.
- Observed state: `scripts/verify-workorch-managed-full.sh` already builds `./bin/workorch`, writes a temporary file-backed config, starts a server on a random local port, waits for `/healthz` and `/readyz`, exports `WORKORCH_BASE_URL`, runs a selected Make target, and stops the process in `trap cleanup`. It is used by CI and full managed verification, but there was no lightweight target that independently proves start/verify/stop is repeatable without running the full e2e chain.
- Expected state: S11 e2e checks should be able to rely on one managed harness that owns the temporary process lifecycle. The harness should expose observable runtime state, stop the server on success/failure, and have a fast smoke gate that runs more than once to catch leaked processes or port assumptions.
- Deviation: lifecycle behavior existed, but repeatability was only implicit in heavier targets. If cleanup regressed, later e2e slices could fail or mask leaked `workorch` processes.
- Owner: e2e harness. This does not change workorchd owner state, template policy, scheduler behavior, TUI, Web, or runtime contracts.
- Event path: `make verify-managed-service-smoke` -> smoke script -> managed harness -> build binary -> start temporary server -> `/healthz`/`/readyz`/HTTP verification -> cleanup trap -> runtime state file -> second iteration.
- Contract gap: missing lightweight repeatability gate and no machine-readable managed process state artifact.
- Proposed change: make the managed harness write `managed-runtime-state.json`, add a two-iteration smoke script that verifies the state becomes `stopped` and the PID is no longer running, and expose it through Makefile.
- Boundary check: no core, appservice, template, Soul, runtime, Web, or TUI production code is changed. New logic is isolated to e2e shell scripts and Makefile.
- Verification: shell syntax check, managed smoke target, full Go tests, `git diff --check`, and e2e boundary search.

## Harness Rules

- Managed verification owns the process it starts and must not depend on a pre-existing service on port 8080.
- The harness must use a temporary config/data root by default.
- The selected e2e target receives `WORKORCH_BASE_URL` and should treat the managed service as deployed.
- Cleanup must stop the managed process before the script exits.
- The smoke target runs at least two iterations to prove startup and cleanup are repeatable.
- Fast smoke mode may skip fmt/vet/test preflight, but it must still build the current binary and run an HTTP service check.

## Acceptance

- `verify-workorch-managed-full.sh` writes `managed-runtime-state.json` with `starting`, `running`, and final `stopped` lifecycle state.
- `verify-workorch-managed-harness-smoke.sh` runs the managed harness twice by default.
- Each smoke iteration verifies the final state is `stopped` and the managed PID no longer exists.
- Makefile exposes `verify-managed-service-smoke`.
- Existing full managed verification remains available through `verify-managed-full`.

## Verification

- `bash -n scripts/verify-workorch-managed-full.sh scripts/verify-workorch-managed-harness-smoke.sh`
- `make verify-managed-service-smoke`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" scripts Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-1-managed-e2e-harness.md`
