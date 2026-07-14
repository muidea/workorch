# S6.4 LLM Context Partition Boundary Tests

Status: completed

Type: roadmap-slice

Owner: llm block

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [LLM Cache Improvement Design](../../30-module-architecture/llm-cache-improvement-design.md)
- [S6.1 StagePacket Minimal Model](./goal-dod-nextaction-s6-1-stage-packet-minimal-model.md)
- [S6.2 StagePacket Evidence Refs](./goal-dod-nextaction-s6-2-stage-packet-evidence-refs.md)
- [S6.3 StagePacket Target Snapshot Integrity](./goal-dod-nextaction-s6-3-stage-packet-target-snapshot-integrity.md)

## Contract Audit

- Scope: `internal/modules/blocks/llm/biz/prompt_messages.go`, `internal/modules/blocks/llm/biz/prompt_messages_test.go`.
- Observed state: S6.1-S6.3 attach `stagePacket` to LLM step input, but the LLM prompt renderer does not expose the packet as an explicit C3/C5 contract section. Existing cache tests cover other dynamic fields, not StagePacket.
- Expected state: StagePacket runtime fields are visible to the LLM in dynamic prompt partitions while C0/C1/C2 stable prefix remains unchanged across StagePacket changes.
- Deviation: LLM input can receive StagePacket only as scattered fallback input, so required output, evidence refs and target snapshot integrity are not protected by explicit partition boundary tests.
- Owner: LLM context/provider.
- Event path: orchestrator step input `stagePacket` -> LLM prompt renderer -> C3 stage packet contract + C5 target snapshot metadata -> provider cache plan -> provider-facing stable prefix and dynamic tail.
- Contract gap: missing C0/C1/C2/C3/C4/C5 boundary regression for StagePacket; missing explicit metadata-only rendering for StagePacket target snapshots.
- Proposed change: render StagePacket contract fields in C3 and target snapshot metadata in C5, then add cache/context tests proving StagePacket mutations affect only dynamic hashes and dynamic provider messages.
- Boundary check: the change consumes neutral structured runtime fields only. It adds no template, language, command, provider or business-specific branch, and it does not read files or repositories.
- Verification: LLM block unit tests for StagePacket rendering and cache hash stability, `git diff --check`, and boundary search for business-specific strings.

## Slice Scope

S6.4 establishes the prompt rendering boundary for StagePacket:

- `C3.stage-packet-contract` carries scope, definition, active stage/focus, next action, required output, allowed/forbidden actions and accepted/rejected evidence refs.
- `C5.stage-packet-target-snapshots` carries target snapshot metadata only.
- Snapshot `content` and `contentPreview` are never copied into StagePacket C5 rendering.
- StagePacket dynamic fields must not enter C0, C1 or C2 stable prefix rendering.

## Acceptance

- Prompt context includes StagePacket contract data when `input.stagePacket` is present.
- StagePacket target snapshot metadata is visible in C5 and remains metadata-only.
- Provider-facing stable prefix contains only C0/C1/C2 sections and does not include StagePacket runtime values.
- Mutating StagePacket required output, evidence refs or target snapshot metadata does not change `stablePrefixHash`, `renderedStablePrefixHash`, `workspaceDefinitionHash`, `capabilityContractHash` or `cacheKey`.

## Verification

- `go test ./internal/modules/blocks/llm/biz -run 'TestStagePacket|TestBuildProviderFacingPromptGroupsStablePrefixAndDynamicTail|TestProviderCachePlanKeepsCacheablePrefixStableAcrossVolatileProgress' -count=1`
- `go test ./internal/modules/blocks/llm/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/prompt_messages_test.go docs/70-roadmap/active/goal-dod-nextaction-s6-4-llm-context-partition-boundary-tests.md`
