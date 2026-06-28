package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (r *resultSubsystem) recordRunResultFromDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	if r == nil || run == nil || step == nil || r.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) {
		return nil
	}
	if err := ensureRunCanRecordFormalResult(run); err != nil {
		return err
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	return r.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace)
}

func (r *resultSubsystem) recordRejectedCompletionDoDAssessment(ctx context.Context, run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if err := ensureRunCanRecordFormalResult(run); err != nil {
		return err
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		assessment = syntheticRejectedDoDAssessment(run, step, output)
	}
	if satisfied, _ := assessment["satisfied"].(bool); satisfied {
		rejected := cloneStepPayload(assessment)
		rejected["satisfied"] = false
		if summary := strings.TrimSpace(stringFromMap(rejected, "summary")); summary != "" {
			rejected["sourceSummary"] = summary
		}
		if reason := strings.TrimSpace(syntheticRejectedDoDReason(step, output)); reason != "" {
			rejected["rejectionReason"] = reason
			rejected["gateFailureReason"] = reason
			if goalOutcome := strings.TrimSpace(stringFromMap(rejected, "goalOutcome")); goalOutcome != "" {
				rejected["sourceGoalOutcome"] = goalOutcome
			}
			rejected["goalOutcome"] = reason
		}
		if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
			rejected["runtimeDiagnostics"] = cloneStepPayload(diagnostics)
		}
		rejected["summary"] = "DoD assessment rejected by runtime completion gate."
		assessment = rejected
	}
	summary, _ := assessment["summary"].(string)
	return r.recordRunResultFromRejectedDoDAssessment(ctx, run, step, assessment, summary)
}

func syntheticRejectedDoDAssessment(run *runModel.Run, step *runModel.Step, output map[string]any) map[string]any {
	reason := syntheticRejectedDoDReason(step, output)
	stepRef := ""
	if step != nil {
		stepRef = string(step.ID)
	}
	item := map[string]any{
		"item":          "runtime execution contract",
		"state":         "failed",
		"evidenceRefs":  []string{stepRef},
		"reason":        reason,
		"failureReason": reason,
	}
	assessment := map[string]any{
		"satisfied":   false,
		"summary":     reason,
		"goalOutcome": reason,
		"items":       []map[string]any{item},
		"goalCoverage": map[string]any{
			"satisfied": false,
			"items": []map[string]any{{
				"goal":         "current Goal requirements",
				"state":        "failed",
				"evidenceRefs": []string{stepRef},
				"reason":       reason,
			}},
		},
	}
	if run != nil && strings.TrimSpace(string(run.GoalID)) != "" {
		assessment["goalId"] = string(run.GoalID)
	}
	return assessment
}

func syntheticRejectedDoDReason(step *runModel.Step, output map[string]any) string {
	for _, key := range []string{"error", "reason", "summary"} {
		if reason := strings.TrimSpace(stringFromMap(output, key)); reason != "" {
			return reason
		}
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		if reason := strings.TrimSpace(stringFromMap(diagnostics, "reason")); reason != "" {
			return reason
		}
		if kind := strings.TrimSpace(stringFromMap(diagnostics, "kind")); kind != "" {
			return "runtime failure: " + kind
		}
	}
	if step != nil && step.ID != "" {
		return fmt.Sprintf("runtime step %s failed before producing an acceptable DoD assessment", step.ID)
	}
	return "runtime failed before producing an acceptable DoD assessment"
}

