# S10.5 Template Capability MCP/LSP Binding

Status: completed

Owner: template/profile

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S10.2 Software Dev Stage Policy Binding](./goal-dod-nextaction-s10-2-software-dev-stage-policy-binding.md)
- [S10.3 Evidence Policy Coverage Binding](./goal-dod-nextaction-s10-3-evidence-policy-coverage-binding.md)
- [S10.4 File And Task Granularity Soul Guidance](./goal-dod-nextaction-s10-4-file-task-granularity-soul-guidance.md)
- [Workorchd State Loop And Projection Closure Plan](./workorchd-state-loop-and-projection-closure-plan.md)

## Contract Audit

- Scope: MCP/LSP capability binding for workspace templates and workspace activation.
- Observed state: `software-dev.contextHints.semanticCodeNavigation` declares `mcp-lsp-bridge` server and LSP capability IDs as optional auxiliary semantic navigation. Embedded MCP server/capability catalogs contain the concrete common tool definitions. Workspace creation strips common tool defaults and managed configs from workspace context; activation recomposes template/system defaults with explicit workspace overrides.
- Expected state: MCP/LSP remains template/profile capability data. Core and generic appservice code must not force `mcp-lsp-bridge` for every workspace. A software-development template can opt into LSP bridge as an optional auxiliary capability, while other templates stay unaffected unless they declare or override MCP defaults.
- Deviation: the implementation was mostly aligned, but S10.5 lacked a dedicated audit and regression tests proving that LSP bridge is not a forced core default and that every template-declared capability points to a catalog entry.
- Owner: template/profile data plus neutral activation composition. The LSP bridge default belongs to software-development template capability policy, not kernel/orchestrator state logic.
- Event path: embedded MCP catalog + workspaceTemplate `contextHints.semanticCodeNavigation` -> workspace template repository -> workspace creation strips copied common defaults -> activation reloads current template context -> `mcpSelection` run executionContext -> LLM/tool capability context.
- Contract gap: no explicit S10.5 record and no focused tests that lock the optional auxiliary semantics, catalog consistency, workspace override behavior, and non-declared template isolation.
- Proposed change: document S10.5, strengthen embedded defaults tests, and add activation regression coverage for a template that does not declare MCP defaults.
- Boundary check: no production core behavior is changed. No template ID, language, command output, or business scenario branch is added to kernel/orchestrator or generic appservice. `mcp-lsp-bridge` remains only in defaults, template-owned tests, and S10.5 documentation.
- Verification: defaults tests, appservice activation tests, full `go test ./...`, `git diff --check`, and boundary search.

## Capability Rules

- `mcp-lsp-bridge` is a shared common tool catalog entry. It can be available in the system catalog without being selected for every workspace.
- A workspace selects the tool only when its template or explicit workspace context declares default MCP server/capability IDs, or when selected skills require MCP references.
- Software-development templates may declare LSP bridge because semantic code navigation is generally useful for software workspaces.
- The capability is optional and auxiliary. It must not replace direct state-change evidence, validation evidence, build/test/lint output, or DoD coverage evidence.
- Workspace context overrides template defaults when explicitly set.
- Managed workspace configs are materialized on demand and should use the shared system default unless the workspace provides an explicit overlay.

## Acceptance

- Embedded software-development template declares LSP bridge through `semanticCodeNavigation` with `provider=mcp`, `optional=true`, and `evidenceStrength=auxiliary`.
- Every template-declared default MCP capability ID exists in the embedded MCP capability catalog.
- Workspace creation does not copy template common tool defaults or managed configs into each workspace.
- Activation selects template MCP defaults when declared, uses updated template defaults when workspace has no override, and respects explicit workspace overrides.
- A template without MCP defaults does not select `mcp-lsp-bridge` just because the system catalog contains it.
- Core remains business-neutral.

## Verification

- `go test ./internal/pkg/workorchdefaults -run 'TestEmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk|TestEmbeddedMCPDefaultsAreUpserted' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivateWorkspaceExecutionAppliesTemplateDefaultMCPContext|TestActivateWorkspaceExecutionDoesNotSelectMCPWithoutTemplateOrWorkspaceDefaults|TestActivateWorkspaceExecutionUsesUpdatedTemplateMCPDefaultWhenWorkspaceHasNoOverride|TestActivateWorkspaceExecutionUsesWorkspaceMCPDefaultOverride' -count=1`
- `go test ./internal/pkg/workorchdefaults ./internal/modules/application/workorchd/biz/appservice -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg/workorchdefaults -g '!**/defaults/**'`
