package orchestrator

import (
	"context"
	"fmt"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (r *repairSubsystem) providerRetryAttemptCount(ctx context.Context, runID common.RunID, rootStepID common.StepID) int {
	if r == nil || r.steps == nil || runID == "" || rootStepID == "" {
		return 0
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if isFailedProviderRetryAttempt(rootStepID, step) {
			count++
		}
	}
	return count
}

func (r *repairSubsystem) effectiveMaxProviderRetryAttempts(ctx context.Context, policy RuntimePolicy, run *runModel.Run, failedStep *runModel.Step) int {
	maxAttempts := maxProviderRetryAttemptsForPolicy(policy)
	if maxAttempts <= maxProviderRetryAttemptsBeforeStateChange {
		return maxAttempts
	}
	if run == nil || failedStep == nil || !runExecutionContractRequiresStateChange(run) {
		return maxAttempts
	}
	if r.hasStateChangingAttempt(ctx, run.ID) {
		return maxAttempts
	}
	return maxProviderRetryAttemptsBeforeStateChange
}

func (r *repairSubsystem) hasStateChangingAttempt(ctx context.Context, runID common.RunID) bool {
	if r == nil || r.steps == nil || runID == "" {
		return false
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		if stepHasStateChangingAttempt(step) {
			return true
		}
	}
	return false
}

func stepHasStateChangingAttempt(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if stepKindCanProduceStateChangingEvidence(step.Kind) {
		return true
	}
	if hasStateChangingLLMExecutionOutput(step.Output) {
		return true
	}
	if graph, ok := runtimeGraphSpecFromStep(step); ok {
		return graphHasStateChangingPatchNode(graph) || graphHasDeclaredUngatedStateChangingToolNode(graph)
	}
	return false
}

func isFailedProviderRetryAttempt(rootStepID common.StepID, step *runModel.Step) bool {
	if step == nil || rootStepID == "" || !boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) || providerRetryRootStepID(step) != rootStepID {
		return false
	}
	return step.State == common.StepStateFailed && isLLMProviderFailureStage(stringFromMap(step.Output, "stage"))
}

func providerRetryStepInput(failedStep *runModel.Step, attempt int, execErr error) map[string]any {
	return providerRetryStepInputWithMaxAttempts(failedStep, attempt, execErr, maxProviderRetryAttempts)
}

func providerRetryStepInputWithMaxAttempts(failedStep *runModel.Step, attempt int, execErr error, maxAttempts int) map[string]any {
	if maxAttempts <= 0 {
		maxAttempts = maxProviderRetryAttempts
	}
	input := cloneStepPayload(failedStep.Input)
	if input == nil {
		input = map[string]any{}
	}
	sanitizeProviderRetryBaseInput(input)
	delete(input, stepInputKeyRuntimeRepairFollowUp)
	delete(input, stepInputKeyRuntimeRepairAttempt)
	rootStepID := providerRetryRootStepID(failedStep)
	input[stepInputKeyProviderRetryFollowUp] = true
	input[stepInputKeyProviderRetryAttempt] = attempt
	input[stepInputKeyProviderRetryRootStep] = string(rootStepID)
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRecoveryAction] = string(runtimeRecoveryActionProviderRetry)
	input[stepInputKeyRecoveryRootStepID] = string(rootStepID)
	input[stepInputKeyRecoveryLineageType] = "provider_retry"
	input[stepInputKeyRecoveryBudgetScope] = recoveryBudgetScope("provider_retry", rootStepID, stringFromMap(failedStep.Output, failureOutputKeyClass), stringFromMap(failedStep.Output, failureOutputKeySignature))
	input["maxProviderRetryAttempts"] = maxAttempts
	input["failedStepId"] = string(rootStepID)
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	input["failedStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	input["failureReason"] = errorString(execErr)
	input[failureOutputKeyClass] = stringFromMap(failedStep.Output, failureOutputKeyClass)
	input[failureOutputKeySignature] = stringFromMap(failedStep.Output, failureOutputKeySignature)
	if stateChangingRepairRequired(nil, failedStep, execErr) || repairObjectiveRequiresStateChange(stringFromMap(input, "repairObjective")) {
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		if _, ok := input["repairGuidance"]; !ok {
			input["repairGuidance"] = stateChangingRepairGuidance()
		}
		narrowStateChangingRecoveryInput(input, failedStep, execErr, "provider_retry")
	}
	if completionGateFollowUpStep(failedStep) {
		input["providerRetryMode"] = "completion_assessment_only"
		input["providerRetryAssessmentOnly"] = true
		input["completionFollowUp"] = true
		setRuntimeStage(input, runtimeStageVerification)
		delete(input, stepOutputKeyExecutionGraph)
		delete(input, "previousStepOutput")
	}
	return input
}

