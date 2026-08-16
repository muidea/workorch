# S10.6 Non-Software Template Isolation

Status: completed

Owner: template/profile

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S10.2 Software Dev Stage Policy Binding](./goal-dod-nextaction-s10-2-software-dev-stage-policy-binding.md)
- [S10.3 Evidence Policy Coverage Binding](./goal-dod-nextaction-s10-3-evidence-policy-coverage-binding.md)
- [S10.5 Template Capability MCP/LSP Binding](./goal-dod-nextaction-s10-5-template-capability-mcp-lsp-binding.md)

## Contract Audit

- Scope: non-software workspace templates and activation execution context.
- Observed state: `generic-governed` declares only generic stage policy data, `weather-query` declares weather-governance Souls and DoD, and `software-dev` owns software-specific `must_produce_state_change`, runtime repair, version-control, semantic navigation, evidence policy, profile contract, and capability policy data.
- Expected state: non-software templates must not inherit software-development strategy from embedded defaults, workspace creation, activation, or common MCP catalogs. Core may interpret neutral fields when present, but must not inject software profile policy into other templates.
- Deviation: S10.5 proved MCP is not forced globally, but S10.6 still needed explicit regression coverage that the wider software-dev template strategy does not leak into non-software templates or their run execution context.
- Owner: template/profile data plus neutral activation composition. This is an isolation guarantee for template-owned strategy; no kernel/orchestrator behavior change is required.
- Event path: embedded template defaults -> workspace template repository -> workspace creation from selected template -> activation `executionContract` / `evidencePolicies` / `mcpSelection` / `mcpServers` -> LLM/tool context.
- Contract gap: missing focused tests for non-software templates that assert software-specific context hints and policies are absent before and after activation.
- Proposed change: document S10.6, add embedded defaults isolation tests, and add activation regression for `generic-governed`.
- Boundary check: no production code is changed. Software-specific strings remain in defaults/tests/docs only, and generic appservice/kernel code is not modified.
- Verification: defaults isolation test, appservice activation isolation test, affected package tests, full `go test ./...`, `git diff --check`, and boundary search.

## Isolation Rules

- A template only receives strategy fields that it declares itself or that the workspace explicitly overlays.
- `stateChangePolicy` is a neutral execution-contract field. The software-dev strategy is the template-bound `must_produce_state_change` value plus `runtimeRepair`, `versionControl`, `semanticCodeNavigation`, `managedWorkspaceConfigs`, `acceptanceSlicePolicy`, `profileContracts.software-dev`, software evidence policies, and software capability usage policies.
- Non-software templates may still use neutral `templateStagePolicies` when declared.
- System-level MCP catalogs may exist, but non-software templates do not select software semantic MCP defaults unless the template/workspace/skill explicitly declares them.
- Activation must build run execution context from the selected template and workspace overlay, not from a global software profile.

## Acceptance

- Embedded non-software templates do not contain software-dev-specific context hints or evidence policies.
- `generic-governed` stage policy has empty software capability/evidence policy refs.
- Activating a `generic-governed` workspace produces the neutral `allowed_but_not_required` state-change policy and no software-dev execution contract, software evidence policies, selected LSP bridge MCP server, or managed workspace config.
- Core remains business-neutral and no production branch is added for template IDs.

## Verification

- `go test ./internal/pkg/workorchdefaults -run 'TestEmbeddedNonSoftwareTemplatesDoNotDeclareSoftwareDevStrategy|TestEmbeddedTemplateStagePolicySchemaWhenDeclared' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivateGenericGovernedWorkspaceDoesNotInheritSoftwareDevStrategy' -count=1`
- `go test ./internal/pkg/workorchdefaults ./internal/modules/application/workorchd/biz/appservice -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg/workorchdefaults -g '!**/defaults/**'`
