package activationcore

import (
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeV2Bootstrap(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	definitionRevision = strings.TrimSpace(definitionRevision)
	if goal == nil || dod == nil || goal.ID == "" || dod.ID == "" || goal.WorkspaceID == "" || goal.WorkspaceID != dod.WorkspaceID ||
		strings.TrimSpace(goal.Text) == "" || len(dod.Items) == 0 || definitionRevision == "" {
		return nil, common.ErrInvalidArgument
	}
	focuses := make([]runtimecontract.CoverageFocus, 0, len(dod.Items))
	requiredEvidenceKinds := []string{runtimecontract.EvidenceKindValidation}
	if activation.ExecutionContract.Normalized().StateChangePolicy == runtimecontract.RuntimeStateChangeMustProduce {
		requiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	}
	dodLines := make([]string, 0, len(dod.Items))
	for idx, item := range dod.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		sequence := idx + 1
		dodLines = append(dodLines, fmt.Sprintf("%d. %s", sequence, item))
		focuses = append(focuses, runtimecontract.CoverageFocus{
			ID:                    fmt.Sprintf("focus-%03d", sequence),
			AssessmentScope:       "dodItem",
			AssessmentID:          fmt.Sprintf("dod-%03d", sequence),
			AssessmentText:        item,
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", sequence))},
			RequiredEvidenceKinds: append([]string(nil), requiredEvidenceKinds...),
			RequireAssessment:     true,
		})
	}
	focuses = appendRuntimeV2ProfileFocuses(focuses, activation.ExecutionContract.ProfileContracts)
	capabilityContract := []string{
		runtimecontract.StepKindFileEdit,
		runtimecontract.StepKindShellCommand,
		runtimecontract.RuntimeBlockKindDecisionRequest,
	}
	capabilityContract = append(capabilityContract, runtimeV2ProfileCapabilityKinds(activation.ExecutionContract.ProfileContracts)...)
	requirements := []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "workspace manifest required"},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, Reason: "validation scratch required"},
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 96 * 1024, Reason: "provider ceiling required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "state-change capability required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}, Reason: "validation capability required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}, Reason: "governance capability required"},
	}
	requirements = append(requirements, runtimeV2ProfileCapabilityRequirements(activation.ExecutionContract.ProfileContracts)...)
	bootstrap := runtimecontract.RuntimeBootstrap{
		Activation: activation,
		Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: definitionRevision, GoalID: goal.ID, GoalDefinition: goal.Text,
			DoDID: dod.ID, DoDDefinition: strings.Join(dodLines, "\n"),
			ChangeImpact:       changeImpact,
			CapabilityContract: capabilityContract,
		},
		Focuses:      focuses,
		Requirements: requirements,
	}
	bootstrap = bootstrap.Normalized()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	return &bootstrap, nil
}

func appendRuntimeV2ProfileFocuses(focuses []runtimecontract.CoverageFocus, profiles []runtimecontract.RuntimeProfileContract) []runtimecontract.CoverageFocus {
	seen := map[string]struct{}{}
	sequence := len(focuses)
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		requirements := map[string]runtimecontract.RuntimeProfileEvidenceRequirement{}
		for _, requirement := range profile.EvidenceRequirements {
			requirements[requirement.Scope+"|"+requirement.ID] = requirement
		}
		for _, dimension := range profile.AcceptanceDimensions {
			if !dimension.Required {
				continue
			}
			sequence, focuses = appendRuntimeV2ProfileFocus(sequence, focuses, seen, "acceptanceDimension", dimension.ID, dimension.Description, "", requirements["acceptanceDimension|"+dimension.ID])
		}
		for _, evidenceRole := range []string{"primary_state_change", "supplemental_evidence", ""} {
			for _, deliverable := range profile.DeliverableExpectations {
				if !deliverable.Required || deliverable.EvidenceRole != evidenceRole {
					continue
				}
				sequence, focuses = appendRuntimeV2ProfileFocus(sequence, focuses, seen, "deliverable", deliverable.ID, deliverable.Description, deliverable.EvidenceRole, requirements["deliverable|"+deliverable.ID])
			}
		}
	}
	return focuses
}

func appendRuntimeV2ProfileFocus(sequence int, focuses []runtimecontract.CoverageFocus, seen map[string]struct{}, scope, id, description, evidenceRole string, requirement runtimecontract.RuntimeProfileEvidenceRequirement) (int, []runtimecontract.CoverageFocus) {
	key := scope + "|" + id
	if id == "" {
		return sequence, focuses
	}
	if _, ok := seen[key]; ok {
		return sequence, focuses
	}
	seen[key] = struct{}{}
	sequence++
	requiredEvidenceKinds := []string{}
	if evidenceRole == "primary_state_change" || len(requirement.AcceptedOperations) > 0 {
		requiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	} else if len(requirement.AcceptedStepKinds) > 0 {
		requiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	}
	focuses = append(focuses, runtimecontract.CoverageFocus{
		ID: fmt.Sprintf("focus-%03d", sequence), AssessmentScope: scope, AssessmentID: id, AssessmentText: description,
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", sequence))},
		RequiredEvidenceKinds: requiredEvidenceKinds, EvidenceRole: evidenceRole,
		AcceptedStepKinds: requirement.AcceptedStepKinds, AcceptedOperations: requirement.AcceptedOperations, RejectedStepKinds: requirement.RejectedStepKinds,
		RequireAssessment: true,
	})
	return sequence, focuses
}

func runtimeV2ProfileCapabilityKinds(profiles []runtimecontract.RuntimeProfileContract) []string {
	ret := []string{}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			ret = append(ret, requirement.AcceptedStepKinds...)
		}
	}
	return ret
}

func runtimeV2ProfileCapabilityRequirements(profiles []runtimecontract.RuntimeProfileContract) []runtimecontract.RuntimeRequirement {
	ret := []runtimecontract.RuntimeRequirement{}
	seen := map[string]struct{}{}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			capabilityClass := runtimecontract.RuntimeCapabilityClassValidation
			if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperations) > 0 {
				capabilityClass = runtimecontract.RuntimeCapabilityClassStateChange
			}
			for _, kind := range requirement.AcceptedStepKinds {
				owner := runtimecontract.RuntimeModuleForOperationKind(kind)
				if owner == "" {
					continue
				}
				key := owner + "|" + string(capabilityClass) + "|" + kind
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				ret = append(ret, runtimecontract.RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: owner, CapabilityClass: capabilityClass,
					RequiredCapabilities: []string{kind}, Reason: "profile evidence capability required",
				})
			}
		}
	}
	return ret
}
