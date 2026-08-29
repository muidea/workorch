package workorchruntimecontract

import "strings"

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
)

type ValidationFailureDiagnostic struct {
	SchemaVersion                 string   `json:"schemaVersion,omitempty"`
	Class                         string   `json:"class,omitempty"`
	Reason                        string   `json:"reason,omitempty"`
	RecommendedEvidenceTargetType string   `json:"recommendedEvidenceTargetType,omitempty"`
	Stage                         string   `json:"stage,omitempty"`
	Operation                     string   `json:"operation,omitempty"`
	CommandKind                   string   `json:"commandKind,omitempty"`
	ExitCode                      int      `json:"exitCode,omitempty"`
	TimedOut                      bool     `json:"timedOut,omitempty"`
	CommandPolicyViolation        bool     `json:"commandPolicyViolation,omitempty"`
	UnavailableCommand            string   `json:"unavailableCommand,omitempty"`
	TargetRefs                    []string `json:"targetRefs,omitempty"`
	ObservationType               string   `json:"observationType,omitempty"`
	EvidenceMode                  string   `json:"evidenceMode,omitempty"`
	StdoutPreview                 string   `json:"stdoutPreview,omitempty"`
	StderrPreview                 string   `json:"stderrPreview,omitempty"`
}

func (d ValidationFailureDiagnostic) Normalized() ValidationFailureDiagnostic {
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.Class = strings.TrimSpace(d.Class)
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
	return l.SchemaVersion == r.SchemaVersion && l.Class == r.Class && l.Reason == r.Reason &&
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
