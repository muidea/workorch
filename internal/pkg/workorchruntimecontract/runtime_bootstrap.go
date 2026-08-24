package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const InitialRuntimeEvidenceRevision = "runtime-evidence-00000000000000000000"

type RuntimeBootstrap struct {
	Definition RuntimeDefinitionContext
	Activation RuntimeActivationContext
	// DefinitionSeed is the complete activation input for semantic planning.
	// It carries no proof binding or owner feasibility authority.
	DefinitionSeed         RuntimeDefinitionSeed
	DefinitionFocuses      []RuntimeDefinitionFocus
	Slices                 []RuntimeAcceptanceSlice
	CoverageItems          []RuntimeCoverageItem
	Requirements           []RuntimeRequirement
	StateChangeRequirement RuntimeRunStateChangeRequirement
	ConvergencePolicy      RuntimeConvergencePolicy
	RecoveryPolicy         RuntimeRecoveryPolicy
	ExecutionBudgetPolicy  RuntimeExecutionBudgetPolicy
}

func (b RuntimeBootstrap) Normalized() RuntimeBootstrap {
	b.Definition = b.Definition.Normalized()
	b.Activation = b.Activation.Normalized()
	b.DefinitionSeed = b.DefinitionSeed.Normalized()
	b.DefinitionFocuses = NormalizeRuntimeDefinitionFocuses(b.DefinitionFocuses)
	b.Slices = NormalizeRuntimeAcceptanceSlices(b.Slices)
	b.CoverageItems = NormalizeRuntimeCoverageItems(b.CoverageItems)
	b.Requirements = normalizeRuntimeRequirements(b.Requirements)
	b.StateChangeRequirement = b.StateChangeRequirement.Normalized()
	b.ConvergencePolicy = b.ConvergencePolicy.Normalized()
	b.RecoveryPolicy = b.RecoveryPolicy.Normalized()
	b.ExecutionBudgetPolicy = b.ExecutionBudgetPolicy.Normalized()
	return b
}

