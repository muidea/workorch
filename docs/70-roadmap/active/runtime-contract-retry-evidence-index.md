# Runtime Contract Retry Evidence Index

## Purpose

This is the L2 evidence index for the runtime contract retry convergence audit. It preserves exact raw source locations and bounded extraction commands. L0/L1 documents should reference these evidence IDs instead of copying raw payloads.

## Evidence Records

### ev:code/convergence-budget-intermediate-graph-nodes/20260623-031

| Field | Value |
| --- | --- |
| claimId | `B7-G3` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-003-replan-001-replan-001-replan-001` |
| evidenceKind | code-and-live-observation |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`, `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget_test.go` |
| extractCommand | `go test ./internal/modules/kernel/orchestrator/biz -run 'TestUpdateConvergenceBudgetDoesNotCountIntermediateCompletedGraphNodesAsNoDelta|TestUpdateConvergenceBudgetDoesNotCountPendingExecutionGraphAsNoDelta|TestUpdateConvergenceBudgetDoesNotCountPatchBaseHashMismatchAsNoDelta|TestUpdateConvergenceBudgetDoesNotCountPatchOldTextNotFoundAsNoDelta' -count=1 -timeout=240s; go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh` |
| extractedSummary | After redeploy and explicit scoped `replan`, both live workspaces created fresh running successor runs and produced accepted execution graphs with real patch/validation activity. `ws-7c...` then escalated on `noCoverageDeltaRounds 4/3` even though several counted rounds were successful intermediate graph nodes such as file reads and patch applications that cannot be expected to move coverage before validation/result recording. The closure makes completed graph nodes without `validationEvidence` skip no-delta accounting while preserving no-delta accounting for completed validation evidence nodes and failed steps. This keeps convergence budget business-neutral and prevents multi-node plans from exhausting coverage budget before their verification nodes run. Focused and full orchestrator/biz tests plus boundary verification passed. |
| owner | core convergence budget |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/focus-contract-required-output-preservation/20260623-032

| Field | Value |
| --- | --- |
| claimId | `B7-G3` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-003-replan-001-replan-001-replan-001-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`, `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go`, `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract_test.go` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; jq '{repairActionPackage:{requiredOutput:.repairActionPackage.requiredOutput,coverageFocus:.repairActionPackage.coverageFocus,coverageFocusBindingRequired:.repairActionPackage.coverageFocusBindingRequired},coverageFocus,stateChangingOutputRequired,repairObjective}' .workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-003-replan-001-replan-001-replan-001-replan-001-replan-001/steps/run-003-replan-001-replan-001-replan-001-replan-001-replan-001/003-*/input.json; go test ./internal/modules/kernel/orchestrator/biz -run 'TestStateChangingConvergencePreservesCoverageFocusRequiredOutput|TestRepairCoverageFocusContractSurvivesEvidenceRequiredOutputRebuild|TestValidateLLMRepairCoverageFocusAcceptsStateChangingReasonBinding|TestRepairActionPackageDeclaresCoverageFocusBinding|TestValidateLLMRepairCoverageFocusNormalizesEvidenceGraphBinding' -count=1 -timeout=240s; go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh; git diff --check` |
| extractedSummary | After redeploy and scoped `replan`, both workspaces correctly created fresh successor runs. `ws-4e...` then blocked on `repair_focus_mismatch`: the step input carried `repairActionPackage.coverageFocusBindingRequired=true` and a concrete `coverageFocus`, but the rebuilt `repairActionPackage.requiredOutput` omitted the mechanical `mustBindCoverageFocus`, `coverageFocusId`, `coverageWorkItemIds`, and binding rule fields. The runtime validator still enforced the focus contract, so provider output that only mentioned the focus in `stateChangingReason` was rejected and consumed convergence budget. Local core closure reapplies the coverage focus contract after requiredOutput rebuilds and inside evidence/validation required-output helpers, accepts focus tokens in neutral tool-contract fields such as `stateChangingReason`, `boundedReason`, and `verificationScope`, and additionally covers the state-changing convergence existing-package path where `mustIncludeStateChangingNode=true` previously returned before focus fields were restored. Patch/apply focus binding remains strict. Focused and full orchestrator/biz tests plus boundary verification passed. `ws-7c...` separately remains blocked by destructive/background service validation shape, which is not closed by this evidence item. |
| owner | core runtime repair contract consistency |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/software-dev-lsp-tool-surface/20260622-019

| Field | Value |
| --- | --- |
| claimId | `B7-G3` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/pkg/workorchdefaults/defaults/mcp_servers.json`, `internal/pkg/workorchdefaults/defaults/mcp_capabilities.json`, `internal/pkg/workorchdefaults/defaults/templates.json`, `internal/pkg/workorchdefaults/defaults/souls.json`, `internal/pkg/workorchdefaults/defaults_test.go` |
| extractCommand | `rg -n "lsp_status|lsp_shutdown|lsp_repair" internal/pkg/workorchdefaults/defaults internal/pkg/workorchdefaults/defaults_test.go docs/40-profiles-capabilities/software-dev/lsp-bridge-integration.md` |
| extractedSummary | `software-dev` profile/default MCP data now declares the full LSP bridge tool surface used by the profile, including `lsp_status`, `lsp_shutdown`, and `lsp_repair`. This closes the observed profile capability binding gap where a generated `mcp/tool_call` for `lsp_status` was rejected by the MCP server `allowedTools` whitelist. The change is profile/template data, not a core runtime special case. |
| owner | template/profile capability data |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/bootstrap-system-defaults/20260622-020

| Field | Value |
| --- | --- |
| claimId | `B7-G3` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | `internal/modules/application/workorchd/biz/bootstrap/assembly.go`, `internal/modules/application/workorchd/biz/bootstrap/runtime_test.go`, `internal/modules/application/workorchd/biz/appservice/workspaceports/workspace_templates_test.go` |
| extractCommand | `go test ./internal/modules/application/workorchd/biz/bootstrap -count=1; go test ./internal/pkg/workorchdefaults -count=1; go test ./internal/modules/application/workorchd/biz/appservice/... -count=1; ./scripts/verify-boundary-contract.sh` |
| extractedSummary | Bootstrap runtime construction now starts the `kernel/system` module against the selected backend repositories, so embedded defaults are upserted into memory/file runtime sessions instead of only being available through the framework main path. Regression coverage proves `NewWithBackend("memory", "")` exposes the updated `mcp-lsp-bridge` `allowedTools`, the `lsp_status` capability catalog entry, and the `software-dev` semantic navigation default capability binding. Workspace template port tests were adjusted to expect built-in templates after system defaults startup while still verifying owner-port create/read/update/delete behavior. Focused bootstrap/defaults tests, appservice package tests, and boundary verification passed locally. |
| owner | bootstrap/system module wiring |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/eventhub/magic-publish/001

| Field | Value |
| --- | --- |
| claimId | `B1-SM-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/pkg/events/magic_hub.go` |
| extractCommand | `sed -n '80,170p' internal/pkg/events/magic_hub.go` |
| extractedSummary | `MagicHub.Publish` prepares envelope, assigns workspace revision, dispatches local wildcard handlers, binds `EventHubLaneKey`, then calls magicCommon `hub.Send`. `PublishAsync` exists and calls `hub.Post`. |
| owner | core |
| closureBatch | 1 |
| loadByDefault | false |

### ev:code/eventhub/lane-policy/001

| Field | Value |
| --- | --- |
| claimId | `B1-SM-006` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/pkg/events/event_contract.go`, `internal/modules/*/*/pkg/events/contracts.go` |
| extractCommand | `sed -n '50,120p' internal/pkg/events/event_contract.go; sed -n '210,300p' internal/pkg/events/event_contract.go; sed -n '1,120p' internal/modules/kernel/orchestrator/pkg/events/contracts.go` |
| extractedSummary | Lane policy is resolved from per-topic contracts. Owner write commands generally use `LanePolicyState`; runtime execute commands use execute lanes; runtime accepted/result topics use special runtime/orchestrator state lanes; many notify topics use topic lanes. |
| owner | core |
| closureBatch | 1 |
| loadByDefault | false |

### ev:code/scheduler/workspace-run-request/001

| Field | Value |
| --- | --- |
| claimId | `B1-SM-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go`, `internal/modules/application/workorchd/service/server/http_run.go`, `internal/modules/application/workorchd/service/server/http_workspace.go` |
| extractCommand | `sed -n '250,380p' internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go; sed -n '266,350p' internal/modules/application/workorchd/service/server/http_run.go` |
| extractedSummary | Workspace run commands publish `/workorchd/command/workspace-run/requested`; scheduler handler validates intent and queues activation, restart, replan, recover, or run continuation; accepted response is returned after scheduling succeeds. |
| owner | workorchd scheduler |
| closureBatch | 1 |
| loadByDefault | false |

### ev:code/orchestrator/dispatch-request/001

| Field | Value |
| --- | --- |
| claimId | `B1-SM-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`, `internal/modules/kernel/orchestrator/module.go` |
| extractCommand | `sed -n '1,130p' internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go; sed -n '430,470p' internal/modules/kernel/orchestrator/module.go` |
| extractedSummary | `requestDispatchStep` publishes `/orchestrator/dispatch-step/requested` with workspace/run/step scope and idempotency key; module handler schedules `DispatchRequestedStep` via background routine when available. |
| owner | core orchestrator |
| closureBatch | 1 |
| loadByDefault | false |

### ev:code/runtime/block-executor/001

| Field | Value |
| --- | --- |
| claimId | `B1-SM-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/blocks/pkg/runtimeevent/runtime_event.go`, `internal/modules/kernel/orchestrator/runtime_event_adapter.go` |
| extractCommand | `sed -n '90,220p' internal/modules/blocks/pkg/runtimeevent/runtime_event.go; sed -n '1,120p' internal/modules/kernel/orchestrator/runtime_event_adapter.go` |
| extractedSummary | Runtime executor accepts command, records queue state, publishes accepted response, starts background worker, then publishes progress and terminal completed/failed events; orchestrator event runtime adapter can request accepted execution and subscribe for terminal events. |
| owner | runtime block infrastructure |
| closureBatch | 1 |
| loadByDefault | false |

### ev:code/projection/manager/001