func sanitizeProviderRetryBaseInput(input map[string]any) {
	if input == nil {
		return
	}
	for _, key := range []string{
		"failedStepInput",
		"failedStepOutput",
		"llmProviderId",
		"previousStepOutput",
		stepInputKeyExecutionContextPackage,
		stepInputKeyWorkspaceFileManifest,
		stepOutputKeyExecutionGraph,
	} {
		delete(input, key)
	}
	if executionContext := slimExecutionContextForStepInput(mapFromValue(input["executionContext"])); len(executionContext) > 0 {
		input["executionContext"] = executionContext
	} else {
		delete(input, "executionContext")
	}
}

func completionGateFollowUpStep(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return boolValue(step.Input["graphFollowUp"]) ||
		boolValue(step.Input["completionFollowUp"]) ||
		boolValue(step.Input[stepInputKeyNoActionableFollowUp])
}

func providerRetryStepID(runID common.RunID, rootStepID common.StepID, attempt int) common.StepID {
	if attempt < 1 {
		attempt = 1
	}
	rootLabel := runtimeReadableStepLabel(runID, rootStepID)
	if rootLabel == "" {
		rootLabel = "root"
	}
	return common.StepID(fmt.Sprintf("%s/provider-retry-%03d-of-%s", runID, attempt, rootLabel))
}

func providerRetryRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyProviderRetryRootStep))); root != "" {
		return normalizeRecoveryRootStepIDFromInput(step.Input, root)
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, "failedStepId"))); root != "" {
			return normalizeRecoveryRootStepIDFromInput(step.Input, root)
		}
	}
	return normalizeRecoveryRootStepIDFromInput(step.Input, step.ID)
}

func normalizeRecoveryRootStepIDFromInput(input map[string]any, root common.StepID) common.StepID {
	if root == "" {
		return ""
	}
	rootText := string(root)
	if !recoveryRootNeedsNormalization(rootText) {
		return root
	}
	if repairRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyRepairOfRootStepID))); repairRoot != "" && !recoveryRootNeedsNormalization(string(repairRoot)) {
		return repairRoot
	}
	if recoveryRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyRecoveryRootStepID))); recoveryRoot != "" && recoveryRoot != root {
		return normalizeRecoveryRootStepIDFromInput(input, recoveryRoot)
	}
	if providerRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyProviderRetryRootStep))); providerRoot != "" && providerRoot != root {
		return normalizeRecoveryRootStepIDFromInput(input, providerRoot)
	}
	if strings.Contains(rootText, "/provider-retry-") || strings.Contains(rootText, "of-provider-retry-") {
		if repairRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyRepairOfRootStepID))); repairRoot != "" {
			return repairRoot
		}
	}
	return root
}

func recoveryRootNeedsNormalization(root string) bool {
	return strings.Contains(root, "/provider-retry-") ||
		strings.Contains(root, "of-provider-retry-")
}

func isRetryableLLMProviderRuntimeFailure(run *runModel.Run, step *runModel.Step) bool {
	if !isLLMProviderRuntimeFailure(run, step) {
		return false
	}
	if stringFromMap(step.Output, failureOutputKeyClass) == failureClassProviderUnavailable &&
		!providerUnavailableFailureRequiresConfigBlock(step) {
		return true
	}
	switch stringFromMap(step.Output, failureOutputKeyClass) {
	case failureClassProviderTimeout, failureClassContextTooLarge, failureClassRateLimited, failureClassProviderStreamIncomplete, failureClassProviderProtocolInvalid:
		return true
	default:
		return false
	}
}

