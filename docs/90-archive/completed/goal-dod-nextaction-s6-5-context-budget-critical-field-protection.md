# S6.5 Context Budget Critical Field Protection

Status: completed

Type: roadmap-slice

Owner: llm block/core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [LLM Cache Improvement Design](../../30-module-architecture/llm-cache-improvement-design.md)
- [S6.1 StagePacket Minimal Model](./goal-dod-nextaction-s6-1-stage-packet-minimal-model.md)
- [S6.2 StagePacket Evidence Refs](./goal-dod-nextaction-s6-2-stage-packet-evidence-refs.md)
- [S6.3 StagePacket Target Snapshot Integrity](./goal-dod-nextaction-s6-3-stage-packet-target-snapshot-integrity.md)
- [S6.4 LLM Context Partition Boundary Tests](./goal-dod-nextaction-s6-4-llm-context-partition-boundary-tests.md)

## Contract Audit

- Scope: `internal/modules/blocks/llm/biz/provider_context_budget.go`, `internal/modules/blocks/llm/biz/provider_context_budget_test.go`.
- Observed state: context budget protection already restores Goal/DoD, repair targets, runtime contract and coverage work item context after each shrink level, but `stagePacket` is not a P0 key and is not restored after budget compaction.
- Expected state: Goal/DoD, StagePacket `requiredOutput`, accepted/rejected evidence refs, required evidence kinds and target snapshot integrity metadata must survive aggressive context budget compaction.
- Deviation: after S6.4, prompt rendering can expose StagePacket in C3/C5, but extreme budget shrink can remove or summarize the packet before prompt rendering.
- Owner: LLM context/provider.
- Event path: orchestrator step input `stagePacket` -> provider context budget shrink -> protected field restore -> prompt context C3/C5 rendering -> provider request.
- Contract gap: missing budget protection for the StagePacket fields that carry the current convergence contract.
- Proposed change: treat StagePacket as a protected P0 runtime contract, restore a metadata-only compact packet after every shrink level, record protected StagePacket fields and warn if the protected packet changes during budgeting.
- Boundary check: the change only consumes neutral structured StagePacket fields. It adds no template, language, command, provider or business-specific branch and does not read files or repositories.
- Verification: context budget unit test for aggressive compaction preserving StagePacket requiredOutput/evidence/snapshot integrity, LLM block tests, `git diff --check`, and boundary search.

## Slice Scope

S6.5 protects the following StagePacket fields under context budget pressure:

- scope: `schemaVersion`, `workspaceId`, `runId`, `stepId`, `idempotencyKey`
- definition and focus: `definition`, `activeStage`, `activeFocus`, `nextAction`
- convergence contract: `requiredOutput`, `allowedActions`, `forbiddenActions`, `requiredEvidenceKinds`
- evidence traceability: `acceptedEvidenceRefs`, `rejectedEvidenceRefs`
- target snapshot metadata: `targetRef`, `contentHash`, `integrityState`, `safeForExactFileWrite`, `contentComplete`, `safeForFileWrite`, `redacted`, `truncated`, `isBinary`, `size`, `source`, `contextSource`, `error`

Snapshot content remains outside the StagePacket protected view.

## Acceptance

- Aggressive budget shrink preserves StagePacket `requiredOutput`.
- Aggressive budget shrink preserves accepted/rejected evidence refs and required evidence kinds.
- Aggressive budget shrink preserves target snapshot integrity metadata.
- Context budget records protected StagePacket fields.
- If a protected StagePacket changes during budget compaction, the request receives an integrity warning instead of silently proceeding.

## Verification

- `go test ./internal/modules/blocks/llm/biz -run 'TestEnforceProviderRequestBudgetPreservesStagePacketCriticalFields|TestStagePacket|TestBuildProviderFacingPromptGroupsStablePrefixAndDynamicTail' -count=1`
- `go test ./internal/modules/blocks/llm/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/modules/blocks/llm/biz/provider_context_budget.go internal/modules/blocks/llm/biz/provider_context_budget_test.go docs/70-roadmap/active/goal-dod-nextaction-s6-5-context-budget-critical-field-protection.md`
