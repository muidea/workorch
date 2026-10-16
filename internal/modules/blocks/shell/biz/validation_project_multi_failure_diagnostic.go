package shell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellProjectDiagnosticInputCurrent(directory string, request RuntimeCommandRequest) bool {
	binding := request.validationProofBinding
	if binding.InputReview == nil || binding.InputReview.DiagnosticSourceContractRef != runtimecontract.RuntimeValidationDiagnosticSourceReviewV1 || binding.Validate() != nil || !slices.Equal(binding.CheckAttestations, request.ValidationCheckAttestations) {
		return false
	}
	current, code := shellVersionedInputReview(directory, request.ValidationScopeTargetRefs, binding)
	if code != "" {
		return false
	}
	current.LineageAssertions = slices.Clone(binding.InputReview.LineageAssertions)
	current.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*current, binding)
	return reflect.DeepEqual(current, binding.InputReview)
}

// Every failing case must resolve uniquely against the previously sealed owner
// inventory. Package failures must be exhausted by real test-case failures.
func shellProjectMultiFailedCases(stage shellProjectStageFact, sources []runtimecontract.RuntimeValidationDiagnosticSource) (map[string][]string, bool) {
	if !stage.Started || stage.ExitCode == 0 || !stage.KnownFailure || stage.TimedOut || stage.CleanupFailed || !stage.TerminalComplete || !shellProjectRawFactValid(stage.Stdout) || !shellProjectRawFactValid(stage.Stderr) || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter {
		return nil, false
	}
	failed := map[string][]string{}
	failedPackages, casePackages := map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
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
		if event.Test == "" {
			failedPackages[event.Package] = true
			continue
		}
		root, _, _ := strings.Cut(event.Test, "/")
		native := event.Package + "/" + root
		sourceRef := ""
		for _, source := range sources {
			if source.NativeCaseRef == native {
				if sourceRef != "" {
					return nil, false
				}
				sourceRef = source.SourceRef
			}
		}
		key := event.Package + "\x00" + event.Test
		if sourceRef == "" || seen[key] {
			return nil, false
		}
		failed[sourceRef] = append(failed[sourceRef], key)
		seen[key], casePackages[event.Package] = true, true
	}
	if scanner.Err() != nil || len(failed) == 0 || len(failedPackages) != len(casePackages) {
		return nil, false
	}
	for pkg := range failedPackages {
		if !casePackages[pkg] {
			return nil, false
		}
	}
	for ref := range failed {
		slices.Sort(failed[ref])
	}
	return failed, true
}

