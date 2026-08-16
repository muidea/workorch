# S6.6 Compact Summary Auxiliary Reference

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S6.4 LLM Context Partition Boundary Tests](./goal-dod-nextaction-s6-4-llm-context-partition-boundary-tests.md)
- [S6.5 Context Budget Critical Field Protection](./goal-dod-nextaction-s6-5-context-budget-critical-field-protection.md)

## Contract Audit

- Scope: `internal/modules/blocks/llm/biz/prompt_messages.go`, `internal/modules/blocks/llm/biz/prompt_messages_test.go`.
- Observed state: C4 current progress and process evidence timeline already compact bulky runtime data, and C4 coverage work items expose ledger-derived focus/unresolved items. The compact summaries do not consistently declare that they are reference-only and must not override the coverage ledger or StagePacket contract.
- Expected state: compact summaries can help the LLM understand recent history, but coverage satisfaction, missing evidence, required output and next action remain owned by the ledger/StagePacket contract.
- Deviation: a provider prompt can present a compact progress digest such as completed/satisfied alongside unresolved ledger items without an explicit authority rule telling the LLM which source wins.
- Owner: LLM context/provider.
- Event path: runtime step input summaries + coverage ledger + StagePacket -> LLM prompt C3/C4/C5 rendering -> provider candidate output -> contract/evidence gate.
- Contract gap: missing explicit prompt contract that compact summary fields are auxiliary references and cannot close coverage or redefine next action without ledger evidence refs.
- Proposed change: add a neutral summary authority policy to compact progress, process evidence timeline, memory recall and repair-package compact refs; update tests to prove conflicting progress summaries do not override `C4.coverage-work-items` and `C3.stage-packet-contract`.
- Boundary check: the change only adds neutral authority metadata to generic prompt sections. It adds no template, language, command, provider or business-specific branch.
- Verification: LLM prompt unit test for compact summary auxiliary policy, existing prompt/cache tests, `git diff --check`, and boundary search.

## Slice Scope

S6.6 adds prompt-visible authority metadata:

- compact summaries are `auxiliary_reference_only`
- coverage ledger and StagePacket remain authoritative for current convergence
- summaries must cite source refs or evidence refs before being used as support
- summary-only completion/progress claims cannot mark coverage satisfied

This slice does not change ledger computation, NextAction selection, evidence persistence, provider adapters or template policy.

## Acceptance

- `C4.current-progress` declares compact progress as auxiliary reference only.
- `C4.process-evidence-timeline` declares history summaries as audit refs, not ledger authority.
- `C4.coverage-work-items` declares ledger authority.
- `C3.stage-packet-contract` remains visible as the current required output contract.
- A test with conflicting “progress says complete” and “ledger says open” proves the prompt renders the ledger and StagePacket as authoritative.

## Verification

- `go test ./internal/modules/blocks/llm/biz -run 'TestCompactSummaryIsAuxiliaryToLedgerAndStagePacket|TestProcessEvidenceTimelineSignalCompactsBulkyHistoricalOutputs|TestStagePacket' -count=1`
- `go test ./internal/modules/blocks/llm/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/prompt_messages_test.go docs/70-roadmap/active/goal-dod-nextaction-s6-6-compact-summary-auxiliary-reference.md`