func (r *resultSubsystem) recordRunResultFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
	workspace *workspaceModel.Workspace,
) error {
	if r == nil || run == nil || step == nil || r.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	if err := ensureRunCanRecordFormalResult(run); err != nil {
		return err
	}
	if err := ensureRunCoverageReadyForFinalResult(run); err != nil {
		return err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	now := runtimeEvidenceTimestamp()
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	results := r
	patchDetails := results.resultPatchDetails(ctx, run.ID)
	assessment, summary = resultAssessmentWithRuntimeFacts(run, assessment, summary, patchDetails)
	evidenceRefs = doDAssessmentEvidenceRefs(step.ID, assessment)
	if err := r.ensureAcceptedOutputEvidenceRefs(ctx, run.ID, "completion result", evidenceRefs); err != nil {
		return err
	}
	webSourceDetails := results.resultWebSourceDetailsForEvidenceRefs(ctx, run.ID, evidenceRefs)
	gitOperationDetails := results.resultGitOperationDetails(ctx, run.ID)
	expectedArtifactRefs := expectedCompletionArtifactRefs(run, patchDetails, webSourceDetails, gitOperationDetails)
	manifestCh := make(chan string, 1)
	go func() {
		manifestCh <- r.workspaceDeliverableManifestContent(ctx, run, step, assessment, evidenceRefs, workspace, patchDetails)
	}()
	type artifactWriteResult struct {
		refs []common.ArtifactID
		err  error
	}
	summaryArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		refs, err := results.recordResultSummaryArtifact(ctx, run, step, summary, assessment, evidenceRefs, expectedArtifactRefs, now)
		summaryArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	manifest := <-manifestCh
	manifestArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		ref, err := results.recordWorkspaceDeliverableManifestArtifact(ctx, run, step, manifest, evidenceRefs, now)
		var refs []common.ArtifactID
		if ref != "" {
			refs = []common.ArtifactID{ref}
		}
		manifestArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	patchArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		refs, err := results.recordPatchDiffArtifact(ctx, run, step, patchDetails, now)
		patchArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	webSourceArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		ref, err := results.recordWebSourceIndexArtifact(ctx, run, step, webSourceDetails, now)
		var refs []common.ArtifactID
		if ref != "" {
			refs = []common.ArtifactID{ref}
		}
		webSourceArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	gitOperationArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		ref, err := results.recordGitOperationIndexArtifact(ctx, run, step, gitOperationDetails, now)
		var refs []common.ArtifactID
		if ref != "" {
			refs = []common.ArtifactID{ref}
		}
		gitOperationArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	summaryArtifact := <-summaryArtifactCh
	if summaryArtifact.err != nil {
		<-manifestArtifactCh
		<-patchArtifactCh
		<-webSourceArtifactCh
		<-gitOperationArtifactCh
		return summaryArtifact.err
	}
	manifestArtifact := <-manifestArtifactCh
	if manifestArtifact.err != nil {
		<-patchArtifactCh
		<-webSourceArtifactCh
		<-gitOperationArtifactCh
		return manifestArtifact.err
	}
	patchArtifact := <-patchArtifactCh
	if patchArtifact.err != nil {
		<-webSourceArtifactCh
		<-gitOperationArtifactCh
		return patchArtifact.err
	}
	webSourceArtifact := <-webSourceArtifactCh
	if webSourceArtifact.err != nil {
		<-gitOperationArtifactCh
		return webSourceArtifact.err
	}
	gitOperationArtifact := <-gitOperationArtifactCh
	if gitOperationArtifact.err != nil {
		return gitOperationArtifact.err
	}
	artifactRefs := append([]common.ArtifactID(nil), summaryArtifact.refs...)
	artifactRefs = append(artifactRefs, manifestArtifact.refs...)
	artifactRefs = append(artifactRefs, patchArtifact.refs...)
	artifactRefs = append(artifactRefs, webSourceArtifact.refs...)
	artifactRefs = append(artifactRefs, gitOperationArtifact.refs...)
	dodAssessment := resultDoDAssessmentFromMap(assessment)
	item := &resultmodel.Result{
		ID:            common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		WorkspaceID:   run.WorkspaceID,
		RunID:         run.ID,
		GoalID:        run.GoalID,
		DoDID:         run.DoDID,
		State:         common.ResultStateAccepted,
		Title:         "Accepted Result",
		Summary:       summary,
		SourceType:    resultcommon.SourceTypeDoDAssessment,
		SourceRef:     string(step.ID),
		EvidenceRefs:  evidenceRefs,
		ArtifactRefs:  artifactRefs,
		DoDAssessment: dodAssessment,
		SummaryDetail: resultSummaryFromDoDAssessment(run, summary, assessment, dodAssessment, evidenceRefs, artifactRefs),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	item.SummaryDetail = finalCompletionResultSummary(item.SummaryDetail)
	if err := r.evidence.Results.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := r.evidence.Results.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.GoalID = item.GoalID
		existing.DoDID = item.DoDID
		existing.State = item.State
		existing.Title = item.Title
		existing.Summary = item.Summary
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.EvidenceRefs = item.EvidenceRefs
		existing.ArtifactRefs = item.ArtifactRefs
		existing.DoDAssessment = item.DoDAssessment
		existing.SummaryDetail = item.SummaryDetail
		existing.UpdatedAt = now
		if err := r.evidence.Results.Update(ctx, existing); err != nil {
			return err
		}
		return r.publishResultEvent(ctx, existing)
	}
	return r.publishResultEvent(ctx, item)
}

func (o *RuntimeOrchestrator) recordCoverageProgressFromCompletionAssessment(ctx context.Context, run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if o == nil || run == nil || step == nil || !requiresDoDAssessmentCompletionGate(run) {
		return nil
	}
	assessment := doDAssessmentFromOutput(output)
	if len(assessment) == 0 {
		return nil
	}
	if _, _, err := validatedCompletionDoDAssessmentForStep(run, step, output); err == nil {
		return nil
	} else {
		var violation runtimeContractViolationError
		if errors.As(err, &violation) {
			return nil
		}
	}
	if err := validateDoDAssessmentItems(assessment["items"]); err != nil {
		return nil
	}
	if err := validateDoDAssessmentEvidenceQuality(ctx, o.steps, o.evidence, run, assessment); err != nil {
		return err
	}
	if o.coverageLedger().updateFromAssessment(run, step, assessment, ctx) && o.runs != nil {
		if err := o.persistRunProgressUpdate(ctx, run); err != nil {
			return err
		}
	}
	return o.resultManager().recordAcceptedSliceResultsFromRun(ctx, run, step)
}

func (r *resultSubsystem) recordAcceptedSliceResultsFromRun(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if r == nil || run == nil || step == nil || r.evidence.Results == nil {
		return nil
	}
	if err := ensureRunCanRecordFormalResult(run); err != nil {
		return err
	}
	slices := mapSliceFromValue(run.ExecutionContext[acceptanceSlicesKey])
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(slices) == 0 || len(items) == 0 {
		return nil
	}
	itemsByID := map[string]map[string]any{}
	for _, item := range items {
		id := strings.TrimSpace(stringFromMap(item, "id"))
		if id != "" {
			itemsByID[id] = item
		}
	}
	for _, slice := range slices {
		if !acceptedAcceptanceSliceAllowsResult(slice) {
			continue
		}
		if err := r.recordAcceptedSliceResult(ctx, run, step, slice, itemsByID); err != nil {
			return err
		}
	}
	return nil
}

func acceptedAcceptanceSliceAllowsResult(slice map[string]any) bool {
	if strings.TrimSpace(stringFromMap(slice, "state")) != "accepted" {
		return false
	}
	policy := strings.TrimSpace(stringFromMap(slice, "resultPolicy"))
	return policy == "" || policy == "slice_result_allowed" || policy == "accepted_slice_result"
}

func (r *resultSubsystem) recordAcceptedSliceResult(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	slice map[string]any,
	itemsByID map[string]map[string]any,
) error {
	sliceID := strings.TrimSpace(stringFromMap(slice, "id"))
	if sliceID == "" {
		return nil
	}
	workItemIDs := stringSliceFromValue(slice["coverageWorkItemIds"])
	if len(workItemIDs) == 0 {
		return nil
	}
	var assessmentItems []resultmodel.DoDAssessmentItem
	var evidenceRefs []string
	for _, id := range workItemIDs {
		item := itemsByID[id]
		if len(item) == 0 {
			continue
		}
		refs := stringSliceFromValue(item["acceptedEvidenceRefs"])
		evidenceRefs = append(evidenceRefs, refs...)
		assessmentItems = append(assessmentItems, resultmodel.DoDAssessmentItem{
			Item:         firstNonEmptyString(stringFromMap(item, "targetText"), id),
			State:        firstNonEmptyString(stringFromMap(item, "status"), "satisfied"),
			EvidenceRefs: refs,
			SourceNote:   firstNonEmptyString(stringFromMap(item, "sourceRequirementId"), stringFromMap(item, "source"), id),
			Confidence:   1,
		})
	}
	evidenceRefs = compactUniqueStrings(append(evidenceRefs, string(step.ID)))
	if len(assessmentItems) == 0 || len(evidenceRefs) == 0 {
		return nil
	}
	if err := r.ensureAcceptedOutputEvidenceRefs(ctx, run.ID, "acceptance slice result", evidenceRefs); err != nil {
		return err
	}
	title := strings.TrimSpace(stringFromMap(slice, "title"))
	if title == "" {
		title = sliceID
	}
	summary := fmt.Sprintf("Acceptance slice %s accepted.", title)
	now := runtimeEvidenceTimestamp()
	resultID := common.ResultID(fmt.Sprintf("%s/result/acceptance-slice/%s", run.ID, acceptanceSliceResultIDPart(sliceID)))
	item := &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		GoalID:       run.GoalID,
		DoDID:        run.DoDID,
		State:        common.ResultStateAccepted,
		Title:        "Accepted Slice: " + title,
		Summary:      summary,
		SourceType:   resultcommon.SourceTypeAcceptanceSlice,
		SourceRef:    string(step.ID),
		EvidenceRefs: evidenceRefs,
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: true,
			Items:     assessmentItems,
			Summary:   summary,
		},
		SummaryDetail: resultmodel.ResultSummary{
			GoalOutcome:  summary,
			DoDItems:     assessmentItems,
			EvidenceRefs: evidenceRefs,
			Diagnostics: map[string]any{
				"resultScope":              "acceptance_slice",
				"sliceId":                  sliceID,
				"sliceState":               stringFromMap(slice, "state"),
				"sliceScope":               stringFromMap(slice, "scope"),
				"coverageWorkItemIds":      workItemIDs,
				"requiredEvidenceKinds":    stringSliceFromValue(slice["requiredEvidenceKinds"]),
				"finalCompletionGateState": "not_final_result",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.evidence.Results.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := r.evidence.Results.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.GoalID = item.GoalID
		existing.DoDID = item.DoDID
		existing.State = item.State
		existing.Title = item.Title
		existing.Summary = item.Summary
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.EvidenceRefs = item.EvidenceRefs
		existing.DoDAssessment = item.DoDAssessment
		existing.SummaryDetail = item.SummaryDetail
		existing.UpdatedAt = now
		if err := r.evidence.Results.Update(ctx, existing); err != nil {
			return err
		}
		return r.publishResultEvent(ctx, existing)
	}
	return r.publishResultEvent(ctx, item)
}

func acceptanceSliceResultIDPart(sliceID string) string {
	sliceID = strings.TrimSpace(sliceID)
	if sliceID == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", "\t", "_", "\n", "_")
	return replacer.Replace(sliceID)
}

func (r *resultSubsystem) recordRunResultFromRejectedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
) error {
	if r == nil || run == nil || step == nil || r.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	if err := ensureRunCanRecordFormalResult(run); err != nil {
		return err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "DoD assessment was not satisfied."
	}
	assessment = rejectedDoDAssessmentWithFallbackEvidenceRefs(step.ID, assessment)
	now := runtimeEvidenceTimestamp()
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	sourceRef, evidenceRefs := rejectedResultSourceAndEvidenceRefs(step, evidenceRefs)
	dodAssessment := resultDoDAssessmentFromMap(assessment)
	item := &resultmodel.Result{
		ID:            common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		WorkspaceID:   run.WorkspaceID,
		RunID:         run.ID,
		GoalID:        run.GoalID,
		DoDID:         run.DoDID,
		State:         common.ResultStateRejected,
		Title:         "Rejected Result",
		Summary:       summary,
		SourceType:    resultcommon.SourceTypeDoDAssessment,
		SourceRef:     sourceRef,
		EvidenceRefs:  evidenceRefs,
		DoDAssessment: dodAssessment,
		SummaryDetail: resultSummaryFromDoDAssessment(run, summary, assessment, dodAssessment, evidenceRefs, nil),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := r.evidence.Results.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := r.evidence.Results.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.State = item.State
		existing.Title = item.Title
		existing.Summary = item.Summary
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.EvidenceRefs = item.EvidenceRefs
		existing.DoDAssessment = item.DoDAssessment
		existing.SummaryDetail = item.SummaryDetail
		existing.UpdatedAt = now
		if err := r.evidence.Results.Update(ctx, existing); err != nil {
			return err
		}
		return r.publishResultEvent(ctx, existing)
	}
	return r.publishResultEvent(ctx, item)
}

