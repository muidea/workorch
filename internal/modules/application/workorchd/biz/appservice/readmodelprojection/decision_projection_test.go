package readmodelprojection

import (
	"reflect"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mustProjectionPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestMapDecisionProjectsFailureRecoveryFields(t *testing.T) {
	item := &decisionmodel.Ticket{
		ID:                     "ticket-runtime-recovery",
		WorkspaceID:            "ws-runtime-recovery",
		RunID:                  "run-runtime-recovery",
		Reason:                 "runtime repair exhausted",
		State:                  common.DecisionTicketStateResolved,
		FailureClass:           "model_runtime_failed",
		FailureSignature:       "model-signature",
		AllowedRecoveryActions: []string{"resume"},
		ResolvedAt:             "2026-07-06T01:02:03Z",
		ResolvedBy:             "auto-governance",
		Context: mustProjectionPayload(t, map[string]any{
			"failureClass":           "runtime_failed",
			"failureSignature":       "class=runtime_failed|kind=llm/workspace_execution",
			"allowedRecoveryActions": []any{"replan", "terminate"},
		}),
	}
	got := mapDecision(item, nil, nil)
	if got.FailureClass != "model_runtime_failed" ||
		got.FailureSignature != "model-signature" ||
		len(got.AllowedRecoveryActions) != 1 ||
		got.AllowedRecoveryActions[0] != "resume" ||
		got.ResolvedAt != "2026-07-06T01:02:03Z" ||
		got.ResolvedBy != "auto-governance" {
		t.Fatalf("projected decision recovery fields = %+v", got)
	}
	view := projectDecisionView(got)
	if len(view.Context.Fields) == 0 ||
		view.FailureClass != got.FailureClass ||
		len(view.AllowedRecoveryActions) != 1 ||
		view.ResolvedAt != got.ResolvedAt ||
		view.ResolvedBy != got.ResolvedBy {
		t.Fatalf("decision view lost recovery fields: %+v", view)
	}
}

func TestMapDecisionFallsBackToContextRecoveryFields(t *testing.T) {
	item := &decisionmodel.Ticket{
		ID:          "ticket-runtime-recovery-context",
		WorkspaceID: "ws-runtime-recovery-context",
		RunID:       "run-runtime-recovery-context",
		Reason:      "runtime repair exhausted",
		State:       common.DecisionTicketStateOpen,
		Context: mustProjectionPayload(t, map[string]any{
			"failureClass":           "runtime_failed",
			"failureSignature":       "class=runtime_failed|kind=llm/workspace_execution",
			"allowedRecoveryActions": []any{"replan", "terminate"},
		}),
	}
	got := mapDecision(item, nil, nil)
	if got.FailureClass != "runtime_failed" ||
		got.FailureSignature != "class=runtime_failed|kind=llm/workspace_execution" ||
		len(got.AllowedRecoveryActions) != 2 ||
		got.AllowedRecoveryActions[0] != "replan" ||
		got.AllowedRecoveryActions[1] != "terminate" {
		t.Fatalf("fallback decision recovery fields = %+v", got)
	}
}

func TestMapDecisionProjectsRuntimeDefinitionClarificationWithoutLoss(t *testing.T) {
	binding := &decisionmodel.DefinitionClarificationBinding{
		Class: decisionmodel.DefinitionClarificationRuntime, OpenBindingHash: "open-binding", RequiredActionKind: "revise_definition_and_restart",
		ViolationOrGapCode: "implicit_one_of", CanonicalSignatureVersion: "gap-v1", CanonicalSignature: "safe-signature", CanonicalSignatureHash: "signature-hash",
		OffendingRefs: []string{"dod:item-1", "goal:goal-1"}, DefinitionRevision: "definition-old", StartupInputHash: "startup-old",
		AuthoritySnapshotID: "authority-1", ClarificationDispatchID: "clarification-dispatch-1", FocusRef: "focus-1",
		LogicalRequestID: "logical-1", SemanticDocumentHash: "document-1",
		ResolutionID: "resolution-1", ResolutionBindingHash: "resolution-binding-hash",
		ReplacementIntentID: "replacement-1", ReplacementIntentHash: "replacement-hash",
		SuccessorDefinitionRevision: "definition-new", SuccessorStartupInputHash: "startup-new",
	}
	got := mapDecision(&decisionmodel.Ticket{ID: "ticket-definition", DefinitionClarification: binding}, nil, nil)
	if got.DefinitionClarification == nil {
		t.Fatal("definition clarification projection is nil")
	}
	projected := got.DefinitionClarification
	want := &projectioncontract.DefinitionClarification{
		Class: string(decisionmodel.DefinitionClarificationRuntime), BindingState: "replacement_intent_bound",
		OpenBindingHash: "open-binding", RequiredAction: "revise_definition_and_restart",
		ViolationOrGapCode: "implicit_one_of", CanonicalSignatureVersion: "gap-v1",
		CanonicalSignature: "safe-signature", CanonicalSignatureHash: "signature-hash",
		OffendingRefs: []string{"dod:item-1", "goal:goal-1"}, DefinitionRevision: "definition-old", StartupInputHash: "startup-old",
		AuthoritySnapshotID: "authority-1", ClarificationDispatchID: "clarification-dispatch-1", FocusRef: "focus-1",
		LogicalRequestID: "logical-1", SemanticDocumentHash: "document-1",
		ResolutionID: "resolution-1", ResolutionBindingHash: "resolution-binding-hash",
		ReplacementIntentID: "replacement-1", ReplacementIntentHash: "replacement-hash",
		SuccessorDefinitionRevision: "definition-new", SuccessorStartupInputHash: "startup-new",
	}
	if !reflect.DeepEqual(projected, want) {
		t.Fatalf("definition clarification projection = %+v, want %+v", projected, want)
	}
	projected.OffendingRefs[0] = "mutated"
	if binding.OffendingRefs[0] != "dod:item-1" {
		t.Fatal("definition clarification mapping retained Decision-owner mutable refs")
	}
	view := projectDecisionView(got)
	view.DefinitionClarification.OffendingRefs[0] = "view-mutated"
	if got.DefinitionClarification.OffendingRefs[0] != "mutated" {
		t.Fatal("public Decision view retained projection mutable refs")
	}
}

func TestMapDecisionProjectsPreSealDefinitionClarificationBindingState(t *testing.T) {
	got := mapDecision(&decisionmodel.Ticket{
		ID: "ticket-pre-seal",
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{
			Class:                 decisionmodel.DefinitionClarificationWorkspacePreSeal,
			RequiredActionKind:    "revise_definition",
			CandidateID:           "candidate-1",
			CandidateRevision:     3,
			CandidateHash:         "candidate-hash",
			ViolationOrGapCode:    "implicit_one_of",
			OffendingRefs:         []string{"dod:item-1"},
			ResolutionID:          "resolution-1",
			ResolutionBindingHash: "resolution-binding-hash",
			DefinitionRevision:    "definition-new",
			StartupInputHash:      "startup-new",
		},
	}, nil, nil)
	binding := got.DefinitionClarification
	if binding == nil || binding.Class != string(decisionmodel.DefinitionClarificationWorkspacePreSeal) ||
		binding.BindingState != "admission_bound" || binding.CandidateID != "candidate-1" ||
		binding.CandidateRevision != 3 || binding.CandidateHash != "candidate-hash" ||
		binding.RequiredAction != "revise_definition" || binding.DefinitionRevision != "definition-new" ||
		binding.StartupInputHash != "startup-new" || binding.ResolutionBindingHash != "resolution-binding-hash" ||
		!reflect.DeepEqual(binding.OffendingRefs, []string{"dod:item-1"}) {
		t.Fatalf("pre-seal definition clarification projection = %+v", binding)
	}
}
