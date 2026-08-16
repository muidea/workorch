# S10.2 Software Dev Stage Policy Binding

Status: completed

Owner: template/profile

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S10.1 Template Stage Policy Schema](./goal-dod-nextaction-s10-1-template-stage-policy-schema.md)

## Contract Audit

- Scope: `software-dev` workspaceTemplate stage strategy and workspace creation/activation binding.
- Observed state: `workorch.template_stage_policy.v1` is defined and activation can pass `templateStagePolicies` through as an opaque execution contract field. `generic-governed` declares a neutral stage policy, but `software-dev` still relies on separate context hints and Soul markdown for stage expectations.
- Expected state: the `software-dev` template declares its stage strategy as structured template/profile data, workspace creation persists that data into workspace-bound `contextHints`, and activation exposes the workspace-bound policy through `executionContract.templateStagePolicies`.
- Deviation: without a workspace-bound `software-dev` stage policy, later StagePacket and NextAction slices cannot trace stage expectations back to the template/profile contract at workspace creation time.
- Owner: template/profile data. workorchd creation and activation remain neutral copy/pass-through paths.
- Event path: embedded template defaults -> template repository -> `CreateWorkspaceWithTemplateWithBackend` -> `workspace.ContextHints.templateStagePolicies` -> `ActivateWorkspaceExecutionWithBackend` -> `run.ExecutionContext.executionContract.templateStagePolicies`.
- Contract gap: `software-dev` lacks a declared stage policy and tests proving the policy is copied to the workspace and emitted in activation context.
- Proposed change: add `templateStagePolicies` to `software-dev.contextHints`, assert its schema and key stages in defaults tests, and add generic appservice tests for workspace creation and activation binding.
- Boundary check: no core branch is added for `software-dev`, language, command, tool output, file name, or provider. Software-development-specific policy IDs stay inside template defaults/tests.
- Verification: defaults schema tests, workspace creation/activation tests, affected package tests, `git diff --check`, and boundary search.

## Bound Policy

`software-dev.contextHints.templateStagePolicies` uses schema `workorch.template_stage_policy.v1`.

Declared stages:

- `planning`: enters from `assess_gap`, requires `gap_baseline` / `assessment`.
- `execution`: enters from `state_change`, requires `state_change`.
- `repair`: enters from `repair`, requires `state_change` and references the software-dev capability and evidence policy IDs.
- `validation`: enters from `validate`, requires `validation` and references the software-dev evidence policy.
- `artifact`: enters from `produce_artifact`, requires `artifact`.

The stage policy intentionally references binding keys such as `planning`, `execution`, `validation`, and `artifact`. The `repair` stage uses the `execution` soul binding because repair is still a state-changing execution responsibility for this template.

## Acceptance

- `software-dev` declares `templateStagePolicies` in embedded defaults.
- Workspace creation persists `templateStagePolicies` in workspace `contextHints`.
- Activation emits `executionContract.templateStagePolicies` from workspace/template-bound context.
- Existing common tool defaults remain unpersisted in workspace `contextHints`.
- Appservice logic remains template-neutral.

## Verification

- `go test ./internal/pkg/workorchdefaults -run 'TestEmbedded(TemplateStagePolicySchemaWhenDeclared|SoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk)' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestCreateWorkspaceWithTemplateCopiesTemplateStagePolicies|TestActivateWorkspaceExecutionWithBackendAddsExecutionContractFromWorkspaceContextHints' -count=1`
- `go test ./internal/pkg/workorchdefaults ./internal/modules/application/workorchd/biz/appservice -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh" internal/modules/application/workorchd/biz/appservice internal/pkg/workorchdefaults -g '!**/defaults/souls/**'`
