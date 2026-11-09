package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"strings"
)

const (
	ValidationFailureDiagnosticSchemaVersion = "workorch.validationFailureDiagnostic"

	ValidationDiagnosticCommandInvalid            = "validation_command_invalid"
	ValidationDiagnosticEnvironmentMissing        = "validation_environment_missing"
	ValidationDiagnosticDependencyMismatch        = "validation_dependency_mismatch"
	ValidationDiagnosticToolchainMismatch         = "validation_toolchain_mismatch"
	ValidationDiagnosticBuildFailed               = "validation_build_failed"
	ValidationDiagnosticTestFailed                = "validation_test_failed"
	ValidationDiagnosticTimeout                   = "validation_timeout"
	ValidationDiagnosticOutputInconclusive        = "validation_output_inconclusive"
	ValidationDiagnosticEvidenceBindingMissing    = "validation_evidence_binding_missing"
	ValidationDiagnosticContractViolation         = "validation_contract_violation"
	ValidationDiagnosticEvidenceMetadataMissing   = "validation_evidence_metadata_missing"
	ValidationDiagnosticScratchRequired           = "validation_scratch_required"
	ValidationDiagnosticScratchEscape             = "validation_scratch_escape"
	ValidationDiagnosticProcessControlForbidden   = "validation_process_control_forbidden"
	ValidationDiagnosticDetachedProcessForbidden  = "validation_detached_process_forbidden"
	ValidationDiagnosticShellSafetyUnproven       = "validation_shell_safety_unproven"
	ValidationDiagnosticManagedValidationRequired = "managed_validation_required"
	ValidationDiagnosticImplementationGap         = "implementation_gap_detected"
	ValidationDiagnosticExternalDependency        = "external_dependency_missing"
	ValidationDiagnosticProviderTransient         = "provider_transient_failure"
	ValidationDiagnosticRuntimeMisalignment       = "runtime_internal_contract_misalignment"

	ValidationEvidenceTargetCommandContract    = "command_contract"
	ValidationEvidenceTargetEnvironment        = "environment"
	ValidationEvidenceTargetDependencyContract = "dependency_contract"
	ValidationEvidenceTargetToolchainContract  = "toolchain_contract"
	ValidationEvidenceTargetValidationOutput   = "validation_output"
	ValidationEvidenceTargetBehaviorEvidence   = "behavior_evidence"
	ValidationEvidenceTargetCoverageBinding    = "coverage_binding"
	ValidationEvidenceTargetFailureSurface     = "failure_surface"
	ValidationEvidenceTargetValidationInput    = "validation_input"
)

type ValidationFailureDiagnostic struct {
	// Diagnostic transaction failure, never execution uncertainty or repair authority.
	Unavailability                RuntimeValidationDiagnosticUnavailability `json:"unavailability,omitempty"`
	TargetRejection               *RuntimeValidationFailureTargetRejection  `json:"targetRejection,omitempty"`
	InputRepair                   *RuntimeValidationInputRepair             `json:"inputRepair,omitempty"`
	InputLocations                []RuntimeValidationInputLocation          `json:"inputLocations,omitempty"`
	InputAddition                 RuntimeValidationInputAddition            `json:"inputAddition,omitzero"`
	SchemaVersion                 string                                    `json:"schemaVersion,omitempty"`
	Class                         string                                    `json:"class,omitempty"`
	OwnerCode                     string                                    `json:"ownerCode,omitempty"`
	Reason                        string                                    `json:"reason,omitempty"`
	RecommendedEvidenceTargetType string                                    `json:"recommendedEvidenceTargetType,omitempty"`
	Stage                         string                                    `json:"stage,omitempty"`
	Operation                     string                                    `json:"operation,omitempty"`
	CommandKind                   string                                    `json:"commandKind,omitempty"`
	ExitCode                      int                                       `json:"exitCode,omitempty"`
	TimedOut                      bool                                      `json:"timedOut,omitempty"`
	CommandPolicyViolation        bool                                      `json:"commandPolicyViolation,omitempty"`
	UnavailableCommand            string                                    `json:"unavailableCommand,omitempty"`
	TargetRefs                    []string                                  `json:"targetRefs,omitempty"`
	ObservationType               string                                    `json:"observationType,omitempty"`
	EvidenceMode                  string                                    `json:"evidenceMode,omitempty"`
	StdoutPreview                 string                                    `json:"stdoutPreview,omitempty"`
	StderrPreview                 string                                    `json:"stderrPreview,omitempty"`
}

type RuntimeValidationDiagnosticUnavailability string

const (
	ValidationDiagnosticAssertionsUnavailable   RuntimeValidationDiagnosticUnavailability = "assertions_unavailable"
	ValidationDiagnosticSourcesUnavailable      RuntimeValidationDiagnosticUnavailability = "sources_unavailable"
	ValidationDiagnosticCapacityExceeded        RuntimeValidationDiagnosticUnavailability = "capacity_exceeded"
	ValidationDiagnosticReproductionUnavailable RuntimeValidationDiagnosticUnavailability = "reproduction_unavailable"
	ValidationDiagnosticBehaviorUnobserved      RuntimeValidationDiagnosticUnavailability = "behavior_unobserved"
	ValidationDiagnosticInputChanged            RuntimeValidationDiagnosticUnavailability = "input_changed"
	ValidationDiagnosticExecutionUnavailable    RuntimeValidationDiagnosticUnavailability = "execution_unavailable"
	ValidationDiagnosticDeadlineExceeded        RuntimeValidationDiagnosticUnavailability = "deadline_exceeded"
)

