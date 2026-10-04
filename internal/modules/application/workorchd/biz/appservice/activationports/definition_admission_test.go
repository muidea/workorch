package activationports

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	activationcore "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	definitionadmission "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBuiltinSoftwareDevProfilePassesDefinitionAdmissionBeforeRun(t *testing.T) {
	builtin := builtinSoftwareDevTemplateForTest(t)
	workspace := &workspacemodel.Workspace{
		ID: "workspace-builtin-software-dev", DeliverableDirectory: "/workspace", TemplateID: builtin.ID,
		EvidencePolicies: workorcheventpayload.CloneSlice(builtin.EvidencePolicies),
	}
	goal := &workspacemodel.Goal{ID: "goal-builtin", WorkspaceID: workspace.ID, Text: "implement the requested behavior", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-builtin", WorkspaceID: workspace.ID, Items: []string{"requested behavior is validated"}, DefinitionRevision: "dod-r1"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, builtin)
	if err != nil {
		t.Fatalf("compile builtin candidate inputs: %v", err)
	}
	assertBuiltinSoftwareDevCandidateAndBootstrap(t, builtin, workspace, goal, dod, candidate)
}

func builtinSoftwareDevTemplateForTest(t *testing.T) *workspacemodel.WorkspaceTemplate {
	t.Helper()
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
		evidencePolicies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
		if err != nil {
			t.Fatalf("software-dev evidence policies: %v", err)
		}
		builtin = &workspacemodel.WorkspaceTemplate{
			ID: item.ID, Name: item.Name, Description: item.Description,
			GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...),
			DefaultSoulID: item.DefaultSoulID, SoulBindings: item.SoulBindings, ContextHints: hints, EvidencePolicies: evidencePolicies,
		}
		break
	}
	if builtin == nil {
		t.Fatal("software-dev builtin template is missing")
	}
	return builtin
}

func assertBuiltinSoftwareDevCandidateAndBootstrap(
	t *testing.T,
	builtin *workspacemodel.WorkspaceTemplate,
	workspace *workspacemodel.Workspace,
	goal *workspacemodel.Goal,
	dod *workspacemodel.DoD,
	candidate workspacemodel.DefinitionAdmissionCandidate,
) *runtimecontract.RuntimeBootstrap {
	t.Helper()
	var parsedRepair *runtimecontract.RuntimeValidationFailureRepairConvergence
	for _, profile := range candidate.RuntimeProfileContracts {
		if profile.ID != "software-dev" {
			continue
		}
		for _, requirement := range profile.EvidenceRequirements {
			if requirement.Scope == "dodItem" && requirement.ID == "*" {
				parsedRepair = requirement.ValidationFailureRepairConvergence
			}
		}
	}
	if parsedRepair == nil || len(parsedRepair.Routes) != 5 || !parsedRepair.RequireFreshValidation || parsedRepair.MaxAttempts != 2 {
		t.Fatalf("builtin software-dev repair convergence was not parsed into the admission candidate: %+v", parsedRepair)
	}
	addition, found := parsedRepair.RouteForTrigger(runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession, runtimecontract.ValidationDiagnosticOutputInconclusive, runtimecontract.ValidationEvidenceTargetValidationOutput, "validation_workspace_behavior_unobserved")
	if !found || !addition.RequireInputAddition || addition.RepairTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget || len(addition.RepairOperationKinds) != 1 || addition.RepairOperationKinds[0] != runtimecontract.StepKindFileWrite {
		t.Fatal("input addition route lost during admission")
	}
	inputRepair, found := parsedRepair.RouteForTrigger(runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession, runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetValidationInput, "validation_input_symbol_case_mismatch")
	if !found || !inputRepair.RequireInputRepair || inputRepair.RequireInputAddition || inputRepair.RepairTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope || len(inputRepair.RepairOperationKinds) != 1 || inputRepair.RepairOperationKinds[0] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatal("input repair exact-content gate lost during admission")
	}
	expectedRoutes := []struct {
		class      string
		targetType string
		ownerCode  string
	}{
		{runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface, "module_manifest_parse_failed"},
		{runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface, "source_compile_failed"},
		{runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationEvidenceTargetBehaviorEvidence, "test_failed"},
	}
	for _, expected := range expectedRoutes {
		route, found := parsedRepair.RouteForTrigger(
			runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession,
			expected.class, expected.targetType, expected.ownerCode,
		)
		if !found || route.RepairOwnerModuleID != runtimecontract.RuntimeModuleFile ||
			route.RepairTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope ||
			len(route.RepairOperationKinds) != 1 || route.RepairOperationKinds[0] != runtimecontract.StepKindFileEdit ||
			route.RepairOperationTargetSelectorRef != "workspace-relative-existing-file-target" {
			t.Fatalf("builtin software-dev exact repair route %q = %+v found=%v", expected.ownerCode, route, found)
		}
	}
	_, dependencyMisrouted := parsedRepair.RouteForTrigger(
		runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession,
		runtimecontract.ValidationDiagnosticDependencyMismatch, runtimecontract.ValidationEvidenceTargetDependencyContract,
		runtimecontract.ValidationDiagnosticDependencyMismatch,
	)
	_, genericBuildMisrouted := parsedRepair.RouteForTrigger(
		runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession,
		runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface, "shell_exit_nonzero",
	)
	_, genericTestMisrouted := parsedRepair.RouteForTrigger(
		runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession,
		runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationEvidenceTargetBehaviorEvidence, "shell_exit_nonzero",
	)
	if dependencyMisrouted || genericBuildMisrouted || genericTestMisrouted {
		t.Fatalf("builtin software-dev repair convergence did not preserve exact owner routes: %+v", parsedRepair)
	}
	result, err := definitionadmission.Compile(candidate)
	if err != nil || result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("builtin software-dev admission = %+v err=%v", result, err)
	}
	executionContract := runtimeExecutionContract(builtin.ContextHints.Map())
	executionContract.ProfileContracts = runtimecontract.NormalizeRuntimeProfileContracts(candidate.RuntimeProfileContracts)
	bootstrap, err := activationcore.RuntimeBootstrap(goal, dod, "definition-builtin-r1", "startup-input-builtin-r1", nil, runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "test-provider", ModelID: "test-model"}},
		ExecutionContract: executionContract,
	})
	if err != nil {
		t.Fatalf("compile admitted builtin software-dev bootstrap: %v", err)
	}
	compiledRepairFocuses := 0
	compiledDependencyMisroute := false
	for _, focus := range bootstrap.DefinitionFocuses {
		if focus.AssessmentScope != "dodItem" || focus.ValidationFailureRepairConvergence == nil {
			continue
		}
		compiledRepairFocuses++
		compiledRepairRoutes := 0
		for _, expected := range expectedRoutes {
			if route, found := focus.ValidationFailureRepairConvergence.RouteForTrigger(
				runtimecontract.RuntimeModuleShell,
				runtimecontract.RuntimeBlockKindManagedValidationSession,
				expected.class,
				expected.targetType,
				expected.ownerCode,
			); found && route.RepairOwnerModuleID == runtimecontract.RuntimeModuleFile {
				compiledRepairRoutes++
			}
		}
		if compiledRepairRoutes != len(expectedRoutes) {
			t.Fatalf("admitted software-dev focus lost its exact bounded repair routes: %+v", focus)
		}
		_, misrouted := focus.ValidationFailureRepairConvergence.RouteForTrigger(
			runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession,
			runtimecontract.ValidationDiagnosticDependencyMismatch, runtimecontract.ValidationEvidenceTargetDependencyContract,
			runtimecontract.ValidationDiagnosticDependencyMismatch,
		)
		compiledDependencyMisroute = compiledDependencyMisroute || misrouted
	}
	if compiledRepairFocuses != len(dod.Items) || compiledDependencyMisroute {
		t.Fatalf("admitted software-dev bootstrap lost its exact bounded repair routes: %+v", bootstrap.DefinitionFocuses)
	}
	if !containsActivationString(bootstrap.Definition.CapabilityContract, runtimecontract.StepKindFileEdit) ||
		!containsActivationString(bootstrap.Definition.CapabilityContract, runtimecontract.StepKindFileWrite) ||
		!containsActivationString(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindPatchApply) {
		t.Fatalf("repair operation missing from bootstrap capability contract: %+v", bootstrap.Definition.CapabilityContract)
	}
	capabilityFound := false
	admissibilityFound := false
	for _, requirement := range bootstrap.Requirements {
		if requirement.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
			(!containsActivationString(requirement.RequiredCapabilities, runtimecontract.StepKindFileEdit) &&
				!containsActivationString(requirement.RequiredCapabilities, runtimecontract.StepKindFileWrite) &&
				!containsActivationString(requirement.RequiredCapabilities, runtimecontract.RuntimeBlockKindPatchApply)) {
			continue
		}
		switch requirement.Kind {
		case runtimecontract.PrerequisiteKindCapability:
			capabilityFound = requirement.CapabilityClass == runtimecontract.RuntimeCapabilityClassStateChange
		case runtimecontract.PrerequisiteKindOperationAdmissibility:
			admissibilityFound = true
		}
	}
	if !capabilityFound || !admissibilityFound {
		t.Fatalf("repair operation did not compile into File startup prerequisites: %+v", bootstrap.Requirements)
	}
	return bootstrap
}