func (b RuntimeBootstrap) Validate() error {
	b = b.Normalized()
	if err := b.Definition.Validate(); err != nil {
		return err
	}
	if err := b.Activation.Validate(); err != nil {
		return err
	}
	if err := b.DefinitionSeed.Validate(); err != nil {
		return fmt.Errorf("runtime bootstrap definition seed: %w", err)
	}
	if b.DefinitionSeed.DefinitionRevision != b.Definition.Revision {
		return fmt.Errorf("runtime bootstrap definition seed revision mismatch")
	}
	if err := b.ConvergencePolicy.Validate(); err != nil {
		return err
	}
	if err := b.RecoveryPolicy.Validate(); err != nil {
		return err
	}
	if err := b.ExecutionBudgetPolicy.Validate(); err != nil {
		return err
	}
	if err := ValidateRuntimeExecutionBudgetPolicySources(b.ExecutionBudgetPolicy, b.ConvergencePolicy, b.RecoveryPolicy); err != nil {
		return err
	}
	if len(b.DefinitionFocuses) == 0 {
		return fmt.Errorf("runtime bootstrap requires at least one definition focus")
	}
	if len(b.DefinitionFocuses) > b.ConvergencePolicy.MaxFocuses {
		return fmt.Errorf("runtime bootstrap has %d definition focuses, exceeding sealed limit %d", len(b.DefinitionFocuses), b.ConvergencePolicy.MaxFocuses)
	}
	if len(b.CoverageItems) == 0 {
		return fmt.Errorf("runtime bootstrap requires explicit coverage items")
	}
	coverageItems := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(b.CoverageItems))
	for _, item := range b.CoverageItems {
		if _, exists := coverageItems[item.ID]; exists {
			return fmt.Errorf("runtime bootstrap coverage item %s is duplicated", item.ID)
		}
		coverageItems[item.ID] = item
	}
	if err := ValidateRuntimeCoverageItemDependencies(b.CoverageItems); err != nil {
		return fmt.Errorf("runtime bootstrap coverage dependencies: %w", err)
	}
	if len(b.DefinitionSeed.CoverageItems) != len(b.CoverageItems) {
		return fmt.Errorf("runtime bootstrap definition seed coverage inventory mismatch")
	}
	for idx := range b.CoverageItems {
		if !sameRuntimeCoverageItem(b.DefinitionSeed.CoverageItems[idx], b.CoverageItems[idx]) {
			return fmt.Errorf("runtime bootstrap definition seed coverage item %q mismatch", b.CoverageItems[idx].ID)
		}
	}
	if len(b.DefinitionSeed.Slices) != len(b.Slices) {
		return fmt.Errorf("runtime bootstrap definition seed slice inventory mismatch")
	}
	for idx := range b.Slices {
		if !sameRuntimeAcceptanceSlice(b.DefinitionSeed.Slices[idx], b.Slices[idx]) {
			return fmt.Errorf("runtime bootstrap definition seed slice %q mismatch", b.Slices[idx].ID)
		}
	}
	if err := ValidateRuntimeDefinitionFocuses(b.DefinitionSeed, b.DefinitionFocuses); err != nil {
		return fmt.Errorf("runtime bootstrap definition focuses: %w", err)
	}
	slices := make(map[string]RuntimeAcceptanceSlice, len(b.Slices))
	for _, slice := range b.Slices {
		if slice.ID == "" || len(slice.CoverageWorkItemIDs) == 0 {
			return fmt.Errorf("runtime bootstrap slice requires identity and coverage")
		}
		if len(slice.CoverageWorkItemIDs) > b.ConvergencePolicy.MaxAcceptanceSliceWorkItems || len(slice.CoverageWorkItemIDs) > b.ConvergencePolicy.MaxCandidateContextItems {
			return fmt.Errorf("runtime bootstrap slice %s exceeds sealed execution budget", slice.ID)
		}
		if _, exists := slices[slice.ID]; exists {
			return fmt.Errorf("runtime bootstrap slice %s is duplicated", slice.ID)
		}
		slices[slice.ID] = slice
	}
	seen := map[string]struct{}{}
	coveredByFocus := map[common.CoverageWorkItemID]struct{}{}
	for _, focus := range b.DefinitionFocuses {
		if focus.ID == "" || len(focus.CoverageWorkItemIDs) == 0 || (len(focus.RequiredEvidenceKinds) == 0 && !focus.RequireAssessment) {
			return fmt.Errorf("runtime bootstrap focus requires identity, coverage and evidence or assessment")
		}
		// A presentation slice may contain multiple CWI, but Runtime authority
		// deliberately projects one focus per CWI. The later semantic binding,
		// receipt, evidence, reopen and reconciliation ledgers all use that same
		// identity, so accepting a shared bootstrap focus would create an
		// ambiguous current-binding owner before the Run can persist it.
		if len(focus.CoverageWorkItemIDs) != 1 {
			return fmt.Errorf("runtime bootstrap focus %s must contain exactly one coverage item", focus.ID)
		}
		if _, ok := seen[focus.ID]; ok {
			return fmt.Errorf("runtime bootstrap focus %s is duplicated", focus.ID)
		}
		if len(focus.CoverageWorkItemIDs) > b.ConvergencePolicy.MaxAcceptanceSliceWorkItems || len(focus.CoverageWorkItemIDs) > b.ConvergencePolicy.MaxCandidateContextItems {
			return fmt.Errorf("runtime bootstrap focus %s exceeds sealed execution budget", focus.ID)
		}
		slice, ok := slices[focus.SliceID]
		if !ok || !coverageWorkItemIDsContain(slice.CoverageWorkItemIDs, focus.CoverageWorkItemIDs) {
			return fmt.Errorf("runtime bootstrap focus %s is outside its declared presentation slice", focus.ID)
		}
		for _, id := range focus.CoverageWorkItemIDs {
			item, ok := coverageItems[id]
			if !ok {
				return fmt.Errorf("runtime bootstrap focus %s coverage item %s is not declared", focus.ID, id)
			}
			if item.SliceID != focus.SliceID {
				return fmt.Errorf("runtime bootstrap coverage item %s is assigned to a different slice", id)
			}
			if _, duplicate := coveredByFocus[id]; duplicate {
				return fmt.Errorf("runtime bootstrap coverage item %s appears in more than one focus", id)
			}
			coveredByFocus[id] = struct{}{}
		}
		seen[focus.ID] = struct{}{}
	}
	if len(coveredByFocus) != len(coverageItems) {
		return fmt.Errorf("runtime bootstrap coverage inventory is not fully represented by focuses")
	}
	return nil
}

