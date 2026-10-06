package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

type RuntimeStateChangePolicy string

const (
	RuntimeStateChangeAllowedButNotRequired RuntimeStateChangePolicy = "allowed_but_not_required"
	RuntimeStateChangeMustProduce           RuntimeStateChangePolicy = "must_produce_state_change"
)

type RuntimeExecutionContract struct {
	OperationContractRevision string
	StateChangePolicy         RuntimeStateChangePolicy
	MaxRepairAttemptsPerFocus int
	ExecutionLimits           *RuntimeExecutionLimits `json:",omitempty"`
	// CandidateDocumentRecovery bounds complete-document provider retries for
	// this Run. Resume cannot change the activation-sealed budget.
	CandidateDocumentRecovery RuntimeCandidateDocumentRecoveryPolicy
	CapabilityUsagePolicies   []RuntimeCapabilityUsagePolicy
	ValidationPolicies        []RuntimeValidationPolicy
	ProfileContracts          []RuntimeProfileContract
	AcceptanceSlicePolicy     *RuntimeAcceptanceSlicePolicy
	TemplateStagePolicy       *RuntimeTemplateStagePolicy
}

type RuntimeCapabilityUsagePolicy struct {
	ID                 string
	Scope              string
	AppliesToCategory  string
	EvidenceStrength   string
	UsageMode          string
	FallbackPolicy     string
	TriggerStages      []string
	TriggerSignals     []string
	PreferredTools     []string
	RequiredAfterKinds []string
	Notes              []string
}

func (c RuntimeExecutionContract) Normalized() RuntimeExecutionContract {
	c.OperationContractRevision = strings.TrimSpace(c.OperationContractRevision)
	c.StateChangePolicy = RuntimeStateChangePolicy(strings.TrimSpace(string(c.StateChangePolicy)))
	c.CandidateDocumentRecovery = c.CandidateDocumentRecovery.Normalized()
	if c.ExecutionLimits != nil {
		limits := *c.ExecutionLimits
		c.ExecutionLimits = &limits
	}
	c.CapabilityUsagePolicies = normalizeRuntimeCapabilityUsagePolicies(c.CapabilityUsagePolicies)
	c.ValidationPolicies = NormalizeRuntimeValidationPolicies(c.ValidationPolicies)
	c.ProfileContracts = NormalizeRuntimeProfileContracts(c.ProfileContracts)
	if c.AcceptanceSlicePolicy != nil {
		policy := c.AcceptanceSlicePolicy.Normalized()
		c.AcceptanceSlicePolicy = &policy
	}
	if c.TemplateStagePolicy != nil {
		policy := c.TemplateStagePolicy.Normalized()
		c.TemplateStagePolicy = &policy
	}
	return c
}

