package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeValidationPolicy is the typed, domain-neutral validation policy sealed
// at activation. Runtime core consumes only these fields and never interprets
// template/profile maps directly.
type RuntimeValidationPolicy struct {
	ID                                         string
	Scope                                      string
	AppliesToEvidenceKinds                     []string
	ValidationSafetyClass                      string
	DiscoveryRequired                          bool
	DiscoverySources                           []string
	AllowedExecutionModes                      []string
	ValidationFailureImpliesStateChange        bool
	ImplementationGapDiagnosticClasses         []string
	MaxValidationOnlyRetriesPerFocus           int
	PreferMinimalPatchBeforeRepeatedValidation bool
	IfNoTargetRoute                            string
	ValidationScratchRequired                  bool
	AllowedValidationModes                     []string
	Notes                                      []string
}

// Validation discovery source IDs are neutral contract vocabulary. Profiles
// select which sources a validation policy needs; the Orchestrator only binds
// those IDs to already-persisted owner observations and never probes an
// environment itself.
const (
	RuntimeValidationDiscoverySourceWorkspaceManifest     = "workspace_manifest"
	RuntimeValidationDiscoverySourceProjectFiles          = "project_files"
	RuntimeValidationDiscoverySourceDeclaredCommands      = "declared_commands"
	RuntimeValidationDiscoverySourceAvailableCapabilities = "available_capabilities"
)

func NormalizeRuntimeValidationPolicies(values []RuntimeValidationPolicy) []RuntimeValidationPolicy {
	byID := make(map[string]RuntimeValidationPolicy, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.AppliesToEvidenceKinds = normalizeStrings(value.AppliesToEvidenceKinds)
		value.ValidationSafetyClass = strings.TrimSpace(value.ValidationSafetyClass)
		value.DiscoverySources = normalizeStrings(value.DiscoverySources)
		value.AllowedExecutionModes = normalizeStrings(value.AllowedExecutionModes)
		value.ImplementationGapDiagnosticClasses = normalizeStrings(value.ImplementationGapDiagnosticClasses)
		value.IfNoTargetRoute = strings.TrimSpace(value.IfNoTargetRoute)
		value.AllowedValidationModes = normalizeStrings(value.AllowedValidationModes)
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
	ret := make([]RuntimeValidationPolicy, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeValidationPolicies(values []RuntimeValidationPolicy) error {
	values = NormalizeRuntimeValidationPolicies(values)
	retryLimits := map[string]int{}
	failureRoutes := map[string]bool{}
	failureRouteSet := map[string]struct{}{}
	for _, value := range values {
		if value.ID == "" || value.Scope == "" || len(value.AppliesToEvidenceKinds) == 0 {
			return fmt.Errorf("runtime validation policy requires id, scope and evidence kinds")
		}
		if value.MaxValidationOnlyRetriesPerFocus < 0 || value.MaxValidationOnlyRetriesPerFocus > MaxValidationOnlyRetriesPerFocusLimit {
			return fmt.Errorf("runtime validation policy %q max validation-only retries must be in 0..%d", value.ID, MaxValidationOnlyRetriesPerFocusLimit)
		}
		for _, kind := range value.AppliesToEvidenceKinds {
			if _, ok := failureRouteSet[kind]; ok && failureRoutes[kind] != value.ValidationFailureImpliesStateChange {
				return fmt.Errorf("runtime validation policies define conflicting failure routes for evidence kind %q", kind)
			}
			failureRouteSet[kind] = struct{}{}
			failureRoutes[kind] = value.ValidationFailureImpliesStateChange
			if value.MaxValidationOnlyRetriesPerFocus == 0 {
				continue
			}
			if previous, ok := retryLimits[kind]; ok && previous != value.MaxValidationOnlyRetriesPerFocus {
				return fmt.Errorf("runtime validation policies define conflicting retry limits for evidence kind %q", kind)
			}
			retryLimits[kind] = value.MaxValidationOnlyRetriesPerFocus
		}
	}
	return nil
}

func RuntimeValidationFailureRequiresStateChange(values []RuntimeValidationPolicy, evidenceKind string) bool {
	evidenceKind = strings.TrimSpace(evidenceKind)
	for _, value := range NormalizeRuntimeValidationPolicies(values) {
		if containsStringValue(value.AppliesToEvidenceKinds, evidenceKind) {
			return value.ValidationFailureImpliesStateChange
		}
	}
	// Legacy/no-policy behavior remains conservative: rejected validation must
	// be followed by state-changing repair before the same validation can recur.
	return true
}

func RuntimeValidationRetryLimit(values []RuntimeValidationPolicy, evidenceKind string) (int, bool) {
	evidenceKind = strings.TrimSpace(evidenceKind)
	for _, value := range NormalizeRuntimeValidationPolicies(values) {
		if value.MaxValidationOnlyRetriesPerFocus <= 0 || !containsStringValue(value.AppliesToEvidenceKinds, evidenceKind) {
			continue
		}
		return value.MaxValidationOnlyRetriesPerFocus, true
	}
	return 0, false
}

// RequiredRuntimeValidationDiscoverySources returns the normalized union of
// sources required for a particular evidence kind. It is intentionally a pure
// policy projection: determining whether an owner observation satisfies a
// source remains an SM-2 Orchestrator decision.
func RequiredRuntimeValidationDiscoverySources(values []RuntimeValidationPolicy, evidenceKind string) []string {
	evidenceKind = strings.TrimSpace(evidenceKind)
	ret := make([]string, 0)
	for _, value := range NormalizeRuntimeValidationPolicies(values) {
		if !value.DiscoveryRequired || !containsStringValue(value.AppliesToEvidenceKinds, evidenceKind) {
			continue
		}
		ret = append(ret, value.DiscoverySources...)
	}
	return normalizeStrings(ret)
}

func sameRuntimeValidationPolicies(left, right []RuntimeValidationPolicy) bool {
	left = NormalizeRuntimeValidationPolicies(left)
	right = NormalizeRuntimeValidationPolicies(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		l, r := left[idx], right[idx]
		if l.ID != r.ID || l.Scope != r.Scope || l.ValidationSafetyClass != r.ValidationSafetyClass ||
			l.DiscoveryRequired != r.DiscoveryRequired || l.ValidationFailureImpliesStateChange != r.ValidationFailureImpliesStateChange ||
			l.MaxValidationOnlyRetriesPerFocus != r.MaxValidationOnlyRetriesPerFocus ||
			l.PreferMinimalPatchBeforeRepeatedValidation != r.PreferMinimalPatchBeforeRepeatedValidation ||
			l.IfNoTargetRoute != r.IfNoTargetRoute || l.ValidationScratchRequired != r.ValidationScratchRequired ||
			!sameStrings(l.AppliesToEvidenceKinds, r.AppliesToEvidenceKinds) || !sameStrings(l.DiscoverySources, r.DiscoverySources) ||
			!sameStrings(l.AllowedExecutionModes, r.AllowedExecutionModes) ||
			!sameStrings(l.ImplementationGapDiagnosticClasses, r.ImplementationGapDiagnosticClasses) ||
			!sameStrings(l.AllowedValidationModes, r.AllowedValidationModes) || !sameStrings(l.Notes, r.Notes) {
			return false
		}
	}
	return true
}
