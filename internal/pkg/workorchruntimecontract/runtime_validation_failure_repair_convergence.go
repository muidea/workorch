package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// RuntimeValidationFailureRepairConvergence is one immutable, Profile-owned
// declaration for recovering a rejected validation proof. Its semantics are
// fixed and conjunctive:
//
//	typed validation rejection -> state-change repair -> fresh validation
//
// It intentionally has no alternative/one-of relation field. The repair
// target surface is independent from the terminal validation surface, and a
// selector or candidate bound never grants an exact prospective target. A
// resource-owner observation must still confirm the concrete target before an
// operation can be sealed.
type RuntimeValidationFailureRepairConvergence struct {
	ID                       string
	TriggerOwnerModuleID     string
	TriggerOperationKind     string
	TriggerDiagnosticClasses []string
	RepairEvidenceKind       string
	RepairOwnerModuleID      string
	RepairOperationKinds     []string
	RepairTargetPolicy       RuntimeOperationTargetPolicy
	RepairTargetRole         string
	// RepairDeclarationTargetSelector* is the Profile-owned semantic target
	// declaration. RepairOperationTargetSelectorRef is the capability owner's
	// concrete operation authorization. They are independent identities and are
	// never treated as aliases.
	RepairDeclarationTargetSelectorRef           string
	RepairDeclarationTargetSelectorOwnerRef      string
	RepairDeclarationTargetSelectorSubjectRef    string
	RepairDeclarationTargetSelectorCandidateRefs []string
	RepairOperationTargetSelectorRef             string
	RequireFreshValidation                       bool
	// MaxAttempts bounds matching rejection-to-repair-to-revalidation cycles
	// for one CWI. Cycle one includes the rejection which opens the first repair;
	// it is not an additional charge on top of that repair cycle.
	MaxAttempts int
}

func (c RuntimeValidationFailureRepairConvergence) Normalized() RuntimeValidationFailureRepairConvergence {
	c.ID = strings.TrimSpace(c.ID)
	c.TriggerOwnerModuleID = strings.TrimSpace(c.TriggerOwnerModuleID)
	c.TriggerOperationKind = CanonicalRuntimeBlockKind(c.TriggerOperationKind)
	c.TriggerDiagnosticClasses = normalizeStrings(c.TriggerDiagnosticClasses)
	c.RepairEvidenceKind = strings.TrimSpace(c.RepairEvidenceKind)
	c.RepairOwnerModuleID = strings.TrimSpace(c.RepairOwnerModuleID)
	c.RepairOperationKinds = canonicalizeOperationKinds(c.RepairOperationKinds)
	c.RepairTargetPolicy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(c.RepairTargetPolicy)))
	c.RepairTargetRole = strings.TrimSpace(c.RepairTargetRole)
	c.RepairDeclarationTargetSelectorRef = strings.TrimSpace(c.RepairDeclarationTargetSelectorRef)
	c.RepairDeclarationTargetSelectorOwnerRef = strings.TrimSpace(c.RepairDeclarationTargetSelectorOwnerRef)
	c.RepairDeclarationTargetSelectorSubjectRef = strings.TrimSpace(c.RepairDeclarationTargetSelectorSubjectRef)
	c.RepairDeclarationTargetSelectorCandidateRefs = normalizeStrings(c.RepairDeclarationTargetSelectorCandidateRefs)
	c.RepairOperationTargetSelectorRef = strings.TrimSpace(c.RepairOperationTargetSelectorRef)
	return c
}

func (c RuntimeValidationFailureRepairConvergence) Clone() RuntimeValidationFailureRepairConvergence {
	c = c.Normalized()
	c.TriggerDiagnosticClasses = append([]string(nil), c.TriggerDiagnosticClasses...)
	c.RepairOperationKinds = append([]string(nil), c.RepairOperationKinds...)
	c.RepairDeclarationTargetSelectorCandidateRefs = append([]string(nil), c.RepairDeclarationTargetSelectorCandidateRefs...)
	return c
}

