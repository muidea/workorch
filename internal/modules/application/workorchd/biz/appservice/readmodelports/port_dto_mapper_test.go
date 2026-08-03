package readmodelports

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustPortEventPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("event payload: %v", err)
	}
	return payload
}

func TestPortWorkspaceSnapshotInfoPreservesTypedProjectionFields(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-map",
			State:            string(common.WorkspaceStateActive),
			ActiveRunID:      "run-map",
			BlockingTicketID: "ticket-map",
		},
		ViewModel: readmodelcore.WorkspaceViewModel{
			WorkspaceID: "ws-map",
			State:       "running",
			PromptMode:  "decision",
			Revision:    7,
			Sections: map[string]readmodelcore.WorkspaceViewModelSection{
				"prompt": {Name: "prompt", Revision: 7, State: "decision"},
			},
		},
		RunHeartbeat: readmodelcore.RunHeartbeat{
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "running",
			Stalled:     true,
		},
		ConvergenceStatus: &readmodelcore.ConvergenceStatus{
			WorkspaceID:          "ws-map",
			RunID:                "run-map",
			RunState:             string(common.RunStateActive),
			ConvergenceState:     "need_state_change",
			MissingEvidenceKinds: []string{"artifact"},
			IneffectiveRetry:     true,
		},
		RunCommand: &readmodelcore.RunCommandStatus{
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "accepted",
			Intent:      "replan",
			DecisionID:  "ticket-map",
			Revision:    9,
		},
		GapBaseline: &readmodelcore.GapBaseline{
			WorkspaceID:          "ws-map",
			RunID:                "run-map",
			BaselineRevision:     "gap-1",
			AcceptedEvidenceRefs: []string{"evidence-a"},
			CurrentFocus:         map[string]any{"id": "focus-1"},
		},
		Runs: []readmodelcore.Run{{
			ID:          "run-map",
			WorkspaceID: "ws-map",
			State:       string(common.RunStateActive),
		}},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-map",
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "open",
		}},
		EventRecords: []readmodelcore.EventRecord{{
			ID:          "event-map",
			SubjectType: "run",
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			Payload: mustPortEventPayload(t, map[string]any{
				"state": "active",
			}),
			PayloadRef: "event://payload/event-map",
		}},
	}

	got := PortWorkspaceSnapshotInfo(snapshot)
	if got.Workspace.ID != "ws-map" || got.Workspace.ActiveRunID != "run-map" {
		t.Fatalf("workspace = %+v, want typed workspace IDs", got.Workspace)
	}
	if got.ViewModel.PromptMode != "decision" || got.ViewModel.Sections["prompt"].Revision != 7 {
		t.Fatalf("viewModel = %+v, want prompt projection", got.ViewModel)
	}
	if got.RunHeartbeat.RunID != "run-map" || !got.RunHeartbeat.Stalled {
		t.Fatalf("runHeartbeat = %+v, want heartbeat fields", got.RunHeartbeat)
	}
	if got.ConvergenceStatus == nil || got.ConvergenceStatus.ConvergenceState != "need_state_change" || !got.ConvergenceStatus.IneffectiveRetry {
		t.Fatalf("convergence = %+v, want convergence fields", got.ConvergenceStatus)
	}
	if got.RunCommand == nil || got.RunCommand.DecisionID != "ticket-map" || got.RunCommand.Revision != 9 {
		t.Fatalf("runCommand = %+v, want decision command metadata", got.RunCommand)
	}
	if got.GapBaseline == nil || got.GapBaseline.AcceptedEvidenceRefs[0] != "evidence-a" {
		t.Fatalf("gapBaseline = %+v, want evidence refs", got.GapBaseline)
	}
	if got.Decisions[0].ID != "ticket-map" || len(got.EventRecords[0].Payload.Fields) != 1 {
		t.Fatalf("governance/event = %+v/%+v, want mapped values", got.Decisions, got.EventRecords)
	}
	if got.EventRecords[0].Summary != "active" || got.EventRecords[0].PayloadRef != "event://payload/event-map" {
		t.Fatalf("event record = %+v, want typed payload summary and payload ref", got.EventRecords[0])
	}
}