| Field | Value |
| --- | --- |
| claimId | `B1-SM-007` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/service/server/workspace_projection.go`, `internal/modules/application/workorchd/service/server/event_subscriptions.go` |
| extractCommand | `sed -n '1,140p' internal/modules/application/workorchd/service/server/workspace_projection.go; sed -n '1,80p' internal/modules/application/workorchd/service/server/event_subscriptions.go` |
| extractedSummary | workorchd creates a `workspaceviewcore.Manager`; projection stream subscribes to `/workorchd/workspace/projection/changed`, while full stream additionally subscribes to all WorkOrch topics. |
| owner | projection |
| closureBatch | 1 |
| loadByDefault | false |

### ev:code/projection/task-derived/001

| Field | Value |
| --- | --- |
| claimId | `B2-TASK-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_projection.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_projection.go` |
| extractCommand | `sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_projection.go; sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_projection.go` |
| extractedSummary | Task decompositions are derived by scanning `snapshot.Steps` and reading `step.Output["executionGraph"]`; the current closure keeps this as projection-owned read model data rather than a separate task-plan owner record. Post-closure task plan lifecycle metadata is projected in the `TaskPlans` view. |
| owner | projection |
| closureBatch | 2 |
| loadByDefault | false |

### ev:code/projection/task-node-mapping/001

| Field | Value |
| --- | --- |
| claimId | `B2-TASK-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_execution_steps.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_state.go` |
| extractCommand | `sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_execution_steps.go; sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_state.go` |
| extractedSummary | Dispatched node state is projected by matching later steps with `graphRootStepId` and `graphNodeId`; post-closure node lifecycle metadata is carried into `TaskPlans` so clients do not need to infer retry/rejected/blocked state from raw progress items. |
| owner | projection |
| closureBatch | 2 |
| loadByDefault | false |

### ev:code/projection/task-plan/001

| Field | Value |
| --- | --- |
| claimId | `B2-TASK-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_projection.go`, `task_plan_item_projection.go`, `task_plan_revisions.go` |
| extractCommand | `sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_projection.go; sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_item_projection.go` |
| extractedSummary | Task plans are projected from task decompositions and retry/correction steps, grouped by anchor step, revision-ordered, and marked with source projection plus lifecycle metadata such as output state, retry lineage, violation/failure class, target scope, blocking ticket, and projected nodes. |
| owner | projection |
| closureBatch | 2 |
| loadByDefault | false |

### ev:code/projection/progress-mixed-items/001

| Field | Value |
| --- | --- |
| claimId | `B2-TASK-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/progress_items.go` |
| extractCommand | `sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/progress_items.go` |
| extractedSummary | `TaskProgressItems` remains the mixed activity timeline for steps, tool calls, patches, validation results, results, artifacts, decisions, and approvals. Structured plan state is now exposed separately through `TaskPlans`, so this mixed list is no longer the only client-facing task-progress surface. |
| owner | projection |
| closureBatch | 2 |
| loadByDefault | false |

### ev:code/evidence/result-recording/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go` |
| extractCommand | `sed -n '120,285p' internal/modules/kernel/orchestrator/biz/runtime_result_recording.go` |
| extractedSummary | Validated DoD assessment recording collects evidence refs, creates summary/manifest/patch/web/git artifact refs, writes an accepted result, and publishes result accepted events. Rejected completion assessments are recorded separately as rejected diagnostic results. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/evidence/slice-result-recording/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go` |
| extractCommand | `sed -n '285,380p' internal/modules/kernel/orchestrator/biz/runtime_result_recording.go` |
| extractedSummary | Coverage-progress assessments can update the coverage ledger and create accepted slice results when acceptance slices allow result recording, without treating the slice as the final completion result. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/evidence/rejected-result-boundary/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-008` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go` |
| extractCommand | `sed -n '456,690p' internal/modules/kernel/orchestrator/biz/runtime_result_recording.go` |
| extractedSummary | Rejected DoD assessments are recorded as `ResultStateRejected` with source/evidence refs. A later recovery step can supersede a rejected result, while accepted completion results require accepted state, matching workspace/run/goal/DoD scope, satisfied DoD assessment, and non-empty evidence refs. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/evidence/artifact-recording/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_result_artifacts.go` |
| extractCommand | `sed -n '1,180p' internal/modules/kernel/orchestrator/biz/runtime_result_artifacts.go` |
| extractedSummary | Result artifact recording creates available artifacts with workspace/run/result/step scope, category, URI, content type, evidence refs, and metadata; existing artifacts are updated idempotently on conflict. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/evidence/memory-boundary/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-009` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_memory.go`, `internal/modules/kernel/orchestrator/biz/runtime_result_memory.go` |
| extractCommand | `sed -n '140,225p' internal/modules/kernel/orchestrator/biz/runtime_memory.go; sed -n '177,235p' internal/modules/kernel/orchestrator/biz/runtime_result_memory.go` |
| extractedSummary | Runtime memory writes reject transient, diagnostic-only, rejected-output, and contract-violation signals. Run-summary memory is written from validated DoD assessment with run/workspace scope, sourceRef, evidence refs, runtime_result tags, importance, and confidence. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/coverage/work-item-ledger/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go` |
| extractCommand | `sed -n '1,260p' internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go` |
| extractedSummary | Coverage work items are stored in run executionContext, updated from completed steps, failed steps, DoD assessments, and governance owner evidence, and synchronized into acceptance projection context. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/coverage/acceptance-gate/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go` |
| extractCommand | `sed -n '260,335p' internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go` |
| extractedSummary | Final acceptance is blocked when coverage work items are unresolved, missing required evidence kinds, or marked satisfied without accepted evidence refs. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/projection/coverage-summary/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-006` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go` |
| extractCommand | `sed -n '1,180p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go` |
| extractedSummary | Projection derives coverage summary, gap baseline, accepted evidence refs, rejected evidence refs, and todo items from coverageWorkItemContext and coverageWorkItems in the run snapshot. |
| owner | projection |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/projection/dod-evidence-trace/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-010` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/dod_evidence_item.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/dod_evidence_trace.go` |
| extractCommand | `sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/dod_evidence_item.go; sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/dod_evidence_trace.go` |
| extractedSummary | DoD evidence projection compares assessment item evidence refs with the runtime evidence index and marks each item as linked, partial, or missing with missingEvidenceRefs. |
| owner | projection |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/projection/result-artifact-owner-read/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-011` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/ownerreadports/result_reads.go`, `internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_reads.go`, `internal/modules/application/workorchd/biz/appservice/readmodelports/run_result_pages.go` |
| extractCommand | `sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/ownerreadports/result_reads.go; sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_reads.go; sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelports/run_result_pages.go` |
| extractedSummary | Read model pages obtain results, validations, patches, and artifacts through owner commands over EventHub with workspace/run scope before projecting paginated views. |
| owner | projection |
| closureBatch | 6 |
| loadByDefault | false |

### ev:code/projection/result-artifact-view/001

| Field | Value |
| --- | --- |
| claimId | `B6-ECP-007` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/artifact_projection.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/result_quality.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/runtime_evidence_summary.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_run_merge.go`, `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_result_artifact_merge.go` |
| extractCommand | `sed -n '1,120p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/artifact_projection.go; sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/result_quality.go; sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/runtime_evidence_summary.go; sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_run_merge.go; sed -n '1,80p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_result_artifact_merge.go` |
| extractedSummary | Projection exposes artifact scope/evidence refs, result quality traces, and runtime evidence totals for tool calls, patches, validations, checkpoints, results, artifacts, and events. Workspace snapshots now merge active run `Results` and `Artifacts`, replacing stale same-ID workspace-base entries with run-level accepted result/artifact detail while preserving result linkage, state, category, and evidence refs. |
| owner | projection |
| closureBatch | 6 |
| loadByDefault | false |

### ev:verify/ci/managed-mock/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | ci |
| sourcePath | `.github/workflows/ci.yml`, `scripts/verify-workorch-managed-full.sh` |
| extractCommand | `sed -n '1,90p' .github/workflows/ci.yml; sed -n '150,210p' scripts/verify-workorch-managed-full.sh` |
| extractedSummary | CI runs `make verify-boundary-contract`, then `verify-workorch-managed-full.sh` with `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file`. The managed verifier runs fmt-check, console-build, vet, go test, build, starts a managed workorch server, applies LLM config, and runs the selected target. CI intentionally excludes real-provider checks. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/make/e2e-matrix/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | makefile |
| sourcePath | `Makefile` |
| extractCommand | `sed -n '160,286p' Makefile` |
| extractedSummary | Makefile exposes CI-safe deployed checks, contract violation recovery, acceptance slice result, projection switch, run resume gap confirmation, web console projection, TUI checks, real runtime closure, real provider checks, full e2e, managed full verification, and governed loadtest targets. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/boundary-contract/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | script |
| sourcePath | `scripts/verify-boundary-contract.sh` |
| extractCommand | `sed -n '1,180p' scripts/verify-boundary-contract.sh` |
| extractedSummary | Boundary verifier scans reusable runtime/Web paths for weather-specific, software-dev-specific, and LSP profile-specific terms while allowing concrete scenarios in tests/docs/e2e/defaults. It fails if reusable runtime layers contain business-specific fallback logic. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/unit/eventhub-lane/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | test |
| sourcePath | `internal/pkg/events/magic_hub_test.go` |
| extractCommand | `sed -n '1,120p' internal/pkg/events/magic_hub_test.go; sed -n '604,720p' internal/pkg/events/magic_hub_test.go` |
| extractedSummary | EventHub tests cover publishing through magicCommon hub, workspace-scoped revisions, wildcard/unsubscribe behavior, and lane independence so run owner updates and reads are not blocked behind unrelated active orchestrator lanes. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/unit/remote-projection-command/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | test |
| sourcePath | `internal/services/workorch/biz/backends/remote_test.go` |
| extractCommand | `sed -n '1,140p' internal/services/workorch/biz/backends/remote_test.go; sed -n '300,455p' internal/services/workorch/biz/backends/remote_test.go` |
| extractedSummary | Remote backend tests cover SSE subscription, projection stream view query, workspace-scoped run status, first resume activation metadata, resume request IDs, and replan as a unified workspace run command. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/e2e/projection-resume/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-006` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | script |
| sourcePath | `scripts/verify-workorch-projection-workspace-switch.sh`, `scripts/verify-run-resume-gap-confirmation-e2e.sh` |
| extractCommand | `sed -n '1,180p' scripts/verify-workorch-projection-workspace-switch.sh; sed -n '1,140p' scripts/verify-run-resume-gap-confirmation-e2e.sh` |
| extractedSummary | Projection switch e2e opens an SSE listener, creates two workspaces, observes projection events, and verifies scope isolation. Run resume gap confirmation e2e can manage its own instance, configures file backend/runtime limits, and verifies resume/gap baseline behavior. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/resource/real-runtime-load/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-007` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | script |
| sourcePath | `scripts/verify-real-runtime-closure.sh`, `scripts/loadtest-governed-workspace-concurrency.sh` |
| extractCommand | `sed -n '1,180p' scripts/verify-real-runtime-closure.sh; sed -n '1,160p' scripts/loadtest-governed-workspace-concurrency.sh` |
| extractedSummary | Real runtime closure verifier performs ready/metrics preflight, checks scheduler/watchdog/resource metric markers, validates stale dispatched threshold, runs governed loadtest with configurable total/concurrency/success rate, and then runs observation/cache/resource checks. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/tui/projection-only/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-008` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | script |
| sourcePath | `scripts/verify-tui-agent.sh`, `scripts/verify-tui-workspace-switch-visual.sh`, `scripts/verify-tui-terminal-render-smoke.py` |
| extractCommand | `sed -n '1,150p' scripts/verify-tui-agent.sh; sed -n '1,160p' scripts/verify-tui-workspace-switch-visual.sh; sed -n '1,120p' scripts/verify-tui-terminal-render-smoke.py` |
| extractedSummary | TUI verifier enforces no direct workorchclient import, no fixed polling, no legacy local backend, no agent polling/direct sessions, and runs TUI/backend/agent tests. Visual and PTY checks verify workspace switch projection refresh, stale view isolation, terminal render, fixed panels, and mouse reporting behavior. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/unit/runtime-regression-map/001

