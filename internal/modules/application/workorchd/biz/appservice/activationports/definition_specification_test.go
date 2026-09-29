package activationports

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestReviewedSpecificationIsNotSilentlyDroppedOnActivation(t *testing.T) {
	ws := &workspace.Workspace{ID: "ws-spec", DeliverableDirectory: "/workspace", TemplateID: "template-spec"}
	goal := &workspace.Goal{ID: "goal-spec", WorkspaceID: ws.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspace.DoD{ID: "dod-spec", WorkspaceID: ws.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspace.WorkspaceTemplate{ID: ws.TemplateID, Name: "spec"}
	candidate, err := definitionAdmissionCandidate(ws, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	// This test isolates candidate selection; semantic completeness is checked
	// by the Workspace admission tests, not by this authoring-input matcher.
	candidate.Specification = &workspace.GoalSpecification{Version: 1, GoalItemRef: candidate.GoalInput.CandidateItemRef, GoalTextHash: workspace.StableDefinitionTextHash(goal.Text)}
	candidate = candidate.WithHash()
	ws.DefinitionAdmissionCandidates = []workspace.DefinitionAdmissionCandidate{candidate}
	reused, err := definitionAdmissionCandidate(ws, goal, dod, template)
	if err != nil || reused.Specification == nil || reused.CandidateHash != candidate.CandidateHash {
		t.Fatalf("reviewed input was discarded: %+v, %v", reused, err)
	}
	// ledger 来源属于已审查候选身份，不属于待比较的原始创作输入。
	candidate.CompilationRef = &workspace.DefinitionSpecificationCompilationRef{
		InputCandidateHash: "input", CompilationInputHash: "compiled-input", ProposalPermitID: "proposal", ProposalHash: "proof",
		CriticPermitIDs: [6]string{"c1", "c2", "c3", "c4", "c5", "c6"},
	}
	candidate = candidate.WithHash()
	ws.DefinitionAdmissionCandidates = []workspace.DefinitionAdmissionCandidate{candidate}
	reused, err = definitionAdmissionCandidate(ws, goal, dod, template)
	if err != nil || reused.CompilationRef == nil || reused.CandidateHash != candidate.CandidateHash {
		t.Fatalf("compiled input was discarded: %+v, %v", reused, err)
	}
	changedGoal := *goal
	changedGoal.Text = "changed goal"
	changedGoal.DefinitionRevision = "goal-r2"
	if _, err := definitionAdmissionCandidate(ws, &changedGoal, dod, template); err == nil {
		t.Fatal("changed Goal silently bypassed specification review")
	}
	changedTemplate := *template
	changedTemplate.Name = "changed template"
	if _, err := definitionAdmissionCandidate(ws, goal, dod, &changedTemplate); err == nil {
		t.Fatal("changed Template silently bypassed specification review")
	}
	if ws.DefinitionAdmissionCandidates[0].CandidateHash != candidate.CandidateHash {
		t.Fatal("history mutated")
	}
}
