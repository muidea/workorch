package readmodelprojection

import (
	"encoding/json"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapAcceptanceProjectionReadsPersistedRuntimeFacts(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{StateRevision: 7, EvidenceSequence: 3, Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-3",
		RemainingFocuses: []runtimecontract.CoverageFocus{{ID: "focus-next"}}, CompletedFocuses: []runtimecontract.CoverageFocus{{ID: "focus-done"}},
		Focus:  runtimecontract.CoverageFocus{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-1", SliceID: "slice-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied},
			{ID: "cwi-2", SliceID: "slice-1", State: runtimecontract.RuntimeCoverageItemStateOpen},
		},
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
		Recovery:          runtimecontract.RuntimeRecoveryLedger{CASReloadAttempts: 1},
		Repair:            runtimecontract.RuntimeRepairLedger{FocusID: "focus-1", SliceID: "slice-1", RepairAttempts: 2},
		RepairDirective: &runtimecontract.RuntimeRepairDirective{ID: "repair-1", FocusID: "focus-1", Trigger: runtimecontract.RuntimeRepairTriggerAssessmentGap, RequiredState: runtimecontract.ConvergenceStateNeedValidation,
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			Closure: &runtimecontract.RuntimeRepairClosure{ID: "repair-closure-70949d7491e40e311242", GapSignature: "gap-1", BaselineEvidenceRevision: "evidence-3", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, GroundedTargetRefs: []string{"main.txt"}},
		},
	}, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation,
		AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1", WorkspaceID: "ws-1", RunID: "run-1", State: runtimecontract.ConvergenceStateNeedValidation, FocusID: "focus-1", RequiredOutputHash: "output-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-3", CapabilityStateHash: "capability-state-hash", PrerequisiteFreshnessHash: "freshness-1", OperationContractRevision: "operations-1"},
	},
		ActiveDispatch: &runtimecontract.RuntimeDispatchRecord{ID: "dispatch-1", Transaction: runtimecontract.AuthorityTransaction{CapabilityStateHash: "capability-state-hash"}, Candidate: runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "validate-1", Kind: runtimecontract.StepKindGitRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: strPtr("status")}},
		}}}}}, OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID: "validate-1", ExecutionKey: "exec-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
			State: runtimecontract.OperationExecutionOwnerCommitted, OwnerReceiptRef: "runtime-receipt/exec-1", OwnerReceiptHash: "hash-1",
		}}},
		OperationReceipts: []runtimecontract.RuntimeOperationReceipt{{ExecutionKey: "exec-1", Fence: 1, OperationID: "validate-1",
			Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone}},
		GovernanceRequests:    []runtimecontract.RuntimeGovernanceRequestRecord{{TicketID: "ticket-1", AuthoritySnapshotID: "snapshot-1", DispatchID: "dispatch-1", DependencyKind: "decision", SubjectRef: "workspace"}},
		GovernanceResolutions: []runtimecontract.RuntimeGovernanceResolutionRecord{{ResolutionID: "resolution-1", TicketID: "ticket-1", AuthoritySnapshotID: "snapshot-1", DispatchID: "dispatch-1", Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue}},
	}
	state.Facts.Resources.CapabilityStates = []runtimecontract.RuntimeCapabilityStateSnapshot{{
		RequirementID: "capability-state:git", SnapshotID: "snapshot-git", AcquisitionKey: "capability-state/ws/run/0", QueryReceiptRef: "receipt-git",
		OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-absent", ContentHash: "hash-git",
		Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired, RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
	}}
	state.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "evidence-1", Kind: runtimecontract.EvidenceKindStateChange, AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1", OperationID: "write-1", Revision: "runtime-evidence-00000000000000000001", TargetRefs: []string{"main.txt"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}
	state.Facts.Resources.TargetSelection = &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection:file", SnapshotID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/profile_declared_candidates", SelectorOwnerRef: "profile-1", SelectorSubjectRef: "subject-1",
		SelectorCandidateRefs: []string{"main.txt"}, Disposition: runtimecontract.RuntimeTargetSelectionSelected,
		TargetRefs: []string{"main.txt"}, CandidateTargetRefs: []string{"main.txt"}, CandidateCount: 1, ObservedRevision: "tree-1",
	}
	state.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{TargetRef: "main.txt", Content: "secret source", ContentHash: "content-1", ContentState: runtimecontract.SnapshotContentStateComplete, Size: 13, ObservedAt: "2026-07-24T00:00:00Z"}}
	state.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}}, ContextWindowTokens: 128000, ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 16384, MaxInputTokens: 111616, MaxBytes: 446464, CapabilityRevision: "capability-1", ConfigurationRevision: "configuration-1"}
	state.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{{
		PrerequisiteID: "target-selection:file", Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		ObservationID: "observation-1", ContentHash: "selection-1", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace", Revision: "tree-1"}},
	}}
	state.Facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-1", FocusID: "focus-1", SliceID: "slice-1", Scope: "acceptance", RequirementID: "assessment-required", RequirementText: "verify result",
		ClaimedDisposition: runtimecontract.AssessmentDispositionTerminalRejection, Disposition: runtimecontract.AssessmentDispositionRepairableGap, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, Summary: "implementation evidence remains incomplete",
		EvidenceRefs: []string{"runtime-operation/validate-1"}, ArtifactRefs: []string{"artifact-1"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-2", Satisfied: false, Reason: "evidence does not close coverage", EvidenceRefs: []string{"runtime-operation/validate-1"}, ArtifactRefs: []common.ArtifactID{"artifact-1"}}},
		Gap:   &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"claimed.txt"}, EvidenceRefs: []string{"runtime-operation/validate-1"}, Reason: "claimed durable change is missing", FailureClass: runtimecontract.FailureClassCoverageAssessmentIncomplete, FailureCode: "coverage_open", FailureSignature: "gap-1"},
	}}
	state.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: "contract-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"}, State: runtimecontract.RuntimeCandidateAttemptStateAccepted, StablePrefixHash: "candidate-prefix-1", Proposal: &runtimecontract.RuntimeCandidate{}}}
	receiptFailure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "target_missing", Signature: "receipt-failure-1", Message: "target is not available", TargetRefs: []string{"missing.txt"}}
	receiptFeedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1"}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected), runtimecontract.RuntimeModuleFile, receiptFailure)
	if err != nil {
		t.Fatalf("receipt feedback: %v", err)
	}
	receiptFeedback.OriginalDiagnostic.OperationID = "write-1"
	receiptFeedback.OriginalDiagnostic.FieldViolations = []runtimecontract.RuntimeFieldViolation{
		{Path: "parameters.summary", Actual: "legacy", Forbidden: []string{"summary", "summary"}},
		{Path: "targetRefs", Required: []string{"missing.txt", "missing.txt"}},
	}
	receiptFeedback.OriginalDiagnostic.DetailsRef = "runtime-diagnostic/ws-1/run-1/write-1"
	receiptFeedback.OriginalDiagnostic.DetailsHash = "details-hash-1"
	receiptFeedback.OriginalDiagnostic.DetailsBytes = 4097
	receiptFeedback.OriginalDiagnostic.ContentType = "application/problem+json"
	receiptFeedback.OriginalDiagnostic.OmittedReason = "raw_owner_payload"
	receiptFeedback.OriginalDiagnostic.Truncated = true
	receiptFeedback.OriginalDiagnostic.Redacted = true
	receiptFeedback.OriginalDiagnostic.TargetObservation = &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", TargetRole: "implementation",
		ObservedRevision: "tree-7", TargetRefs: []string{"missing.txt"},
	}
	receiptFeedback.OriginalDiagnostic.ValidationDiagnostic = &runtimecontract.ValidationFailureDiagnostic{
		SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion, Class: runtimecontract.ValidationDiagnosticBuildFailed,
		Reason: "safe structured diagnostic", Operation: runtimecontract.StepKindShellCommand, TargetRefs: []string{"missing.txt"},
	}
	state.OperationReceipts = append(state.OperationReceipts, runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec-2", Fence: 1, OperationID: "write-1", Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Feedback: &receiptFeedback,
	})
	projection := mapAcceptanceProjection(&runmodel.Run{ID: "run-1", RuntimeState: &state})
	if projection.ActiveSliceID != "slice-1" || projection.CurrentFocus == nil || projection.CurrentFocus.ID != "focus-1" || projection.Budget == nil || projection.Budget.RepairAttempts != 2 {
		t.Fatalf("acceptance projection = %+v", projection)
	}
	if len(projection.Slices) != 1 || projection.Slices[0].State != "active" {
		t.Fatalf("slice projection = %+v", projection.Slices)
	}
	if projection.RepairDirective == nil || projection.RepairDirective.RequiredAction != string(runtimecontract.ConvergenceStateNeedValidation) ||
		len(projection.Operations) != 1 || projection.Operations[0].Fence != 1 || projection.Operations[0].Outcome != string(runtimecontract.RuntimeOperationOutcomeAccepted) ||
		projection.RecoveryBudget == nil || projection.RecoveryBudget.CASReloadAttempts != 1 || projection.Operations[0].Kind != runtimecontract.StepKindGitRead || projection.Operations[0].VariantID != "status" ||
		projection.Operations[0].CapabilityStateHash != "capability-state-hash" || len(projection.CapabilityStates) != 1 ||
		len(projection.CapabilityStates[0].RequiredOperationSequence) != 1 || len(projection.Governance) != 1 || projection.Governance[0].ResolutionID != "resolution-1" {
		t.Fatalf("runtime remediation projection = %+v", projection)
	}
	if projection.Audit == nil || projection.Audit.WorkspaceID != "ws-1" || projection.Audit.RunID != "run-1" || projection.Audit.StateRevision != 7 ||
		projection.Audit.Authority.SnapshotID != "authority-1" || len(projection.Audit.CandidateAttempts) != 1 || projection.Audit.CandidateAttempts[0].StablePrefixHash != "candidate-prefix-1" ||
		len(projection.Audit.AcceptedEvidence) != 1 || projection.Audit.AcceptedEvidence[0].OperationID != "write-1" || len(projection.Audit.RemainingFocusIDs) != 1 || len(projection.Audit.CompletedFocusIDs) != 1 ||
		projection.Audit.Grounding.TargetSelection == nil || projection.Audit.Grounding.TargetSelection.SelectorOwnerRef != "profile-1" || len(projection.Audit.Grounding.PrerequisiteObservations) != 1 ||
		len(projection.Audit.Grounding.TargetSnapshots) != 1 || projection.Audit.Grounding.TargetSnapshots[0].ContentHash != "content-1" || projection.Audit.Grounding.ProviderCeiling == nil ||
		projection.Audit.Grounding.ProviderCeiling.EffectiveMaxOutputTokens != 16384 || projection.Audit.Grounding.ProviderCeiling.MaxBytes != 446464 ||
		len(projection.Audit.Dispatches) != 1 || len(projection.Audit.Dispatches[0].Operations) != 1 ||
		projection.Audit.Dispatches[0].Operations[0].ExecutionKey != "exec-1" || projection.Audit.Dispatches[0].Operations[0].Fence != 1 ||
		len(projection.Audit.OperationReceipts) != 2 {
		t.Fatalf("runtime audit = %+v", projection.Audit)
	}
	if projection.RepairDirective == nil || projection.RepairDirective.RequiredAction != string(runtimecontract.ConvergenceStateNeedValidation) ||
		projection.RepairDirective.Closure == nil || projection.RepairDirective.Closure.BaselineEvidenceRevision != "evidence-3" ||
		projection.Audit.RepairDirective == nil || projection.Audit.RepairDirective.RequiredAction != string(runtimecontract.ConvergenceStateNeedValidation) ||
		len(projection.Audit.Assessments) != 1 {
		t.Fatalf("directive/assessment audit = %+v/%+v", projection.RepairDirective, projection.Audit)
	}
	observation := projection.Audit.Assessments[0]
	if observation.ClaimedDisposition != string(runtimecontract.AssessmentDispositionTerminalRejection) ||
		observation.CanonicalDisposition != string(runtimecontract.AssessmentDispositionRepairableGap) ||
		observation.Summary != "implementation evidence remains incomplete" || observation.RequirementText != "verify result" || len(observation.Items) != 1 ||
		observation.Items[0].Reason != "evidence does not close coverage" || observation.Gap == nil ||
		len(observation.Gap.MissingEvidenceKinds) != 1 || observation.Gap.MissingEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		observation.Gap.FailureCode != "coverage_open" || observation.Gap.Reason != "claimed durable change is missing" {
		t.Fatalf("assessment observation lost original diagnostic = %+v", observation)
	}
	for _, receipt := range projection.Audit.OperationReceipts {
		if receipt.ExecutionKey != "exec-2" {
			continue
		}
		diagnostic := receipt.OriginalDiagnostic
		if receipt.StableCategory != string(runtimecontract.RuntimeOperationOutcomeRejected) || diagnostic == nil || diagnostic.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
			diagnostic.OriginalCode != "target_missing" || diagnostic.OriginalMessage != "target is not available" || diagnostic.DetailsRef != "runtime-diagnostic/ws-1/run-1/write-1" ||
			diagnostic.DetailsHash != "details-hash-1" || diagnostic.DetailsBytes != 4097 || diagnostic.OmittedReason != "raw_owner_payload" ||
			!diagnostic.Truncated || !diagnostic.Redacted || len(diagnostic.FieldViolations) != 2 ||
			len(diagnostic.FieldViolations[0].Forbidden) != 2 || diagnostic.FieldViolations[0].Forbidden[0] != "summary" || diagnostic.FieldViolations[0].Forbidden[1] != "summary" ||
			diagnostic.TargetObservation == nil || diagnostic.TargetObservation.ObservedRevision != "tree-7" ||
			diagnostic.ValidationDiagnostic == nil || diagnostic.ValidationDiagnostic.Reason != "safe structured diagnostic" {
			t.Fatalf("receipt feedback audit = %+v", receipt)
		}
		break
	}
	raw, err := json.Marshal(projection.Audit)
	if err != nil {
		t.Fatalf("marshal runtime audit: %v", err)
	}
	if strings.Contains(string(raw), "secret source") || strings.Contains(string(raw), `"parameters":`) {
		t.Fatalf("runtime audit leaked raw source or candidate payload: %s", raw)
	}
	var auditWire map[string]any
	if err := json.Unmarshal(raw, &auditWire); err != nil {
		t.Fatalf("unmarshal runtime audit: %v", err)
	}
	repair, ok := auditWire["repair"].(map[string]any)
	if !ok {
		t.Fatalf("runtime audit repair = %#v, want object", auditWire["repair"])
	}
	for _, field := range []string{"repairAttempts", "validationOnlyRetries", "noProgressCount"} {
		if _, ok := repair[field]; !ok {
			t.Fatalf("runtime audit repair omits zero-safe counter %q: %s", field, raw)
		}
	}
	if _, leaked := repair["terminalDeliveryAttempts"]; leaked {
		t.Fatalf("runtime audit repair leaks SM-4 terminal delivery accounting: %s", raw)
	}
}

func TestMapAcceptanceProjectionDoesNotPresentTerminalFocusAsActive(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-terminal", RunID: "run-terminal",
			Focus:    runtimecontract.CoverageFocus{ID: "focus-terminal", SliceID: "slice-terminal", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-terminal"}},
			Slices:   []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-terminal", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-terminal"}}},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-terminal", SliceID: "slice-terminal", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		},
		Decision: runtimecontract.RuntimeDecision{
			State:    runtimecontract.ConvergenceStateFailed,
			Terminal: &runtimecontract.RuntimeTerminalDecision{Code: "terminal-test", Summary: "terminal"},
		},
	}

	projection := mapAcceptanceProjection(&runmodel.Run{ID: "run-terminal", RuntimeState: &state})
	if projection.ActiveSliceID != "" || len(projection.Slices) != 1 || projection.Slices[0].State == "active" {
		t.Fatalf("terminal acceptance projection = %+v, want historical focus without active slice", projection)
	}
	if projection.CurrentFocus == nil || projection.CurrentFocus.State != string(runtimecontract.ConvergenceStateFailed) {
		t.Fatalf("terminal focus = %+v, want terminal decision state", projection.CurrentFocus)
	}
}

func strPtr(value string) *string { return &value }