func rejectedResultSourceAndEvidenceRefs(step *runModel.Step, evidenceRefs []string) (string, []string) {
	if step == nil {
		return "", evidenceRefs
	}
	sourceRef := string(step.ID)
	rootStepID := common.StepID("")
	switch {
	case boolValue(step.Input[stepInputKeyProviderRetryFollowUp]):
		rootStepID = providerRetryRootStepID(step)
	case boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]):
		rootStepID = runtimeRepairRootStepID(step)
	}
	if rootStepID == "" || rootStepID == step.ID {
		return sourceRef, evidenceRefs
	}
	seen := map[string]bool{}
	for _, ref := range evidenceRefs {
		if strings.TrimSpace(ref) != "" {
			seen[ref] = true
		}
	}
	for _, ref := range []string{
		string(rootStepID),
		string(step.ID),
		stringFromMap(step.Input, "failedStepId"),
		stringFromMap(step.Input, "previousStepId"),
	} {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		evidenceRefs = append(evidenceRefs, ref)
		seen[ref] = true
	}
	sort.Strings(evidenceRefs)
	return sourceRef, evidenceRefs
}

func (r *resultSubsystem) rejectedRunResultSummary(ctx context.Context, runID common.RunID) (string, bool) {
	if r == nil || r.evidence.Results == nil || runID == "" {
		return "", false
	}
	results, err := r.evidence.Results.ListByRun(ctx, runID)
	if err != nil {
		return "", false
	}
	for _, result := range results {
		if result == nil || result.State != common.ResultStateRejected {
			continue
		}
		if r.rejectedResultSupersededByRecovery(ctx, runID, result) {
			_ = r.supersedeRejectedResult(ctx, result)
			continue
		}
		if result.State == common.ResultStateRejected {
			return strings.TrimSpace(result.Summary), true
		}
	}
	return "", false
}

