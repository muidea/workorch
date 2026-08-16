# S9.5 Watchdog Idempotency Key

Status: completed

Owner: core/workorchd

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [S9.4 Watchdog External Timeout Scope](./goal-dod-nextaction-s9-4-watchdog-external-timeout-scope.md)
- [EventHub Driven Workorchd State Loop Design](../superseded/eventhub-driven-workorchd-state-loop-design.md)
- [Runtime Execution Event Queue State Loop Design](../superseded/runtime-execution-event-queue-state-loop-design.md)

## Contract Audit

- Scope: stale runtime-gap watchdog recovery command from workorchd runports to orchestrator owner.
- Observed state: runtime-gap recovery command carries workspace/run/step/gap/request/queue metadata, but does not expose a stable idempotency key. Repeated watchdog ticks can publish indistinguishable recovery commands that cannot be correlated or de-duplicated before the background resume task updates owner state.
- Expected state: every watchdog compensation command has workspace/run/step scoped idempotency metadata. Repeated triggers for the same runtime gap should carry the same key and should not enqueue duplicate background recovery work while the first recovery is in flight.
- Deviation: recovery intent identity is implicit in multiple fields and not enforced at the command/scheduler boundary.
- Owner: core/workorchd.
- Event path: runtime checkpoint gap -> runports `RuntimeGapRecovery` -> `TopicOrchestratorCommandRecoverRuntimeGap` -> orchestrator owner background resume patch.
- Contract gap: missing canonical idempotency key and in-flight guard for runtime-gap recovery commands.
- Proposed change: derive a stable watchdog recovery idempotency key from workspace/run/step/gap/request/queue metadata, publish it in command and EventHub payload, include it in the execution context recovery patch, and guard duplicate in-flight recovery commands in orchestrator module.
- Boundary check: key derivation uses only structural runtime scope and evidence metadata. It does not inspect template ID, Soul ID, workspace title, programming language, command text, provider name, or output text.
- Verification: unit tests for command payload key propagation, patch propagation, duplicate in-flight command guard, affected package tests, boundary search, and `git diff --check`.

## Design Notes

- The idempotency key shape is `runtime-gap-recovery|<workspace>|<run>|<step>|<gap>|<request>|<queue>`.
- If request or queue metadata is missing, the key uses `-` for that component. The workspace/run/step/gap scope remains mandatory.
- The key is attached to both `RecoverRuntimeGapCommand.IdempotencyKey` and `PayloadKeyIdempotencyKey`.
- The orchestrator module only suppresses duplicate in-flight scheduling for the same key. After the first recovery finishes, later recovery attempts are evaluated again against current owner state.

## Acceptance

- Watchdog recovery command payload carries a stable idempotency key.
- Runtime-gap recovery patch records the same idempotency key for audit and downstream LLM context.
- Duplicate recovery commands with the same key do not enqueue duplicate background resume tasks while a prior task is still running.
- Commands with different keys remain independently schedulable.

## Verification

- `go test ./internal/modules/application/workorchd/biz/appservice/runports -run 'TestRecoverRuntimeGapPublishesGapSpecificCommandPayload' -count=1`
- `go test ./internal/modules/kernel/orchestrator -run 'TestModuleRecoverRuntimeGapCommand(SchedulesBackgroundDispatch|DeduplicatesInFlightIdempotencyKey)|TestRuntimeGapRecoveryPatchContainsGapSpecificContext' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/runports ./internal/modules/kernel/orchestrator -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/... -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/application/workorchd/biz/appservice/runports internal/modules/kernel/orchestrator docs/70-roadmap/active/goal-dod-nextaction-s9-5-watchdog-idempotency-key.md`