func isLLMProviderRuntimeFailure(run *runModel.Run, step *runModel.Step) bool {
	if run == nil || step == nil || step.Kind != initialStepKind(run) {
		return false
	}
	return isLLMProviderFailureStage(stringFromMap(step.Output, "stage"))
}

func (r *repairSubsystem) runtimeRepairAttemptCount(ctx context.Context, runID common.RunID) int {
	if r == nil || r.steps == nil || runID == "" {
		return 0
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if isRuntimeRepairBudgetStep(runID, step) {
			count++
		}
	}
	return count
}

func (r *repairSubsystem) runtimeRepairAttemptCountForStep(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	if r == nil || r.steps == nil || runID == "" || failedStep == nil {
		return 0
	}
	scope := runtimeRepairScopeFromFailedStep(failedStep)
	return r.runtimeRepairAttemptCountForScopeInWindow(ctx, runID, scope, runtimeRepairTargetBudgetWindowRoot(failedStep))
}

func (r *repairSubsystem) runtimeRepairAttemptCountForEffectiveScope(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	if r == nil || r.steps == nil || runID == "" || failedStep == nil {
		return 0
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	exactAttempts := runtimeRepairAttemptCountForScopeInSteps(runID, steps, runtimeRepairScopeFromFailedStep(failedStep), runtimeRepairTargetBudgetWindowRoot(failedStep))
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	if targetScope == "" {
		rootClassAttempts := runtimeRepairAttemptCountForRootClassInSteps(runID, steps, failedStep)
		if rootClassAttempts > exactAttempts {
			return rootClassAttempts
		}
		return exactAttempts
	}
	class, _ := runtimeRepairScopeClassAndSignature(failedStep)
	targetAttempts := runtimeRepairAttemptCountForFailureTargetInSteps(runID, steps, class, targetScope, runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep))
	attempts := exactAttempts
	if targetAttempts > exactAttempts {
		attempts = targetAttempts
	}
	contractAttempts := runtimeRepairAttemptCountForTargetContractCorrectionInSteps(runID, steps, failedStep, targetScope)
	if contractAttempts > attempts {
		attempts = contractAttempts
	}
	return attempts
}

func (r *repairSubsystem) runtimeRepairAttemptCountForScope(ctx context.Context, runID common.RunID, scope runtimeRepairScope) int {
	return r.runtimeRepairAttemptCountForScopeInWindow(ctx, runID, scope, "")
}

func (r *repairSubsystem) runtimeRepairAttemptCountForScopeInWindow(ctx context.Context, runID common.RunID, scope runtimeRepairScope, windowRoot common.StepID) int {
	if r == nil || r.steps == nil || runID == "" {
		return 0
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	return runtimeRepairAttemptCountForScopeInSteps(runID, steps, scope, windowRoot)
}

func (r *repairSubsystem) runtimeRepairAttemptCountForRootClassInWindow(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	if r == nil || r.steps == nil || runID == "" || failedStep == nil || !boolValue(failedStep.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return 0
	}
	windowRoot := runtimeRepairTargetBudgetWindowRoot(failedStep)
	if windowRoot == "" {
		return 0
	}
	failureClass, _ := runtimeRepairScopeClassAndSignature(failedStep)
	failureClass = strings.TrimSpace(failureClass)
	if failureClass == "" {
		return 0
	}
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	return runtimeRepairAttemptCountForRootClassInSteps(runID, steps, failedStep)
}

func (r *repairSubsystem) runtimeRepairAttemptCountForFailureTarget(ctx context.Context, runID common.RunID, failureClass string, targetScope string) int {
	return r.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, runID, failureClass, targetScope, "")
}

func (r *repairSubsystem) runtimeRepairAttemptCountForFailureTargetInWindow(ctx context.Context, runID common.RunID, failureClass string, targetScope string, windowRoot common.StepID) int {
	if r == nil || r.steps == nil || runID == "" {
		return 0
	}
	targetScope = strings.TrimSpace(targetScope)
	if targetScope == "" {
		return 0
	}
	failureClass = strings.TrimSpace(failureClass)
	steps, err := r.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	return runtimeRepairAttemptCountForFailureTargetInSteps(runID, steps, failureClass, targetScope, windowRoot)
}

