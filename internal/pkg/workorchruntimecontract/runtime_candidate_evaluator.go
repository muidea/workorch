package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeCandidateCorrectionDisposition is the only same-contract correction
// outcome a complete candidate rejection may carry. SM-3 consumes it; no other
// component reinterprets raw reason text to choose retry or terminal.
type RuntimeCandidateCorrectionDisposition string

const (
	// RuntimeCandidateCorrectionCorrectableOnce means there is exactly one safe
	// structural correction under the same sealed manifest. Attempt 1 may open
	// only when a non-empty CorrectionDirective can be verified from the
	// rejection + manifest.
	RuntimeCandidateCorrectionCorrectableOnce RuntimeCandidateCorrectionDisposition = "correctable_once"
	// RuntimeCandidateCorrectionRejectedFinal means no safe same-contract
	// correction exists. SM-3 must form a typed terminal with zero correction
	// provider call.
	RuntimeCandidateCorrectionRejectedFinal RuntimeCandidateCorrectionDisposition = "rejected_final"
)

// RuntimeCandidateCorrectionDirective is the pure structural delta for the one
// permitted same-manifest correction. It is deliberately free of raw
// provider/decoder/owner error text and of Runtime action selection.
type RuntimeCandidateCorrectionDirective struct {
	Disposition  RuntimeCandidateCorrectionDisposition
	Objectives   []RuntimeCandidateCorrectionObjective
	ManifestHash string
	// TerminalCode is required when Disposition is rejected_final and empty
	// otherwise. It is the stable terminal code SM-3 must seal; it is not a
	// free-form reason.
	TerminalCode string
}

func (d RuntimeCandidateCorrectionDirective) Normalized() RuntimeCandidateCorrectionDirective {
	d.Disposition = RuntimeCandidateCorrectionDisposition(strings.TrimSpace(string(d.Disposition)))
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.TerminalCode = strings.TrimSpace(d.TerminalCode)
	seen := map[RuntimeCandidateCorrectionObjective]struct{}{}
	objectives := make([]RuntimeCandidateCorrectionObjective, 0, len(d.Objectives))
	for _, objective := range d.Objectives {
		objective = RuntimeCandidateCorrectionObjective(strings.TrimSpace(string(objective)))
		if objective == "" {
			continue
		}
		if _, found := seen[objective]; found {
			continue
		}
		seen[objective] = struct{}{}
		objectives = append(objectives, objective)
	}
	// Stable sort via the existing Normalized path on a diagnostic shell.
	d.Objectives = (&RuntimeCandidateFailureDiagnostic{Objectives: objectives}).Normalized().Objectives
	return d
}

func (d RuntimeCandidateCorrectionDirective) Validate() error {
	d = d.Normalized()
	switch d.Disposition {
	case RuntimeCandidateCorrectionCorrectableOnce:
		if len(d.Objectives) == 0 {
			return fmt.Errorf("correctable_once directive requires at least one objective")
		}
		if d.TerminalCode != "" {
			return fmt.Errorf("correctable_once directive must not carry a terminal code")
		}
		return nil
	case RuntimeCandidateCorrectionRejectedFinal:
		if d.TerminalCode == "" {
			return fmt.Errorf("rejected_final directive requires a stable terminal code")
		}
		if len(d.Objectives) != 0 {
			return fmt.Errorf("rejected_final directive must not carry correction objectives")
		}
		return nil
	default:
		return fmt.Errorf("candidate correction disposition %q is not declared", d.Disposition)
	}
}

// RuntimeCandidateRejectionCodeDisposition returns the sealed CorrectionDisposition
// for a complete-candidate rejection code. Codes that are not in this matrix are
// not ordinary model errors: callers must treat them as runtime_contract_defect.
func RuntimeCandidateRejectionCodeDisposition(code string) (RuntimeCandidateCorrectionDisposition, bool) {
	switch strings.TrimSpace(code) {
	case "candidate_contract_invalid",
		"candidate_schema_invalid",
		"candidate_replays_rejected_shape",
		"candidate_partially_replays_rejected_shape",
		"candidate_replays_no_progress_shape",
		"candidate_action_evidence_mismatch",
		"candidate_target_outside_grounding",
		"candidate_creation_scope_missing_file_target",
		"operation_target_not_regular":
		return RuntimeCandidateCorrectionCorrectableOnce, true
	case "candidate_replays_rejected_candidate_shape",
		"candidate_retry_exhausted",
		"workspace_resolution_failed",
		"operation_feasibility_probe_invalid",
		"provider",
		"provider_transient",
		"provider_authentication",
		"provider_configuration",
		"runtime_contract_defect":
		return RuntimeCandidateCorrectionRejectedFinal, true
	default:
		return "", false
	}
}

// RuntimeCandidateRejectionTerminalCode is the stable terminal code for a
// rejected_final complete rejection. Correctable codes return empty.
func RuntimeCandidateRejectionTerminalCode(code string) string {
	disposition, ok := RuntimeCandidateRejectionCodeDisposition(code)
	if !ok || disposition != RuntimeCandidateCorrectionRejectedFinal {
		return ""
	}
	switch strings.TrimSpace(code) {
	case "candidate_replays_rejected_candidate_shape":
		return "candidate_retry_exhausted"
	case "candidate_retry_exhausted":
		return "candidate_retry_exhausted"
	case "workspace_resolution_failed", "operation_feasibility_probe_invalid",
		"provider", "provider_transient", "provider_authentication", "provider_configuration":
		return strings.TrimSpace(code)
	case "runtime_contract_defect":
		return "runtime_contract_defect"
	default:
		return strings.TrimSpace(code)
	}
}

