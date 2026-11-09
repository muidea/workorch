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
	current, code := shellInputReview(directory, request.ValidationScopeTargetRefs, binding)
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
	scanner.Buffer(make([]byte, 4096), shellProjectRawOutputMaxBytes+1)
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

func runShellProjectMultiFailureDiagnostic(ctx context.Context, directory string, env []string, request RuntimeCommandRequest, original shellProjectStageFact, prefixCount int, baseline RuntimeCommandResult) ([]shellProjectStageFact, []shellProjectStageSpec, *RuntimeCommandResult) {
	var stages []shellProjectStageFact
	var specs []shellProjectStageSpec
	// The runner decoded this exact stage once. Preserve the failed native
	// facts independently of whether every source can acquire repair behavior.
	if baseline.ValidationObservation != nil {
		observation := baseline.ValidationObservation.Clone()
		observation.ProofClass = ""
		observation.ObservationProtocolRef = shellProjectMultiFailureDiagnosticProtocolRef
		baseline.ValidationObservation = &observation
	}
	reject := func(unavailability runtimecontract.RuntimeValidationDiagnosticUnavailability, code, reason string) ([]shellProjectStageFact, []shellProjectStageSpec, *RuntimeCommandResult) {
		if ctx.Err() == context.DeadlineExceeded {
			unavailability = runtimecontract.ValidationDiagnosticDeadlineExceeded
			code, reason = "validation_failure_deadline_exceeded", "independent diagnostic could not complete within the original operation context: "+ctx.Err().Error()
		} else if ctx.Err() != nil {
			unavailability = runtimecontract.ValidationDiagnosticExecutionUnavailable
			code, reason = "validation_failure_execution_unavailable", "independent diagnostic was cancelled: "+ctx.Err().Error()
		}
		baseline.workspaceBehaviorTargetRefs = nil
		baseline.ValidationFailureDiagnostic = &runtimecontract.ValidationFailureDiagnostic{
			SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
			Class:         runtimecontract.ValidationDiagnosticOutputInconclusive, Unavailability: unavailability,
			OwnerCode: code, Reason: reason, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetValidationOutput,
			Stage: original.ID, Operation: shellDiagnosticOperationKind(request), CommandKind: "go", ExitCode: original.ExitCode,
		}
		baseline.RuntimeDiagnostics.ValidationFailureDiagnostic = baseline.ValidationFailureDiagnostic
		return stages, specs, &baseline
	}
	if !shellProjectDiagnosticInputCurrent(directory, request) || ctx.Err() != nil {
		return reject(runtimecontract.ValidationDiagnosticInputChanged, "validation_failure_input_changed", "independent diagnostic requires the exact current sealed input subject")
	}
	sources := request.validationProofBinding.InputReview.DiagnosticSources
	want, eligible := shellProjectMultiFailedCases(original, sources)
	if !eligible {
		return reject(runtimecontract.ValidationDiagnosticSourcesUnavailable, "validation_failure_sources_unresolved", "complete failed native cases do not resolve uniquely against the sealed source inventory")
	}
	if len(want) > 5 || prefixCount+len(want) > runtimecontract.RuntimeValidationExecutionMaxStages {
		return reject(runtimecontract.ValidationDiagnosticCapacityExceeded, "validation_failure_sources_over_capacity", "failed source union exceeds the bounded independent diagnostic execution capacity")
	}
	if baseline.ValidationObservation == nil || baseline.ValidationObservation.ValidateFailureDetails() != nil || baseline.ValidationObservation.FailureDetailsTruncated {
		return reject(runtimecontract.ValidationDiagnosticAssertionsUnavailable, "validation_failure_assertions_unavailable", "native source-bound assertion bodies are missing, invalid, ambiguous or over capacity")
	}
	coveragePackages, err := shellIsolatedCoveragePackages(directory, request.ValidationScopeTargetRefs, "")
	if err != nil {
		return reject(runtimecontract.ValidationDiagnosticSourcesUnavailable, "validation_failure_scope_unavailable", "independent coverage scope could not be compiled")
	}
	result := baseline
	for _, source := range sources {
		cases, failed := want[source.SourceRef]
		if !failed {
			continue
		}
		if !shellProjectFailureDetailsComplete(cases, baseline.ValidationObservation.CheckFailures) || ctx.Err() != nil {
			return reject(runtimecontract.ValidationDiagnosticAssertionsUnavailable, "validation_failure_assertions_unavailable", "failed native cases lack complete exact source-bound assertions")
		}
		file, err := os.CreateTemp(request.ValidationScratchDirectory, "project-source-*.cover")
		if err != nil {
			return reject(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent diagnostic scratch could not be created")
		}
		coveragePath := file.Name()
		if file.Close() != nil {
			return reject(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent diagnostic scratch could not be closed")
		}
		processPath, err := prepareShellProcessBehavior(request, directory)
		if err != nil {
			return reject(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent process observation could not be prepared")
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
		stage, _ := runShellProjectStageWithBudget(ctx, directory, stageEnv, spec, request.projectRawBudget)
		stages = append(stages, stage)
		got, complete := shellProjectMultiFailedCases(stage, sources)
		if !complete || len(got) != 1 || !slices.Equal(got[source.SourceRef], cases) || stage.SourceBefore != original.SourceAfter || ctx.Err() != nil {
			return reject(runtimecontract.ValidationDiagnosticReproductionUnavailable, "validation_failure_reproduction_unavailable", "independent execution did not reproduce the exact original failed cases on unchanged sources")
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
			return reject(runtimecontract.ValidationDiagnosticReproductionUnavailable, "validation_failure_assertions_changed", "independent execution did not preserve every original source-bound assertion")
		}
		output.workspaceProcessBehaviorTargetRefs, output.workspaceProcessBehaviorError = shellProcessBehaviorTargets(ctx, "go", directory, processPath, stageEnv, request.ValidationScopeTargetRefs)
		_ = attachShellWorkspaceBehavior(&output, request, directory, coveragePath)
		after, sourceErr := shellProjectSourceSetHash(directory)
		if sourceErr != nil || after != original.SourceAfter || len(output.workspaceBehaviorTargetRefs) == 0 || output.workspaceProcessBehaviorError != nil || ctx.Err() != nil {
			return reject(runtimecontract.ValidationDiagnosticBehaviorUnobserved, "validation_failure_behavior_unobserved", "independent execution lacks complete unchanged-source implementation behavior")
		}
		stages[len(stages)-1].DiagnosticSourceRef = source.SourceRef
		stages[len(stages)-1].BehaviorTargetRefs = slices.Clone(output.workspaceBehaviorTargetRefs)
		result.workspaceBehaviorTargetRefs = append(result.workspaceBehaviorTargetRefs, output.workspaceBehaviorTargetRefs...)
	}
	if len(stages) != len(want) || !shellProjectDiagnosticInputCurrent(directory, request) || ctx.Err() != nil {
		return reject(runtimecontract.ValidationDiagnosticInputChanged, "validation_failure_input_changed", "independent source union or original input is no longer complete and current")
	}
	result.Status, result.ExitCode = "failed", int64(original.ExitCode)
	result.ValidationObservation = baseline.ValidationObservation
	result.workspaceBehaviorTargetRefs = canonicalShellBehaviorTargets(result.workspaceBehaviorTargetRefs)
	// Preserve the complete independently observed union. The common diagnostic
	// candidate boundary reports overflow explicitly; dropping it here would
	// hide a known capacity failure as missing attribution.
	return stages, specs, &result
}
