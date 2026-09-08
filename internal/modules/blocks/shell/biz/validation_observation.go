package shell

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type shellGoTestJSONEvent struct {
	Action     string `json:"Action"`
	Package    string `json:"Package"`
	ImportPath string `json:"ImportPath"`
	Test       string `json:"Test"`
	Output     string `json:"Output"`
}

type shellGoTestJSONDiagnosticRecord struct {
	Action     string
	Package    string
	ImportPath string
	Test       string
	Output     string
}

// shellGoTestJSONFacts is the single owner-private decoding result for one
// bounded `go test -json` transcript. Check outcomes and compiler/package
// diagnostics must be decoded together: separate scanners previously allowed
// target extraction to recognize build-output while owner diagnostic
// classification saw only the JSON envelope and fell back to a generic exit
// code.
type shellGoTestJSONFacts struct {
	Observation       runtimecontract.RuntimeValidationObservation
	DiagnosticOutput  string
	DiagnosticRecords []shellGoTestJSONDiagnosticRecord
	HasBuildOutput    bool
	// Package-scoped native case outcomes are private parse facts, not another
	// state machine. Passing/skipped subcase output cannot diagnose a failed sibling.
	CaseOutcomes map[string]string
}

// attachShellValidationObservation records the owner fact that a sealed
// managed-validation recipe was actually run. A recipe may be a generic
// command (for example a deterministic file-presence assertion), in which
// case Shell cannot honestly manufacture named checks; the recipe identity is
// still an exact, receipt-backed validation observation. Go test recipes add
// their parsed checks to that same observation.
//
// Ordinary shell/command execution intentionally does not get this
// observation. The managed-validation bit comes from the sealed operation,
// not from a candidate summary or the command text.
func attachShellValidationObservation(output *RuntimeCommandResult, command string, args []string, recipeID string, managed bool) {
	if output == nil || !managed {
		return
	}
	isGoTest := filepath.Base(strings.TrimSpace(command)) == "go" && len(args) > 0 && strings.TrimSpace(args[0]) == "test"
	isGoTestJSON := isGoTest && shellGoTestJSONEnabled(args[1:])
	observation := shellValidationObservation(output.Stdout, output.Stderr, isGoTest && !isGoTestJSON)
	if isGoTestJSON {
		facts := parseGoTestJSONFacts(output.Stdout, output.Stderr)
		output.goTestJSONFacts = &facts
		observation = facts.Observation
	}
	observation.RecipeID = strings.TrimSpace(recipeID)
	if isGoTestJSON {
		observation.ObservationProtocolRef = shellGoTestObservationProtocolRef
	}
	rawOutput := strings.Join([]string{output.Stdout, output.Stderr}, "\n")
	if preview := strings.TrimSpace(rawOutput); preview != "" {
		observation.OutputPreview = shellPreview(common.RedactSensitiveString(preview), 2048)
	}
	observation = observation.Normalized()
	if conflicts := runtimecontract.RuntimeValidationConflictingChecks(observation); len(conflicts) != 0 {
		output.EvidenceQuality = string(common.EvidenceQualityInvalid)
		output.EvidenceReason = "validation check has conflicting pass/fail/skip outcomes"
		output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "conflicting_check_outcome")
		output.ValidationObservation = &observation
		return
	}
	if len(observation.FailedChecks) != 0 {
		output.EvidenceQuality = string(common.EvidenceQualityInvalid)
		if strings.TrimSpace(output.EvidenceReason) == "" {
			output.EvidenceReason = "validation command reported failed checks"
		}
		output.ValidationObservation = &observation
		return
	}
	if len(observation.IncompleteChecks) != 0 {
		output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
		output.EvidenceReason = "validation check contains skipped subchecks"
		output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "incomplete_check")
		output.ValidationObservation = &observation
		return
	}
	if isGoTest && output.ExitCode == 0 && goTestObservationIsExplicitZeroWork(observation, rawOutput) {
		output.ZeroWork = true
		output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
		if strings.TrimSpace(output.EvidenceReason) == "" {
			output.EvidenceReason = "validation command reported no executed checks"
		}
		output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "zero_work")
		output.ValidationObservation = &observation
		return
	}
	if goTestObservationIsAllSkipped(observation) {
		output.EvidenceQuality = string(common.EvidenceQualitySkipped)
		if strings.TrimSpace(output.EvidenceReason) == "" {
			output.EvidenceReason = "all observed validation checks were skipped"
		}
		output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "all_skipped")
		output.ValidationObservation = &observation
		return
	}
	if output.ExitCode == 0 && !output.ZeroWork && strings.TrimSpace(output.EvidenceQuality) != string(common.EvidenceQualityInconclusive) &&
		strings.TrimSpace(output.EvidenceQuality) != string(common.EvidenceQualityInvalid) && len(observation.PassedChecks) != 0 {
		output.EvidenceQuality = string(common.EvidenceQualityValid)
	}
	output.ValidationObservation = &observation
}