// SealRuntimeCandidateRejection completes a complete-document rejection with
// CorrectionDisposition and, when correctable, a non-empty objective set. When
// the code cannot be classified, or a correctable rejection has no structural
// objective, the result is runtime_contract_defect rather than
// candidate_correction_undefined.
//
// This is a pure algebra helper: it does not choose Runtime action, CAS, or
// provider I/O. SM-3 consumes the sealed failure.
func SealRuntimeCandidateRejection(failure *RuntimeCandidateFailure, manifest RuntimeCandidateContractManifest) (*RuntimeCandidateFailure, *RuntimeCandidateCorrectionDirective, error) {
	if failure == nil {
		return nil, nil, fmt.Errorf("runtime_contract_defect: complete rejection is missing")
	}
	sealed := *failure
	sealed.Class = RuntimeCandidateFailureClass(strings.TrimSpace(string(sealed.Class)))
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
		sealed.Diagnostic.Objectives = RuntimeCandidateCorrectionObjectivesForFailure(&sealed)
		sealed.Diagnostic = sealed.Diagnostic.Clone()
	}

	disposition, known := RuntimeCandidateRejectionCodeDisposition(sealed.Code)
	if !known {
		defect := runtimeContractDefectFailure(sealed, "complete rejection code is not declared in the correction matrix")
		directive := &RuntimeCandidateCorrectionDirective{
			Disposition:  RuntimeCandidateCorrectionRejectedFinal,
			ManifestHash: manifest.Hash,
			TerminalCode: "runtime_contract_defect",
		}
		return defect, directive.Normalized().ptr(), nil
	}

	switch disposition {
	case RuntimeCandidateCorrectionCorrectableOnce:
		objectives := RuntimeCandidateCorrectionObjectivesForFailure(&sealed)
		if len(objectives) == 0 {
			defect := runtimeContractDefectFailure(sealed, "correctable rejection has no manifest-bound correction objective")
			directive := &RuntimeCandidateCorrectionDirective{
				Disposition:  RuntimeCandidateCorrectionRejectedFinal,
				ManifestHash: manifest.Hash,
				TerminalCode: "runtime_contract_defect",
			}
			return defect, directive.Normalized().ptr(), nil
		}
		if sealed.Diagnostic == nil {
			sealed.Diagnostic = &RuntimeCandidateFailureDiagnostic{}
		}
		sealed.Diagnostic.Objectives = objectives
		if sealed.Diagnostic.ManifestHash == "" {
			sealed.Diagnostic.ManifestHash = manifest.Hash
		}
		sealed.Diagnostic = sealed.Diagnostic.Clone()
		directive := &RuntimeCandidateCorrectionDirective{
			Disposition:  RuntimeCandidateCorrectionCorrectableOnce,
			Objectives:   objectives,
			ManifestHash: manifest.Hash,
		}
		return &sealed, directive.Normalized().ptr(), nil
	case RuntimeCandidateCorrectionRejectedFinal:
		directive := &RuntimeCandidateCorrectionDirective{
			Disposition:  RuntimeCandidateCorrectionRejectedFinal,
			ManifestHash: manifest.Hash,
			TerminalCode: RuntimeCandidateRejectionTerminalCode(sealed.Code),
		}
		return &sealed, directive.Normalized().ptr(), nil
	default:
		defect := runtimeContractDefectFailure(sealed, "complete rejection disposition is not declared")
		directive := &RuntimeCandidateCorrectionDirective{
			Disposition:  RuntimeCandidateCorrectionRejectedFinal,
			ManifestHash: manifest.Hash,
			TerminalCode: "runtime_contract_defect",
		}
		return defect, directive.Normalized().ptr(), nil
	}
}

func (d RuntimeCandidateCorrectionDirective) ptr() *RuntimeCandidateCorrectionDirective {
	clone := d.Normalized()
	return &clone
}

func runtimeContractDefectFailure(source RuntimeCandidateFailure, detail string) *RuntimeCandidateFailure {
	message := strings.TrimSpace(detail)
	if message == "" {
		message = "complete candidate rejection is not classifiable"
	}
	return &RuntimeCandidateFailure{
		Class:      RuntimeCandidateFailureClassInternal,
		Code:       "runtime_contract_defect",
		Signature:  "runtime-contract-defect",
		Reason:     message,
		TargetRefs: append([]string(nil), source.TargetRefs...),
		Diagnostic: source.Diagnostic.Clone(),
	}
}

// RuntimeCandidateFailureIsCorrectableOnce reports whether a sealed complete
// rejection may open attempt 1. It never reads Reason.
func RuntimeCandidateFailureIsCorrectableOnce(failure *RuntimeCandidateFailure) bool {
	if failure == nil {
		return false
	}
	disposition, ok := RuntimeCandidateRejectionCodeDisposition(failure.Code)
	if !ok || disposition != RuntimeCandidateCorrectionCorrectableOnce {
		return false
	}
	return len(RuntimeCandidateCorrectionObjectivesForFailure(failure)) != 0
}

// RuntimeCandidateFailureIsRejectedFinal reports whether a sealed complete
// rejection must terminal without a correction call.
func RuntimeCandidateFailureIsRejectedFinal(failure *RuntimeCandidateFailure) bool {
	if failure == nil {
		return false
	}
	disposition, ok := RuntimeCandidateRejectionCodeDisposition(failure.Code)
	return ok && disposition == RuntimeCandidateCorrectionRejectedFinal
}
