package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"path"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeValidationFailureDetailLimit = 32

// RuntimeValidationCheckFailure is a capability-owner observation, not a repair
// authorization. DiagnosticHash addresses the assertion text without exporting
// logs, expected/actual secrets or tool-private messages. TargetRef is a bounded
// location in the sealed validation scope; it need not be the faulty resource.
type RuntimeValidationCheckFailure struct {
	CheckRef       string `json:"checkRef"`
	CaseRef        string `json:"caseRef,omitempty"`
	PackageRef     string `json:"packageRef,omitempty"`
	TargetRef      string `json:"targetRef,omitempty"`
	Line           int    `json:"line,omitempty"`
	DiagnosticHash string `json:"diagnosticHash"`
}

func normalizeRuntimeValidationCheckFailures(values []RuntimeValidationCheckFailure) []RuntimeValidationCheckFailure {
	ret := append([]RuntimeValidationCheckFailure(nil), values...)
	for i := range ret {
		ret[i].CheckRef = strings.TrimSpace(ret[i].CheckRef)
		ret[i].CaseRef = strings.TrimSpace(ret[i].CaseRef)
		ret[i].PackageRef = strings.TrimSpace(ret[i].PackageRef)
		ret[i].TargetRef = strings.TrimSpace(ret[i].TargetRef)
		ret[i].DiagnosticHash = strings.TrimSpace(ret[i].DiagnosticHash)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		l, r := ret[i], ret[j]
		if l.CheckRef != r.CheckRef {
			return l.CheckRef < r.CheckRef
		}
		if l.CaseRef != r.CaseRef {
			return l.CaseRef < r.CaseRef
		}
		if l.PackageRef != r.PackageRef {
			return l.PackageRef < r.PackageRef
		}
		if l.TargetRef != r.TargetRef {
			return l.TargetRef < r.TargetRef
		}
		if l.Line != r.Line {
			return l.Line < r.Line
		}
		return l.DiagnosticHash < r.DiagnosticHash
	})
	return ret
}

// ValidateFailureDetails does not require every tool to support native check
// diagnostics. If supplied, they must be bounded, safe and attached to an actual
// failed check. Missing details remain explicitly observable at the renderer.
func (o RuntimeValidationObservation) ValidateFailureDetails() error {
	if len(o.CheckFailures) > RuntimeValidationFailureDetailLimit {
		return fmt.Errorf("validation failure detail bound exceeded")
	}
	seen := make(map[RuntimeValidationCheckFailure]struct{}, len(o.CheckFailures))
	for _, value := range o.CheckFailures {
		if !containsString(o.FailedChecks, value.CheckRef) || value.CheckRef == "" {
			return fmt.Errorf("validation failure detail requires an observed failed check")
		}
		for _, ref := range []string{value.CheckRef, value.CaseRef, value.PackageRef, value.TargetRef} {
			if len(ref) > 512 || strings.IndexFunc(ref, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
				return fmt.Errorf("validation failure detail has unsafe reference")
			}
		}
		if value.Line < 0 || (value.Line > 0 && value.TargetRef == "") ||
			(value.TargetRef != "" && (path.IsAbs(value.TargetRef) || path.Clean(value.TargetRef) != value.TargetRef ||
				value.TargetRef == "." || value.TargetRef == ".." || strings.HasPrefix(value.TargetRef, "../") || strings.ContainsAny(value.TargetRef, "\\:"))) {
			return fmt.Errorf("validation failure detail requires a relative location")
		}
		if len(value.DiagnosticHash) != sha256.Size*2 || strings.ToLower(value.DiagnosticHash) != value.DiagnosticHash {
			return fmt.Errorf("validation failure detail requires sha256 diagnostic identity")
		}
		if _, err := hex.DecodeString(value.DiagnosticHash); err != nil {
			return fmt.Errorf("invalid validation diagnostic identity: %w", err)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate validation failure detail")
		}
		seen[value] = struct{}{}
	}
	return nil
}

// Preserve exact hashes of records with no detail extension. This is an
// optional fact extension, never an archive rewrite or authority migration.
func writeRuntimeValidationFailureDetailsHash(h hash.Hash, observation RuntimeValidationObservation) {
	if len(observation.CheckFailures) == 0 && !observation.FailureDetailsTruncated && !observation.OutputTruncated && len(observation.IncompleteChecks) == 0 {
		return
	}
	writeHashString(h, "validation-check-failure-details/v1")
	writeHashString(h, fmt.Sprint(observation.FailureDetailsTruncated))
	writeHashString(h, fmt.Sprint(observation.OutputTruncated))
	writeHashStrings(h, normalizeStrings(observation.IncompleteChecks))
	for _, detail := range normalizeRuntimeValidationCheckFailures(observation.CheckFailures) {
		writeHashStrings(h, []string{detail.CheckRef, detail.CaseRef, detail.PackageRef, detail.TargetRef, fmt.Sprint(detail.Line), detail.DiagnosticHash})
	}
}