func containsActivationString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestBuiltinSoftwareDevBootstrapBindsEveryCompiledManagedCWI(t *testing.T) {
	builtin := builtinSoftwareDevTemplateForTest(t)
	workspace := &workspacemodel.Workspace{
		ID: "workspace-builtin-proof-set", DeliverableDirectory: "/workspace", TemplateID: builtin.ID,
		EvidencePolicies: workorcheventpayload.CloneSlice(builtin.EvidencePolicies),
	}
	goal := &workspacemodel.Goal{
		ID: "goal-builtin-proof-set", WorkspaceID: workspace.ID, Text: builtin.GoalText, DefinitionRevision: "goal-proof-set-r1",
	}
	dod := &workspacemodel.DoD{
		ID: "dod-builtin-proof-set", WorkspaceID: workspace.ID, Items: append([]string(nil), builtin.DoDItems...), DefinitionRevision: "dod-proof-set-r1",
	}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, builtin)
	if err != nil {
		t.Fatalf("compile builtin proof-set candidate: %v", err)
	}
	bootstrap := assertBuiltinSoftwareDevCandidateAndBootstrap(t, builtin, workspace, goal, dod, candidate)
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		bootstrap.Definition.Revision, bootstrap.DefinitionSeed.CoverageItems, bootstrap.DefinitionFocuses,
	)
	if err != nil {
		t.Fatalf("compile builtin exact proof requirements: %v", err)
	}
	managedFocuses := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeDefinitionFocus)
	for _, focus := range bootstrap.DefinitionFocuses {
		if containsActivationString(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) &&
			containsActivationString(focus.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession) {
			managedFocuses[focus.CoverageWorkItemIDs[0]] = focus
		}
	}
	// The current built-in Definition has one managed requirement per actual
	// DoD work unit plus validation_report. Optional acceptance dimensions and
	// goalCoverage are not invented as work units by this assertion.
	if len(requirements) != len(builtin.DoDItems)+1 || len(managedFocuses) != len(requirements) {
		t.Fatalf("builtin compiled managed proof set: requirements=%d managedFocuses=%d DoD=%d", len(requirements), len(managedFocuses), len(builtin.DoDItems))
	}
	seenScopes := make(map[string]int)
	seenHashes := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		focus, found := managedFocuses[requirement.CoverageWorkItemID]
		if !found {
			t.Fatalf("proof requirement %q has no exact managed focus: %+v", requirement.Hash, requirement)
		}
		seenScopes[requirement.SourceScope]++
		if requirement.ValidationRecipeID != "" || requirement.DeclarationRef != runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(requirement.SourceScope, requirement.SourceID) {
			t.Fatalf("builtin wildcard became recipe/identity authority: %+v", requirement)
		}
		if _, duplicate := seenHashes[requirement.Hash]; duplicate {
			t.Fatalf("builtin proof requirement hash reused across CWIs: %+v", requirement)
		}
		seenHashes[requirement.Hash] = struct{}{}
		ownerFactBound := false
		for _, fact := range focus.RequiredOwnerFacts {
			fact = fact.Normalized()
			if fact.OwnerModuleID == runtimecontract.RuntimeModuleShell &&
				fact.FactKind == runtimecontract.RuntimeOwnerFactSelectableValidationSurface &&
				fact.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession &&
				fact.ValidationProofRequirementHash == requirement.Hash {
				ownerFactBound = true
			}
		}
		convergenceBound := false
		for _, convergence := range focus.OwnerFactConvergences {
			convergence = convergence.Normalized()
			fact := convergence.Requirement
			if fact.OwnerModuleID == runtimecontract.RuntimeModuleShell &&
				fact.FactKind == runtimecontract.RuntimeOwnerFactSelectableValidationSurface &&
				fact.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession &&
				fact.ValidationProofRequirementHash == requirement.Hash &&
				convergence.AcquisitionOwnerModuleID == runtimecontract.RuntimeModuleFile &&
				convergence.AcquisitionOperationKind == runtimecontract.StepKindFileWrite &&
				convergence.ReobserveOwnerModuleID == runtimecontract.RuntimeModuleShell {
				convergenceBound = true
			}
		}
		if !ownerFactBound || !convergenceBound {
			t.Fatalf("builtin exact CWI has no Profile-owned proof preparation closure: focus=%+v requirement=%+v", focus, requirement)
		}
	}
	if seenScopes["dodItem"] != len(builtin.DoDItems) || seenScopes["deliverable"] != 1 {
		t.Fatalf("builtin actual managed proof scopes=%v, want all DoD plus validation_report", seenScopes)
	}
}

