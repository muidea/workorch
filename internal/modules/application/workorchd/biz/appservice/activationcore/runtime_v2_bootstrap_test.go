package activationcore

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2BootstrapBuildsOrderedDoDFocuses(t *testing.T) {
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"实现行为一", "验证行为二", "覆盖行为三"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 3 || bootstrap.Focuses[0].ID != "focus-001" || bootstrap.Focuses[2].CoverageWorkItemIDs[0] != "cwi-003" {
		t.Fatalf("bootstrap focuses = %+v", bootstrap.Focuses)
	}
	if bootstrap.Definition.GoalDefinition != "交付完整能力" || bootstrap.Definition.DoDDefinition == "" || len(bootstrap.Requirements) != 7 {
		t.Fatalf("bootstrap = %+v", bootstrap)
	}
	if !containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeCapabilityClassValidation, runtimecontract.StepKindFileRead) {
		t.Fatalf("bootstrap must provide a non-mutating file/read validation capability: %+v", bootstrap.Requirements)
	}
	facts, err := bootstrap.InitialFacts(common.WorkspaceID("ws-1"), common.RunID("run-1"))
	if err != nil {
		t.Fatal(err)
	}
	if facts.Focus.ID != "focus-001" || len(facts.RemainingFocuses) != 2 || facts.EvidenceRevision != runtimecontract.InitialRuntimeEvidenceRevision {
		t.Fatalf("initial facts = %+v", facts)
	}
}

func containsBootstrapCapability(requirements []runtimecontract.RuntimeRequirement, owner string, class runtimecontract.RuntimeCapabilityClass, kind string) bool {
	for _, requirement := range requirements {
		if requirement.Kind == runtimecontract.PrerequisiteKindCapability && requirement.OwnerModuleID == owner && requirement.CapabilityClass == class &&
			len(requirement.RequiredCapabilities) == 1 && requirement.RequiredCapabilities[0] == kind {
			return true
		}
	}
	return false
}

func TestRuntimeV2BootstrapAllowsValidationOnlyFocusWhenStateChangeIsOptional(t *testing.T) {
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "完成只读核对"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"核对结果已验证"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{runtimecontract.EvidenceKindValidation}
	if len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].RequiredEvidenceKinds) != 1 || bootstrap.Focuses[0].RequiredEvidenceKinds[0] != want[0] {
		t.Fatalf("optional state-change focus = %+v, want validation only", bootstrap.Focuses)
	}
}

func TestRuntimeV2BootstrapAppendsRequiredProfileFocusesWithTypedRequirements(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID:                   "software-dev",
			AcceptanceDimensions: []runtimecontract.RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true, Description: "entrypoint works"}},
			DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
				{ID: "source_change", Required: true, EvidenceRole: "primary_state_change", Description: "workspace changed"},
				{ID: "coverage_map", Required: true, EvidenceRole: "supplemental_evidence", Description: "coverage is mapped"},
				{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence", Description: "snapshot exists"},
			},
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{ID: "runtime_entrypoint", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"shell/command"}},
				{ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change", AcceptedStepKinds: []string{"patch/apply"}},
				{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperations: []string{"commit"}},
			},
		}},
	}}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 5 {
		t.Fatalf("profile focuses = %+v", bootstrap.Focuses)
	}
	entrypoint, sourceChange, coverageMap, snapshot := bootstrap.Focuses[1], bootstrap.Focuses[2], bootstrap.Focuses[3], bootstrap.Focuses[4]
	if entrypoint.AssessmentScope != "acceptanceDimension" || len(entrypoint.RequiredEvidenceKinds) != 1 || entrypoint.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation ||
		sourceChange.EvidenceRole != "primary_state_change" || sourceChange.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		len(coverageMap.RequiredEvidenceKinds) != 0 || !coverageMap.RequireAssessment ||
		len(snapshot.AcceptedOperations) != 1 || snapshot.AcceptedOperations[0] != "commit" || snapshot.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("typed profile focuses = %+v", bootstrap.Focuses)
	}
	foundGit := false
	for _, requirement := range bootstrap.Requirements {
		if requirement.OwnerModuleID == runtimecontract.RuntimeModuleGit && requirement.CapabilityClass == runtimecontract.RuntimeCapabilityClassStateChange &&
			len(requirement.RequiredCapabilities) == 1 && requirement.RequiredCapabilities[0] == "git/write" {
			foundGit = true
		}
	}
	if !foundGit {
		t.Fatalf("profile capability requirements = %+v, want git/write state-change", bootstrap.Requirements)
	}
}
