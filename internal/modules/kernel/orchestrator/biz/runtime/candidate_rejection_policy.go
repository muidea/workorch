package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CandidateRejectionCodeDisposition is the SM-3 owner-local correction policy.
// The shared runtime contract deliberately carries only the DTO vocabulary;
// deciding whether a complete rejection may open attempt 1 or must terminal is
// a state-machine transition owned exclusively by Orchestrator.
func CandidateRejectionCodeDisposition(code string) (runtimecontract.RuntimeCandidateCorrectionDisposition, bool) {
	switch strings.TrimSpace(code) {
	case "candidate_contract_invalid",
		"candidate_schema_invalid",
		"candidate_replays_rejected_shape",
		"candidate_partially_replays_rejected_shape",
		"candidate_replays_no_progress_shape",
		"candidate_action_evidence_mismatch",
		"candidate_target_outside_grounding",
		"candidate_creation_scope_missing_file_target":
		return runtimecontract.RuntimeCandidateCorrectionCorrectableOnce, true
	case "candidate_replays_rejected_candidate_shape",
		"candidate_retry_exhausted",
		"workspace_resolution_failed",
		"operation_feasibility_probe_invalid",
		"provider",
		"provider_transient",
		"provider_authentication",
		"provider_configuration",
		"runtime_contract_defect":
		return runtimecontract.RuntimeCandidateCorrectionRejectedFinal, true
	default:
		return "", false
	}
}

// CandidateRejectionTerminalCode returns the stable terminal code selected by
// the SM-3 owner-local rejected-final policy. Correctable codes return empty.
func CandidateRejectionTerminalCode(code string) string {
	disposition, ok := CandidateRejectionCodeDisposition(code)
	if !ok || disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal {
		return ""
	}
	switch strings.TrimSpace(code) {
	case "candidate_replays_rejected_candidate_shape", "candidate_retry_exhausted":
		return "candidate_retry_exhausted"
	case "workspace_resolution_failed", "operation_feasibility_probe_invalid",
		"provider", "provider_transient", "provider_authentication", "provider_configuration", "runtime_contract_defect":
		return strings.TrimSpace(code)
	default:
		return strings.TrimSpace(code)
	}
}

// CandidateCorrectionObjectivesForFailure derives the bounded same-manifest
// structural delta from an already-preserved candidate diagnostic. It is an
// SM-3 policy projection, not a shared contract helper and not LLM logic.
func CandidateCorrectionObjectivesForFailure(failure *runtimecontract.RuntimeCandidateFailure) []runtimecontract.RuntimeCandidateCorrectionObjective {
	if failure == nil {
		return nil
	}
	var diagnostic runtimecontract.RuntimeCandidateFailureDiagnostic
	if failure.Diagnostic != nil {
		diagnostic = failure.Diagnostic.Normalized()
	}
	objectives := append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), diagnostic.Objectives...)
	if len(diagnostic.Forbidden) != 0 {
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionRemoveForbiddenField)
	}
	if len(diagnostic.Required) != 0 {
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField)
	}
	if (strings.Contains(diagnostic.Path, "targetRefs") || strings.Contains(diagnostic.Path, "targetRef")) && len(diagnostic.Allowed) != 0 {
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionReplaceTarget)
	}
	if strings.HasSuffix(diagnostic.Path, ".kind") && len(diagnostic.Allowed) != 0 {
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSelectOperationKind)
	}
	if strings.Contains(diagnostic.Path, "coverage") {
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSatisfyCoverage)
	}
	if strings.Contains(diagnostic.Path, "dependsOn") {
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSatisfyDependency)
	}
	if strings.Contains(diagnostic.Path, ".parameters") {
		switch {
		case diagnostic.Actual == "unmatched_variant" && len(diagnostic.Allowed) != 0:
			objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape)
		case strings.HasPrefix(diagnostic.Actual, "missing_") || strings.HasPrefix(diagnostic.Actual, "empty_"):
			objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField)
		case len(diagnostic.Forbidden) != 0 || strings.HasPrefix(diagnostic.Actual, "duplicate_"):
			objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionRemoveForbiddenField)
		case len(diagnostic.Allowed) != 0:
			objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape)
		}
	}
	switch strings.TrimSpace(failure.Code) {
	case "candidate_contract_invalid", "candidate_schema_invalid":
		switch {
		case diagnostic.Path == "", diagnostic.Path == "$", diagnostic.Path == "$.operations",
			diagnostic.Actual == "candidate_shape", diagnostic.Actual == "missing_id", diagnostic.Actual == "",
			strings.HasPrefix(diagnostic.Actual, "duplicate_"):
			objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField)
		}
	case "candidate_replays_rejected_shape", "candidate_partially_replays_rejected_shape", "candidate_replays_no_progress_shape":
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape)
	case "candidate_target_outside_grounding", "candidate_creation_scope_missing_file_target":
		if len(diagnostic.Allowed) != 0 {
			objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionReplaceTarget)
		}
	case "candidate_action_evidence_mismatch":
		objectives = append(objectives, runtimecontract.RuntimeCandidateCorrectionSatisfyCoverage)
	}
	return (&runtimecontract.RuntimeCandidateFailureDiagnostic{Objectives: objectives}).Normalized().Objectives
}

