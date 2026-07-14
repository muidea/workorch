# Goal/DoD NextAction S2.2 Current Definition Revision Binding

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S2.1 Workspace Definition Revision Model](./goal-dod-nextaction-s2-1-definition-revision-model.md)

## Contract Audit

- Scope: bind current Goal/DoD definitions to `executionContext.definitionRevision` before first run creation.
- Observed state: run owner state can store `DefinitionRevision`, and run resume/recovery context patches already generate a `definitionRevision` map. The initial workspace activation context includes `goalId`, `doDId`, `goal`, `dod`, cumulative `goals/dods`, and definition change impact, but did not always include the explicit `definitionRevision` map.
- Expected state: every first execution run created from current Goal/DoD definitions should carry the same neutral definition revision contract as later resume/recovery paths. Run owner creation should derive `Run.DefinitionRevision` from that context.
- Deviation: current activation could create a run without `Run.DefinitionRevision`, leaving definition traceability implicit in `goalId/doDId`.
- Owner: workorchd appservice activation and run owner command. This is core definition lifecycle metadata, not template/Soul/policy.
- Event path: workspace activation -> effective Goal/DoD selection -> `executionContext.definitionRevision` -> run create command -> kernel `Run.DefinitionRevision`.
- Contract gap: initial activation context lacked the shared current definition revision map.
- Proposed change: add a shared activationcore helper for current definition revision context, use it in activation context and existing run definition context patch, and assert activation-created runs expose the derived revision.
- Boundary check: revision is derived from definition IDs only. No Goal/DoD natural language parsing, template ID, provider, language, file, or tool rule is introduced.
- Verification: activationcore context test, appservice activation test, affected package tests, boundary search, and `git diff --check`.

## Closure Items

1. Done: shared current definition revision helper created in activationcore.
2. Done: initial activation context includes `definitionRevision`.
3. Done: run definition context patch reuses the same helper.
4. Done: activation-created run exposes derived `Run.DefinitionRevision`.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/biz/appservice/activationcore -run 'WorkspaceExecutionContextBaseContextBuildsDefinitionContext|CurrentWorkspaceDefinitionRevisionContext' -count=1
go test ./internal/modules/application/workorchd/biz/appservice -run 'ActivateWorkspaceExecutionWithBackendUsesLatestDefinitionRevisionWithoutDuplicateLineage' -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchport ./internal/pkg/workorchclient -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-s2-2-current-definition-revision-binding.md internal/modules/application/workorchd/biz/appservice/activationcore/activation.go internal/modules/application/workorchd/biz/appservice/activationcore/definition_revision_context.go internal/modules/application/workorchd/biz/appservice/runports/definition_context_patch_projection.go internal/modules/application/workorchd/biz/appservice/activationcore/activation_test.go internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service_test.go -S
git diff --check
```
