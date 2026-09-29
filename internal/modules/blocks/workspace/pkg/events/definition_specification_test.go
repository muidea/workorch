package events

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestSpecificationRequestResultRejectsReplayPermitAndBudgetDrift(t *testing.T) {
	identity := workspace.DefinitionSpecificationRequestIdentity{
		WorkspaceID: "ws", CandidateID: "candidate", CandidateRevision: 1,
		CandidateHash: "candidate-hash", CompilationInputHash: "input-hash",
		Phase: workspace.DefinitionSpecificationProposal, LogicalRequestID: "proposal-1",
	}
	valid := DefinitionSpecificationRequestResult{Identity: identity, Disposition: DefinitionSpecificationPermitted,
		PermitID: identity.PermitID(), Consumed: 1, Remaining: workspace.DefinitionSpecificationRequestLimit - 1}
	if err := valid.ValidateFor(identity); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*DefinitionSpecificationRequestResult)
	}{
		{"cross workspace", func(r *DefinitionSpecificationRequestResult) { r.Identity.WorkspaceID = "other" }},
		{"wrong permit", func(r *DefinitionSpecificationRequestResult) { r.PermitID = "other" }},
		{"budget drift", func(r *DefinitionSpecificationRequestResult) { r.Remaining++ }},
		{"zero consumption", func(r *DefinitionSpecificationRequestResult) { r.Consumed = 0 }},
		{"reserved grants permit", func(r *DefinitionSpecificationRequestResult) { r.Disposition = DefinitionSpecificationAlreadyReserved }},
		{"completed lacks result", func(r *DefinitionSpecificationRequestResult) {
			r.Disposition = DefinitionSpecificationAlreadyCompleted
			r.PermitID = ""
		}},
		{"unknown outcome", func(r *DefinitionSpecificationRequestResult) { r.Disposition = "unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := valid.Clone()
			tc.mutate(&got)
			if got.ValidateFor(identity) == nil {
				t.Fatal("invalid owner result accepted")
			}
		})
	}
	replay := valid.Clone()
	replay.Disposition, replay.PermitID = DefinitionSpecificationAlreadyReserved, ""
	if err := replay.ValidateFor(identity); err != nil {
		t.Fatal(err)
	}
	replay.Disposition = DefinitionSpecificationAlreadyCompleted
	replay.Completed = &workspace.DefinitionSpecificationRequestCompletion{FailureCode: "provider_failed"}
	if err := replay.ValidateFor(identity); err != nil {
		t.Fatal(err)
	}
}
