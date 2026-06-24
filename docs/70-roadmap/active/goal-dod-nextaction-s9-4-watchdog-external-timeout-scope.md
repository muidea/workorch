# S9.4 Watchdog External Timeout Scope

Status: completed

Owner: core/workorchd

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Workorchd EventHub / Watchdog Scope Closure Plan](./workorchd-eventhub-watchdog-scope-closure-plan.md)
- [Runtime Execution Event Queue State Loop Design](../../30-module-architecture/runtime-execution-event-queue-state-loop-design.md)

## Contract Audit

- Scope: periodic stale dispatched runtime-gap watchdog and `RuntimeRecoveryPolicy` retry eligibility.
- Observed state: `RuntimeRecoveryPolicy.AllowsWatchdogExternalRetry` only checks `watchdogRecovery` plus idempotency or dedupe key. A step marked idempotent with a non-external `sideEffectPolicy` can be classified as watchdog-recoverable.
- Expected state: periodic watchdog may only auto-recover stale runtime gaps for external interactions with explicit retry policy and idempotent or dedupe-key safety. Internal command dispatch defects, workspace-only work, projection gaps, and request-not-accepted states remain diagnostic or explicit/manual recovery paths.
- Deviation: the generic policy helper does not enforce the external side-effect boundary promised by the watchdog design documents.
- Owner: core runtime contract, consumed by workorchd run recovery.
- Event path: runtime request -> accepted/progress checkpoint `runtimeRecoveryPolicy` -> runports runtime gap classifier -> watchdog recovery command.
- Contract gap: missing neutral `sideEffectPolicy` guard in the reusable watchdog retry eligibility function.
- Proposed change: require `sideEffectPolicy` to be `external_read` or `external_write` before `retry_external` can allow watchdog recovery. Keep terminal-result finalization independent because durable terminal evidence, not external retry policy, justifies that path.
- Boundary check: the rule is data-driven and business-neutral. It does not inspect template ID, Soul ID, workspace title, programming language, command text, provider name, or output text.
- Verification: unit tests for the policy helper and runports stale gap recovery, affected package tests, boundary search, and `git diff --check`.

## Design Notes

- `watchdogRecovery=retry_external` is not sufficient by itself.
- `idempotencyPolicy=idempotent` or `dedupe_key_required` with an `idempotencyKey` is still required.
- `sideEffectPolicy=external_read` or `external_write` is now also required for watchdog retry eligibility.
- Non-external work can still be recovered through explicit owner commands, manual `/run resume`, restart, or durable terminal result finalization when the terminal result checkpoint exists.
- Non-external `retry_external` policy is projected as `retry_external_not_external`, so diagnostics distinguish side-effect-scope mismatch from idempotency mismatch.

## Acceptance

- Non-external idempotent steps are not auto-recovered by the periodic stale runtime gap watchdog.
- External idempotent steps remain eligible for supported stale runtime gaps.
- Existing `request_not_accepted` and terminal-result evidence boundaries remain unchanged.

## Verification

- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/runports -run 'TestRecoverStaleOrphanDispatchedStep(SkipsNonExternalRecoverablePolicy|ResumesWorkerLostGap|ResumesStaleWorkerHeartbeatGap)' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/runports ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/pkg/workorchruntimecontract internal/modules/application/workorchd/biz/appservice/runports docs/70-roadmap/active/goal-dod-nextaction-s9-4-watchdog-external-timeout-scope.md`