func shellGoTestJSONEnabled(args []string) bool {
	for _, value := range args {
		value = strings.TrimSpace(value)
		if value == "-json" || value == "--json" || value == "-json=true" || value == "--json=true" {
			return true
		}
	}
	return false
}

func goTestJSONValidationObservation(stdout, stderr string) runtimecontract.RuntimeValidationObservation {
	return parseGoTestJSONFacts(stdout, stderr).Observation
}

func parseGoTestJSONFacts(stdout, stderr string) shellGoTestJSONFacts {
	observation := runtimecontract.RuntimeValidationObservation{}
	diagnosticRecords := make([]shellGoTestJSONDiagnosticRecord, 0)
	hasBuildOutput := false
	caseOutcomes := make(map[string]string)
	var diagnosticOutput strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(strings.Join([]string{stdout, stderr}, "\n")))
	scanner.Buffer(make([]byte, 4096), 2*MaxCommandOutputBytes+1024)
	for scanner.Scan() {
		line := scanner.Text()
		var event shellGoTestJSONEvent
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &event) != nil {
			if strings.TrimSpace(line) != "" {
				diagnosticRecords = append(diagnosticRecords, shellGoTestJSONDiagnosticRecord{Output: line})
				diagnosticOutput.WriteString(line)
				diagnosticOutput.WriteByte('\n')
			}
			continue
		}
		switch strings.TrimSpace(event.Action) {
		case "output", "build-output":
			if strings.TrimSpace(event.Output) != "" {
				diagnosticRecords = append(diagnosticRecords, shellGoTestJSONDiagnosticRecord{
					Action: strings.TrimSpace(event.Action), Package: strings.TrimSpace(event.Package),
					ImportPath: strings.TrimSpace(event.ImportPath), Test: strings.TrimSpace(event.Test), Output: event.Output,
				})
				hasBuildOutput = hasBuildOutput || strings.TrimSpace(event.Action) == "build-output"
				diagnosticOutput.WriteString(event.Output)
				if !strings.HasSuffix(event.Output, "\n") {
					diagnosticOutput.WriteByte('\n')
				}
			}
		}
		name := topLevelGoTestCheck(event.Test)
		if event.Test != "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			key := strings.TrimSpace(event.Package) + "\x00" + strings.TrimSpace(event.Test)
			if previous := caseOutcomes[key]; previous != "" && previous != event.Action {
				caseOutcomes[key] = "conflicting"
			} else {
				caseOutcomes[key] = event.Action
			}
		}
		if name == "" {
			if parent, _, nested := strings.Cut(strings.TrimSpace(event.Test), "/"); nested && parent != "" && strings.TrimSpace(event.Action) == "skip" {
				observation.IncompleteChecks = append(observation.IncompleteChecks, parent)
			}
			continue
		}
		switch strings.TrimSpace(event.Action) {
		case "run":
			observation.ExecutedChecks = append(observation.ExecutedChecks, name)
		case "pass":
			observation.PassedChecks = append(observation.PassedChecks, name)
		case "fail":
			observation.FailedChecks = append(observation.FailedChecks, name)
		case "skip":
			observation.SkippedChecks = append(observation.SkippedChecks, name)
		}
	}
	return shellGoTestJSONFacts{
		Observation:       observation.Normalized(),
		DiagnosticOutput:  diagnosticOutput.String(),
		DiagnosticRecords: diagnosticRecords,
		HasBuildOutput:    hasBuildOutput,
		CaseOutcomes:      caseOutcomes,
	}
}

