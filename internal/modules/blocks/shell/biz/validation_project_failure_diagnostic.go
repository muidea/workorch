package shell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
)

const shellProjectFailureDiagnosticRequirements = "v7 only: after a complete unchanged root test/race failure exclusively in exact reviewed native cases, execute one independent instrumented exact-package diagnostic under the original deadline. Preserve original failed stages and raw streams. Require identical failed cases and assertions, complete output, observed implementation, unchanged sources and cleanup. Diagnostic never completes the four-stage conjunction or grants credit. Unknown, neighboring, skipped, ambiguous, missing, stale or over-budget facts remain closed."
const shellProjectFailureDiagnosticStageID = "reviewed-cwi-failure-diagnostic"

// This gate consumes actual package-qualified events, never command labels or
// a guessed implementation target. In particular a known failure does not hide
// a skipped/unfinished/unknown sibling or a package build failure.
func shellProjectExactFailedCases(stage shellProjectStageFact, request RuntimeCommandRequest) ([]string, bool) {
	if !stage.Started || stage.ExitCode == 0 || !stage.KnownFailure || stage.TimedOut || stage.CleanupFailed || !stage.TerminalComplete || !shellProjectRawFactValid(stage.Stdout) || !shellProjectRawFactValid(stage.Stderr) || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter {
		return nil, false
	}
	pkg, err := shellIsolatedValidationPackage(request.ValidationCheckAttestations)
	if err != nil || pkg == "" {
		return nil, false
	}
	allowed := map[string]bool{}
	for _, mapping := range request.ValidationCheckAttestations {
		for _, ref := range mapping.NativeCheckRefs() {
			allowed[ref] = true
		}
	}
	var failed []string
	failedPackages, casePackages := map[string]bool{}, map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(stage.Stdout.Raw))
	scanner.Buffer(make([]byte, 4096), MaxCommandOutputBytes+1)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event shellGoTestJSONEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Action == "build-fail" || event.Action == "build-output" || event.Action == "skip" && event.Test != "" {
			return nil, false
		}
		if event.Action != "fail" {
			continue
		}
		if !shellPackageIdentityMatchesDirectory([]string{event.Package}, pkg) {
			return nil, false
		}
		if event.Test == "" {
			failedPackages[event.Package] = true
			continue
		}
		root, _, _ := strings.Cut(event.Test, "/")
		if !allowed[root] {
			return nil, false
		}
		failed = append(failed, event.Package+"\x00"+event.Test)
		casePackages[event.Package] = true
	}
	if scanner.Err() != nil || len(failed) == 0 || len(failedPackages) != 1 || len(casePackages) != 1 {
		return nil, false
	}
	for pkg := range failedPackages {
		if !casePackages[pkg] {
			return nil, false
		}
	}
	slices.Sort(failed)
	if len(slices.Compact(slices.Clone(failed))) != len(failed) {
		return nil, false
	}
	return failed, true
}

func runShellProjectFailureDiagnostic(ctx context.Context, directory string, env []string, request RuntimeCommandRequest, original shellProjectStageFact, spec shellProjectStageSpec, coveragePath, processPath string) (shellProjectStageFact, *RuntimeCommandResult) {
	want, eligible := shellProjectExactFailedCases(original, request)
	if !eligible || ctx.Err() != nil {
		return shellProjectStageFact{}, nil
	}
	spec.ID = shellProjectFailureDiagnosticStageID
	stage, _ := runShellProjectStage(ctx, directory, env, spec)
	got, complete := shellProjectExactFailedCases(stage, request)
	if !complete || !slices.Equal(want, got) || stage.SourceBefore != original.SourceAfter || ctx.Err() != nil {
		return stage, nil
	}
	output := shellBaseOutput(request, "go", spec.Args, directory, "", "go")
	output.Status, output.ExitCode = "failed", int64(stage.ExitCode)
	output.Stdout, output.Stderr = string(stage.Stdout.Raw), string(stage.Stderr.Raw)
	attachShellValidationObservation(&output, "go", spec.Args, request.ValidationRecipeID, true)
	attachShellValidationCheckFailures(&output, request)
	output.workspaceProcessBehaviorTargetRefs, output.workspaceProcessBehaviorError = shellProcessBehaviorTargets(ctx, "go", directory, processPath, env, request.ValidationScopeTargetRefs)
	_ = attachShellWorkspaceBehavior(&output, request, directory, coveragePath)
	if output.ValidationObservation == nil || output.ValidationObservation.ValidateFailureDetails() != nil || len(output.ValidationObservation.CheckFailures) == 0 || output.ValidationObservation.FailureDetailsTruncated || len(output.workspaceBehaviorTargetRefs) == 0 || ctx.Err() != nil {
		return stage, nil
	}
	// Independently compare every original assertion fact, not just Test names.
	baseline := shellBaseOutput(request, "go", spec.Args, directory, "", "go")
	baseline.ExitCode, baseline.Stdout, baseline.Stderr = int64(original.ExitCode), string(original.Stdout.Raw), string(original.Stderr.Raw)
	attachShellValidationObservation(&baseline, "go", spec.Args, request.ValidationRecipeID, true)
	attachShellValidationCheckFailures(&baseline, request)
	if baseline.ValidationObservation == nil || !slices.Equal(baseline.ValidationObservation.CheckFailures, output.ValidationObservation.CheckFailures) {
		return stage, nil
	}
	after, err := shellProjectSourceSetHash(directory)
	if err != nil || after != original.SourceAfter {
		return stage, nil
	}
	return stage, &output
}