func TestPortRunInfoPreservesSealedTargetWithoutAliasing(t *testing.T) {
	sealed := runtimecontract.ProviderModelTarget{ProviderID: "provider-read", ModelID: "model-read"}
	got := PortRunInfo(appmodelcore.Run{
		ID:              "run-read",
		WorkspaceID:     "ws-read",
		SealedLLMTarget: &sealed,
	})
	if got.SealedLLMTarget == nil || !got.SealedLLMTarget.Same(sealed) {
		t.Fatalf("sealed target = %+v, want %+v", got.SealedLLMTarget, sealed)
	}
	got.SealedLLMTarget.ModelID = "mutated-port-value"
	if sealed.ModelID != "model-read" {
		t.Fatalf("port mapping aliases app model target: %+v", sealed)
	}
}

func TestPortAcceptanceProjectionSeparatesDirectiveFromAssessmentObservation(t *testing.T) {
	input := readmodelcore.AcceptanceProjection{
		RepairDirective: &readmodelcore.RuntimeRepairDirective{
			ID: "repair-1", FocusID: "focus-1", Trigger: "assessment_gap", RequiredAction: "need_state_change",
			RequiredEvidenceKinds: []string{"state_change"}, TargetRefs: []string{"owner-grounded.go"},
			Closure: &readmodelcore.RuntimeRepairClosure{ID: "closure-1", BaselineEvidenceRevision: "evidence-1", GroundedTargetRefs: []string{"owner-grounded.go"}},
		},
		Audit: &readmodelcore.RuntimeAudit{
			Repair: readmodelcore.RuntimeAuditRepair{FocusID: "focus-1", RepairAttempts: 2},
			CoverageBindings: []readmodelcore.RuntimeAuditCoverageBinding{{
				ID: "binding/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", Hash: "binding-hash",
				TargetRefs: []string{"owner-grounded.go"}, DeclarationRef: "profile/fixture",
				Proofs: []readmodelcore.RuntimeAuditCoverageProof{{EvidenceKind: "validation", ValidationRecipeID: "recipe-1", RequiredCheckRefs: []string{"check-1"}}},
			}},
			Dispatches: []readmodelcore.RuntimeAuditDispatch{{
				ID: "dispatch-1", AuthoritySnapshotID: "authority-1", State: "committed", CandidateHash: "candidate-1", CompletedOperations: 1,
				Operations: []readmodelcore.RuntimeAuditSealedOperation{{
					ID: "validate-1", Kind: "shell/command", BindingRefs: []string{"binding/cwi-1"}, TargetRefs: []string{"owner-grounded.go"},
				}},
			}},
			AcceptedEvidence: []readmodelcore.RuntimeAuditEvidence{{
				ID: "evidence-1", DispatchID: "dispatch-1", OperationID: "validate-1", BindingRefs: []string{"binding/cwi-1"},
				CoverageWorkItemIDs: []string{"cwi-1"}, ValidationRecipeID: "recipe-1", PassedChecks: []string{"check-1"},
			}},
			OperationReceipts: []readmodelcore.RuntimeAuditOperationReceipt{{
				ExecutionKey: "exec/ws/run/dispatch-1/validate-1", Fence: 1, OperationID: "validate-1", Outcome: "accepted",
				ObservedEffectScope: "none", FeedbackDomain: "operation", StableCategory: "accepted",
				OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
					SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "blocks/shell", OriginalCode: "owner_observed", OriginalMessage: "safe owner fact",
					FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "parameters.summary", Forbidden: []string{"summary", "summary"}}},
					DetailsRef:      "diagnostic/receipt", DetailsHash: "details-hash", DetailsBytes: 8193, OmittedReason: "raw_owner_payload", Truncated: true, Redacted: true,
				}, ValidationRecipeID: "recipe-1", PassedChecks: []string{"check-1"},
			}},
			Failure: &readmodelcore.RuntimeAuditFailure{
				Class: "runtime_internal_contract_misalignment", Code: "definition_mismatch", Signature: "semantic-rejection", Message: "definition revision is stale",
				OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{OwnerModuleID: "blocks/run", OriginalCode: "definition_mismatch", OriginalMessage: "definition revision is stale", ObservedRevision: "4"},
			},
			Assessments: []readmodelcore.RuntimeAuditAssessment{{
				ID: "assessment-1", FocusID: "focus-1", ClaimedDisposition: "terminal_rejection", CanonicalDisposition: "repairable_gap", Summary: "producer observation", EvidenceRefs: []string{"evidence-1"},
				Items: []readmodelcore.RuntimeAuditAssessmentItem{{CoverageWorkItemID: "cwi-1", Satisfied: false, Reason: "item reason"}},
				Gap:   &readmodelcore.RuntimeAuditAssessmentGap{MissingEvidenceKinds: []string{"validation"}, TargetRefs: []string{"claimed.go"}, Reason: "claimed reason", FailureCode: "coverage_open"},
			}},
		},
	}
	got := portAcceptanceProjectionInfo(input)
	if got.RepairDirective == nil || got.RepairDirective.RequiredAction != "need_state_change" || got.RepairDirective.Closure == nil ||
		got.RepairDirective.Closure.GroundedTargetRefs[0] != "owner-grounded.go" || got.Audit == nil || len(got.Audit.Assessments) != 1 {
		t.Fatalf("acceptance port observation/directive = %+v", got)
	}
	observation := got.Audit.Assessments[0]
	if observation.ClaimedDisposition != "terminal_rejection" || observation.CanonicalDisposition != "repairable_gap" ||
		observation.Summary != "producer observation" || observation.Gap == nil || observation.Gap.MissingEvidenceKinds[0] != "validation" ||
		observation.Gap.TargetRefs[0] != "claimed.go" || observation.Items[0].Reason != "item reason" {
		t.Fatalf("assessment observation port mapping = %+v", observation)
	}
	if got.Audit.Failure == nil || got.Audit.Failure.Code != "definition_mismatch" || got.Audit.Failure.OriginalDiagnostic == nil ||
		got.Audit.Failure.OriginalDiagnostic.OwnerModuleID != "blocks/run" || got.Audit.Failure.OriginalDiagnostic.ObservedRevision != "4" {
		t.Fatalf("failure diagnostic port mapping = %+v", got.Audit.Failure)
	}
	if len(got.Audit.CoverageBindings) != 1 || got.Audit.CoverageBindings[0].ID != "binding/cwi-1" ||
		len(got.Audit.CoverageBindings[0].Proofs) != 1 || got.Audit.CoverageBindings[0].Proofs[0].RequiredCheckRefs[0] != "check-1" ||
		len(got.Audit.Dispatches) != 1 || got.Audit.Dispatches[0].ID != "dispatch-1" || len(got.Audit.Dispatches[0].Operations) != 1 ||
		got.Audit.Dispatches[0].Operations[0].BindingRefs[0] != "binding/cwi-1" ||
		len(got.Audit.AcceptedEvidence) != 1 || got.Audit.AcceptedEvidence[0].BindingRefs[0] != "binding/cwi-1" ||
		got.Audit.AcceptedEvidence[0].CoverageWorkItemIDs[0] != "cwi-1" ||
		len(got.Audit.OperationReceipts) != 1 || got.Audit.OperationReceipts[0].ExecutionKey != "exec/ws/run/dispatch-1/validate-1" ||
		got.Audit.OperationReceipts[0].OriginalDiagnostic == nil || got.Audit.OperationReceipts[0].OriginalDiagnostic.OwnerModuleID != "blocks/shell" ||
		got.Audit.OperationReceipts[0].OriginalDiagnostic.OriginalMessage != "safe owner fact" ||
		got.Audit.OperationReceipts[0].OriginalDiagnostic.DetailsBytes != 8193 || len(got.Audit.OperationReceipts[0].OriginalDiagnostic.FieldViolations) != 1 ||
		len(got.Audit.OperationReceipts[0].OriginalDiagnostic.FieldViolations[0].Forbidden) != 2 || got.Audit.OperationReceipts[0].OriginalDiagnostic.OmittedReason != "raw_owner_payload" {
		t.Fatalf("binding audit chain port mapping = %+v", got.Audit)
	}
}

