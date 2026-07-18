package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const InitialRuntimeEvidenceRevision = "evidence-v2-00000000000000000000"

type RuntimeBootstrap struct {
	SchemaVersion          uint32
	Definition             RuntimeDefinitionContext
	Activation             RuntimeActivationContext
	Focuses                []CoverageFocus
	Slices                 []RuntimeAcceptanceSlice
	CoverageItems          []RuntimeCoverageItem
	Requirements           []RuntimeRequirement
	StateChangeRequirement RuntimeRunStateChangeRequirement
	ConvergencePolicy      RuntimeConvergencePolicy
	RecoveryPolicy         RuntimeRecoveryPolicy
}

func (b RuntimeBootstrap) Normalized() RuntimeBootstrap {
	b.Definition = b.Definition.Normalized()
	b.Activation = b.Activation.Normalized()
	b.Focuses = normalizeCoverageFocusQueue(b.Focuses)
	b.Slices = NormalizeRuntimeAcceptanceSlices(b.Slices)
	b.CoverageItems = NormalizeRuntimeCoverageItems(b.CoverageItems)
	b.Requirements = normalizeRuntimeRequirements(b.Requirements)
	b.StateChangeRequirement = b.StateChangeRequirement.Normalized()
	b.ConvergencePolicy = b.ConvergencePolicy.Normalized()
	b.RecoveryPolicy = b.RecoveryPolicy.Normalized()
	return b
}

