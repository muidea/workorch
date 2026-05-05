package orchestrator

import (
	"context"
	"fmt"
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) recordResultSummaryArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	summary string,
	assessment map[string]any,
	evidenceRefs []string,
	artifactRefs []common.ArtifactID,
	now string,
) ([]common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil {
		return nil, nil
	}
	item := &resultmodel.Artifact{
		ID:           common.ArtifactID(fmt.Sprintf("%s/artifact/result-summary", run.ID)),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypeText,
		Category:     resultcommon.ArtifactCategoryFinalSummary,
		Name:         "Accepted Result Summary",
		Summary:      resultSummaryArtifactContent(run, summary, assessment, evidenceRefs, artifactRefs, o.resultEvidenceDetails(ctx, evidenceRefs)),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/summary", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: evidenceRefs,
		Metadata: map[string]any{
			"sourceType": resultcommon.SourceTypeDoDAssessment,
			"sourceRef":  string(step.ID),
			"category":   resultcommon.ArtifactCategoryFinalSummary,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := o.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := o.evidence.Artifacts.Get(ctx, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.ResultID = item.ResultID
		existing.StepID = item.StepID
		existing.ArtifactType = item.ArtifactType
		existing.Category = item.Category
		existing.Name = item.Name
		existing.Summary = item.Summary
		existing.URI = item.URI
		existing.ContentType = item.ContentType
		existing.State = item.State
		existing.EvidenceRefs = item.EvidenceRefs
		existing.Metadata = item.Metadata
		existing.UpdatedAt = now
		if err := o.evidence.Artifacts.Update(ctx, existing); err != nil {
			return nil, err
		}
		return []common.ArtifactID{existing.ID}, o.publishArtifactCreated(ctx, existing)
	}
	return []common.ArtifactID{item.ID}, o.publishArtifactCreated(ctx, item)
}

func (o *RuntimeOrchestrator) recordWorkspaceDeliverableManifestArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	manifest string,
	evidenceRefs []string,
	now string,
) (common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil {
		return "", nil
	}
	item := &resultmodel.Artifact{
		ID:           common.ArtifactID(fmt.Sprintf("%s/artifact/deliverable-manifest", run.ID)),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypeReport,
		Category:     resultcommon.ArtifactCategoryRuntimeEvidence,
		Name:         "Workspace Deliverable Manifest",
		Summary:      manifest,
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/deliverable-manifest", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: evidenceRefs,
		Metadata: map[string]any{
			"sourceType":   resultcommon.SourceTypeDoDAssessment,
			"sourceRef":    string(step.ID),
			"manifestType": "workspace_deliverables",
			"category":     resultcommon.ArtifactCategoryRuntimeEvidence,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := o.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return "", err
		}
		existing, getErr := o.evidence.Artifacts.Get(ctx, item.ID)
		if getErr != nil {
			return "", getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.ResultID = item.ResultID
		existing.StepID = item.StepID
		existing.ArtifactType = item.ArtifactType
		existing.Category = item.Category
		existing.Name = item.Name
		existing.Summary = item.Summary
		existing.URI = item.URI
		existing.ContentType = item.ContentType
		existing.State = item.State
		existing.EvidenceRefs = item.EvidenceRefs
		existing.Metadata = item.Metadata
		existing.UpdatedAt = now
		if err := o.evidence.Artifacts.Update(ctx, existing); err != nil {
			return "", err
		}
		return existing.ID, o.publishArtifactCreated(ctx, existing)
	}
	return item.ID, o.publishArtifactCreated(ctx, item)
}

func (o *RuntimeOrchestrator) recordPatchDiffArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	patchDetails []resultPatchDetail,
	now string,
) ([]common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil || !hasPatchDeliverableContent(patchDetails) {
		return nil, nil
	}
	patchIDs := make([]string, 0, len(patchDetails))
	targetRefs := make([]string, 0, len(patchDetails))
	for _, patch := range patchDetails {
		if patch.ID != "" {
			patchIDs = append(patchIDs, string(patch.ID))
		}
		if patch.TargetRef != "" {
			targetRefs = append(targetRefs, patch.TargetRef)
		}
	}
	item := &resultmodel.Artifact{
		ID:           patchDiffArtifactID(run),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypePatch,
		Category:     resultcommon.ArtifactCategoryRuntimeEvidence,
		Name:         "Code Change Diff",
		Summary:      patchDiffArtifactContent(patchDetails),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/code-change-diff", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: patchIDs,
		Metadata: map[string]any{
			"sourceType":   resultcommon.SourceTypeDoDAssessment,
			"sourceRef":    string(step.ID),
			"category":     resultcommon.ArtifactCategoryRuntimeEvidence,
			"artifactKind": "code_change_diff",
			"patchIds":     patchIDs,
			"targetRefs":   targetRefs,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(patchDetails) == 1 {
		item.PatchID = patchDetails[0].ID
	}
	if err := o.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := o.evidence.Artifacts.Get(ctx, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.ResultID = item.ResultID
		existing.StepID = item.StepID
		existing.PatchID = item.PatchID
		existing.ArtifactType = item.ArtifactType
		existing.Category = item.Category
		existing.Name = item.Name
		existing.Summary = item.Summary
		existing.URI = item.URI
		existing.ContentType = item.ContentType
		existing.State = item.State
		existing.EvidenceRefs = item.EvidenceRefs
		existing.Metadata = item.Metadata
		existing.UpdatedAt = now
		if err := o.evidence.Artifacts.Update(ctx, existing); err != nil {
			return nil, err
		}
		return []common.ArtifactID{existing.ID}, o.publishArtifactCreated(ctx, existing)
	}
	return []common.ArtifactID{item.ID}, o.publishArtifactCreated(ctx, item)
}

func resultSummaryFromDoDAssessment(
	run *runModel.Run,
	summary string,
	assessment map[string]any,
	dodAssessment resultmodel.DoDAssessment,
	evidenceRefs []string,
	artifactRefs []common.ArtifactID,
) resultmodel.ResultSummary {
	ret := resultmodel.ResultSummary{
		GoalOutcome: firstNonEmptyOrchestratorString(
			stringFromMap(assessment, "goalOutcome"),
			stringFromMap(assessment, "implementationSummary"),
			summary,
		),
		DoDItems:              append([]resultmodel.DoDAssessmentItem(nil), dodAssessment.Items...),
		ImplementationSummary: firstNonEmptyOrchestratorString(stringFromMap(assessment, "implementationSummary"), summary),
		VerificationSummary:   firstNonEmptyOrchestratorString(stringFromMap(assessment, "verificationSummary"), verificationSummaryFromEvidence(runDialogLanguage(run), evidenceRefs)),
		EvidenceRefs:          append([]string(nil), evidenceRefs...),
		ArtifactRefs:          append([]common.ArtifactID(nil), artifactRefs...),
		KnownLimitations:      stringSliceFromValue(assessment["knownLimitations"]),
		ResidualRisks:         stringSliceFromValue(assessment["residualRisks"]),
		NextActions:           stringSliceFromValue(assessment["nextActions"]),
	}
	if len(ret.ResidualRisks) == 0 {
		ret.ResidualRisks = localizedDefaultList(runDialogLanguage(run), "residual_risks")
	}
	if len(ret.KnownLimitations) == 0 {
		ret.KnownLimitations = localizedDefaultList(runDialogLanguage(run), "known_limitations")
	}
	if len(ret.NextActions) == 0 {
		ret.NextActions = localizedDefaultList(runDialogLanguage(run), "next_actions")
	}
	return ret
}

func (o *RuntimeOrchestrator) resultPatchDetails(ctx context.Context, runID common.RunID) []resultPatchDetail {
	if o == nil || o.evidence.Patches == nil || runID == "" {
		return nil
	}
	patches, err := o.evidence.Patches.ListByRun(ctx, runID)
	if err != nil || len(patches) == 0 {
		return nil
	}
	ret := make([]resultPatchDetail, 0, len(patches))
	for _, patch := range patches {
		if patch == nil {
			continue
		}
		ret = append(ret, resultPatchDetail{
			ID:        patch.ID,
			PatchType: strings.TrimSpace(patch.PatchType),
			TargetRef: strings.TrimSpace(patch.TargetRef),
			Summary:   strings.TrimSpace(patch.Summary),
			State:     patch.State,
			Content:   patch.Content,
			Diff:      patch.Diff,
		})
	}
	return ret
}

func (o *RuntimeOrchestrator) resultEvidenceDetails(ctx context.Context, evidenceRefs []string) []resultEvidenceDetail {
	if o == nil || o.steps == nil || len(evidenceRefs) == 0 {
		return nil
	}
	ret := make([]resultEvidenceDetail, 0, len(evidenceRefs))
	for _, ref := range evidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		step, err := o.steps.Get(ctx, common.StepID(ref))
		if err != nil || step == nil || len(step.Output) == 0 {
			continue
		}
		detail := resultEvidenceDetail{
			Ref:      ref,
			Kind:     step.Kind,
			Command:  strings.TrimSpace(stringFromMap(step.Output, "command")),
			ExitCode: evidenceValueString(step.Output["exitCode"]),
			Stdout:   boundedEvidenceText(stringFromMap(step.Output, "stdout")),
			Stderr:   boundedEvidenceText(stringFromMap(step.Output, "stderr")),
			Summary:  strings.TrimSpace(stringFromMap(step.Output, "summary")),
		}
		if detail.Stdout == "" && detail.Stderr == "" && detail.Summary == "" {
			continue
		}
		ret = append(ret, detail)
	}
	return ret
}
