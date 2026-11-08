package shell

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	shellFailureTargetMarkerPrefix         = "WORKORCH_FAILURE_TARGET="
	shellFailureTargetMarkerSchema         = "workorch.shell.failure_target"
	shellValidationDiagnosticMaxTargetRefs = runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs
)

// shellFailureTargetRecord is intentionally a closed JSONL protocol. It is
// parsed only from stdout of the executed tool; command targets, stderr and
// diagnostic previews never enter this path.
type shellFailureTargetRecord struct {
	SchemaVersion   string   `json:"schemaVersion"`
	TargetRole      string   `json:"targetRole"`
	TargetRefs      []string `json:"targetRefs"`
	DiagnosticClass string   `json:"diagnosticClass"`
}

type shellFailureTargetMarkerCandidate struct {
	Present    bool
	Valid      bool
	TargetRefs []string
	Rejection  *runtimecontract.RuntimeValidationFailureTargetRejection
}

// decodeShellFailureTargetMarkerCandidate decodes only a bounded diagnostic
// hint.  A
// present-but-invalid marker is deliberately distinct from an absent marker:
// the former invalidates the complete marker+tool candidate transaction and
// must never let the caller fall back to only the tool-derived subset.
func decodeShellFailureTargetMarkerCandidate(result RuntimeCommandResult, targetRole string) shellFailureTargetMarkerCandidate {
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil || strings.TrimSpace(diagnostic.Class) == "" || strings.TrimSpace(targetRole) == "" {
		return shellFailureTargetMarkerCandidate{}
	}
	record, _, present, valid := parseShellFailureTargetRecord(result.Stdout)
	if !present {
		return shellFailureTargetMarkerCandidate{}
	}
	candidate := shellFailureTargetMarkerCandidate{Present: true}
	if !valid || record.TargetRole != strings.TrimSpace(targetRole) || record.DiagnosticClass != strings.TrimSpace(diagnostic.Class) ||
		len(record.TargetRefs) == 0 {
		return candidate
	}
	for _, value := range record.TargetRefs {
		if strings.TrimSpace(value) == "" {
			return candidate
		}
	}
	refs := canonicalShellFailureTargetRefs(record.TargetRefs)
	if len(refs) == 0 {
		return candidate
	}
	if len(refs) > shellValidationDiagnosticMaxTargetRefs {
		candidate.Rejection = &runtimecontract.RuntimeValidationFailureTargetRejection{Code: runtimecontract.RuntimeFailureTargetsOverCapacity, CandidateCount: len(refs), Limit: shellValidationDiagnosticMaxTargetRefs}
		return candidate
	}
	// Duplicate entries beyond the wire bound are malformed, even when their
	// deduplicated set fits. They cannot evade the closed marker contract.
	if len(record.TargetRefs) > shellValidationDiagnosticMaxTargetRefs {
		return candidate
	}
	candidate.Valid = true
	candidate.TargetRefs = refs
	return candidate
}