func coverageWorkItemIDsContain(container, contained []common.CoverageWorkItemID) bool {
	if len(contained) == 0 || len(container) < len(contained) {
		return false
	}
	available := make(map[common.CoverageWorkItemID]struct{}, len(container))
	for _, id := range container {
		available[id] = struct{}{}
	}
	for _, id := range contained {
		if _, ok := available[id]; !ok {
			return false
		}
	}
	return true
}

func (b RuntimeBootstrap) Clone() RuntimeBootstrap {
	b = b.Normalized()
	b.Definition = b.Definition.Clone()
	b.Activation = b.Activation.Clone()
	b.DefinitionSeed = b.DefinitionSeed.Clone()
	b.DefinitionFocuses = CloneRuntimeDefinitionFocuses(b.DefinitionFocuses)
	b.Slices = append([]RuntimeAcceptanceSlice(nil), b.Slices...)
	for idx := range b.Slices {
		b.Slices[idx] = b.Slices[idx].Normalized()
	}
	b.CoverageItems = append([]RuntimeCoverageItem(nil), b.CoverageItems...)
	for idx := range b.CoverageItems {
		b.CoverageItems[idx].PrerequisiteCoverageWorkItemIDs = cloneRuntimeCoveragePrerequisiteIDs(b.CoverageItems[idx].PrerequisiteCoverageWorkItemIDs)
		b.CoverageItems[idx].ReopenDependencies = cloneRuntimeCoverageReopenDependencies(b.CoverageItems[idx].ReopenDependencies)
	}
	b.Requirements = append([]RuntimeRequirement(nil), b.Requirements...)
	for idx := range b.Requirements {
		b.Requirements[idx].RequiredCapabilities = append([]string(nil), b.Requirements[idx].RequiredCapabilities...)
	}
	return b
}

func (b RuntimeBootstrap) InitialFacts(workspaceID common.WorkspaceID, runID common.RunID) (RuntimeFacts, error) {
	b = b.Clone()
	if workspaceID == "" || runID == "" {
		return RuntimeFacts{}, common.ErrInvalidArgument
	}
	if err := b.Validate(); err != nil {
		return RuntimeFacts{}, err
	}
	focusSeeds := RuntimeFocusSeedsFromDefinitionFocuses(b.DefinitionFocuses)
	facts := RuntimeFacts{
		WorkspaceID: workspaceID, RunID: runID, DefinitionRevision: b.Definition.Revision,
		DefinitionSeed: b.DefinitionSeed.Clone(), DefinitionFocuses: CloneRuntimeDefinitionFocuses(b.DefinitionFocuses),
		EvidenceRevision: InitialRuntimeEvidenceRevision, Focus: focusSeeds[0],
		RemainingFocuses:      append([]CoverageFocus(nil), focusSeeds[1:]...),
		Activation:            b.Activation.Clone(),
		StateChange:           NewRuntimeStateChangeLedger(b.StateChangeRequirement),
		ConvergencePolicy:     b.ConvergencePolicy.Normalized(),
		RecoveryPolicy:        b.RecoveryPolicy.Normalized(),
		ExecutionBudgetPolicy: b.ExecutionBudgetPolicy.Clone(),
		Recovery:              RuntimeRecoveryLedger{},
		Repair:                RuntimeRepairLedger{},
		Coverage:              InitialRuntimeCoverageFacts(b.CoverageItems),
		Slices:                append([]RuntimeAcceptanceSlice(nil), b.Slices...),
	}
	return InitializeRuntimeCWILedgerForActivation(facts)
}

