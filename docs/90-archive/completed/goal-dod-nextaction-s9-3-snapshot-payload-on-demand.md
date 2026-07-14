# S9.3 Snapshot Payload On Demand

Status: completed

Owner: core/repository

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: LLM-visible repair target snapshots in `runtime_llm_context` and provider prompt rendering.
- Observed state: repair target snapshots can carry complete `content` when a snapshot is safe and below the current snapshot byte limit. This makes large object body visibility part of the default LLM input path.
- Expected state: default LLM context carries snapshot metadata and refs; exact payload/snapshot body is read only by explicit follow-up action or targeted snapshot hydration.
- Deviation: safe snapshots can bypass ref-only projection and enter the default context as full content.
- Owner: core.
- Event path: repair context target snapshot -> `compactStepInputForLLMContext` -> `repairActionPackage.targetSnapshots` -> provider prompt renderer -> provider request.
- Contract gap: no mandatory `snapshotRef` metadata and no ref-only default compaction rule for repair target snapshots.
- Proposed change: compact repair target snapshots to metadata-only by default in runtime context and provider prompt rendering, synthesize a stable `snapshotRef` when target/hash metadata exists, and mark omitted body metadata without declaring the stored snapshot unsafe.
- Boundary check: no template ID, Soul ID, programming language, command output marker, or software-development-specific policy is introduced. The rule is generic for target snapshot payload bodies.
- Verification: LLM context unit tests proving content is omitted, refs and safety metadata remain, existing generic unsafe truncation behavior is preserved, plus boundary search.

## Design Notes

- `contentComplete` and `safeForFileWrite` describe the underlying stored snapshot, not prompt body visibility.
- `contentOmitted=true` means the body is not present in the default LLM context.
- `snapshotRef` is a stable metadata reference derived from target and content hash when no explicit ref exists.
- Exact content can still be acquired by a later explicit read/hydration path; this slice only prevents default payload injection.

## Acceptance

- `repairActionPackage.targetSnapshots` in compact LLM input omits `content` and `contentPreview`.
- Compact target snapshots retain `targetRef`, `contentHash`, `snapshotRef`, `contentComplete`, and `safeForFileWrite`.
- Existing generic compaction still marks truly truncated/redacted prompt content unsafe.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(LLMContext|CompactLLMContext|GenericLLMContext)' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/blocks/llm/biz -run 'TestBuildProviderPromptMessages(MarksTruncatedRepairSnapshotUnsafe|UsesProviderProfileSnapshotBudget|UsesStateChangingRepairAction)|TestBuildPromptContext|TestRuntimeAdapterPreservesSafeRepairSnapshotMetadataInPrompt' -count=1`
- `go test ./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel/orchestrator/biz/runtime_llm_context.go internal/modules/kernel/orchestrator/biz/runtime_llm_context_test.go internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/prompt_messages_test.go internal/modules/blocks/llm/biz/runtime_adapter_test.go`