| Field | Value |
| --- | --- |
| claimId | `B7-VFY-009` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | test-index |
| sourcePath | `internal/modules/kernel/orchestrator`, `internal/modules/blocks/llm`, `internal/modules/application/workorchd/biz/appservice` |
| extractCommand | `rg -n "func Test.*(Controlled|Concrete|StateChanging|Patch|Contract|Violation|Retry|Coverage|Accepted|Rejected|Artifact|Projection|Context|Cache|StablePrefix|Snapshot|Available)" internal/modules/kernel/orchestrator internal/modules/blocks/llm internal/modules/application/workorchd/biz/appservice -g '*_test.go'` |
| extractedSummary | Existing unit coverage includes contract violation retry, state-changing repair contract preservation, coverage work items, rejected results, result/artifact recording, graph follow-up, patch/apply safety, prompt cache/stable prefix, current-action, snapshot safety, projection snapshots, workspace view store/manager, and run status projection. |
| owner | verification |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/contract/concrete-validation/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` |
| extractCommand | `sed -n '224,268p' internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` |
| extractedSummary | `validateConcreteValidationEvidenceOutputContract` requires concrete validation evidence when the input requires it. `validateConcreteValidationBehaviorEvidenceGraph` accepts declared validation evidence nodes and otherwise rejects graphs without `validationEvidence.mode`, `validationEvidence.observationType`, and controlled-substitute substitute metadata. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/shell-safety/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` |
| extractCommand | `sed -n '269,430p' internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` |
| extractedSummary | Concrete validation shell safety rejects process termination, detached/background processes, and detected workspace mutation. Write detection relies on command-line markers and temporary-target heuristics rather than a runtime-owned validation scratch path. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/state-changing/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` |
| extractCommand | `sed -n '60,110p' internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` |
| extractedSummary | When `stateChangingOutputRequired` is true, runtime accepts valid state-changing graph/output, incremental artifact phase output, or structured decision/approval blocker. A failed `dodAssessment` without executable output is rejected as `repair_no_state_changing_output`. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/patch-granularity/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_patch_contract.go` |
| extractCommand | `sed -n '45,112p' internal/modules/kernel/orchestrator/biz/runtime_patch_contract.go` |
| extractedSummary | Existing-target `file_write` is allowed only when the current repair context item exists, is complete, safe for file write, not redacted/truncated/binary, has contentHash, and the patch request carries matching baseHash without redaction/template markers. Otherwise the contract requires exact patch forms or a real blocker. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/current-snapshot/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go` |
| extractCommand | `sed -n '500,512p' internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go` |
| extractedSummary | `applyCurrentSnapshotDirectPatchContract` sets `mustUseCurrentTargetSnapshot`, `mustPatchDirectlyFromProvidedSnapshot`, `sameGraphReadPatchRule`, `patchApplyFieldRule`, and `readBeforePatchRecoveryRule`. It tells the model to patch from supplied current snapshots rather than read before patch in the same graph. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/prompt-controlled-substitute/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/blocks/llm/biz/http_provider.go` |
| extractCommand | `sed -n '1132,1140p' internal/modules/blocks/llm/biz/http_provider.go` |
| extractedSummary | The LLM prompt tells the model that when concrete validation evidence is required, it should try neutral controlled substitutes such as local fixture, stub/mock, temporary config, isolated working directory, or alternate available port; evidence-producing nodes must declare validationEvidence mode and observationType. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/graph-state-changing/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_graph_contract.go` |
| extractCommand | `sed -n '40,155p' internal/modules/kernel/orchestrator/biz/runtime_graph_contract.go` |
| extractedSummary | State-changing repair graphs without `patch/apply` or declared auditable state-changing operations are rejected. Patch/apply must not depend on same-graph `file/read` when current snapshots are supplied. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | false |

### ev:code/contract/violation-feedback/001

| Field | Value |
| --- | --- |
| claimId | `B3-CE-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go` |
| extractCommand | `sed -n '320,386p' internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go` |
| extractedSummary | Current retry hints are rule-based but broad. `state_changing_required_missing` says to satisfy stateChangingOutputRequired without read-only graph; `concrete_validation_contract_missing` says to return bounded concrete validation evidence with metadata. This prepares Batch 4 targeted retry audit. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/violation-taxonomy/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go` |
| extractCommand | `sed -n '1,120p' internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go; sed -n '280,356p' internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go` |
| extractedSummary | Contract violations map to violated rules, recoverability, retryHint, and forbiddenRepeat. State-changing and concrete validation have rule-specific hints, but patch/apply failures such as patch dispatch invalid or graph-depends-on-LLM fall back to `runtime_contract_violation` and generic valid-output guidance. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/recovery-routing/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_repair.go` |
| extractCommand | `sed -n '479,525p' internal/modules/kernel/orchestrator/biz/runtime_repair.go; sed -n '555,581p' internal/modules/kernel/orchestrator/biz/runtime_repair.go; sed -n '651,702p' internal/modules/kernel/orchestrator/biz/runtime_repair.go` |
| extractedSummary | Failed step recovery chooses `contract_violation_retry` while within budget, creates a `contract-retry-*` step through `recoveryDispatchSpec`, publishes retry-requested event, and dispatches via `requestDispatchStep`. Exhaustion routes to contract violation decision. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/recovery-projection/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go` |
| extractCommand | `sed -n '144,163p' internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go; sed -n '940,990p' internal/modules/kernel/orchestrator/biz/runtime_repair_input.go` |
| extractedSummary | Retry input receives `contractViolationRecovery`, `contractViolationRetry=true`, lineage `contract_violation_retry`, and generic repair guidance to use expectedContract and avoid repeating actualOutputSummary. It is applied after repair context projections and before convergence contract application. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/retry-budget/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`, `internal/pkg/workorchruntimepolicy/policy.go` |
| extractCommand | `sed -n '191,276p' internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go; sed -n '1,63p' internal/pkg/workorchruntimepolicy/policy.go` |
| extractedSummary | Contract violation retry budget counts retries for the failed source step, total retries in the run, and repeated signature/rule. Default policy is per-step `2`, per-run `8`, repeated signature `1`. Budget gates retry but does not itself create a narrower correction contract. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/escalation-decision/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_repair.go` |
| extractCommand | `sed -n '722,775p' internal/modules/kernel/orchestrator/biz/runtime_repair.go` |
| extractedSummary | Exhausted contract violation retry creates a decision ticket with recovery policy, rejected result policy, allowed recovery actions, loop state, failed step, failure metadata, attempts, runAttempts, signatureRepeatCount, policy limits, and compact failed input/output, then blocks the run and publishes escalated event. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/events/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/pkg/events/topics.go`, `internal/modules/kernel/orchestrator/biz/runtime_events.go` |
| extractCommand | `sed -n '1,30p' internal/modules/kernel/orchestrator/pkg/events/topics.go; sed -n '491,555p' internal/modules/kernel/orchestrator/biz/runtime_events.go` |
| extractedSummary | Orchestrator defines recorded, retry_requested, and escalated contract violation topics. Event payloads clone violation metadata and add workspaceId, runId, sourceStepId, retryStepId or ticketId, and optional context. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/memory-boundary/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-006` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_memory.go`, `internal/modules/kernel/orchestrator/biz/runtime_memory_integration_test.go` |
| extractCommand | `sed -n '313,320p' internal/modules/kernel/orchestrator/biz/runtime_memory.go; sed -n '158,199p' internal/modules/kernel/orchestrator/biz/runtime_memory_integration_test.go` |
| extractedSummary | Repair lesson memory is skipped for `contractViolationRetry` or `contractViolationRecovery`. Tests reject diagnostic-only contract violation memory writes and verify contract violation retry does not create repair lesson memory. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/retry/projection-payload/001

| Field | Value |
| --- | --- |
| claimId | `B4-RC-007` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelprojection/event_payload_contract_violation_allowed_keys.go` |
| extractCommand | `sed -n '1,40p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/event_payload_contract_violation_allowed_keys.go` |
| extractedSummary | Projection allowlist for contract violation events includes attempt, maxAttempts, recoverability, rejectedOutputRef, retryHint, retryStepId, signatureRepeatCount, sourceStepId, ticketId, and violatedRule, but no correction profile or correction mode. |
| owner | projection |
| closureBatch | 4 |
| loadByDefault | false |

### ev:code/context/prompt-partition/001

| Field | Value |
| --- | --- |
| claimId | `B5-CTX-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/blocks/llm/biz/prompt_messages.go` |
| extractCommand | `sed -n '150,185p' internal/modules/blocks/llm/biz/prompt_messages.go; sed -n '186,296p' internal/modules/blocks/llm/biz/prompt_messages.go` |
| extractedSummary | Prompt construction appends C0 request/language, C1 Goal/DoD, C2 capability/governance, C3 workspace/stage/repair package, C4 execution signals/current-action, and C5 trace/budget. Prompt metadata marks C0 stable, C1 workspace_definition, C2 capability_contract, C3 stage_bundle, C4 volatile_tail, and C5 large_refs. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | false |

### ev:code/context/current-action/001

| Field | Value |
| --- | --- |
| claimId | `B5-CTX-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/blocks/llm/biz/prompt_messages.go` |
| extractCommand | `sed -n '2074,2180p' internal/modules/blocks/llm/biz/prompt_messages.go; sed -n '2237,2295p' internal/modules/blocks/llm/biz/prompt_messages.go` |
| extractedSummary | Current-action handles assessment-only, concrete validation, and state-changing requests. State-changing action tells the model to use repairActionPackage accepted kinds, avoid read-only graphs, use existing snapshots instead of file/read, keep patches complete and bounded, and return patch/apply when required. It does not expose a concise `availableTargetSnapshots` field or classify missing-snapshot claims. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | false |

### ev:code/context/repair-snapshots/001

| Field | Value |
| --- | --- |
| claimId | `B5-CTX-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go` |
| extractCommand | `sed -n '480,625p' internal/modules/kernel/orchestrator/biz/runtime_repair_input.go; sed -n '648,682p' internal/modules/kernel/orchestrator/biz/runtime_repair_input.go; sed -n '75,105p' internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go` |
| extractedSummary | Repair input projects `failureTargetRepairContext`, `patchRepairContext`, guidance, `repairActionPackage`, and target snapshots. When implementation target evidence exists, requiredOutput is updated to use current snapshots and return patch/apply directly. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | false |

### ev:code/context/snapshot-compaction/001

| Field | Value |
| --- | --- |
| claimId | `B5-CTX-003` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/blocks/llm/biz/prompt_messages.go`, `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go` |
| extractCommand | `sed -n '1240,1325p' internal/modules/blocks/llm/biz/prompt_messages.go; sed -n '820,864p' internal/modules/kernel/orchestrator/biz/runtime_llm_context.go` |
| extractedSummary | Prompt compaction preserves snapshot metadata and marks truncated content incomplete/unsafe with prompt extraction required. Runtime context also compacts target snapshots from targetSnapshots, failureTargetRepairContext, and patchRepairContext into bounded lineage summaries. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | false |

### ev:code/context/cache-plan/001

| Field | Value |
| --- | --- |
| claimId | `B5-CTX-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code |
| sourcePath | `internal/modules/blocks/llm/biz/provider_cache_plan.go` |
| extractCommand | `sed -n '13,105p' internal/modules/blocks/llm/biz/provider_cache_plan.go; sed -n '128,165p' internal/modules/blocks/llm/biz/provider_cache_plan.go` |
| extractedSummary | Provider cache plan hashes sections by C0/C1/C2/C3/C4/C5 groups, builds a provider-neutral cache key from stable/workspace/capability hashes, marks C4 sections as volatile breakers, preserves prompt section order, and exposes adapter hints without provider-private core fields. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | false |

