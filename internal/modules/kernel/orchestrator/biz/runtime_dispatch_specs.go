package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (o *RuntimeOrchestrator) nextDispatchSpecs(
	ctx context.Context,
	run *runModel.Run,
) ([]stepDispatchSpec, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	if summary, ok := o.resultManager().rejectedRunResultSummary(ctx, run.ID); ok {
		reason := strings.TrimSpace(summary)
		if reason == "" {
			reason = "run has rejected result"
		}
		if !isTerminalRunState(run.State) {
			if err := o.failLoadedRun(ctx, run, reason); err != nil {
				return nil, err
			}
		}
		return nil, common.ErrConflict
	}
	if o.coverageLedger().ensure(run) && o.runs != nil {
		if err := o.runs.Update(ctx, run); err != nil {
			return nil, err
		}
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		input := initialStepInput(run)
		input = o.hydrateInitialRepairTargetSnapshots(ctx, run, input)
		if providerID := common.RuntimeLLMProviderIDFromContext(ctx); providerID != "" {
			input["llmProviderId"] = providerID
		}
		return []stepDispatchSpec{{
			ID:    common.StepID(fmt.Sprintf("%s/step-1", run.ID)),
			Kind:  initialStepKind(run),
			Input: input,
		}}, nil
	}

	last := terminalExecutionStep(steps)
	if last == nil {
		return nil, common.ErrConflict
	}
	return o.nextDispatchSpecsFromStep(ctx, run, last)
}

func (o *RuntimeOrchestrator) hydrateInitialRepairTargetSnapshots(ctx context.Context, run *runModel.Run, input map[string]any) map[string]any {
	if o == nil || o.workspaces == nil || run == nil || len(input) == 0 {
		return input
	}
	if !boolValue(input["replanStateChangingContinuation"]) && runtimeStageFromStepInput(input) != runtimeStageRepair {
		return input
	}
	pkg := mapFromValue(input["repairActionPackage"])
	targetRefs := initialRepairTargetRefs(input, pkg)
	if len(targetRefs) == 0 {
		return o.convergenceManager().apply(input, o.convergenceManager().build(run, nil, nil, map[string]any{"input": input}, nil))
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return o.convergenceManager().apply(input, o.convergenceManager().build(run, nil, nil, map[string]any{"input": input}, nil))
	}
	var snapshots []map[string]any
	for _, targetRef := range targetRefs {
		if len(snapshots) >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) == 0 || !boolValue(item["exists"]) || boolValue(item["isBinary"]) {
			continue
		}
		snapshots = append(snapshots, item)
	}
	if len(snapshots) == 0 {
		return o.convergenceManager().apply(input, o.convergenceManager().build(run, nil, nil, map[string]any{"input": input}, nil))
	}
	input["patchRepairContext"] = mergeRepairTargetSnapshots(mapSliceFromValue(input["patchRepairContext"]), snapshots)
	pkg = repairActionPackageForEvidenceAcquisitionFollowUp(input, pkg, snapshots)
	input["repairActionPackage"] = pkg
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	delete(input, "concreteFailureEvidenceRequired")
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Runtime loaded current target snapshots from repairTargetSet before this replan repair. Use patchRepairContext and repairActionPackage.targetSnapshots as the current patch baseline.",
		"Do not generate stale hunks from historical summaries when current target snapshots are available.",
	)
	return o.convergenceManager().apply(input, o.convergenceManager().build(run, nil, nil, map[string]any{"input": input}, nil))
}