func (c RuntimeValidationFailureRepairConvergence) Validate() error {
	c = c.Normalized()
	if c.ID == "" || c.TriggerOwnerModuleID == "" || c.TriggerOperationKind == "" || len(c.TriggerDiagnosticClasses) == 0 {
		return fmt.Errorf("validation failure repair convergence requires identity and a typed trigger owner, operation and diagnostic class")
	}
	trigger, found := RuntimeOperationDescriptorForKind(c.TriggerOperationKind)
	if !found || trigger.OwnerModuleID != c.TriggerOwnerModuleID || !RuntimeOperationKindSupportsCoverageEvidence(c.TriggerOperationKind, EvidenceKindValidation) {
		return fmt.Errorf("validation failure repair convergence trigger operation %q is not validation proof owned by %q", c.TriggerOperationKind, c.TriggerOwnerModuleID)
	}
	for _, diagnosticClass := range c.TriggerDiagnosticClasses {
		if !runtimeValidationDiagnosticClassKnown(diagnosticClass) {
			return fmt.Errorf("validation failure repair convergence %q has unknown diagnostic class %q", c.ID, diagnosticClass)
		}
	}
	if c.RepairEvidenceKind != EvidenceKindStateChange || c.RepairOwnerModuleID == "" || len(c.RepairOperationKinds) == 0 {
		return fmt.Errorf("validation failure repair convergence %q requires owner-backed state_change repair operations", c.ID)
	}
	for _, kind := range c.RepairOperationKinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID != c.RepairOwnerModuleID || !RuntimeOperationKindSupportsCoverageEvidence(kind, EvidenceKindStateChange) {
			return fmt.Errorf("validation failure repair convergence %q operation %q is not state-change proof owned by %q", c.ID, kind, c.RepairOwnerModuleID)
		}
	}
	if c.RepairTargetRole == "" || c.RepairDeclarationTargetSelectorRef == "" || c.RepairDeclarationTargetSelectorOwnerRef == "" || c.RepairDeclarationTargetSelectorSubjectRef == "" || c.RepairOperationTargetSelectorRef == "" {
		return fmt.Errorf("validation failure repair convergence %q requires an independent repair target role and complete selector identity", c.ID)
	}
	if c.RepairDeclarationTargetSelectorRef == c.RepairOperationTargetSelectorRef {
		return fmt.Errorf("validation failure repair convergence %q requires distinct declaration and operation selector identities", c.ID)
	}
	switch c.RepairTargetPolicy {
	case RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		// Both policies are selector-backed. Exact prospective targets are
		// deliberately absent from this pre-seal declaration.
	default:
		return fmt.Errorf("validation failure repair convergence %q requires a selector-backed repair target policy, got %q", c.ID, c.RepairTargetPolicy)
	}
	if !c.RequireFreshValidation {
		return fmt.Errorf("validation failure repair convergence %q must require fresh validation after repair", c.ID)
	}
	if c.MaxAttempts < 1 || c.MaxAttempts > MaxRepairAttemptsPerFocusLimit {
		return fmt.Errorf("validation failure repair convergence %q max attempts must be in 1..%d", c.ID, MaxRepairAttemptsPerFocusLimit)
	}
	return nil
}

// MatchesTrigger compares only the typed producer identity sealed by the
// Profile. It does not inspect diagnostic text, select a repair target or
// choose a Runtime transition.
func (c RuntimeValidationFailureRepairConvergence) MatchesTrigger(ownerModuleID, operationKind, diagnosticClass string) bool {
	c = c.Normalized()
	if c.Validate() != nil {
		return false
	}
	return c.TriggerOwnerModuleID == strings.TrimSpace(ownerModuleID) &&
		c.TriggerOperationKind == CanonicalRuntimeBlockKind(operationKind) &&
		containsString(c.TriggerDiagnosticClasses, strings.TrimSpace(diagnosticClass))
}