func runtimeRepairAttemptCountForScopeInSteps(runID common.RunID, steps []*runModel.Step, scope runtimeRepairScope, windowRoot common.StepID) int {
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		if runtimeRepairStepMatchesScope(step, scope) {
			count++
		}
	}
	return count
}

func runtimeRepairAttemptCountForRootClassInSteps(runID common.RunID, steps []*runModel.Step, failedStep *runModel.Step) int {
	if failedStep == nil || !boolValue(failedStep.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return 0
	}
	windowRoot := runtimeRepairTargetBudgetWindowRoot(failedStep)
	if windowRoot == "" {
		return 0
	}
	failureClass, _ := runtimeRepairScopeClassAndSignature(failedStep)
	failureClass = strings.TrimSpace(failureClass)
	if failureClass == "" {
		return 0
	}
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		stepClass := runtimeRepairStepFailureClass(step)
		if stepClass == failureClass {
			count++
		}
	}
	return count
}

func runtimeRepairAttemptCountForFailureTargetInSteps(runID common.RunID, steps []*runModel.Step, failureClass string, targetScope string, windowRoot common.StepID) int {
	targetScope = strings.TrimSpace(targetScope)
	if targetScope == "" {
		return 0
	}
	failureClass = strings.TrimSpace(failureClass)
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if runtimeFailureTargetScopeFromStep(step) != targetScope {
			continue
		}
		if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		if failureClass != "" {
			stepClass := runtimeRepairStepFailureClass(step)
			if stepClass != "" && stepClass != failureClass {
				continue
			}
		}
		count++
	}
	return count
}

func runtimeRepairAttemptCountForTargetContractCorrectionInSteps(runID common.RunID, steps []*runModel.Step, failedStep *runModel.Step, targetScope string) int {
	targetScope = strings.TrimSpace(targetScope)
	if targetScope == "" || !runtimeRepairStepIsContractCorrectionFailure(failedStep) {
		return 0
	}
	windowRoot := runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep)
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if runtimeFailureTargetScopeFromStep(step) != targetScope {
			continue
		}
		if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		if !runtimeRepairStepIsContractCorrectionFailure(step) {
			continue
		}
		count++
	}
	return count
}

func runtimeRepairStepIsContractCorrectionFailure(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind {
		return false
	}
	if marker := strings.TrimSpace(stringFromMap(step.Output, outputKeyContractViolationType)); marker != "" {
		return true
	}
	if runtimeFailureIsLLMContractLoop(step) {
		return true
	}
	return strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) == failureClassRuntime &&
		strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)) != ""
}

func runtimeRepairStepFailureClass(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
		stringFromMap(step.Input, failureOutputKeyClass),
		stringFromMap(step.Output, failureOutputKeyClass),
	))
}

func duplicateRuntimeFailureBlockerCanAutoResume(
	ctx context.Context,
	steps repository.StepRepository,
	repair *repairSubsystem,
	policy RuntimePolicy,
	run *runModel.Run,
	ticket *decisionmodel.Ticket,
) bool {
	if steps == nil || repair == nil || run == nil || ticket == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || ticket.State != common.DecisionTicketStateOpen {
		return false
	}
	if !strings.HasSuffix(string(ticket.ID), "duplicate-failure-decision") {
		return false
	}
	failedStep, err := steps.Get(ctx, ticket.StepID)
	if err != nil || failedStep == nil || failedStep.State != common.StepStateFailed {
		return false
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	if signature == "" || !duplicateRepairDecisionFailureClass(class) {
		return false
	}
	attempts := repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep)
	return attempts > 0 && attempts < maxRuntimeRepairAttemptsForPolicy(policy)
}