func TestPortAcceptanceProjectionPreservesSemanticPlanningScopeAndOriginalDiagnostics(t *testing.T) {
	diagnostic := readmodelcore.RuntimeSemanticDiagnostic{
		SchemaVersion: "diagnostic-v1", OwnerModuleID: "blocks/file", OriginalClass: "owner_contract",
		OriginalCode: "target_stale", OriginalMessage: "owner message", Signature: "signature-1",
		OperationID: "op-1", TargetRefs: []string{"source/a.go"}, ExpectedRevision: "tree-1", ObservedRevision: "tree-2",
		FieldViolations: []readmodelcore.RuntimeSemanticFieldViolation{{
			Path: "$.operations[0]", Actual: "summary", Allowed: []string{"targetRef"},
			Required: []string{"content"}, Forbidden: []string{"payload", "summary"},
		}},
		DetailsRef: "runtime-diagnostic/ws/run/op-1", DetailsHash: "details-hash", DetailsBytes: 4097,
		ContentType: "application/json", OmittedReason: "bounded_payload", Truncated: true, Redacted: true,
	}
	input := readmodelcore.AcceptanceProjection{SemanticPlanning: &readmodelcore.RuntimeSemanticPlanning{
		WorkspaceID: "ws-semantic", RunID: "run-semantic", DefinitionRevision: "definition-semantic", StateRevision: 31,
		State: "feasibility", Phase: "semantic_feasibility_required", Attempt: 2, CatalogRefreshAttempts: 1,
		LogicalRequestID: "request-1", ManifestHash: "manifest-1", DocumentHash: "document-1",
		FeasibilityBatchID: "batch-1", FeasibilitySetHash: "set-1", SealedBindingSetHash: "binding-set-1",
		Items: []readmodelcore.RuntimeSemanticPlanningItem{{
			CoverageWorkItemID: "cwi-1", ProposalHash: "proposal-1", OwnerModuleID: "blocks/file", OperationKind: "file/write",
			Status: "prerequisite_required", TargetRefs: []string{"source/a.go"}, ValidationRecipeID: "recipe-1",
			RequiredCheckRefs: []string{"check-1"}, ObservedRevision: "tree-2", Diagnostic: &diagnostic,
		}},
		Rejections: []readmodelcore.RuntimeSemanticDiagnostic{diagnostic},
		FeasibilityGaps: []readmodelcore.RuntimeSemanticFeasibilityGap{{
			RefreshGeneration: 1, ManifestHash: "manifest-old", DocumentHash: "document-old",
			FeasibilityBatchID: "batch-old", FeasibilitySetHash: "set-old",
			Items:      []readmodelcore.RuntimeSemanticPlanningItem{{CoverageWorkItemID: "cwi-1", Status: "stale", ObservedRevision: "tree-1"}},
			Rejections: []readmodelcore.RuntimeSemanticDiagnostic{diagnostic},
		}},
		Failure: &diagnostic,
	}}

	got := portAcceptanceProjectionInfo(input)
	semantic := got.SemanticPlanning
	if semantic == nil || semantic.WorkspaceID != "ws-semantic" || semantic.RunID != "run-semantic" ||
		semantic.DefinitionRevision != "definition-semantic" || semantic.StateRevision != 31 || semantic.State != "feasibility" ||
		semantic.Phase != "semantic_feasibility_required" || semantic.Attempt != 2 || semantic.CatalogRefreshAttempts != 1 ||
		semantic.LogicalRequestID != "request-1" || semantic.ManifestHash != "manifest-1" || semantic.DocumentHash != "document-1" ||
		semantic.FeasibilityBatchID != "batch-1" || semantic.FeasibilitySetHash != "set-1" || semantic.SealedBindingSetHash != "binding-set-1" {
		t.Fatalf("semantic planning port scope = %+v", semantic)
	}
	if len(semantic.Items) != 1 || semantic.Items[0].CoverageWorkItemID != "cwi-1" || semantic.Items[0].OwnerModuleID != "blocks/file" ||
		semantic.Items[0].TargetRefs[0] != "source/a.go" || semantic.Items[0].RequiredCheckRefs[0] != "check-1" || semantic.Items[0].Diagnostic == nil {
		t.Fatalf("semantic planning port item = %+v", semantic.Items)
	}
	if len(semantic.FeasibilityGaps) != 1 || semantic.FeasibilityGaps[0].RefreshGeneration != 1 ||
		semantic.FeasibilityGaps[0].FeasibilitySetHash != "set-old" || len(semantic.FeasibilityGaps[0].Rejections) != 1 ||
		semantic.Failure == nil || len(semantic.Rejections) != 1 {
		t.Fatalf("semantic planning port lineage = %+v", semantic)
	}
	portDiagnostic := semantic.Items[0].Diagnostic
	if portDiagnostic.OriginalMessage != "owner message" || len(portDiagnostic.FieldViolations) != 1 ||
		portDiagnostic.FieldViolations[0].Actual != "summary" || portDiagnostic.FieldViolations[0].Allowed[0] != "targetRef" ||
		portDiagnostic.FieldViolations[0].Required[0] != "content" || portDiagnostic.FieldViolations[0].Forbidden[1] != "summary" ||
		portDiagnostic.DetailsRef != "runtime-diagnostic/ws/run/op-1" || portDiagnostic.DetailsHash != "details-hash" ||
		portDiagnostic.DetailsBytes != 4097 || portDiagnostic.ContentType != "application/json" ||
		portDiagnostic.OmittedReason != "bounded_payload" || !portDiagnostic.Truncated || !portDiagnostic.Redacted {
		t.Fatalf("semantic planning port diagnostic = %+v", portDiagnostic)
	}

	semantic.Items[0].TargetRefs[0] = "mutated"
	semantic.Items[0].Diagnostic.FieldViolations[0].Allowed[0] = "mutated"
	if input.SemanticPlanning.Items[0].TargetRefs[0] != "source/a.go" || input.SemanticPlanning.Items[0].Diagnostic.FieldViolations[0].Allowed[0] != "targetRef" {
		t.Fatalf("semantic planning port mapping aliases read model: %+v", input.SemanticPlanning)
	}
}