func StableRuntimeValidationFailureRepairConvergenceHash(value RuntimeValidationFailureRepairConvergence) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-validation-failure-repair-convergence")
	writeHashString(h, value.ID)
	writeHashString(h, value.TriggerOwnerModuleID)
	writeHashString(h, value.TriggerOperationKind)
	writeHashStrings(h, value.TriggerDiagnosticClasses)
	writeHashString(h, value.RepairEvidenceKind)
	writeHashString(h, value.RepairOwnerModuleID)
	writeHashStrings(h, value.RepairOperationKinds)
	writeHashString(h, string(value.RepairTargetPolicy))
	writeHashString(h, value.RepairTargetRole)
	writeHashString(h, value.RepairDeclarationTargetSelectorRef)
	writeHashString(h, value.RepairDeclarationTargetSelectorOwnerRef)
	writeHashString(h, value.RepairDeclarationTargetSelectorSubjectRef)
	writeHashStrings(h, value.RepairDeclarationTargetSelectorCandidateRefs)
	writeHashString(h, value.RepairOperationTargetSelectorRef)
	writeHashString(h, fmt.Sprintf("%t", value.RequireFreshValidation))
	writeHashString(h, fmt.Sprintf("%d", value.MaxAttempts))
	return "runtime-validation-failure-repair-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func SameRuntimeValidationFailureRepairConvergence(left, right *RuntimeValidationFailureRepairConvergence) bool {
	if left == nil || right == nil {
		return left == right
	}
	l := left.Normalized()
	r := right.Normalized()
	return l.ID == r.ID && l.TriggerOwnerModuleID == r.TriggerOwnerModuleID && l.TriggerOperationKind == r.TriggerOperationKind &&
		sameStrings(l.TriggerDiagnosticClasses, r.TriggerDiagnosticClasses) && l.RepairEvidenceKind == r.RepairEvidenceKind &&
		l.RepairOwnerModuleID == r.RepairOwnerModuleID && sameStrings(l.RepairOperationKinds, r.RepairOperationKinds) &&
		l.RepairTargetPolicy == r.RepairTargetPolicy && l.RepairTargetRole == r.RepairTargetRole &&
		l.RepairDeclarationTargetSelectorRef == r.RepairDeclarationTargetSelectorRef && l.RepairDeclarationTargetSelectorOwnerRef == r.RepairDeclarationTargetSelectorOwnerRef &&
		l.RepairDeclarationTargetSelectorSubjectRef == r.RepairDeclarationTargetSelectorSubjectRef &&
		sameStrings(l.RepairDeclarationTargetSelectorCandidateRefs, r.RepairDeclarationTargetSelectorCandidateRefs) &&
		l.RepairOperationTargetSelectorRef == r.RepairOperationTargetSelectorRef &&
		l.RequireFreshValidation == r.RequireFreshValidation && l.MaxAttempts == r.MaxAttempts
}

func cloneRuntimeValidationFailureRepairConvergence(value *RuntimeValidationFailureRepairConvergence) *RuntimeValidationFailureRepairConvergence {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func runtimeValidationDiagnosticClassKnown(value string) bool {
	switch strings.TrimSpace(value) {
	case ValidationDiagnosticCommandInvalid,
		ValidationDiagnosticEnvironmentMissing,
		ValidationDiagnosticDependencyMismatch,
		ValidationDiagnosticToolchainMismatch,
		ValidationDiagnosticBuildFailed,
		ValidationDiagnosticTestFailed,
		ValidationDiagnosticTimeout,
		ValidationDiagnosticOutputInconclusive,
		ValidationDiagnosticEvidenceBindingMissing,
		ValidationDiagnosticContractViolation,
		ValidationDiagnosticEvidenceMetadataMissing,
		ValidationDiagnosticScratchRequired,
		ValidationDiagnosticScratchEscape,
		ValidationDiagnosticProcessControlForbidden,
		ValidationDiagnosticDetachedProcessForbidden,
		ValidationDiagnosticShellSafetyUnproven,
		ValidationDiagnosticManagedValidationRequired,
		ValidationDiagnosticImplementationGap,
		ValidationDiagnosticExternalDependency,
		ValidationDiagnosticProviderTransient,
		ValidationDiagnosticRuntimeMisalignment:
		return true
	default:
		return false
	}
}