func (r *resultSubsystem) hasAcceptedCompletionResult(ctx context.Context, run *runModel.Run) bool {
	if r == nil || r.evidence.Results == nil || run == nil || run.ID == "" {
		return false
	}
	results, err := r.evidence.Results.ListByRun(ctx, run.ID)
	if err != nil {
		return false
	}
	for _, result := range results {
		if acceptedCompletionResultMatchesRun(result, run) {
			return true
		}
	}
	return false
}

func acceptedCompletionResultMatchesRun(result *resultmodel.Result, run *runModel.Run) bool {
	if result == nil || run == nil {
		return false
	}
	if result.State != common.ResultStateAccepted ||
		result.SourceType != resultcommon.SourceTypeDoDAssessment ||
		result.WorkspaceID != run.WorkspaceID ||
		result.RunID != run.ID {
		return false
	}
	if run.GoalID != "" && result.GoalID != run.GoalID {
		return false
	}
	if run.DoDID != "" && result.DoDID != run.DoDID {
		return false
	}
	if !result.DoDAssessment.Satisfied || strings.TrimSpace(result.Summary) == "" {
		return false
	}
	for _, ref := range result.EvidenceRefs {
		if strings.TrimSpace(ref) != "" {
			return true
		}
	}
	return false
}

func (r *resultSubsystem) rejectedResultSupersededByRecovery(ctx context.Context, runID common.RunID, result *resultmodel.Result) bool {
	if r == nil || r.steps == nil || result == nil || runID == "" {
		return false
	}
	sourceRef := common.StepID(strings.TrimSpace(result.SourceRef))
	if sourceRef == "" {
		return false
	}
	steps, err := r.steps.ListByRun(ctx, runID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		if !recoveryStepSupersedesRejectedSource(step, sourceRef) {
			continue
		}
		return true
	}
	return false
}