// goTestObservationIsExplicitZeroWork recognizes the tool-level zero-work
// signal only when the parsed observation contains no executable check. It is
// intentionally independent of workspace/profile semantics and complements
// configured evidence policies for callers that did not supply markers.
func goTestObservationIsExplicitZeroWork(observation runtimecontract.RuntimeValidationObservation, rawOutput string) bool {
	observation = observation.Normalized()
	if len(observation.ExecutedChecks) != 0 || len(observation.PassedChecks) != 0 || len(observation.FailedChecks) != 0 || len(observation.SkippedChecks) != 0 {
		return false
	}
	return strings.Contains(strings.ToLower(rawOutput), "no tests to run")
}

// goTestObservationIsAllSkipped is intentionally expressed only in terms of the
// owner-reported observation. The Shell owner may parse a tool's output, but it
// must not infer workspace/profile semantics from test names or let a successful
// process exit turn an all-skipped execution into valid evidence.
func goTestObservationIsAllSkipped(observation runtimecontract.RuntimeValidationObservation) bool {
	observation = observation.Normalized()
	if len(observation.ExecutedChecks) == 0 || len(observation.PassedChecks) != 0 || len(observation.FailedChecks) != 0 {
		return false
	}
	if len(observation.SkippedChecks) != len(observation.ExecutedChecks) {
		return false
	}
	executed := make(map[string]struct{}, len(observation.ExecutedChecks))
	for _, name := range observation.ExecutedChecks {
		executed[name] = struct{}{}
	}
	for _, name := range observation.SkippedChecks {
		if _, found := executed[name]; !found {
			return false
		}
	}
	return true
}

func appendUniqueShellEvidenceTag(values []string, tag string) []string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return values
	}
	for _, value := range values {
		if strings.TrimSpace(value) == tag {
			return values
		}
	}
	return append(values, tag)
}

func goTestValidationObservation(stdout, stderr string) runtimecontract.RuntimeValidationObservation {
	return shellValidationObservation(stdout, stderr, true)
}

func shellValidationObservation(stdout, stderr string, includeGoTestChecks bool) runtimecontract.RuntimeValidationObservation {
	observation := runtimecontract.RuntimeValidationObservation{}
	scanner := bufio.NewScanner(strings.NewReader(strings.Join([]string{stdout, stderr}, "\n")))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !includeGoTestChecks {
			continue
		}
		switch {
		case strings.HasPrefix(line, "=== RUN"):
			name := topLevelGoTestCheck(strings.TrimSpace(strings.TrimPrefix(line, "=== RUN")))
			if name != "" {
				observation.ExecutedChecks = append(observation.ExecutedChecks, name)
			}
		case strings.HasPrefix(line, "--- PASS:"):
			name := topLevelGoTestCheck(goTestResultName(strings.TrimSpace(strings.TrimPrefix(line, "--- PASS:"))))
			if name != "" {
				observation.PassedChecks = append(observation.PassedChecks, name)
			}
		case strings.HasPrefix(line, "--- FAIL:"):
			name := topLevelGoTestCheck(goTestResultName(strings.TrimSpace(strings.TrimPrefix(line, "--- FAIL:"))))
			if name != "" {
				observation.FailedChecks = append(observation.FailedChecks, name)
			}
		case strings.HasPrefix(line, "--- SKIP:"):
			name := topLevelGoTestCheck(goTestResultName(strings.TrimSpace(strings.TrimPrefix(line, "--- SKIP:"))))
			if name != "" {
				observation.SkippedChecks = append(observation.SkippedChecks, name)
			}
		}
	}
	return observation.Normalized()
}

func goTestResultName(value string) string {
	if idx := strings.Index(value, " ("); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func topLevelGoTestCheck(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "/") {
		return ""
	}
	return value
}
