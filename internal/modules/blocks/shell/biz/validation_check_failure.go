package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var shellGoAssertionLocationPattern = regexp.MustCompile(`^\s*((?:\./)?[[:alnum:]_./-]+\.go):([0-9]+):\s*(.*)$`)

// Join the Test identity and its assertion location from the same native JSON
// event. Passing check B's output must never describe failing check A. The
// bounded transcript stays private, including assertion/expected/actual text.
func attachShellValidationCheckFailures(output *RuntimeCommandResult, request RuntimeCommandRequest) {
	if output == nil || output.ValidationObservation == nil || output.goTestJSONFacts == nil {
		return
	}
	observation := output.ValidationObservation.Clone()
	observation.CheckFailures = nil
	observation.FailureDetailsTruncated = false
	observation.FailureDetailProtocolRef = ""
	if len(observation.FailedChecks) != 0 {
		observation.FailureDetailProtocolRef = runtimecontract.RuntimeValidationFailureDetailProtocol
	}
	observation.OutputTruncated = output.OutputTruncated
	if output.OutputTruncated {
		output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
		output.EvidenceReason = "native validation transcript was truncated"
	}
	failed := make(map[string]struct{}, len(observation.FailedChecks))
	for _, check := range observation.FailedChecks {
		failed[check] = struct{}{}
	}
	scope := make(map[string]struct{}, len(request.ValidationScopeTargetRefs))
	for _, ref := range request.ValidationScopeTargetRefs {
		if target, ok := normalizeShellDiagnosticTargetRef(ref, output.WorkingDirectory); ok {
			scope[target] = struct{}{}
		}
	}
	seen := make(map[runtimecontract.RuntimeValidationCheckFailure]struct{})
	for _, record := range output.goTestJSONFacts.DiagnosticRecords {
		check, _, _ := strings.Cut(record.Test, "/")
		if _, ok := failed[check]; !ok || record.Action != "output" {
			continue
		}
		if outcome := output.goTestJSONFacts.CaseOutcomes[record.Package+"\x00"+record.Test]; outcome != "" && outcome != "fail" {
			continue
		}
		for _, line := range strings.Split(record.Output, "\n") {
			match := shellGoAssertionLocationPattern.FindStringSubmatch(line)
			if len(match) != 4 {
				continue
			}
			target, ok := shellDiagnosticTargetInScope(match[1], record.Package, record.ImportPath, output.WorkingDirectory, scope)
			if !ok {
				continue
			}
			lineNumber, err := strconv.Atoi(match[2])
			if err != nil || lineNumber <= 0 {
				continue
			}
			// Keep the exact assertion identity for receipt/replay fencing.
			// The versioned comparison digest is diagnostic-only.
			digest := sha256.Sum256([]byte(strings.TrimSpace(match[3])))
			detail := runtimecontract.RuntimeValidationCheckFailure{CheckRef: check, CaseRef: record.Test,
				PackageRef: record.Package, TargetRef: target, Line: lineNumber, DiagnosticHash: hex.EncodeToString(digest[:]),
				ComparisonHash: shellAssertionComparisonHash(match[3], request.ValidationScratchDirectory)}
			if output.goTestJSONFacts.CaseOutcomes[record.Package+"\x00"+record.Test] == "fail" {
				detail.ReportedAssertion = shellReportedAssertion(match[3])
			}
			probe := runtimecontract.RuntimeValidationObservation{FailedChecks: []string{check}, CheckFailures: []runtimecontract.RuntimeValidationCheckFailure{detail},
				FailureDetailProtocolRef: observation.FailureDetailProtocolRef}
			if probe.ValidateFailureDetails() != nil {
				continue
			}
			if _, duplicate := seen[detail]; duplicate {
				continue
			}
			seen[detail] = struct{}{}
			if len(observation.CheckFailures) == runtimecontract.RuntimeValidationFailureDetailLimit {
				observation.FailureDetailsTruncated = true
				continue
			}
			observation.CheckFailures = append(observation.CheckFailures, detail)
		}
	}
	observation = observation.Normalized()
	output.ValidationObservation = &observation
}