func TestBuiltinSoftwareDevExplicitAcceptanceDimensionGetsExactProofClosure(t *testing.T) {
	builtin := builtinSoftwareDevTemplateForTest(t)
	workspace := &workspacemodel.Workspace{ID: "workspace-builtin-dimension", DeliverableDirectory: "/workspace", TemplateID: builtin.ID}
	goal := &workspacemodel.Goal{ID: "goal-builtin-dimension", WorkspaceID: workspace.ID, Text: "deliver an executable entrypoint", DefinitionRevision: "goal-dimension-r1"}
	dod := &workspacemodel.DoD{ID: "dod-builtin-dimension", WorkspaceID: workspace.ID, Items: []string{"entrypoint is validated"}, DefinitionRevision: "dod-dimension-r1"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, builtin)
	if err != nil {
		t.Fatal(err)
	}
	profiles := runtimecontract.NormalizeRuntimeProfileContracts(candidate.RuntimeProfileContracts)
	foundDimension := false
	for profileIndex := range profiles {
		for dimensionIndex := range profiles[profileIndex].AcceptanceDimensions {
			dimension := &profiles[profileIndex].AcceptanceDimensions[dimensionIndex]
			if dimension.ID == "runtime_entrypoint" {
				dimension.Required = true
				foundDimension = true
			}
		}
		for requirementIndex := range profiles[profileIndex].EvidenceRequirements {
			requirement := &profiles[profileIndex].EvidenceRequirements[requirementIndex]
			if requirement.Scope == "acceptanceDimension" && requirement.ID == "runtime_entrypoint" {
				requirement.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			}
		}
	}
	if !foundDimension {
		t.Fatal("builtin software-dev profile has no runtime_entrypoint dimension")
	}
	executionContract := runtimeExecutionContract(builtin.ContextHints.Map())
	executionContract.ProfileContracts = runtimecontract.NormalizeRuntimeProfileContracts(profiles)
	bootstrap, err := activationcore.RuntimeBootstrap(goal, dod, "definition-builtin-dimension", "startup-input-builtin-dimension", nil, runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "test-provider", ModelID: "test-model"}},
		ExecutionContract: executionContract,
	})
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(bootstrap.Definition.Revision, bootstrap.DefinitionSeed.CoverageItems, bootstrap.DefinitionFocuses)
	if err != nil {
		t.Fatal(err)
	}
	for _, requirement := range requirements {
		if requirement.SourceScope != "acceptanceDimension" || requirement.SourceID != "runtime_entrypoint" {
			continue
		}
		for _, focus := range bootstrap.DefinitionFocuses {
			if len(focus.CoverageWorkItemIDs) != 1 || focus.CoverageWorkItemIDs[0] != requirement.CoverageWorkItemID {
				continue
			}
			for _, fact := range focus.RequiredOwnerFacts {
				if fact.Normalized().ValidationProofRequirementHash == requirement.Hash {
					return
				}
			}
		}
		t.Fatalf("acceptanceDimension requirement has no exact owner fact: %+v", requirement)
	}
	t.Fatal("explicit runtime_entrypoint dimension compiled no exact managed proof requirement")
}