// SealCandidateRejection is the only SM-3 policy that converts a complete
// candidate rejection into either one bounded same-manifest correction or a
// terminal outcome. It does not mutate an aggregate; callers persist the
// returned facts through the owner-local reducer/CAS transition.
func SealCandidateRejection(failure *runtimecontract.RuntimeCandidateFailure, manifest runtimecontract.RuntimeCandidateContractManifest) (*runtimecontract.RuntimeCandidateFailure, *runtimecontract.RuntimeCandidateCorrectionDirective, error) {
	if failure == nil {
		return nil, nil, fmt.Errorf("runtime_contract_defect: complete rejection is missing")
	}
	sealed := *failure
	sealed.Class = runtimecontract.RuntimeCandidateFailureClass(strings.TrimSpace(string(sealed.Class)))
	sealed.Code = strings.TrimSpace(sealed.Code)
	sealed.Signature = strings.TrimSpace(sealed.Signature)
	sealed.Reason = strings.TrimSpace(sealed.Reason)
	if sealed.Diagnostic != nil {
		sealed.Diagnostic = sealed.Diagnostic.Clone()
	}
	manifest = manifest.Normalized()
	if sealed.Diagnostic != nil && sealed.Diagnostic.ManifestHash == "" && manifest.Hash != "" {
		sealed.Diagnostic.ManifestHash = manifest.Hash
	}
	if sealed.Diagnostic != nil && len(sealed.Diagnostic.Objectives) == 0 {
		sealed.Diagnostic.Objectives = CandidateCorrectionObjectivesForFailure(&sealed)
		sealed.Diagnostic = sealed.Diagnostic.Clone()
	}

	disposition, known := CandidateRejectionCodeDisposition(sealed.Code)
	if !known {
		return candidateRejectionContractDefect(sealed, "complete rejection code is not declared in the correction matrix"), rejectedFinalDirective(manifest.Hash, "runtime_contract_defect"), nil
	}
	switch disposition {
	case runtimecontract.RuntimeCandidateCorrectionCorrectableOnce:
		objectives := CandidateCorrectionObjectivesForFailure(&sealed)
		if len(objectives) == 0 {
			return candidateRejectionContractDefect(sealed, "correctable rejection has no manifest-bound correction objective"), rejectedFinalDirective(manifest.Hash, "runtime_contract_defect"), nil
		}
		if sealed.Diagnostic == nil {
			sealed.Diagnostic = &runtimecontract.RuntimeCandidateFailureDiagnostic{}
		}
		sealed.Diagnostic.Objectives = objectives
		if sealed.Diagnostic.ManifestHash == "" {
			sealed.Diagnostic.ManifestHash = manifest.Hash
		}
		sealed.Diagnostic = sealed.Diagnostic.Clone()
		directive := runtimecontract.RuntimeCandidateCorrectionDirective{
			Disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce,
			Objectives:  objectives, ManifestHash: manifest.Hash,
		}.Normalized()
		return &sealed, &directive, nil
	case runtimecontract.RuntimeCandidateCorrectionRejectedFinal:
		return &sealed, rejectedFinalDirective(manifest.Hash, CandidateRejectionTerminalCode(sealed.Code)), nil
	default:
		return candidateRejectionContractDefect(sealed, "complete rejection disposition is not declared"), rejectedFinalDirective(manifest.Hash, "runtime_contract_defect"), nil
	}
}

func rejectedFinalDirective(manifestHash, terminalCode string) *runtimecontract.RuntimeCandidateCorrectionDirective {
	directive := runtimecontract.RuntimeCandidateCorrectionDirective{
		Disposition:  runtimecontract.RuntimeCandidateCorrectionRejectedFinal,
		ManifestHash: manifestHash, TerminalCode: terminalCode,
	}.Normalized()
	return &directive
}

func candidateRejectionContractDefect(source runtimecontract.RuntimeCandidateFailure, detail string) *runtimecontract.RuntimeCandidateFailure {
	message := strings.TrimSpace(detail)
	if message == "" {
		message = "complete candidate rejection is not classifiable"
	}
	return &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInternal, Code: "runtime_contract_defect",
		Signature: "runtime-contract-defect", Reason: message,
		TargetRefs: append([]string(nil), source.TargetRefs...), Diagnostic: source.Diagnostic.Clone(),
	}
}

// CandidateFailureIsCorrectableOnce reports whether persisted SM-3 failure may
// open attempt 1. It never derives state from raw reason text.
func CandidateFailureIsCorrectableOnce(failure *runtimecontract.RuntimeCandidateFailure) bool {
	if failure == nil {
		return false
	}
	disposition, ok := CandidateRejectionCodeDisposition(failure.Code)
	return ok && disposition == runtimecontract.RuntimeCandidateCorrectionCorrectableOnce && len(CandidateCorrectionObjectivesForFailure(failure)) != 0
}

// CandidateFailureIsRejectedFinal reports whether a persisted SM-3 failure
// must terminal without a correction provider request.
func CandidateFailureIsRejectedFinal(failure *runtimecontract.RuntimeCandidateFailure) bool {
	if failure == nil {
		return false
	}
	disposition, ok := CandidateRejectionCodeDisposition(failure.Code)
	return ok && disposition == runtimecontract.RuntimeCandidateCorrectionRejectedFinal
}
