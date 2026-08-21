package activationports

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestLatestPriorInvalidDefinitionCandidateDoesNotReopenAcknowledgedLineage(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-1", DeliverableDirectory: "/workspace"}
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-revision-1"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-revision-1"}
	template := &workspacemodel.WorkspaceTemplate{ID: "template-1", Name: "template"}
	invalidBeforeEligible := definitionAdmissionCandidate(workspace, goal, dod, template)
	invalidBeforeEligible.ProposedRelationInputs[0].RelationKind = "one_of"
	invalidBeforeEligible = invalidBeforeEligible.WithHash()
	eligible := invalidBeforeEligible.Clone()
	eligible.CandidateRevision = 2
	eligible.ProposedRelationInputs[0].RelationKind = "all_of"
	eligible = eligible.WithHash()
	workspace.DefinitionAdmissionCandidates = []workspacemodel.DefinitionAdmissionCandidate{invalidBeforeEligible, eligible}
	workspace.ActivationAdmission = &workspacemodel.WorkspaceActivationAdmission{
		CandidateID: eligible.CandidateID, CandidateRevision: eligible.CandidateRevision,
		State: workspacemodel.WorkspaceActivationEligible,
	}
	current := eligible.Clone()
	current.CandidateRevision = 3
	current = current.WithHash()
	if _, _, ok := latestPriorInvalidDefinitionCandidate(workspace, current); ok {
		t.Fatal("invalid candidate before eligible baseline was reopened")
	}

	invalidAfterEligible := current.Clone()
	invalidAfterEligible.ProposedRelationInputs[0].RelationKind = "one_of"
	invalidAfterEligible = invalidAfterEligible.WithHash()
	workspace.DefinitionAdmissionCandidates = append(workspace.DefinitionAdmissionCandidates, invalidAfterEligible)
	corrected := invalidAfterEligible.Clone()
	corrected.CandidateRevision = 4
	corrected.ProposedRelationInputs[0].RelationKind = "all_of"
	corrected = corrected.WithHash()
	selected, _, ok := latestPriorInvalidDefinitionCandidate(workspace, corrected)
	if !ok || selected.CandidateRevision != invalidAfterEligible.CandidateRevision {
		t.Fatal("unresolved invalid predecessor was not selected")
	}
}
