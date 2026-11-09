package shell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"

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
	// The diagnostic is inside the already sealed transaction. Check current
	// source bytes/membership against its exact declared method, rather than
	// silently swapping a newer built-in review declaration mid-execution.
	current.ScopeContractRef, current.ScopeRequirements = binding.InputReview.ScopeContractRef, binding.InputReview.ScopeRequirements
	current.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*current, binding)
	return reflect.DeepEqual(current, binding.InputReview)
}

// Every failing case must resolve uniquely against the previously sealed owner
// inventory. Package failures must be exhausted by real test-case failures.
func shellProjectMultiFailedCases(stage shellProjectStageFact, sources []runtimecontract.RuntimeValidationDiagnosticSource) (map[string][]string, bool) {
	if stage.NativeSourceFailure != "" || !stage.Started || stage.ExitCode == 0 || !stage.KnownFailure || stage.TimedOut || stage.CleanupFailed || !stage.TerminalComplete || !shellProjectRawFactValid(stage.Stdout) || !shellProjectRawFactValid(stage.Stderr) || stage.SourceBefore == "" || stage.SourceBefore != stage.SourceAfter {
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
	if original.NativeSourceFailure != "" {
		kind := runtimecontract.ValidationDiagnosticSourcesUnavailable
		if original.NativeSourceFailure == "validation_native_source_raw_capacity_exceeded" {
			kind = runtimecontract.ValidationDiagnosticCapacityExceeded
		}
		return reject(kind, original.NativeSourceFailure, "owner-controlled native source acquisition failed; original known failure and raw output are preserved")
	}
	want, eligible := shellProjectMultiFailedCases(original, sources)
	if !eligible {
		return reject(runtimecontract.ValidationDiagnosticSourcesUnavailable, "validation_failure_sources_unresolved", "complete failed native cases do not resolve uniquely against the sealed source inventory")
	}
	if len(want) > 5 || prefixCount+len(want) > runtimecontract.RuntimeValidationExecutionMaxStages {
		return reject(runtimecontract.ValidationDiagnosticCapacityExceeded, "validation_failure_sources_over_capacity", "failed source union exceeds the bounded independent diagnostic execution capacity")
	}
	if issue := baseline.validationFailureDetailIssue; issue != nil {
		return reject(issue.unavailability(), issue.code, issue.reason)
	}
	if baseline.ValidationObservation == nil || baseline.ValidationObservation.ValidateFailureDetails() != nil || baseline.ValidationObservation.FailureDetailsTruncated {
		return reject(runtimecontract.ValidationDiagnosticAssertionsUnavailable, "validation_failure_assertions_unavailable", "native source-bound assertion bodies are missing, invalid, ambiguous or over capacity")
	}
	coveragePackages, err := shellIsolatedCoveragePackages(directory, request.ValidationScopeTargetRefs, "")
	if err != nil {
		return reject(runtimecontract.ValidationDiagnosticSourcesUnavailable, "validation_failure_scope_unavailable", "independent coverage scope could not be compiled")
	}
	type job struct {
		source runtimecontract.RuntimeValidationDiagnosticSource
		cases  []string
		result shellSourceDiagnosticResult
	}
	var jobs []job
	for _, source := range sources {
		cases, failed := want[source.SourceRef]
		if !failed {
			continue
		}
		if !shellProjectFailureDetailsComplete(cases, baseline.ValidationObservation.CheckFailures) || ctx.Err() != nil {
			return reject(runtimecontract.ValidationDiagnosticAssertionsUnavailable, "validation_failure_assertions_unavailable", "failed native cases lack complete exact source-bound assertions")
		}
		jobs = append(jobs, job{source: source, cases: cases})
	}
	// At most five independent sources. Each owns a complete bounded workspace,
	// environment, coverage and process lifetime. Join every job before receipt
	// publication; catalog order, not completion order, determines the result.
	var joined sync.WaitGroup
	for index := range jobs {
		joined.Add(1)
		go func(index int) {
			defer joined.Done()
			j := &jobs[index]
			j.result = runShellProjectFailureSourceIsolated(ctx, directory, env, request, original, baseline, j.source, j.cases, coveragePackages)
		}(index)
	}
	joined.Wait()
	result := baseline
	var first *shellSourceDiagnosticResult
	for index := range jobs {
		job := &jobs[index].result
		if job.stage != nil {
			stages = append(stages, *job.stage)
			specs = append(specs, *job.spec)
		}
		if job.code != "" && first == nil {
			first = job
		}
		result.workspaceBehaviorTargetRefs = append(result.workspaceBehaviorTargetRefs, job.targets...)
	}
	if first != nil {
		return reject(first.unavailability, first.code, first.reason)
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

// A source result is private to this operation; it is neither a scheduler nor
// another authority/receipt. Only a complete source union can authorize repair.
type shellSourceDiagnosticResult struct {
	stage          *shellProjectStageFact
	spec           *shellProjectStageSpec
	targets        []string
	unavailability runtimecontract.RuntimeValidationDiagnosticUnavailability
	code, reason   string
}

func runShellProjectFailureSourceIsolated(ctx context.Context, directory string, env []string, request RuntimeCommandRequest, original shellProjectStageFact, baseline RuntimeCommandResult, source runtimecontract.RuntimeValidationDiagnosticSource, cases []string, coveragePackages string) (result shellSourceDiagnosticResult) {
	fail := func(kind runtimecontract.RuntimeValidationDiagnosticUnavailability, code, reason string) shellSourceDiagnosticResult {
		result.unavailability, result.code, result.reason = kind, code, reason
		result.targets = nil
		return result
	}
	scratch, err := os.MkdirTemp(request.ValidationScratchDirectory, "diagnostic-source-")
	if err != nil {
		return fail(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent diagnostic copy could not be prepared")
	}
	defer func() {
		if err := os.RemoveAll(scratch); err != nil {
			result.unavailability = runtimecontract.ValidationDiagnosticExecutionUnavailable
			result.code = "validation_failure_cleanup_unavailable"
			result.reason = "independent diagnostic copy did not complete cleanup"
			result.targets = nil
			if result.stage != nil {
				result.stage.CleanupFailed = true
				result.stage.DiagnosticSourceRef = ""
				result.stage.BehaviorTargetRefs = nil
			}
		}
	}()
	for _, name := range []string{"workspace", "home", "tmp", "cache"} {
		if err := os.Mkdir(filepath.Join(scratch, name), 0700); err != nil {
			return fail(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent diagnostic directories could not be prepared")
		}
	}
	copied := filepath.Join(scratch, "workspace")
	if _, err := copyManagedValidationWorkspace(directory, copied); err != nil {
		return fail(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "complete independent workspace copy unavailable")
	}
	current, err := shellProjectSourceSetHash(copied)
	if err != nil || current != original.SourceAfter || !shellProjectDiagnosticInputCurrent(copied, request) || ctx.Err() != nil {
		return fail(runtimecontract.ValidationDiagnosticInputChanged, "validation_failure_input_changed", "independent diagnostic copy differs from the exact original source")
	}
	directory = copied
	request.ValidationScratchDirectory = scratch
	env = managedValidationEnvironment(env, scratch)
	sources := request.validationProofBinding.InputReview.DiagnosticSources
	file, err := os.CreateTemp(request.ValidationScratchDirectory, "project-source-*.cover")
	if err != nil {
		return fail(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent diagnostic scratch could not be created")
	}
	coveragePath := file.Name()
	if file.Close() != nil {
		return fail(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent diagnostic scratch could not be closed")
	}
	processPath, err := prepareShellProcessBehavior(request, directory)
	if err != nil {
		return fail(runtimecontract.ValidationDiagnosticExecutionUnavailable, "validation_failure_execution_unavailable", "independent process observation could not be prepared")
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
	if shellNativeChannelMethodRequired(request.validationProofBinding.InputReview.Files) {
		spec.NativeSources = []runtimecontract.RuntimeValidationDiagnosticSource{source}
	}
	result.spec = &spec
	stageEnv := shellProcessBehaviorEnvironment(shellBehaviorEnvironment(env), processPath, args)
	stage, _ := runShellProjectStageWithBudget(ctx, directory, stageEnv, spec, request.projectRawBudget)
	if stage.Started {
		stage.DiagnosticOriginStageID = original.ID
		stage.DiagnosticIsolationRef = filepath.Base(request.ValidationScratchDirectory)
	}
	result.stage = &stage
	got, complete := shellProjectMultiFailedCases(stage, sources)
	if !complete || len(got) != 1 || !slices.Equal(got[source.SourceRef], cases) || stage.SourceBefore != original.SourceAfter || ctx.Err() != nil {
		return fail(runtimecontract.ValidationDiagnosticReproductionUnavailable, "validation_failure_reproduction_unavailable", "independent execution did not reproduce the exact original failed cases on unchanged sources")
	}
	output := shellBaseOutput(request, "go", args, directory, "", "go")
	output.Status, output.ExitCode = "failed", int64(stage.ExitCode)
	output.Stdout, output.Stderr = string(stage.Stdout.Raw), string(stage.Stderr.Raw)
	attachShellValidationObservation(&output, "go", args, request.ValidationRecipeID, true)
	attachShellNativeExecutionFacts(&output, stage)
	attachShellValidationCheckFailures(&output, request)
	if issue := output.validationFailureDetailIssue; issue != nil {
		return fail(issue.unavailability(), issue.code, "independent source "+source.SourceRef+": "+issue.reason)
	}
	var expected []runtimecontract.RuntimeValidationCheckFailure
	pkgRef := strings.TrimSuffix(source.NativeCaseRef, "/"+native)
	for _, detail := range baseline.ValidationObservation.CheckFailures {
		if detail.PackageRef == pkgRef && detail.CheckRef == native {
			expected = append(expected, detail)
		}
	}
	if output.ValidationObservation == nil || output.ValidationObservation.ValidateFailureDetails() != nil || output.ValidationObservation.FailureDetailsTruncated || !shellSameFailureAssertions(expected, output.ValidationObservation.CheckFailures) || !shellProjectFailureDetailsComplete(cases, output.ValidationObservation.CheckFailures) {
		return fail(runtimecontract.ValidationDiagnosticReproductionUnavailable, "validation_failure_assertions_changed", "independent execution did not preserve every original source-bound assertion")
	}
	output.workspaceProcessBehaviorTargetRefs, output.workspaceProcessBehaviorError = shellProcessBehaviorTargets(ctx, "go", directory, processPath, stageEnv, request.ValidationScopeTargetRefs)
	_ = attachShellWorkspaceBehavior(&output, request, directory, coveragePath)
	after, sourceErr := shellProjectSourceSetHash(directory)
	if sourceErr != nil || after != original.SourceAfter || len(output.workspaceBehaviorTargetRefs) == 0 || output.workspaceProcessBehaviorError != nil || ctx.Err() != nil {
		return fail(runtimecontract.ValidationDiagnosticBehaviorUnobserved, "validation_failure_behavior_unobserved", "independent execution lacks complete unchanged-source implementation behavior")
	}
	stage.DiagnosticSourceRef = source.SourceRef
	stage.BehaviorTargetRefs = slices.Clone(output.workspaceBehaviorTargetRefs)

	result.targets = slices.Clone(output.workspaceBehaviorTargetRefs)
	return result
}
