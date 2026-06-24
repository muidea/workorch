# S5.1 NextAction Minimal Model

Status: completed

Type: roadmap-slice

Owner: core/runtime-contract

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)

## Contract Audit

- Scope: runtime contract model for deterministic NextAction.
- Observed state: appservice read models expose local `GapNextAction`, and runtime coverage state has internal next-action strings, but core has no reusable scoped `NextAction` contract.
- Expected state: runtime can represent a deterministic next action with workspace/run scope, action type, focus coverage items, required evidence, stage, reason and idempotency key.
- Deviation: downstream scheduler/projection work would otherwise continue adding unrelated ad hoc next-action strings.
- Owner: core/runtime-contract.
- Event path: coverage/evidence/governance ledger -> NextAction model -> later scheduler/projection slices.
- Contract gap: missing common model and stable identity contract; S5.1 does not implement the interpreter ordering rules.
- Proposed change: add `workorchruntimecontract.NextAction`, supported action type constants, `Normalized`, `HasScope`, `StableNextActionID` and `StableNextActionIdempotencyKey`.
- Boundary check: the model is business-neutral and contains no template, language, provider, command-output or workspace-specific behavior.
- Verification: runtime contract unit tests for stable scoped identity, scope isolation and ledger-field rebuild.

## Design

`NextAction` is the canonical core contract for one deterministic next step. It can be rebuilt from authoritative runtime state and should not depend on prompt text, UI state or transient explanation text.

The minimal model contains:

- `id`
- `workspaceId`
- `runId`
- `type`
- `subjectType`
- `subjectId`
- `focusCoverageItemIds`
- `requiredOutput`
- `allowedActions`
- `forbiddenActions`
- `requiredEvidenceKinds`
- `stage`
- `stageMode`
- `idempotencyKey`
- `reason`

Supported action types:

- `assess_gap`
- `plan`
- `state_change`
- `validate`
- `assessment`
- `repair`
- `request_decision`
- `request_approval`
- `complete`
- `fail`

Identity rules:

- `workspaceId` and `runId` are part of the idempotency key.
- action `type`, `subjectType`, `subjectId`, `stage`, `stageMode`, `focusCoverageItemIds` and `requiredEvidenceKinds` are part of the idempotency key.
- `reason`, `requiredOutput`, `allowedActions` and `forbiddenActions` do not change identity; they are renderable contract detail.

## Acceptance

- Core has one `NextAction` model that can be reused by S5.2-S5.8.
- The model carries workspace/run scope.
- The model derives stable id and idempotency key from ledger-reconstructable fields.
- Tests prove same ledger fields rebuild the same identity and different workspace/run scopes do not collide.

## Verification

- `go test ./internal/pkg/workorchruntimecontract -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/pkg/workorchruntimecontract/next_action.go \
  internal/pkg/workorchruntimecontract/next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-1-nextaction-minimal-model.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
