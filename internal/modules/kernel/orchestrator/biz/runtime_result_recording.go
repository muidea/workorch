package orchestrator

import (
	"context"
	"fmt"
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
	patchDetails := o.resultPatchDetails(ctx, run.ID)
	expectedArtifactRefs := expectedCompletionArtifactRefs(run, patchDetails)
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
		refs, err := o.recordResultSummaryArtifact(ctx, run, step, summary, assessment, evidenceRefs, expectedArtifactRefs, now)
		summaryArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	manifest := <-manifestCh
	manifestArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		ref, err := o.recordWorkspaceDeliverableManifestArtifact(ctx, run, step, manifest, evidenceRefs, now)
		var refs []common.ArtifactID
		if ref != "" {
			refs = []common.ArtifactID{ref}
		}
		manifestArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	patchArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		refs, err := o.recordPatchDiffArtifact(ctx, run, step, patchDetails, now)
		patchArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	summaryArtifact := <-summaryArtifactCh
	if summaryArtifact.err != nil {
		<-manifestArtifactCh
		<-patchArtifactCh
		return summaryArtifact.err
	}
	manifestArtifact := <-manifestArtifactCh
	if manifestArtifact.err != nil {
		<-patchArtifactCh
		return manifestArtifact.err
	}
	patchArtifact := <-patchArtifactCh
	if patchArtifact.err != nil {
		return patchArtifact.err
	}
	artifactRefs := append([]common.ArtifactID(nil), summaryArtifact.refs...)
	artifactRefs = append(artifactRefs, manifestArtifact.refs...)
	artifactRefs = append(artifactRefs, patchArtifact.refs...)
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
	)
}
