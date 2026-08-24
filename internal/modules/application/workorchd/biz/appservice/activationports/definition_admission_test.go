package activationports

import (
	"context"
	"errors"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	definitionadmission "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBuiltinSoftwareDevProfilePassesDefinitionAdmissionBeforeRun(t *testing.T) {
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatalf("load builtin templates: %v", err)
	}
	var builtin *workspacemodel.WorkspaceTemplate
	for _, item := range templates {
		if item.ID != "software-dev" {
			continue
		}
		hints, err := workorcheventpayload.FromMap(item.ContextHints)
		if err != nil {
			t.Fatalf("software-dev context hints: %v", err)
		}
		builtin = &workspacemodel.WorkspaceTemplate{
			ID: item.ID, Name: item.Name, Description: item.Description,
			GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...),
			DefaultSoulID: item.DefaultSoulID, SoulBindings: item.SoulBindings, ContextHints: hints,
		}
		break
	}
	if builtin == nil {
		t.Fatal("software-dev builtin template is missing")
	}
	workspace := &workspacemodel.Workspace{ID: "workspace-builtin-software-dev", DeliverableDirectory: "/workspace", TemplateID: builtin.ID}
	goal := &workspacemodel.Goal{ID: "goal-builtin", WorkspaceID: workspace.ID, Text: "implement the requested behavior", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-builtin", WorkspaceID: workspace.ID, Items: []string{"requested behavior is validated"}, DefinitionRevision: "dod-r1"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, builtin)
	if err != nil {
		t.Fatalf("compile builtin candidate inputs: %v", err)
	}
	result, err := definitionadmission.Compile(candidate)
	if err != nil || result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("builtin software-dev admission = %+v err=%v", result, err)
	}
}

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
			"targetSelectorRef": "file/workspace_deliverable", "targetSelectorOwnerRef": "software-dev", "targetSelectorSubjectRef": "validation_preparation",
			"acceptedStepKinds":  []any{"file/write", "validation/managed_session"},
			"requiredOwnerFacts": []any{map[string]any{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session"}},
			"ownerFactConvergences": []any{map[string]any{
				"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session",
				"missingTrigger": "owner_fact_missing", "acquisitionOwnerModuleId": "blocks/file", "acquisitionOperationKind": "file/write",
				"acquisitionTargetPolicy": "prospective_owner_target", "acquisitionTargetSelectorRef": "workspace-relative-new-file-target",
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
	result, err := definitionadmission.Compile(candidate)
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
	blocked, err := definitionadmission.Compile(missing)
	if err != nil || blocked.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || blocked.ViolationCode != workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved {
		t.Fatalf("missing profile closure result = %+v err=%v", blocked, err)
	}
	if command := workspaceClarificationCommand(missing, blocked); command.RequiredActionKind != "select_or_fix_template_profile" {
		t.Fatalf("profile clarification action = %q", command.RequiredActionKind)
	}
}

func TestCurrentEligibleDefinitionAdmissionRejectsIncompleteCurrentConvergenceWithoutReauthoring(t *testing.T) {
	profile := map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{
		"evidenceRequirements": []any{map[string]any{
			"id": "validation_preparation", "scope": "deliverable",
			"targetSelectorRef": "file/workspace_deliverable", "targetSelectorOwnerRef": "software-dev", "targetSelectorSubjectRef": "validation_preparation",
			"acceptedStepKinds":  []any{"file/write", "validation/managed_session"},
			"requiredOwnerFacts": []any{map[string]any{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session"}},
			"ownerFactConvergences": []any{map[string]any{
				"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session",
				"missingTrigger": "owner_fact_missing", "acquisitionOwnerModuleId": "blocks/file", "acquisitionOperationKind": "file/write",
				"acquisitionTargetPolicy": "prospective_owner_target", "acquisitionTargetSelectorRef": "workspace-relative-new-file-target",
				"targetSelectorRef": "file/workspace_deliverable", "targetSelectorOwnerRef": "software-dev", "targetSelectorSubjectRef": "validation_preparation",
				"reobserveOwnerModuleId": "blocks/shell", "reobserveFactKind": "selectable_validation_surface", "maxAcquisitionAttempts": 1,
			}},
		}},
	}}}
	hints, err := workorcheventpayload.FromMap(profile)
	if err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{ID: "workspace-strict-current", DeliverableDirectory: "/workspace", TemplateID: "software-dev"}
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "software dev", ContextHints: hints}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	// This reproduces persistence authored before acquisition target policy and
	// operation selector became mandatory. Rehashing makes the shape internally
	// self-consistent, so only current static admission can reject it.
	candidate.OwnerFactConvergenceBindings[0].Convergence.AcquisitionTargetPolicy = ""
	candidate.OwnerFactConvergenceBindings[0].Convergence.AcquisitionTargetSelectorRef = ""
	candidate = candidate.WithHash()
	startup := startupInputForActivationPortTest(candidate)
	admission := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	workspace.DefinitionAdmissionCandidates = []workspacemodel.DefinitionAdmissionCandidate{candidate}
	workspace.StartupInput = &startup
	workspace.ActivationAdmission = &admission

	before := workspace.DefinitionAdmissionCandidates[0].CandidateHash
	err = validateCurrentWorkspaceDefinitionAdmission(workspace)
	if err == nil || workspace.DefinitionAdmissionCandidates[0].CandidateHash != before {
		t.Fatalf("incomplete current convergence err=%v candidateHash=%q", err, workspace.DefinitionAdmissionCandidates[0].CandidateHash)
	}
	want := invalidCurrentWorkspaceStartupInput(err)
	if !errors.Is(want, common.ErrConflict) || !strings.Contains(want.Error(), "recreate the Workspace from the current Template") {
		t.Fatalf("strict current-shape error = %v", want)
	}
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	_, ensureErr := EnsureWorkspaceDefinitionAdmission(context.Background(), hub, workspace, goal, dod)
	if !errors.Is(ensureErr, common.ErrConflict) || !strings.Contains(ensureErr.Error(), "recreate the Workspace from the current Template") {
		t.Fatalf("Restart pre-seal current-shape error = %v", ensureErr)
	}
}

func TestCurrentDefinitionAdmissionMatchesOnlyExactOwnerInputSnapshot(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-exact", DeliverableDirectory: "/workspace", TemplateID: "template-exact"}
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "exact", Description: "sealed"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	startup := startupInputForActivationPortTest(candidate)
	workspace.DefinitionAdmissionCandidates = []workspacemodel.DefinitionAdmissionCandidate{candidate}
	workspace.StartupInput = &startup
	workspace.ActivationAdmission = &workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: workspace.ID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	matchedCandidate, matches, err := currentDefinitionAdmissionMatchesOwnerInputs(workspace, goal, dod, template, startup.DefinitionRevision)
	if err != nil || !matches || !workspaceDefinitionAdmissionEligible(workspace, matchedCandidate) {
		t.Fatalf("exact owner input did not reuse admission: matches=%t err=%v candidate=%+v", matches, err, matchedCandidate)
	}

	driftedTemplate := *template
	driftedTemplate.Description = "new live template revision"
	_, matches, err = currentDefinitionAdmissionMatchesOwnerInputs(workspace, goal, dod, &driftedTemplate, startup.DefinitionRevision)
	if err != nil || matches {
		t.Fatalf("live Template drift reused stale admission: matches=%t err=%v", matches, err)
	}

	driftedWorkspace := *workspace
	driftedWorkspace.DeliverableDirectory = "/workspace-v2"
	_, matches, err = currentDefinitionAdmissionMatchesOwnerInputs(&driftedWorkspace, goal, dod, template, startup.DefinitionRevision)
	if err != nil || matches {
		t.Fatalf("resolved directory drift reused stale admission: matches=%t err=%v", matches, err)
	}
}

func TestRuntimeBootstrapStartupSealRejectsTemplateAndProfileDrift(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-seal", DeliverableDirectory: "/workspace", TemplateID: "template-seal"}
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "sealed"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	startup := startupInputForActivationPortTest(candidate)
	workspace.DefinitionAdmissionCandidates = []workspacemodel.DefinitionAdmissionCandidate{candidate}
	workspace.StartupInput = &startup
	workspace.ActivationAdmission = &workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: workspace.ID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	bootstrap := &runtimecontract.RuntimeBootstrap{
		Definition: runtimecontract.RuntimeDefinitionContext{Revision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash},
		Activation: runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{}},
	}
	if err := validateRuntimeBootstrapStartupSeal(workspace, template, candidate, bootstrap); err != nil {
		t.Fatalf("exact startup seal rejected: %v", err)
	}

	driftedTemplate := *template
	driftedTemplate.Description = "drifted"
	if err := validateRuntimeBootstrapStartupSeal(workspace, &driftedTemplate, candidate, bootstrap); err == nil {
		t.Fatal("Template drift was accepted by final bootstrap gate")
	}
	driftedBootstrap := *bootstrap
	driftedBootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "drifted-profile"}}
	if err := validateRuntimeBootstrapStartupSeal(workspace, template, candidate, &driftedBootstrap); err == nil {
		t.Fatal("Runtime profile drift was accepted by final bootstrap gate")
	}
}