func TestDefinitionAdmissionBlocksIncompleteValidationRepairContractBeforeRun(t *testing.T) {
	newInputs := func(profile map[string]any) (*workspacemodel.Workspace, *workspacemodel.Goal, *workspacemodel.DoD, *workspacemodel.WorkspaceTemplate) {
		hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"software-dev": profile}})
		if err != nil {
			t.Fatal(err)
		}
		workspace := &workspacemodel.Workspace{ID: "workspace-repair-admission", DeliverableDirectory: "/workspace", TemplateID: "software-dev"}
		goal := &workspacemodel.Goal{ID: "goal-repair-admission", WorkspaceID: workspace.ID, Text: "implement behavior", DefinitionRevision: "goal-r1"}
		dod := &workspacemodel.DoD{ID: "dod-repair-admission", WorkspaceID: workspace.ID, Items: []string{"behavior passes validation"}, DefinitionRevision: "dod-r1"}
		template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "software dev", ContextHints: hints}
		return workspace, goal, dod, template
	}
	newProfile := func() map[string]any {
		return map[string]any{"evidenceRequirements": []any{map[string]any{
			"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"},
			"validationFailureRepairConvergence": map[string]any{
				"id": "dod-validation-repair", "routes": []any{map[string]any{
					"id": "build-failure-to-file", "triggerOwnerModuleId": "blocks/shell", "triggerOperationKind": "validation/managed_session",
					"triggerDiagnosticClass": "validation_build_failed", "triggerEvidenceTargetType": "failure_surface", "triggerOwnerDiagnosticCode": "shell_exit_nonzero",
					"repairEvidenceKind": "state_change", "repairOwnerModuleId": "blocks/file", "repairOperationKinds": []any{"file/write", "patch/apply"},
					"repairTargetPolicy": "prospective_owner_target", "repairTargetRole": "deliverable_implementation",
					"repairDeclarationTargetSelectorRef": "file/workspace_deliverable", "repairDeclarationTargetSelectorOwnerRef": "software-dev",
					"repairDeclarationTargetSelectorSubjectRef": "dodItem_repair", "repairOperationTargetSelectorRef": "workspace-relative-new-file-target",
				}},
				"requireFreshValidation": true, "maxAttempts": 2,
			},
		}}}
	}
	validWorkspace, validGoal, validDoD, validTemplate := newInputs(newProfile())
	validCandidate, err := definitionAdmissionCandidate(validWorkspace, validGoal, validDoD, validTemplate)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := definitionadmission.Compile(validCandidate)
	if err != nil || admitted.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("valid repair admission = %+v err=%v", admitted, err)
	}

	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{name: "missing diagnostic class", edit: func(v map[string]any) { delete(v["routes"].([]any)[0].(map[string]any), "triggerDiagnosticClass") }},
		{name: "unknown diagnostic class", edit: func(v map[string]any) {
			v["routes"].([]any)[0].(map[string]any)["triggerDiagnosticClass"] = "free_form_failure"
		}},
		{name: "non validation trigger", edit: func(v map[string]any) {
			route := v["routes"].([]any)[0].(map[string]any)
			route["triggerOwnerModuleId"] = "blocks/file"
			route["triggerOperationKind"] = "file/write"
		}},
		{name: "non state change repair", edit: func(v map[string]any) { v["routes"].([]any)[0].(map[string]any)["repairEvidenceKind"] = "validation" }},
		{name: "cross owner repair kind", edit: func(v map[string]any) {
			v["routes"].([]any)[0].(map[string]any)["repairOperationKinds"] = []any{"shell/command"}
		}},
		{name: "non selector backed policy", edit: func(v map[string]any) { v["routes"].([]any)[0].(map[string]any)["repairTargetPolicy"] = "exact_target" }},
		{name: "missing operation selector", edit: func(v map[string]any) {
			delete(v["routes"].([]any)[0].(map[string]any), "repairOperationTargetSelectorRef")
		}},
		{name: "aliased declaration and operation selectors", edit: func(v map[string]any) {
			route := v["routes"].([]any)[0].(map[string]any)
			route["repairOperationTargetSelectorRef"] = route["repairDeclarationTargetSelectorRef"]
		}},
		{name: "fresh validation disabled", edit: func(v map[string]any) { v["requireFreshValidation"] = false }},
		{name: "attempt budget zero", edit: func(v map[string]any) { v["maxAttempts"] = 0 }},
		{name: "attempt budget overflow", edit: func(v map[string]any) { v["maxAttempts"] = runtimecontract.MaxRepairAttemptsPerFocusLimit + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := newProfile()
			requirement := profile["evidenceRequirements"].([]any)[0].(map[string]any)
			repair := requirement["validationFailureRepairConvergence"].(map[string]any)
			tc.edit(repair)
			workspace, goal, dod, template := newInputs(profile)
			candidate, candidateErr := definitionAdmissionCandidate(workspace, goal, dod, template)
			if candidateErr != nil {
				t.Fatalf("candidate authoring err=%v", candidateErr)
			}
			blocked, compileErr := definitionadmission.Compile(candidate)
			if compileErr != nil || blocked.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || blocked.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid {
				t.Fatalf("incomplete repair was not blocked before Run: result=%+v err=%v", blocked, compileErr)
			}
		})
	}

	conflictingProfile := newProfile()
	wildcard := conflictingProfile["evidenceRequirements"].([]any)[0].(map[string]any)
	exactRepair := make(map[string]any, len(wildcard["validationFailureRepairConvergence"].(map[string]any)))
	for key, value := range wildcard["validationFailureRepairConvergence"].(map[string]any) {
		exactRepair[key] = value
	}
	exactRepair["maxAttempts"] = 1
	conflictingProfile["evidenceRequirements"] = append(conflictingProfile["evidenceRequirements"].([]any), map[string]any{
		"id": "dod-001", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"},
		"validationFailureRepairConvergence": exactRepair,
	})
	workspace, goal, dod, template := newInputs(conflictingProfile)
	candidate, candidateErr := definitionAdmissionCandidate(workspace, goal, dod, template)
	if candidateErr != nil {
		t.Fatalf("conflicting candidate authoring err=%v", candidateErr)
	}
	blocked, compileErr := definitionadmission.Compile(candidate)
	if compileErr != nil || blocked.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || blocked.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid {
		t.Fatalf("conflicting exact/wildcard repair declarations were not blocked before Run: result=%+v err=%v", blocked, compileErr)
	}
}