### ev:ws7c/run4/ctr-statechange-005/request-prompt/001

| Field | Value |
| --- | --- |
| claimId | `B5-CTX-002` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001` |
| stepId | `contract-retry-001-of-planning-001-5` |
| evidenceKind | request |
| sourcePath | `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/run-001-replan-001-replan-001-replan-001/llm-calls/run-001-replan-001-replan-001-replan-001/006-contract-retry-001-of-planning-001-5/request.json` |
| extractCommand | `jq '{inputKeys:(.input\|keys), promptSections:(.promptContext.sections//[]\|map({id,cacheability,protected,bytes:(.content\|tostring\|length)})[:24]), contextBudget:.contextBudget.cachePlan}' request.json; jq -r '.promptContext.sections[]? \| select(.id=="C4.current-action" or .id=="C3.repair-action-package" or .id=="C3.stage-contract") \| .content[0:3000]' request.json` |
| extractedSummary | Request had promptContext sections C0-C5, cache plan hashes, C3 stage/repair package, large C4 current-action/current-progress, and complete snapshots for `main.go` and `main_test.go`. Current-action is large and nested; no concise top-level `availableTargetSnapshots` summary is visible. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | true |

### ev:service/readyz/001

| Field | Value |
| --- | --- |
| claimId | `C-SERVICE-PROJECTION-READY-001` |
| workspaceId | all |
| runId | n/a |
| evidenceKind | service |
| sourcePath | `GET http://127.0.0.1:8080/readyz` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/readyz \| jq '{status,projection:.projection, watchdog:.watchdog.latestDurationMs}'` |
| extractedSummary | Service returned `status=ready`; projection had `sourceEventRevision=9281`, `projectedSourceRevision=9281`, `workspaceTotal=2`. |
| owner | projection |
| closureBatch | 1 |
| loadByDefault | false |

### ev:service/process/001

| Field | Value |
| --- | --- |
| claimId | `C-SERVICE-RESOURCE-001` |
| workspaceId | all |
| runId | n/a |
| evidenceKind | process |
| sourcePath | local process table |
| extractCommand | `ps -p $(pgrep -f './bin/workorch' \| head -n1) -o pid,pcpu,pmem,rss,etime,cmd` |
| extractedSummary | `PID 1846519`, about `7.9%` CPU, `RSS 387936 KiB`, command `./bin/workorch`. |
| owner | external |
| closureBatch | 7 |
| loadByDefault | false |

### ev:ws4e/snapshot/current/001

| Field | Value |
| --- | --- |
| claimId | `C-WS4E-STATE-001` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | `run-001-replan-001-replan-001-replan-001` |
| evidenceKind | snapshot |
| sourcePath | `GET /api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot` |
| extractCommand | `curl -fsS "$url" \| jq '{projection, workspace:.snapshot.workspace, run:.snapshot.run, step:{id:.snapshot.step.id,kind:.snapshot.step.kind,state:.snapshot.step.state}, coverageSummary:.snapshot.coverageSummary, taskProgress:.snapshot.taskProgress}'` |
| extractedSummary | Workspace `waiting_decision`; run `blocked_on_decision`; current step failed; blocking ticket present; coverage summary `total=40`, `satisfied=6`, `evidence_ready=6`, `failed=23`, `open=5`; current focus `cwi-012`. |
| owner | projection |
| closureBatch | 1 |
| loadByDefault | false |

### ev:ws4e/events/all-runs/001

| Field | Value |
| --- | --- |
| claimId | `C-WS4E-EVENT-TIMELINE-001` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | all four runs |
| evidenceKind | event |
| sourcePath | `.workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/*/events/events-000001.jsonl` |
| extractCommand | `jq -r '.record.Topic' "$f" \| sort \| uniq -c \| sort -nr; jq -r '.record.OccurredAt' "$f" \| awk 'NR==1{first=$0}{last=$0}END{print first,last,NR}'` |
| extractedSummary | Run event counts: `224`, `398`, `241`, `235`. Latest run contains `170` llm stream progress, `17` dispatched steps, `3` contract violations, `2` retry requests, and `1` ticket creation. |
| owner | core |
| closureBatch | 1 |
| loadByDefault | false |

### ev:ws4e/steps/summary/001

| Field | Value |
| --- | --- |
| claimId | `C-WS4E-STEP-SUMMARY-001` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | all four runs |
| evidenceKind | output |
| sourcePath | `.workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/*/steps/*/*/output.json` |
| extractCommand | `find "$dir" -mindepth 2 -maxdepth 2 -name output.json \| sort \| while read f; do jq -r '[step,status,failureClass,contractViolationType,error,summary,nodeCount,dodItems] \| @tsv' "$f"; done` |
| extractedSummary | Latest run ended with repeated `validation_evidence_required` failures on controlled-substitute validation graph outputs. Earlier runs include completed patches, validation probes, and accepted results. |
| owner | core |
| closureBatch | 2 |
| loadByDefault | false |

### ev:ws4e/run4/ctr-validation-003/output/001

| Field | Value |
| --- | --- |
| claimId | `C-WS4E-CONTROLLED-SUBSTITUTE-001` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | `run-001-replan-001-replan-001-replan-001` |
| stepId | `contract-retry-001-of-graph-003-001-validate-focus-cwi-009-operability-s-probe-operabi` |
| evidenceKind | output |
| sourcePath | `.workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-001-replan-001-replan-001-replan-001/steps/run-001-replan-001-replan-001-replan-001/017-contract-retry-001-of-graph-003-001-validate-focus-cwi-009-operability-s-probe-operabi/output.json` |
| extractCommand | `jq '{status,failureClass,stage,contractViolationType,contractViolationRule,error,summary,graphNodes:((.executionGraph.nodes // .graph.nodes // [])\|map({id,kind:(.kind // .type)})), shellNodes:...}' output.json` |
| extractedSummary | `status=failed`, `failureClass=runtime_failed`, `stage=llm_execution_graph_plan`, violation `validation_evidence_required` / `concrete_validation_contract_missing`. Output graph has 11 nodes. The shell node `prepare_multilang_definition_fixtures` declares `validationEvidence.mode=controlled_substitute`, `observationType=controlled_substitute_observation`, and `/tmp/workorch-cwi-012-*` substitute fixtures, but runtime rejected it as shell mutation. |
| owner | core |
| closureBatch | 3 |
| loadByDefault | true |

### ev:ws4e/artifact/git/001

| Field | Value |
| --- | --- |
| claimId | `C-WS4E-ARTIFACT-001` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | all |
| evidenceKind | git |
| sourcePath | `.workorch-workspaces/ws-4e83810542ac4d65` |
| extractCommand | `git -C .workorch-workspaces/ws-4e83810542ac4d65 status --short; git -C ... log --oneline -n 5; find ... -path '*/.git' -prune -o -type f -printf '%P\\t%s\\t%TY-%Tm-%Td %TH:%TM:%TS\\n'` |
| extractedSummary | Commits `421ec47 Expose runtime status probe surface` and `27b9622 Add executable unsaved sync validation contract`; files include `cmd/lsp-bridge/main.go` 9157 bytes and `lsp-bridge` 2636836 bytes; untracked `go.mod` and `lsp-bridge`. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:ws7c/snapshot/current/001

| Field | Value |
| --- | --- |
| claimId | `C-WS7C-STATE-001` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001` |
| evidenceKind | snapshot |
| sourcePath | `GET /api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot` |
| extractCommand | `curl -fsS "$url" \| jq '{projection, workspace:.snapshot.workspace, run:.snapshot.run, step:{id:.snapshot.step.id,kind:.snapshot.step.kind,state:.snapshot.step.state}, coverageSummary:.snapshot.coverageSummary, taskProgress:.snapshot.taskProgress}'` |
| extractedSummary | Workspace `waiting_decision`; run `blocked_on_decision`; blocking ticket present; current focus around provider route/fallback; coverage summary `total=89`, `satisfied=2`, `in_progress=1`, `failed=26`, `open=60`. |
| owner | projection |
| closureBatch | 1 |
| loadByDefault | false |

### ev:ws7c/events/all-runs/001

| Field | Value |
| --- | --- |
| claimId | `C-WS7C-EVENT-TIMELINE-001` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | all four runs |
| evidenceKind | event |
| sourcePath | `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/*/events/events-000001.jsonl` |
| extractCommand | `jq -r '.record.Topic' "$f" \| sort \| uniq -c \| sort -nr; jq -r '.record.OccurredAt' "$f" \| awk 'NR==1{first=$0}{last=$0}END{print first,last,NR}'` |
| extractedSummary | Run event counts: `252`, `42`, `98`, `181`. Latest run contains `133` llm stream progress, `6` contract violations, `5` retry requests, and `1` ticket creation. |
| owner | core |
| closureBatch | 1 |
| loadByDefault | false |

### ev:ws7c/steps/summary/001

| Field | Value |
| --- | --- |
| claimId | `C-WS7C-STEP-SUMMARY-001` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | all four runs |
| evidenceKind | output |
| sourcePath | `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/*/steps/*/*/output.json` |
| extractCommand | `find "$dir" -mindepth 2 -maxdepth 2 -name output.json \| sort \| while read f; do jq -r '[step,status,failureClass,contractViolationType,error,summary,nodeCount,dodItems] \| @tsv' "$f"; done` |
| extractedSummary | Early runs created source files and tests, then focused validation failed. Later retries hit `coverage_targets_required`, `patch_dispatch_invalid`, and finally repeated `state_changing_required_missing`. |
| owner | core |
| closureBatch | 2 |
| loadByDefault | false |

### ev:ws7c/run4/ctr-statechange-005/output/001

| Field | Value |
| --- | --- |
| claimId | `C-WS7C-STATECHANGE-SNAPSHOT-001` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001` |
| stepId | `contract-retry-001-of-planning-001-5` |
| evidenceKind | output |
| sourcePath | `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/run-001-replan-001-replan-001-replan-001/steps/run-001-replan-001-replan-001-replan-001/006-contract-retry-001-of-planning-001-5/output.json` |
| extractCommand | `jq '{status,failureClass,stage,contractViolationType,contractViolationRule,error,summary,graphNodes:((.executionGraph.nodes // .graph.nodes // [])\|length), dodItems:((.dodAssessment.items // [])\|length)}' output.json` |
| extractedSummary | `status=failed`, `failureClass=dod_coverage_missing`, `stage=llm_dod_assessment`, violation `repair_no_state_changing_output` / `state_changing_required_missing`. Output has no graph nodes and 3 DoD items. Summary claims missing current target snapshots and diagnostics. |
| owner | core |
| closureBatch | 4 |
| loadByDefault | true |