func recoveryStepSupersedesRejectedSource(step *runModel.Step, sourceRef common.StepID) bool {
	if step == nil || sourceRef == "" || step.ID == sourceRef {
		return false
	}
	switch step.State {
	case common.StepStateDispatched, common.StepStateCompleted:
	default:
		return false
	}
	if !boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) &&
		!boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) &&
		!boolValue(step.Input["decisionAuthorizedRetry"]) {
		return false
	}
	for _, key := range []string{
		"failedStepId",
		stepInputKeyRepairOfStepID,
		stepInputKeyProviderRetryRootStep,
		"previousStepId",
	} {
		if common.StepID(strings.TrimSpace(stringFromMap(step.Input, key))) == sourceRef {
			return true
		}
	}
	return false
}

func (r *resultSubsystem) supersedeRejectedResult(ctx context.Context, result *resultmodel.Result) error {
	if r == nil || r.evidence.Results == nil || result == nil || result.State != common.ResultStateRejected {
		return nil
	}
	result.State = common.ResultStateSuperseded
	result.Title = "Superseded Rejected Result"
	result.UpdatedAt = runtimeEvidenceTimestamp()
	return r.evidence.Results.Update(ctx, result)
}

func (r *resultSubsystem) recordCompletionEvidenceFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
	workspace *workspaceModel.Workspace,
) error {
	if r == nil || run == nil || step == nil || assessment == nil {
		return nil
	}
	if err := ensureRunCanRecordFormalResult(run); err != nil {
		return err
	}
	if err := r.ensureAcceptedOutputEvidenceRefs(ctx, run.ID, "completion evidence", doDAssessmentEvidenceRefs(step.ID, assessment)); err != nil {
		return err
	}
	return runParallel(
		func() error {
			return r.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace)
		},
		func() error {
			return r.recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
		},
		func() error {
			return r.recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
		},
		func() error {
			return r.recordValidationCommandMemoriesFromValidatedDoDAssessment(ctx, run, step, assessment)
		},
	)
}

