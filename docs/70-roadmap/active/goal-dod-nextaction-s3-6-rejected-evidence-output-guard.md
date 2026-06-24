# S3.6 Rejected Evidence Output Guard

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: kernel orchestrator accepted result, available artifact, and completion memory write paths.
- Observed state: accepted result/artifact/memory writers collect `evidenceRefs` from validated DoD assessment, coverage items, patch details, source details, and validation details, but they do not reject refs that are already known in EvidenceRecord as `Status=rejected`.
- Expected state: rejected EvidenceRecord can remain in the ledger as diagnostic/provenance, but it must not be used to create accepted result, available artifact, or high-value completion memory.
- Deviation: a validated assessment or coverage item can accidentally promote a rejected diagnostic/validation evidence ref into formal accepted outputs.
- Owner: core. The orchestrator result subsystem owns formal result/artifact/memory writes.
- Event path: validated assessment / accepted slice / artifact source details -> result subsystem evidence ref gate -> accepted result/artifact/memory write or fail fast.
- Contract gap: accepted output writers do not consult the EvidenceRecord ledger for known rejected refs before writing formal outputs.
- Proposed change: add a neutral EvidenceRecord status gate in result subsystem accepted-output paths. The gate only blocks refs that match an existing rejected EvidenceRecord by ID, sourceRef, stepID, or subjectID.
- Boundary check: implementation only consumes structured evidence status and refs. It does not inspect template IDs, Soul IDs, programming languages, filenames, commands, provider names, or business goal text.
- Verification: owner tests prove rejected evidence refs cannot write accepted result/artifact/memory, while rejected results remain allowed for failure reporting; boundary search verifies no business-specific core logic is introduced.

## Scope

In scope:

- Accepted completion result writes.
- Accepted slice result writes.
- Available artifact writes.
- Completion memory writes derived from accepted DoD assessment.
- Preflight guard before parallel completion evidence writers to avoid partial accepted output writes.

Out of scope:

- Requiring every historical step ref to already have an accepted EvidenceRecord.
- Coverage ledger satisfaction from accepted/rejected evidence.
- Rejected result writes used to report failed completion gates.
- TUI/Web projection changes.

## Design

The result subsystem resolves the current run EvidenceRecord list once per write path and blocks if an output evidence ref matches a rejected record by:

- EvidenceRecord `ID`
- EvidenceRecord `SourceRef`
- EvidenceRecord `StepID`
- EvidenceRecord `SubjectID`

Unknown refs are not rejected in this slice. Full required-evidence satisfaction belongs to P4 Coverage Ledger.

## Acceptance

- Accepted DoD result cannot be written with a known rejected evidence ref.
- Accepted slice result cannot be written with a known rejected evidence ref.
- Available artifact cannot be written with a known rejected evidence ref.
- Completion memory cannot be written with a known rejected evidence ref.
- Rejected completion result remains allowed to reference rejected evidence for failure reporting.

## Verification

Executed on 2026-06-24:

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestAcceptedCompletionOutputsRejectRejectedEvidenceRefs|TestAcceptedSliceResultRejectsRejectedEvidenceRefs|TestAvailableArtifactRejectsRejectedEvidenceRefs|TestCompletionMemoryRejectsRejectedEvidenceRefs|TestRejectedCompletionResultMayReferenceRejectedEvidence' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/kernel/orchestrator -count=1`
- `go test ./internal/modules/kernel/... -count=1`
- `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md docs/70-roadmap/active/goal-dod-nextaction-s3-6-rejected-evidence-output-guard.md internal/modules/kernel/orchestrator/biz/runtime_result_evidence_guard.go internal/modules/kernel/orchestrator/biz/runtime_result_recording.go internal/modules/kernel/orchestrator/biz/runtime_result_artifacts.go internal/modules/kernel/orchestrator/biz/runtime_result_memory.go internal/modules/kernel/orchestrator/biz/runtime_result_evidence_guard_test.go -S`

Boundary result: the only matches are the generic validation commands documented in the roadmap. The S3.6 core implementation does not introduce template, language, command, provider, or business-specific branching.