func (b RuntimeBootstrap) Validate() error {
	b = b.Normalized()
	if b.SchemaVersion != RuntimeStateSchemaVersionCurrent {
		return fmt.Errorf("unsupported runtime bootstrap schema version %d", b.SchemaVersion)
	}
	if err := b.Definition.Validate(); err != nil {
		return err
	}
	if err := b.Activation.Validate(); err != nil {
		return err
	}
	if err := b.ConvergencePolicy.Validate(); err != nil {
		return err
	}
	if err := b.RecoveryPolicy.Validate(); err != nil {
		return err
	}
	if len(b.Focuses) == 0 {
		return fmt.Errorf("runtime bootstrap requires at least one coverage focus")
	}
	if len(b.CoverageItems) == 0 {
		return fmt.Errorf("runtime bootstrap requires explicit coverage items")
	}
	coverageItems := make(map[common.CoverageWorkItemID]struct{}, len(b.CoverageItems))
	for _, item := range b.CoverageItems {
		coverageItems[item.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, focus := range b.Focuses {
		if focus.ID == "" || len(focus.CoverageWorkItemIDs) == 0 || (len(focus.RequiredEvidenceKinds) == 0 && !focus.RequireAssessment) {
			return fmt.Errorf("runtime bootstrap focus requires identity, coverage and evidence or assessment")
		}
		if _, ok := seen[focus.ID]; ok {
			return fmt.Errorf("runtime bootstrap focus %s is duplicated", focus.ID)
		}
		for _, id := range focus.CoverageWorkItemIDs {
			if _, ok := coverageItems[id]; !ok {
				return fmt.Errorf("runtime bootstrap focus %s coverage item %s is not declared", focus.ID, id)
			}
		}
		seen[focus.ID] = struct{}{}
	}
	return nil
}

func (b RuntimeBootstrap) Clone() RuntimeBootstrap {
	b = b.Normalized()
	b.Definition = b.Definition.Clone()
	b.Activation = b.Activation.Clone()
	b.Focuses = append([]CoverageFocus(nil), b.Focuses...)
	for idx := range b.Focuses {
		b.Focuses[idx] = b.Focuses[idx].Normalized()
	}
	b.Slices = append([]RuntimeAcceptanceSlice(nil), b.Slices...)
	for idx := range b.Slices {
		b.Slices[idx] = b.Slices[idx].Normalized()
	}
	b.CoverageItems = append([]RuntimeCoverageItem(nil), b.CoverageItems...)
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
	return RuntimeFacts{
		WorkspaceID: workspaceID, RunID: runID, DefinitionRevision: b.Definition.Revision,
		EvidenceRevision: InitialRuntimeEvidenceRevision, Focus: b.Focuses[0],
		RemainingFocuses:  append([]CoverageFocus(nil), b.Focuses[1:]...),
		Activation:        b.Activation.Clone(),
		StateChange:       NewRuntimeStateChangeLedger(b.StateChangeRequirement),
		ConvergencePolicy: b.ConvergencePolicy.Normalized(),
		RecoveryPolicy:    b.RecoveryPolicy.Normalized(),
		Recovery:          RuntimeRecoveryLedger{},
		Repair:            RuntimeRepairLedger{},
		Coverage:          InitialRuntimeCoverageFacts(b.CoverageItems),
		Slices:            append([]RuntimeAcceptanceSlice(nil), b.Slices...),
	}, nil
}

func SameRuntimeBootstrap(left RuntimeBootstrap, right RuntimeBootstrap) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.SchemaVersion != right.SchemaVersion || !SameRuntimeDefinitionContext(left.Definition, right.Definition) || !SameRuntimeActivationContext(left.Activation, right.Activation) ||
		len(left.Focuses) != len(right.Focuses) || len(left.Slices) != len(right.Slices) || len(left.CoverageItems) != len(right.CoverageItems) ||
		len(left.Requirements) != len(right.Requirements) ||
		left.StateChangeRequirement.Required != right.StateChangeRequirement.Required ||
		!sameCoverageWorkItemIDs(left.StateChangeRequirement.CoverageWorkItemIDs, right.StateChangeRequirement.CoverageWorkItemIDs) ||
		left.ConvergencePolicy != right.ConvergencePolicy || left.RecoveryPolicy != right.RecoveryPolicy {
		return false
	}
	for idx := range left.Focuses {
		if !sameCoverageFocus(left.Focuses[idx], right.Focuses[idx]) {
			return false
		}
	}
	for idx := range left.Slices {
		if !sameRuntimeAcceptanceSlice(left.Slices[idx], right.Slices[idx]) {
			return false
		}
	}
	for idx := range left.CoverageItems {
		if left.CoverageItems[idx].Normalized() != right.CoverageItems[idx].Normalized() {
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
		left.EvidenceRole == right.EvidenceRole && left.RequireAssessment == right.RequireAssessment &&
		left.RequireDirectEvidence == right.RequireDirectEvidence && left.RequireClaimEvidence == right.RequireClaimEvidence &&
		left.RequireExecutableBehaviorEvidence == right.RequireExecutableBehaviorEvidence && left.RequireCoverageTargets == right.RequireCoverageTargets &&
		sameStrings(left.RequiredEvidenceKinds, right.RequiredEvidenceKinds) && sameStrings(left.TargetRefs, right.TargetRefs) &&
		sameStrings(left.AcceptedStepKinds, right.AcceptedStepKinds) && sameStrings(left.AcceptedOperations, right.AcceptedOperations) &&
		sameStrings(left.RejectedStepKinds, right.RejectedStepKinds)
}

func sameRuntimeRequirement(left RuntimeRequirement, right RuntimeRequirement) bool {
	leftValues := normalizeRuntimeRequirements([]RuntimeRequirement{left})
	rightValues := normalizeRuntimeRequirements([]RuntimeRequirement{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left = leftValues[0]
	right = rightValues[0]
	return left.Kind == right.Kind && left.OwnerModuleID == right.OwnerModuleID && left.SubjectRef == right.SubjectRef &&
		left.CapabilityClass == right.CapabilityClass && left.MinimumBytes == right.MinimumBytes && left.Reason == right.Reason &&
		sameStrings(left.RequiredCapabilities, right.RequiredCapabilities)
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
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.CapabilityClass = RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
		value.RequiredCapabilities = normalizeStrings(value.RequiredCapabilities)
		value.Reason = strings.TrimSpace(value.Reason)
		key := strings.Join([]string{string(value.Kind), value.OwnerModuleID, value.SubjectRef, string(value.CapabilityClass), strings.Join(value.RequiredCapabilities, ","), fmt.Sprintf("%d", value.MinimumBytes)}, "|")
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
