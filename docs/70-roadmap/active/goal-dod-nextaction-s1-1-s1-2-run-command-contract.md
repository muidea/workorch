# Goal/DoD NextAction S1.1-S1.2 Run Command Contract

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S0.1 Command Chain Audit](./goal-dod-nextaction-s0-1-command-chain-audit.md)

## Contract Audit

- Scope: `WorkspaceRunCommand` intent normalization and canonical run command vocabulary.
- Observed state: TUI help and command parser already expose `/run resume` as the only resume-style run command. Remote and EventHub backends still each normalize run command intent locally.
- Expected state: `internal/pkg/workorchport` owns canonical run command intent semantics. Empty intent normalizes to `resume`; supported intents are `resume`, `restart`, `replan` and `recover`; no legacy resume-style alias is part of the command contract.
- Deviation: intent validation was duplicated in backend implementations, so future changes could accidentally reintroduce non-canonical aliases in one backend but not another.
- Owner: `workorchport` owns process-facing command DTO contracts; remote and EventHub backends are adapters that must consume the same contract.
- Event path: TUI/agent/Web command -> `WorkspaceRunCommand` -> port intent normalization -> backend adapter -> workorchd scheduler command -> projection.
- Contract gap: no single reusable normalization function existed at the port boundary.
- Proposed change: add `NormalizeRunCommandIntent` to `workorchport`, use it from remote and EventHub run backends, and reject unsupported intents before any backend side effect.
- Boundary check: this is generic command vocabulary. No template, language, provider, file or workspace-specific logic is introduced.
- Verification: port unit tests for supported/default/unsupported intents, backend tests for unsupported intent rejection, and existing TUI tests proving `/run resume` is the canonical run command.

## Closure Items

1. Done: define canonical run command intent normalization in `internal/pkg/workorchport`.
2. Done: make remote and EventHub run backends consume the same normalization.
3. Done: keep resume-style command wording canonical in TUI help and command handling; unsupported intents are rejected before backend side effects.

## Verification

Commands:

```bash
go test ./internal/pkg/workorchport -count=1
go test ./internal/services/workorch/biz/backends -run 'RunCommand|RunRestart|FirstResume|ResumeSends|Replan|UnsupportedIntent' -count=1
go test ./internal/services/workorch/service/tui -run 'NonCanonicalRunCommands|RunResumeCommand|HelpPrefersRunResume' -count=1
rg -n "CommandRun[A-Za-z]+|RunCommandIntent[A-Za-z]+" internal/pkg/workorchport internal/services/workorch internal/pkg/workorchclient internal/modules/application/workorchd -g '!service/server/consoleassets/dist/**' -S
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" internal/pkg/workorchport/run.go internal/pkg/workorchport/run_test.go internal/services/workorch/biz/backends/remote.go internal/services/workorch/biz/backends/eventhub.go internal/services/workorch/biz/backends/remote_test.go internal/services/workorch/biz/backends/eventhub_test.go docs/70-roadmap/active/goal-dod-nextaction-s1-1-s1-2-run-command-contract.md -S
```

Result:

- Affected Go tests passed.
- No user-facing or port/backend run command path exposes a legacy resume-style alias.
- Boundary search found no template, language, command-output or provider-specific logic in changed implementation. The `city` hit is pre-existing test fixture text outside the changed lines.