func TestDefinitionAdmissionRejectsValidationRepairNonObjectAlternativeAndDuplicateShapesBeforeRuntimeWork(t *testing.T) {
	validRepair := func() map[string]any {
		return map[string]any{
			"id": "dod-validation-repair", "routes": []any{map[string]any{
				"id": "test-failure-to-file", "triggerOwnerModuleId": "blocks/shell", "triggerOperationKind": "validation/managed_session",
				"triggerDiagnosticClass": "validation_test_failed", "triggerEvidenceTargetType": "behavior_evidence", "triggerOwnerDiagnosticCode": "shell_exit_nonzero",
				"repairEvidenceKind": "state_change", "repairOwnerModuleId": "blocks/file", "repairOperationKinds": []any{"file/write"},
				"repairTargetPolicy": "prospective_owner_target", "repairTargetRole": "deliverable_implementation",
				"repairDeclarationTargetSelectorRef": "file/workspace_deliverable", "repairDeclarationTargetSelectorOwnerRef": "software-dev",
				"repairDeclarationTargetSelectorSubjectRef": "dodItem_repair", "repairOperationTargetSelectorRef": "workspace-relative-new-file-target",
			}},
			"requireFreshValidation": true, "maxAttempts": 2,
		}
	}
	cloneMap := func(input map[string]any) map[string]any {
		ret := make(map[string]any, len(input))
		for key, value := range input {
			ret[key] = value
		}
		return ret
	}
	compile := func(t *testing.T, requirements []any) workspacemodel.DefinitionAdmissionResult {
		t.Helper()
		hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{
			"evidenceRequirements": requirements,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		workspace := &workspacemodel.Workspace{ID: "workspace-strict-repair-admission", DeliverableDirectory: "/workspace", TemplateID: "software-dev"}
		goal := &workspacemodel.Goal{ID: "goal-strict-repair-admission", WorkspaceID: workspace.ID, Text: "implement behavior", DefinitionRevision: "goal-r1"}
		dod := &workspacemodel.DoD{ID: "dod-strict-repair-admission", WorkspaceID: workspace.ID, Items: []string{"behavior passes validation"}, DefinitionRevision: "dod-r1"}
		template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "software dev", ContextHints: hints}
		candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
		if err != nil {
			t.Fatalf("candidate authoring must preserve malformed current input for typed admission: %v", err)
		}
		result, err := definitionadmission.Compile(candidate)
		if err != nil {
			t.Fatalf("compile malformed candidate: %v", err)
		}
		return result
	}
	newRequirement := func(repair any) map[string]any {
		return map[string]any{
			"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"},
			"validationFailureRepairConvergence": repair,
		}
	}
	withUnknown := func(key string, value any) map[string]any {
		repair := cloneMap(validRepair())
		repair[key] = value
		return repair
	}
	withRouteValue := func(key string, value any) map[string]any {
		repair := validRepair()
		repair["routes"].([]any)[0].(map[string]any)[key] = value
		return repair
	}
	withAdditionalRoute := func(route any) map[string]any {
		repair := validRepair()
		repair["routes"] = append(repair["routes"].([]any), route)
		return repair
	}
	duplicateRouteIdentity := func() map[string]any {
		repair := validRepair()
		route := cloneMap(repair["routes"].([]any)[0].(map[string]any))
		route["triggerDiagnosticClass"] = "validation_build_failed"
		route["triggerEvidenceTargetType"] = "failure_surface"
		repair["routes"] = append(repair["routes"].([]any), route)
		return repair
	}

	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "array", value: []any{validRepair()}},
		{name: "scalar", value: "validation-repair"},
		{name: "null", value: nil},
		{name: "empty object", value: map[string]any{}},
		{name: "unknown key", value: withUnknown("unknownField", true)},
		{name: "oneOf relation", value: withUnknown("oneOf", []any{validRepair()})},
		{name: "anyOf relation", value: withUnknown("anyOf", []any{validRepair()})},
		{name: "alternative relation", value: withUnknown("alternative", validRepair())},
		{name: "legacy alias", value: withUnknown("repairTargetSelectorRef", "workspace-relative-new-file-target")},
		{name: "fractional attempt budget", value: withUnknown("maxAttempts", 1.5)},
		{name: "mixed null route", value: withAdditionalRoute(nil)},
		{name: "mixed scalar route", value: withAdditionalRoute("build-route")},
		{name: "duplicate route identity", value: duplicateRouteIdentity()},
		{name: "diagnostic class array", value: withRouteValue("triggerDiagnosticClass", []any{"validation_test_failed", 1})},
		{name: "scalar optional selector candidates", value: withRouteValue("repairDeclarationTargetSelectorCandidateRefs", "src/main.go")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := compile(t, []any{newRequirement(tc.value)})
			if result.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || result.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid {
				t.Fatalf("invalid repair shape escaped pre-seal admission: %+v", result)
			}
		})
	}

	t.Run("duplicate normalized scope and id", func(t *testing.T) {
		first := newRequirement(validRepair())
		second := newRequirement(validRepair())
		second["id"] = " * "
		second["scope"] = " dodItem "
		result := compile(t, []any{first, second})
		if result.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || result.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid {
			t.Fatalf("duplicate scope+id escaped pre-seal admission through last-wins normalization: %+v", result)
		}
	})
}