func SameRuntimeBootstrap(left RuntimeBootstrap, right RuntimeBootstrap) bool {
	left = left.Normalized()
	right = right.Normalized()
	if !SameRuntimeDefinitionContext(left.Definition, right.Definition) || !SameRuntimeActivationContext(left.Activation, right.Activation) ||
		left.DefinitionSeed.Hash != right.DefinitionSeed.Hash ||
		len(left.DefinitionFocuses) != len(right.DefinitionFocuses) || len(left.Slices) != len(right.Slices) || len(left.CoverageItems) != len(right.CoverageItems) ||
		len(left.Requirements) != len(right.Requirements) ||
		left.StateChangeRequirement.Required != right.StateChangeRequirement.Required ||
		!sameCoverageWorkItemIDs(left.StateChangeRequirement.CoverageWorkItemIDs, right.StateChangeRequirement.CoverageWorkItemIDs) ||
		left.ConvergencePolicy != right.ConvergencePolicy || left.RecoveryPolicy != right.RecoveryPolicy ||
		!SameRuntimeExecutionBudgetPolicy(left.ExecutionBudgetPolicy, right.ExecutionBudgetPolicy) {
		return false
	}
	for idx := range left.DefinitionFocuses {
		if !sameRuntimeDefinitionFocus(left.DefinitionFocuses[idx], right.DefinitionFocuses[idx]) {
			return false
		}
	}
	for idx := range left.Slices {
		if !sameRuntimeAcceptanceSlice(left.Slices[idx], right.Slices[idx]) {
			return false
		}
	}
	for idx := range left.CoverageItems {
		if !sameRuntimeCoverageItem(left.CoverageItems[idx], right.CoverageItems[idx]) {
			return false
		}
	}
	for idx := range left.Requirements {
		if !sameRuntimeRequirement(left.Requirements[idx], right.Requirements[idx]) {
			return false
		}
	}
	return true
}

func sameRuntimeCoverageItem(left, right RuntimeCoverageItem) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.ID != right.ID || left.SliceID != right.SliceID || left.SourceScope != right.SourceScope ||
		left.SourceID != right.SourceID || left.TargetText != right.TargetText || left.Order != right.Order ||
		!sameCoverageWorkItemIDs(left.PrerequisiteCoverageWorkItemIDs, right.PrerequisiteCoverageWorkItemIDs) ||
		len(left.ReopenDependencies) != len(right.ReopenDependencies) {
		return false
	}
	for idx := range left.ReopenDependencies {
		l, r := left.ReopenDependencies[idx], right.ReopenDependencies[idx]
		if l.CoverageWorkItemID != r.CoverageWorkItemID || !sameStrings(l.TriggerEvidenceKinds, r.TriggerEvidenceKinds) {
			return false
		}
	}
	return true
}

func sameRuntimeAcceptanceSlice(left, right RuntimeAcceptanceSlice) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.Title == right.Title && left.Scope == right.Scope && left.Priority == right.Priority &&
		left.CompletionPolicy == right.CompletionPolicy && left.ResultPolicy == right.ResultPolicy &&
		sameCoverageWorkItemIDs(left.CoverageWorkItemIDs, right.CoverageWorkItemIDs) && sameStrings(left.RequiredEvidenceKinds, right.RequiredEvidenceKinds)
}

func sameCoverageFocus(left CoverageFocus, right CoverageFocus) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.SliceID == right.SliceID && sameCoverageWorkItemIDs(left.CoverageWorkItemIDs, right.CoverageWorkItemIDs) &&
		left.AssessmentScope == right.AssessmentScope && left.AssessmentID == right.AssessmentID && left.AssessmentText == right.AssessmentText &&
		left.EvidenceDeclarationRef == right.EvidenceDeclarationRef &&
		left.EvidenceRole == right.EvidenceRole && left.TargetRole == right.TargetRole && left.TargetSelectorRef == right.TargetSelectorRef && left.TargetSelectorOwnerRef == right.TargetSelectorOwnerRef && left.TargetSelectorSubjectRef == right.TargetSelectorSubjectRef && left.ProspectiveTargetRef == right.ProspectiveTargetRef && left.RequireTargetGrounding == right.RequireTargetGrounding && left.RequireAssessment == right.RequireAssessment &&
		left.RequireDirectEvidence == right.RequireDirectEvidence && left.RequireClaimEvidence == right.RequireClaimEvidence &&
		left.RequireExecutableBehaviorEvidence == right.RequireExecutableBehaviorEvidence && left.RequireCoverageTargets == right.RequireCoverageTargets && left.ValidationRecipeID == right.ValidationRecipeID &&
		sameRuntimeOwnerFactRequirements(left.RequiredOwnerFacts, right.RequiredOwnerFacts) && SameRuntimeOwnerFactConvergences(left.OwnerFactConvergences, right.OwnerFactConvergences) &&
		sameCoverageEvidenceBindings(left.CoverageEvidenceBindings, right.CoverageEvidenceBindings) &&
		sameStrings(left.RequiredEvidenceKinds, right.RequiredEvidenceKinds) && sameStrings(left.TargetRefs, right.TargetRefs) && sameStrings(left.TargetDiscoverySources, right.TargetDiscoverySources) && sameStrings(left.TargetSelectorCandidateRefs, right.TargetSelectorCandidateRefs) && sameStrings(left.RequiredCheckRefs, right.RequiredCheckRefs) && sameStrings(left.MetricConstraints, right.MetricConstraints) && sameStrings(left.ArtifactRefs, right.ArtifactRefs) &&
		sameStrings(left.AcceptedStepKinds, right.AcceptedStepKinds) && SameRuntimeOperationVariantRefs(left.AcceptedOperationVariants, right.AcceptedOperationVariants) &&
		sameStrings(left.RejectedStepKinds, right.RejectedStepKinds)
}

