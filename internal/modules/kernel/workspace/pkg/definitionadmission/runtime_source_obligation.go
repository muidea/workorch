package definitionadmission

import (
	"fmt"
	"slices"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompileRuntimeSourceObligationInventory 仅从已封存、eligible 的 exact candidate
// 投影来源图。不读取可变草稿，不解释正文，也不授予运行期 binding 或 credit。
func CompileRuntimeSourceObligationInventory(candidate workspacemodel.DefinitionAdmissionCandidate, startup workspacemodel.WorkspaceStartupInputContract, admission workspacemodel.WorkspaceActivationAdmission) (*runtimecontract.RuntimeSourceObligationInventory, error) {
	if err := admission.ValidateCandidate(startup, candidate); err != nil {
		return nil, fmt.Errorf("runtime source obligations require exact Workspace admission: %w", err)
	}
	if admission.State != workspacemodel.WorkspaceActivationEligible {
		return nil, fmt.Errorf("runtime source obligations require eligible Workspace admission")
	}
	if candidate.Specification == nil {
		return nil, nil
	}
	result, err := Compile(candidate)
	if err != nil {
		return nil, err
	}
	if result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		return nil, fmt.Errorf("runtime source obligations require complete all_of specification")
	}
	sources, ok := specificationDoDSources(candidate.DoDInputs)
	if !ok {
		return nil, fmt.Errorf("runtime source obligations cannot resolve confirmed DoD source order")
	}
	mappings := make(map[string]string, len(startup.CandidateItemToItemRefMappings))
	for _, mapping := range startup.CandidateItemToItemRefMappings {
		mappings[mapping.CandidateItemRef] = mapping.ItemRef
	}
	spec := candidate.Specification.Clone()
	inventory := runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent,
		CandidateHash: candidate.CandidateHash, SpecificationHash: startup.SpecificationHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash,
		Sources: []runtimecontract.RuntimeDefinitionSource{{
			CandidateItemRef: candidate.GoalInput.CandidateItemRef, ItemRef: mappings[candidate.GoalInput.CandidateItemRef],
			SourceScope: "goal", SourceID: spec.GoalItemRef, Text: candidate.GoalInput.Text,
		}},
	}
	for _, mapping := range spec.ExecutionProofMappings {
		inventory.ExecutionProofMappings = append(inventory.ExecutionProofMappings, runtimecontract.RuntimeExecutionProofMapping(mapping))
	}
	for _, mapping := range spec.EvidencePurposeMappings {
		inventory.EvidencePurposeMappings = append(inventory.EvidencePurposeMappings, runtimecontract.RuntimeEvidencePurposeMapping(mapping))
	}
	for _, item := range candidate.DoDInputs {
		inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{
			CandidateItemRef: item.CandidateItemRef, ItemRef: mappings[item.CandidateItemRef],
			SourceScope: "dodItem", SourceID: sources[item.CandidateItemRef], Text: item.Text,
		})
	}
	for _, resource := range spec.Resources {
		inventory.Resources = append(inventory.Resources, runtimecontract.RuntimeSourceResource{
			ID: resource.ID, Ref: resource.Ref, SnapshotHash: resource.SnapshotHash, Required: resource.Required, Available: resource.Available,
		})
	}
	for _, requirement := range spec.Requirements {
		inventory.Requirements = append(inventory.Requirements, runtimecontract.RuntimeSourceRequirement{
			ID: requirement.ID, DimensionID: requirement.DimensionID, Required: requirement.Required, SourceKind: requirement.SourceKind,
			SourceItemRefs: slices.Clone(requirement.SourceItemRefs), ResourceIDs: slices.Clone(requirement.ResourceIDs), Text: requirement.Text,
		})
	}
	for _, obligation := range spec.Obligations {
		inventory.Obligations = append(inventory.Obligations, runtimecontract.RuntimeSourceObligation{
			ID: obligation.ID, RequirementID: obligation.RequirementID, DoDItemRefs: slices.Clone(obligation.DoDItemRefs), Text: obligation.Text,
		})
	}
	for _, verification := range spec.Verifications {
		if verification.Status != workspacemodel.GoalVerificationDeclared || !SpecificationVerificationApplies(spec, verification, candidate.DoDInputs, candidate.RuntimeProfileContracts) {
			return nil, fmt.Errorf("runtime source obligation verification %q is not exact and applicable", verification.ID)
		}
		inventory.Verifications = append(inventory.Verifications, runtimecontract.RuntimeSourceVerification{
			ID: verification.ID, ObligationID: verification.ObligationID, Declaration: verification.Declaration,
			ProfileID: verification.ProfileID, EvidenceRequirementID: verification.EvidenceRequirementID, Scope: verification.Scope,
		})
	}
	inventory = inventory.WithHash()
	if err := inventory.Validate(); err != nil {
		return nil, err
	}
	if err := runtimecontract.ValidateRuntimeSourceObligationApplicability(&inventory, candidate.RuntimeProfileContracts); err != nil {
		return nil, err
	}
	return &inventory, nil
}