// shellFailureTargetCandidateUnion validates the complete owner-private
// candidate transaction against the matching sealed per-operation scope.  The
// boolean reports whether the whole transaction was admissible; an empty but
// admissible result simply means there is no scope-bound observation.
func shellFailureTargetCandidateUnion(result RuntimeCommandResult, targetRole string, validationScopeTargetRefs []string) ([]string, bool, *runtimecontract.RuntimeValidationFailureTargetRejection) {
	reject := func(code string, count int) ([]string, bool, *runtimecontract.RuntimeValidationFailureTargetRejection) {
		return nil, false, &runtimecontract.RuntimeValidationFailureTargetRejection{Code: code, CandidateCount: count, Limit: shellValidationDiagnosticMaxTargetRefs}
	}
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil || diagnostic.TargetRejection != nil || result.validationFailureTargetCandidatesInvalid {
		if diagnostic != nil && diagnostic.TargetRejection != nil {
			return nil, false, diagnostic.TargetRejection.Clone()
		}
		return reject(runtimecontract.RuntimeFailureTargetsInvalid, 0)
	}
	diagnosticTargetRefs := canonicalShellFailureTargetRefs(diagnostic.TargetRefs)
	if len(diagnosticTargetRefs) > shellValidationDiagnosticMaxTargetRefs {
		return reject(runtimecontract.RuntimeFailureTargetsOverCapacity, len(diagnosticTargetRefs))
	}
	if len(diagnosticTargetRefs) != 0 && !shellTargetRefsSubset(diagnosticTargetRefs, validationScopeTargetRefs) {
		return reject(runtimecontract.RuntimeFailureTargetsOutsideScope, len(diagnosticTargetRefs))
	}
	marker := decodeShellFailureTargetMarkerCandidate(result, targetRole)
	if diagnostic.Class == runtimecontract.ValidationDiagnosticDependencyMismatch {
		// A marker cannot widen a dependency owner's exact typed repair surface.
		if marker.Present {
			return reject(runtimecontract.RuntimeFailureTargetsInvalidMarker, len(marker.TargetRefs))
		}
		return diagnosticTargetRefs, true, nil
	}
	if marker.Rejection != nil {
		return nil, false, marker.Rejection.Clone()
	}
	if marker.Present && !marker.Valid {
		return reject(runtimecontract.RuntimeFailureTargetsInvalidMarker, len(marker.TargetRefs))
	}
	if marker.Present && !shellTargetRefsSubset(marker.TargetRefs, validationScopeTargetRefs) {
		return reject(runtimecontract.RuntimeFailureTargetsOutsideScope, len(marker.TargetRefs))
	}
	refs, valid := shellMergeFailureTargetCandidateRefs(diagnosticTargetRefs, marker.TargetRefs)
	if !valid {
		return reject(runtimecontract.RuntimeFailureTargetsOverCapacity, len(canonicalShellFailureTargetRefs(append(append([]string(nil), diagnosticTargetRefs...), marker.TargetRefs...))))
	}
	return refs, true, nil
}

func shellMergeFailureTargetCandidateRefs(diagnosticTargetRefs, markerTargetRefs []string) ([]string, bool) {
	refs := canonicalShellFailureTargetRefs(append(append([]string(nil), diagnosticTargetRefs...), markerTargetRefs...))
	if len(refs) > shellValidationDiagnosticMaxTargetRefs {
		return nil, false
	}
	return refs, true
}

func canonicalShellFailureTargetRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	refs := make([]string, 0, len(seen))
	for value := range seen {
		refs = append(refs, value)
	}
	sort.Strings(refs)
	return refs
}

func parseShellFailureTargetRecord(stdout string) (shellFailureTargetRecord, string, bool, bool) {
	var record shellFailureTargetRecord
	var raw string
	markerCount := 0
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, shellFailureTargetMarkerPrefix) {
			continue
		}
		markerCount++
		if markerCount > 1 {
			return shellFailureTargetRecord{}, "", true, false
		}
		raw = strings.TrimSpace(strings.TrimPrefix(line, shellFailureTargetMarkerPrefix))
	}
	if markerCount == 0 {
		return shellFailureTargetRecord{}, "", false, false
	}
	if raw == "" {
		return shellFailureTargetRecord{}, "", true, false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return shellFailureTargetRecord{}, "", true, false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return shellFailureTargetRecord{}, "", true, false
	}
	record.SchemaVersion = strings.TrimSpace(record.SchemaVersion)
	record.TargetRole = strings.TrimSpace(record.TargetRole)
	record.DiagnosticClass = strings.TrimSpace(record.DiagnosticClass)
	if record.SchemaVersion != shellFailureTargetMarkerSchema || record.TargetRole == "" || record.DiagnosticClass == "" {
		return shellFailureTargetRecord{}, "", true, false
	}
	return record, raw, true, true
}