### ev:ws7c/run4/ctr-statechange-005/request/001

| Field | Value |
| --- | --- |
| claimId | `C-WS7C-STATECHANGE-SNAPSHOT-001` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001` |
| stepId | `contract-retry-001-of-planning-001-5` |
| evidenceKind | request |
| sourcePath | `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/run-001-replan-001-replan-001-replan-001/llm-calls/run-001-replan-001-replan-001-replan-001/006-contract-retry-001-of-planning-001-5/request.json` |
| extractCommand | `jq -r '.. \| objects \| select((.targetRef? // \"\") == \"main_test.go\" or (.targetRef? // \"\") == \"main.go\") \| {targetRef,exists,contentComplete,contentHash,size,source,safeForFileWrite,preview:(.content // .text // .preview // \"\" \| tostring \| .[0:80])} \| @json' request.json` |
| extractedSummary | Request size `1269409` bytes. Complete snapshots exist: `main.go` hash `4f09b6...`, size `2162`, source `patch_repair_context`, `safeForFileWrite=true`; `main_test.go` hash `f96c76...`, size `3844`, sources `failure_target_current_workspace_file` and `current_workspace_file`, `safeForFileWrite=true`. String counts: `main_test.go=295`, `main.go=69`. |
| owner | core |
| closureBatch | 5 |
| loadByDefault | true |

### ev:ws7c/artifact/git/001

| Field | Value |
| --- | --- |
| claimId | `C-WS7C-ARTIFACT-001` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | all |
| evidenceKind | git |
| sourcePath | `.workorch-workspaces/ws-7ced70e98ea50c43` |
| extractCommand | `git -C .workorch-workspaces/ws-7ced70e98ea50c43 status --short; git -C ... log --oneline -n 5; find ... -path '*/.git' -prune -o -type f -printf '%P\\t%s\\t%TY-%Tm-%Td %TH:%TM:%TS\\n'` |
| extractedSummary | No commits. Untracked files: `go.mod` 27 bytes, `main.go` 2162 bytes, `main_test.go` 3844 bytes. |
| owner | core |
| closureBatch | 6 |
| loadByDefault | false |

### ev:verify/managed-ci-safe/20260622-001

| Field | Value |
| --- | --- |
| claimId | `C-B7-MANAGED-CI-SAFE-20260622-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/tmp.CwFeuHCFwK` |
| extractCommand | `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh` |
| extractedSummary | Local managed verifier completed at `2026-06-22 14:20:51 CST`: `make fmt-check`, console build, `go vet ./...`, targeted `go test` for LLM and orchestrator packages, `go build ./application/workorch`, managed service startup on `http://127.0.0.1:34370`, `verify-workorch-http`, workspace template/Soul e2e, mock LLM workspace execution, and MCP stdio workspace execution all passed. Managed artifacts were retained at `/tmp/tmp.CwFeuHCFwK`. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-001

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-CONVERGENCE-20260622-001` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001-replan-001` |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/workorch-b7-live-convergence-20260622142251` |
| extractCommand | `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-convergence-20260622142251 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=8 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=5 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh` |
| extractedSummary | Current deployed `http://127.0.0.1:8080` was ready with projection state `ready` and two workspaces. Goal convergence failed for both workspaces: both runs were `blocked_on_decision`, each had one open decision, ineffective retry was true, latest result was rejected, and coverage still had a validation evidence gap. Process resource envelope during the 15.15s observation was bounded: CPU `1.32%`, RSS `469172224`, read delta `0`, write delta `4096`. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-002

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-CONVERGENCE-20260622-002` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001-replan-001-replan-001` |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/workorch-b7-progress2-latest-20260622143111` |
| extractCommand | `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-progress2-latest-20260622143111 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=12 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=5 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=0 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh` |
| extractedSummary | After resolving both deployed open decisions with `resolutionType=replan`, both workspaces entered a fresh running run and produced stream progress, then returned to `blocked_on_decision`. `ws-4e...` latest rejected result: evidence acquisition node `prepare_multilang_definition_scratch` still wrote deliverable files instead of using scratch. `ws-7c...` latest rejected result: patch target `cwi_005_provider_prefix_test.go` was rejected as unrelated to failure target scope `[main_test.go]` even though `coverageFocus` pointed to `cwi-005`. Resource envelope stayed bounded: CPU `1.32%`, RSS `156127232`, read delta `0`, write delta `0`. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/context-contract/20260622-001

| Field | Value |
| --- | --- |
| claimId | `C-B7-CONTEXT-CONTRACT-20260622-001` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh; git diff --check -- internal/modules/kernel/orchestrator/biz/execution_context_package.go internal/modules/kernel/orchestrator/biz/runtime_capability_contract.go internal/modules/kernel/orchestrator/biz/runtime_patch_requests.go internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/execution_context_package_test.go internal/modules/kernel/orchestrator/biz/runtime_contract_optimization_test.go internal/modules/kernel/orchestrator/biz/runtime_patch_integration_test.go` |
| extractedSummary | Local code closure after `ev:verify/live-convergence/20260622-002`: orchestrator/biz package tests passed, boundary contract verification passed, and diff whitespace check passed. Covered regressions include current run MCP capability context merged into `selectedContext`, undeclared MCP server IDs rejected by runtime capability contract, structurally declared non-substantive patch requests rejected without natural-language placeholder/deferral parsing in core, and duplicate coverage target entries deduped by normalized requirement identity. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/managed-ci-safe/20260622-002

| Field | Value |
| --- | --- |
| claimId | `C-B7-MANAGED-CI-SAFE-20260622-002` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/tmp.WoVjdTAXve` |
| extractCommand | `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh` |
| extractedSummary | Local managed verifier reran after `ev:code/context-contract/20260622-001`: `make fmt-check`, console build, `go vet ./...`, targeted `go test` for LLM and orchestrator packages, `go build ./application/workorch`, managed service startup on `http://127.0.0.1:34552`, `verify-workorch-http`, workspace template/Soul e2e, mock LLM workspace execution, and MCP stdio workspace execution all passed. The MCP stdio workspace completed run `run-001` with `steps=3`, `toolCalls=3`, `validations=3`, and `runtimeCheckpoints=26`. Managed artifacts were retained at `/tmp/tmp.WoVjdTAXve`. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-003

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-CONVERGENCE-20260622-003` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-001-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-001`, `run-002` |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/workorch-b7-live-continuation-20260622154108` |
| extractCommand | `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-continuation-20260622154108 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=75 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=8 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=1 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=0 ./scripts/observe-workorch-runtime-closure.sh` |
| extractedSummary | Deployed `http://127.0.0.1:8080` remained ready with two running workspaces and projection ready. Both active runs had no open decisions or pending approvals. SSE delivered `/run/llm_stream/progress` and projection-change events for both workspaces. The latest active steps were LLM provider calls waiting on stream progress, not EventHub dispatch loss or TUI projection staleness. `stalledRuns=0`, runtime publish failure deltas were `0`, pprof found no synchronous runtime send stack, CPU averaged `5.23%`, RSS was `426364928`, read delta `0`, and write delta `65536`. Goal convergence was not required for this observation and still remained false because both runs were running and coverage gaps were open. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/profile-evidence-state-change/20260622-004

| Field | Value |
| --- | --- |
| claimId | `C-B7-PROFILE-EVIDENCE-STATE-CHANGE-20260622-004` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `go test ./internal/modules/kernel/orchestrator/biz -run 'TestBuildConvergenceContractDoesNotReopenPrimaryStateChangeWhenOnlyValidationMissing\|TestBuildConvergenceContractUsesProfileRequiredStateChangingEvidenceForCoverageTarget\|TestValidateLLMExecutionOutputContractAllowsAcceptedGitWriteStateChangingGraph\|TestValidateLLMExecutionOutputContractAllowsDeclaredStateChangingShellGraph\|TestValidateLLMExecutionOutputContractRejectsUndeclaredStateChangingShellGraph' -count=1 -timeout=240s; go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s` |
| extractedSummary | Local closure for the live `state_changing_required_missing` regression exposed after `ev:verify/live-convergence/20260622-003`: coverage targets that explicitly require `state_change` but whose structured `missingEvidenceKinds` no longer include `state_change` are no longer treated as needing another primary state-changing output. Mixed coverage targets now derive the current supplemental profile evidence contract, including `git/write` + `commit`, without reintroducing unrelated primary `patch/apply`; execution contract validation accepts `git/write` only when `requiredOutput.acceptedKinds` and `acceptedOperations` explicitly allow it. Targeted tests and full `orchestrator/biz` package passed. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/shell-bash-feature/20260622-005

| Field | Value |
| --- | --- |
| claimId | `C-B7-SHELL-BASH-FEATURE-20260622-005` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `go test ./internal/modules/blocks/shell/biz -run 'TestRuntimeAdapter_ExecuteRuntimeStepNormalizesShPipeStatusScriptToBash|TestRuntimeAdapter_ExecuteRuntimeStepNormalizesShPipefailScriptToBash' -count=1; go test ./internal/modules/blocks/shell/biz -count=1; ./scripts/verify-boundary-contract.sh` |
| extractedSummary | Local shell block closure for validation scripts that request `sh -c` but use bash-only syntax. The shell runtime adapter now normalizes scripts using `${PIPESTATUS[...]}` to bash, preserving the existing pipefail normalization and keeping command policy validation on the actual executable. This is a generic shell feature compatibility rule, not a language, test framework, or workspace-template special case. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/managed-ci-safe/20260622-006

