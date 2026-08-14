package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	shellFailureTargetObservationPrefix  = "WORKORCH_FAILURE_TARGET="
	shellFailureTargetObservationSchema  = "workorch.shell.failure_target"
	shellFailureTargetObservationSubject = "shell_runtime_policy"
	shellFailureTargetObservationMaxRefs = 8
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

func shellFailureTargetObservation(result RuntimeCommandResult, targetRole, ownerRevision string) *runtimecontract.RuntimeFailureTargetObservation {
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil || strings.TrimSpace(diagnostic.Class) == "" || strings.TrimSpace(targetRole) == "" || strings.TrimSpace(ownerRevision) == "" {
		return nil
	}
	record, raw, ok := parseShellFailureTargetRecord(result.Stdout)
	if !ok || record.TargetRole != strings.TrimSpace(targetRole) || record.DiagnosticClass != strings.TrimSpace(diagnostic.Class) || len(record.TargetRefs) > shellFailureTargetObservationMaxRefs {
		return nil
	}
	hash := sha256.Sum256([]byte(raw))
	observation := runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID:    ModuleID,
		SubjectRef:       shellFailureTargetObservationSubject,
		TargetRole:       record.TargetRole,
		ObservedRevision: ownerRevision,
		TargetRefs:       record.TargetRefs,
		DiagnosticClass:  record.DiagnosticClass,
		EvidenceRefs:     []string{"workorch://shell/failure-target/sha256/" + hex.EncodeToString(hash[:])},
		ContentHash:      hex.EncodeToString(hash[:]),
	}.Normalized()
	if err := observation.Validate(); err != nil || len(observation.TargetRefs) == 0 || len(observation.TargetRefs) > shellFailureTargetObservationMaxRefs {
		return nil
	}
	return &observation
}

func parseShellFailureTargetRecord(stdout string) (shellFailureTargetRecord, string, bool) {
	var record shellFailureTargetRecord
	var raw string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, shellFailureTargetObservationPrefix) {
			continue
		}
		if raw != "" {
			return shellFailureTargetRecord{}, "", false
		}
		raw = strings.TrimSpace(strings.TrimPrefix(line, shellFailureTargetObservationPrefix))
	}
	if raw == "" {
		return shellFailureTargetRecord{}, "", false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return shellFailureTargetRecord{}, "", false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return shellFailureTargetRecord{}, "", false
	}
	record.SchemaVersion = strings.TrimSpace(record.SchemaVersion)
	record.TargetRole = strings.TrimSpace(record.TargetRole)
	record.DiagnosticClass = strings.TrimSpace(record.DiagnosticClass)
	if record.SchemaVersion != shellFailureTargetObservationSchema || record.TargetRole == "" || record.DiagnosticClass == "" {
		return shellFailureTargetRecord{}, "", false
	}
	return record, raw, true
}
