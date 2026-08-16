# Goal/DoD NextAction S0.1 Command Chain Audit

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [EventHub Driven Workorchd State Loop Design](../superseded/eventhub-driven-workorchd-state-loop-design.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: resume / replan / restart / governance resolution command flow.
- Observed state: decision resolve currently has two continuation paths. `governanceports.ResolveDecisionView` and `BatchResolveDecisionViews` directly publish kernel orchestrator resume, while HTTP governance handlers publish `TopicWorkorchDCommandWorkspaceRunRequested` and let workorchd scheduler accept, project, and enqueue continuation.
- Expected state: TUI, Web, REST and embedded CLI only create scoped commands/events. `workorchd` scheduler owns command acceptance, projection refresh and runtime continuation scheduling. Kernel orchestrator resume is only reached from scheduler-owned execution, not from read-model governance ports.
- Deviation: appservice governance port both resolves owner state and triggers runtime continuation. This bypasses workorchd scheduler command projection, creates different behavior between HTTP and embedded/TUI paths, and can double-schedule HTTP decision resolution.
- Owner: workorchd appservice and workorchd scheduler command path. This is core command plumbing, not workspaceTemplate/Soul policy.
- Event path: endpoint or CLI command -> appservice resolve decision/approval owner state -> publish `TopicWorkorchDCommandWorkspaceRunRequested` -> workorchd scheduler accepted response -> projection `RunCommand` accepted -> scheduler queue -> runtime resume/replan/restart.
- Contract gap: decision resolution was allowed to bypass scheduler acceptance and projection. Embedded CLI governance resolution also relied on that bypass, so removing it requires dispatcher-side scheduler command publish.
- Proposed change: make `governanceports` resolve-only; publish governance continuation from entry orchestration layers. HTTP already does this. CLI dispatcher must publish the same workorchd scheduler command after successful decision/approval resolution and fail the CLI command if scheduler acceptance is missing.
- Boundary check: the change uses only generic governance resolution type, workspace/run scope, intent and request id. No template, language, provider, file or business-specific rule is added to core.
- Verification: update governanceports tests to prove decision resolve no longer publishes orchestrator resume; add CLI dispatcher test proving decision resolve publishes scheduler command and does not publish direct orchestrator resume; run affected Go packages and boundary search.

## Current Command Paths

| Command | Current owner path | Gap |
| --- | --- | --- |
| `/run resume` / REST run resume | `TopicWorkorchDCommandWorkspaceRunRequested` -> scheduler -> run continuation | canonical path exists |
| `/run restart` / REST restart | `TopicWorkorchDCommandWorkspaceRunRequested` -> scheduler -> workspace restart | canonical path exists |
| decision resolve via HTTP | resolve decision -> scheduler command -> projection -> continuation | canonical path exists, but direct appservice resume causes duplicate path |
| decision resolve via CLI dispatcher | resolve decision -> direct orchestrator resume from governanceports | bypasses scheduler and projection |
| approval resolve via HTTP | resolve approval -> scheduler command -> projection -> continuation | canonical path exists |
| approval resolve via CLI dispatcher | resolve approval only | needs same scheduler command if the resolved approval belongs to a run |

## Closure Items

1. Done: remove direct `TopicOrchestratorCommandResumeRun` publish from `governanceports` decision resolve functions.
2. Done: add CLI dispatcher governance continuation publish to `TopicWorkorchDCommandWorkspaceRunRequested`.
3. Done: map decision `resolutionType=replan` to `intent=replan`; all other decision and approval resolutions use `intent=resume`.
4. Done: require scheduler accepted response for CLI governance continuation. Missing or rejected response is a command failure because owner scheduling was not accepted.
5. Done: keep HTTP governance continuation path unchanged.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/biz/appservice/governanceports -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/service/server -run 'Governance|Scheduler|WorkspaceRunCommand|RunCommand|Decision|Approval' -count=1
rg -n "TopicOrchestratorCommandResumeRun|publishDecisionResolutionRunResume|ResumeRunCommand" internal/modules/application/workorchd/biz/appservice/governanceports internal/modules/application/workorchd/biz/appservice/cli_*.go -S
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" internal/modules/application/workorchd/biz/appservice/cli_governance_continuation.go internal/modules/application/workorchd/biz/appservice/cli_governance_commands.go internal/modules/application/workorchd/biz/appservice/governanceports/decision_resolve_views.go internal/modules/application/workorchd/biz/appservice/governanceports/decision_batch_resolve_views.go docs/70-roadmap/active/goal-dod-nextaction-s0-1-command-chain-audit.md -S
```

Result:

- Affected Go tests passed.
- Direct orchestrator resume references remain only in negative tests proving governance ports and CLI dispatcher no longer publish that command.
- Boundary search found no template, language, command, provider or workspace-specific rule in the changed implementation.