| Field | Value |
| --- | --- |
| claimId | `C-B7-MANAGED-CI-SAFE-20260622-006` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/tmp.vQ618dTrHL` |
| extractCommand | `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/blocks/shell/biz ./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh` |
| extractedSummary | Managed verifier reran for the current local closure packet: `make fmt-check`, console build, `go vet ./...`, targeted Go tests for shell, LLM, and orchestrator packages, `go build ./application/workorch`, managed service startup on `http://127.0.0.1:35188`, HTTP verification, workspace template/Soul e2e, mock LLM workspace execution, and MCP stdio workspace execution all passed. MCP stdio completed run `run-001` with `steps=3`, `toolCalls=3`, `validations=3`, and `runtimeCheckpoints=26`. Managed artifacts were retained at `/tmp/tmp.vQ618dTrHL`. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-007

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-CONVERGENCE-20260622-007` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-002` |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/workorch-b7-live-post-managed-20260622163431` |
| extractCommand | `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-post-managed-20260622163431 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=45 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=8 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=1 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh` |
| extractedSummary | Post-managed-verification live observation against `http://127.0.0.1:8080` failed the required goal-convergence gate because both observed workspaces were still `blocked_on_decision` with one open decision each, `ineffectiveRetry=true`, latest rejected results, and open coverage gaps requiring `validation,state_change`. Projection and display contract were ready, pprof found no synchronous runtime send stack, event publish failure deltas were zero, IO deltas were bounded, CPU averaged `1.42%`, RSS was `350994432`, and no SSE events were captured because both runs were waiting on decisions. This is stale-state/live-governance evidence, not proof of a new code defect in the current local packet; a fresh explicit replan/resume is required before the next positive convergence observation. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-008

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-CONVERGENCE-20260622-008` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001` |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/workorch-b7-live-after-replan-20260622163954` |
| extractCommand | `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-after-replan-20260622163954 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=90 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=8 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=1 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh` |
| extractedSummary | After explicitly resolving both open decisions with `resolutionType=replan`, `ws-4e...` was running with active `/run/llm_stream/progress`, no open decisions, projection ready, and coverage still missing validation. `ws-7c...` entered `waiting_decision` on `run-002-replan-001` after a new validation failure: `main_test.go:113:17: assignment mismatch: 2 variables but loadConfig returns 1 value`. The new decision context showed the replan successor inherited an already escalated convergence budget (`repairAttempts 4/4`, validation/no-delta counters exhausted), so the first post-replan validation failure immediately opened a repair-exhausted decision. pprof found no synchronous runtime send stack, runtime publish failure deltas were zero, CPU averaged `49.74%`, RSS was `523026432`, read delta `16384`, and write delta `23568384`; event-record IO deltas were high but not threshold-gated in this run. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/replan-budget-reset/20260622-009

| Field | Value |
| --- | --- |
| claimId | `C-B7-REPLAN-BUDGET-RESET-20260622-009` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `go test ./internal/modules/kernel/orchestrator/biz -run 'TestResetConvergenceBudgetForReplanSuccessorClearsRuntimeCounters|TestRuntimeOrchestrator_RunningResumeReplansFromResolvedDecisionOnFailedNonTerminalStep|TestRuntimeOrchestrator_BlockerAutoResumePolicyMatrix|TestUpdateConvergenceBudget' -count=1` |
| extractedSummary | Local core closure for the post-replan budget inheritance defect: replan successors now reset runtime convergence counters and escalated decisions while preserving budget limits and focus/slice scope. Targeted orchestrator tests passed, including a regression that proves `repairAttempts`, `llmRounds`, validation/no-delta counters, repeated-failure signatures, and previous decision reasons are removed for a replan successor. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/managed-ci-safe/20260622-010

| Field | Value |
| --- | --- |
| claimId | `C-B7-MANAGED-CI-SAFE-20260622-010` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/tmp.Ynb8y5h8uD` |
| extractCommand | `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/blocks/shell/biz ./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh` |
| extractedSummary | Managed verifier reran after the local replan budget reset closure: `make fmt-check`, console build, `go vet ./...`, targeted Go tests for shell, LLM, and orchestrator packages, `go build ./application/workorch`, managed service startup on `http://127.0.0.1:32668`, HTTP verification, workspace template/Soul e2e, mock LLM workspace execution, and MCP stdio workspace execution all passed. MCP stdio completed `run-001` with `steps=3`, `toolCalls=3`, `validations=3`, and `runtimeCheckpoints=26`. Managed artifacts were retained at `/tmp/tmp.Ynb8y5h8uD`. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-011

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-CONVERGENCE-20260622-011` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001` |
| evidenceKind | verifier-output |
| sourcePath | `/tmp/workorch-b7-live-after-redeploy-20260622170203` |
| extractCommand | `WORKORCH_OBSERVE_BASE_URL=http://127.0.0.1:8080 WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-b7-live-after-redeploy-20260622170203 WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 WORKORCH_OBSERVE_DURATION_SECONDS=90 WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=8 WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 WORKORCH_OBSERVE_REQUIRE_PPROF=1 WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=0 WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK=0 WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1 ./scripts/observe-workorch-runtime-closure.sh` |
| extractedSummary | After redeploy and explicit `replan` resolution of both open decisions, both workspaces created fresh `run-002-replan-001-replan-001` successors. `ws-4e...` returned to `waiting_decision` after shell validation failed because the command attempted to `cd` into the structured runtime `validationScratchDir` before that directory existed. The open decision context contained `repairActionPackage.requiredOutput.validationScratchDir=/tmp/workorch-validation-scratch/ws-4e83810542ac4d65/run/run-002-replan-001-replan-001_step-1`. `ws-7c...` remained running with `/run/llm_stream/progress`, patch events, and no open decision. Goal convergence failed for both because coverage still required validation evidence. Projection/display contract passed, pprof found no synchronous runtime send stack, runtime publish failure deltas were zero, CPU averaged `166.51%`, RSS was `656510976`, write delta was `129544192`, and event-record hydrate deltas were high without configured thresholds. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/runtime-validation-scratch-prepare/20260622-012

| Field | Value |
| --- | --- |
| claimId | `C-B7-RUNTIME-VALIDATION-SCRATCH-PREPARE-20260622-012` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go internal/modules/kernel/orchestrator/biz/runtime_validation_diagnostic_test.go internal/modules/kernel/orchestrator/biz/runtime_mainline_integration_test.go; go test ./internal/modules/kernel/orchestrator/biz -run 'TestEnsureRuntimeValidationScratchDirCreatesOnlyControlledRoot|TestRuntimeOrchestrator_DispatchPreparesValidationScratchBeforeRuntimeRequest|TestRuntimeOrchestrator_DispatchStepRequestsRuntimeExecutionWithoutWaitingForResult|TestResetConvergenceBudgetForReplanSuccessorClearsRuntimeCounters' -count=1 -timeout=240s; go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh` |
| extractedSummary | Local core closure for the runtime validation scratch preparation defect exposed by `ev:verify/live-convergence/20260622-011`: dispatch now prepares a structured `validationScratchDir` before sending the runtime request, including request-only/event-driven runtime block paths. The helper only creates directories under the runtime-owned `os.TempDir()/workorch-validation-scratch` root and rejects sibling or arbitrary absolute paths without parsing shell command text. Focused tests, the full orchestrator/biz package, and boundary verification passed. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-convergence/20260622-013

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-RUN-LIFECYCLE-OVERWRITE-20260622-013` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001` |
| evidenceKind | live-api-and-owner-state-extract |
| sourcePath | `/tmp/workorch-ws4-latest-snapshot.json`, `/tmp/workorch-ws7-latest-snapshot.json` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/readyz; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; jq '.[runId] | {State,BlockingTicketID}' .workorch-data/internal/workspaces/<workspace>/runs/<run>/run.json` |
| extractedSummary | After redeploy, projection readiness was healthy (`sourceEventRevision=projectedSourceRevision=6527`). `ws-7c...` correctly persisted `run.state=blocked_on_decision` with its open contract-violation decision. `ws-4e...` had an open repair-exhausted decision and `runHeartbeat.activityState=waiting_decision`, and its owner workspace file was `waiting_decision`, but the owner run file was overwritten back to `State=running` with an empty `BlockingTicketID`; the API list therefore still showed `ws-4e...` as running. The live blocker is a core-neutral run lifecycle overwrite by non-lifecycle progress writes during parallel graph execution, not a template or MCP-specific rule. CPU was still elevated during observation (`./bin/workorch` around `82.8%` CPU and `925968` KiB RSS). |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/run-progress-lifecycle-preserve/20260622-014

| Field | Value |
| --- | --- |
| claimId | `C-B7-RUN-PROGRESS-LIFECYCLE-PRESERVE-20260622-014` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_run_state.go internal/modules/kernel/orchestrator/biz/runtime_run_state_subsystem_test.go internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go internal/modules/kernel/orchestrator/biz/runtime_repair.go internal/modules/kernel/orchestrator/biz/runtime_graph.go internal/modules/kernel/orchestrator/biz/runtime_result_recording.go internal/modules/kernel/orchestrator/biz/runtime_artifact_assemble.go internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go; go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestratorPersistRunProgressUpdatePreservesBlockedDecisionLifecycle|TestRuntimeOrchestrator_RuntimeRepairExhaustion|TestRuntimeOrchestrator_ResumeRunningFailedDoDCoverageRepairExhaustionBlocksRun' -count=1 -timeout=120s; go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh; git diff --check` |
| extractedSummary | Local core closure for `ev:verify/live-convergence/20260622-013`: non-lifecycle run progress writes now use a scoped owner read and preserve existing blocked/terminal lifecycle fields before persisting coverage, gap baseline, or reconciler progress. Lifecycle transitions such as start, block, complete, fail, cancel, and supersede still use the state-machine paths. The regression test proves a stale `running` progress snapshot cannot overwrite an owner `blocked_on_decision` run while still saving updated execution context. Focused tests, full orchestrator/biz tests, boundary verification, and diff whitespace check passed. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/startup-governance-blocker-reconcile/20260622-015

| Field | Value |
| --- | --- |
| claimId | `C-B7-STARTUP-GOVERNANCE-BLOCKER-RECONCILE-20260622-015` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | `run-002-replan-001-replan-001-replan-001` |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `curl -fsS http://127.0.0.1:8080/readyz; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; jq '.[runId] | {State,BlockingTicketID,BlockingApprovalID}' .workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-002-replan-001-replan-001-replan-001/run.json; go test ./internal/modules/application/workorchd/biz/appservice -run 'TestRecoverStartupInactiveRunningWorkspaces' -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice/runports ./internal/modules/application/workorchd/service/server -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh` |
| extractedSummary | Latest redeploy still showed historical owner inconsistency for `ws-4e...`: workspace owner was `waiting_decision`, heartbeat was `activityState=waiting_decision`, and exactly one open decision ticket existed for the active run, but run owner remained `State=running` with empty `BlockingTicketID`; API snapshot therefore displayed `running` and did not select the decision detail. Local appservice closure extends startup recovery to reconcile active runs with a unique open decision or pending approval into `blocked_on_decision` or `blocked_on_approval`, preserving workspace scope and avoiding any template/tool/provider special case. Regression tests cover waiting-decision and approval-blocked startup reconciliation. Focused appservice tests, runports/server tests, and boundary verification passed. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-startup-governance-reconcile/20260622-016

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-STARTUP-GOVERNANCE-RECONCILE-20260622-016` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001` |
| evidenceKind | live-api-and-owner-state-extract |
| sourcePath | deployed `http://127.0.0.1:8080` plus local owner files |
| extractCommand | `curl -fsS http://127.0.0.1:8080/readyz; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; jq '.[runId] | {State,BlockingTicketID,BlockingApprovalID}' .workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-002-replan-001-replan-001-replan-001/run.json` |
| extractedSummary | After redeploy, startup reconciliation took effect. `/readyz` reported `projection.state=ready` and `sourceEventRevision=projectedSourceRevision=51`. Workspace list now projects both workspaces as `waiting_decision` with scoped `blockingTicketId`; `ws-4e...` changed from the previous stale `running` API state to `waiting_decision`. Its owner run file is now `State=blocked_on_decision` with `BlockingTicketID=run-002-replan-001-replan-001-replan-001-graph-006-005-validate-lsp-status-repair-exhausted-decision`. Snapshot top-level `decisions` includes the selected open ticket and heartbeat reports `activityState=waiting_decision`. Process resource was bounded during the sample (`./bin/workorch` about `8.4%` CPU and `414644` KiB RSS). This closes the audited startup owner-state inconsistency; positive goal convergence remains a separate Batch 7 live gate because both workspaces are now correctly waiting for decision. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/global-decision-page-scoped-resolution/20260622-017

