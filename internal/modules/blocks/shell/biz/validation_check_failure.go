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
// unrelated transcript stays private. Only source-bound assertions travel.
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
	assertionBytes := 0
	assertions, incomplete := shellSourceBoundAssertions(output.goTestJSONFacts, failed)
	observation.FailureDetailsTruncated = incomplete
	for _, assertion := range assertions {
		record := assertion.record
		check, _, _ := strings.Cut(record.Test, "/")
		target, ok := shellDiagnosticTargetInScope(assertion.target, record.Package, record.ImportPath, output.WorkingDirectory, scope)
		if !ok {
			continue
		}
		digest := sha256.Sum256([]byte(strings.TrimSpace(assertion.body)))
		detail := runtimecontract.RuntimeValidationCheckFailure{CheckRef: check, CaseRef: record.Test,
			PackageRef: record.Package, TargetRef: target, Line: assertion.line, DiagnosticHash: hex.EncodeToString(digest[:]),
			AssertionText: assertion.body, ComparisonHash: shellAssertionComparisonHash(assertion.body, request.ValidationScratchDirectory)}
		if output.goTestJSONFacts.CaseOutcomes[record.Package+"\x00"+record.Test] == "fail" {
			detail.ReportedAssertion = shellReportedAssertion(assertion.body)
		}
		probe := runtimecontract.RuntimeValidationObservation{FailedChecks: []string{check}, CheckFailures: []runtimecontract.RuntimeValidationCheckFailure{detail}, FailureDetailProtocolRef: observation.FailureDetailProtocolRef}
		if probe.ValidateFailureDetails() != nil {
			observation.FailureDetailsTruncated = true
			continue
		}
		if _, duplicate := seen[detail]; duplicate {
			continue
		}
		seen[detail] = struct{}{}
		if len(observation.CheckFailures) == runtimecontract.RuntimeValidationFailureDetailLimit || assertionBytes+len(detail.AssertionText) > runtimecontract.RuntimeValidationAssertionsMaxBytes {
			observation.FailureDetailsTruncated = true
			continue
		}
		assertionBytes += len(detail.AssertionText)
		observation.CheckFailures = append(observation.CheckFailures, detail)
	}
	observation = observation.Normalized()
	output.ValidationObservation = &observation
}

// Go emits a decorated source line followed by more-indented continuation
// lines, sometimes split across JSON events. Join only that native case/source
// frame. Unknown continuation shapes are explicit gaps, never complete prefixes.
type shellSourceAssertion struct {
	record shellGoTestJSONDiagnosticRecord
	target string
	line   int
	indent int
	body   string
}

func shellSourceBoundAssertions(facts *shellGoTestJSONFacts, failed map[string]struct{}) ([]shellSourceAssertion, bool) {
	var out []shellSourceAssertion
	pending := map[string]int{}
	incomplete := false
	consume := func(record shellGoTestJSONDiagnosticRecord, line string) {
		key := record.Package + "\x00" + record.Test
		match := shellGoAssertionLocationPattern.FindStringSubmatch(line)
		if len(match) == 4 {
			n, err := strconv.Atoi(match[2])
			if err != nil || n <= 0 {
				incomplete = true
				delete(pending, key)
				return
			}
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			out = append(out, shellSourceAssertion{record: record, target: match[1], line: n, indent: indent, body: match[3]})
			pending[key] = len(out) - 1
			return
		}
		idx, ok := pending[key]
		if !ok {
			return
		}
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--- FAIL:") || strings.HasPrefix(trim, "--- PASS:") || strings.HasPrefix(trim, "=== ") {
			delete(pending, key)
			return
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if trim == "" || indent > out[idx].indent {
			if len(out[idx].body)+1+len(line) > runtimecontract.RuntimeValidationAssertionMaxBytes {
				incomplete = true
				delete(pending, key)
			} else {
				out[idx].body += "\n" + line
			}
		} else {
			incomplete = true
			delete(pending, key)
		}
	}
	// test2json may split a single native line across several Output events.
	// Reassemble its bytes before interpreting source frames or continuations.
	fragments := map[string]string{}
	records := map[string]shellGoTestJSONDiagnosticRecord{}
	order := []string{}
	for _, record := range facts.DiagnosticRecords {
		check, _, _ := strings.Cut(record.Test, "/")
		if _, ok := failed[check]; !ok || record.Action != "output" {
			continue
		}
		key := record.Package + "\x00" + record.Test
		if outcome := facts.CaseOutcomes[key]; outcome != "" && outcome != "fail" {
			continue
		}
		if _, ok := records[key]; !ok {
			order = append(order, key)
		}
		records[key] = record
		body := fragments[key] + record.Output
		for {
			line, rest, ok := strings.Cut(body, "\n")
			if !ok {
				break
			}
			consume(record, line)
			body = rest
		}
		fragments[key] = body
	}
	for _, key := range order {
		if fragments[key] != "" {
			consume(records[key], fragments[key])
		}
	}
	return out, incomplete
}