func TestPortRunInfoProjectsFreshActivationStatusWithoutAliasing(t *testing.T) {
	activation := &appmodelcore.RuntimeActivationStatus{
		Disposition:            "fresh_activation_required",
		ActualSchemaVersion:    2,
		SupportedSchemaVersion: 4,
	}
	got := PortRunInfo(appmodelcore.Run{
		ID:                "run-incompatible-schema",
		WorkspaceID:       "ws-incompatible-schema",
		RuntimeActivation: activation,
	})
	if got.RuntimeActivation == nil ||
		got.RuntimeActivation.Disposition != "fresh_activation_required" ||
		got.RuntimeActivation.ActualSchemaVersion != 2 ||
		got.RuntimeActivation.SupportedSchemaVersion != 4 {
		t.Fatalf("runtime activation = %+v, want bounded fresh-activation projection", got.RuntimeActivation)
	}
	got.RuntimeActivation.Disposition = "mutated-port-value"
	if activation.Disposition != "fresh_activation_required" {
		t.Fatalf("port mapping aliases app model activation: %+v", activation)
	}
}

func TestPortWorkspaceSnapshotInfoDoesNotFallbackToPayloadSummaryMap(t *testing.T) {
	got := PortWorkspaceSnapshotInfo(readmodelcore.WorkspaceSnapshot{
		EventRecords: []readmodelcore.EventRecord{{
			ID:             "event-summary-only",
			SubjectType:    "run",
			PayloadSummary: mustPortEventPayload(t, map[string]any{"state": "active"}),
		}},
	})
	if len(got.EventRecords) != 0 {
		t.Fatalf("event records = %+v, want summary-only map record rejected", got.EventRecords)
	}
}

