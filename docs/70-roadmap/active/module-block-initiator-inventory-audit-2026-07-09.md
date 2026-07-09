# Module / Block / Initiator Inventory Audit

Status: draft

Date: 2026-07-09

Related:

- `module-block-contract-closure-validation-plan.md`
- `../../00-core/runtime-stage-contract.md`
- `../../30-module-architecture/llm-provider-assembly.md`

## Purpose

This audit freezes the current registered `module`, runtime `block`, and
`initiator` inventory and checks whether each component still matches its
functional definition. The check is structural and contract-oriented: lifecycle,
owner state, cross-module boundary, and runtime neutrality are more important
than file size.

## Functional Definition Baseline

| Kind | Functional definition |
| --- | --- |
| initiator | Process-level infrastructure provider. It may provide repository providers, EventHub wrappers, runtime policy, config loading, route registry, client factory, or background routine. It must not own workspace/run/result business state or business strategy. |
| kernel module | Formal owner of one business or governance state family. It owns repository access for that state, subscribes to owner command/query topics, publishes formal events, and must be the authority for its state transitions. |
| application module | Product/API/application use-case surface. It may assemble routes, projections, command facades, context registry, and configuration surfaces. It must not become the owner of kernel state or repair owner state through projection. |
| runtime block | Runtime capability adapter for technical execution or external systems. It produces normalized execution results/evidence candidates, but must not own formal business state, decide run completion, or mutate coverage/result state directly. |
| process service | Process mode selection and user surfaces under `internal/services/<entry>`. It is not a plugin module and must depend on stable `workorchport` / `workorchclient` contracts rather than owner repositories. |

## Registered Inventory Snapshot

| Kind | ID / component | Code path | Current fit |
| --- | --- | --- | --- |
| initiator | `workorch/runtime` | `internal/initiators/workorchruntime` | Fits initiator role: repository provider, EventHub owner wrapper, runtime policy. |
| application module | `application/workorchd` | `internal/modules/application/workorchd` | Fits application role, but has borrowed-export validation gaps listed below. |
| kernel module | `kernel/workspace` | `internal/modules/kernel/workspace` | Fits owner role for Workspace / Goal / DoD, with active-run clearing and event-count gaps listed below. |
| kernel module | `kernel/run` | `internal/modules/kernel/run` | Fits owner role for Run / Step lifecycle and runtime checkpoints. |
| kernel module | `kernel/orchestrator` | `internal/modules/kernel/orchestrator` | Fits orchestration authority role for convergence, evidence, coverage, repair/replan, and block dispatch. |
| kernel module | `kernel/result` | `internal/modules/kernel/result` | Fits Result / Artifact owner role. |
| kernel module | `kernel/decision` | `internal/modules/kernel/decision` | Fits DecisionTicket / Resolution owner role. |
| kernel module | `kernel/approval` | `internal/modules/kernel/approval` | Fits Approval owner role. |
| kernel module | `kernel/governance` | `internal/modules/kernel/governance` | Fits governance trace / recommendation role. |
| kernel module | `kernel/eventstream` | `internal/modules/kernel/eventstream` | Fits audit event storage/query role; not an owner-state repair authority. |
| kernel module | `kernel/memory` | `internal/modules/kernel/memory` | Fits memory and recall trace owner role. |
| kernel module | `kernel/mcp` | `internal/modules/kernel/mcp` | Fits MCP server/capability registry role. |
| kernel module | `kernel/skill` | `internal/modules/kernel/skill` | Fits skill registry role. |
| kernel module | `kernel/soul` | `internal/modules/kernel/soul` | Fits soul catalog/stage policy metadata role. |
| kernel module | `kernel/system` | `internal/modules/kernel/system` | Fits infrastructure/status role; intentionally narrow. |
| runtime block | `blocks/file` | `internal/modules/blocks/file` | Fits block role; registers file list/read/edit/patch execution kinds. |
| runtime block | `blocks/git` | `internal/modules/blocks/git` | Fits block role; registers git read/write execution kinds. |
| runtime block | `blocks/llm` | `internal/modules/blocks/llm` | Fits LLM provider/context block role; cache-prefix and critical-field contracts remain high-risk and need dedicated tests when changed. |
| runtime block | `blocks/mcp` | `internal/modules/blocks/mcp` | Fits MCP invocation block role. |
| runtime block | `blocks/shell` | `internal/modules/blocks/shell` | Fits shell execution block role. |
| runtime block | `blocks/ssh` | `internal/modules/blocks/ssh` | Fits SSH execution block role. |
| runtime block | `blocks/web` | `internal/modules/blocks/web` | Fits web/search/fetch block role. |
| process/product surface | workorch service / port / client / web | `internal/services/workorch`, `internal/pkg/workorchport`, `internal/pkg/workorchclient`, `web` | Fits process/product surface role; should stay outside `internal/modules`. |