func sameCoverageEvidenceBindings(left, right []CoverageEvidenceBinding) bool {
	left = NormalizeCoverageEvidenceBindings(left)
	right = NormalizeCoverageEvidenceBindings(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx].ID != right[idx].ID || left[idx].Hash != right[idx].Hash {
			return false
		}
	}
	return true
}

func sameRuntimeRequirement(left RuntimeRequirement, right RuntimeRequirement) bool {
	leftValues := normalizeRuntimeRequirements([]RuntimeRequirement{left})
	rightValues := normalizeRuntimeRequirements([]RuntimeRequirement{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left = leftValues[0]
	right = rightValues[0]
	return left.Kind == right.Kind && left.Scope == right.Scope && left.FocusID == right.FocusID && left.OwnerModuleID == right.OwnerModuleID && left.SubjectRef == right.SubjectRef && left.TargetSelectorRef == right.TargetSelectorRef && left.TargetSelectorOwnerRef == right.TargetSelectorOwnerRef && left.TargetSelectorSubjectRef == right.TargetSelectorSubjectRef && left.ProspectiveTargetRef == right.ProspectiveTargetRef &&
		left.CapabilityClass == right.CapabilityClass && left.MinimumBytes == right.MinimumBytes && left.Reason == right.Reason &&
		sameStrings(left.RequiredCapabilities, right.RequiredCapabilities) && sameStrings(left.TargetSelectorCandidateRefs, right.TargetSelectorCandidateRefs)
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func sameCoverageWorkItemIDs(left []common.CoverageWorkItemID, right []common.CoverageWorkItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func normalizeRuntimeRequirements(values []RuntimeRequirement) []RuntimeRequirement {
	ret := make([]RuntimeRequirement, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value.Kind = PrerequisiteKind(strings.TrimSpace(string(value.Kind)))
		value.Scope = RuntimePrerequisiteScope(strings.TrimSpace(string(value.Scope)))
		if value.Scope == "" {
			value.Scope = RuntimePrerequisiteScopeGlobal
		}
		value.FocusID = strings.TrimSpace(value.FocusID)
		if value.Scope != RuntimePrerequisiteScopeFocus {
			value.Scope = RuntimePrerequisiteScopeGlobal
			value.FocusID = ""
		}
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.TargetSelectorOwnerRef = strings.TrimSpace(value.TargetSelectorOwnerRef)
		value.TargetSelectorSubjectRef = strings.TrimSpace(value.TargetSelectorSubjectRef)
		value.TargetSelectorCandidateRefs = normalizeStrings(value.TargetSelectorCandidateRefs)
		value.ProspectiveTargetRef = strings.TrimSpace(value.ProspectiveTargetRef)
		value.CapabilityClass = RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
		value.RequiredCapabilities = normalizeStrings(value.RequiredCapabilities)
		value.Reason = strings.TrimSpace(value.Reason)
		key := strings.Join([]string{string(value.Kind), string(value.Scope), value.FocusID, value.OwnerModuleID, value.SubjectRef, value.TargetSelectorRef, value.TargetSelectorOwnerRef, value.TargetSelectorSubjectRef, strings.Join(value.TargetSelectorCandidateRefs, ","), value.ProspectiveTargetRef, string(value.CapabilityClass), strings.Join(value.RequiredCapabilities, ","), fmt.Sprintf("%d", value.MinimumBytes)}, "|")
		if value.Kind == "" || key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
