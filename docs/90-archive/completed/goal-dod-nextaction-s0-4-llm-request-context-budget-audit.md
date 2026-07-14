# Goal/DoD NextAction S0.4 LLM Request And Context Budget Audit

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [LLM Cache Improvement Design](../../30-module-architecture/llm-cache-improvement-design.md)
- [S6.1 StagePacket Minimal Model](./goal-dod-nextaction-s6-1-stage-packet-minimal-model.md)
- [S6.2 StagePacket Evidence Refs](./goal-dod-nextaction-s6-2-stage-packet-evidence-refs.md)
- [S6.3 StagePacket Target Snapshot Integrity](./goal-dod-nextaction-s6-3-stage-packet-target-snapshot-integrity.md)
- [S6.4 LLM Context Partition Boundary Tests](./goal-dod-nextaction-s6-4-llm-context-partition-boundary-tests.md)
- [S6.5 Context Budget Critical Field Protection](./goal-dod-nextaction-s6-5-context-budget-critical-field-protection.md)
- [S6.6 Compact Summary Auxiliary Reference](./goal-dod-nextaction-s6-6-compact-summary-auxiliary-reference.md)

## Contract Audit

- Scope: LLM request partitioning, StagePacket fields, context budget protection, cache-stable prefix and compact summary references.
- Observed state: StagePacket carries definition/current focus/required output, evidence refs and target snapshot integrity. Context budget protects Goal/DoD, required output and evidence fields. Context partition tests prevent dynamic fields from entering stable C0. Compact summary is auxiliary rather than authoritative ledger state.
- Expected state: LLM input can reconstruct latest Goal/DoD, current gap, required output, accepted/rejected evidence refs and target snapshot integrity without breaking provider-facing stable prefix semantics.
- Deviation: S0.4 originally had no standalone request/context inventory, so the later S6 slices carried the closure evidence without a single audit baseline.
- Owner: core StagePacket assembly and LLM block request rendering.
- Event path: NextAction -> StagePacket -> C0/C1/C2/C3/C4/C5 rendering -> provider request -> contract validation.
- Contract gap: documentation gap only. Later S6 slices provide the implementation and tests.
- Proposed change: record this request/context inventory as the S0.4 audit baseline and link the completed implementation slices.
- Boundary check: context sections are neutral runtime contracts. Provider private cache controls remain adapter-owned; no template, language, tool, or provider-specific behavior is added.
- Verification: linked S6 tests plus roadmap coverage audit.

## Context Inventory

| Partition/Model | Responsibility | Verified By |
| --- | --- | --- |
| StagePacket | definition, current focus, required output | S6.1 |
| Evidence refs | accepted/rejected evidence traceability | S6.2 |
| Target snapshot | integrity and truncation safety | S6.3 |
| C0-C5 rendering | stable prefix and dynamic field separation | S6.4 |
| Context budget | critical field protection | S6.5 |
| Compact summary | auxiliary reference only | S6.6 |

## Verification

- S6.1-S6.6 completed and document StagePacket, evidence refs, snapshot integrity, rendering boundary, budget protection and compact summary behavior.
- `python3` roadmap coverage audit reports S0.4 as covered by this document.