func runtimeRepairExhaustionBlockerCanAutoResume(
	ctx context.Context,
	steps repository.StepRepository,
	repair *repairSubsystem,
	policy RuntimePolicy,
	run *runModel.Run,
	ticket *decisionmodel.Ticket,
) bool {
	if steps == nil || repair == nil || run == nil || ticket == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || ticket.State != common.DecisionTicketStateOpen {
		return false
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		return false
	}
	if !strings.HasSuffix(string(ticket.ID), "repair-exhausted-decision") {
		return false
	}
	if attempts, ok := ticket.Context["attempts"]; !ok || intFromAny(attempts) < maxRuntimeRepairAttemptsForPolicy(policy) {
		return false
	}
	failedStep, err := steps.Get(ctx, ticket.StepID)
	if err != nil || failedStep == nil || failedStep.State != common.StepStateFailed {
		return false
	}
	if isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	if runtimeRepairExhaustionHasCompletedCoverageEvidence(ticket.Context) {
		return true
	}
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	if targetScope == "" {
		return false
	}
	if runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep) == "" {
		class, _ := runtimeRepairScopeClassAndSignature(failedStep)
		if !runtimeFailureTargetUsesRunBudget(class, targetScope) {
			return false
		}
	}
	return repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) < maxRuntimeRepairAttemptsForPolicy(policy)
}

func runtimeRepairExhaustionHasCompletedCoverageEvidence(context map[string]any) bool {
	if len(context) == 0 {
		return false
	}
	for _, source := range []any{
		context["coverageTargetSet"],
		mapFromValue(context["failedStepInput"])["coverageTargetSet"],
		mapFromValue(context["failedStepInput"])["missingDoDItems"],
	} {
		for _, target := range mapSliceFromValue(source) {
			if coverageTargetHasCompletedActionEvidenceCandidate(target) {
				return true
			}
		}
	}
	return false
}

func coverageTargetHasCompletedActionEvidenceCandidate(target map[string]any) bool {
	if len(target) == 0 {
		return false
	}
	for _, candidate := range mapSliceFromValue(target["evidenceCandidates"]) {
		if evidenceValueString(candidate["state"]) != string(common.StepStateCompleted) {
			continue
		}
		switch evidenceValueString(candidate["kind"]) {
		case "patch/apply", "shell/command", "mcp/tool_call", "git/write":
			return true
		}
	}
	return false
}

func knownReadableDecisionBlockerCanAutoRepair(
	ctx context.Context,
	steps repository.StepRepository,
	workspaces repository.WorkspaceRepository,
	run *runModel.Run,
	ticket *decisionmodel.Ticket,
) bool {
	if steps == nil || run == nil || ticket == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || ticket.State != common.DecisionTicketStateOpen {
		return false
	}
	if strings.TrimSpace(ticket.DecisionKind) != "" &&
		strings.TrimSpace(ticket.DecisionKind) != decisioncommon.ResolutionTypeProvideMissingInfo {
		return false
	}
	step, err := steps.Get(ctx, ticket.StepID)
	if err != nil || step == nil || step.State != common.StepStateCompleted {
		return false
	}
	context := enrichDecisionContextWithReadableWorkspaceResources(ctx, workspaces, run, nil, ticket.Context, ticket.Question, ticket.Reason)
	return knownReadableWorkspaceResourceDecisionContext(context)
}

func runtimeRepairExhaustionBlockerHasSuccessEvidence(ctx context.Context, steps repository.StepRepository, ticket *decisionmodel.Ticket) bool {
	if steps == nil || ticket == nil || ticket.RunID == "" {
		return false
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		return false
	}
	scope := runtimeRepairScopeFromDecisionContext(ticket.Context)
	if scope.failureSignature == "" && scope.rootStepID == "" && scope.failedStepID == "" {
		return false
	}
	stepsForRun, err := steps.ListByRunSummary(ctx, ticket.RunID)
	if err != nil {
		return false
	}
	for _, step := range stepsForRun {
		if !isRuntimeRepairSuccessEvidenceStep(step, scope) {
			continue
		}
		if ticket.StepID != "" && step.ID == ticket.StepID {
			continue
		}
		return true
	}
	return false
}

