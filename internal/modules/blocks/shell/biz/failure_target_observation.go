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
	shellValidationDiagnosticMaxTargetRefs = runtimecontract.RuntimeValidationFailureObservationMaxCandidateTargetRefs
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
		len(record.TargetRefs) == 0 || len(record.TargetRefs) > shellValidationDiagnosticMaxTargetRefs {
		return candidate
	}
	for _, value := range record.TargetRefs {
		if strings.TrimSpace(value) == "" {
			return candidate
		}
	}
	refs := canonicalShellFailureTargetRefs(record.TargetRefs)
	if len(refs) == 0 || len(refs) > shellValidationDiagnosticMaxTargetRefs {
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
func shellFailureTargetCandidateUnion(result RuntimeCommandResult, targetRole string, validationScopeTargetRefs []string) ([]string, bool) {
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil || result.validationFailureTargetCandidatesInvalid {
		return nil, false
	}
	diagnosticTargetRefs := canonicalShellFailureTargetRefs(diagnostic.TargetRefs)
	if len(diagnosticTargetRefs) > shellValidationDiagnosticMaxTargetRefs ||
		(len(diagnosticTargetRefs) != 0 && !shellTargetRefsSubset(diagnosticTargetRefs, validationScopeTargetRefs)) {
		return nil, false
	}
	marker := decodeShellFailureTargetMarkerCandidate(result, targetRole)
	if marker.Present && (!marker.Valid || !shellTargetRefsSubset(marker.TargetRefs, validationScopeTargetRefs)) {
		return nil, false
	}
	refs, valid := shellMergeFailureTargetCandidateRefs(diagnosticTargetRefs, marker.TargetRefs)
	if !valid {
		return nil, false
	}
	return refs, true
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
