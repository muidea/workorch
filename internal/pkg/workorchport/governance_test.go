package workorchport

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestDecisionInfoFromProjectionPreservesCompleteContractAndDetachesMutableValues(t *testing.T) {
	input := projectioncontract.Decision{
		ID:                     "ticket-1",
		WorkspaceID:            "workspace-1",
		RunID:                  "run-1",
		StepID:                 "step-1",
		Reason:                 "reason",
		DecisionKind:           "recovery",
		Question:               "continue?",
		Context:                workorcheventpayload.New(workorcheventpayload.StringField("context", "original")),
		FailureClass:           "contract",
		FailureSignature:       "signature",
		AllowedRecoveryActions: []string{"resume", "restart"},
		State:                  "open",
		ResolutionID:           "resolution-1",
		ResolutionType:         "replan",
		ResolutionPayload:      workorcheventpayload.New(workorcheventpayload.StringField("resolution", "original")),
		ContinuationAction:     "resume",
		ResumeRunID:            "run-2",
		SuccessorRunID:         "run-3",
		AutoGovernance: &projectioncontract.AutoGovernanceSummary{
			TraceID:      "trace-1",
			Action:       "allow",
			Status:       "applied",
			EvidenceRefs: []string{"evidence-1"},
		},
		ResolvedAt: "resolved-at",
		ResolvedBy: "operator",
		CreatedAt:  "created-at",
		UpdatedAt:  "updated-at",
	}

	got := DecisionInfoFromProjection(input)
	if got.ID != common.TicketID(input.ID) || got.WorkspaceID != common.WorkspaceID(input.WorkspaceID) ||
		got.RunID != common.RunID(input.RunID) || got.StepID != common.StepID(input.StepID) ||
		got.ResumeRunID != common.RunID(input.ResumeRunID) || got.SuccessorRunID != common.RunID(input.SuccessorRunID) {
		t.Fatalf("typed identity mismatch: %+v", got)
	}
	if got.DecisionKind != input.DecisionKind || got.FailureClass != input.FailureClass ||
		got.FailureSignature != input.FailureSignature || got.ResolutionID != input.ResolutionID ||
		got.ResolvedAt != input.ResolvedAt || got.ResolvedBy != input.ResolvedBy ||
		got.CreatedAt != input.CreatedAt || got.UpdatedAt != input.UpdatedAt {
		t.Fatalf("complete decision fields were not preserved: %+v", got)
	}
	if !reflect.DeepEqual(got.Context, input.Context) || !reflect.DeepEqual(got.ResolutionPayload, input.ResolutionPayload) ||
		!reflect.DeepEqual(got.AllowedRecoveryActions, input.AllowedRecoveryActions) ||
		!reflect.DeepEqual(got.AutoGovernance, input.AutoGovernance) {
		t.Fatalf("nested decision values mismatch: %+v", got)
	}

	input.Context.Fields[0].Value.String = "mutated"
	input.ResolutionPayload.Fields[0].Value.String = "mutated"
	input.AllowedRecoveryActions[0] = "mutated"
	input.AutoGovernance.EvidenceRefs[0] = "mutated"
	if value, _ := got.Context.String("context"); value != "original" {
		t.Fatalf("context shares mutable storage: %q", value)
	}
	if value, _ := got.ResolutionPayload.String("resolution"); value != "original" {
		t.Fatalf("resolution payload shares mutable storage: %q", value)
	}
	if got.AllowedRecoveryActions[0] != "resume" || got.AutoGovernance.EvidenceRefs[0] != "evidence-1" {
		t.Fatalf("decision slices share mutable storage: %+v", got)
	}
}

func TestGovernanceProjectionPointerConversionPreservesNil(t *testing.T) {
	if got := DecisionInfoPtrFromProjection(nil); got != nil {
		t.Fatalf("nil decision projection = %+v, want nil", got)
	}
	if got := ApprovalInfoPtrFromProjection(nil); got != nil {
		t.Fatalf("nil approval projection = %+v, want nil", got)
	}
}

func TestApprovalInfoFromProjectionPreservesCompleteContractAndDetachesMutableValues(t *testing.T) {
	input := projectioncontract.Approval{
		ID:          "approval-1",
		WorkspaceID: "workspace-1",
		RunID:       "run-1",
		StepID:      "step-1",
		Reason:      "reason",
		Action:      "patch/apply",
		Context:     workorcheventpayload.New(workorcheventpayload.StringField("context", "original")),
		State:       "pending",
		AutoGovernance: &projectioncontract.AutoGovernanceSummary{
			TraceID:      "trace-1",
			Action:       "require",
			Status:       "pending",
			EvidenceRefs: []string{"evidence-1"},
		},
		ResolvedAt: "resolved-at",
		ResolvedBy: "operator",
		CreatedAt:  "created-at",
		UpdatedAt:  "updated-at",
	}

	got := ApprovalInfoFromProjection(input)
	if got.ID != common.ApprovalID(input.ID) || got.WorkspaceID != common.WorkspaceID(input.WorkspaceID) ||
		got.RunID != common.RunID(input.RunID) || got.StepID != common.StepID(input.StepID) {
		t.Fatalf("typed identity mismatch: %+v", got)
	}
	if got.Reason != input.Reason || got.Action != input.Action || got.State != input.State ||
		got.ResolvedAt != input.ResolvedAt || got.ResolvedBy != input.ResolvedBy ||
		got.CreatedAt != input.CreatedAt || got.UpdatedAt != input.UpdatedAt {
		t.Fatalf("complete approval fields were not preserved: %+v", got)
	}

	input.Context.Fields[0].Value.String = "mutated"
	input.AutoGovernance.EvidenceRefs[0] = "mutated"
	if value, _ := got.Context.String("context"); value != "original" {
		t.Fatalf("context shares mutable storage: %q", value)
	}
	if got.AutoGovernance.EvidenceRefs[0] != "evidence-1" {
		t.Fatalf("auto governance shares mutable storage: %+v", got.AutoGovernance)
	}
}