| Field | Value |
| --- | --- |
| claimId | `C-B7-GLOBAL-DECISION-PAGE-SCOPED-RESOLUTION-20260622-017` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | n/a |
| evidenceKind | live-api-symptom-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus local worktree |
| extractCommand | `curl -sS -i http://127.0.0.1:8080/api/v1/workorch/decisions; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/<workspace>/snapshot; go test ./internal/modules/application/workorchd/biz/appservice -run 'TestListDecisionPageWithBackendScopesResolvedDuplicateTicketIDs|TestListDecisionPageWithBackendProjectsAuditPayloads|TestListDecisionsWithBackendReturnsFilteredTickets' -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/governanceports -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice/... -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/service/server -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh` |
| extractedSummary | After startup reconciliation, workspace snapshots correctly exposed each open decision, but global `GET /api/v1/workorch/decisions` returned HTTP 409. The live data contained historical resolved tickets whose IDs can repeat across workspaces. The global decision page projected resolved tickets by reading resolutions with an unscoped ticket ID and then keyed resolution maps only by `ticketID`, so duplicate ticket IDs could conflict or attach the wrong resolution. Local closure scopes resolution owner reads with each ticket's `WorkspaceID` and keys internal resolution maps by `workspaceID + ticketID` in both governance and read-model paths. Regression tests cover duplicate resolved ticket IDs across workspaces and prove global decision page returns both scoped records with their own resolution payloads. Appservice full package tests, server tests, and boundary verification passed. Deployment verification for the global `/decisions` endpoint remains pending until the next redeploy. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/global-governance-page-scoped-cursor/20260622-018

| Field | Value |
| --- | --- |
| claimId | `C-B7-GLOBAL-GOVERNANCE-PAGE-SCOPED-CURSOR-20260622-018` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | local worktree |
| extractCommand | `curl -sS -i http://127.0.0.1:8080/api/v1/workorch/decisions; stat -Lc 'pid-exe %y' /proc/<workorch-pid>/exe; stat -Lc 'src %y %n' internal/modules/application/workorchd/biz/appservice/governanceports/decision_resolution_reads.go; go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestSelectDecisionPageWindowUsesWorkspaceScopedCursor|TestSelectApprovalPageWindowUsesWorkspaceScopedCursor' -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice -run 'TestListDecisionPageWithBackendScopesResolvedDuplicateTicketIDs|TestListDecisionPageWithBackendProjectsAuditPayloads|TestListApprovalPage|TestListApprovals|TestReadApproval|TestReadDecision' -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice/... -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/service/server -count=1 -timeout=300s` |
| extractedSummary | The current deployed process still returned HTTP 409 for global `/api/v1/workorch/decisions`, and its binary timestamp was older than the scoped-resolution source edits, so live verification still requires rebuilding/restarting the instance. During the same audit, global governance pagination was found to use only bare ticket/approval IDs as cursors. Historical live data already contains duplicate ticket IDs across workspaces, so a bare cursor is not a stable position in a global governance queue. Local closure makes decision and approval page cursors workspace-scoped opaque values while preserving the existing `cursorAfterId` / `nextCursorAfterId` API fields. Regression tests prove paging through duplicate decision and approval IDs from different workspaces does not repeat or skip entries. Focused appservice/readmodelprojection/governance tests, appservice full subpackage tests, and server tests passed. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/workspace-snapshot-llm-usage-fanout/20260622-026

| Field | Value |
| --- | --- |
| claimId | `C-B7-WORKSPACE-SNAPSHOT-LLM-USAGE-FANOUT-20260622-026` |
| workspaceId | n/a |
| runId | n/a |
| evidenceKind | code-and-test-output |
| sourcePath | `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot_loading.go`, `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot_run_derived_loading.go`, `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot_test.go` |
| extractCommand | `go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -run 'TestWorkspaceSnapshot(BuildsSnapshotFromReaders|MergesRunScopedLLMUsageEvents|LoadsRunScopedLLMUsageOnlyForProjectionRun|UsesCachedRunSnapshotForProjectionRun)' -count=1 -timeout=180s` |
| extractedSummary | Workspace snapshot assembly previously loaded bounded workspace-scoped LLM usage and then loaded run-scoped LLM usage for every historical run. That created payload hydration fan-out on high-frequency workspace snapshot/projection paths as run history grew. Local closure keeps workspace-scoped usage and only supplements run-scoped usage for the selected projection run, preserving active-run usage completeness without hydrating every historical run. Regression tests prove no projection run performs only workspace-level event queries, active projection run usage is still merged, and historical runs are not queried for run-scoped LLM usage. |
| owner | projection/readmodel IO |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/coverage-assessment-required-output-correction/20260623-027

| Field | Value |
| --- | --- |
| claimId | `C-B7-COVERAGE-ASSESSMENT-REQUIRED-OUTPUT-CORRECTION-20260623-027` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-003` |
| evidenceKind | code-and-test-output |
| sourcePath | `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`, `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go` |
| extractCommand | `go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolation(CorrectionProfilesCoverPrimaryRules|CoverageAssessmentPreservesCurrentRequiredOutputShape|RecoveryActionRetriesWithinBudget|RuntimeRepairInputIncludesContractViolationRecovery)' -count=1 -timeout=240s; go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; go test ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh; go build -o ./bin/workorch ./application/workorch; git diff --check` |
| extractedSummary | Latest deployed observation showed both live workspaces blocked on `dod_assessment_missing_evidence_refs` while their `expectedContract.repairActionPackage.requiredOutput` still required evidence acquisition or state-changing output. The previous correction profile always narrowed this rule to `traceable_coverage_assessment`, which could steer the next retry toward another assessment instead of the current required output shape. The closure now derives the correction profile from structured `repairActionPackage.requiredOutput`: state-changing requirements keep `state_changing_output`, concrete validation requirements keep `bounded_validation_evidence`, and concrete failure evidence acquisition keeps `bounded_failure_evidence`. Pure assessment-stage coverage violations still use `traceable_coverage_assessment`. Targeted tests, appservice/orchestrator package tests, boundary verification, evidence reference integrity, whitespace check, and `./bin/workorch` build passed. |
| owner | core runtime contract retry |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/patch-basehash-refresh-repair/20260622-021

| Field | Value |
| --- | --- |
| claimId | `C-B7-PATCH-BASEHASH-REFRESH-REPAIR-20260622-021` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus local worktree |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; go test ./internal/modules/kernel/orchestrator/biz -run 'TestUpdateConvergenceBudgetDoesNotCountPatchBaseHashMismatchAsNoDelta|TestRuntimeRepairActionPackageTreatsBaseHashMismatchAsStalePatch|TestRuntimeRepairActionPackageGroundsCurrentPatchFailure' -count=1; go test ./internal/modules/kernel/orchestrator/biz -count=1; ./scripts/verify-boundary-contract.sh; go build -o ./bin/workorch ./application/workorch` |
| extractedSummary | Live `ws-7...` was blocked on a `patch/apply` step with `failureClass=patch_apply_failed` and `error=conflict: patch baseHash mismatch`. The failure came from a stale patch snapshot after another graph node had already changed the same target, but convergence budget treated it as ordinary `noCoverageDeltaRounds` and escalated to decision before the runtime could rebuild the patch from the current target snapshot. Local core closure now identifies only the neutral `patch/apply` / `patch_apply_failed` `baseHash mismatch` shape as a mechanical stale patch conflict: it does not consume coverage no-delta budget, while still tracking failure signatures and repair budgets. The same failure now enters the existing stale patch action contract so the next repair must use current target snapshots, rebuild from current evidence, and must not reuse the rejected patch hunk. Focused regression tests, full orchestrator/biz tests, boundary verification, and build passed. Live redeploy plus explicit replan/resume is still required to verify fresh convergence. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:verify/live-patch-basehash-refresh/20260622-022

| Field | Value |
| --- | --- |
| claimId | `C-B7-LIVE-PATCH-BASEHASH-REFRESH-20260622-022` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001-replan-001-replan-001` |
| evidenceKind | live-api-observation |
| sourcePath | deployed `http://127.0.0.1:8080` |
| extractCommand | `sha256sum /proc/<workorch-pid>/exe ./bin/workorch; curl -fsS http://127.0.0.1:8080/readyz; POST /api/v1/workorch/decisions/<patch-basehash-ticket>/resolve?workspaceId=ws-7ced70e98ea50c43; curl -fsS /api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot` |
| extractedSummary | After rebuilding and restarting the deployed service, the running process SHA matched `./bin/workorch`. Resolving the stale `patch baseHash mismatch` decision with scoped `replan` created successor run `run-002-replan-001-replan-001-replan-001-replan-001-replan-001`, cleared the open decision, and advanced through LLM execution, contract retry, graph dispatch, and state-changing file output. The previous `patch baseHash mismatch` decision did not recur. The successor later blocked on a different validation failure: `shell/command` `go test -run 'TestMetricsEndpoint|TestStatsEndpoint|TestStatsStreamEndpoint' ./...` failed because `main_test.go` had build errors (`"time" imported and not used`, unused `port`, and assignment mismatch with `testNewProxy` returning two values). Runtime projected structured `validation_build_failed` diagnostics, current target snapshot, and a scoped decision after `repairAttempts 5/4`. This verifies the baseHash refresh closure path but does not close live goal convergence. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/patch-application-failure-narrow-retry/20260622-023

