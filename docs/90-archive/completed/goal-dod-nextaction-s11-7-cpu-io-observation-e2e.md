# S11.7 CPU/IO Observation E2E

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [Projection First ReadModel IO Closure Design](../../30-module-architecture/projection-first-readmodel-io-closure-design.md)
- [Workorchd EventHub Watchdog Scope Closure Plan](./workorchd-eventhub-watchdog-scope-closure-plan.md)
- [S9.1 Projection Readmodel First](./goal-dod-nextaction-s9-1-projection-readmodel-first.md)
- [S9.2 History Recent Compact Refs](./goal-dod-nextaction-s9-2-history-recent-compact-refs.md)
- [S9.3 Snapshot Payload On Demand](./goal-dod-nextaction-s9-3-snapshot-payload-on-demand.md)
- [S9.4 Watchdog External Timeout Scope](./goal-dod-nextaction-s9-4-watchdog-external-timeout-scope.md)
- [S9.5 Watchdog Idempotency Key](./goal-dod-nextaction-s9-5-watchdog-idempotency-key.md)
- [S9.6 Service Restart Running Recoverable](./goal-dod-nextaction-s9-6-service-restart-running-recoverable.md)
- [S11.1 Managed E2E Harness](./goal-dod-nextaction-s11-1-managed-e2e-harness.md)

## Contract Audit

- Scope: managed WorkOrch service idle observation with multiple precreated workspaces, projection readmodel metrics, event record IO pressure metrics, SSE liveness, and process CPU/RSS/IO sampling.
- Observed state: previous S9 closures added readmodel-first projection, bounded event metadata queries, payload-on-demand metrics, and managed idle observation through `verify-runtime-idle-baseline`. The roadmap still lacked a first-class S11.7 artifact and `verify-e2e-full` did not explicitly include the CPU/IO idle observation gate.
- Expected state: a managed latest service can load multiple idle workspaces without continuous full event scans, raw payload hydration, projection rebuild backlog, projection schedule drops, runtime event publish failures, or sustained process CPU/IO churn.
- Deviation: observation capability existed, but S11.7 was not represented as an explicit e2e target in the roadmap closure chain. A full e2e run could therefore miss the long-running CPU/IO regression gate.
- Owner: e2e/runtime observation. This slice validates already-designed runtime behavior. It does not alter core orchestration, workspaceTemplate/Soul/policy, LLM context, or client rendering.
- Event path: managed service startup -> workspace precreation -> projection drain -> `/metrics` and `/readyz` before snapshot -> SSE observation window -> `/metrics` and process resource after snapshot -> threshold evaluation -> observation summary artifacts.
- Ledger impact: none. The slice does not change Goal/DoD, coverage ledger, evidence ledger, completion gate, or NextAction interpretation.
- Stage Packet impact: none.
- Contract gap: missing explicit S11.7 validation target and roadmap artifact for CPU/IO observation.
- Proposed change: expose `verify-cpu-io-observation-e2e` as the S11.7 target, map it to the managed `verify-runtime-idle-baseline` gate, and include it in `verify-e2e-full`.
- Boundary check: no production runtime or template logic is changed. The e2e gate uses neutral service metrics and process resource counters; it does not assert software-development, language, tool, or template-specific behavior.
- Verification: target graph checks, shell syntax checks, managed S11.7 gate execution, diff hygiene, and boundary search over changed files.

## E2E Contract

`verify-cpu-io-observation-e2e` is the S11.7 grouped validation target. It aliases the existing managed idle baseline gate so the roadmap item has a stable acceptance command while preserving the lower-level diagnostic entry `verify-runtime-idle-baseline`.

The managed gate must:

- build and start a latest local WorkOrch service with file-backed repository data isolated under a temporary directory;
- precreate at least 2 idle workspaces before observation;
- wait for workspace projection to drain before capturing baseline metrics;
- observe the service long enough to catch fixed polling, repeated projection rebuild, or event history reread regressions;
- require local process resource evidence when the service PID can be inferred;
- fail if projection pending rebuilds remain above threshold;
- fail if projection schedule dropped count increases;
- fail if event payload hydration increases without an explicit payload-read action;
- fail if event metadata record scan pressure increases in the idle window;
- fail if runtime event publish failures increase;
- fail if process CPU, read bytes, or bounded write bytes exceed configured idle thresholds.

Default thresholds are intentionally strict for idle observation:

- precreated workspace count: `2`;
- observation duration: `20s`;
- auto snapshot limit: `0`;
- pending projection rebuilds: `0`;
- projection schedule dropped delta: `0`;
- event metadata records scanned delta: `0`;
- event payload hydrates delta: `0`;
- event payload hydrate bytes delta: `0`;
- runtime event publish failures delta: `0`;
- process CPU percent: `20`;
- process read bytes delta: `0`;
- process write bytes delta: `65536`.

The write threshold is intentionally non-zero because the managed observation
itself generates HTTP/SSE access log writes in the observed process. The gate is
meant to catch sustained full-scan or rewrite churn, not the small bounded log
write produced by the observer.

## Acceptance

- `verify-cpu-io-observation-e2e` exists as the S11.7 validation target.
- `verify-cpu-io-observation-e2e` runs the same managed idle baseline contract as `verify-runtime-idle-baseline`.
- `verify-e2e-full` includes S11.7 through `verify-cpu-io-observation-e2e`.
- Observation summary artifacts include projection, event IO pressure, runtime event publish failure, SSE, and process resource sections.
- S11.7 remains an e2e validation layer only; any future production fix must be driven by a concrete failing metric owner.

## Verification

- `make -n verify-cpu-io-observation-e2e`
- `make -n verify-e2e-full`
- `bash -n scripts/observe-workorch-runtime-closure.sh scripts/verify-workorch-managed-full.sh`
- `WORKORCH_MANAGED_SKIP_PREFLIGHT=1 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 make verify-cpu-io-observation-e2e`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-7-cpu-io-observation-e2e.md docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md`

2026-06-25 latest verification:

- command: `WORKORCH_MANAGED_SKIP_PREFLIGHT=1 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 make verify-cpu-io-observation-e2e`
- result: passed
- artifacts: `/tmp/tmp.xtbICIvhp7/observe-runtime-closure`
- observed workspaces: 2 idle workspaces
- projection state: `ready`
- projection pending rebuilds: `0`
- projection schedule dropped delta: `0`
- event metadata records scanned delta: `0`
- event payload hydrates delta: `0`
- event payload hydrate bytes delta: `0`
- runtime event publish failure delta: `0`
- process CPU percent: `0.0`
- process read bytes delta: `0`
- process write bytes delta: `0`
