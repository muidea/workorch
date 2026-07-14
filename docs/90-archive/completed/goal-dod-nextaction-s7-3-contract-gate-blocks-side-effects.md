# S7.3 Contract Gate Blocks Side Effects

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [Contract Violation Recovery Loop Design](../../30-module-architecture/contract-violation-recovery-loop-design.md)
- [S3.6 Rejected Evidence Output Guard](./goal-dod-nextaction-s3-6-rejected-evidence-output-guard.md)
- [S7.2 Parse/Schema Gate Recoverable Diagnostic](./goal-dod-nextaction-s7-2-parse-schema-gate-recoverable-diagnostic.md)

## Contract Audit

- Scope: `internal/modules/kernel/orchestrator/biz/runtime_graph.go`, graph dispatch admission, rejected diagnostic evidence and accepted output guard tests.
- Observed state: Runtime step failure paths call `attachContractViolationDiagnostics` and `recordRejectedDiagnosticEvidenceForContractViolation`, and S3.6 already prevents known rejected evidence refs from backing accepted result, artifact or completion memory. Existing integration tests prove assessment-only executable graphs are rejected and not executed. However, graph continuation generation still accepts any step carrying an `executionGraph` shape without an explicit local admission guard that the graph source step is completed and free of contract violation markers.
- Expected state: a step whose output failed the runtime contract must be terminal diagnostic material only. Even if the rejected output contains an `executionGraph`, the graph dispatch layer must not generate child dispatch specs from that output.
- Deviation: normal mainline failure handling prevents this in practice, but the graph dispatch owner lacks a direct side-effect admission check. A caller that passes a failed or contract-violating step into `readyGraphStepSpecsWithSteps` can still receive executable specs.
- Owner: core.
- Event path: LLM output -> contract gate -> failed source step with rejected diagnostic -> graph dispatch admission -> no child side-effect specs; accepted result/artifact/memory remains guarded by rejected evidence refs.
- Contract gap: graph continuation owner does not explicitly reject failed or contract-violating graph sources before side-effect dispatch spec generation.
- Proposed change: add a business-neutral graph source admission guard before `readyGraphStepSpecsWithSteps` parses/dispatches graph nodes, and add a focused unit test proving a rejected contract-violation graph produces no executable specs.
- Boundary check: the guard uses only generic step state and contract violation fields. It does not inspect template ID, Soul ID, workspace title, programming language, provider, command output or business text.
- Verification: focused graph test, existing rejected evidence output guard test, orchestrator package targeted test, `git diff --check`, and boundary search over changed files.

## Slice Scope

S7.3 covers side-effect admission at graph dispatch and accepted output guard verification.

In scope:

- Do not generate graph child specs from failed source steps.
- Do not generate graph child specs from source steps already marked with `contractViolationType` or `contractViolation`.
- Keep rejected evidence refs blocked from accepted result/artifact/memory paths.

Out of scope:

- Retry inheritance semantics.
- Retry budget and repeated signature escalation.
- Projection rendering of recovery state.
- Template/Soul prompt strategy.
- Provider transport retry behavior.

## Acceptance

- A failed LLM workspace step with `executionGraph` and contract violation markers produces zero graph dispatch specs.
- A completed step with contract violation markers also produces zero graph dispatch specs.
- Existing accepted output guards still reject known rejected evidence refs for accepted result, artifact and memory writes.
- The change does not add business, template, language, command, provider or scenario-specific logic to core.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestGraphSourceContractViolationBlocksSideEffectSpecs|TestAccepted.*RejectedEvidence|TestAvailableArtifactRejectsRejectedEvidenceRefs|TestCompletionMemoryRejectsRejectedEvidenceRefs' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -run 'TestGraphSourceContractViolationBlocksSideEffectSpecs|TestAccepted.*RejectedEvidence|TestAvailableArtifactRejectsRejectedEvidenceRefs|TestCompletionMemoryRejectsRejectedEvidenceRefs' -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/modules/kernel/orchestrator/biz/runtime_graph.go internal/modules/kernel/orchestrator/biz/runtime_graph_test.go docs/70-roadmap/active/goal-dod-nextaction-s7-3-contract-gate-blocks-side-effects.md`
