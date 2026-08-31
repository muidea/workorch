// RuntimeStateRecord raw-clone helpers deliberately enumerate the persisted
// typed graph so Run can copy it opaquely without invoking semantic canonicalization.

package workorchruntimecontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

func cloneRuntimeStateRawPointer[T any](in *T, clone func(T) T) *T {
	if in == nil {
		return nil
	}
	value := clone(*in)
	return &value
}

func cloneRuntimeStateRawSlice[T any](in []T, clone func(T) T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	for idx := range in {
		out[idx] = clone(in[idx])
	}
	return out
}

func cloneRuntimeStateRawMap[K comparable, V any](in map[K]V, cloneKey func(K) K, cloneValue func(V) V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[cloneKey(key)] = cloneValue(value)
	}
	return out
}

func cloneRuntimeMemoryRecallSelectionEvidenceRaw(in workorchmemorycontract.RecallSelectionEvidence) workorchmemorycontract.RecallSelectionEvidence {
	out := in
	out.CandidateMemoryIDs = cloneRuntimeStateRawSlice(in.CandidateMemoryIDs, func(item common.MemoryID) common.MemoryID { return item })
	out.SelectedMemoryIDs = cloneRuntimeStateRawSlice(in.SelectedMemoryIDs, func(item common.MemoryID) common.MemoryID { return item })
	out.ExcludedMemoryIDs = cloneRuntimeStateRawSlice(in.ExcludedMemoryIDs, func(item common.MemoryID) common.MemoryID { return item })
	out.TypeQuota = cloneRuntimeStateRawSlice(in.TypeQuota, func(item workorchmemorycontract.TypeQuotaEntry) workorchmemorycontract.TypeQuotaEntry { return item })
	out.SelectionScores = cloneRuntimeStateRawSlice(in.SelectionScores, func(item workorchmemorycontract.RecallSelectionScore) workorchmemorycontract.RecallSelectionScore {
		return item
	})
	out.RetrievedMemoryIDs = cloneRuntimeStateRawSlice(in.RetrievedMemoryIDs, func(item common.MemoryID) common.MemoryID { return item })
	out.RetrievedMemoryHits = cloneRuntimeStateRawSlice(in.RetrievedMemoryHits, func(item workorchmemorycontract.RecallHit) workorchmemorycontract.RecallHit { return item })
	out.SemanticRecall = cloneRuntimeStateRawPointer(in.SemanticRecall, cloneRuntimeMemoryRecallSelectionEvidenceRaw)
	return out
}

func cloneAssessmentCandidateRaw(in AssessmentCandidate) AssessmentCandidate {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.Items = cloneRuntimeStateRawSlice(in.Items, func(item RuntimeAssessmentItemFact) RuntimeAssessmentItemFact {
		return cloneRuntimeAssessmentItemFactRaw(item)
	})
	out.Gap = cloneRuntimeStateRawPointer(in.Gap, func(item RuntimeAssessmentGap) RuntimeAssessmentGap { return cloneRuntimeAssessmentGapRaw(item) })
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneAuthoritySnapshotRaw(in AuthoritySnapshot) AuthoritySnapshot {
	out := in
	out.AuthorityInput = cloneRuntimeEquivalentAuthorityInputIdentityRaw(in.AuthorityInput)
	return out
}

func cloneAuthorityTransactionRaw(in AuthorityTransaction) AuthorityTransaction {
	out := in
	return out
}

func cloneCoverageEvidenceBindingRaw(in CoverageEvidenceBinding) CoverageEvidenceBinding {
	out := in
	out.Proofs = cloneRuntimeStateRawSlice(in.Proofs, func(item CoverageEvidenceProof) CoverageEvidenceProof { return cloneCoverageEvidenceProofRaw(item) })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetRefs, func(item string) string { return item })
	out.PrerequisiteRefs = cloneRuntimeStateRawSlice(in.PrerequisiteRefs, func(item string) string { return item })
	return out
}

