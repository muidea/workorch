package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func StableRuntimeCandidatePrefixHash(definition RuntimeDefinitionContext) string {
	return StableRuntimeCandidatePrefixHashWithExecutionContract(definition, RuntimeExecutionContract{})
}

// StableRuntimeProfileContractsHash seals the normalized profile policy set
// independently from live capability catalogs. Workspace Definition admission
// uses it to prove that the Runtime bootstrap consumes the exact pre-seal
// profile snapshot rather than a later application-side reinterpretation.
func StableRuntimeProfileContractsHash(profiles []RuntimeProfileContract) string {
	hash := StableRuntimeCandidatePrefixHashWithExecutionContract(RuntimeDefinitionContext{}, RuntimeExecutionContract{ProfileContracts: profiles})
	return "runtime-profile-contracts:" + hash
}

func StableRuntimeCandidatePrefixHashWithExecutionContract(definition RuntimeDefinitionContext, execution RuntimeExecutionContract) string {
	definition = definition.Normalized()
	execution = execution.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-candidate-contract")
	writeHashString(h, definition.StartupInputHash)
	writeHashString(h, definition.Revision)
	writeHashString(h, string(definition.GoalID))
	writeHashString(h, definition.GoalDefinition)
	writeHashString(h, string(definition.DoDID))
	writeHashString(h, definition.DoDDefinition)
	writeHashStrings(h, definition.CapabilityContract)
	writeRuntimeDefinitionChangeImpactHash(h, definition.ChangeImpact)
	writeHashString(h, execution.OperationContractRevision)
	writeHashString(h, string(execution.StateChangePolicy))
	writeHashString(h, fmt.Sprintf("%d", execution.MaxRepairAttemptsPerFocus))
	writeHashString(h, fmt.Sprintf("%d", execution.CandidateDocumentRecovery.MaxAttempts))
	if execution.ExecutionLimits != nil {
		writeHashString(h, StableRuntimeExecutionLimitsHash(*execution.ExecutionLimits))
	}
	writeHashString(h, fmt.Sprintf("%d", len(execution.CapabilityUsagePolicies)))
	for _, policy := range execution.CapabilityUsagePolicies {
		writeHashString(h, policy.ID)
		writeHashString(h, policy.Scope)
		writeHashString(h, policy.AppliesToCategory)
		writeHashString(h, policy.EvidenceStrength)
		writeHashString(h, policy.UsageMode)
		writeHashString(h, policy.FallbackPolicy)
		writeHashStrings(h, policy.TriggerStages)
		writeHashStrings(h, policy.TriggerSignals)
		writeHashStrings(h, policy.PreferredTools)
		writeHashStrings(h, policy.RequiredAfterKinds)
		writeHashStrings(h, policy.Notes)
	}
	writeHashString(h, fmt.Sprintf("%d", len(execution.ValidationPolicies)))
	for _, policy := range execution.ValidationPolicies {
		writeHashString(h, policy.ID)
		writeHashString(h, policy.Scope)
		writeHashStrings(h, policy.AppliesToEvidenceKinds)
		writeHashString(h, policy.ValidationSafetyClass)
		writeHashString(h, fmt.Sprintf("%t", policy.DiscoveryRequired))
		writeHashStrings(h, policy.DiscoverySources)
		writeHashStrings(h, policy.AllowedExecutionModes)
		writeHashString(h, fmt.Sprintf("%d", policy.MaxValidationOnlyRetriesPerFocus))
		writeHashString(h, fmt.Sprintf("%t", policy.ValidationScratchRequired))
		writeHashStrings(h, policy.AllowedValidationModes)
		writeHashStrings(h, policy.Notes)
	}
	writeHashString(h, fmt.Sprintf("%d", len(execution.ProfileContracts)))
	for _, profile := range execution.ProfileContracts {
		writeHashString(h, profile.ID)
		writeHashString(h, fmt.Sprintf("%d", len(profile.AcceptanceDimensions)))
		for _, dimension := range profile.AcceptanceDimensions {
			writeHashString(h, dimension.ID)
			writeHashString(h, fmt.Sprintf("%t", dimension.Required))
			writeHashString(h, dimension.Description)
		}
		writeHashString(h, fmt.Sprintf("%d", len(profile.DeliverableExpectations)))
		for _, expectation := range profile.DeliverableExpectations {
			writeHashString(h, expectation.ID)
			writeHashString(h, fmt.Sprintf("%t", expectation.Required))
			writeHashString(h, expectation.EvidenceRole)
			writeHashString(h, expectation.Description)
		}
		writeHashString(h, fmt.Sprintf("%d", len(profile.EvidenceRequirements)))
		for _, requirement := range profile.EvidenceRequirements {
			writeHashString(h, requirement.ID)
			writeHashString(h, requirement.Scope)
			writeHashString(h, requirement.EvidenceRole)
			writeHashString(h, requirement.TargetRole)
			writeHashStrings(h, requirement.TargetRefs)
			writeHashStrings(h, requirement.TargetDiscoverySources)
			writeHashString(h, requirement.TargetSelectorRef)
			writeHashString(h, requirement.TargetSelectorOwnerRef)
			writeHashString(h, requirement.TargetSelectorSubjectRef)
			writeHashStrings(h, requirement.TargetSelectorCandidateRefs)
			writeHashStrings(h, requirement.ProspectiveTargetAllOfConstraintRefs)
			writeHashString(h, fmt.Sprintf("%t", requirement.RequireTargetGrounding))
			writeHashStrings(h, requirement.RequiredEvidenceKinds)
			writeHashString(h, fmt.Sprintf("%t", requirement.RequireDirectEvidence))
			writeHashString(h, fmt.Sprintf("%t", requirement.RequireClaimEvidence))
			writeHashString(h, fmt.Sprintf("%t", requirement.RequireExecutableBehaviorEvidence))
			writeHashString(h, fmt.Sprintf("%t", requirement.RequireCoverageTargets))
			writeHashStrings(h, requirement.AcceptedStepKinds)
			writeHashString(h, fmt.Sprintf("%d", len(requirement.AcceptedOperationVariants)))
			for _, variant := range requirement.AcceptedOperationVariants {
				writeHashString(h, variant.Kind)
				writeHashString(h, variant.VariantID)
			}
			writeHashString(h, requirement.ValidationRecipeID)
			writeHashStrings(h, requirement.RequiredCheckRefs)
			writeHashStrings(h, requirement.MetricConstraints)
			writeHashStrings(h, requirement.ArtifactRefs)
			writeHashString(h, fmt.Sprintf("%d", len(requirement.CapabilityStateRequirements)))
			for _, state := range requirement.CapabilityStateRequirements {
				writeHashString(h, state.OwnerModuleID)
				writeHashString(h, state.CapabilityStateKind)
				writeHashString(h, state.SubjectRef)
			}
			writeHashString(h, fmt.Sprintf("%d", len(requirement.RequiredOwnerFacts)))
			for _, ownerFact := range requirement.RequiredOwnerFacts {
				writeHashString(h, ownerFact.OwnerModuleID)
				writeHashString(h, string(ownerFact.FactKind))
				writeHashString(h, ownerFact.OperationKind)
				writeHashString(h, ownerFact.ValidationProofRequirementHash)
			}
			writeHashString(h, fmt.Sprintf("%d", len(requirement.OwnerFactConvergences)))
			for _, convergence := range requirement.OwnerFactConvergences {
				writeHashString(h, convergence.Requirement.OwnerModuleID)
				writeHashString(h, string(convergence.Requirement.FactKind))
				writeHashString(h, convergence.Requirement.OperationKind)
				writeHashString(h, convergence.Requirement.ValidationProofRequirementHash)
				writeHashString(h, fmt.Sprintf("%t", convergence.RequirePreexisting))
				writeHashString(h, convergence.ProspectiveSatisfactionContractRef)
				writeHashString(h, convergence.MissingTrigger)
				writeHashString(h, convergence.AcquisitionOwnerModuleID)
				writeHashString(h, convergence.AcquisitionOperationKind)
				writeHashString(h, convergence.AcquisitionOperationVariantID)
				writeHashString(h, string(convergence.AcquisitionTargetPolicy))
				writeHashString(h, convergence.AcquisitionTargetSelectorRef)
				writeHashString(h, convergence.TargetSelectorRef)
				writeHashString(h, convergence.TargetSelectorOwnerRef)
				writeHashString(h, convergence.TargetSelectorSubjectRef)
				writeHashString(h, convergence.ReobserveOwnerModuleID)
				writeHashString(h, string(convergence.ReobserveFactKind))
				writeHashString(h, fmt.Sprintf("%d", convergence.MaxAcquisitionAttempts))
				writeHashStrings(h, convergence.DependsOnRequirementRefs)
			}
			if requirement.ValidationFailureRepairConvergence == nil {
				writeHashString(h, "validation-failure-repair:absent")
			} else {
				writeHashString(h, "validation-failure-repair:present")
				writeHashString(h, StableRuntimeValidationFailureRepairConvergenceHash(*requirement.ValidationFailureRepairConvergence))
			}
			if requirement.ImplementationWorkPolicy != nil {
				writeHashString(h, "implementation-work-policy")
				writeHashString(h, StableRuntimeImplementationWorkPolicyHash(*requirement.ImplementationWorkPolicy))
			}
			if requirement.VerificationPolicy != nil {
				writeHashString(h, "verification-policy")
				writeHashString(h, StableRuntimeVerificationPolicyHash(*requirement.VerificationPolicy))
			}
			writeHashStrings(h, requirement.RejectedStepKinds)
			writeHashString(h, requirement.Description)
		}
		writeHashString(h, fmt.Sprintf("%d", len(profile.CoverageDependencies)))
		for _, dependency := range profile.CoverageDependencies {
			writeHashString(h, dependency.ID)
			writeHashString(h, dependency.SourceScope)
			writeHashString(h, dependency.SourceRequirementID)
			writeHashString(h, dependency.DependentScope)
			writeHashString(h, dependency.DependentRequirementID)
			writeHashString(h, fmt.Sprintf("%t", dependency.RequireSourceSettled))
			writeHashStrings(h, dependency.TriggerEvidenceKinds)
		}
		writeHashString(h, fmt.Sprintf("%d", len(profile.BootstrapRecipes)))
		for _, recipe := range profile.BootstrapRecipes {
			writeHashString(h, recipe.ID)
			writeHashStrings(h, recipe.RequiredGoalTerms)
			writeHashStrings(h, recipe.RequiredDoDTerms)
			writeHashString(h, fmt.Sprintf("%d", len(recipe.Artifacts)))
			for _, artifact := range recipe.Artifacts {
				writeHashString(h, artifact.ID)
				writeHashString(h, artifact.TargetRole)
				writeHashStrings(h, artifact.AllowedOperationKinds)
				writeHashStrings(h, artifact.DependsOnArtifactIDs)
			}
		}
	}
	if execution.AcceptanceSlicePolicy == nil {
		writeHashString(h, "acceptance-slice-policy:absent")
	} else {
		policy := execution.AcceptanceSlicePolicy.Normalized()
		writeHashString(h, "acceptance-slice-policy:present")
		writeHashString(h, policy.SchemaVersion)
		writeHashString(h, fmt.Sprintf("%d", policy.MaxWorkItems))
		writeHashString(h, fmt.Sprintf("%d", policy.MaxFocuses))
		writeHashString(h, fmt.Sprintf("%d", policy.MaxCandidateContextItems))
		writeHashString(h, policy.CompletionPolicy)
		writeHashString(h, policy.ResultPolicy)
		writeHashString(h, fmt.Sprintf("%d", len(policy.Groups)))
		for _, group := range policy.Groups {
			writeHashString(h, group.ID)
			writeHashString(h, group.Title)
			writeHashString(h, group.Scope)
			writeHashString(h, fmt.Sprintf("%d", group.Priority))
			writeHashString(h, fmt.Sprintf("%d", group.MaxWorkItems))
			writeHashStrings(h, group.Match.EvidenceRoles)
			writeHashStrings(h, group.Match.MissingEvidenceKinds)
			writeHashStrings(h, group.Match.SourceRequirementIDs)
			writeHashStrings(h, group.Match.SourceScopes)
			writeHashString(h, fmt.Sprintf("%t", group.Match.All))
		}
	}
	if execution.TemplateStagePolicy == nil {
		writeHashString(h, "template-stage-policy:absent")
	} else {
		policy := execution.TemplateStagePolicy.Normalized()
		writeHashString(h, "template-stage-policy:present")
		writeHashString(h, policy.SchemaVersion)
		writeHashString(h, fmt.Sprintf("%d", len(policy.Stages)))
		for _, stage := range policy.Stages {
			writeHashString(h, stage.ID)
			writeHashString(h, stage.SoulBinding)
			writeHashString(h, fmt.Sprintf("%d", len(stage.RequiredOutputs)))
			for _, output := range stage.RequiredOutputs {
				writeHashString(h, output.Kind)
				writeHashStrings(h, output.EvidenceKinds)
			}
			writeHashString(h, stage.Completion.Policy)
			writeHashString(h, stage.Governance.DecisionMode)
			writeHashString(h, stage.Governance.ApprovalMode)
			writeHashStrings(h, stage.PolicyRefs.EvidencePolicies)
			writeHashStrings(h, stage.PolicyRefs.CapabilityUsagePolicies)
		}
	}
	return "candidate-prefix-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeDefinitionContext 是 Runtime 初始化时封存的 Goal/DoD 定义输入。
// 后续 candidate request 和 restart recovery 只能读取该持久化快照，不能临时重建或替换。
type RuntimeDefinitionContext struct {
	// StartupInputHash binds the formal Goal/DoD, Template snapshot and resolved
	// project directory promoted by the Workspace owner. It is immutable for the
	// lifetime of one Run and Resume must never replace it.
	StartupInputHash   string
	Revision           string
	GoalID             common.GoalID
	GoalDefinition     string
	DoDID              common.DoDID
	DoDDefinition      string
	CapabilityContract []string
	ChangeImpact       *RuntimeDefinitionChangeImpact
}

func (c RuntimeDefinitionContext) Normalized() RuntimeDefinitionContext {
	c.StartupInputHash = strings.TrimSpace(c.StartupInputHash)
	c.Revision = strings.TrimSpace(c.Revision)
	c.GoalDefinition = strings.TrimSpace(c.GoalDefinition)
	c.DoDDefinition = strings.TrimSpace(c.DoDDefinition)
	c.CapabilityContract = normalizeStrings(c.CapabilityContract)
	if c.ChangeImpact != nil {
		impact := c.ChangeImpact.Normalized()
		c.ChangeImpact = &impact
	}
	return c
}

func (c RuntimeDefinitionContext) Validate() error {
	c = c.Normalized()
	if c.StartupInputHash == "" || c.Revision == "" || c.GoalID == "" || c.DoDID == "" {
		return fmt.Errorf("runtime definition context requires revision, goal and DoD identity")
	}
	if c.GoalDefinition == "" || c.DoDDefinition == "" {
		return fmt.Errorf("runtime definition context requires Goal and DoD definitions")
	}
	if len(c.CapabilityContract) == 0 {
		return fmt.Errorf("runtime definition context requires a capability contract")
	}
	if c.ChangeImpact != nil {
		if err := c.ChangeImpact.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c RuntimeDefinitionContext) Clone() RuntimeDefinitionContext {
	c = c.Normalized()
	c.CapabilityContract = append([]string(nil), c.CapabilityContract...)
	return c
}

func SameRuntimeDefinitionContext(left RuntimeDefinitionContext, right RuntimeDefinitionContext) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.StartupInputHash != right.StartupInputHash || left.Revision != right.Revision || left.GoalID != right.GoalID || left.GoalDefinition != right.GoalDefinition ||
		left.DoDID != right.DoDID || left.DoDDefinition != right.DoDDefinition || len(left.CapabilityContract) != len(right.CapabilityContract) ||
		!SameRuntimeDefinitionChangeImpact(left.ChangeImpact, right.ChangeImpact) {
		return false
	}
	for idx := range left.CapabilityContract {
		if left.CapabilityContract[idx] != right.CapabilityContract[idx] {
			return false
		}
	}
	return true
}

func writeRuntimeDefinitionChangeImpactHash(h hashStringWriter, impact *RuntimeDefinitionChangeImpact) {
	if impact == nil {
		writeHashString(h, "definition-change-impact:absent")
		return
	}
	value := impact.Normalized()
	writeHashString(h, "definition-change-impact:present")
	writeHashString(h, value.Version)
	writeHashString(h, fmt.Sprintf("%t", value.Changed))
	writeHashString(h, value.Policy)
	writeHashString(h, fmt.Sprintf("%t", value.RequiredPreExecutionAssessment))
	writeRuntimeDefinitionChangeSetHash(h, value.GoalChange)
	writeRuntimeDefinitionChangeSetHash(h, value.DoDChange)
	for _, requirement := range value.StrategyRequirements {
		writeHashString(h, requirement)
	}
}

func writeRuntimeDefinitionChangeSetHash(h hashStringWriter, value RuntimeDefinitionChangeSet) {
	writeHashString(h, value.EffectiveDefinitionID)
	writeHashString(h, value.LatestDefinitionID)
	for _, id := range value.PreviousDefinitionIDs {
		writeHashString(h, id)
	}
	for _, id := range value.AllDefinitionIDs {
		writeHashString(h, id)
	}
	writeHashString(h, fmt.Sprintf("%t", value.ChangedInPlace))
	writeHashString(h, fmt.Sprintf("%t", value.AllEntriesConstrain))
	for _, revision := range value.DefinitionRevisions {
		writeHashString(h, revision.ID)
		writeHashString(h, fmt.Sprintf("%d", revision.RevisionCount))
		writeHashString(h, revision.Source)
		writeHashString(h, revision.DefinitionRevision)
		writeHashString(h, revision.PreviousDefinitionHash)
		writeHashString(h, revision.PreviousDefinitionID)
		writeHashString(h, revision.LastEventID)
		writeHashString(h, revision.LastOccurredAt)
	}
}