func isRuntimeRepairSuccessEvidenceStep(step *runModel.Step, scope runtimeRepairScope) bool {
	if step == nil || step.State != common.StepStateCompleted || step.Input == nil {
		return false
	}
	if !runtimeRepairStepMatchesScope(step, scope) {
		return false
	}
	if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return false
	}
	if isProviderFailureClass(stringFromMap(step.Output, failureOutputKeyClass)) || isLLMProviderFailureStage(stringFromMap(step.Output, "stage")) {
		return false
	}
	return true
}

func isRuntimeRepairBudgetStep(runID common.RunID, step *runModel.Step) bool {
	if step == nil || step.RunID != runID || !boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return false
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		return false
	}
	return true
}

type runtimeRepairScope struct {
	failedStepID     common.StepID
	rootStepID       common.StepID
	failureClass     string
	failureSignature string
}

func runtimeRepairScopeFromFailedStep(step *runModel.Step) runtimeRepairScope {
	if step == nil {
		return runtimeRepairScope{}
	}
	class, signature := runtimeRepairScopeClassAndSignature(step)
	return runtimeRepairScope{
		failedStepID:     step.ID,
		rootStepID:       runtimeRepairRootStepID(step),
		failureClass:     class,
		failureSignature: signature,
	}
}

func runtimeRepairScopeClassAndSignature(step *runModel.Step) (string, string) {
	if step == nil {
		return "", ""
	}
	outputClass := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	outputSignature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	inputClass := strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfFailureClass))
	inputSignature := strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfFailureSig))
	if outputSignature != "" && outputClass != "" && !isProviderFailureClass(outputClass) {
		return outputClass, outputSignature
	}
	if outputSignature != "" && (!boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) || inputSignature == "" || failureSignatureHasTargetScope(outputSignature)) {
		return firstNonEmptyString(outputClass, inputClass), outputSignature
	}
	return firstNonEmptyString(inputClass, outputClass), firstNonEmptyString(inputSignature, outputSignature)
}

func failureSignatureHasTargetScope(signature string) bool {
	return runtimeFailureTargetScope(signature) != ""
}

func runtimeFailureTargetScopeFromStep(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	_, signature := runtimeRepairScopeClassAndSignature(step)
	if targetScope := runtimeFailureTargetScope(signature); targetScope != "" {
		return targetScope
	}
	if targetScope := runtimeFailureTargetScope(firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeyRepairOfFailureSig),
		stringFromMap(step.Input, failureOutputKeySignature),
		stringFromMap(step.Output, failureOutputKeySignature),
	)); targetScope != "" {
		return targetScope
	}
	return runtimeFailureTargetScopeFromRepairEvidence(step)
}

func runtimeFailureTargetScopeFromRepairEvidence(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	targets := runtimeFailureTargetRefsFromRepairEvidence(step)
	if len(targets) == 0 {
		return ""
	}
	return "failureTargets=" + strings.Join(targets, ",")
}