func TestPortRuntimeProgressPreservesPrerequisiteRefreshAudit(t *testing.T) {
	got := portRuntimeProgressInfo(&readmodelcore.RuntimeProgress{
		PrerequisiteRefreshState: "prerequisite_refreshing", PrerequisiteRefreshUsed: 1, PrerequisiteRefreshLimit: 3,
		AcceptedValidationEvidenceCount: 2,
		PrerequisiteInvalidations: []readmodelcore.RuntimePrerequisiteRefreshInvalidation{{
			Signature: "resource-change-1", Source: "owner_receipt_resource_change", OwnerModuleID: "file",
			ResourceDomain: "workspace_target", SubjectRef: "main.go",
			ResourceChanges: []readmodelcore.RuntimePrerequisiteResourceChange{{
				OwnerModuleID: "file", ResourceDomain: "workspace_target", SubjectRef: "main.go", BeforeRevision: "v1", AfterRevision: "v2",
			}},
		}},
	})
	if got == nil || got.PrerequisiteRefreshState != "prerequisite_refreshing" || got.PrerequisiteRefreshUsed != 1 ||
		got.PrerequisiteRefreshLimit != 3 || got.AcceptedValidationEvidenceCount != 2 || len(got.PrerequisiteInvalidations) != 1 {
		t.Fatalf("runtime refresh port = %+v", got)
	}
	invalidation := got.PrerequisiteInvalidations[0]
	if invalidation.Source != "owner_receipt_resource_change" || invalidation.ResourceDomain != "workspace_target" ||
		invalidation.SubjectRef != "main.go" || len(invalidation.ResourceChanges) != 1 || invalidation.ResourceChanges[0].AfterRevision != "v2" {
		t.Fatalf("runtime refresh source port = %+v", invalidation)
	}
}