// RuntimeValidationFailureFingerprint compares failure observations, not
// transaction identities. Closure, dispatch, evidence revision, target revision
// and RequiredOutputHash deliberately remain in their existing fencing hashes.
// This value cannot authorize a receipt, candidate, repair surface or credit.
func RuntimeValidationFailureFingerprint(fact EvidenceFact, bindings ...CoverageEvidenceBinding) string {
	coverageIDs := runtimeValidationFailureCoverageIDs(fact, bindings)
	if fact.Kind != EvidenceKindValidation || fact.Validation == nil || len(coverageIDs) == 0 {
		return ""
	}
	o := fact.Validation.Normalized()
	if o.RecipeID == "" || o.ObservationProtocolRef == "" || len(o.FailedChecks) == 0 || o.ValidateFailureDetails() != nil {
		return ""
	}
	h := sha256.New()
	writeHashString(h, "validation-failure-comparison/v1")
	for _, id := range coverageIDs {
		writeHashString(h, string(id))
	}
	writeHashStrings(h, []string{string(fact.FailureClass), fact.FailureCode, o.RecipeID, o.ObservationProtocolRef})
	writeHashStrings(h, o.FailedChecks)
	// Location/line changes are not evidence that an assertion was repaired.
	// Dedup because one assertion can be logged from multiple call locations.
	diagnostics := make([]string, 0, len(o.CheckFailures))
	for _, detail := range o.CheckFailures {
		diagnostics = append(diagnostics, strings.Join([]string{detail.CheckRef, detail.CaseRef, detail.PackageRef, detail.DiagnosticHash}, "\x00"))
	}
	writeHashStrings(h, normalizeStrings(diagnostics))
	writeHashString(h, fmt.Sprint(o.FailureDetailsTruncated))
	writeHashString(h, fmt.Sprint(o.OutputTruncated))
	writeHashStrings(h, o.IncompleteChecks)
	return hex.EncodeToString(h.Sum(nil))
}

// Rejected evidence deliberately has no CWI credit. Resolve its diagnostic
// attribution only through the supplied sealed bindings, never by filling in
// CoverageWorkItemIDs or guessing from the current focus. An unknown binding
// gives no comparison identity; this helper cannot grant proof or credit.
func runtimeValidationFailureCoverageIDs(fact EvidenceFact, bindings []CoverageEvidenceBinding) []common.CoverageWorkItemID {
	if len(fact.CoverageWorkItemIDs) != 0 {
		return normalizeCoverageWorkItemIDs(fact.CoverageWorkItemIDs)
	}
	ids := make([]common.CoverageWorkItemID, 0, len(fact.BindingRefs))
	for _, ref := range normalizeStrings(fact.BindingRefs) {
		id := common.CoverageWorkItemID("")
		for _, binding := range bindings {
			if binding.ID != ref {
				continue
			}
			if id != "" && id != binding.CoverageWorkItemID {
				return nil
			}
			id = binding.CoverageWorkItemID
		}
		if id == "" {
			return nil
		}
		ids = append(ids, id)
	}
	return normalizeCoverageWorkItemIDs(ids)
}

// RuntimeValidationFailureOccurrences is a pure, same-Run evidence-ledger
// comparison. Only the Orchestrator's private receipt event may consume the
// result to update its existing repair ledger. Readers only display it.
func RuntimeValidationFailureOccurrences(ledger EvidenceLedger, current EvidenceFact, bindings ...CoverageEvidenceBinding) int {
	fingerprint := RuntimeValidationFailureFingerprint(current, bindings...)
	if fingerprint == "" {
		return 0
	}
	// A later, credited pass of these exact checks closes their comparison
	// window. An unrelated pass, read or causation-only repair cannot reset it.
	resolvedAt := ""
	coverageIDs := runtimeValidationFailureCoverageIDs(current, bindings)
	currentObservation := current.Validation.Normalized()
	for _, fact := range ledger.Accepted {
		if fact.Kind != EvidenceKindValidation || fact.Validation == nil || fact.Revision == "" ||
			len(fact.CoverageWorkItemIDs) == 0 || !sameCoverageWorkItemIDs(fact.CoverageWorkItemIDs, coverageIDs) {
			continue
		}
		observation := fact.Validation.Normalized()
		if observation.RecipeID != currentObservation.RecipeID || observation.ObservationProtocolRef != currentObservation.ObservationProtocolRef {
			continue
		}
		passed := true
		for _, check := range currentObservation.FailedChecks {
			passed = passed && RuntimeValidationCheckPassed(observation, check)
		}
		if passed && EvidenceRevisionAfter(fact.Revision, resolvedAt) {
			resolvedAt = fact.Revision
		}
	}
	seen := make(map[string]struct{})
	for _, fact := range ledger.Rejected {
		if fact.ID != "" && (resolvedAt == "" || EvidenceRevisionAfter(fact.Revision, resolvedAt)) && RuntimeValidationFailureFingerprint(fact, bindings...) == fingerprint {
			seen[string(fact.ID)] = struct{}{}
		}
	}
	return len(seen)
}
