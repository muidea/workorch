package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

type RuntimeEvidencePolicy struct {
	ID          string
	Profile     string
	Rule        string
	AppliesTo   string
	CommandName string
	ArgsContain []string
	// RequiredArgs is a command-argument set that a
	// Shell validation candidate must include when it selects CommandName. It
	// is a profile-owned observable-validation recipe, not an Orchestrator
	// action rule. Keeping it on the sealed evidence policy lets the candidate
	// boundary reject a command that cannot yield the observation the profile
	// requires before any side effect or validation session is started.
	RequiredArgs         []string
	ZeroWorkMarkers      []string
	WorkEvidenceMarkers  []string
	QualityOnZeroWork    string
	MarksZeroWork        bool
	EvidenceTags         []string
	EvidenceReason       string
	CoverageRequirements []RuntimeCoverageEvidenceRequirement
	Description          string
}

type RuntimeCoverageEvidenceRequirement struct {
	ID                                string
	Scope                             string
	RequiredEvidenceKinds             []string
	RequireDirectEvidence             bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
	AcceptedStepKinds                 []string
	RejectedStepKinds                 []string
	Description                       string
}

func (p RuntimeEvidencePolicy) Normalized() RuntimeEvidencePolicy {
	p.ID = strings.TrimSpace(p.ID)
	p.Profile = strings.TrimSpace(p.Profile)
	p.Rule = strings.TrimSpace(p.Rule)
	p.AppliesTo = strings.TrimSpace(p.AppliesTo)
	p.CommandName = strings.TrimSpace(p.CommandName)
	p.ArgsContain = normalizeStrings(p.ArgsContain)
	p.RequiredArgs = normalizeStrings(p.RequiredArgs)
	p.ZeroWorkMarkers = normalizeStrings(p.ZeroWorkMarkers)
	p.WorkEvidenceMarkers = normalizeStrings(p.WorkEvidenceMarkers)
	p.QualityOnZeroWork = strings.TrimSpace(p.QualityOnZeroWork)
	p.EvidenceTags = normalizeStrings(p.EvidenceTags)
	p.EvidenceReason = strings.TrimSpace(p.EvidenceReason)
	p.Description = strings.TrimSpace(p.Description)
	p.CoverageRequirements = normalizeRuntimeCoverageEvidenceRequirements(p.CoverageRequirements)
	return p
}

func (p RuntimeEvidencePolicy) Validate() error {
	p = p.Normalized()
	if p.ID == "" || p.AppliesTo == "" {
		return fmt.Errorf("runtime evidence policy requires id and applies-to kind")
	}
	if len(p.ZeroWorkMarkers) > 0 && p.QualityOnZeroWork == "" {
		return fmt.Errorf("runtime evidence policy with zero-work markers requires a quality")
	}
	if len(p.RequiredArgs) > 0 && (p.AppliesTo != StepKindShellCommand || p.CommandName == "") {
		return fmt.Errorf("runtime evidence policy required args require a shell command name")
	}
	for _, requirement := range p.CoverageRequirements {
		if requirement.ID == "" || requirement.Scope == "" {
			return fmt.Errorf("runtime coverage evidence requirement requires id and scope")
		}
	}
	return nil
}

func NormalizeRuntimeEvidencePolicies(values []RuntimeEvidencePolicy) []RuntimeEvidencePolicy {
	byID := map[string]RuntimeEvidencePolicy{}
	for _, value := range values {
		value = value.Normalized()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeEvidencePolicy, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeEvidencePolicies(values []RuntimeEvidencePolicy) error {
	for _, value := range NormalizeRuntimeEvidencePolicies(values) {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func SameRuntimeEvidencePolicies(left []RuntimeEvidencePolicy, right []RuntimeEvidencePolicy) bool {
	left = NormalizeRuntimeEvidencePolicies(left)
	right = NormalizeRuntimeEvidencePolicies(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if !sameRuntimeEvidencePolicy(left[idx], right[idx]) {
			return false
		}
	}
	return true
}

func normalizeRuntimeCoverageEvidenceRequirements(values []RuntimeCoverageEvidenceRequirement) []RuntimeCoverageEvidenceRequirement {
	byKey := map[string]RuntimeCoverageEvidenceRequirement{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.RequiredEvidenceKinds = normalizeStrings(value.RequiredEvidenceKinds)
		value.AcceptedStepKinds = normalizeStrings(value.AcceptedStepKinds)
		value.RejectedStepKinds = normalizeStrings(value.RejectedStepKinds)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" && value.Scope != "" {
			byKey[value.Scope+"|"+value.ID] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeCoverageEvidenceRequirement, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byKey[key])
	}
	return nilIfEmpty(ret)
}

func sameRuntimeEvidencePolicy(left RuntimeEvidencePolicy, right RuntimeEvidencePolicy) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.ID != right.ID || left.Profile != right.Profile || left.Rule != right.Rule || left.AppliesTo != right.AppliesTo ||
		left.CommandName != right.CommandName || left.QualityOnZeroWork != right.QualityOnZeroWork || left.MarksZeroWork != right.MarksZeroWork ||
		left.EvidenceReason != right.EvidenceReason || left.Description != right.Description ||
		!sameStrings(left.ArgsContain, right.ArgsContain) || !sameStrings(left.RequiredArgs, right.RequiredArgs) || !sameStrings(left.ZeroWorkMarkers, right.ZeroWorkMarkers) ||
		!sameStrings(left.WorkEvidenceMarkers, right.WorkEvidenceMarkers) || !sameStrings(left.EvidenceTags, right.EvidenceTags) ||
		len(left.CoverageRequirements) != len(right.CoverageRequirements) {
		return false
	}
	for idx := range left.CoverageRequirements {
		l, r := left.CoverageRequirements[idx], right.CoverageRequirements[idx]
		if l.ID != r.ID || l.Scope != r.Scope || l.RequireDirectEvidence != r.RequireDirectEvidence || l.RequireClaimEvidence != r.RequireClaimEvidence ||
			l.RequireExecutableBehaviorEvidence != r.RequireExecutableBehaviorEvidence || l.Description != r.Description ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) || !sameStrings(l.AcceptedStepKinds, r.AcceptedStepKinds) || !sameStrings(l.RejectedStepKinds, r.RejectedStepKinds) {
			return false
		}
	}
	return true
}
