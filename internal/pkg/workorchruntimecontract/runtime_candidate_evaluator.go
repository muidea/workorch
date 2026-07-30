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
