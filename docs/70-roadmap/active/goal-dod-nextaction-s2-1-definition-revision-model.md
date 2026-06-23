# Goal/DoD NextAction S2.1 Workspace Definition Revision Model

Status: active

Type: contract-audit

Last Updated: 2026-06-23

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)

## Contract Audit

- Scope: minimal `WorkspaceDefinitionRevision` mapping for run traceability.
- Observed state: kernel `Run` already stores `DefinitionRevision`; run creation derives it from `executionContext.definitionRevision` when the explicit command field is empty. Activation also passes the derived value into the run owner command. However appservice/API/client/port run DTOs did not expose the persisted run `definitionRevision`.
- Expected state: every run must be traceable to the active Goal/DoD definition revision through owner state, REST/API DTOs, remote backend DTOs, and projection consumers. The field is neutral lifecycle metadata and must not depend on template, language, provider, or business text.
- Deviation: callers could inspect raw `executionContext.definitionRevision`, but the first-class run DTO omitted the owner field, making revision traceability implicit and easy to lose across remote/TUI/Web paths.
- Owner: core model and workorchd appservice DTO mapping. This is core definition lifecycle metadata, not template/Soul/policy.
- Event path: activation builds execution context -> run owner create command carries `DefinitionRevision` -> kernel run stores it -> appservice/client/port DTOs expose it.
- Contract gap: the owner state field was not projected through all public run DTOs.
- Proposed change: add `definitionRevision` to appservice `Run`, client `Run`, and port `RunInfo`; map it from kernel run owner state and remote client responses; add tests proving appservice reads and remote mapping preserve the value.
- Boundary check: the field is copied from owner state without parsing Goal/DoD text or checking template/language/tool/provider strings.
- Verification: appservice run read test, remote backend run mapping test, affected package tests, boundary search, and `git diff --check`.

## Closure Items

1. Done: appservice run DTO exposes `definitionRevision`.
2. Done: runtime run mapping copies kernel `Run.DefinitionRevision` into appservice DTOs.
3. Done: client and port DTOs preserve run `definitionRevision` for remote/TUI/Web consumers.
4. Done: eventhub resource mapping preserves run `definitionRevision` while still compacting heavy execution context.
5. Done: tests cover appservice run view mapping, remote backend mapping, and eventhub resource mapping.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/biz/appservice/runports ./internal/modules/application/workorchd/biz/appservice/runtimecore -run 'RunViewProjectsRunModel|ReadRunViewUsesOwnerReadAndProjectsView' -count=1
go test ./internal/services/workorch/biz/backends -run 'RemoteRunGetMapsDefinitionRevision|RemoteRunListMapsDefinitionRevision|EventHubRunResourceMapCompactsExecutionContext' -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchport ./internal/pkg/workorchclient -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-s2-1-definition-revision-model.md internal/modules/application/workorchd/biz/appservice/appmodelcore/run.go internal/modules/application/workorchd/biz/appservice/runtimecore/run_helpers.go internal/pkg/workorchclient/types.go internal/pkg/workorchport/run.go internal/services/workorch/biz/backends/remote.go internal/services/workorch/biz/backends/eventhub.go internal/modules/application/workorchd/biz/appservice/runports/views_test.go internal/services/workorch/biz/backends/remote_test.go internal/services/workorch/biz/backends/eventhub_test.go -S
git diff --check
```