func initialRepairTargetRefs(input map[string]any, pkg map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, value := range stringSliceFromValue(input["repairTargetSet"]) {
		add(value)
	}
	for _, value := range stringSliceFromValue(pkg["repairTargetSet"]) {
		add(value)
	}
	for _, item := range mapSliceFromValue(input["repairTargetSet"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(pkg["repairTargetSet"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(input["targetSnapshots"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(pkg["targetSnapshots"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, key := range []string{
		"failureTargetScope",
		failureOutputKeySignature,
		stepInputKeyRepairOfFailureSig,
	} {
		for _, value := range repairPatchTargetRefsFromScopeText(stringFromMap(input, key)) {
			add(value)
		}
		for _, value := range repairPatchTargetRefsFromScopeText(stringFromMap(pkg, key)) {
			add(value)
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) nextDispatchSpecsAfterStep(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, error) {
	if stepID == "" {
		return o.nextDispatchSpecs(ctx, run)
	}
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	if o.coverageLedger().ensure(run) && o.runs != nil {
		if err := o.runs.Update(ctx, run); err != nil {
			return nil, err
		}
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step != nil && step.ID == stepID {
			return o.nextDispatchSpecsFromStep(ctx, run, step)
		}
	}
	return nil, common.ErrConflict
}

func (o *RuntimeOrchestrator) nextDispatchSpecsAfterResolvedDecision(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
	resumeInput map[string]any,
) ([]stepDispatchSpec, error) {
	if boolValue(resumeInput["knownReadableDecisionRepair"]) {
		if specs, ok, err := o.knownReadableDecisionRepairSpecs(ctx, run, stepID); ok || err != nil {
			return specs, err
		}
	}
	if boolValue(resumeInput["providerRetryDecisionRetry"]) {
		if specs, ok, err := o.providerRetrySpecsAfterResolvedDecision(ctx, run, stepID); ok || err != nil {
			return specs, err
		}
	}
	if boolValue(resumeInput["repairPlanDecisionRetry"]) {
		if specs, ok, err := o.runtimeRepairSpecsAfterResolvedDecision(ctx, run, stepID); ok || err != nil {
			return specs, err
		}
	}
	if contextGateDecisionResumeInput(resumeInput) {
		if specs, ok, err := o.contextIntegritySpecsAfterResolvedDecision(ctx, run, stepID, resumeInput); ok || err != nil {
			return specs, err
		}
	}
	return o.nextDispatchSpecsAfterStep(ctx, run, stepID)
}

func (o *RuntimeOrchestrator) runtimeRepairSpecsAfterResolvedDecision(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, bool, error) {
	if o == nil || run == nil || stepID == "" || o.steps == nil {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return nil, true, err
	}
	if step == nil || step.State != common.StepStateFailed {
		return nil, false, nil
	}
	attempt := o.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, step) + 1
	stepID, err = uniqueDispatchStepID(ctx, o.steps, run.ID, runtimeRepairFollowUpStepID(run.ID, step, attempt))
	if err != nil {
		return nil, true, err
	}
	input := o.runtimeRepairFollowUpStepInput(ctx, run, step, attempt, nil)
	input["decisionAuthorizedRetry"] = true
	return []stepDispatchSpec{{
		ID:    stepID,
		Kind:  initialStepKind(run),
		Input: input,
	}}, true, nil
}

func contextGateDecisionResumeInput(resumeInput map[string]any) bool {
	context := mapFromValue(resumeInput["ticketContext"])
	if len(context) == 0 {
		return false
	}
	return strings.TrimSpace(stringFromMap(context, decisionContextKeyRecoveryPolicy)) == decisionContextRecoveryPolicyContext ||
		strings.TrimSpace(stringFromMap(context, decisionContextKeyDependency)) == decisionContextDependencyContextGate
}

func (o *RuntimeOrchestrator) providerRetrySpecsAfterResolvedDecision(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, bool, error) {
	if o == nil || run == nil || stepID == "" {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return nil, false, err
	}
	if step == nil || step.State != common.StepStateFailed || !isLLMProviderRuntimeFailure(run, step) {
		return nil, false, nil
	}
	rootStepID := providerRetryRootStepID(step)
	attempt := o.repairManager().providerRetryAttemptCount(ctx, run.ID, rootStepID) + 1
	stepID, err = uniqueDispatchStepID(ctx, o.steps, run.ID, providerRetryStepID(run.ID, rootStepID, attempt))
	if err != nil {
		return nil, true, err
	}
	input := providerRetryStepInputWithMaxAttempts(step, attempt, nil, maxProviderRetryAttemptsForPolicy(o.policy))
	input["decisionAuthorizedRetry"] = true
	return []stepDispatchSpec{{
		ID:    stepID,
		Kind:  step.Kind,
		Input: input,
	}}, true, nil
}

func (o *RuntimeOrchestrator) contextIntegritySpecsAfterResolvedDecision(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
	resumeInput map[string]any,
) ([]stepDispatchSpec, bool, error) {
	if o == nil || run == nil || stepID == "" {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return nil, false, err
	}
	if step == nil || step.State != common.StepStateFailed || !runtimeContextIntegrityGateFailure(step) {
		return nil, false, nil
	}
	input := contextIntegrityRetryInput(step, resumeInput)
	if contextIntegrityRetryStillMissingRequiredRepairContext(input) {
		return nil, true, common.ErrConflict
	}
	baseID := common.StepID(fmt.Sprintf("%s/context-gate-retry", step.ID))
	nextStepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, baseID)
	if err != nil {
		return nil, true, err
	}
	return []stepDispatchSpec{{
		ID:    nextStepID,
		Kind:  step.Kind,
		Input: input,
	}}, true, nil
}

func contextIntegrityRetryInput(step *runModel.Step, resumeInput map[string]any) map[string]any {
	input := cloneStepPayload(step.Input)
	if input == nil {
		input = map[string]any{}
	}
	ticketContext := cloneStepPayload(mapFromValue(resumeInput["ticketContext"]))
	if len(ticketContext) > 0 {
		input["ticketContext"] = ticketContext
	}
	if decisionResolution := cloneStepPayload(mapFromValue(resumeInput["decisionResolution"])); len(decisionResolution) > 0 {
		input["decisionResolution"] = decisionResolution
	}
	input["contextGateRecovery"] = map[string]any{
		"sourceStepId": string(step.ID),
		"resolved":     true,
		"recovery":     "regenerate_llm_context",
	}
	if reason := strings.TrimSpace(stringFromMap(mapFromValue(step.Output["contextBudget"]), "contextGateReason")); reason != "" {
		mapFromValue(input["contextGateRecovery"])["contextGateReason"] = reason
	}
	hydrateContextIntegrityRetryRepairPackage(input, step, ticketContext)
	return input
}

func hydrateContextIntegrityRetryRepairPackage(input map[string]any, step *runModel.Step, ticketContext map[string]any) {
	if input == nil {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if pkg == nil {
		pkg = map[string]any{}
	}
	failedStepInput := mapFromValue(ticketContext["failedStepInput"])
	failedStepInputPkg := mapFromValue(failedStepInput["repairActionPackage"])
	if len(pkg) == 0 && len(failedStepInputPkg) > 0 {
		pkg = cloneStepPayload(failedStepInputPkg)
	}
	if len(mapFromValue(pkg["currentFailure"])) == 0 {
		if currentFailure := currentFailureForEvidenceAcquisitionFollowUp(input, pkg); len(currentFailure) > 0 {
			pkg["currentFailure"] = currentFailure
		} else if currentFailure := currentFailureForEvidenceAcquisitionFollowUp(failedStepInput, failedStepInputPkg); len(currentFailure) > 0 {
			pkg["currentFailure"] = currentFailure
		} else if currentFailure := currentFailureFromContextGateTicket(ticketContext); len(currentFailure) > 0 {
			pkg["currentFailure"] = currentFailure
		}
	}
	if len(mapFromValue(pkg["currentExecutionProgress"])) == 0 {
		if progress := currentExecutionProgressForEvidenceAcquisitionFollowUp(input, pkg); len(progress) > 0 {
			pkg["currentExecutionProgress"] = progress
		} else if progress := currentExecutionProgressForEvidenceAcquisitionFollowUp(failedStepInput, failedStepInputPkg); len(progress) > 0 {
			pkg["currentExecutionProgress"] = progress
		} else if progress := currentExecutionProgressFromContextGateTicket(step, ticketContext); len(progress) > 0 {
			pkg["currentExecutionProgress"] = progress
		}
	}
	input["repairActionPackage"] = pkg
}

func currentFailureFromContextGateTicket(ticketContext map[string]any) map[string]any {
	if len(ticketContext) == 0 {
		return nil
	}
	output := mapFromValue(ticketContext["failedStepOutput"])
	ret := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			ret[key] = value
		}
	}
	add("failedStepId", stringFromMap(ticketContext, "failedStepId"))
	add("failedStepKind", stringFromMap(ticketContext, "failedStepKind"))
	add("failureClass", firstNonEmptyString(stringFromMap(ticketContext, failureOutputKeyClass), stringFromMap(output, failureOutputKeyClass)))
	add("failureSignature", firstNonEmptyString(stringFromMap(ticketContext, failureOutputKeySignature), stringFromMap(output, failureOutputKeySignature)))
	add("targetScope", stringFromMap(ticketContext, "failureTargetScope"))
	add("targetRef", repairActionTargetRefFromScope(stringFromMap(ticketContext, "failureTargetScope")))
	add("reason", firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(ticketContext, "reason")))
	add("error", firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(ticketContext, "error")))
	add("stage", stringFromMap(output, "stage"))
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func failedAssessmentDecisionShouldRepair(step *runModel.Step, output map[string]any, decisionSpec runtimeDecisionBlockSpec) bool {
	if step == nil || output == nil {
		return false
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageVerification {
		return false
	}
	if !failedAssessmentHasActionableCoverageGap(output) {
		return false
	}
	if strings.TrimSpace(decisionSpec.Reason) == "" {
		return false
	}
	return true
}

func currentExecutionProgressFromContextGateTicket(step *runModel.Step, ticketContext map[string]any) map[string]any {
	failedStepInput := mapFromValue(ticketContext["failedStepInput"])
	failedStepInputPkg := mapFromValue(failedStepInput["executionContextPackage"])
	failedStepInputContext := mapFromValue(failedStepInput["executionContext"])
	for _, source := range []map[string]any{
		mapFromValue(failedStepInput["currentProgressDigest"]),
		replanContinuationCurrentProgressDigest(failedStepInputPkg),
		replanContinuationCurrentProgressDigest(failedStepInputContext),
	} {
		if repairProgressDigestIsInformative(source) {
			return compactLLMContextMap(source, 0)
		}
	}
	reason := firstNonEmptyString(
		stringFromMap(mapFromValue(step.Output["contextBudget"]), "contextGateReason"),
		stringFromMap(step.Output, "reason"),
		stringFromMap(step.Output, "error"),
	)
	if strings.TrimSpace(reason) == "" {
		return nil
	}
	return map[string]any{
		"currentGap":         reason,
		"nextRequiredAction": "regenerate_llm_context_after_resolved_context_gate",
	}
}

func contextIntegrityRetryStillMissingRequiredRepairContext(input map[string]any) bool {
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(input["stateChangingOutputRequired"]) &&
		!repairObjectiveRequiresStateChange(stringFromMap(input, "repairObjective")) &&
		!repairActionPackageRequiresStateChangingOutput(pkg) &&
		!boolValue(required["mustIncludeStateChangingNode"]) {
		return false
	}
	return len(mapFromValue(pkg["currentFailure"])) == 0 ||
		len(mapFromValue(pkg["currentExecutionProgress"])) == 0
}

func (o *RuntimeOrchestrator) knownReadableDecisionRepairSpecs(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, bool, error) {
	if o == nil || run == nil || stepID == "" {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return nil, false, err
	}
	if step == nil || step.State != common.StepStateCompleted {
		return nil, false, nil
	}
	decisionSpec := decisionBlockSpec(step.Output)
	if decisionSpec.Reason == "" {
		return nil, false, nil
	}
	decisionSpec.Context = o.enrichDecisionContextWithReadableWorkspaceResources(ctx, run, nil, decisionSpec.Context, decisionSpec.Question, decisionSpec.Reason)
	if !knownReadableWorkspaceResourceDecisionContext(decisionSpec.Context) {
		return nil, false, nil
	}
	repairOf := *step
	repairOf.Input = cloneStepPayload(step.Input)
	repairOf.Output = cloneStepPayload(step.Output)
	repairOf.State = common.StepStateFailed
	repairOf.Output["convertedDecision"] = convertedDecisionPayload(decisionSpec)
	repairOf.Output["continuableDecisionConvertedToRepair"] = true
	repairOf.Output["status"] = "failed"
	repairOf.Output["error"] = "decision requested a workspace resource that is already readable"
	attempt := o.repairManager().runtimeRepairAttemptCountForStep(ctx, run.ID, &repairOf) + 1
	stepID, err = uniqueDispatchStepID(ctx, o.steps, run.ID, runtimeRepairFollowUpStepID(run.ID, &repairOf, attempt))
	if err != nil {
		return nil, true, err
	}
	return []stepDispatchSpec{{
		ID:    stepID,
		Kind:  initialStepKind(run),
		Input: o.runtimeRepairFollowUpStepInput(ctx, run, &repairOf, attempt, common.ErrInvalidArgument),
	}}, true, nil
}

func (o *RuntimeOrchestrator) nextDispatchSpecsFromStep(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) ([]stepDispatchSpec, error) {
	if run == nil || last == nil {
		return nil, common.ErrInvalidArgument
	}
	if last.State == common.StepStateCompleted {
		specs, steps := o.graphManager().readyGraphStepSpecsWithSteps(ctx, run, last)
		if len(specs) > 0 {
			return specs, nil
		}
		nextStepID, nextKind := nextStepSpec(last.Output)
		if nextStepID != "" {
			uniqueStepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, nextStepID)
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    uniqueStepID,
				Kind:  nextKind,
				Input: nextStepInputFromOutput(run, last),
			}}, nil
		}
		if shouldDispatchGraphFollowUp(run, last, last.Output) {
			var input map[string]any
			if len(steps) > 0 {
				input = o.graphManager().graphFollowUpStepInputFromSteps(run, last, steps)
			} else {
				input = o.graphManager().graphFollowUpStepInput(ctx, run, last)
			}
			stepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, graphFollowUpStepID(run.ID, last))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: input,
			}}, nil
		}
		if shouldDispatchLLMCompletionFollowUp(run, last, last.Output) {
			stepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, llmCompletionFollowUpStepID(run.ID, last))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: o.graphManager().llmCompletionFollowUpStepInput(ctx, run, last),
			}}, nil
		}
		if shouldDispatchNoActionableContinuationFollowUp(run, last, last.Output) {
			stepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, noActionableContinuationFollowUpStepID(run.ID, last))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: o.graphManager().noActionableContinuationFollowUpStepInput(ctx, run, last),
			}}, nil
		}
	}
	if last.State == common.StepStateDispatched {
		return []stepDispatchSpec{{
			ID:    last.ID,
			Kind:  last.Kind,
			Input: cloneStepPayload(last.Input),
		}}, nil
	}
	if last.State == common.StepStateFailed && requiresDoDAssessmentCompletionGate(run) {
		action := o.recoveryActionForFailedStep(ctx, run, last)
		switch action {
		case runtimeRecoveryActionProviderRetry:
			spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, last, nil, action, maxProviderRetryAttemptsForPolicy(o.policy), o, false)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, common.ErrConflict
			}
			return []stepDispatchSpec{spec}, nil
		case runtimeRecoveryActionRuntimeRepair:
			spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, last, nil, action, maxProviderRetryAttemptsForPolicy(o.policy), o, false)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, common.ErrConflict
			}
			return []stepDispatchSpec{spec}, nil
		case runtimeRecoveryActionProviderRetryExhausted,
			runtimeRecoveryActionRuntimeRepairExhausted,
			runtimeRecoveryActionDuplicateFailure,
			runtimeRecoveryActionContextIntegrityBlocked:
			if err := o.blockRecoveryActionFromFailedStep(ctx, run, last, nil, action); err != nil {
				return nil, err
			}
			return nil, common.ErrBlocked
		default:
			return nil, common.ErrConflict
		}
	}
	return nil, common.ErrConflict
}