func (c RuntimeExecutionContract) Validate() error {
	c = c.Normalized()
	switch c.StateChangePolicy {
	case RuntimeStateChangeAllowedButNotRequired, RuntimeStateChangeMustProduce:
	default:
		return fmt.Errorf("runtime execution contract has invalid state-change policy %q", c.StateChangePolicy)
	}
	for _, policy := range c.CapabilityUsagePolicies {
		if policy.ID == "" {
			return fmt.Errorf("runtime capability usage policy requires id")
		}
	}
	if c.MaxRepairAttemptsPerFocus < 0 || c.MaxRepairAttemptsPerFocus > MaxRepairAttemptsPerFocusLimit {
		return fmt.Errorf("runtime execution contract max repair attempts must be in 0..%d", MaxRepairAttemptsPerFocusLimit)
	}
	if err := c.CandidateDocumentRecovery.Validate(); err != nil {
		return err
	}
	if c.ExecutionLimits != nil {
		if err := c.ExecutionLimits.Validate(); err != nil {
			return err
		}
	}
	if err := ValidateRuntimeValidationPolicies(c.ValidationPolicies); err != nil {
		return err
	}
	if err := ValidateRuntimeProfileContracts(c.ProfileContracts); err != nil {
		return err
	}
	if c.AcceptanceSlicePolicy != nil {
		if err := c.AcceptanceSlicePolicy.Validate(); err != nil {
			return err
		}
	}
	if c.TemplateStagePolicy != nil {
		if err := c.TemplateStagePolicy.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateExecutableAdmission 用于新 activation ingress；内部重载/CAS 校验
// 保留纯 shape/source 合同，不能借纯校验结果绕过外部 availability。
func (c RuntimeExecutionContract) ValidateExecutableAdmission() error {
	if err := c.Validate(); err != nil {
		return err
	}
	return ValidateRuntimeImplementationWorkAvailability(c.ProfileContracts)
}

func SameRuntimeExecutionContract(left RuntimeExecutionContract, right RuntimeExecutionContract) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.OperationContractRevision != right.OperationContractRevision || left.StateChangePolicy != right.StateChangePolicy || left.MaxRepairAttemptsPerFocus != right.MaxRepairAttemptsPerFocus ||
		left.CandidateDocumentRecovery != right.CandidateDocumentRecovery ||
		!SameRuntimeExecutionLimits(left.ExecutionLimits, right.ExecutionLimits) ||
		len(left.CapabilityUsagePolicies) != len(right.CapabilityUsagePolicies) ||
		!sameRuntimeValidationPolicies(left.ValidationPolicies, right.ValidationPolicies) ||
		!SameRuntimeProfileContracts(left.ProfileContracts, right.ProfileContracts) || !SameRuntimeTemplateStagePolicy(left.TemplateStagePolicy, right.TemplateStagePolicy) ||
		!sameRuntimeAcceptanceSlicePolicy(left.AcceptanceSlicePolicy, right.AcceptanceSlicePolicy) {
		return false
	}
	for idx := range left.CapabilityUsagePolicies {
		if !sameRuntimeCapabilityUsagePolicy(left.CapabilityUsagePolicies[idx], right.CapabilityUsagePolicies[idx]) {
			return false
		}
	}
	return true
}

func normalizeRuntimeCapabilityUsagePolicies(values []RuntimeCapabilityUsagePolicy) []RuntimeCapabilityUsagePolicy {
	byID := map[string]RuntimeCapabilityUsagePolicy{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.AppliesToCategory = strings.TrimSpace(value.AppliesToCategory)
		value.EvidenceStrength = strings.TrimSpace(value.EvidenceStrength)
		value.UsageMode = strings.TrimSpace(value.UsageMode)
		value.FallbackPolicy = strings.TrimSpace(value.FallbackPolicy)
		value.TriggerStages = normalizeStrings(value.TriggerStages)
		value.TriggerSignals = normalizeStrings(value.TriggerSignals)
		value.PreferredTools = normalizeStrings(value.PreferredTools)
		value.RequiredAfterKinds = normalizeStrings(value.RequiredAfterKinds)
		value.Notes = normalizeStrings(value.Notes)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeCapabilityUsagePolicy, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func sameRuntimeCapabilityUsagePolicy(left RuntimeCapabilityUsagePolicy, right RuntimeCapabilityUsagePolicy) bool {
	leftValues := normalizeRuntimeCapabilityUsagePolicies([]RuntimeCapabilityUsagePolicy{left})
	rightValues := normalizeRuntimeCapabilityUsagePolicies([]RuntimeCapabilityUsagePolicy{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left, right = leftValues[0], rightValues[0]
	return left.ID == right.ID && left.Scope == right.Scope && left.AppliesToCategory == right.AppliesToCategory &&
		left.EvidenceStrength == right.EvidenceStrength && left.UsageMode == right.UsageMode && left.FallbackPolicy == right.FallbackPolicy &&
		sameStrings(left.TriggerStages, right.TriggerStages) && sameStrings(left.TriggerSignals, right.TriggerSignals) &&
		sameStrings(left.PreferredTools, right.PreferredTools) && sameStrings(left.RequiredAfterKinds, right.RequiredAfterKinds) && sameStrings(left.Notes, right.Notes)
}

func sameRuntimeAcceptanceSlicePolicy(left *RuntimeAcceptanceSlicePolicy, right *RuntimeAcceptanceSlicePolicy) bool {
	if left == nil || right == nil {
		return left == right
	}
	l := left.Normalized()
	r := right.Normalized()
	if l.SchemaVersion != r.SchemaVersion || l.MaxWorkItems != r.MaxWorkItems || l.MaxFocuses != r.MaxFocuses || l.MaxCandidateContextItems != r.MaxCandidateContextItems || l.CompletionPolicy != r.CompletionPolicy || l.ResultPolicy != r.ResultPolicy || len(l.Groups) != len(r.Groups) {
		return false
	}
	for idx := range l.Groups {
		lg, rg := l.Groups[idx], r.Groups[idx]
		if lg.ID != rg.ID || lg.Title != rg.Title || lg.Scope != rg.Scope || lg.Priority != rg.Priority || lg.MaxWorkItems != rg.MaxWorkItems ||
			lg.Match.All != rg.Match.All || !sameStrings(lg.Match.EvidenceRoles, rg.Match.EvidenceRoles) ||
			!sameStrings(lg.Match.MissingEvidenceKinds, rg.Match.MissingEvidenceKinds) ||
			!sameStrings(lg.Match.SourceRequirementIDs, rg.Match.SourceRequirementIDs) ||
			!sameStrings(lg.Match.SourceScopes, rg.Match.SourceScopes) {
			return false
		}
	}
	return true
}
