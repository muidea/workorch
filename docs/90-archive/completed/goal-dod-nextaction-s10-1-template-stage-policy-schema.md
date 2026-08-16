# S10.1 Template Stage Policy Schema

Status: completed

Owner: template/profile

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [EventHub Driven Workorchd State Loop Design](../superseded/eventhub-driven-workorchd-state-loop-design.md)
- [S10 Template/Profile/Soul Strategy](./goal-dod-nextaction-iterative-delivery-plan.md#p10-slices-templateprofilesoul-strategy)

## Contract Audit

- Scope: workspaceTemplate/profile strategy data carried through `contextHints` into activation execution contract.
- Observed state: template strategy currently uses loosely named `contextHints` fields such as `profileContracts`, `capabilityUsagePolicies`, `runtimeRepair`, and `acceptanceSlicePolicy`. There is no explicit stage policy schema that can describe stage entry, exit, required outputs, governance gates, capability policy references, and evidence policy references.
- Expected state: stage strategy is declared as structured template/profile data. workorchd copies and exposes it through execution contract without interpreting template IDs, Soul IDs, natural language content, programming language, command names, or provider-specific details.
- Deviation: without a declared stage policy schema, later stage behavior risks being encoded either in Soul markdown only or in core ad hoc conditionals.
- Owner: template/profile strategy definition, with workorchd activation only performing neutral structured-field pass-through.
- Event path: workspaceTemplate `contextHints.templateStagePolicies` -> workspace/template context merge -> activation `executionContract.templateStagePolicies` -> later StagePacket/rendering slices consume structured fields.
- Contract gap: missing canonical schema name and pass-through point for template stage policy data.
- Proposed change: define `workorch.template_stage_policy.v1`, document its fields, and allow activation execution contract to carry `templateStagePolicies` as an opaque structured policy object.
- Boundary check: the schema is generic. It does not define software-dev behavior, language rules, command names, tool names, or template-specific branching in core.
- Verification: unit tests for execution contract pass-through and default schema validation helper, boundary search, affected package tests, and `git diff --check`.

## Schema

`templateStagePolicies` lives under workspaceTemplate/profile `contextHints`.

```json
{
  "schemaVersion": "workorch.template_stage_policy.v1",
  "stages": [
    {
      "id": "planning",
      "soulBinding": "planning",
      "entry": {
        "nextAction": "assess_gap",
        "requiresDefinitionRevision": true,
        "requiresGapBaseline": true
      },
      "requiredOutputs": [
        {
          "kind": "gap_baseline",
          "evidenceKinds": ["assessment"]
        }
      ],
      "completion": {
        "policy": "required_outputs_accepted"
      },
      "governance": {
        "decisionMode": "when_required",
        "approvalMode": "when_required"
      },
      "policyRefs": {
        "evidencePolicies": [],
        "capabilityUsagePolicies": []
      }
    }
  ]
}
```

## Field Rules

- `schemaVersion` is required and must be `workorch.template_stage_policy.v1`.
- `stages` is required and must be a non-empty list.
- Each stage must declare a stable `id`.
- `soulBinding` is optional but, when present, is a binding key such as `planning`, `execution`, `decision`, `approval`, `validation`, or `artifact`. It is not a Soul ID.
- `entry.nextAction` is a symbolic NextAction value. Core may interpret the symbol later, but must not infer behavior from template ID or natural language.
- `requiredOutputs[].kind` and `requiredOutputs[].evidenceKinds` are structured contracts used by later StagePacket slices.
- `policyRefs` references policy IDs declared elsewhere in the same template/profile context. It does not embed business-specific runtime logic.

## Acceptance

- The schema is documented as `workorch.template_stage_policy.v1`.
- workorchd activation can pass `templateStagePolicies` into `executionContract`.
- Existing templates without `templateStagePolicies` remain valid for this slice.
- The pass-through keeps `templateStagePolicies` opaque and does not branch on template ID, Soul ID, language, command, or provider.

## Verification

- `go test ./internal/modules/application/workorchd/biz/appservice/activationcore -run 'TestWorkspaceExecutionContractIncludesTemplateStagePoliciesFromHints' -count=1`
- `go test ./internal/pkg/workorchdefaults -run 'TestEmbeddedTemplateStagePolicySchemaWhenDeclared' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/pkg/workorchdefaults -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh" internal/modules/application/workorchd/biz/appservice/activationcore internal/pkg/workorchdefaults -g '!**/defaults/souls/**'`