func uniqueDispatchStepID(ctx context.Context, steps repository.StepRepository, runID common.RunID, base common.StepID) (common.StepID, error) {
	if steps == nil || runID == "" || base == "" {
		return base, nil
	}
	existing, err := steps.Get(ctx, base)
	if errors.Is(err, common.ErrNotFound) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	if existing != nil && existing.RunID != runID {
		return "", common.ErrConflict
	}
	for idx := 2; ; idx++ {
		candidate := common.StepID(fmt.Sprintf("%s-%d", base, idx))
		existing, err := steps.Get(ctx, candidate)
		if errors.Is(err, common.ErrNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		if existing != nil && existing.RunID != runID {
			return "", common.ErrConflict
		}
	}
}

func terminalExecutionStep(steps []*runModel.Step) *runModel.Step {
	if len(steps) == 0 {
		return nil
	}
	referenced := map[common.StepID]bool{}
	for _, step := range steps {
		if previousStepID := previousExecutionStepID(step); previousStepID != "" {
			referenced[previousStepID] = true
		}
	}
	var terminals []*runModel.Step
	for _, step := range steps {
		if step == nil || referenced[step.ID] {
			continue
		}
		terminals = append(terminals, step)
	}
	if len(terminals) == 0 {
		return nil
	}
	if len(terminals) == 1 {
		return terminals[0]
	}
	for _, step := range terminals {
		if step.State == common.StepStateDispatched {
			return step
		}
	}
	if graphStep := latestCompletedGraphStep(terminals); graphStep != nil {
		return graphStep
	}
	return terminals[len(terminals)-1]
}

func previousExecutionStepID(step *runModel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	for _, key := range []string{
		"previousStepId",
		stepInputKeyRepairOfStepID,
		"failedStepId",
	} {
		previousStepID := strings.TrimSpace(stringFromMap(step.Input, key))
		if previousStepID != "" && common.StepID(previousStepID) != step.ID {
			return common.StepID(previousStepID)
		}
	}
	return ""
}

func (o *RuntimeOrchestrator) dispatchStepSpecsWithWorkspace(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec, workspace *workspaceModel.Workspace) error {
	if len(specs) == 0 {
		return common.ErrConflict
	}
	if len(specs) == 1 {
		return o.dispatchStepWithWorkspace(ctx, run, specs[0].ID, specs[0].Kind, specs[0].Input, workspace)
	}
	return o.dispatchGraphStepBatchWithWorkspace(ctx, run, specs, workspace)
}

func (o *RuntimeOrchestrator) dispatchUniqueStepWithWorkspace(
	ctx context.Context,
	run *runModel.Run,
	baseStepID common.StepID,
	kind string,
	input map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	stepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, baseStepID)
	if err != nil {
		return err
	}
	return o.dispatchStepWithWorkspace(ctx, run, stepID, kind, input, workspace)
}

