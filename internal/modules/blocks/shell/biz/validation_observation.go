package shell

import (
	"bufio"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func attachShellValidationObservation(output *RuntimeCommandResult, command string, args []string) {
	if output == nil || filepath.Base(strings.TrimSpace(command)) != "go" || len(args) == 0 || strings.TrimSpace(args[0]) != "test" {
		return
	}
	observation := goTestValidationObservation(output.Stdout, output.Stderr)
	if preview := strings.TrimSpace(strings.Join([]string{output.Stdout, output.Stderr}, "\n")); preview != "" {
		observation.OutputPreview = shellPreview(common.RedactSensitiveString(preview), 2048)
	}
	observation = observation.Normalized()
	output.ValidationObservation = &observation
	if output.ZeroWork || strings.TrimSpace(output.EvidenceQuality) == string(common.EvidenceQualityInconclusive) {
		return
	}
	if goTestObservationIsAllSkipped(observation) {
		output.EvidenceQuality = string(common.EvidenceQualitySkipped)
		if strings.TrimSpace(output.EvidenceReason) == "" {
			output.EvidenceReason = "all observed validation checks were skipped"
		}
		output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "all_skipped")
		return
	}
	if output.ExitCode == 0 {
		output.EvidenceQuality = string(common.EvidenceQualityValid)
	}
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
	observation := runtimecontract.RuntimeValidationObservation{}
	scanner := bufio.NewScanner(strings.NewReader(strings.Join([]string{stdout, stderr}, "\n")))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