| Field | Value |
| --- | --- |
| claimId | `C-B7-PATCH-APPLICATION-FAILURE-NARROW-RETRY-20260622-023` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus local worktree |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; jq . .workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/<run>/patches/<step>/patch/1.json; go test ./internal/modules/kernel/orchestrator/biz -run 'TestApplyRuntimeRepairPatchFailureRecoveryContractNarrowsPatchRetry|TestApplyRuntimeRepairPatchContextProjectionRequiresStateChangingRepair' -count=1; go test ./internal/modules/kernel/orchestrator/biz -count=1; ./scripts/verify-boundary-contract.sh; git diff --check` |
| extractedSummary | After the root-cause shift contract forced repair away from the repeated failed surface, live `ws-7...` advanced to a real `patch/apply` against the root-cause target, but then blocked on `patch_apply_failed` with `conflict: patch text_replace oldText not found`. The runtime already provided current target snapshots, but the subsequent repair contract did not narrowly identify the failed patch literal as stale rejected material. Local core closure adds a neutral patch application failure recovery contract into `repairActionPackage.requiredOutput`: it records compact `failedPatchRequests`, current `patchRepairContext` refs, requires rebuilding from current snapshots/contentHash, forbids retrying stale `oldText`/diff/content/baseHash, and preserves existing state-changing Goal/DoD coverage requirements. The regression test proves a failed `patch/apply` oldText conflict now projects `mustRepairFailedPatchApplication`, `mustUsePatchRepairContext`, `mustNotRetryFailedPatchLiteral`, failed patch summaries, and preferred current repair suggestions. Focused and full orchestrator/biz tests, boundary verification, and whitespace diff check passed. Live redeploy plus explicit replan/resume remains required to prove positive convergence. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/llm-target-snapshot-compaction-safety/20260622-024

| Field | Value |
| --- | --- |
| claimId | `C-B7-LLM-TARGET-SNAPSHOT-COMPACTION-SAFETY-20260622-024` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-002` |
| evidenceKind | live-input-audit-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus local worktree |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; jq '.repairActionPackage.targetSnapshots | map({targetRef,contentHash,contentLen:(.content|length),contentComplete,safeForFileWrite,truncated})' .workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/<run>/steps/<run>/<state-changing-follow-up>/input.json; go test ./internal/modules/kernel/orchestrator/biz -run 'TestGenericLLMContextCompactionMarksTruncatedTargetSnapshotUnsafe|TestCompactStepOutputForLLMContextOmitsRedactedRejectedPatchFragments|TestCompactLLMContextDropsHeavyTransportFields' -count=1; go test ./internal/modules/kernel/orchestrator/biz -count=1; ./scripts/verify-boundary-contract.sh; git diff --check` |
| extractedSummary | Post-replan live audit showed the repair follow-up did receive `repairActionPackage.targetSnapshots`, but a target snapshot whose original file-read output was about 9.9 KiB reached the LLM-visible input with only a 4 KiB `content` string while still declaring `contentComplete=true`, `safeForFileWrite=true`, and `truncated=false`. The model then generated a `text_replace` oldText that was not present in the current target and the run blocked again on `patch_apply_failed`. Local core closure normalizes generic LLM context compaction for structured target snapshots: if exact snapshot content is compacted or redacted, it is downgraded to `contentPreview`, marks `contentComplete=false`, `safeForFileWrite=false`, `truncated=true`, and records omitted byte metadata. Patch request compaction remains separate and is not treated as a target snapshot. Focused tests, full orchestrator/biz tests, boundary verification, and whitespace diff check passed. Live redeploy plus explicit replan/resume remains required to verify positive convergence. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/scoped-root-cause-strategy-reassessment/20260622-025

| Field | Value |
| --- | --- |
| claimId | `C-B7-SCOPED-ROOT-CAUSE-STRATEGY-REASSESSMENT-20260622-025` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-002-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-003` |
| evidenceKind | live-output-audit-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus local worktree |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; jq '.promptContext.sections[] | select(.id=="C3.repair-action-package") | .content' .workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/<run>/llm-calls/<run>/003-contract-retry-001-of-planning-001-2/request.json; jq '.executionGraph.nodes[] | select(.kind=="patch/apply") | .input | {targetRef,patchType,failedSurface,rootCauseTarget,repairMode}' .workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/<run>/steps/<run>/003-contract-retry-001-of-planning-001-2/output.json; go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMExecutionOutputContract(RejectsUndistinguishedStrategyReassessmentRootCause|AllowsScopedRootCauseWithinFailedSurface|RejectsRepeatedFailedSurfacePatchWithoutTargetShift|AllowsRepeatedFailedSurfaceSameTargetWhenPrimaryDiagnosticAddressed|AllowsGroundedRootCauseCandidateShift)' -count=1` |
| extractedSummary | Live audit of `replan-003` showed the provider-facing `C3.repair-action-package` now carries the complete current target snapshot (`contentView=complete`, `contentComplete=true`, `safeForFileWrite=true`), so the previous unsafe compaction issue was not the active blocker. The next blocker was strategy reassessment: two state-changing outputs declared `repairMode=shift_root_cause_target`, used current target content, and changed a concrete sub-scope inside the failed resource, but the core contract rejected them because the patch target still matched the coarse failed-surface resource. Local core closure keeps broad repeated same-surface rejection intact, but recognizes a neutral scoped root-cause inside the failed resource when `rootCauseTarget` names the target plus an additional sub-scope, the patch is substantive, and verification remains bounded. `rootCauseTarget` parsing now treats `resource:scope` / `resource#scope` as the same resource plus sub-scope for consistency checks. Focused regression tests prove undistinguished root causes are still rejected while scoped root-cause repair is accepted. Full redeploy plus fresh replan/resume is still required to prove live positive convergence. |
| owner | core |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/focus-bound-evidence-graph-normalization/20260623-028

| Field | Value |
| --- | --- |
| claimId | `C-B7-FOCUS-BOUND-EVIDENCE-GRAPH-NORMALIZATION-20260623-028` |
| workspaceId | `ws-4e83810542ac4d65`, `ws-7ced70e98ea50c43` |
| runId | `run-003-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract_test.go` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMRepairCoverageFocus' -count=1 -timeout=300s; ./scripts/verify-boundary-contract.sh; git diff --check` |
| extractedSummary | Post-redeploy live snapshots showed both workspaces blocked on `repair_focus_mismatch` after replan successors reached `repair/strict_convergence` with `repairObjective=acquire_concrete_failure_evidence`. The returned graphs contained bounded evidence-acquisition actions (`shell/command` or `mcp/tool_call`) but did not explicitly repeat the current `coverageFocusId` / `coverageWorkItemIds`, so the runtime consumed retry budget and escalated to decision before dispatching otherwise focus-scoped evidence acquisition. Local core closure now normalizes only non-`patch/apply` evidence graph nodes when the structured repair contract already contains a unique current coverage focus and state-changing output is not required. `patch/apply` remains strictly rejected unless it explicitly binds focus. Focused tests prove evidence nodes receive injected focus metadata and unbound patch graphs still fail. |
| owner | core runtime contract normalization |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/convergence-budget-graph-pending-and-stale-patch/20260623-029

| Field | Value |
| --- | --- |
| claimId | `C-B7-CONVERGENCE-BUDGET-GRAPH-PENDING-STALE-PATCH-20260623-029` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | `run-003-replan-001-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`, `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget_test.go` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; jq '{graphNodeId:.graphNodeId,repairObjective:.repairObjective,budget:.acceptance.budget}' .workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-003-replan-001-replan-001/steps/run-003-replan-001-replan-001/*/input.json; go test ./internal/modules/kernel/orchestrator/biz -run 'TestUpdateConvergenceBudget|TestConvergenceBudget' -count=1 -timeout=300s` |
| extractedSummary | Live successor run started with a clean tracking budget, then a completed LLM step that only returned an executionGraph and a later completed repair-planning step consumed `noCoverageDeltaRounds` before their graph nodes could finish. The next `patch/apply` stale `oldText not found` conflict was then escalated to decision as `noCoverageDeltaRounds 5/3` instead of entering the existing patch-rebuild recovery path. Local core closure keeps LLM rounds counted, but no longer counts completed LLM steps that only return a pending executionGraph as no-coverage-delta. It also treats neutral stale `patch/apply` `oldText not found` conflicts like baseHash mismatch for no-delta accounting: failure signatures are still tracked, but the no-delta budget is not consumed by a mechanical stale patch conflict. |
| owner | core runtime convergence budget |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/contract-correction-governance-blocker-shape/20260623-030

| Field | Value |
| --- | --- |
| claimId | `C-B7-CONTRACT-CORRECTION-GOVERNANCE-BLOCKER-SHAPE-20260623-030` |
| workspaceId | `ws-7ced70e98ea50c43` |
| runId | `run-003-replan-001-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`, `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-7ced70e98ea50c43/snapshot; go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolation(ConcreteValidationForbidsContinuableDecisionBlocker|CorrectionProfilesCoverPrimaryRules|CoverageAssessmentPreservesCurrentRequiredOutputShape|RecoveryActionRetriesWithinBudget|RuntimeRepairInputIncludesContractViolationRecovery)' -count=1 -timeout=300s` |
| extractedSummary | Live verification-stage retry showed `decision requests a continuable candidate choice`: runtime correctly converted a continuable decision blocker into a contract failure, but the correction contract still listed bare `blocked_on_decision` as an allowed output. That made the retry prompt internally contradictory: it asked for bounded validation evidence while allowing another unqualified decision blocker. Local core closure changes correction contracts to allow only `blocked_on_decision_with_required_external_dependency` and `blocked_on_approval_with_required_governance_dependency`, while explicitly forbidding continuable candidate decision requests. The rule is based on generic governance/dependency shape, not workspace title, template, provider, endpoint, command, or language. |
| owner | core runtime contract retry |
| closureBatch | 7 |
| loadByDefault | false |

### ev:code/graph-node-primary-diagnostic-dependency-evidence/20260623-033

| Field | Value |
| --- | --- |
| claimId | `C-B7-GRAPH-NODE-PRIMARY-DIAGNOSTIC-DEPENDENCY-EVIDENCE-20260623-033` |
| workspaceId | `ws-4e83810542ac4d65` |
| runId | `run-003-replan-001-replan-001-replan-001-replan-001-replan-001-replan-001` |
| evidenceKind | live-blocker-and-code-test-output |
| sourcePath | deployed `http://127.0.0.1:8080` plus `internal/modules/kernel/orchestrator/biz/runtime_patch_contract.go`, `internal/modules/kernel/orchestrator/biz/runtime_primary_diagnostic_repair_test.go` |
| extractCommand | `curl -fsS http://127.0.0.1:8080/api/v1/workorch/workspaces/ws-4e83810542ac4d65/snapshot; jq '{graphNodeId,graphCompletedNodes,previousStepOutput,graphEvidence,primaryDiagnosticRepair}' .workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/<run>/steps/<run>/008-graph-002-002-add-output-limits-struct/input.json; go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMExecutionOutputContract(AllowsGraphNodeAfterDependencyAddressesPrimaryDiagnostic|RejectsPatchThatMissesPrimaryDiagnostics|AllowsPatchOverPrimaryDiagnosticLines|AllowsRelatedPatchWithPrimaryDiagnosticDeclaration|AllowsInferredRelatedShapePatch|AllowsTextReplacePrimaryDiagnosticPatch|AllowsTextReplaceDeletingPrimaryDiagnosticLine|AllowsImportPatchForLinePayloadShortIdentifier|RejectsInferredRelatedCommentOnlyPatch)|TestStructuredFailureSignalsExtractShortLinePayloadIdentifier' -count=1 -timeout=240s` |
| extractedSummary | Post-redeploy live `ws-4e...` moved past the previous focus requiredOutput blocker and produced a state-changing repair graph. The first graph node patched the primary diagnostic target and completed; the second graph node targeted the same diagnostic resource but was rejected with `primary diagnostic repair requires patch/apply to address line-referenced resource shape blockers before secondary changes` because the child step inherited the original primary diagnostic contract and the gate evaluated every graph node in isolation. Local core closure keeps the direct primary diagnostic gate strict, but also recognizes completed dependency graph evidence: when `previousStepOutput` or `graphEvidence` already contains a patch that directly satisfies the current primary diagnostic contract, later same-scope graph-node patches are no longer rejected by the stale primary diagnostic gate. This is business-neutral and consumes only structured graph evidence, target refs, line refs, and identifiers; it does not hardcode workspace IDs, templates, languages, commands, file names, or provider behavior. Focused regression tests prove the old blocker path is accepted only when dependency evidence has already addressed the primary diagnostic, while existing rejection tests remain active. |
| owner | core runtime graph/patch contract |
| closureBatch | 7 |
| loadByDefault | false |

## Default Load Set For Next Code Closure

Load only:

- L0: `runtime-contract-retry-convergence-closure-plan.md`
- The relevant L1 workspace audit summary
- The target batch section from `runtime-contract-retry-full-chain-audit-plan.md`
- Up to 3 evidence records from this index

Do not load raw `events-000001.jsonl`, full `request.json`, full `run.json`, or checkpoint directories unless a bounded extraction command is first defined here.