func mergeResumeInput(specs []stepDispatchSpec, resumeInput map[string]any) {
	if len(resumeInput) == 0 {
		return
	}
	for idx := range specs {
		if specs[idx].Input == nil {
			specs[idx].Input = map[string]any{}
		}
		for key, value := range resumeInput {
			if resumeInputShouldPreserveExistingStage(key, specs[idx].Input) {
				continue
			}
			specs[idx].Input[key] = value
		}
	}
}

func resumeInputShouldPreserveExistingStage(key string, input map[string]any) bool {
	if resumeInputContractKey(key) {
		return true
	}
	switch strings.TrimSpace(key) {
	case stepInputKeySoulStage, stepInputKeyRuntimeStage, stepInputKeyRuntimeStageMode:
		return strings.TrimSpace(stringFromMap(input, key)) != ""
	default:
		return false
	}
}

func resumeInputContractKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "repairObjective",
		"stateChangingOutputRequired",
		"repairGuidance",
		"repairActionPackage",
		"convergenceContract",
		"concreteFailureEvidenceRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		stepInputKeyAssessmentOnly,
		"assessmentOnlyReason",
		"outputMode",
		"outputModeSelector",
		"artifactOutputContract",
		"artifactContinuationContext",
		"artifactContinuationId":
		return true
	default:
		return false
	}
}