func runtimeFailureTargetRefsFromRepairEvidence(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRuntimeFailureTargetValue(value)
		if value == "" {
			return
		}
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			ret = append(ret, item)
		}
	}
	addScope := func(scope string) {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return
		}
		for _, part := range strings.Split(scope, "|") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "failureTargets", "targetRef", "conflictRef":
				add(value)
			}
		}
	}
	addTargetsFromMap := func(value map[string]any) {
		if len(value) == 0 {
			return
		}
		for _, key := range []string{"failureTargetScope", "targetScope"} {
			addScope(stringFromMap(value, key))
		}
		for _, key := range []string{"targetRef", "conflictRef"} {
			add(stringFromMap(value, key))
		}
		for _, key := range []string{"failureTargets", "targetRefs", "relatedFailureTargets", "affectedTargets"} {
			for _, target := range stringSliceFromValue(value[key]) {
				add(target)
			}
		}
		for _, item := range mapSliceFromValue(value["targetSnapshots"]) {
			add(stringFromMap(item, "targetRef"))
		}
		for _, item := range mapSliceFromValue(value["currentTargetSnapshots"]) {
			add(stringFromMap(item, "targetRef"))
		}
		for _, item := range mapSliceFromValue(value["failureTargetRepairContext"]) {
			add(stringFromMap(item, "targetRef"))
		}
		for _, item := range mapSliceFromValue(value["patchRepairContext"]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	for _, source := range []map[string]any{
		step.Input,
		step.Output,
		mapFromValue(step.Input["repairActionPackage"]),
		mapFromValue(step.Output["repairActionPackage"]),
		mapFromValue(step.Input["currentFailure"]),
		mapFromValue(step.Output["currentFailure"]),
		mapFromValue(mapFromValue(step.Input["repairActionPackage"])["currentFailure"]),
		mapFromValue(mapFromValue(step.Output["repairActionPackage"])["currentFailure"]),
	} {
		addTargetsFromMap(source)
	}
	if len(ret) == 0 {
		return nil
	}
	return limitedStrings(ret, maxLLMContextSliceItems)
}

func runtimeRepairTargetBudgetWindowRoot(step *runModel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	for _, key := range []string{
		stepInputKeyRepairOfRootStepID,
		stepInputKeyGraphRootStepID,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyProviderRetryRootStep,
	} {
		root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, key)))
		if root != "" {
			return normalizeRecoveryRootStepIDFromInput(step.Input, root)
		}
	}
	return ""
}

func runtimeRepairTargetBudgetWindowRootForFailureTarget(step *runModel.Step) common.StepID {
	class, _ := runtimeRepairScopeClassAndSignature(step)
	if runtimeFailureTargetUsesRunBudget(class, runtimeFailureTargetScopeFromStep(step)) {
		return ""
	}
	return runtimeRepairTargetBudgetWindowRoot(step)
}

func runtimeFailureTargetUsesRunBudget(class string, targetScope string) bool {
	if strings.TrimSpace(targetScope) == "" {
		return false
	}
	switch strings.TrimSpace(class) {
	case failureClassMCPRuntime, failureClassMCPCapabilityDenied, failureClassPatchApply:
		return true
	default:
		return false
	}
}

func runtimeFailureTargetScope(signature string) string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ""
	}
	parts := strings.Split(signature, "|")
	values := map[string]string{}
	for _, part := range parts {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = normalizeRuntimeFailureTargetValue(value)
		if name == "" || value == "" {
			continue
		}
		values[name] = value
	}
	if runtimeFailureSignatureIsCapabilityCall(values) {
		if values["serverId"] != "" && values["tool"] != "" {
			scope := "serverId=" + values["serverId"] + "|tool=" + values["tool"]
			if values["node"] != "" {
				scope += "|node=" + values["node"]
			}
			return scope
		}
		for _, key := range []string{"serverId", "tool", "uri"} {
			if value := values[key]; value != "" {
				return key + "=" + value
			}
		}
	}
	for _, key := range []string{"failureTargets", "targetRef", "conflictRef"} {
		if value := values[key]; value != "" {
			return key + "=" + value
		}
	}
	if values["serverId"] != "" && values["tool"] != "" {
		scope := "serverId=" + values["serverId"] + "|tool=" + values["tool"]
		if values["node"] != "" {
			scope += "|node=" + values["node"]
		}
		return scope
	}
	for _, key := range []string{"serverId", "tool", "uri"} {
		if value := values[key]; value != "" {
			return key + "=" + value
		}
	}
	return ""
}

func runtimeFailureSignatureIsCapabilityCall(values map[string]string) bool {
	if len(values) == 0 {
		return false
	}
	return values["class"] == failureClassMCPRuntime ||
		values["class"] == failureClassMCPCapabilityDenied ||
		values["kind"] == "mcp/tool_call" ||
		strings.HasPrefix(values["stage"], "mcp_")
}

func normalizeRuntimeFailureTargetValue(value string) string {
	items := strings.Split(value, ",")
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.TrimPrefix(item, "./")
		if item == "" {
			continue
		}
		ret = append(ret, item)
	}
	return strings.Join(ret, ",")
}