func cloneCoverageEvidenceProofRaw(in CoverageEvidenceProof) CoverageEvidenceProof {
	out := in
	out.RequiredEffectScopes = cloneRuntimeStateRawSlice(in.RequiredEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.AllowedOperationKinds = cloneRuntimeStateRawSlice(in.AllowedOperationKinds, func(item string) string { return item })
	out.AllowedOperationVariants = cloneRuntimeStateRawSlice(in.AllowedOperationVariants, func(item RuntimeOperationVariantRef) RuntimeOperationVariantRef {
		return cloneRuntimeOperationVariantRefRaw(item)
	})
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneCoverageEvidenceProofIdentityRaw(in CoverageEvidenceProofIdentity) CoverageEvidenceProofIdentity {
	out := in
	out.AllowedOperationKinds = cloneRuntimeStateRawSlice(in.AllowedOperationKinds, func(item string) string { return item })
	out.AllowedOperationVariants = cloneRuntimeStateRawSlice(in.AllowedOperationVariants, func(item RuntimeOperationVariantRef) RuntimeOperationVariantRef {
		return cloneRuntimeOperationVariantRefRaw(item)
	})
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetRefs, func(item string) string { return item })
	out.RequiredEffectScopes = cloneRuntimeStateRawSlice(in.RequiredEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneCoverageFocusRaw(in CoverageFocus) CoverageFocus {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.CoverageEvidenceBindings = cloneRuntimeStateRawSlice(in.CoverageEvidenceBindings, func(item CoverageEvidenceBinding) CoverageEvidenceBinding {
		return cloneCoverageEvidenceBindingRaw(item)
	})
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetDiscoverySources = cloneRuntimeStateRawSlice(in.TargetDiscoverySources, func(item string) string { return item })
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetRefs, func(item string) string { return item })
	out.AcceptedStepKinds = cloneRuntimeStateRawSlice(in.AcceptedStepKinds, func(item string) string { return item })
	out.AcceptedOperationVariants = cloneRuntimeStateRawSlice(in.AcceptedOperationVariants, func(item RuntimeOperationVariantRef) RuntimeOperationVariantRef {
		return cloneRuntimeOperationVariantRefRaw(item)
	})
	out.RejectedStepKinds = cloneRuntimeStateRawSlice(in.RejectedStepKinds, func(item string) string { return item })
	out.RequiredOwnerFacts = cloneRuntimeStateRawSlice(in.RequiredOwnerFacts, func(item RuntimeOwnerFactRequirement) RuntimeOwnerFactRequirement {
		return cloneRuntimeOwnerFactRequirementRaw(item)
	})
	out.OwnerFactConvergences = cloneRuntimeStateRawSlice(in.OwnerFactConvergences, func(item RuntimeOwnerFactConvergence) RuntimeOwnerFactConvergence {
		return cloneRuntimeOwnerFactConvergenceRaw(item)
	})
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneEvidenceFactRaw(in EvidenceFact) EvidenceFact {
	out := in
	out.ContentAddress = cloneRuntimeStateRawPointer(in.ContentAddress, func(item RuntimeEvidenceContentAddress) RuntimeEvidenceContentAddress {
		return cloneRuntimeEvidenceContentAddressRaw(item)
	})
	out.ExitCode = cloneRuntimeStateRawPointer(in.ExitCode, func(item int64) int64 { return item })
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.BindingRefs = cloneRuntimeStateRawSlice(in.BindingRefs, func(item string) string { return item })
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.EvidenceTags = cloneRuntimeStateRawSlice(in.EvidenceTags, func(item string) string { return item })
	out.Validation = cloneRuntimeStateRawPointer(in.Validation, func(item RuntimeValidationObservation) RuntimeValidationObservation {
		return cloneRuntimeValidationObservationRaw(item)
	})
	return out
}

func cloneEvidenceLedgerRaw(in EvidenceLedger) EvidenceLedger {
	out := in
	out.Accepted = cloneRuntimeStateRawSlice(in.Accepted, func(item EvidenceFact) EvidenceFact { return cloneEvidenceFactRaw(item) })
	out.Rejected = cloneRuntimeStateRawSlice(in.Rejected, func(item EvidenceFact) EvidenceFact { return cloneEvidenceFactRaw(item) })
	return out
}

func cloneExternalDependencyRaw(in ExternalDependency) ExternalDependency {
	out := in
	out.AuthorizationContext = in.AuthorizationContext.Clone()
	return out
}

func cloneFailureFactRaw(in FailureFact) FailureFact {
	out := in
	out.OriginalDiagnostic = cloneRuntimeStateRawPointer(in.OriginalDiagnostic, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetObservation = cloneRuntimeStateRawPointer(in.TargetObservation, func(item RuntimeFailureTargetObservation) RuntimeFailureTargetObservation {
		return cloneRuntimeFailureTargetObservationRaw(item)
	})
	out.ValidationDiagnostic = cloneRuntimeStateRawPointer(in.ValidationDiagnostic, func(item ValidationFailureDiagnostic) ValidationFailureDiagnostic {
		return cloneValidationFailureDiagnosticRaw(item)
	})
	out.ValidationFailureObservation = cloneRuntimeStateRawPointer(in.ValidationFailureObservation, func(item RuntimeValidationFailureObservation) RuntimeValidationFailureObservation {
		return cloneRuntimeValidationFailureObservationRaw(item)
	})
	out.GovernanceDependency = cloneRuntimeStateRawPointer(in.GovernanceDependency, func(item ExternalDependency) ExternalDependency { return cloneExternalDependencyRaw(item) })
	return out
}

func cloneFailureTargetRaw(in FailureTarget) FailureTarget {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetObservation = cloneRuntimeStateRawPointer(in.TargetObservation, func(item RuntimeFailureTargetObservation) RuntimeFailureTargetObservation {
		return cloneRuntimeFailureTargetObservationRaw(item)
	})
	return out
}

func cloneGovernanceCandidateRaw(in GovernanceCandidate) GovernanceCandidate {
	out := in
	out.ExternalDependency = cloneExternalDependencyRaw(in.ExternalDependency)
	return out
}

func cloneOperationParameterRaw(in OperationParameter) OperationParameter {
	out := in
	out.String = cloneRuntimeStateRawPointer(in.String, func(item string) string { return item })
	out.Integer = cloneRuntimeStateRawPointer(in.Integer, func(item int64) int64 { return item })
	out.Boolean = cloneRuntimeStateRawPointer(in.Boolean, func(item bool) bool { return item })
	out.StringList = cloneRuntimeStateRawSlice(in.StringList, func(item string) string { return item })
	return out
}

func clonePrerequisiteStateRaw(in PrerequisiteState) PrerequisiteState {
	out := in
	out.Pending = cloneRuntimeStateRawSlice(in.Pending, func(item RuntimePrerequisite) RuntimePrerequisite { return cloneRuntimePrerequisiteRaw(item) })
	out.Satisfied = cloneRuntimeStateRawSlice(in.Satisfied, func(item RuntimePrerequisite) RuntimePrerequisite { return cloneRuntimePrerequisiteRaw(item) })
	out.Retired = cloneRuntimeStateRawSlice(in.Retired, func(item RuntimePrerequisite) RuntimePrerequisite { return cloneRuntimePrerequisiteRaw(item) })
	return out
}

func cloneProviderCeilingRaw(in ProviderCeiling) ProviderCeiling {
	out := in
	out.Selection = cloneProviderInferenceSelectionRaw(in.Selection)
	return out
}

func cloneProviderInferenceSelectionRaw(in ProviderInferenceSelection) ProviderInferenceSelection {
	out := in
	out.Target = cloneProviderModelTargetRaw(in.Target)
	return out
}

func cloneProviderModelTargetRaw(in ProviderModelTarget) ProviderModelTarget {
	out := in
	return out
}

func cloneRequiredOutputContractRaw(in RequiredOutputContract) RequiredOutputContract {
	out := in
	out.AcceptedKinds = cloneRuntimeStateRawSlice(in.AcceptedKinds, func(item string) string { return item })
	out.AcceptedOperationVariants = cloneRuntimeStateRawSlice(in.AcceptedOperationVariants, func(item RuntimeOperationVariantRef) RuntimeOperationVariantRef {
		return cloneRuntimeOperationVariantRefRaw(item)
	})
	out.AcceptedEffectScopes = cloneRuntimeStateRawSlice(in.AcceptedEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredEffectScopes = cloneRuntimeStateRawSlice(in.RequiredEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.PrerequisiteIDs = cloneRuntimeStateRawSlice(in.PrerequisiteIDs, func(item string) string { return item })
	out.RequiredOperationSequence = cloneRuntimeStateRawSlice(in.RequiredOperationSequence, func(item RuntimeRequiredOperationVariant) RuntimeRequiredOperationVariant {
		return cloneRuntimeRequiredOperationVariantRaw(item)
	})
	return out
}

func cloneRuntimeAcceptanceDimensionRaw(in RuntimeAcceptanceDimension) RuntimeAcceptanceDimension {
	out := in
	return out
}

func cloneRuntimeAcceptanceSliceRaw(in RuntimeAcceptanceSlice) RuntimeAcceptanceSlice {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeAcceptanceSliceGroupPolicyRaw(in RuntimeAcceptanceSliceGroupPolicy) RuntimeAcceptanceSliceGroupPolicy {
	out := in
	out.Match = cloneRuntimeAcceptanceSliceMatchRaw(in.Match)
	return out
}

func cloneRuntimeAcceptanceSliceMatchRaw(in RuntimeAcceptanceSliceMatch) RuntimeAcceptanceSliceMatch {
	out := in
	out.EvidenceRoles = cloneRuntimeStateRawSlice(in.EvidenceRoles, func(item string) string { return item })
	out.MissingEvidenceKinds = cloneRuntimeStateRawSlice(in.MissingEvidenceKinds, func(item string) string { return item })
	out.SourceRequirementIDs = cloneRuntimeStateRawSlice(in.SourceRequirementIDs, func(item string) string { return item })
	out.SourceScopes = cloneRuntimeStateRawSlice(in.SourceScopes, func(item string) string { return item })
	return out
}

func cloneRuntimeAcceptanceSlicePolicyRaw(in RuntimeAcceptanceSlicePolicy) RuntimeAcceptanceSlicePolicy {
	out := in
	out.Groups = cloneRuntimeStateRawSlice(in.Groups, func(item RuntimeAcceptanceSliceGroupPolicy) RuntimeAcceptanceSliceGroupPolicy {
		return cloneRuntimeAcceptanceSliceGroupPolicyRaw(item)
	})
	return out
}

func cloneRuntimeActivationContextRaw(in RuntimeActivationContext) RuntimeActivationContext {
	out := in
	out.ProviderSelection = cloneProviderInferenceSelectionRaw(in.ProviderSelection)
	out.DefaultSoul = cloneRuntimeStateRawPointer(in.DefaultSoul, func(item RuntimeSoulContext) RuntimeSoulContext { return cloneRuntimeSoulContextRaw(item) })
	out.StageSoulBindings = cloneRuntimeStateRawSlice(in.StageSoulBindings, func(item RuntimeStageSoulBinding) RuntimeStageSoulBinding {
		return cloneRuntimeStageSoulBindingRaw(item)
	})
	out.Skills = cloneRuntimeStateRawSlice(in.Skills, func(item RuntimeSkillContext) RuntimeSkillContext { return cloneRuntimeSkillContextRaw(item) })
	out.SkillContextBudget = cloneRuntimeStateRawPointer(in.SkillContextBudget, func(item RuntimeSkillContextBudget) RuntimeSkillContextBudget {
		return cloneRuntimeSkillContextBudgetRaw(item)
	})
	out.MCPServerIDs = cloneRuntimeStateRawSlice(in.MCPServerIDs, func(item common.MCPServerID) common.MCPServerID { return item })
	out.MCPCapabilities = cloneRuntimeStateRawSlice(in.MCPCapabilities, func(item RuntimeMCPCapabilityContext) RuntimeMCPCapabilityContext {
		return cloneRuntimeMCPCapabilityContextRaw(item)
	})
	out.OperationPolicies = cloneRuntimeStateRawSlice(in.OperationPolicies, func(item RuntimeOperationPolicyFact) RuntimeOperationPolicyFact {
		return cloneRuntimeOperationPolicyFactRaw(item)
	})
	out.Memories = cloneRuntimeStateRawSlice(in.Memories, func(item RuntimeMemoryContext) RuntimeMemoryContext { return cloneRuntimeMemoryContextRaw(item) })
	out.MemorySelection = cloneRuntimeStateRawPointer(in.MemorySelection, func(item workorchmemorycontract.RecallSelectionEvidence) workorchmemorycontract.RecallSelectionEvidence {
		return cloneRuntimeMemoryRecallSelectionEvidenceRaw(item)
	})
	out.EvidencePolicies = cloneRuntimeStateRawSlice(in.EvidencePolicies, func(item RuntimeEvidencePolicy) RuntimeEvidencePolicy { return cloneRuntimeEvidencePolicyRaw(item) })
	out.ExecutionContract = cloneRuntimeExecutionContractRaw(in.ExecutionContract)
	out.Continuation = cloneRuntimeContinuationContextRaw(in.Continuation)
	return out
}

func cloneRuntimeAssessmentFactRaw(in RuntimeAssessmentFact) RuntimeAssessmentFact {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.Items = cloneRuntimeStateRawSlice(in.Items, func(item RuntimeAssessmentItemFact) RuntimeAssessmentItemFact {
		return cloneRuntimeAssessmentItemFactRaw(item)
	})
	out.Gap = cloneRuntimeStateRawPointer(in.Gap, func(item RuntimeAssessmentGap) RuntimeAssessmentGap { return cloneRuntimeAssessmentGapRaw(item) })
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeAssessmentAuthorityInputRaw(in RuntimeAssessmentAuthorityInput) RuntimeAssessmentAuthorityInput {
	out := in
	out.Focus = cloneCoverageFocusRaw(in.Focus)
	out.OperationAdmissibilities = cloneRuntimeStateRawSlice(in.OperationAdmissibilities, func(item RuntimeOperationAdmissibility) RuntimeOperationAdmissibility {
		return cloneRuntimeOperationAdmissibilityRaw(item)
	})
	return out
}

func cloneRuntimeAssessmentAdjudicationRaw(in RuntimeAssessmentAdjudication) RuntimeAssessmentAdjudication {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.Proofs = cloneRuntimeStateRawSlice(in.Proofs, func(item RuntimeAssessmentAdjudicationProof) RuntimeAssessmentAdjudicationProof { return item })
	return out
}

func cloneRuntimeAssessmentGapRaw(in RuntimeAssessmentGap) RuntimeAssessmentGap {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.MissingEvidenceKinds = cloneRuntimeStateRawSlice(in.MissingEvidenceKinds, func(item string) string { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeAssessmentItemFactRaw(in RuntimeAssessmentItemFact) RuntimeAssessmentItemFact {
	out := in
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item common.ArtifactID) common.ArtifactID { return item })
	return out
}

func cloneRuntimeBindingContradictionIdentityRaw(in RuntimeBindingContradictionIdentity) RuntimeBindingContradictionIdentity {
	out := in
	out.OwnerPolicy = cloneRuntimeOwnerPolicyIdentityRaw(in.OwnerPolicy)
	return out
}

func cloneRuntimeBootstrapArtifactRaw(in RuntimeBootstrapArtifact) RuntimeBootstrapArtifact {
	out := in
	out.AllowedOperationKinds = cloneRuntimeStateRawSlice(in.AllowedOperationKinds, func(item string) string { return item })
	out.DependsOnArtifactIDs = cloneRuntimeStateRawSlice(in.DependsOnArtifactIDs, func(item string) string { return item })
	return out
}

func cloneRuntimeBootstrapRecipeRaw(in RuntimeBootstrapRecipe) RuntimeBootstrapRecipe {
	out := in
	out.RequiredGoalTerms = cloneRuntimeStateRawSlice(in.RequiredGoalTerms, func(item string) string { return item })
	out.RequiredDoDTerms = cloneRuntimeStateRawSlice(in.RequiredDoDTerms, func(item string) string { return item })
	out.Artifacts = cloneRuntimeStateRawSlice(in.Artifacts, func(item RuntimeBootstrapArtifact) RuntimeBootstrapArtifact {
		return cloneRuntimeBootstrapArtifactRaw(item)
	})
	return out
}

func cloneRuntimeBudgetAllocationRaw(in RuntimeBudgetAllocation) RuntimeBudgetAllocation {
	out := in
	return out
}

func cloneRuntimeBudgetConsumptionFactRaw(in RuntimeBudgetConsumptionFact) RuntimeBudgetConsumptionFact {
	out := in
	return out
}

func cloneRuntimeBudgetDimensionPolicyRaw(in RuntimeBudgetDimensionPolicy) RuntimeBudgetDimensionPolicy {
	out := in
	return out
}

func cloneRuntimeBudgetReconciliationRecordRaw(in RuntimeBudgetReconciliationRecord) RuntimeBudgetReconciliationRecord {
	out := in
	return out
}

func cloneRuntimeCWIBindingRefRaw(in RuntimeCWIBindingRef) RuntimeCWIBindingRef {
	out := in
	return out
}

func cloneRuntimeCWIBindingRetirementLineageRaw(in RuntimeCWIBindingRetirementLineage) RuntimeCWIBindingRetirementLineage {
	out := in
	out.RetiredBindingRef = cloneRuntimeCWIBindingRefRaw(in.RetiredBindingRef)
	return out
}

func cloneRuntimeCWIReconciliationBatchRaw(in RuntimeCWIReconciliationBatch) RuntimeCWIReconciliationBatch {
	out := in
	out.Observations = cloneRuntimeStateRawSlice(in.Observations, func(item RuntimeCWIReconciliationObservation) RuntimeCWIReconciliationObservation {
		return cloneRuntimeCWIReconciliationObservationRaw(item)
	})
	return out
}

func cloneRuntimeCWIReconciliationBindingRefRaw(in RuntimeCWIReconciliationBindingRef) RuntimeCWIReconciliationBindingRef {
	out := in
	out.OwnerModuleIDs = cloneRuntimeStateRawSlice(in.OwnerModuleIDs, func(item string) string { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.AllowedOperationKinds = cloneRuntimeStateRawSlice(in.AllowedOperationKinds, func(item string) string { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetRefs, func(item string) string { return item })
	out.ValidationRecipeIDs = cloneRuntimeStateRawSlice(in.ValidationRecipeIDs, func(item string) string { return item })
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeCWIReconciliationContractRaw(in RuntimeCWIReconciliationContract) RuntimeCWIReconciliationContract {
	out := in
	out.Bindings = cloneRuntimeStateRawSlice(in.Bindings, func(item RuntimeCWIReconciliationBindingRef) RuntimeCWIReconciliationBindingRef {
		return cloneRuntimeCWIReconciliationBindingRefRaw(item)
	})
	return out
}

func cloneRuntimeCWIReconciliationGenerationRaw(in RuntimeCWIReconciliationGeneration) RuntimeCWIReconciliationGeneration {
	out := in
	out.Request = cloneRuntimeCWIReconciliationRequestRaw(in.Request)
	out.Batch = cloneRuntimeStateRawPointer(in.Batch, func(item RuntimeCWIReconciliationBatch) RuntimeCWIReconciliationBatch {
		return cloneRuntimeCWIReconciliationBatchRaw(item)
	})
	out.Results = cloneRuntimeStateRawSlice(in.Results, func(item RuntimeCWIReconciliationResult) RuntimeCWIReconciliationResult {
		return cloneRuntimeCWIReconciliationResultRaw(item)
	})
	out.Reopened = cloneRuntimeStateRawSlice(in.Reopened, func(item RuntimeCWIReconciliationReopenedLineage) RuntimeCWIReconciliationReopenedLineage {
		return cloneRuntimeCWIReconciliationReopenedLineageRaw(item)
	})
	return out
}

func cloneRuntimeCWIReconciliationLedgerRaw(in RuntimeCWIReconciliationLedger) RuntimeCWIReconciliationLedger {
	out := in
	out.Request = cloneRuntimeStateRawPointer(in.Request, func(item RuntimeCWIReconciliationRequest) RuntimeCWIReconciliationRequest {
		return cloneRuntimeCWIReconciliationRequestRaw(item)
	})
	out.Batch = cloneRuntimeStateRawPointer(in.Batch, func(item RuntimeCWIReconciliationBatch) RuntimeCWIReconciliationBatch {
		return cloneRuntimeCWIReconciliationBatchRaw(item)
	})
	out.Results = cloneRuntimeStateRawSlice(in.Results, func(item RuntimeCWIReconciliationResult) RuntimeCWIReconciliationResult {
		return cloneRuntimeCWIReconciliationResultRaw(item)
	})
	out.Reopened = cloneRuntimeStateRawSlice(in.Reopened, func(item RuntimeCWIReconciliationReopenedLineage) RuntimeCWIReconciliationReopenedLineage {
		return cloneRuntimeCWIReconciliationReopenedLineageRaw(item)
	})
	out.History = cloneRuntimeStateRawSlice(in.History, func(item RuntimeCWIReconciliationGeneration) RuntimeCWIReconciliationGeneration {
		return cloneRuntimeCWIReconciliationGenerationRaw(item)
	})
	return out
}

func cloneRuntimeCWIReconciliationObservationRaw(in RuntimeCWIReconciliationObservation) RuntimeCWIReconciliationObservation {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.CheckRefs = cloneRuntimeStateRawSlice(in.CheckRefs, func(item string) string { return item })
	out.MetricRefs = cloneRuntimeStateRawSlice(in.MetricRefs, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	out.OriginalDiagnostic = cloneRuntimeStateRawPointer(in.OriginalDiagnostic, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	return out
}

func cloneRuntimeCWIReconciliationReopenedLineageRaw(in RuntimeCWIReconciliationReopenedLineage) RuntimeCWIReconciliationReopenedLineage {
	out := in
	out.PreviousEvidenceRefs = cloneRuntimeStateRawSlice(in.PreviousEvidenceRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeCWIReconciliationRequestRaw(in RuntimeCWIReconciliationRequest) RuntimeCWIReconciliationRequest {
	out := in
	out.Bindings = cloneRuntimeStateRawSlice(in.Bindings, func(item RuntimeCWIReconciliationBindingRef) RuntimeCWIReconciliationBindingRef {
		return cloneRuntimeCWIReconciliationBindingRefRaw(item)
	})
	return out
}

func cloneRuntimeCWIReconciliationResultRaw(in RuntimeCWIReconciliationResult) RuntimeCWIReconciliationResult {
	out := in
	return out
}

func cloneRuntimeCWIReopenLineageRaw(in RuntimeCWIReopenLineage) RuntimeCWIReopenLineage {
	out := in
	out.PreviousBindingRef = cloneRuntimeCWIBindingRefRaw(in.PreviousBindingRef)
	return out
}

func cloneRuntimeCandidateRaw(in RuntimeCandidate) RuntimeCandidate {
	out := in
	out.StateChange = cloneRuntimeStateRawPointer(in.StateChange, func(item StateChangeCandidate) StateChangeCandidate { return cloneStateChangeCandidateRaw(item) })
	out.Validation = cloneRuntimeStateRawPointer(in.Validation, func(item ValidationCandidate) ValidationCandidate { return cloneValidationCandidateRaw(item) })
	out.Assessment = cloneRuntimeStateRawPointer(in.Assessment, func(item AssessmentCandidate) AssessmentCandidate { return cloneAssessmentCandidateRaw(item) })
	out.Governance = cloneRuntimeStateRawPointer(in.Governance, func(item GovernanceCandidate) GovernanceCandidate { return cloneGovernanceCandidateRaw(item) })
	return out
}

func cloneRuntimeCandidateContractManifestRaw(in RuntimeCandidateContractManifest) RuntimeCandidateContractManifest {
	out := in
	out.EffectPolicy.AllowedOperationEffects = cloneRuntimeStateRawSlice(in.EffectPolicy.AllowedOperationEffects, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.EffectPolicy.AllowedAcceptedReceiptEffects = cloneRuntimeStateRawSlice(in.EffectPolicy.AllowedAcceptedReceiptEffects, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredTopLevelFields = cloneRuntimeStateRawSlice(in.RequiredTopLevelFields, func(item string) string { return item })
	out.AllowedTopLevelFields = cloneRuntimeStateRawSlice(in.AllowedTopLevelFields, func(item string) string { return item })
	out.ForbiddenTopLevelFields = cloneRuntimeStateRawSlice(in.ForbiddenTopLevelFields, func(item string) string { return item })
	out.Bindings = cloneRuntimeStateRawSlice(in.Bindings, func(item CoverageEvidenceBinding) CoverageEvidenceBinding {
		return cloneCoverageEvidenceBindingRaw(item)
	})
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.RequiredOperationSequence = cloneRuntimeStateRawSlice(in.RequiredOperationSequence, func(item RuntimeRequiredOperationVariant) RuntimeRequiredOperationVariant {
		return cloneRuntimeRequiredOperationVariantRaw(item)
	})
	out.Operations = cloneRuntimeStateRawSlice(in.Operations, func(item RuntimeCandidateOperationManifest) RuntimeCandidateOperationManifest {
		return cloneRuntimeCandidateOperationManifestRaw(item)
	})
	return out
}

func cloneRuntimeCandidateOperationManifestRaw(in RuntimeCandidateOperationManifest) RuntimeCandidateOperationManifest {
	out := in
	out.AllowedOperationFields = cloneRuntimeStateRawSlice(in.AllowedOperationFields, func(item string) string { return item })
	out.RequiredOperationFields = cloneRuntimeStateRawSlice(in.RequiredOperationFields, func(item string) string { return item })
	out.AllowedVariantIDs = cloneRuntimeStateRawSlice(in.AllowedVariantIDs, func(item string) string { return item })
	out.Parameters = cloneRuntimeStateRawSlice(in.Parameters, func(item RuntimeOperationParameterContract) RuntimeOperationParameterContract { return item })
	out.RequireOneOfParameters = cloneRuntimeStateRawSlice(in.RequireOneOfParameters, func(item string) string { return item })
	out.ForbiddenParameters = cloneRuntimeStateRawSlice(in.ForbiddenParameters, func(item string) string { return item })
	out.Variants = cloneRuntimeStateRawSlice(in.Variants, func(item RuntimeOperationVariantContract) RuntimeOperationVariantContract {
		value := item
		value.Discriminators = cloneRuntimeStateRawSlice(item.Discriminators, func(value RuntimeOperationParameterPredicate) RuntimeOperationParameterPredicate { return value })
		value.RequiredParameters = cloneRuntimeStateRawSlice(item.RequiredParameters, func(value string) string { return value })
		value.RequiredNonEmpty = cloneRuntimeStateRawSlice(item.RequiredNonEmpty, func(value string) string { return value })
		value.ForbiddenParameters = cloneRuntimeStateRawSlice(item.ForbiddenParameters, func(value string) string { return value })
		value.DistinctParameterPairs = cloneRuntimeStateRawSlice(item.DistinctParameterPairs, func(value RuntimeOperationParameterPair) RuntimeOperationParameterPair { return value })
		return value
	})
	out.Rules = cloneRuntimeStateRawSlice(in.Rules, func(item string) string { return item })
	out.DynamicParameterPrefixes = cloneRuntimeStateRawSlice(in.DynamicParameterPrefixes, func(item string) string { return item })
	out.RequiredOperationReferences = cloneRuntimeStateRawSlice(in.RequiredOperationReferences, func(item string) string { return item })
	out.BindingAuthorizations = cloneRuntimeStateRawSlice(in.BindingAuthorizations, func(item RuntimeOperationBindingAuthorization) RuntimeOperationBindingAuthorization {
		value := item
		value.AllowedBindingRefs = cloneRuntimeStateRawSlice(item.AllowedBindingRefs, func(ref string) string { return ref })
		return value
	})
	out.AcceptedEffectScopes = cloneRuntimeStateRawSlice(in.AcceptedEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredEffectScopes = cloneRuntimeStateRawSlice(in.RequiredEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.Target.ExactTargetRefs = cloneRuntimeStateRawSlice(in.Target.ExactTargetRefs, func(item string) string { return item })
	out.Target.OwnerScopeTargetRefs = cloneRuntimeStateRawSlice(in.Target.OwnerScopeTargetRefs, func(item string) string { return item })
	out.Target.OwnerSelectorRefs = cloneRuntimeStateRawSlice(in.Target.OwnerSelectorRefs, func(item string) string { return item })
	out.Target.ProspectiveConstraintRefs = cloneRuntimeStateRawSlice(in.Target.ProspectiveConstraintRefs, func(item string) string { return item })
	out.Target.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.Target.ProspectiveTargetRefs, func(item string) string { return item })
	out.PrerequisiteSnapshotRefs = cloneRuntimeStateRawSlice(in.PrerequisiteSnapshotRefs, func(item string) string { return item })
	out.AdmissibilitySnapshotRefs = cloneRuntimeStateRawSlice(in.AdmissibilitySnapshotRefs, func(item string) string { return item })
	out.ValidationRecipes = cloneRuntimeStateRawSlice(in.ValidationRecipes, func(item RuntimeValidationRecipe) RuntimeValidationRecipe {
		return cloneRuntimeValidationRecipeRaw(item)
	})
	return out
}

func cloneRuntimeCandidateAttemptRecordRaw(in RuntimeCandidateAttemptRecord) RuntimeCandidateAttemptRecord {
	out := in
	out.Transaction = cloneAuthorityTransactionRaw(in.Transaction)
	out.AdmissionTargetAuthorizations = cloneRuntimeStateRawSlice(in.AdmissionTargetAuthorizations, func(item RuntimeDispatchOperationTargetAuthorization) RuntimeDispatchOperationTargetAuthorization {
		return cloneRuntimeDispatchOperationTargetAuthorizationRaw(item)
	})
	out.AdmissionCoverageBindings = cloneRuntimeStateRawSlice(in.AdmissionCoverageBindings, func(item CoverageEvidenceBinding) CoverageEvidenceBinding {
		return cloneCoverageEvidenceBindingRaw(item)
	})
	out.AdmissionValidationRecipes = cloneRuntimeStateRawSlice(in.AdmissionValidationRecipes, func(item RuntimeValidationRecipe) RuntimeValidationRecipe {
		return cloneRuntimeValidationRecipeRaw(item)
	})
	out.AdmissionRepairCommitment = cloneRuntimeStateRawPointer(in.AdmissionRepairCommitment, func(item RuntimeDispatchRepairCommitment) RuntimeDispatchRepairCommitment {
		return cloneRuntimeDispatchRepairCommitmentRaw(item)
	})
	out.Proposal = cloneRuntimeStateRawPointer(in.Proposal, func(item RuntimeCandidate) RuntimeCandidate { return cloneRuntimeCandidateRaw(item) })
	out.Feasibilities = cloneRuntimeStateRawSlice(in.Feasibilities, func(item RuntimeCandidateFeasibilityRecord) RuntimeCandidateFeasibilityRecord {
		return cloneRuntimeCandidateFeasibilityRecordRaw(item)
	})
	out.Failure = cloneRuntimeStateRawPointer(in.Failure, func(item RuntimeCandidateFailure) RuntimeCandidateFailure {
		return cloneRuntimeCandidateFailureRaw(item)
	})
	out.Correction = cloneRuntimeStateRawPointer(in.Correction, func(item RuntimeCandidateCorrectionDirective) RuntimeCandidateCorrectionDirective {
		return cloneRuntimeCandidateCorrectionDirectiveRaw(item)
	})
	out.Stale = cloneRuntimeStateRawPointer(in.Stale, func(item RuntimePrerequisiteStale) RuntimePrerequisiteStale {
		return cloneRuntimePrerequisiteStaleRaw(item)
	})
	out.OwnerFeasibility = cloneRuntimeStateRawPointer(in.OwnerFeasibility, func(item RuntimeOwnerFeasibilityObservation) RuntimeOwnerFeasibilityObservation {
		return cloneRuntimeOwnerFeasibilityObservationRaw(item)
	})
	out.BindingContradiction = cloneRuntimeStateRawPointer(in.BindingContradiction, func(item RuntimeBindingContradictionIdentity) RuntimeBindingContradictionIdentity {
		return cloneRuntimeBindingContradictionIdentityRaw(item)
	})
	out.PrerequisiteRequest = cloneRuntimeStateRawPointer(in.PrerequisiteRequest, func(item RuntimeOperationPrerequisiteRequest) RuntimeOperationPrerequisiteRequest {
		return cloneRuntimeOperationPrerequisiteRequestRaw(item)
	})
	return out
}

func cloneRuntimeCandidateCorrectionDirectiveRaw(in RuntimeCandidateCorrectionDirective) RuntimeCandidateCorrectionDirective {
	out := in
	out.Objectives = cloneRuntimeStateRawSlice(in.Objectives, func(item RuntimeCandidateCorrectionObjective) RuntimeCandidateCorrectionObjective { return item })
	out.Witness = cloneRuntimeStateRawPointer(in.Witness, func(item RuntimeCandidateCorrectionWitness) RuntimeCandidateCorrectionWitness {
		return cloneRuntimeCandidateCorrectionWitnessRaw(item)
	})
	return out
}

func cloneRuntimeCandidateCorrectionWitnessRaw(in RuntimeCandidateCorrectionWitness) RuntimeCandidateCorrectionWitness {
	out := in
	out.ParameterEntryEncodings = cloneRuntimeStateRawSlice(in.ParameterEntryEncodings, func(item RuntimeCandidateParameterEntryEncodingViolation) RuntimeCandidateParameterEntryEncodingViolation {
		cloned := item
		cloned.MisplacedParameters = cloneRuntimeStateRawSlice(item.MisplacedParameters, func(parameter RuntimeCandidateMisplacedParameter) RuntimeCandidateMisplacedParameter {
			return parameter
		})
		return cloned
	})
	return out
}

func cloneRuntimeCandidateDocumentRecoveryPolicyRaw(in RuntimeCandidateDocumentRecoveryPolicy) RuntimeCandidateDocumentRecoveryPolicy {
	out := in
	return out
}

func cloneRuntimeCandidateFailureRaw(in RuntimeCandidateFailure) RuntimeCandidateFailure {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.Diagnostic = cloneRuntimeStateRawPointer(in.Diagnostic, func(item RuntimeCandidateFailureDiagnostic) RuntimeCandidateFailureDiagnostic {
		return cloneRuntimeCandidateFailureDiagnosticRaw(item)
	})
	out.ValidationDiagnostic = cloneRuntimeStateRawPointer(in.ValidationDiagnostic, func(item ValidationFailureDiagnostic) ValidationFailureDiagnostic {
		return cloneValidationFailureDiagnosticRaw(item)
	})
	return out
}

func cloneRuntimeCandidateFailureDiagnosticRaw(in RuntimeCandidateFailureDiagnostic) RuntimeCandidateFailureDiagnostic {
	out := in
	out.Allowed = cloneRuntimeStateRawSlice(in.Allowed, func(item string) string { return item })
	out.Required = cloneRuntimeStateRawSlice(in.Required, func(item string) string { return item })
	out.Forbidden = cloneRuntimeStateRawSlice(in.Forbidden, func(item string) string { return item })
	out.ParameterEntryEncodings = cloneRuntimeStateRawSlice(in.ParameterEntryEncodings, func(item RuntimeCandidateParameterEntryEncodingViolation) RuntimeCandidateParameterEntryEncodingViolation {
		cloned := item
		cloned.MisplacedParameters = cloneRuntimeStateRawSlice(item.MisplacedParameters, func(parameter RuntimeCandidateMisplacedParameter) RuntimeCandidateMisplacedParameter {
			return parameter
		})
		return cloned
	})
	out.Objectives = cloneRuntimeStateRawSlice(in.Objectives, func(item RuntimeCandidateCorrectionObjective) RuntimeCandidateCorrectionObjective { return item })
	return out
}

func cloneRuntimeCandidateFeasibilityRecordRaw(in RuntimeCandidateFeasibilityRecord) RuntimeCandidateFeasibilityRecord {
	out := in
	out.Observation = cloneRuntimeStateRawPointer(in.Observation, func(item RuntimeOwnerFeasibilityObservation) RuntimeOwnerFeasibilityObservation {
		return cloneRuntimeOwnerFeasibilityObservationRaw(item)
	})
	out.ReplayProof = cloneRuntimeStateRawPointer(in.ReplayProof, func(item RuntimeOperationReplayProof) RuntimeOperationReplayProof {
		return cloneRuntimeOperationReplayProofRaw(item)
	})
	return out
}

func cloneRuntimeCandidateRequestOutcomeRaw(in RuntimeCandidateRequestOutcome) RuntimeCandidateRequestOutcome {
	out := in
	return out
}

func cloneRuntimeCandidateRequestSessionRaw(in RuntimeCandidateRequestSession) RuntimeCandidateRequestSession {
	out := in
	out.CandidateManifest = cloneRuntimeStateRawPointer(in.CandidateManifest, func(item RuntimeCandidateContractManifest) RuntimeCandidateContractManifest {
		return cloneRuntimeCandidateContractManifestRaw(item)
	})
	out.RepairCommitment = cloneRuntimeStateRawPointer(in.RepairCommitment, func(item RuntimeDispatchRepairCommitment) RuntimeDispatchRepairCommitment {
		return cloneRuntimeDispatchRepairCommitmentRaw(item)
	})
	out.ExpectedDocument = cloneRuntimeStateRawPointer(in.ExpectedDocument, func(item RuntimeExpectedDocumentContract) RuntimeExpectedDocumentContract {
		return cloneRuntimeExpectedDocumentContractRaw(item)
	})
	out.Transaction = cloneAuthorityTransactionRaw(in.Transaction)
	out.LastDocumentFraming = cloneRuntimeStateRawPointer(in.LastDocumentFraming, func(item RuntimeProviderDocumentFraming) RuntimeProviderDocumentFraming {
		return cloneRuntimeProviderDocumentFramingRaw(item)
	})
	out.LastOutcome = cloneRuntimeCandidateRequestOutcomeRaw(in.LastOutcome)
	return out
}

func cloneRuntimeCapabilitiesRaw(in RuntimeCapabilities) RuntimeCapabilities {
	out := in
	out.StateChangeKinds = cloneRuntimeStateRawSlice(in.StateChangeKinds, func(item string) string { return item })
	out.ValidationKinds = cloneRuntimeStateRawSlice(in.ValidationKinds, func(item string) string { return item })
	out.GovernanceKinds = cloneRuntimeStateRawSlice(in.GovernanceKinds, func(item string) string { return item })
	out.PrerequisiteFacts = cloneRuntimeStateRawSlice(in.PrerequisiteFacts, func(item RuntimeCapabilityPrerequisiteFact) RuntimeCapabilityPrerequisiteFact {
		return cloneRuntimeCapabilityPrerequisiteFactRaw(item)
	})
	return out
}

func cloneRuntimeCapabilityPrerequisiteFactRaw(in RuntimeCapabilityPrerequisiteFact) RuntimeCapabilityPrerequisiteFact {
	out := in
	out.Requirement = cloneRuntimeRequirementRaw(in.Requirement)
	return out
}

func cloneRuntimeCapabilityStateSnapshotRaw(in RuntimeCapabilityStateSnapshot) RuntimeCapabilityStateSnapshot {
	out := in
	out.RequiredOperationSequence = cloneRuntimeStateRawSlice(in.RequiredOperationSequence, func(item RuntimeRequiredOperationVariant) RuntimeRequiredOperationVariant {
		return cloneRuntimeRequiredOperationVariantRaw(item)
	})
	out.InvalidatedByReceiptRefs = cloneRuntimeStateRawSlice(in.InvalidatedByReceiptRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeCapabilityUsagePolicyRaw(in RuntimeCapabilityUsagePolicy) RuntimeCapabilityUsagePolicy {
	out := in
	out.TriggerStages = cloneRuntimeStateRawSlice(in.TriggerStages, func(item string) string { return item })
	out.TriggerSignals = cloneRuntimeStateRawSlice(in.TriggerSignals, func(item string) string { return item })
	out.PreferredTools = cloneRuntimeStateRawSlice(in.PreferredTools, func(item string) string { return item })
	out.RequiredAfterKinds = cloneRuntimeStateRawSlice(in.RequiredAfterKinds, func(item string) string { return item })
	out.Notes = cloneRuntimeStateRawSlice(in.Notes, func(item string) string { return item })
	return out
}

func cloneRuntimeContinuationContextRaw(in RuntimeContinuationContext) RuntimeContinuationContext {
	out := in
	out.GovernanceHistory = cloneRuntimeStateRawSlice(in.GovernanceHistory, func(item RuntimeGovernanceHistoryEntry) RuntimeGovernanceHistoryEntry {
		return cloneRuntimeGovernanceHistoryEntryRaw(item)
	})
	return out
}

func cloneRuntimeContinuationFailureRaw(in RuntimeContinuationFailure) RuntimeContinuationFailure {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeConvergencePolicyRaw(in RuntimeConvergencePolicy) RuntimeConvergencePolicy {
	out := in
	return out
}

func cloneRuntimeCoverageDependencyRaw(in RuntimeCoverageDependency) RuntimeCoverageDependency {
	out := in
	out.TriggerEvidenceKinds = cloneRuntimeStateRawSlice(in.TriggerEvidenceKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeCoverageEvidenceRequirementRaw(in RuntimeCoverageEvidenceRequirement) RuntimeCoverageEvidenceRequirement {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetDiscoverySources = cloneRuntimeStateRawSlice(in.TargetDiscoverySources, func(item string) string { return item })
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.AcceptedStepKinds = cloneRuntimeStateRawSlice(in.AcceptedStepKinds, func(item string) string { return item })
	out.AcceptedOperationVariants = cloneRuntimeStateRawSlice(in.AcceptedOperationVariants, func(item RuntimeOperationVariantRef) RuntimeOperationVariantRef {
		return cloneRuntimeOperationVariantRefRaw(item)
	})
	out.RejectedStepKinds = cloneRuntimeStateRawSlice(in.RejectedStepKinds, func(item string) string { return item })
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeCoverageItemRaw(in RuntimeCoverageItem) RuntimeCoverageItem {
	out := in
	out.PrerequisiteCoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.PrerequisiteCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.ReopenDependencies = cloneRuntimeStateRawSlice(in.ReopenDependencies, func(item RuntimeCoverageReopenDependency) RuntimeCoverageReopenDependency {
		return cloneRuntimeCoverageReopenDependencyRaw(item)
	})
	return out
}

func cloneRuntimeCoverageItemFactRaw(in RuntimeCoverageItemFact) RuntimeCoverageItemFact {
	out := in
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	out.PrerequisiteCoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.PrerequisiteCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.ReopenDependencies = cloneRuntimeStateRawSlice(in.ReopenDependencies, func(item RuntimeCoverageReopenDependency) RuntimeCoverageReopenDependency {
		return cloneRuntimeCoverageReopenDependencyRaw(item)
	})
	out.ReopenedByEvidenceRefs = cloneRuntimeStateRawSlice(in.ReopenedByEvidenceRefs, func(item string) string { return item })
	out.CurrentBindingRef = cloneRuntimeStateRawPointer(in.CurrentBindingRef, func(item RuntimeCWIBindingRef) RuntimeCWIBindingRef { return cloneRuntimeCWIBindingRefRaw(item) })
	out.BindingLineage = cloneRuntimeStateRawSlice(in.BindingLineage, func(item RuntimeCWIBindingRef) RuntimeCWIBindingRef { return cloneRuntimeCWIBindingRefRaw(item) })
	out.BindingRetirementLineage = cloneRuntimeStateRawSlice(in.BindingRetirementLineage, func(item RuntimeCWIBindingRetirementLineage) RuntimeCWIBindingRetirementLineage {
		return cloneRuntimeCWIBindingRetirementLineageRaw(item)
	})
	out.ReopenLineage = cloneRuntimeStateRawSlice(in.ReopenLineage, func(item RuntimeCWIReopenLineage) RuntimeCWIReopenLineage {
		return cloneRuntimeCWIReopenLineageRaw(item)
	})
	return out
}

func cloneRuntimeCoverageProofLineageRaw(in RuntimeCoverageProofLineage) RuntimeCoverageProofLineage {
	out := in
	out.SourceProofIdentity = cloneCoverageEvidenceProofIdentityRaw(in.SourceProofIdentity)
	out.CurrentProofIdentity = cloneCoverageEvidenceProofIdentityRaw(in.CurrentProofIdentity)
	out.Gap = cloneRuntimeStateRawPointer(in.Gap, func(item RuntimeCoverageProofLineageGap) RuntimeCoverageProofLineageGap {
		return cloneRuntimeCoverageProofLineageGapRaw(item)
	})
	return out
}

func cloneRuntimeCoverageProofLineageGapRaw(in RuntimeCoverageProofLineageGap) RuntimeCoverageProofLineageGap {
	out := in
	return out
}

func cloneRuntimeCoverageReopenDependencyRaw(in RuntimeCoverageReopenDependency) RuntimeCoverageReopenDependency {
	out := in
	out.TriggerEvidenceKinds = cloneRuntimeStateRawSlice(in.TriggerEvidenceKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeDecisionRaw(in RuntimeDecision) RuntimeDecision {
	out := in
	out.Focus = cloneCoverageFocusRaw(in.Focus)
	out.RequiredOutput = cloneRequiredOutputContractRaw(in.RequiredOutput)
	out.Prerequisites = cloneRuntimeStateRawSlice(in.Prerequisites, func(item RuntimePrerequisite) RuntimePrerequisite { return cloneRuntimePrerequisiteRaw(item) })
	out.AuthoritySnapshot = cloneAuthoritySnapshotRaw(in.AuthoritySnapshot)
	out.Terminal = cloneRuntimeStateRawPointer(in.Terminal, func(item RuntimeTerminalDecision) RuntimeTerminalDecision {
		return cloneRuntimeTerminalDecisionRaw(item)
	})
	return out
}

func cloneRuntimeDefinitionChangeImpactRaw(in RuntimeDefinitionChangeImpact) RuntimeDefinitionChangeImpact {
	out := in
	out.GoalChange = cloneRuntimeDefinitionChangeSetRaw(in.GoalChange)
	out.DoDChange = cloneRuntimeDefinitionChangeSetRaw(in.DoDChange)
	out.StrategyRequirements = cloneRuntimeStateRawSlice(in.StrategyRequirements, func(item string) string { return item })
	return out
}

func cloneRuntimeDefinitionChangeSetRaw(in RuntimeDefinitionChangeSet) RuntimeDefinitionChangeSet {
	out := in
	out.PreviousDefinitionIDs = cloneRuntimeStateRawSlice(in.PreviousDefinitionIDs, func(item string) string { return item })
	out.AllDefinitionIDs = cloneRuntimeStateRawSlice(in.AllDefinitionIDs, func(item string) string { return item })
	out.DefinitionRevisions = cloneRuntimeStateRawSlice(in.DefinitionRevisions, func(item RuntimeDefinitionRevision) RuntimeDefinitionRevision {
		return cloneRuntimeDefinitionRevisionRaw(item)
	})
	return out
}

func cloneRuntimeDefinitionClarificationDispatchRaw(in RuntimeDefinitionClarificationDispatch) RuntimeDefinitionClarificationDispatch {
	out := in
	out.OffendingItemRefs = cloneRuntimeStateRawSlice(in.OffendingItemRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeDefinitionClarificationRequestRecordRaw(in RuntimeDefinitionClarificationRequestRecord) RuntimeDefinitionClarificationRequestRecord {
	out := in
	return out
}

func cloneRuntimeDefinitionClarificationStateRaw(in RuntimeDefinitionClarificationState) RuntimeDefinitionClarificationState {
	out := in
	out.Observation = cloneRuntimeDefinitionGapObservationRaw(in.Observation)
	out.Dispatch = cloneRuntimeStateRawPointer(in.Dispatch, func(item RuntimeDefinitionClarificationDispatch) RuntimeDefinitionClarificationDispatch {
		return cloneRuntimeDefinitionClarificationDispatchRaw(item)
	})
	out.Request = cloneRuntimeStateRawPointer(in.Request, func(item RuntimeDefinitionClarificationRequestRecord) RuntimeDefinitionClarificationRequestRecord {
		return cloneRuntimeDefinitionClarificationRequestRecordRaw(item)
	})
	return out
}

func cloneRuntimeDefinitionContextRaw(in RuntimeDefinitionContext) RuntimeDefinitionContext {
	out := in
	out.CapabilityContract = cloneRuntimeStateRawSlice(in.CapabilityContract, func(item string) string { return item })
	out.ChangeImpact = cloneRuntimeStateRawPointer(in.ChangeImpact, func(item RuntimeDefinitionChangeImpact) RuntimeDefinitionChangeImpact {
		return cloneRuntimeDefinitionChangeImpactRaw(item)
	})
	return out
}

func cloneRuntimeDefinitionFocusRaw(in RuntimeDefinitionFocus) RuntimeDefinitionFocus {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.TargetDiscoverySources = cloneRuntimeStateRawSlice(in.TargetDiscoverySources, func(item string) string { return item })
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.RequiredOwnerFacts = cloneRuntimeStateRawSlice(in.RequiredOwnerFacts, func(item RuntimeOwnerFactRequirement) RuntimeOwnerFactRequirement {
		return cloneRuntimeOwnerFactRequirementRaw(item)
	})
	out.OwnerFactConvergences = cloneRuntimeStateRawSlice(in.OwnerFactConvergences, func(item RuntimeOwnerFactConvergence) RuntimeOwnerFactConvergence {
		return cloneRuntimeOwnerFactConvergenceRaw(item)
	})
	out.ValidationFailureRepairConvergence = cloneRuntimeStateRawPointer(in.ValidationFailureRepairConvergence, func(item RuntimeValidationFailureRepairConvergence) RuntimeValidationFailureRepairConvergence {
		return cloneRuntimeValidationFailureRepairConvergenceRaw(item)
	})
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeDefinitionGapObservationRaw(in RuntimeDefinitionGapObservation) RuntimeDefinitionGapObservation {
	out := in
	out.OffendingItemRefs = cloneRuntimeStateRawSlice(in.OffendingItemRefs, func(item string) string { return item })
	out.OriginalDiagnostic = cloneRuntimeOriginalDiagnosticRaw(in.OriginalDiagnostic)
	return out
}

func cloneRuntimeDefinitionRevisionRaw(in RuntimeDefinitionRevision) RuntimeDefinitionRevision {
	out := in
	return out
}

func cloneRuntimeDefinitionSeedRaw(in RuntimeDefinitionSeed) RuntimeDefinitionSeed {
	out := in
	out.CoverageItems = cloneRuntimeStateRawSlice(in.CoverageItems, func(item RuntimeCoverageItem) RuntimeCoverageItem { return cloneRuntimeCoverageItemRaw(item) })
	out.Slices = cloneRuntimeStateRawSlice(in.Slices, func(item RuntimeAcceptanceSlice) RuntimeAcceptanceSlice { return cloneRuntimeAcceptanceSliceRaw(item) })
	return out
}

func cloneRuntimeDeliverableExpectationRaw(in RuntimeDeliverableExpectation) RuntimeDeliverableExpectation {
	out := in
	return out
}

func cloneRuntimeDeliveryAcknowledgementRaw(in RuntimeDeliveryAcknowledgement) RuntimeDeliveryAcknowledgement {
	out := in
	out.Outcome = cloneTerminalOutcomeRaw(in.Outcome)
	return out
}

func cloneRuntimeDiagnosticRecordRaw(in RuntimeDiagnosticRecord) RuntimeDiagnosticRecord {
	out := in
	return out
}

func cloneRuntimeDispatchOperationTargetAuthorizationRaw(in RuntimeDispatchOperationTargetAuthorization) RuntimeDispatchOperationTargetAuthorization {
	out := in
	out.Target = cloneRuntimeOperationTargetAuthorizationRaw(in.Target)
	return out
}

func cloneRuntimeDispatchRecordRaw(in RuntimeDispatchRecord) RuntimeDispatchRecord {
	out := in
	out.Transaction = cloneAuthorityTransactionRaw(in.Transaction)
	out.Candidate = cloneRuntimeCandidateRaw(in.Candidate)
	out.CoverageBindings = cloneRuntimeStateRawSlice(in.CoverageBindings, func(item CoverageEvidenceBinding) CoverageEvidenceBinding {
		return cloneCoverageEvidenceBindingRaw(item)
	})
	out.OperationTargetAuthorizations = cloneRuntimeStateRawSlice(in.OperationTargetAuthorizations, func(item RuntimeDispatchOperationTargetAuthorization) RuntimeDispatchOperationTargetAuthorization {
		return cloneRuntimeDispatchOperationTargetAuthorizationRaw(item)
	})
	out.ValidationRecipes = cloneRuntimeStateRawSlice(in.ValidationRecipes, func(item RuntimeValidationRecipe) RuntimeValidationRecipe {
		return cloneRuntimeValidationRecipeRaw(item)
	})
	out.RepairCommitment = cloneRuntimeStateRawPointer(in.RepairCommitment, func(item RuntimeDispatchRepairCommitment) RuntimeDispatchRepairCommitment {
		return cloneRuntimeDispatchRepairCommitmentRaw(item)
	})
	out.AssessmentAuthorityInput = cloneRuntimeStateRawPointer(in.AssessmentAuthorityInput, func(item RuntimeAssessmentAuthorityInput) RuntimeAssessmentAuthorityInput {
		return cloneRuntimeAssessmentAuthorityInputRaw(item)
	})
	out.AssessmentAdjudication = cloneRuntimeStateRawPointer(in.AssessmentAdjudication, func(item RuntimeAssessmentAdjudication) RuntimeAssessmentAdjudication {
		return cloneRuntimeAssessmentAdjudicationRaw(item)
	})
	out.OperationAdmissibilities = cloneRuntimeStateRawSlice(in.OperationAdmissibilities, func(item RuntimeOperationAdmissibility) RuntimeOperationAdmissibility {
		return cloneRuntimeOperationAdmissibilityRaw(item)
	})
	out.OperationAdmissibilityObservations = cloneRuntimeStateRawSlice(in.OperationAdmissibilityObservations, func(item RuntimePrerequisiteObservation) RuntimePrerequisiteObservation {
		return cloneRuntimePrerequisiteObservationRaw(item)
	})
	out.OperationPolicies = cloneRuntimeStateRawSlice(in.OperationPolicies, func(item RuntimeOperationPolicyFact) RuntimeOperationPolicyFact {
		return cloneRuntimeOperationPolicyFactRaw(item)
	})
	out.CompletedOperationIDs = cloneRuntimeStateRawSlice(in.CompletedOperationIDs, func(item string) string { return item })
	out.OperationExecutions = cloneRuntimeStateRawSlice(in.OperationExecutions, func(item RuntimeOperationExecutionRecord) RuntimeOperationExecutionRecord {
		return cloneRuntimeOperationExecutionRecordRaw(item)
	})
	return out
}

// CloneRuntimeDispatchRecordRaw detaches a persisted dispatch without
// normalizing any authority representation. Owner DTO conversion must use this
// clone so fresh-load validators observe whitespace, ordering, duplicates and
// nil/empty distinctions exactly as stored.
func CloneRuntimeDispatchRecordRaw(in RuntimeDispatchRecord) RuntimeDispatchRecord {
	return cloneRuntimeDispatchRecordRaw(in)
}

func cloneRuntimeDriveLeaseRaw(in RuntimeDriveLease) RuntimeDriveLease {
	out := in
	return out
}

func cloneRuntimeEquivalentAuthorityInputIdentityRaw(in RuntimeEquivalentAuthorityInputIdentity) RuntimeEquivalentAuthorityInputIdentity {
	out := in
	return out
}

func cloneRuntimeEvidenceContentAddressRaw(in RuntimeEvidenceContentAddress) RuntimeEvidenceContentAddress {
	out := in
	return out
}

func cloneRuntimeEvidencePolicyRaw(in RuntimeEvidencePolicy) RuntimeEvidencePolicy {
	out := in
	out.ArgsContain = cloneRuntimeStateRawSlice(in.ArgsContain, func(item string) string { return item })
	out.RecipeArgs = cloneRuntimeStateRawSlice(in.RecipeArgs, func(item string) string { return item })
	out.RequiredArgs = cloneRuntimeStateRawSlice(in.RequiredArgs, func(item string) string { return item })
	out.ZeroWorkMarkers = cloneRuntimeStateRawSlice(in.ZeroWorkMarkers, func(item string) string { return item })
	out.WorkEvidenceMarkers = cloneRuntimeStateRawSlice(in.WorkEvidenceMarkers, func(item string) string { return item })
	out.EvidenceTags = cloneRuntimeStateRawSlice(in.EvidenceTags, func(item string) string { return item })
	out.CoverageRequirements = cloneRuntimeStateRawSlice(in.CoverageRequirements, func(item RuntimeCoverageEvidenceRequirement) RuntimeCoverageEvidenceRequirement {
		return cloneRuntimeCoverageEvidenceRequirementRaw(item)
	})
	return out
}

func cloneRuntimeExecutionBudgetLedgerRaw(in RuntimeExecutionBudgetLedger) RuntimeExecutionBudgetLedger {
	out := in
	out.Policy = cloneRuntimeExecutionBudgetPolicyRaw(in.Policy)
	out.Allocations = cloneRuntimeStateRawSlice(in.Allocations, func(item RuntimeBudgetAllocation) RuntimeBudgetAllocation {
		return cloneRuntimeBudgetAllocationRaw(item)
	})
	out.Consumptions = cloneRuntimeStateRawSlice(in.Consumptions, func(item RuntimeBudgetConsumptionFact) RuntimeBudgetConsumptionFact {
		return cloneRuntimeBudgetConsumptionFactRaw(item)
	})
	out.Reconciliations = cloneRuntimeStateRawSlice(in.Reconciliations, func(item RuntimeBudgetReconciliationRecord) RuntimeBudgetReconciliationRecord {
		return cloneRuntimeBudgetReconciliationRecordRaw(item)
	})
	return out
}

func cloneRuntimeExecutionBudgetPolicyRaw(in RuntimeExecutionBudgetPolicy) RuntimeExecutionBudgetPolicy {
	out := in
	out.Dimensions = cloneRuntimeStateRawSlice(in.Dimensions, func(item RuntimeBudgetDimensionPolicy) RuntimeBudgetDimensionPolicy {
		return cloneRuntimeBudgetDimensionPolicyRaw(item)
	})
	return out
}

func cloneRuntimeExecutionContractRaw(in RuntimeExecutionContract) RuntimeExecutionContract {
	out := in
	out.CandidateDocumentRecovery = cloneRuntimeCandidateDocumentRecoveryPolicyRaw(in.CandidateDocumentRecovery)
	out.CapabilityUsagePolicies = cloneRuntimeStateRawSlice(in.CapabilityUsagePolicies, func(item RuntimeCapabilityUsagePolicy) RuntimeCapabilityUsagePolicy {
		return cloneRuntimeCapabilityUsagePolicyRaw(item)
	})
	out.ValidationPolicies = cloneRuntimeStateRawSlice(in.ValidationPolicies, func(item RuntimeValidationPolicy) RuntimeValidationPolicy {
		return cloneRuntimeValidationPolicyRaw(item)
	})
	out.ProfileContracts = cloneRuntimeStateRawSlice(in.ProfileContracts, func(item RuntimeProfileContract) RuntimeProfileContract { return cloneRuntimeProfileContractRaw(item) })
	out.AcceptanceSlicePolicy = cloneRuntimeStateRawPointer(in.AcceptanceSlicePolicy, func(item RuntimeAcceptanceSlicePolicy) RuntimeAcceptanceSlicePolicy {
		return cloneRuntimeAcceptanceSlicePolicyRaw(item)
	})
	out.TemplateStagePolicy = cloneRuntimeStateRawPointer(in.TemplateStagePolicy, func(item RuntimeTemplateStagePolicy) RuntimeTemplateStagePolicy {
		return cloneRuntimeTemplateStagePolicyRaw(item)
	})
	return out
}

func cloneRuntimeExpectedDocumentContractRaw(in RuntimeExpectedDocumentContract) RuntimeExpectedDocumentContract {
	out := in
	out.BatchCoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.BatchCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	return out
}

func cloneRuntimeFactsRaw(in RuntimeFacts) RuntimeFacts {
	out := in
	out.DefinitionSeed = cloneRuntimeDefinitionSeedRaw(in.DefinitionSeed)
	out.DefinitionFocuses = cloneRuntimeStateRawSlice(in.DefinitionFocuses, func(item RuntimeDefinitionFocus) RuntimeDefinitionFocus { return cloneRuntimeDefinitionFocusRaw(item) })
	out.Focus = cloneCoverageFocusRaw(in.Focus)
	out.RemainingFocuses = cloneRuntimeStateRawSlice(in.RemainingFocuses, func(item CoverageFocus) CoverageFocus { return cloneCoverageFocusRaw(item) })
	out.CompletedFocuses = cloneRuntimeStateRawSlice(in.CompletedFocuses, func(item CoverageFocus) CoverageFocus { return cloneCoverageFocusRaw(item) })
	out.Evidence = cloneEvidenceLedgerRaw(in.Evidence)
	out.Assessments = cloneRuntimeStateRawSlice(in.Assessments, func(item RuntimeAssessmentFact) RuntimeAssessmentFact { return cloneRuntimeAssessmentFactRaw(item) })
	out.Failure = cloneRuntimeStateRawPointer(in.Failure, func(item FailureFact) FailureFact { return cloneFailureFactRaw(item) })
	out.Prerequisites = clonePrerequisiteStateRaw(in.Prerequisites)
	out.Capabilities = cloneRuntimeCapabilitiesRaw(in.Capabilities)
	out.Resources = cloneRuntimeResourceFactsRaw(in.Resources)
	out.Activation = cloneRuntimeActivationContextRaw(in.Activation)
	out.ReconciliationContract = cloneRuntimeCWIReconciliationContractRaw(in.ReconciliationContract)
	out.StateChange = cloneRuntimeStateChangeLedgerRaw(in.StateChange)
	out.ConvergencePolicy = cloneRuntimeConvergencePolicyRaw(in.ConvergencePolicy)
	out.RecoveryPolicy = cloneRuntimeRecoveryPolicyRaw(in.RecoveryPolicy)
	out.ExecutionBudgetPolicy = cloneRuntimeExecutionBudgetPolicyRaw(in.ExecutionBudgetPolicy)
	out.Recovery = cloneRuntimeRecoveryLedgerRaw(in.Recovery)
	out.Repair = cloneRuntimeRepairLedgerRaw(in.Repair)
	out.RepairDirective = cloneRuntimeStateRawPointer(in.RepairDirective, func(item RuntimeRepairDirective) RuntimeRepairDirective { return cloneRuntimeRepairDirectiveRaw(item) })
	out.Coverage = cloneRuntimeStateRawSlice(in.Coverage, func(item RuntimeCoverageItemFact) RuntimeCoverageItemFact {
		return cloneRuntimeCoverageItemFactRaw(item)
	})
	out.Feasibility = cloneRuntimeStateRawPointer(in.Feasibility, func(item RuntimeFeasibilityFact) RuntimeFeasibilityFact { return cloneRuntimeFeasibilityFactRaw(item) })
	out.Slices = cloneRuntimeStateRawSlice(in.Slices, func(item RuntimeAcceptanceSlice) RuntimeAcceptanceSlice { return cloneRuntimeAcceptanceSliceRaw(item) })
	return out
}

func cloneRuntimeFailureSignatureCountRaw(in RuntimeFailureSignatureCount) RuntimeFailureSignatureCount {
	out := in
	return out
}

func cloneRuntimeFailureTargetObservationRaw(in RuntimeFailureTargetObservation) RuntimeFailureTargetObservation {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.CatalogDependency = cloneRuntimeStateRawPointer(in.CatalogDependency, func(item RuntimeFreshnessDependency) RuntimeFreshnessDependency {
		return cloneRuntimeFreshnessDependencyRaw(item)
	})
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeFeasibilityFactRaw(in RuntimeFeasibilityFact) RuntimeFeasibilityFact {
	out := in
	out.RequiredOperationKinds = cloneRuntimeStateRawSlice(in.RequiredOperationKinds, func(item string) string { return item })
	out.FeasibleOperationKinds = cloneRuntimeStateRawSlice(in.FeasibleOperationKinds, func(item string) string { return item })
	out.MissingCapabilityKinds = cloneRuntimeStateRawSlice(in.MissingCapabilityKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeFeedbackRaw(in RuntimeFeedback) RuntimeFeedback {
	out := in
	out.OriginalDiagnostic = cloneRuntimeStateRawPointer(in.OriginalDiagnostic, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	return out
}

func cloneRuntimeFieldViolationRaw(in RuntimeFieldViolation) RuntimeFieldViolation {
	out := in
	out.Allowed = cloneRuntimeStateRawSlice(in.Allowed, func(item string) string { return item })
	out.Required = cloneRuntimeStateRawSlice(in.Required, func(item string) string { return item })
	out.Forbidden = cloneRuntimeStateRawSlice(in.Forbidden, func(item string) string { return item })
	return out
}

func cloneRuntimeFreshnessDependencyRaw(in RuntimeFreshnessDependency) RuntimeFreshnessDependency {
	out := in
	return out
}

func cloneRuntimeGovernanceHistoryEntryRaw(in RuntimeGovernanceHistoryEntry) RuntimeGovernanceHistoryEntry {
	out := in
	out.Failure = cloneRuntimeStateRawPointer(in.Failure, func(item RuntimeContinuationFailure) RuntimeContinuationFailure {
		return cloneRuntimeContinuationFailureRaw(item)
	})
	return out
}

func cloneRuntimeGovernanceRequestRecordRaw(in RuntimeGovernanceRequestRecord) RuntimeGovernanceRequestRecord {
	out := in
	return out
}

func cloneRuntimeGovernanceRequirementRaw(in RuntimeGovernanceRequirement) RuntimeGovernanceRequirement {
	out := in
	return out
}

func cloneRuntimeGovernanceResolutionRecordRaw(in RuntimeGovernanceResolutionRecord) RuntimeGovernanceResolutionRecord {
	out := in
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeMCPCapabilityContextRaw(in RuntimeMCPCapabilityContext) RuntimeMCPCapabilityContext {
	out := in
	out.Tags = cloneRuntimeStateRawSlice(in.Tags, func(item string) string { return item })
	out.StageHints = cloneRuntimeStateRawSlice(in.StageHints, func(item string) string { return item })
	return out
}

func cloneRuntimeMemoryContextRaw(in RuntimeMemoryContext) RuntimeMemoryContext {
	out := in
	out.Tags = cloneRuntimeStateRawSlice(in.Tags, func(item string) string { return item })
	return out
}

func cloneRuntimeOperationRaw(in RuntimeOperation) RuntimeOperation {
	out := in
	out.DependsOn = cloneRuntimeStateRawSlice(in.DependsOn, func(item string) string { return item })
	out.BindingRefs = cloneRuntimeStateRawSlice(in.BindingRefs, func(item string) string { return item })
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.Parameters = cloneRuntimeStateRawSlice(in.Parameters, func(item OperationParameter) OperationParameter { return cloneOperationParameterRaw(item) })
	return out
}

func cloneRuntimeOperationAdmissibilityRaw(in RuntimeOperationAdmissibility) RuntimeOperationAdmissibility {
	out := in
	out.OperationKinds = cloneRuntimeStateRawSlice(in.OperationKinds, func(item string) string { return item })
	out.OperationVariants = cloneRuntimeStateRawSlice(in.OperationVariants, func(item RuntimeOperationAdmissibilityVariant) RuntimeOperationAdmissibilityVariant {
		return cloneRuntimeOperationAdmissibilityVariantRaw(item)
	})
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.SelectorRefs = cloneRuntimeStateRawSlice(in.SelectorRefs, func(item string) string { return item })
	out.ConstraintRefs = cloneRuntimeStateRawSlice(in.ConstraintRefs, func(item string) string { return item })
	out.ValidationRecipeRefs = cloneRuntimeStateRawSlice(in.ValidationRecipeRefs, func(item string) string { return item })
	out.ValidationSurfaces = cloneRuntimeStateRawSlice(in.ValidationSurfaces, func(item RuntimeValidationSurface) RuntimeValidationSurface {
		return cloneRuntimeValidationSurfaceRaw(item)
	})
	out.OperationPolicies = cloneRuntimeStateRawSlice(in.OperationPolicies, func(item RuntimeOperationPolicyFact) RuntimeOperationPolicyFact {
		return cloneRuntimeOperationPolicyFactRaw(item)
	})
	out.MCPCapabilities = cloneRuntimeStateRawSlice(in.MCPCapabilities, func(item RuntimeMCPCapabilityContext) RuntimeMCPCapabilityContext {
		return cloneRuntimeMCPCapabilityContextRaw(item)
	})
	out.ValidationRecipes = cloneRuntimeStateRawSlice(in.ValidationRecipes, func(item RuntimeValidationRecipe) RuntimeValidationRecipe {
		return cloneRuntimeValidationRecipeRaw(item)
	})
	return out
}

func cloneRuntimeOperationAdmissibilityVariantRaw(in RuntimeOperationAdmissibilityVariant) RuntimeOperationAdmissibilityVariant {
	out := in
	out.AllowedTargetPolicies = cloneRuntimeStateRawSlice(in.AllowedTargetPolicies, func(item RuntimeOperationTargetPolicy) RuntimeOperationTargetPolicy { return item })
	out.SelectorRefs = cloneRuntimeStateRawSlice(in.SelectorRefs, func(item string) string { return item })
	out.ConstraintRefs = cloneRuntimeStateRawSlice(in.ConstraintRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeOperationEvidenceRaw(in RuntimeOperationEvidence) RuntimeOperationEvidence {
	out := in
	out.ContentAddress = cloneRuntimeStateRawPointer(in.ContentAddress, func(item RuntimeEvidenceContentAddress) RuntimeEvidenceContentAddress {
		return cloneRuntimeEvidenceContentAddressRaw(item)
	})
	out.ExitCode = cloneRuntimeStateRawPointer(in.ExitCode, func(item int64) int64 { return item })
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	out.EvidenceTags = cloneRuntimeStateRawSlice(in.EvidenceTags, func(item string) string { return item })
	out.Validation = cloneRuntimeStateRawPointer(in.Validation, func(item RuntimeValidationObservation) RuntimeValidationObservation {
		return cloneRuntimeValidationObservationRaw(item)
	})
	return out
}

func cloneRuntimeOperationExecutionRecordRaw(in RuntimeOperationExecutionRecord) RuntimeOperationExecutionRecord {
	out := in
	out.ReplayProof = cloneRuntimeStateRawPointer(in.ReplayProof, func(item RuntimeOperationReplayProof) RuntimeOperationReplayProof {
		return cloneRuntimeOperationReplayProofRaw(item)
	})
	return out
}

func cloneRuntimeOperationGovernanceBindingRaw(in RuntimeOperationGovernanceBinding) RuntimeOperationGovernanceBinding {
	out := in
	out.Grant = cloneRuntimeStateRawPointer(in.Grant, func(item RuntimeOperationGovernanceGrant) RuntimeOperationGovernanceGrant {
		return cloneRuntimeOperationGovernanceGrantRaw(item)
	})
	return out
}

func cloneRuntimeOperationGovernanceGrantRaw(in RuntimeOperationGovernanceGrant) RuntimeOperationGovernanceGrant {
	out := in
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeOperationParameterMatchRaw(in RuntimeOperationParameterMatch) RuntimeOperationParameterMatch {
	out := in
	out.String = cloneRuntimeStateRawPointer(in.String, func(item string) string { return item })
	out.Boolean = cloneRuntimeStateRawPointer(in.Boolean, func(item bool) bool { return item })
	return out
}

func cloneRuntimeOperationPolicyFactRaw(in RuntimeOperationPolicyFact) RuntimeOperationPolicyFact {
	out := in
	out.ParameterMatches = cloneRuntimeStateRawSlice(in.ParameterMatches, func(item RuntimeOperationParameterMatch) RuntimeOperationParameterMatch {
		return cloneRuntimeOperationParameterMatchRaw(item)
	})
	return out
}

func cloneRuntimeOperationPrerequisiteRequestRaw(in RuntimeOperationPrerequisiteRequest) RuntimeOperationPrerequisiteRequest {
	out := in
	return out
}

func cloneRuntimeOperationReceiptRaw(in RuntimeOperationReceipt) RuntimeOperationReceipt {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.Evidence = cloneRuntimeOperationEvidenceRaw(in.Evidence)
	out.Feedback = cloneRuntimeStateRawPointer(in.Feedback, func(item RuntimeFeedback) RuntimeFeedback { return cloneRuntimeFeedbackRaw(item) })
	out.Governance = cloneRuntimeStateRawPointer(in.Governance, func(item RuntimeGovernanceRequirement) RuntimeGovernanceRequirement {
		return cloneRuntimeGovernanceRequirementRaw(item)
	})
	out.ResourceChanges = cloneRuntimeStateRawSlice(in.ResourceChanges, func(item RuntimeResourceChange) RuntimeResourceChange { return cloneRuntimeResourceChangeRaw(item) })
	out.PrerequisiteStale = cloneRuntimeStateRawPointer(in.PrerequisiteStale, func(item RuntimePrerequisiteStale) RuntimePrerequisiteStale {
		return cloneRuntimePrerequisiteStaleRaw(item)
	})
	return out
}

func cloneRuntimeOperationReplayProofRaw(in RuntimeOperationReplayProof) RuntimeOperationReplayProof {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeOperationTargetAuthorizationRaw(in RuntimeOperationTargetAuthorization) RuntimeOperationTargetAuthorization {
	out := in
	out.ExactTargetRefs = cloneRuntimeStateRawSlice(in.ExactTargetRefs, func(item string) string { return item })
	out.OwnerScopeTargetRefs = cloneRuntimeStateRawSlice(in.OwnerScopeTargetRefs, func(item string) string { return item })
	out.OwnerSelectorRefs = cloneRuntimeStateRawSlice(in.OwnerSelectorRefs, func(item string) string { return item })
	out.ProspectiveConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveConstraintRefs, func(item string) string { return item })
	out.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeOperationVariantRefRaw(in RuntimeOperationVariantRef) RuntimeOperationVariantRef {
	out := in
	return out
}

func cloneRuntimeOriginalDiagnosticRaw(in RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.FieldViolations = cloneRuntimeStateRawSlice(in.FieldViolations, func(item RuntimeFieldViolation) RuntimeFieldViolation { return cloneRuntimeFieldViolationRaw(item) })
	out.SemanticViolations = cloneRuntimeStateRawSlice(in.SemanticViolations, func(item RuntimeSemanticContractViolation) RuntimeSemanticContractViolation {
		return cloneRuntimeSemanticContractViolationRaw(item)
	})
	out.TargetObservation = cloneRuntimeStateRawPointer(in.TargetObservation, func(item RuntimeFailureTargetObservation) RuntimeFailureTargetObservation {
		return cloneRuntimeFailureTargetObservationRaw(item)
	})
	out.ValidationDiagnostic = cloneRuntimeStateRawPointer(in.ValidationDiagnostic, func(item ValidationFailureDiagnostic) ValidationFailureDiagnostic {
		return cloneValidationFailureDiagnosticRaw(item)
	})
	out.ValidationFailureObservation = cloneRuntimeStateRawPointer(in.ValidationFailureObservation, func(item RuntimeValidationFailureObservation) RuntimeValidationFailureObservation {
		return cloneRuntimeValidationFailureObservationRaw(item)
	})
	out.GovernanceDependency = cloneRuntimeStateRawPointer(in.GovernanceDependency, func(item ExternalDependency) ExternalDependency { return cloneExternalDependencyRaw(item) })
	return out
}

func cloneRuntimeOwnerFactConvergenceRaw(in RuntimeOwnerFactConvergence) RuntimeOwnerFactConvergence {
	out := in
	out.Requirement = cloneRuntimeOwnerFactRequirementRaw(in.Requirement)
	out.DependsOnRequirementRefs = cloneRuntimeStateRawSlice(in.DependsOnRequirementRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeOwnerFactProspectiveSatisfactionRaw(in RuntimeOwnerFactProspectiveSatisfaction) RuntimeOwnerFactProspectiveSatisfaction {
	out := in
	out.Requirement = cloneRuntimeOwnerFactRequirementRaw(in.Requirement)
	out.ProspectiveValidationRecipeRefs = cloneRuntimeStateRawSlice(in.ProspectiveValidationRecipeRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeOwnerFactProspectiveSatisfactionObservationRaw(in RuntimeOwnerFactProspectiveSatisfactionObservation) RuntimeOwnerFactProspectiveSatisfactionObservation {
	out := in
	out.Satisfaction = cloneRuntimeStateRawPointer(in.Satisfaction, func(item RuntimeOwnerFactProspectiveSatisfaction) RuntimeOwnerFactProspectiveSatisfaction {
		return cloneRuntimeOwnerFactProspectiveSatisfactionRaw(item)
	})
	out.Stale = cloneRuntimeStateRawPointer(in.Stale, func(item RuntimePrerequisiteStale) RuntimePrerequisiteStale {
		return cloneRuntimePrerequisiteStaleRaw(item)
	})
	return out
}

func cloneRuntimeOwnerFactRequirementRaw(in RuntimeOwnerFactRequirement) RuntimeOwnerFactRequirement {
	out := in
	return out
}

func cloneRuntimeOwnerFeasibilityObservationRaw(in RuntimeOwnerFeasibilityObservation) RuntimeOwnerFeasibilityObservation {
	out := in
	out.OwnerPolicy = cloneRuntimeOwnerPolicyIdentityRaw(in.OwnerPolicy)
	out.Feedback = cloneRuntimeStateRawPointer(in.Feedback, func(item RuntimeFeedback) RuntimeFeedback { return cloneRuntimeFeedbackRaw(item) })
	out.Stale = cloneRuntimeStateRawPointer(in.Stale, func(item RuntimePrerequisiteStale) RuntimePrerequisiteStale {
		return cloneRuntimePrerequisiteStaleRaw(item)
	})
	out.PrerequisiteRequest = cloneRuntimeStateRawPointer(in.PrerequisiteRequest, func(item RuntimeOperationPrerequisiteRequest) RuntimeOperationPrerequisiteRequest {
		return cloneRuntimeOperationPrerequisiteRequestRaw(item)
	})
	return out
}

func cloneRuntimeOwnerPolicyIdentityRaw(in RuntimeOwnerPolicyIdentity) RuntimeOwnerPolicyIdentity {
	out := in
	return out
}

func cloneRuntimePrerequisiteRaw(in RuntimePrerequisite) RuntimePrerequisite {
	out := in
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.RequiredCapabilities = cloneRuntimeStateRawSlice(in.RequiredCapabilities, func(item string) string { return item })
	return out
}

func cloneRuntimePrerequisiteInvalidationRecordRaw(in RuntimePrerequisiteInvalidationRecord) RuntimePrerequisiteInvalidationRecord {
	out := in
	out.PrerequisiteIDs = cloneRuntimeStateRawSlice(in.PrerequisiteIDs, func(item string) string { return item })
	out.ObservationIDs = cloneRuntimeStateRawSlice(in.ObservationIDs, func(item string) string { return item })
	out.InvalidatedDependencies = cloneRuntimeStateRawSlice(in.InvalidatedDependencies, func(item RuntimeFreshnessDependency) RuntimeFreshnessDependency {
		return cloneRuntimeFreshnessDependencyRaw(item)
	})
	out.ResourceChanges = cloneRuntimeStateRawSlice(in.ResourceChanges, func(item RuntimeResourceChange) RuntimeResourceChange { return cloneRuntimeResourceChangeRaw(item) })
	out.Stale = cloneRuntimeStateRawPointer(in.Stale, func(item RuntimePrerequisiteStale) RuntimePrerequisiteStale {
		return cloneRuntimePrerequisiteStaleRaw(item)
	})
	out.RefreshDiagnostic = cloneRuntimeStateRawPointer(in.RefreshDiagnostic, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	return out
}

func cloneRuntimePrerequisiteObservationRaw(in RuntimePrerequisiteObservation) RuntimePrerequisiteObservation {
	out := in
	out.Dependencies = cloneRuntimeStateRawSlice(in.Dependencies, func(item RuntimeFreshnessDependency) RuntimeFreshnessDependency {
		return cloneRuntimeFreshnessDependencyRaw(item)
	})
	return out
}

func cloneRuntimePrerequisiteStaleRaw(in RuntimePrerequisiteStale) RuntimePrerequisiteStale {
	out := in
	return out
}

func cloneRuntimeProfileCapabilityStateRequirementRaw(in RuntimeProfileCapabilityStateRequirement) RuntimeProfileCapabilityStateRequirement {
	out := in
	return out
}

func cloneRuntimeProfileContractRaw(in RuntimeProfileContract) RuntimeProfileContract {
	out := in
	out.AcceptanceDimensions = cloneRuntimeStateRawSlice(in.AcceptanceDimensions, func(item RuntimeAcceptanceDimension) RuntimeAcceptanceDimension {
		return cloneRuntimeAcceptanceDimensionRaw(item)
	})
	out.DeliverableExpectations = cloneRuntimeStateRawSlice(in.DeliverableExpectations, func(item RuntimeDeliverableExpectation) RuntimeDeliverableExpectation {
		return cloneRuntimeDeliverableExpectationRaw(item)
	})
	out.EvidenceRequirements = cloneRuntimeStateRawSlice(in.EvidenceRequirements, func(item RuntimeProfileEvidenceRequirement) RuntimeProfileEvidenceRequirement {
		return cloneRuntimeProfileEvidenceRequirementRaw(item)
	})
	out.CoverageDependencies = cloneRuntimeStateRawSlice(in.CoverageDependencies, func(item RuntimeCoverageDependency) RuntimeCoverageDependency {
		return cloneRuntimeCoverageDependencyRaw(item)
	})
	out.BootstrapRecipes = cloneRuntimeStateRawSlice(in.BootstrapRecipes, func(item RuntimeBootstrapRecipe) RuntimeBootstrapRecipe { return cloneRuntimeBootstrapRecipeRaw(item) })
	return out
}

func cloneRuntimeProfileEvidenceRequirementRaw(in RuntimeProfileEvidenceRequirement) RuntimeProfileEvidenceRequirement {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.TargetDiscoverySources = cloneRuntimeStateRawSlice(in.TargetDiscoverySources, func(item string) string { return item })
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.AcceptedStepKinds = cloneRuntimeStateRawSlice(in.AcceptedStepKinds, func(item string) string { return item })
	out.AcceptedOperationVariants = cloneRuntimeStateRawSlice(in.AcceptedOperationVariants, func(item RuntimeOperationVariantRef) RuntimeOperationVariantRef {
		return cloneRuntimeOperationVariantRefRaw(item)
	})
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	out.RequiredOwnerFacts = cloneRuntimeStateRawSlice(in.RequiredOwnerFacts, func(item RuntimeOwnerFactRequirement) RuntimeOwnerFactRequirement {
		return cloneRuntimeOwnerFactRequirementRaw(item)
	})
	out.OwnerFactConvergences = cloneRuntimeStateRawSlice(in.OwnerFactConvergences, func(item RuntimeOwnerFactConvergence) RuntimeOwnerFactConvergence {
		return cloneRuntimeOwnerFactConvergenceRaw(item)
	})
	out.ValidationFailureRepairConvergence = cloneRuntimeStateRawPointer(in.ValidationFailureRepairConvergence, func(item RuntimeValidationFailureRepairConvergence) RuntimeValidationFailureRepairConvergence {
		return cloneRuntimeValidationFailureRepairConvergenceRaw(item)
	})
	out.CapabilityStateRequirements = cloneRuntimeStateRawSlice(in.CapabilityStateRequirements, func(item RuntimeProfileCapabilityStateRequirement) RuntimeProfileCapabilityStateRequirement {
		return cloneRuntimeProfileCapabilityStateRequirementRaw(item)
	})
	out.RejectedStepKinds = cloneRuntimeStateRawSlice(in.RejectedStepKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeProspectiveTargetClauseRaw(in RuntimeProspectiveTargetClause) RuntimeProspectiveTargetClause {
	out := in
	out.AllOf = cloneRuntimeStateRawSlice(in.AllOf, func(item RuntimeProspectiveTargetPredicate) RuntimeProspectiveTargetPredicate {
		return cloneRuntimeProspectiveTargetPredicateRaw(item)
	})
	return out
}

func cloneRuntimeProspectiveTargetPredicateRaw(in RuntimeProspectiveTargetPredicate) RuntimeProspectiveTargetPredicate {
	out := in
	return out
}

func cloneRuntimeProspectiveTargetSatisfactionConstraintRaw(in RuntimeProspectiveTargetSatisfactionConstraint) RuntimeProspectiveTargetSatisfactionConstraint {
	out := in
	out.AllOf = cloneRuntimeStateRawSlice(in.AllOf, func(item RuntimeProspectiveTargetClause) RuntimeProspectiveTargetClause {
		return cloneRuntimeProspectiveTargetClauseRaw(item)
	})
	return out
}

func cloneRuntimeProspectiveWorkspaceManifestDeltaRaw(in RuntimeProspectiveWorkspaceManifestDelta) RuntimeProspectiveWorkspaceManifestDelta {
	out := in
	out.ProspectiveTargetRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetRefs, func(item string) string { return item })
	out.ResultingTargetRefs = cloneRuntimeStateRawSlice(in.ResultingTargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeProviderDocumentFramingRaw(in RuntimeProviderDocumentFraming) RuntimeProviderDocumentFraming {
	out := in
	return out
}

func cloneRuntimeRecoverableWaitRecordRaw(in RuntimeRecoverableWaitRecord) RuntimeRecoverableWaitRecord {
	out := in
	return out
}

func cloneRuntimeRecoveryLedgerRaw(in RuntimeRecoveryLedger) RuntimeRecoveryLedger {
	out := in
	return out
}

func cloneRuntimeRecoveryPolicyRaw(in RuntimeRecoveryPolicy) RuntimeRecoveryPolicy {
	out := in
	return out
}

func cloneRuntimeRejectedOperationShapeRaw(in RuntimeRejectedOperationShape) RuntimeRejectedOperationShape {
	out := in
	out.Operation = cloneRuntimeOperationRaw(in.Operation)
	return out
}

func cloneRuntimeRepairClosureRaw(in RuntimeRepairClosure) RuntimeRepairClosure {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.GroundedTargetRefs = cloneRuntimeStateRawSlice(in.GroundedTargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeDispatchRepairCommitmentRaw(in RuntimeDispatchRepairCommitment) RuntimeDispatchRepairCommitment {
	out := in
	out.Closure = cloneRuntimeRepairClosureRaw(in.Closure)
	return out
}

func cloneRuntimeRepairDirectiveRaw(in RuntimeRepairDirective) RuntimeRepairDirective {
	out := in
	out.CurrentFailure = cloneRuntimeStateRawPointer(in.CurrentFailure, func(item FailureFact) FailureFact { return cloneFailureFactRaw(item) })
	out.RequiredEffectScopes = cloneRuntimeStateRawSlice(in.RequiredEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.AllowedOperationKinds = cloneRuntimeStateRawSlice(in.AllowedOperationKinds, func(item string) string { return item })
	out.RejectedOperationShapes = cloneRuntimeStateRawSlice(in.RejectedOperationShapes, func(item RuntimeRejectedOperationShape) RuntimeRejectedOperationShape {
		return cloneRuntimeRejectedOperationShapeRaw(item)
	})
	out.NoProgressOperationShapes = cloneRuntimeStateRawSlice(in.NoProgressOperationShapes, func(item RuntimeRejectedOperationShape) RuntimeRejectedOperationShape {
		return cloneRuntimeRejectedOperationShapeRaw(item)
	})
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.Closure = cloneRuntimeStateRawPointer(in.Closure, func(item RuntimeRepairClosure) RuntimeRepairClosure { return cloneRuntimeRepairClosureRaw(item) })
	return out
}

func cloneRuntimeRepairLedgerRaw(in RuntimeRepairLedger) RuntimeRepairLedger {
	out := in
	out.ActiveGap = cloneRuntimeStateRawPointer(in.ActiveGap, func(item RuntimeAssessmentGap) RuntimeAssessmentGap { return cloneRuntimeAssessmentGapRaw(item) })
	out.SignatureCounts = cloneRuntimeStateRawSlice(in.SignatureCounts, func(item RuntimeFailureSignatureCount) RuntimeFailureSignatureCount {
		return cloneRuntimeFailureSignatureCountRaw(item)
	})
	out.NoProgressOperationShapes = cloneRuntimeStateRawSlice(in.NoProgressOperationShapes, func(item RuntimeRejectedOperationShape) RuntimeRejectedOperationShape {
		return cloneRuntimeRejectedOperationShapeRaw(item)
	})
	return out
}

func cloneRuntimeRepairSurfaceConfirmationRaw(in RuntimeRepairSurfaceConfirmation) RuntimeRepairSurfaceConfirmation {
	out := in
	out.Observation = cloneRuntimeValidationFailureObservationRaw(in.Observation)
	out.ConfirmedTargetRefs = cloneRuntimeStateRawSlice(in.ConfirmedTargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeRepairSurfaceConfirmationRecordRaw(in RuntimeRepairSurfaceConfirmationRecord) RuntimeRepairSurfaceConfirmationRecord {
	out := in
	out.Confirmation = cloneRuntimeRepairSurfaceConfirmationRaw(in.Confirmation)
	return out
}

func cloneRuntimeRequiredOperationVariantRaw(in RuntimeRequiredOperationVariant) RuntimeRequiredOperationVariant {
	out := in
	out.DependsOnRequirementIDs = cloneRuntimeStateRawSlice(in.DependsOnRequirementIDs, func(item string) string { return item })
	return out
}

func cloneRuntimeRequirementRaw(in RuntimeRequirement) RuntimeRequirement {
	out := in
	out.TargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.TargetSelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.RequiredCapabilities = cloneRuntimeStateRawSlice(in.RequiredCapabilities, func(item string) string { return item })
	return out
}

func cloneRuntimeResourceChangeRaw(in RuntimeResourceChange) RuntimeResourceChange {
	out := in
	return out
}

func cloneRuntimeResourceFactsRaw(in RuntimeResourceFacts) RuntimeResourceFacts {
	out := in
	out.Manifest = cloneRuntimeStateRawPointer(in.Manifest, func(item WorkspaceManifest) WorkspaceManifest { return cloneWorkspaceManifestRaw(item) })
	out.TargetSnapshots = cloneRuntimeStateRawSlice(in.TargetSnapshots, func(item TargetSnapshot) TargetSnapshot { return cloneTargetSnapshotRaw(item) })
	out.TargetSelection = cloneRuntimeStateRawPointer(in.TargetSelection, func(item RuntimeTargetSelection) RuntimeTargetSelection { return cloneRuntimeTargetSelectionRaw(item) })
	out.FailureTarget = cloneRuntimeStateRawPointer(in.FailureTarget, func(item FailureTarget) FailureTarget { return cloneFailureTargetRaw(item) })
	out.ValidationScratch = cloneRuntimeStateRawPointer(in.ValidationScratch, func(item ValidationScratch) ValidationScratch { return cloneValidationScratchRaw(item) })
	out.ProviderCeiling = cloneRuntimeStateRawPointer(in.ProviderCeiling, func(item ProviderCeiling) ProviderCeiling { return cloneProviderCeilingRaw(item) })
	out.CapabilityStates = cloneRuntimeStateRawSlice(in.CapabilityStates, func(item RuntimeCapabilityStateSnapshot) RuntimeCapabilityStateSnapshot {
		return cloneRuntimeCapabilityStateSnapshotRaw(item)
	})
	out.OperationAdmissibilities = cloneRuntimeStateRawSlice(in.OperationAdmissibilities, func(item RuntimeOperationAdmissibility) RuntimeOperationAdmissibility {
		return cloneRuntimeOperationAdmissibilityRaw(item)
	})
	out.PrerequisiteObservations = cloneRuntimeStateRawSlice(in.PrerequisiteObservations, func(item RuntimePrerequisiteObservation) RuntimePrerequisiteObservation {
		return cloneRuntimePrerequisiteObservationRaw(item)
	})
	out.RetiredPrerequisiteObservations = cloneRuntimeStateRawSlice(in.RetiredPrerequisiteObservations, func(item RuntimePrerequisiteObservation) RuntimePrerequisiteObservation {
		return cloneRuntimePrerequisiteObservationRaw(item)
	})
	return out
}

func cloneRuntimeSemanticContractViolationRaw(in RuntimeSemanticContractViolation) RuntimeSemanticContractViolation {
	out := in
	out.RequiredCoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.RequiredCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.Required = cloneRuntimeStateRawSlice(in.Required, func(item string) string { return item })
	out.Allowed = cloneRuntimeStateRawSlice(in.Allowed, func(item string) string { return item })
	return out
}

func cloneRuntimeSemanticCorrectionDirectiveRaw(in RuntimeSemanticCorrectionDirective) RuntimeSemanticCorrectionDirective {
	out := in
	out.Violations = cloneRuntimeStateRawSlice(in.Violations, func(item RuntimeSemanticContractViolation) RuntimeSemanticContractViolation {
		return cloneRuntimeSemanticContractViolationRaw(item)
	})
	return out
}

func cloneRuntimeSemanticFeasibilityGapRaw(in RuntimeSemanticFeasibilityGap) RuntimeSemanticFeasibilityGap {
	out := in
	out.Feasibilities = cloneRuntimeStateRawSlice(in.Feasibilities, func(item RuntimeSemanticProofFeasibility) RuntimeSemanticProofFeasibility {
		return cloneRuntimeSemanticProofFeasibilityRaw(item)
	})
	out.Rejections = cloneRuntimeStateRawSlice(in.Rejections, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	return out
}

func cloneRuntimeSemanticOwnerFactAcquisitionRaw(in RuntimeSemanticOwnerFactAcquisition) RuntimeSemanticOwnerFactAcquisition {
	out := in
	out.Requirement = cloneRuntimeOwnerFactRequirementRaw(in.Requirement)
	out.ProspectiveTargetSatisfactionConstraint = cloneRuntimeStateRawPointer(in.ProspectiveTargetSatisfactionConstraint, func(item RuntimeProspectiveTargetSatisfactionConstraint) RuntimeProspectiveTargetSatisfactionConstraint {
		return cloneRuntimeProspectiveTargetSatisfactionConstraintRaw(item)
	})
	out.AcquisitionConstraintRefs = cloneRuntimeStateRawSlice(in.AcquisitionConstraintRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeSemanticPlanBatchRaw(in RuntimeSemanticPlanBatch) RuntimeSemanticPlanBatch {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.PendingCoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.PendingCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.DependencyBarrierRefs = cloneRuntimeStateRawSlice(in.DependencyBarrierRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeSemanticPlanBatchFeasibilityResultRaw(in RuntimeSemanticPlanBatchFeasibilityResult) RuntimeSemanticPlanBatchFeasibilityResult {
	out := in
	out.Feasibilities = cloneRuntimeStateRawSlice(in.Feasibilities, func(item RuntimeSemanticProofFeasibility) RuntimeSemanticProofFeasibility {
		return cloneRuntimeSemanticProofFeasibilityRaw(item)
	})
	return out
}

func cloneRuntimeSemanticPlanDocumentRaw(in RuntimeSemanticPlanDocument) RuntimeSemanticPlanDocument {
	out := in
	out.BatchCoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.BatchCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.CoverageOrder = cloneRuntimeStateRawSlice(in.CoverageOrder, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.Proposals = cloneRuntimeStateRawSlice(in.Proposals, func(item RuntimeSemanticProofProposal) RuntimeSemanticProofProposal {
		return cloneRuntimeSemanticProofProposalRaw(item)
	})
	return out
}

func cloneRuntimeSemanticPlanManifestRaw(in RuntimeSemanticPlanManifest) RuntimeSemanticPlanManifest {
	out := in
	out.CoverageItems = cloneRuntimeStateRawSlice(in.CoverageItems, func(item RuntimeCoverageItem) RuntimeCoverageItem { return cloneRuntimeCoverageItemRaw(item) })
	out.AllowedOwnerModuleIDs = cloneRuntimeStateRawSlice(in.AllowedOwnerModuleIDs, func(item string) string { return item })
	out.AllowedOperationKinds = cloneRuntimeStateRawSlice(in.AllowedOperationKinds, func(item string) string { return item })
	out.AllowedTargetPolicies = cloneRuntimeStateRawSlice(in.AllowedTargetPolicies, func(item RuntimeOperationTargetPolicy) RuntimeOperationTargetPolicy { return item })
	out.ProofObligations = cloneRuntimeStateRawSlice(in.ProofObligations, func(item RuntimeSemanticProofObligation) RuntimeSemanticProofObligation {
		return cloneRuntimeSemanticProofObligationRaw(item)
	})
	out.OwnerCatalogs = cloneRuntimeStateRawSlice(in.OwnerCatalogs, func(item RuntimeOperationAdmissibility) RuntimeOperationAdmissibility {
		return cloneRuntimeOperationAdmissibilityRaw(item)
	})
	out.OwnerFactDependencyCatalogs = cloneRuntimeStateRawSlice(in.OwnerFactDependencyCatalogs, func(item RuntimeOperationAdmissibility) RuntimeOperationAdmissibility {
		return cloneRuntimeOperationAdmissibilityRaw(item)
	})
	return out
}

func cloneRuntimeSemanticPlanningGenerationRaw(in RuntimeSemanticPlanningGeneration) RuntimeSemanticPlanningGeneration {
	out := in
	out.InvalidationSignatures = cloneRuntimeStateRawSlice(in.InvalidationSignatures, func(item string) string { return item })
	out.PrerequisiteIDs = cloneRuntimeStateRawSlice(in.PrerequisiteIDs, func(item string) string { return item })
	out.ReopenedCoverageIDs = cloneRuntimeStateRawSlice(in.ReopenedCoverageIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.ReopenTriggerRefs = cloneRuntimeStateRawSlice(in.ReopenTriggerRefs, func(item string) string { return item })
	out.ChangedCoverageIDs = cloneRuntimeStateRawSlice(in.ChangedCoverageIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.ClosureChangeTriggerRefs = cloneRuntimeStateRawSlice(in.ClosureChangeTriggerRefs, func(item string) string { return item })
	out.Snapshot = cloneRuntimeSemanticPlanningLedgerRaw(in.Snapshot)
	return out
}

func cloneRuntimeSemanticPlanningLedgerRaw(in RuntimeSemanticPlanningLedger) RuntimeSemanticPlanningLedger {
	out := in
	out.DefinitionSeed = cloneRuntimeDefinitionSeedRaw(in.DefinitionSeed)
	out.Manifest = cloneRuntimeStateRawPointer(in.Manifest, func(item RuntimeSemanticPlanManifest) RuntimeSemanticPlanManifest {
		return cloneRuntimeSemanticPlanManifestRaw(item)
	})
	out.CorrectionDirective = cloneRuntimeStateRawPointer(in.CorrectionDirective, func(item RuntimeSemanticCorrectionDirective) RuntimeSemanticCorrectionDirective {
		return cloneRuntimeSemanticCorrectionDirectiveRaw(item)
	})
	out.CoverageOrderDocument = cloneRuntimeStateRawPointer(in.CoverageOrderDocument, func(item RuntimeSemanticPlanDocument) RuntimeSemanticPlanDocument {
		return cloneRuntimeSemanticPlanDocumentRaw(item)
	})
	out.Document = cloneRuntimeStateRawPointer(in.Document, func(item RuntimeSemanticPlanDocument) RuntimeSemanticPlanDocument {
		return cloneRuntimeSemanticPlanDocumentRaw(item)
	})
	out.Batches = cloneRuntimeStateRawSlice(in.Batches, func(item RuntimeSemanticPlanBatch) RuntimeSemanticPlanBatch {
		return cloneRuntimeSemanticPlanBatchRaw(item)
	})
	out.PendingCoverageIDs = cloneRuntimeStateRawSlice(in.PendingCoverageIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.CompletedBatchIDs = cloneRuntimeStateRawSlice(in.CompletedBatchIDs, func(item string) string { return item })
	out.BatchFeasibilityResults = cloneRuntimeStateRawSlice(in.BatchFeasibilityResults, func(item RuntimeSemanticPlanBatchFeasibilityResult) RuntimeSemanticPlanBatchFeasibilityResult {
		return cloneRuntimeSemanticPlanBatchFeasibilityResultRaw(item)
	})
	out.BatchDocuments = cloneRuntimeStateRawSlice(in.BatchDocuments, func(item RuntimeSemanticPlanDocument) RuntimeSemanticPlanDocument {
		return cloneRuntimeSemanticPlanDocumentRaw(item)
	})
	out.MergedDocument = cloneRuntimeStateRawPointer(in.MergedDocument, func(item RuntimeSemanticPlanDocument) RuntimeSemanticPlanDocument {
		return cloneRuntimeSemanticPlanDocumentRaw(item)
	})
	out.Feasibilities = cloneRuntimeStateRawSlice(in.Feasibilities, func(item RuntimeSemanticProofFeasibility) RuntimeSemanticProofFeasibility {
		return cloneRuntimeSemanticProofFeasibilityRaw(item)
	})
	out.SealedLineageRefs = cloneRuntimeStateRawSlice(in.SealedLineageRefs, func(item string) string { return item })
	out.Rejections = cloneRuntimeStateRawSlice(in.Rejections, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	out.FeasibilityGaps = cloneRuntimeStateRawSlice(in.FeasibilityGaps, func(item RuntimeSemanticFeasibilityGap) RuntimeSemanticFeasibilityGap {
		return cloneRuntimeSemanticFeasibilityGapRaw(item)
	})
	out.Failure = cloneRuntimeStateRawPointer(in.Failure, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	out.History = cloneRuntimeStateRawSlice(in.History, func(item RuntimeSemanticPlanningGeneration) RuntimeSemanticPlanningGeneration {
		return cloneRuntimeSemanticPlanningGenerationRaw(item)
	})
	return out
}

func cloneRuntimeSemanticProofFeasibilityRaw(in RuntimeSemanticProofFeasibility) RuntimeSemanticProofFeasibility {
	out := in
	out.OwnerPolicy = cloneRuntimeOwnerPolicyIdentityRaw(in.OwnerPolicy)
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	out.ProspectiveWorkspaceManifestDelta = cloneRuntimeStateRawPointer(in.ProspectiveWorkspaceManifestDelta, func(item RuntimeProspectiveWorkspaceManifestDelta) RuntimeProspectiveWorkspaceManifestDelta {
		return cloneRuntimeProspectiveWorkspaceManifestDeltaRaw(item)
	})
	out.OwnerFactProspectiveSatisfactionObservation = cloneRuntimeStateRawPointer(in.OwnerFactProspectiveSatisfactionObservation, func(item RuntimeOwnerFactProspectiveSatisfactionObservation) RuntimeOwnerFactProspectiveSatisfactionObservation {
		return cloneRuntimeOwnerFactProspectiveSatisfactionObservationRaw(item)
	})
	out.OriginalDiagnostic = cloneRuntimeStateRawPointer(in.OriginalDiagnostic, func(item RuntimeOriginalDiagnostic) RuntimeOriginalDiagnostic {
		return cloneRuntimeOriginalDiagnosticRaw(item)
	})
	return out
}

func cloneRuntimeSemanticProofObligationRaw(in RuntimeSemanticProofObligation) RuntimeSemanticProofObligation {
	out := in
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.DeclarationTargetDiscoverySources = cloneRuntimeStateRawSlice(in.DeclarationTargetDiscoverySources, func(item string) string { return item })
	out.DeclarationTargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.DeclarationTargetSelectorCandidateRefs, func(item string) string { return item })
	out.DeclarationProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.DeclarationProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.RequiredOwnerFacts = cloneRuntimeStateRawSlice(in.RequiredOwnerFacts, func(item RuntimeOwnerFactRequirement) RuntimeOwnerFactRequirement {
		return cloneRuntimeOwnerFactRequirementRaw(item)
	})
	out.OwnerFactConvergences = cloneRuntimeStateRawSlice(in.OwnerFactConvergences, func(item RuntimeOwnerFactConvergence) RuntimeOwnerFactConvergence {
		return cloneRuntimeOwnerFactConvergenceRaw(item)
	})
	out.MetricConstraints = cloneRuntimeStateRawSlice(in.MetricConstraints, func(item string) string { return item })
	out.ArtifactRefs = cloneRuntimeStateRawSlice(in.ArtifactRefs, func(item string) string { return item })
	out.OwnerFactAcquisition = cloneRuntimeStateRawPointer(in.OwnerFactAcquisition, func(item RuntimeSemanticOwnerFactAcquisition) RuntimeSemanticOwnerFactAcquisition {
		return cloneRuntimeSemanticOwnerFactAcquisitionRaw(item)
	})
	out.ValidationFailureRepair = cloneRuntimeStateRawPointer(in.ValidationFailureRepair, func(item RuntimeSemanticValidationFailureRepair) RuntimeSemanticValidationFailureRepair {
		return cloneRuntimeSemanticValidationFailureRepairRaw(item)
	})
	return out
}

func cloneRuntimeSemanticProofProposalRaw(in RuntimeSemanticProofProposal) RuntimeSemanticProofProposal {
	out := in
	out.Target = cloneRuntimeOperationTargetAuthorizationRaw(in.Target)
	out.RequiredEvidenceKinds = cloneRuntimeStateRawSlice(in.RequiredEvidenceKinds, func(item string) string { return item })
	out.RequiredEffectScopes = cloneRuntimeStateRawSlice(in.RequiredEffectScopes, func(item RuntimeEffectScope) RuntimeEffectScope { return item })
	out.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeSemanticValidationFailureRepairRaw(in RuntimeSemanticValidationFailureRepair) RuntimeSemanticValidationFailureRepair {
	out := in
	out.TriggerDiagnosticClasses = cloneRuntimeStateRawSlice(in.TriggerDiagnosticClasses, func(item string) string { return item })
	out.RepairOperationKinds = cloneRuntimeStateRawSlice(in.RepairOperationKinds, func(item string) string { return item })
	out.RepairDeclarationTargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.RepairDeclarationTargetSelectorCandidateRefs, func(item string) string { return item })
	out.RepairConstraintRefs = cloneRuntimeStateRawSlice(in.RepairConstraintRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeSkillContextRaw(in RuntimeSkillContext) RuntimeSkillContext {
	out := in
	out.TriggerRules = cloneRuntimeStateRawSlice(in.TriggerRules, func(item string) string { return item })
	out.ReferenceRefs = cloneRuntimeStateRawSlice(in.ReferenceRefs, func(item string) string { return item })
	out.RequiredToolKinds = cloneRuntimeStateRawSlice(in.RequiredToolKinds, func(item string) string { return item })
	out.RequiredMCPServerRefs = cloneRuntimeStateRawSlice(in.RequiredMCPServerRefs, func(item string) string { return item })
	out.RequiredCapabilityRefs = cloneRuntimeStateRawSlice(in.RequiredCapabilityRefs, func(item string) string { return item })
	out.Constraints = cloneRuntimeStateRawSlice(in.Constraints, func(item string) string { return item })
	return out
}

func cloneRuntimeSkillContextBudgetRaw(in RuntimeSkillContextBudget) RuntimeSkillContextBudget {
	out := in
	return out
}

func cloneRuntimeSoulContextRaw(in RuntimeSoulContext) RuntimeSoulContext {
	out := in
	out.BusinessDomains = cloneRuntimeStateRawSlice(in.BusinessDomains, func(item string) string { return item })
	out.AllowedSkillRefs = cloneRuntimeStateRawSlice(in.AllowedSkillRefs, func(item string) string { return item })
	out.AllowedMCPServerRefs = cloneRuntimeStateRawSlice(in.AllowedMCPServerRefs, func(item string) string { return item })
	out.AllowedCapabilityRefs = cloneRuntimeStateRawSlice(in.AllowedCapabilityRefs, func(item string) string { return item })
	out.AllowedCapabilityCategories = cloneRuntimeStateRawSlice(in.AllowedCapabilityCategories, func(item string) string { return item })
	out.AllowedToolKinds = cloneRuntimeStateRawSlice(in.AllowedToolKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeStageSoulBindingRaw(in RuntimeStageSoulBinding) RuntimeStageSoulBinding {
	out := in
	out.Soul = cloneRuntimeSoulContextRaw(in.Soul)
	return out
}

func cloneRuntimeStateChangeLedgerRaw(in RuntimeStateChangeLedger) RuntimeStateChangeLedger {
	out := in
	out.CoverageWorkItemIDs = cloneRuntimeStateRawSlice(in.CoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	out.EvidenceRefs = cloneRuntimeStateRawSlice(in.EvidenceRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeStateRecordRaw(in RuntimeStateRecord) RuntimeStateRecord {
	out := in
	out.Definition = cloneRuntimeDefinitionContextRaw(in.Definition)
	out.Facts = cloneRuntimeFactsRaw(in.Facts)
	out.SemanticPlanning = cloneRuntimeSemanticPlanningLedgerRaw(in.SemanticPlanning)
	out.Reconciliation = cloneRuntimeCWIReconciliationLedgerRaw(in.Reconciliation)
	out.CoverageProofLineage = cloneRuntimeStateRawSlice(in.CoverageProofLineage, func(item RuntimeCoverageProofLineage) RuntimeCoverageProofLineage {
		return cloneRuntimeCoverageProofLineageRaw(item)
	})
	out.Decision = cloneRuntimeDecisionRaw(in.Decision)
	out.ActiveDispatch = cloneRuntimeStateRawPointer(in.ActiveDispatch, func(item RuntimeDispatchRecord) RuntimeDispatchRecord { return cloneRuntimeDispatchRecordRaw(item) })
	out.CompletedDispatches = cloneRuntimeStateRawSlice(in.CompletedDispatches, func(item RuntimeDispatchRecord) RuntimeDispatchRecord { return cloneRuntimeDispatchRecordRaw(item) })
	out.ProcessedEventIDs = cloneRuntimeStateRawSlice(in.ProcessedEventIDs, func(item string) string { return item })
	out.RepairSurfaceConfirmations = cloneRuntimeStateRawSlice(in.RepairSurfaceConfirmations, func(item RuntimeRepairSurfaceConfirmationRecord) RuntimeRepairSurfaceConfirmationRecord {
		return cloneRuntimeRepairSurfaceConfirmationRecordRaw(item)
	})
	out.Diagnostics = cloneRuntimeStateRawSlice(in.Diagnostics, func(item RuntimeDiagnosticRecord) RuntimeDiagnosticRecord {
		return cloneRuntimeDiagnosticRecordRaw(item)
	})
	out.CandidateRequestSessions = cloneRuntimeStateRawSlice(in.CandidateRequestSessions, func(item RuntimeCandidateRequestSession) RuntimeCandidateRequestSession {
		return cloneRuntimeCandidateRequestSessionRaw(item)
	})
	out.CandidateAttempts = cloneRuntimeStateRawSlice(in.CandidateAttempts, func(item RuntimeCandidateAttemptRecord) RuntimeCandidateAttemptRecord {
		return cloneRuntimeCandidateAttemptRecordRaw(item)
	})
	out.ExecutionBudget = cloneRuntimeExecutionBudgetLedgerRaw(in.ExecutionBudget)
	out.RecoverableWaits = cloneRuntimeStateRawSlice(in.RecoverableWaits, func(item RuntimeRecoverableWaitRecord) RuntimeRecoverableWaitRecord {
		return cloneRuntimeRecoverableWaitRecordRaw(item)
	})
	out.OperationRecoveryAuthorizations = cloneRuntimeStateRawSlice(in.OperationRecoveryAuthorizations, func(item RuntimeOperationRecoveryAuthorization) RuntimeOperationRecoveryAuthorization {
		return item
	})
	out.DefinitionClarification = cloneRuntimeStateRawPointer(in.DefinitionClarification, func(item RuntimeDefinitionClarificationState) RuntimeDefinitionClarificationState {
		return cloneRuntimeDefinitionClarificationStateRaw(item)
	})
	out.GovernanceRequests = cloneRuntimeStateRawSlice(in.GovernanceRequests, func(item RuntimeGovernanceRequestRecord) RuntimeGovernanceRequestRecord {
		return cloneRuntimeGovernanceRequestRecordRaw(item)
	})
	out.GovernanceResolutions = cloneRuntimeStateRawSlice(in.GovernanceResolutions, func(item RuntimeGovernanceResolutionRecord) RuntimeGovernanceResolutionRecord {
		return cloneRuntimeGovernanceResolutionRecordRaw(item)
	})
	out.OperationGovernanceBindings = cloneRuntimeStateRawSlice(in.OperationGovernanceBindings, func(item RuntimeOperationGovernanceBinding) RuntimeOperationGovernanceBinding {
		return cloneRuntimeOperationGovernanceBindingRaw(item)
	})
	out.OperationReceipts = cloneRuntimeStateRawSlice(in.OperationReceipts, func(item RuntimeOperationReceipt) RuntimeOperationReceipt {
		return cloneRuntimeOperationReceiptRaw(item)
	})
	out.DeliveryAcknowledgements = cloneRuntimeStateRawSlice(in.DeliveryAcknowledgements, func(item RuntimeDeliveryAcknowledgement) RuntimeDeliveryAcknowledgement {
		return cloneRuntimeDeliveryAcknowledgementRaw(item)
	})
	out.PrerequisiteInvalidations = cloneRuntimeStateRawSlice(in.PrerequisiteInvalidations, func(item RuntimePrerequisiteInvalidationRecord) RuntimePrerequisiteInvalidationRecord {
		return cloneRuntimePrerequisiteInvalidationRecordRaw(item)
	})
	out.DriveLease = cloneRuntimeStateRawPointer(in.DriveLease, func(item RuntimeDriveLease) RuntimeDriveLease { return cloneRuntimeDriveLeaseRaw(item) })
	out.Termination = cloneRuntimeStateRawPointer(in.Termination, func(item RuntimeTerminationRecord) RuntimeTerminationRecord {
		return cloneRuntimeTerminationRecordRaw(item)
	})
	return out
}

func cloneRuntimeTargetSelectionRaw(in RuntimeTargetSelection) RuntimeTargetSelection {
	out := in
	out.SelectorCandidateRefs = cloneRuntimeStateRawSlice(in.SelectorCandidateRefs, func(item string) string { return item })
	out.ProspectiveTargetAllOfConstraintRefs = cloneRuntimeStateRawSlice(in.ProspectiveTargetAllOfConstraintRefs, func(item string) string { return item })
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	out.CandidateTargetRefs = cloneRuntimeStateRawSlice(in.CandidateTargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeTemplateStageRaw(in RuntimeTemplateStage) RuntimeTemplateStage {
	out := in
	out.RequiredOutputs = cloneRuntimeStateRawSlice(in.RequiredOutputs, func(item RuntimeTemplateStageRequiredOutput) RuntimeTemplateStageRequiredOutput {
		return cloneRuntimeTemplateStageRequiredOutputRaw(item)
	})
	out.Completion = cloneRuntimeTemplateStageCompletionRaw(in.Completion)
	out.Governance = cloneRuntimeTemplateStageGovernanceRaw(in.Governance)
	out.PolicyRefs = cloneRuntimeTemplateStagePolicyRefsRaw(in.PolicyRefs)
	return out
}

func cloneRuntimeTemplateStageCompletionRaw(in RuntimeTemplateStageCompletion) RuntimeTemplateStageCompletion {
	out := in
	return out
}

func cloneRuntimeTemplateStageGovernanceRaw(in RuntimeTemplateStageGovernance) RuntimeTemplateStageGovernance {
	out := in
	return out
}

func cloneRuntimeTemplateStagePolicyRaw(in RuntimeTemplateStagePolicy) RuntimeTemplateStagePolicy {
	out := in
	out.Stages = cloneRuntimeStateRawSlice(in.Stages, func(item RuntimeTemplateStage) RuntimeTemplateStage { return cloneRuntimeTemplateStageRaw(item) })
	return out
}

func cloneRuntimeTemplateStagePolicyRefsRaw(in RuntimeTemplateStagePolicyRefs) RuntimeTemplateStagePolicyRefs {
	out := in
	out.EvidencePolicies = cloneRuntimeStateRawSlice(in.EvidencePolicies, func(item string) string { return item })
	out.CapabilityUsagePolicies = cloneRuntimeStateRawSlice(in.CapabilityUsagePolicies, func(item string) string { return item })
	return out
}

func cloneRuntimeTemplateStageRequiredOutputRaw(in RuntimeTemplateStageRequiredOutput) RuntimeTemplateStageRequiredOutput {
	out := in
	out.EvidenceKinds = cloneRuntimeStateRawSlice(in.EvidenceKinds, func(item string) string { return item })
	return out
}

func cloneRuntimeTerminalDecisionRaw(in RuntimeTerminalDecision) RuntimeTerminalDecision {
	out := in
	return out
}

func cloneRuntimeTerminationRecordRaw(in RuntimeTerminationRecord) RuntimeTerminationRecord {
	out := in
	return out
}

func cloneRuntimeValidationFailureObservationRaw(in RuntimeValidationFailureObservation) RuntimeValidationFailureObservation {
	out := in
	out.CandidateTargetRefs = cloneRuntimeStateRawSlice(in.CandidateTargetRefs, func(item string) string { return item })
	out.ValidationScopeTargetRefs = cloneRuntimeStateRawSlice(in.ValidationScopeTargetRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeValidationFailureRepairConvergenceRaw(in RuntimeValidationFailureRepairConvergence) RuntimeValidationFailureRepairConvergence {
	out := in
	out.TriggerDiagnosticClasses = cloneRuntimeStateRawSlice(in.TriggerDiagnosticClasses, func(item string) string { return item })
	out.RepairOperationKinds = cloneRuntimeStateRawSlice(in.RepairOperationKinds, func(item string) string { return item })
	out.RepairDeclarationTargetSelectorCandidateRefs = cloneRuntimeStateRawSlice(in.RepairDeclarationTargetSelectorCandidateRefs, func(item string) string { return item })
	return out
}

func cloneRuntimeValidationObservationRaw(in RuntimeValidationObservation) RuntimeValidationObservation {
	out := in
	out.ExecutedChecks = cloneRuntimeStateRawSlice(in.ExecutedChecks, func(item string) string { return item })
	out.PassedChecks = cloneRuntimeStateRawSlice(in.PassedChecks, func(item string) string { return item })
	out.FailedChecks = cloneRuntimeStateRawSlice(in.FailedChecks, func(item string) string { return item })
	out.SkippedChecks = cloneRuntimeStateRawSlice(in.SkippedChecks, func(item string) string { return item })
	return out
}

func cloneRuntimeValidationPolicyRaw(in RuntimeValidationPolicy) RuntimeValidationPolicy {
	out := in
	out.AppliesToEvidenceKinds = cloneRuntimeStateRawSlice(in.AppliesToEvidenceKinds, func(item string) string { return item })
	out.DiscoverySources = cloneRuntimeStateRawSlice(in.DiscoverySources, func(item string) string { return item })
	out.AllowedExecutionModes = cloneRuntimeStateRawSlice(in.AllowedExecutionModes, func(item string) string { return item })
	out.AllowedValidationModes = cloneRuntimeStateRawSlice(in.AllowedValidationModes, func(item string) string { return item })
	out.Notes = cloneRuntimeStateRawSlice(in.Notes, func(item string) string { return item })
	return out
}

func cloneRuntimeValidationRecipeRaw(in RuntimeValidationRecipe) RuntimeValidationRecipe {
	out := in
	out.Args = cloneRuntimeStateRawSlice(in.Args, func(item string) string { return item })
	return out
}

func cloneRuntimeValidationSurfaceRaw(in RuntimeValidationSurface) RuntimeValidationSurface {
	out := in
	out.ProspectiveTargetSatisfactionConstraint = cloneRuntimeStateRawPointer(in.ProspectiveTargetSatisfactionConstraint, func(item RuntimeProspectiveTargetSatisfactionConstraint) RuntimeProspectiveTargetSatisfactionConstraint {
		return cloneRuntimeProspectiveTargetSatisfactionConstraintRaw(item)
	})
	out.CheckRefs = cloneRuntimeStateRawSlice(in.CheckRefs, func(item string) string { return item })
	return out
}

func cloneStateChangeCandidateRaw(in StateChangeCandidate) StateChangeCandidate {
	out := in
	out.Operations = cloneRuntimeStateRawSlice(in.Operations, func(item StateChangingOperation) StateChangingOperation { return cloneStateChangingOperationRaw(item) })
	return out
}

func cloneStateChangingOperationRaw(in StateChangingOperation) StateChangingOperation {
	out := in
	out.Operation = cloneRuntimeOperationRaw(in.Operation)
	return out
}

func cloneTargetSnapshotRaw(in TargetSnapshot) TargetSnapshot {
	out := in
	return out
}

func cloneTerminalOutcomeRaw(in TerminalOutcome) TerminalOutcome {
	out := in
	return out
}

func cloneValidationCandidateRaw(in ValidationCandidate) ValidationCandidate {
	out := in
	out.Operations = cloneRuntimeStateRawSlice(in.Operations, func(item ValidationOperation) ValidationOperation { return cloneValidationOperationRaw(item) })
	return out
}

func cloneValidationFailureDiagnosticRaw(in ValidationFailureDiagnostic) ValidationFailureDiagnostic {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	return out
}

func cloneValidationOperationRaw(in ValidationOperation) ValidationOperation {
	out := in
	out.Operation = cloneRuntimeOperationRaw(in.Operation)
	return out
}

func cloneValidationScratchRaw(in ValidationScratch) ValidationScratch {
	out := in
	return out
}

func cloneWorkspaceManifestRaw(in WorkspaceManifest) WorkspaceManifest {
	out := in
	out.TargetRefs = cloneRuntimeStateRawSlice(in.TargetRefs, func(item string) string { return item })
	return out
}