func shellProjectFailureDetailsComplete(cases []string, details []runtimecontract.RuntimeValidationCheckFailure) bool {
	for _, key := range cases {
		pkg, cas, _ := strings.Cut(key, "\x00")
		found := false
		for _, detail := range details {
			if detail.PackageRef == pkg && (detail.CaseRef == cas || strings.HasPrefix(detail.CaseRef, cas+"/")) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func runShellProjectMultiFailureDiagnostic(ctx context.Context, directory string, env []string, request RuntimeCommandRequest, original shellProjectStageFact, prefixCount int) ([]shellProjectStageFact, []shellProjectStageSpec, *RuntimeCommandResult) {
	if !shellProjectDiagnosticInputCurrent(directory, request) || ctx.Err() != nil {
		return nil, nil, nil
	}
	sources := request.validationProofBinding.InputReview.DiagnosticSources
	want, eligible := shellProjectMultiFailedCases(original, sources)
	if !eligible || len(want) > 5 || prefixCount+len(want) > runtimecontract.RuntimeValidationExecutionMaxStages {
		return nil, nil, nil
	}
	baseline := shellBaseOutput(request, "go", []string{"test", "-json", "-count=1", "./..."}, directory, "", "go")
	baseline.ExitCode, baseline.Stdout, baseline.Stderr = int64(original.ExitCode), string(original.Stdout.Raw), string(original.Stderr.Raw)
	attachShellValidationObservation(&baseline, "go", baseline.Args, request.ValidationRecipeID, true)
	attachShellValidationCheckFailures(&baseline, request)
	if baseline.ValidationObservation == nil || baseline.ValidationObservation.ValidateFailureDetails() != nil || baseline.ValidationObservation.FailureDetailsTruncated {
		return nil, nil, nil
	}
	// The parser's legacy JSON protocol is only an owner-local decode result.
	// The version selected by the sealed project binding must survive receipt
	// projection; otherwise the original stages/raw references would be lost.
	baseline.ValidationObservation.ObservationProtocolRef = shellProjectMultiFailureDiagnosticProtocolRef
	coveragePackages, err := shellIsolatedCoveragePackages(directory, request.ValidationScopeTargetRefs, "")
	if err != nil {
		return nil, nil, nil
	}
	var stages []shellProjectStageFact
	var specs []shellProjectStageSpec
	result := baseline
	for _, source := range sources {
		cases, failed := want[source.SourceRef]
		if !failed {
			continue
		}
		if !shellProjectFailureDetailsComplete(cases, baseline.ValidationObservation.CheckFailures) || ctx.Err() != nil {
			return stages, specs, nil
		}
		file, err := os.CreateTemp(request.ValidationScratchDirectory, "project-source-*.cover")
		if err != nil {
			return stages, specs, nil
		}
		coveragePath := file.Name()
		if file.Close() != nil {
			return stages, specs, nil
		}
		processPath, err := prepareShellProcessBehavior(request, directory)
		if err != nil {
			return stages, specs, nil
		}
		native := path.Base(source.NativeCaseRef)
		pkg := path.Dir(source.InputTargetRef)
		selector := "./" + pkg
		if pkg == "." {
			selector = "."
		}
		args := []string{"test", "-coverpkg=" + coveragePackages, "-covermode=atomic", "-coverprofile=" + coveragePath, "-json", "-count=1", "-run", "^" + regexp.QuoteMeta(native) + "$", selector}
		if original.ID == "project-race" {
			args = append(args[:1], append([]string{"-race"}, args[1:]...)...)
		}
		args = shellProcessBehaviorArgs(args, processPath)
		spec := shellProjectStageSpec{ID: "project-diagnostic-" + strings.TrimPrefix(source.SourceRef, "shell-project-source-"), Scope: "diagnostic-source:" + source.SourceRef, Args: args, JSON: true}
		specs = append(specs, spec)
		stageEnv := shellProcessBehaviorEnvironment(shellBehaviorEnvironment(env), processPath, args)
		stage, _ := runShellProjectStage(ctx, directory, stageEnv, spec)
		stages = append(stages, stage)
		got, complete := shellProjectMultiFailedCases(stage, sources)
		if !complete || len(got) != 1 || !slices.Equal(got[source.SourceRef], cases) || stage.SourceBefore != original.SourceAfter || ctx.Err() != nil {
			return stages, specs, nil
		}
		output := shellBaseOutput(request, "go", args, directory, "", "go")
		output.Status, output.ExitCode = "failed", int64(stage.ExitCode)
		output.Stdout, output.Stderr = string(stage.Stdout.Raw), string(stage.Stderr.Raw)
		attachShellValidationObservation(&output, "go", args, request.ValidationRecipeID, true)
		attachShellValidationCheckFailures(&output, request)
		var expected []runtimecontract.RuntimeValidationCheckFailure
		pkgRef := strings.TrimSuffix(source.NativeCaseRef, "/"+native)
		for _, detail := range baseline.ValidationObservation.CheckFailures {
			if detail.PackageRef == pkgRef && detail.CheckRef == native {
				expected = append(expected, detail)
			}
		}
		if output.ValidationObservation == nil || output.ValidationObservation.ValidateFailureDetails() != nil || output.ValidationObservation.FailureDetailsTruncated || !slices.Equal(expected, output.ValidationObservation.CheckFailures) || !shellProjectFailureDetailsComplete(cases, output.ValidationObservation.CheckFailures) {
			return stages, specs, nil
		}
		output.workspaceProcessBehaviorTargetRefs, output.workspaceProcessBehaviorError = shellProcessBehaviorTargets(ctx, "go", directory, processPath, stageEnv, request.ValidationScopeTargetRefs)
		_ = attachShellWorkspaceBehavior(&output, request, directory, coveragePath)
		after, sourceErr := shellProjectSourceSetHash(directory)
		if sourceErr != nil || after != original.SourceAfter || len(output.workspaceBehaviorTargetRefs) == 0 || output.workspaceProcessBehaviorError != nil || ctx.Err() != nil {
			return stages, specs, nil
		}
		stages[len(stages)-1].DiagnosticSourceRef = source.SourceRef
		stages[len(stages)-1].BehaviorTargetRefs = slices.Clone(output.workspaceBehaviorTargetRefs)
		result.workspaceBehaviorTargetRefs = append(result.workspaceBehaviorTargetRefs, output.workspaceBehaviorTargetRefs...)
	}
	if len(stages) != len(want) || !shellProjectDiagnosticInputCurrent(directory, request) || ctx.Err() != nil {
		return stages, specs, nil
	}
	result.Status, result.ExitCode = "failed", int64(original.ExitCode)
	result.ValidationObservation = baseline.ValidationObservation
	result.workspaceBehaviorTargetRefs = canonicalShellBehaviorTargets(result.workspaceBehaviorTargetRefs)
	if len(result.workspaceBehaviorTargetRefs) > runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs {
		return stages, specs, nil
	}
	return stages, specs, &result
}