func startupInputForActivationPortTest(candidate workspacemodel.DefinitionAdmissionCandidate) workspacemodel.WorkspaceStartupInputContract {
	mappings := []workspacemodel.CandidateItemToItemRefMapping{{CandidateItemRef: candidate.GoalInput.CandidateItemRef, ItemRef: "goal-item"}}
	dodRefs := make([]string, 0, len(candidate.DoDInputs))
	dodTexts := make([]string, 0, len(candidate.DoDInputs))
	for _, item := range candidate.DoDInputs {
		ref := "dod-item-" + item.CandidateItemRef
		mappings = append(mappings, workspacemodel.CandidateItemToItemRefMapping{CandidateItemRef: item.CandidateItemRef, ItemRef: ref})
		dodRefs = append(dodRefs, ref)
		dodTexts = append(dodTexts, item.Text)
	}
	return (workspacemodel.WorkspaceStartupInputContract{
		WorkspaceID: candidate.WorkspaceID, DefinitionRevision: candidate.GoalInput.SourceIdentity + "|" + candidate.DoDInputs[0].SourceIdentity,
		GoalDefinitionRef: "goal-item", GoalTextHash: workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		DoDDefinitionRefs: dodRefs, DoDItemSetHash: workspacemodel.StableDefinitionStringSetHash(dodTexts),
		WorkspaceTemplateRef: candidate.WorkspaceTemplateSnapshot.TemplateRef, TemplateRevision: candidate.WorkspaceTemplateSnapshot.Revision,
		TemplateSnapshotHash: candidate.WorkspaceTemplateSnapshot.SnapshotHash, RuntimeProfileContractsHash: candidate.RuntimeProfileContractsHash,
		ResolvedProjectWorkspaceDirectory: candidate.ResolvedProjectWorkspaceDirectory, CandidateItemToItemRefMappings: mappings,
	}).WithHash()
}