func runtimeRepairScopeFromDecisionContext(context map[string]any) runtimeRepairScope {
	if len(context) == 0 {
		return runtimeRepairScope{}
	}
	return runtimeRepairScope{
		failedStepID:     common.StepID(strings.TrimSpace(stringFromMap(context, "failedStepId"))),
		rootStepID:       common.StepID(strings.TrimSpace(firstNonEmptyString(stringFromMap(context, "repairOfRootStepId"), stringFromMap(context, stepInputKeyRepairOfRootStepID)))),
		failureClass:     strings.TrimSpace(stringFromMap(context, "failureClass")),
		failureSignature: strings.TrimSpace(stringFromMap(context, "failureSignature")),
	}
}

func runtimeRepairRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfRootStepID))); root != "" {
		return normalizeRecoveryRootStepIDFromInput(step.Input, root)
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyProviderRetryRootStep))); root != "" {
			return normalizeRecoveryRootStepIDFromInput(step.Input, root)
		}
	}
	if root := graphRootStepID(step); root != "" {
		return normalizeRecoveryRootStepIDFromInput(step.Input, root)
	}
	return normalizeRecoveryRootStepIDFromInput(step.Input, step.ID)
}

func runtimeRepairStepMatchesScope(step *runModel.Step, scope runtimeRepairScope) bool {
	if step == nil {
		return false
	}
	if scope.failureSignature != "" {
		stepSignature := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(step.Input, stepInputKeyRepairOfFailureSig),
			stringFromMap(step.Input, failureOutputKeySignature),
			stringFromMap(step.Output, failureOutputKeySignature),
		))
		if stepSignature != scope.failureSignature {
			if stepSignature == "" && !runtimeRepairStepHasExplicitScope(step) {
				return true
			}
			return false
		}
		if scope.failureClass != "" {
			stepClass := strings.TrimSpace(firstNonEmptyString(
				stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
				stringFromMap(step.Input, failureOutputKeyClass),
				stringFromMap(step.Output, failureOutputKeyClass),
			))
			if stepClass != "" && stepClass != scope.failureClass {
				return false
			}
		}
		return true
	}
	if scope.rootStepID != "" {
		stepRoot := common.StepID(strings.TrimSpace(firstNonEmptyString(
			stringFromMap(step.Input, stepInputKeyRepairOfRootStepID),
			string(graphRootStepID(step)),
		)))
		if stepRoot == scope.rootStepID {
			return true
		}
	}
	if scope.failedStepID != "" {
		if common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfStepID))) == scope.failedStepID ||
			common.StepID(strings.TrimSpace(stringFromMap(step.Input, "failedStepId"))) == scope.failedStepID {
			return true
		}
	}
	if scope.failureSignature == "" && !runtimeRepairStepHasExplicitScope(step) {
		return true
	}
	return false
}

func runtimeRepairStepHasExplicitScope(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	for _, key := range []string{
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failedStepId",
	} {
		if strings.TrimSpace(stringFromMap(step.Input, key)) != "" || strings.TrimSpace(stringFromMap(step.Output, key)) != "" {
			return true
		}
	}
	return false
}

func isProviderFailureClass(class string) bool {
	switch class {
	case failureClassProviderUnavailable,
		failureClassProviderSchemaDrift,
		failureClassProviderProtocolInvalid,
		failureClassProviderTimeout,
		failureClassContextTooLarge,
		failureClassRateLimited,
		failureClassProviderStreamIncomplete,
		failureClassAuthOrConfigError:
		return true
	default:
		return false
	}
}

func isLLMProviderFailureStage(stage string) bool {
	switch stage {
	case "llm_provider_unavailable",
		"llm_provider_client_unavailable",
		"llm_provider_operation_unsupported",
		"llm_provider_timeout",
		"llm_provider_request_too_large",
		"llm_provider_rate_limited",
		"llm_provider_auth_or_config_error",
		"llm_provider_stream_incomplete",
		"llm_provider_error",
		"llm_provider_stream_error",
		"llm_provider_request_failed",
		"llm_provider_schema_drift",
		"llm_provider_response_schema_invalid",
		"llm_provider_protocol_invalid":
		return true
	default:
		return false
	}
}