func TestDefinitionAdmissionSealsValidationRepairBudgetIntoStartupInputHash(t *testing.T) {
	profile := func(maxAttempts int) map[string]any {
		return map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{"evidenceRequirements": []any{map[string]any{
			"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"},
			"validationFailureRepairConvergence": map[string]any{
				"id": "repair", "routes": []any{map[string]any{
					"id": "test-failure-to-file", "triggerOwnerModuleId": "blocks/shell", "triggerOperationKind": "validation/managed_session",
					"triggerDiagnosticClass": "validation_test_failed", "triggerEvidenceTargetType": "behavior_evidence", "triggerOwnerDiagnosticCode": "shell_exit_nonzero",
					"repairEvidenceKind": "state_change", "repairOwnerModuleId": "blocks/file", "repairOperationKinds": []any{"file/write"},
					"repairTargetPolicy": "prospective_owner_target", "repairTargetRole": "implementation",
					"repairDeclarationTargetSelectorRef": "file/workspace_deliverable", "repairDeclarationTargetSelectorOwnerRef": "software-dev", "repairDeclarationTargetSelectorSubjectRef": "dodItem_repair",
					"repairOperationTargetSelectorRef": "workspace-relative-new-file-target",
				}},
				"requireFreshValidation": true, "maxAttempts": maxAttempts,
			},
		}}}}}
	}
	compile := func(maxAttempts int) workspacemodel.WorkspaceStartupInputContract {
		hints, err := workorcheventpayload.FromMap(profile(maxAttempts))
		if err != nil {
			t.Fatal(err)
		}
		workspace := &workspacemodel.Workspace{ID: "workspace-repair-hash", DeliverableDirectory: "/workspace", TemplateID: "software-dev"}
		goal := &workspacemodel.Goal{ID: "goal-repair-hash", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
		dod := &workspacemodel.DoD{ID: "dod-repair-hash", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
		template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "software dev", ContextHints: hints}
		candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
		if err != nil {
			t.Fatal(err)
		}
		result, err := definitionadmission.Compile(candidate)
		if err != nil || result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
			t.Fatalf("repair hash admission = %+v err=%v", result, err)
		}
		return startupInputForActivationPortTest(candidate)
	}
	first, second := compile(1), compile(2)
	if first.StartupInputHash == second.StartupInputHash {
		t.Fatal("validation repair budget drift did not change StartupInputHash")
	}
}