func (u RuntimeValidationDiagnosticUnavailability) Validate() error {
	switch u {
	case "", ValidationDiagnosticAssertionsUnavailable, ValidationDiagnosticSourcesUnavailable,
		ValidationDiagnosticCapacityExceeded, ValidationDiagnosticReproductionUnavailable,
		ValidationDiagnosticBehaviorUnobserved, ValidationDiagnosticInputChanged,
		ValidationDiagnosticExecutionUnavailable, ValidationDiagnosticDeadlineExceeded:
		return nil
	default:
		return fmt.Errorf("unknown validation diagnostic unavailability %q", u)
	}
}

// RuntimeValidationFailureTargetRejection explains why the complete candidate
// transaction cannot become a repair observation. It never grants a target or
// an action. Full source output remains in the original owner diagnostic store.
type RuntimeValidationFailureTargetRejection struct {
	Code string `json:"code"`
	// Count at the rejecting boundary (tool candidates, marker, or merged
	// union). An earlier rejection never asserts that later sources were
	// admitted or that this is the size of an unconstructed overall union.
	CandidateCount int `json:"candidateCount"`
	Limit          int `json:"limit"`
}

const (
	RuntimeFailureTargetsInvalid       = "invalid_candidates"
	RuntimeFailureTargetsOverCapacity  = "capacity_exceeded"
	RuntimeFailureTargetsOutsideScope  = "outside_sealed_scope"
	RuntimeFailureTargetsInvalidMarker = "invalid_marker"
)

func (r *RuntimeValidationFailureTargetRejection) Clone() *RuntimeValidationFailureTargetRejection {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

func (r RuntimeValidationFailureTargetRejection) Validate() error {
	if r.Limit != RuntimeFailureTargetObservationMaxTargetRefs || r.CandidateCount < 0 {
		return fmt.Errorf("failure target rejection requires the shared capacity and a nonnegative count")
	}
	switch r.Code {
	case RuntimeFailureTargetsOverCapacity:
		if r.CandidateCount <= r.Limit {
			return fmt.Errorf("failure target capacity rejection requires an overflowing union")
		}
	case RuntimeFailureTargetsOutsideScope:
		if r.CandidateCount == 0 {
			return fmt.Errorf("failure target scope rejection requires observed candidates")
		}
	case RuntimeFailureTargetsInvalid, RuntimeFailureTargetsInvalidMarker:
	default:
		return fmt.Errorf("unknown failure target rejection code")
	}
	return nil
}

func (d ValidationFailureDiagnostic) Normalized() ValidationFailureDiagnostic {
	d.TargetRejection = d.TargetRejection.Clone()
	d.InputRepair = d.InputRepair.Clone()
	d.InputLocations = append([]RuntimeValidationInputLocation(nil), d.InputLocations...)
	d.InputAddition = d.InputAddition.Normalized()
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.Class = strings.TrimSpace(d.Class)
	d.OwnerCode = strings.TrimSpace(d.OwnerCode)
	d.Reason = strings.TrimSpace(d.Reason)
	d.RecommendedEvidenceTargetType = strings.TrimSpace(d.RecommendedEvidenceTargetType)
	d.Stage = strings.TrimSpace(d.Stage)
	d.Operation = CanonicalRuntimeBlockKind(d.Operation)
	d.CommandKind = strings.TrimSpace(d.CommandKind)
	d.UnavailableCommand = strings.TrimSpace(d.UnavailableCommand)
	d.TargetRefs = normalizeStrings(d.TargetRefs)
	d.ObservationType = strings.TrimSpace(d.ObservationType)
	d.EvidenceMode = strings.TrimSpace(d.EvidenceMode)
	d.StdoutPreview = strings.TrimSpace(d.StdoutPreview)
	d.StderrPreview = strings.TrimSpace(d.StderrPreview)
	return d
}

func (d ValidationFailureDiagnostic) Clone() ValidationFailureDiagnostic {
	d = d.Normalized()
	d.TargetRefs = append([]string(nil), d.TargetRefs...)
	return d
}

func sameValidationFailureDiagnostic(left, right *ValidationFailureDiagnostic) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	l := left.Normalized()
	r := right.Normalized()
	return l.Unavailability == r.Unavailability && reflect.DeepEqual(l.TargetRejection, r.TargetRejection) && SameRuntimeValidationInputRepair(l.InputRepair, r.InputRepair) && reflect.DeepEqual(l.InputLocations, r.InputLocations) && l.InputAddition == r.InputAddition && l.SchemaVersion == r.SchemaVersion && l.Class == r.Class && l.OwnerCode == r.OwnerCode && l.Reason == r.Reason &&
		l.RecommendedEvidenceTargetType == r.RecommendedEvidenceTargetType && l.Stage == r.Stage &&
		l.Operation == r.Operation && l.CommandKind == r.CommandKind && l.ExitCode == r.ExitCode &&
		l.TimedOut == r.TimedOut && l.CommandPolicyViolation == r.CommandPolicyViolation &&
		l.UnavailableCommand == r.UnavailableCommand && sameStrings(l.TargetRefs, r.TargetRefs) &&
		l.ObservationType == r.ObservationType && l.EvidenceMode == r.EvidenceMode &&
		l.StdoutPreview == r.StdoutPreview && l.StderrPreview == r.StderrPreview
}

// SameValidationFailureDiagnostic reports canonical equality for the bounded
// typed diagnostic. Runtime aggregate invariants use it to prove that the
// reducer-facing copy and the immutable owner-diagnostic copy have not drifted
// across persistence or recovery.
func SameValidationFailureDiagnostic(left, right *ValidationFailureDiagnostic) bool {
	return sameValidationFailureDiagnostic(left, right)
}
