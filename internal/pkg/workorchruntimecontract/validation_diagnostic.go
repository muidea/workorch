package workorchruntimecontract

const (
	ValidationFailureDiagnosticSchemaVersion = "workorch.validationFailureDiagnostic.v1"

	ValidationDiagnosticCommandInvalid         = "validation_command_invalid"
	ValidationDiagnosticEnvironmentMissing     = "validation_environment_missing"
	ValidationDiagnosticDependencyMismatch     = "validation_dependency_mismatch"
	ValidationDiagnosticToolchainMismatch      = "validation_toolchain_mismatch"
	ValidationDiagnosticBuildFailed            = "validation_build_failed"
	ValidationDiagnosticTestFailed             = "validation_test_failed"
	ValidationDiagnosticTimeout                = "validation_timeout"
	ValidationDiagnosticOutputInconclusive     = "validation_output_inconclusive"
	ValidationDiagnosticEvidenceBindingMissing = "validation_evidence_binding_missing"
	ValidationDiagnosticContractViolation      = "validation_contract_violation"
	ValidationDiagnosticImplementationGap      = "implementation_gap_detected"
	ValidationDiagnosticExternalDependency     = "external_dependency_missing"
	ValidationDiagnosticProviderTransient      = "provider_transient_failure"
	ValidationDiagnosticRuntimeMisalignment    = "runtime_internal_contract_misalignment"

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

func (d ValidationFailureDiagnostic) ToMap() map[string]any {
	ret := map[string]any{}
	if d.SchemaVersion != "" {
		ret["schemaVersion"] = d.SchemaVersion
	}
	if d.Class != "" {
		ret["class"] = d.Class
	}
	if d.Reason != "" {
		ret["reason"] = d.Reason
	}
	if d.RecommendedEvidenceTargetType != "" {
		ret["recommendedEvidenceTargetType"] = d.RecommendedEvidenceTargetType
	}
	if d.Stage != "" {
		ret["stage"] = d.Stage
	}
	if d.Operation != "" {
		ret["operation"] = d.Operation
	}
	if d.CommandKind != "" {
		ret["commandKind"] = d.CommandKind
	}
	if d.ExitCode != 0 {
		ret["exitCode"] = d.ExitCode
	}
	if d.TimedOut {
		ret["timedOut"] = true
	}
	if d.CommandPolicyViolation {
		ret["commandPolicyViolation"] = true
	}
	if d.UnavailableCommand != "" {
		ret["unavailableCommand"] = d.UnavailableCommand
	}
	if len(d.TargetRefs) > 0 {
		ret["targetRefs"] = append([]string(nil), d.TargetRefs...)
	}
	if d.ObservationType != "" {
		ret["observationType"] = d.ObservationType
	}
	if d.EvidenceMode != "" {
		ret["evidenceMode"] = d.EvidenceMode
	}
	if d.StdoutPreview != "" {
		ret["stdoutPreview"] = d.StdoutPreview
	}
	if d.StderrPreview != "" {
		ret["stderrPreview"] = d.StderrPreview
	}
	return ret
}
