package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) recordRunResultFromDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	if run == nil || step == nil || o.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) {
		return nil
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
	return o.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace)
}

func (o *RuntimeOrchestrator) recordRunResultFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
	workspace *workspaceModel.Workspace,
) error {
	if run == nil || step == nil || o.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	now := runtimeEvidenceTimestamp()
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	results := o.resultManager()
	patchDetails := results.resultPatchDetails(ctx, run.ID)
	assessment, summary = resultAssessmentWithRuntimeFacts(run, assessment, summary, patchDetails)
	evidenceRefs = doDAssessmentEvidenceRefs(step.ID, assessment)
	webSourceDetails := results.resultWebSourceDetails(ctx, run.ID)
	gitOperationDetails := results.resultGitOperationDetails(ctx, run.ID)
	expectedArtifactRefs := expectedCompletionArtifactRefs(run, patchDetails, webSourceDetails, gitOperationDetails)
	manifestCh := make(chan string, 1)
	go func() {
		manifestCh <- o.workspaceDeliverableManifestContent(ctx, run, step, assessment, evidenceRefs, workspace, patchDetails)
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
	if err := o.evidence.Results.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.Results.Get(ctx, item.ID)
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
		if err := o.evidence.Results.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishResultAccepted(ctx, existing)
	}
	return o.publishResultAccepted(ctx, item)
}

func (o *RuntimeOrchestrator) recordRunResultFromRejectedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
) error {
	if run == nil || step == nil || o.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
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
	if err := o.evidence.Results.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.Results.Get(ctx, item.ID)
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
		if err := o.evidence.Results.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishResultAccepted(ctx, existing)
	}
	return o.publishResultAccepted(ctx, item)
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

func (o *RuntimeOrchestrator) rejectedRunResultSummary(ctx context.Context, runID common.RunID) (string, bool) {
	if o == nil || o.evidence.Results == nil || runID == "" {
		return "", false
	}
	results, err := o.evidence.Results.ListByRun(ctx, runID)
	if err != nil {
		return "", false
	}
	for _, result := range results {
		if result == nil || result.State != common.ResultStateRejected {
			continue
		}
		if o.rejectedResultSupersededByRecovery(ctx, runID, result) {
			_ = o.supersedeRejectedResult(ctx, result)
			continue
		}
		if result.State == common.ResultStateRejected {
			return strings.TrimSpace(result.Summary), true
		}
	}
	return "", false
}

func (o *RuntimeOrchestrator) rejectedResultSupersededByRecovery(ctx context.Context, runID common.RunID, result *resultmodel.Result) bool {
	if o == nil || o.steps == nil || result == nil || runID == "" {
		return false
	}
	sourceRef := common.StepID(strings.TrimSpace(result.SourceRef))
	if sourceRef == "" {
		return false
	}
	steps, err := o.steps.ListByRun(ctx, runID)
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

func (o *RuntimeOrchestrator) supersedeRejectedResult(ctx context.Context, result *resultmodel.Result) error {
	if o == nil || o.evidence.Results == nil || result == nil || result.State != common.ResultStateRejected {
		return nil
	}
	result.State = common.ResultStateSuperseded
	result.Title = "Superseded Rejected Result"
	result.UpdatedAt = runtimeEvidenceTimestamp()
	return o.evidence.Results.Update(ctx, result)
}

func (o *RuntimeOrchestrator) recordCompletionEvidenceFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
	workspace *workspaceModel.Workspace,
) error {
	return runParallel(
		func() error {
			return o.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace)
		},
		func() error {
			return o.recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
		},
		func() error {
			return o.recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
		},
		func() error {
			return o.recordValidationCommandMemoriesFromValidatedDoDAssessment(ctx, run, step, assessment)
		},
	)
}
