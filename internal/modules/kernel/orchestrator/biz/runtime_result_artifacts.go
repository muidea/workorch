package orchestrator

import (
	"context"
	"fmt"
	"strings"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
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
		Name:         "Workspace Change Diff",
		Summary:      patchDiffArtifactContent(patchDetails),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/workspace-change-diff", run.ID),
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

func (o *RuntimeOrchestrator) recordWebSourceIndexArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	sourceDetails []resultWebSourceDetail,
	now string,
) (common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil || len(sourceDetails) == 0 {
		return "", nil
	}
	evidenceRefs := resultWebSourceEvidenceRefs(sourceDetails)
	item := &resultmodel.Artifact{
		ID:           webSourceIndexArtifactID(run),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypeReport,
		Category:     resultcommon.ArtifactCategorySource,
		Name:         "Web Source Index",
		Summary:      webSourceIndexArtifactContent(sourceDetails),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/web-source-index", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: evidenceRefs,
		Metadata: map[string]any{
			"sourceType":   resultcommon.SourceTypeDoDAssessment,
			"sourceRef":    string(step.ID),
			"category":     resultcommon.ArtifactCategorySource,
			"artifactKind": "web_source_index",
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

func (o *RuntimeOrchestrator) resultWebSourceDetails(ctx context.Context, runID common.RunID) []resultWebSourceDetail {
	if o == nil || o.steps == nil || runID == "" {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	ret := make([]resultWebSourceDetail, 0)
	for _, step := range steps {
		if step == nil || len(step.Output) == 0 {
			continue
		}
		switch step.Kind {
		case webcommon.StepKindSearch:
			ret = append(ret, resultWebSearchDetails(step)...)
		case webcommon.StepKindFetch:
			if detail, ok := resultWebFetchDetail(step); ok {
				ret = append(ret, detail)
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func resultWebSearchDetails(step *runModel.Step) []resultWebSourceDetail {
	results := mapSliceFromValue(step.Output["results"])
	if len(results) == 0 {
		return nil
	}
	ret := make([]resultWebSourceDetail, 0, len(results))
	for _, result := range results {
		url := stringFromMap(result, "url")
		if url == "" {
			continue
		}
		ret = append(ret, resultWebSourceDetail{
			StepID:         step.ID,
			Kind:           step.Kind,
			ProviderID:     stringFromMap(step.Output, "provider"),
			Query:          stringFromMap(step.Output, "query"),
			SearchedAt:     stringFromMap(step.Output, "searchedAt"),
			Title:          stringFromMap(result, "title"),
			URL:            url,
			Source:         stringFromMap(result, "source"),
			PublishedAt:    stringFromMap(result, "publishedAt"),
			Rank:           int(int64FromValue(result["rank"])),
			ContentHash:    stringFromMap(result, "contentHash"),
			PolicyDecision: resultPolicyDecision(step.Output),
		})
	}
	return ret
}

func resultWebFetchDetail(step *runModel.Step) (resultWebSourceDetail, bool) {
	url := stringFromMap(step.Output, "url")
	finalURL := stringFromMap(step.Output, "finalUrl")
	if url == "" && finalURL == "" {
		return resultWebSourceDetail{}, false
	}
	return resultWebSourceDetail{
		StepID:         step.ID,
		Kind:           step.Kind,
		ProviderID:     stringFromMap(step.Output, "provider"),
		URL:            url,
		FinalURL:       finalURL,
		Title:          stringFromMap(step.Output, "title"),
		Source:         stringFromMap(step.Output, "source"),
		RetrievedAt:    stringFromMap(step.Output, "retrievedAt"),
		PublishedAt:    stringFromMap(step.Output, "publishedAt"),
		ContentHash:    stringFromMap(step.Output, "contentHash"),
		Truncated:      boolFromValue(step.Output["truncated"]),
		PolicyDecision: resultPolicyDecision(step.Output),
	}, true
}

func resultPolicyDecision(output map[string]any) string {
	policy := mapFromValue(output["policy"])
	return stringFromMap(policy, "decision")
}

func resultWebSourceEvidenceRefs(sourceDetails []resultWebSourceDetail) []string {
	ret := make([]string, 0, len(sourceDetails))
	for _, item := range sourceDetails {
		if item.StepID != "" {
			ret = append(ret, string(item.StepID))
		}
	}
	return compactUniqueStrings(ret)
}

func webSourceIndexArtifactID(run *runModel.Run) common.ArtifactID {
	if run == nil {
		return ""
	}
	return common.ArtifactID(fmt.Sprintf("%s/artifact/web-source-index", run.ID))
}

func webSourceIndexArtifactContent(sourceDetails []resultWebSourceDetail) string {
	var builder strings.Builder
	builder.WriteString("# Web Source Index\n\n")
	builder.WriteString("| Evidence | Kind | Source | URL | Retrieved | Policy |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, item := range sourceDetails {
		source := firstNonEmptyOrchestratorString(item.Source, item.Title, item.ProviderID)
		sourceURL := firstNonEmptyOrchestratorString(item.FinalURL, item.URL)
		retrieved := firstNonEmptyOrchestratorString(item.RetrievedAt, item.SearchedAt, item.PublishedAt)
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | %s | %s | %s | %s |\n",
			markdownInlineCode(string(item.StepID)),
			markdownInlineCode(item.Kind),
			markdownTableCell(source),
			markdownTableCell(sourceURL),
			markdownTableCell(retrieved),
			markdownTableCell(item.PolicyDecision),
		))
	}
	return strings.TrimSpace(builder.String())
}

type resultWebSourceDetail struct {
	StepID         common.StepID
	Kind           string
	ProviderID     string
	Query          string
	SearchedAt     string
	Title          string
	URL            string
	FinalURL       string
	Source         string
	PublishedAt    string
	RetrievedAt    string
	ContentHash    string
	Rank           int
	Truncated      bool
	PolicyDecision string
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
