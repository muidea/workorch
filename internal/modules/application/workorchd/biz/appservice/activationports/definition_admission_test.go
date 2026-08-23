package activationports

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionadmissioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestLatestPriorInvalidDefinitionCandidateDoesNotReopenAcknowledgedLineage(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-1", DeliverableDirectory: "/workspace"}
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-revision-1"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-revision-1"}
	template := &workspacemodel.WorkspaceTemplate{ID: "template-1", Name: "template"}
	invalidBeforeEligible, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatalf("definitionAdmissionCandidate: %v", err)
	}
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

func TestDefinitionAdmissionCandidateSealsProfileOwnerFactClosureBeforeRun(t *testing.T) {
	profile := map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{
		"evidenceRequirements": []any{map[string]any{
			"id": "validation_preparation", "scope": "deliverable",
			"acceptedStepKinds":  []any{"file/write", "validation/managed_session"},
			"requiredOwnerFacts": []any{map[string]any{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session"}},
			"ownerFactConvergences": []any{map[string]any{
				"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session",
				"missingTrigger": "owner_fact_missing", "acquisitionOwnerModuleId": "blocks/file", "acquisitionOperationKind": "file/write",
				"targetSelectorRef": "file/workspace_deliverable", "targetSelectorOwnerRef": "software-dev", "targetSelectorSubjectRef": "validation_preparation",
				"reobserveOwnerModuleId": "blocks/shell", "reobserveFactKind": "selectable_validation_surface", "maxAcquisitionAttempts": 1,
			}},
		}},
	}}}
	hints, err := workorcheventpayload.FromMap(profile)
	if err != nil {
		t.Fatalf("profile payload: %v", err)
	}
	workspace := &workspacemodel.Workspace{ID: "workspace-owner-fact", DeliverableDirectory: "/workspace", TemplateID: "template-owner-fact"}
	goal := &workspacemodel.Goal{ID: "goal-owner-fact", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-owner-fact", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "owner fact", ContextHints: hints}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatalf("definitionAdmissionCandidate: %v", err)
	}
	if len(candidate.AdmissionPolicyBindings) != 1 || len(candidate.RequiredOwnerFactBindings) != 1 || len(candidate.OwnerFactConvergenceBindings) != 1 {
		t.Fatalf("candidate profile closure = %+v", candidate)
	}
	result, err := definitionadmissioncore.Compile(candidate)
	if err != nil || result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("profile closure admission = %+v err=%v", result, err)
	}

	profileContracts := profile["profileContracts"].(map[string]any)
	profileBody := profileContracts["software-dev"].(map[string]any)
	evidence := profileBody["evidenceRequirements"].([]any)[0].(map[string]any)
	delete(evidence, "ownerFactConvergences")
	hintsWithoutClosure, err := workorcheventpayload.FromMap(profile)
	if err != nil {
		t.Fatalf("profile payload without closure: %v", err)
	}
	template.ContextHints = hintsWithoutClosure
	missing, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatalf("candidate without closure: %v", err)
	}
	blocked, err := definitionadmissioncore.Compile(missing)
	if err != nil || blocked.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || blocked.ViolationCode != workspacemodel.DefinitionAdmissionPrerequisiteUnresolved {
		t.Fatalf("missing profile closure result = %+v err=%v", blocked, err)
	}
}
