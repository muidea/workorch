# S10.3 Evidence Policy Coverage Binding

Status: completed

Owner: template/profile

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S10.1 Template Stage Policy Schema](./goal-dod-nextaction-s10-1-template-stage-policy-schema.md)
- [S10.2 Software Dev Stage Policy Binding](./goal-dod-nextaction-s10-2-software-dev-stage-policy-binding.md)

## Contract Audit

- Scope: template-bound evidence policy data and coverage work item `requiredEvidenceKinds` derivation.
- Observed state: coverage work items already derive `requiredEvidenceKinds` from structured DoD items and `executionContract.profileContracts.*.evidenceRequirements`. Template-bound `evidencePolicies` are copied to workspace/run context and injected into LLM inputs, but they do not declare or drive coverage requirements.
- Expected state: a template evidence policy can declare which coverage scopes require which evidence kinds. Runtime consumes the declaration through the existing neutral evidence requirement interpreter and does not branch on template ID, language, command names, or natural language text.
- Deviation: without an explicit evidence-policy-to-coverage binding, S10 template policy data can affect evidence quality but cannot be traced as the source of required coverage evidence kinds.
- Owner: template/profile policy data plus neutral runtime requirement normalization. The policy content remains template-owned; runtime only interprets generic `coverageRequirements` fields.
- Event path: workspaceTemplate `evidencePolicies[].coverageRequirements` -> workspace-bound evidence policies -> activation `run.ExecutionContext.evidencePolicies` -> coverage requirement normalization -> `CoverageWorkItem.requiredEvidenceKinds` and evidence quality validation.
- Contract gap: `evidencePolicies` have no structured coverage binding field and runtime has no neutral path to convert policy-bound requirements into coverage required evidence kinds.
- Proposed change: add generic `coverageRequirements` entries to evidence policies, extend `profileEvidenceKindRequirement` parsing to include both profile and evidence-policy sources, and prefer explicit `requiredEvidenceKinds` when present.
- Boundary check: no software-dev, tool, language, command, or provider branch is added to kernel logic. Software-dev-specific policy IDs and validation markers remain in defaults/tests.
- Verification: unit tests for policy-derived required evidence kinds, defaults tests for software-dev policy binding, affected package tests, full `go test ./...`, `git diff --check`, and boundary search.

## Policy Shape

`evidencePolicies[].coverageRequirements[]` uses the same neutral fields as profile `evidenceRequirements`:

```json
{
  "id": "*",
  "scope": "dod_item",
  "requiredEvidenceKinds": ["validation"],
  "requireDirectEvidence": true,
  "requireClaimEvidence": true,
  "requireExecutableBehaviorEvidence": true,
  "acceptedStepKinds": ["shell/command", "mcp/tool_call"],
  "rejectedStepKinds": ["file/read", "file/list"]
}
```

Field rules:

- `scope` is normalized with the existing coverage scopes: `dod_item`, `goal_coverage`, `acceptance_dimension`, `deliverable`.
- `id` matches the same aliases as profile evidence requirements. `*` or `all` applies to all items in the matching scope.
- `evidenceRole` can narrow a requirement to a coverage source role such as `primary_state_change` or `supplemental_evidence`.
- `requiredEvidenceKinds` is authoritative when present. If omitted, runtime falls back to the existing generic inference from accepted step kinds, operations, and direct evidence flags.

## Acceptance

- Coverage work items can derive `requiredEvidenceKinds` from evidence policy data.
- Evidence policy coverage requirements are workspace/run-bound through existing template binding.
- Existing profile `evidenceRequirements` behavior is preserved.
- Runtime uses the same requirement matching and quality validation path for profile and evidence policy requirements.
- Non-target templates remain unaffected when they do not declare `coverageRequirements`.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestCoverageWorkItemsDeriveRequiredEvidenceKindsFromEvidencePolicies|TestCoverageWorkItemsDoNotInferGovernanceEvidenceFromBusinessText' -count=1`
- `go test ./internal/pkg/workorchdefaults -run 'TestEmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz ./internal/pkg/workorchdefaults -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh" internal/modules/kernel/orchestrator/biz internal/pkg/workorchdefaults -g '!**/defaults/souls/**'`