func ensureRunCanRecordFormalResult(run *runModel.Run) error {
	if run == nil {
		return nil
	}
	if run.State == common.RunStateCreated {
		return common.ErrConflict
	}
	return nil
}

func ensureRunCoverageReadyForFinalResult(run *runModel.Run) error {
	if run == nil {
		return nil
	}
	if active := activeCoverageWorkItemIDs(run.ExecutionContext); len(active) > 0 {
		return fmt.Errorf("%w: final result coverage gate not ready: unresolved coverage work items: %s", common.ErrConflict, strings.Join(active, ","))
	}
	if err := newCoverageSubsystem().validateReadyForAcceptance(run); err != nil {
		return fmt.Errorf("%w: final result coverage gate not ready: %v", common.ErrConflict, err)
	}
	return nil
}

func activeCoverageWorkItemIDs(context map[string]any) []string {
	var ret []string
	for _, item := range mapSliceFromValue(context[coverageWorkItemsKey]) {
		status := strings.ToLower(strings.TrimSpace(stringFromMap(item, "status")))
		if !runtimecontract.CoverageWorkItemStatusActive(status) {
			continue
		}
		id := strings.TrimSpace(stringFromMap(item, "id"))
		if id == "" {
			id = strings.TrimSpace(stringFromMap(item, "normalizedSourceKey"))
		}
		if id == "" {
			id = "unknown"
		}
		ret = append(ret, id)
	}
	return compactUniqueStrings(ret)
}

func finalCompletionResultSummary(summary resultmodel.ResultSummary) resultmodel.ResultSummary {
	if summary.Diagnostics == nil {
		summary.Diagnostics = map[string]any{}
	}
	summary.Diagnostics["resultScope"] = "final_completion"
	summary.Diagnostics["finalCompletionGateState"] = "final_result"
	return summary
}