## Initial Gaps

### Gap 1. Borrowed runtime exports are under-validated

- Area: `application/workorchd` runtime session.
- Evidence:
  - `bootstrap.Exports` includes `CoverageWorkItems`.
  - `ExportsFromInitiator` resolves orchestrator repositories but does not copy `CoverageWorkItems` into `RepositoryExports`.
  - `runtimecore.HasRuntimeExports` only checks `Infrastructure.EventHub`, so incomplete borrowed exports can pass session construction and fail later as generic `invalid argument`.
- Functional risk:
  - Application projection and read-model code can run with a partial owner repository surface.
  - Missing dependencies are discovered downstream instead of fail-fast at the application/initiator boundary.
- Closure direction:
  - Copy `orchestratorRepos.CoverageWorkItems` in `ExportsFromInitiator`.
  - Strengthen `HasRuntimeExports` or add a named validation function for the exact appservice repository/export set.
  - Add a pairwise test: initiator exports -> appservice session must include all owner repositories used by projection/runtime sessions.

### Gap 2. Workspace definition-change contract is not fully closed

- Area: `kernel/workspace`.
- Evidence:
  - `go test ./internal/modules/... -count=1` fails in `kernel/workspace/biz`.
  - `TestGoalDoDManager_DefinitionChangeReopensTerminalWorkspace` expects terminal workspace definition changes to reopen the workspace without retaining `ActiveRunID`, but `markWorkspaceDefinitionsChanged` currently does not clear `ActiveRunID`.
  - `TestGoalDoDManager_PublishesDefinitionNotificationsThroughAsyncHub` reports unexpected publish counts for definition notifications.
- Functional risk:
  - Workspace definition revision changes may leave stale run binding, which violates the owner invariant that a reopened definition should not keep a terminal run as active authority.
  - Event consumers can see more definition-related notifications than the pairwise contract expects.
- Closure direction:
  - Clarify whether definition changes should publish both workspace-state and definition events, then make test and implementation agree.
  - Clear stale `ActiveRunID` when a terminal/superseded workspace is reopened by Goal/DoD definition change.

### Gap 3. Application module tests expose broad invalid-argument failures

- Area: `application/workorchd/biz/appservice` and `application/workorchd/service/server`.
- Evidence:
  - `go test ./internal/modules/... -count=1` fails many appservice tests at `CreateWorkspaceWithSession`, `ReadRunSnapshotWithSession`, and related backend/session helpers with generic `invalid argument`.
  - `TestServiceRunReturnsErrorWhenBackgroundSubmitFails` also fails in the server service package.
- Functional risk:
  - The failing shape suggests setup/borrowed runtime dependencies are not failing at the owning boundary with a specific missing dependency.
  - This makes module/block closure harder because consumer failures are not attributed to one provider contract.
- Closure direction:
  - Fix the initiator/application export validation first, then rerun appservice tests to separate real appservice deviations from setup noise.
  - Convert generic missing-dependency failures into named errors or test assertions at the session boundary.

## Boundary Checks

| Check | Current result |
| --- | --- |
| Entry side-effect imports live in `application/workorch/main.go` | Pass. The entry explicitly imports the initiator, application module, kernel modules, and runtime blocks. |
| Runtime blocks own no formal workspace/run/result state | Pass by structure. Blocks register `runtimeevent.Executor` handlers and hold adapters/executors, not owner repositories. |
| Kernel modules expose owner command/query topics | Pass by structure. Kernel modules subscribe to module-specific EventHub topics in `Start`. |
| Application/service code stays out of `internal/modules/kernel/*/biz` direct owner implementation | Needs focused scan after setup fixes. Current appservice uses owner ports heavily, but tests still use repositories as fixtures. |
| Core runtime neutrality | Needs targeted scan, not complete in this audit. Known template/profile terms appear mostly in tests/default data; production kernel/runtime changes still need the standard neutrality scan before closure. |

## Verification Run

```bash
go test ./internal/initiators/... -count=1
```

Result: pass.

```bash
go test ./internal/modules/... -count=1
```

Result: fail.

Primary observed failures:

- `internal/modules/application/workorchd/biz/appservice`: broad `invalid argument` setup/session failures.
- `internal/modules/application/workorchd/service/server`: background submit failure expectation mismatch.
- `internal/modules/kernel/workspace/biz`: definition notification count and stale `ActiveRunID` on definition reopen.

## Recommended Closure Order

1. Close the initiator -> application export contract first.
2. Close the workspace definition-change owner invariant.
3. Rerun `go test ./internal/modules/application/workorchd/biz/appservice -count=1` and split remaining failures by owner boundary.
4. Rerun `go test ./internal/modules/kernel/workspace/biz -count=1`.
5. Re-run full `go test ./internal/modules/... -count=1`.