func TestDefinitionAdmissionRejectsRemovedPresealExactTargetField(t *testing.T) {
	hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{
		"evidenceRequirements": []any{map[string]any{
			"id": "source_change", "scope": "deliverable", "prospectiveTargetRef": "legacy-target.txt",
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{ID: "workspace-legacy-target", DeliverableDirectory: "/workspace", TemplateID: "software-dev"}
	goal := &workspacemodel.Goal{ID: "goal-legacy-target", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-legacy-target", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "software dev", ContextHints: hints}
	_, err = definitionAdmissionCandidate(workspace, goal, dod, template)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "pre-seal execution contract rejects prospectiveTargetRef") {
		t.Fatalf("removed pre-seal exact target field err=%v", err)
	}
}

func TestDefinitionAdmissionRejectsBootstrapArtifactExactTarget(t *testing.T) {
	hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{
		"bootstrapRecipes": []any{map[string]any{
			"id": "bootstrap", "requiredGoalTerms": []any{"goal"}, "artifacts": []any{map[string]any{
				"id": "source", "targetRole": "implementation", "TargetRef": "main.go", "allowedOperationKinds": []any{"file/write"},
			}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{ID: "workspace-bootstrap-target", DeliverableDirectory: "/workspace", TemplateID: "software-dev"}
	goal := &workspacemodel.Goal{ID: "goal-bootstrap-target", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-bootstrap-target", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "software dev", ContextHints: hints}
	_, err = definitionAdmissionCandidate(workspace, goal, dod, template)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "pre-seal bootstrap artifact rejects targetRef") {
		t.Fatalf("bootstrap artifact exact target err=%v", err)
	}
}

func TestDefinitionAdmissionRejectsRemovedPresealExactTargetInEvidencePolicies(t *testing.T) {
	legacyPolicy, err := workorcheventpayload.FromMap(map[string]any{
		"id": "legacy-target-policy", "appliesTo": "file/write",
		"coverageRequirements": []any{map[string]any{
			"id": "deliverable", "scope": "deliverable", "ProspectiveTargetRef": "legacy-target.txt",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	newInputs := func() (*workspacemodel.Workspace, *workspacemodel.Goal, *workspacemodel.DoD, *workspacemodel.WorkspaceTemplate) {
		workspace := &workspacemodel.Workspace{ID: "workspace-legacy-policy", DeliverableDirectory: "/workspace", TemplateID: "template-legacy-policy"}
		goal := &workspacemodel.Goal{ID: "goal-legacy-policy", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
		dod := &workspacemodel.DoD{ID: "dod-legacy-policy", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
		template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "legacy policy"}
		return workspace, goal, dod, template
	}
	for _, tc := range []struct {
		name   string
		attach func(*workspacemodel.Workspace, *workspacemodel.WorkspaceTemplate)
	}{
		{name: "template", attach: func(_ *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) {
			template.EvidencePolicies = []workorcheventpayload.Payload{legacyPolicy}
		}},
		{name: "workspace", attach: func(workspace *workspacemodel.Workspace, _ *workspacemodel.WorkspaceTemplate) {
			workspace.EvidencePolicies = []workorcheventpayload.Payload{legacyPolicy}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace, goal, dod, template := newInputs()
			tc.attach(workspace, template)
			_, err := definitionAdmissionCandidate(workspace, goal, dod, template)
			if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "pre-seal evidence policy rejects prospectiveTargetRef") {
				t.Fatalf("removed evidence-policy exact target err=%v", err)
			}
		})
	}
}

func TestDefinitionAdmissionCandidateSealsWorkspaceEvidencePolicySnapshot(t *testing.T) {
	policy, err := workorcheventpayload.FromMap(map[string]any{
		"id": "workspace-validation", "appliesTo": "shell/command", "commandName": "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{
		ID: "workspace-evidence-seal", DeliverableDirectory: "/workspace", TemplateID: "template-evidence-seal",
		EvidencePolicies: []workorcheventpayload.Payload{policy},
	}
	goal := &workspacemodel.Goal{ID: "goal-evidence-seal", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-evidence-seal", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "evidence seal"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.AdmissionPolicyBindings) != 1 || !strings.HasPrefix(candidate.AdmissionPolicyBindings[0].PolicyRef, "workspace-evidence-policy:") {
		t.Fatalf("workspace evidence policy was not sealed: %+v", candidate.AdmissionPolicyBindings)
	}
	changedPolicy := policy.Clone()
	for idx := range changedPolicy.Fields {
		if changedPolicy.Fields[idx].Name == "commandName" {
			changedPolicy.Fields[idx].Value.String = "make"
		}
	}
	changedWorkspace := *workspace
	changedWorkspace.EvidencePolicies = []workorcheventpayload.Payload{changedPolicy}
	changed, err := definitionAdmissionCandidate(&changedWorkspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	if changed.CandidateHash == candidate.CandidateHash {
		t.Fatal("workspace evidence policy drift did not change the pre-seal candidate hash")
	}
}

func TestDefinitionAdmissionRejectsLossyEvidencePolicyInputsPerOwner(t *testing.T) {
	valid := workorcheventpayload.New(
		workorcheventpayload.StringField("id", "validation-policy"),
		workorcheventpayload.StringField("appliesTo", runtimecontract.StepKindShellCommand),
	)
	missingID := workorcheventpayload.New(
		workorcheventpayload.StringField("appliesTo", runtimecontract.StepKindShellCommand),
	)
	invalidTypedPolicy := workorcheventpayload.New(
		workorcheventpayload.StringField("id", "invalid-policy"),
	)
	for _, owner := range []struct {
		name   string
		attach func(*workspacemodel.Workspace, *workspacemodel.WorkspaceTemplate, []workorcheventpayload.Payload)
	}{
		{name: "template", attach: func(_ *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate, policies []workorcheventpayload.Payload) {
			template.EvidencePolicies = policies
		}},
		{name: "workspace", attach: func(workspace *workspacemodel.Workspace, _ *workspacemodel.WorkspaceTemplate, policies []workorcheventpayload.Payload) {
			workspace.EvidencePolicies = policies
		}},
	} {
		for _, input := range []struct {
			name     string
			policies []workorcheventpayload.Payload
			want     string
		}{
			{name: "empty id", policies: []workorcheventpayload.Payload{missingID}, want: "requires a non-empty string id"},
			{name: "duplicate id", policies: []workorcheventpayload.Payload{valid, valid.Clone()}, want: "duplicate id"},
			{name: "invalid typed policy", policies: []workorcheventpayload.Payload{invalidTypedPolicy}, want: "is invalid"},
		} {
			t.Run(owner.name+"/"+input.name, func(t *testing.T) {
				workspace := &workspacemodel.Workspace{ID: "workspace-lossy-policy", DeliverableDirectory: "/workspace", TemplateID: "template-lossy-policy"}
				goal := &workspacemodel.Goal{ID: "goal-lossy-policy", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
				dod := &workspacemodel.DoD{ID: "dod-lossy-policy", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
				template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "lossy policy"}
				owner.attach(workspace, template, workorcheventpayload.CloneSlice(input.policies))
				_, err := definitionAdmissionCandidate(workspace, goal, dod, template)
				if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), input.want) {
					t.Fatalf("lossy evidence policy err=%v, want %q", err, input.want)
				}
			})
		}
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
				"acquisitionTargetPolicy": "prospective_owner_target", "acquisitionTargetSelectorRef": "workspace-relative-new-file-target", "prospectiveSatisfactionContractRef": "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
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
	command, err := (workspaceevents.DefinitionAdmissionClarificationBinding{Candidate: missing, AdmissionResult: blocked}).DecisionRequestIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if command.RequiredActionKind != "select_or_fix_template_profile" {
		t.Fatalf("profile clarification action = %q", command.RequiredActionKind)
	}
}

func TestFreshDefinitionAdmissionReauthorsIncompleteHistoricalConvergence(t *testing.T) {
	profile := map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{
		"evidenceRequirements": []any{map[string]any{
			"id": "validation_preparation", "scope": "deliverable",
			"targetSelectorRef": "file/workspace_deliverable", "targetSelectorOwnerRef": "software-dev", "targetSelectorSubjectRef": "validation_preparation",
			"acceptedStepKinds":  []any{"file/write", "validation/managed_session"},
			"requiredOwnerFacts": []any{map[string]any{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session"}},
			"ownerFactConvergences": []any{map[string]any{
				"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session",
				"missingTrigger": "owner_fact_missing", "acquisitionOwnerModuleId": "blocks/file", "acquisitionOperationKind": "file/write",
				"acquisitionTargetPolicy": "prospective_owner_target", "acquisitionTargetSelectorRef": "workspace-relative-new-file-target", "prospectiveSatisfactionContractRef": "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
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
	if !errors.Is(want, common.ErrConflict) || !strings.Contains(want.Error(), "start a new execution with the current Template") {
		t.Fatalf("strict current-shape error = %v", want)
	}
	fresh, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := definitionadmission.Compile(fresh)
	if err != nil || compiled.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || fresh.CandidateRevision != candidate.CandidateRevision+1 || fresh.BaseStartupInputHash != startup.StartupInputHash {
		t.Fatalf("fresh current-config admission = %+v candidate=%+v err=%v", compiled, fresh, err)
	}
	if workspace.DefinitionAdmissionCandidates[0].CandidateHash != before || workspace.DefinitionAdmissionCandidates[0].OwnerFactConvergenceBindings[0].Convergence.AcquisitionTargetPolicy != "" {
		t.Fatal("fresh authoring rewrote the archived candidate")
	}
}

func TestDefinitionAdmissionCandidateBindsExactOwnerInputSnapshot(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-exact", DeliverableDirectory: "/workspace", TemplateID: "template-exact"}
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "exact", Description: "sealed"}
	candidate, err := definitionAdmissionCandidateForIdentity(workspace, goal, dod, template, "workspace-definition", 1, "", "")
	if err != nil {
		t.Fatal(err)
	}

	driftedTemplate := *template
	driftedTemplate.Description = "new live template revision"
	templateDrift, err := definitionAdmissionCandidateForIdentity(workspace, goal, dod, &driftedTemplate, candidate.CandidateID, candidate.CandidateRevision, "", "")
	if err != nil || templateDrift.CandidateHash == candidate.CandidateHash {
		t.Fatalf("live Template drift reused candidate hash: candidate=%q drift=%q err=%v", candidate.CandidateHash, templateDrift.CandidateHash, err)
	}

	driftedWorkspace := *workspace
	driftedWorkspace.DeliverableDirectory = "/workspace-v2"
	directoryDrift, err := definitionAdmissionCandidateForIdentity(&driftedWorkspace, goal, dod, template, candidate.CandidateID, candidate.CandidateRevision, "", "")
	if err != nil || directoryDrift.CandidateHash == candidate.CandidateHash {
		t.Fatalf("resolved directory drift reused candidate hash: candidate=%q drift=%q err=%v", candidate.CandidateHash, directoryDrift.CandidateHash, err)
	}
}

func TestDefinitionAdmissionCandidateReusesExactCurrentCandidateAfterPromotion(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-reuse", DeliverableDirectory: "/workspace", TemplateID: "template-reuse"}
	goal := &workspacemodel.Goal{ID: "goal-reuse", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-reuse", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "reuse"}
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
	reused, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil || reused.CandidateRevision != candidate.CandidateRevision || reused.CandidateHash != candidate.CandidateHash {
		t.Fatalf("exact promoted candidate was not reused: got=%+v want=%+v err=%v", reused, candidate, err)
	}
}

func TestRuntimeBootstrapStartupSealRejectsTemplateAndProfileDrift(t *testing.T) {
	workspacePolicy := workorcheventpayload.New(
		workorcheventpayload.StringField("id", "workspace-validation"),
		workorcheventpayload.StringField("appliesTo", runtimecontract.StepKindShellCommand),
		workorcheventpayload.StringField("commandName", "go"),
	)
	workspace := &workspacemodel.Workspace{
		ID: "workspace-seal", DeliverableDirectory: "/workspace", TemplateID: "template-seal",
		EvidencePolicies: []workorcheventpayload.Payload{workspacePolicy},
	}
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
	sealedPolicies, err := compileRuntimeEvidencePolicies("Workspace "+string(workspace.ID), workspace.EvidencePolicies)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := &runtimecontract.RuntimeBootstrap{
		Definition: runtimecontract.RuntimeDefinitionContext{Revision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash},
		Activation: runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{}, EvidencePolicies: sealedPolicies},
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
	policyDriftedBootstrap := bootstrap.Clone()
	policyDriftedBootstrap.Activation.EvidencePolicies[0].CommandName = "make"
	if err := validateRuntimeBootstrapStartupSeal(workspace, template, candidate, &policyDriftedBootstrap); err == nil {
		t.Fatal("Runtime evidence-policy projection drift was accepted by final bootstrap gate")
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

func TestBuiltinSoftwareDevSpecificationWildcardRemainsApplicableToProductionBootstrap(t *testing.T) {
	template := builtinSoftwareDevTemplateForTest(t)
	w := &workspacemodel.Workspace{ID: "ws-default-spec", TemplateID: template.ID, DeliverableDirectory: t.TempDir(), EvidencePolicies: workorcheventpayload.CloneSlice(template.EvidencePolicies)}
	goal := &workspacemodel.Goal{ID: "goal-default", WorkspaceID: w.ID, Text: "implement requested behavior", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-default", WorkspaceID: w.ID, Items: append([]string{"user criterion"}, template.DoDItems...), DefinitionRevision: "dod-r1"}
	candidate, err := definitionAdmissionCandidate(w, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	spec := workspacemodel.GoalSpecification{Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(goal.Text), Dimensions: []workspacemodel.GoalSpecificationDimension{{ID: "all", Required: true, Activated: true}}}
	for n, item := range candidate.DoDInputs {
		id := fmt.Sprintf("req-%03d", n+1)
		spec.Requirements = append(spec.Requirements, workspacemodel.GoalSpecificationRequirement{ID: id, DimensionID: "all", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		spec.Obligations = append(spec.Obligations, workspacemodel.GoalSpecificationObligation{ID: "obl-" + id, RequirementID: id, DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		spec.Verifications = append(spec.Verifications, workspacemodel.GoalSpecificationVerification{ID: "ver-" + id, ObligationID: "obl-" + id, Declaration: "按默认模板声明验证", Status: workspacemodel.GoalVerificationDeclared, ProfileID: "software-dev", EvidenceRequirementID: "*", Scope: "dodItem"})
	}
	hash := spec.ContentHash()
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		spec.Reviews = append(spec.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewPass, SpecificationHash: hash, Reason: "synthetic independent review", EvidenceRefs: []string{candidate.GoalInput.CandidateItemRef}, RequirementIDs: []string{spec.Requirements[0].ID}})
	}
	candidate.Specification = &spec
	candidate = candidate.WithHash()
	bootstrap := assertBuiltinSoftwareDevCandidateAndBootstrap(t, template, w, goal, dod, candidate)
	count := 0
	for _, focus := range bootstrap.DefinitionFocuses {
		if focus.AssessmentScope == "dodItem" {
			count++
			if len(focus.AcceptedStepKinds) == 0 || len(focus.RequiredOwnerFacts) == 0 || focus.ValidationFailureRepairConvergence == nil || focus.ValidationRecipeID != "" || len(focus.RequiredCheckRefs) != 0 {
				t.Fatalf("default template contribution lost or fabricated recipe/check: %+v", focus)
			}
		}
	}
	if count != len(dod.Items) {
		t.Fatalf("default template incomplete sources: %d want %d", count, len(dod.Items))
	}
}
