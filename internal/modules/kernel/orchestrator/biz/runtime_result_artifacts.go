package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type resultEventPublisher interface {
	publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) error
	publishResultAccepted(ctx context.Context, item *resultmodel.Result) error
	publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) error
}

type resultSubsystem struct {
	evidence RuntimeEvidenceRepositories
	steps    repository.StepRepository
	events   resultEventPublisher
}

func newResultSubsystem(evidence RuntimeEvidenceRepositories, steps repository.StepRepository, events resultEventPublisher) *resultSubsystem {
	return &resultSubsystem{
		evidence: evidence,
		steps:    steps,
		events:   events,
	}
}

func (r *resultSubsystem) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) error {
	if r == nil || r.events == nil {
		return nil
	}
	return r.events.publishArtifactCreated(ctx, item)
}

func (r *resultSubsystem) publishResultAccepted(ctx context.Context, item *resultmodel.Result) error {
	if r == nil || r.events == nil {
		return nil
	}
	return r.events.publishResultAccepted(ctx, item)
}

func (r *resultSubsystem) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) error {
	if r == nil || r.events == nil {
		return nil
	}
	return r.events.publishMemoryChanged(ctx, topic, item)
}

func (r *resultSubsystem) latestAcceptedResultContext(ctx context.Context, run *runModel.Run) map[string]any {
	if r == nil || run == nil || r.evidence.Results == nil {
		return nil
	}
	results, err := r.evidence.Results.ListByWorkspace(ctx, run.WorkspaceID)
	if err != nil || len(results) == 0 {
		return nil
	}
	sort.SliceStable(results, func(i, j int) bool {
		return resultSortKey(results[i]) > resultSortKey(results[j])
	})
	for _, item := range results {
		if item == nil || item.State != common.ResultStateAccepted {
			continue
		}
		return map[string]any{
			"id":           string(item.ID),
			"runId":        string(item.RunID),
			"goalId":       string(item.GoalID),
			"doDId":        string(item.DoDID),
			"title":        item.Title,
			"summary":      item.Summary,
			"sourceType":   item.SourceType,
			"sourceRef":    item.SourceRef,
			"evidenceRefs": cloneStringsForExecutionContext(item.EvidenceRefs),
			"artifactRefs": artifactIDsForExecutionContext(item.ArtifactRefs),
		}
	}
	return nil
}

func (r *resultSubsystem) currentDeliverableContexts(ctx context.Context, run *runModel.Run) []map[string]any {
	if r == nil || run == nil || r.evidence.Artifacts == nil {
		return nil
	}
	items, err := r.evidence.Artifacts.ListByWorkspace(ctx, run.WorkspaceID)
	if err != nil || len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return artifactSortKey(items[i]) > artifactSortKey(items[j])
	})
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		if item == nil || item.State != common.ArtifactStateAvailable {
			continue
		}
		ret = append(ret, map[string]any{
			"id":           string(item.ID),
			"runId":        string(item.RunID),
			"resultId":     string(item.ResultID),
			"stepId":       string(item.StepID),
			"type":         item.ArtifactType,
			"name":         item.Name,
			"summary":      item.Summary,
			"uri":          item.URI,
			"contentType":  item.ContentType,
			"evidenceRefs": cloneStringsForExecutionContext(item.EvidenceRefs),
		})
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func (r *resultSubsystem) recordResultSummaryArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	summary string,
	assessment map[string]any,
	evidenceRefs []string,
	artifactRefs []common.ArtifactID,
	now string,
) ([]common.ArtifactID, error) {
	if r == nil || run == nil || step == nil || r.evidence.Artifacts == nil {
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
		Summary:      resultSummaryArtifactContent(run, summary, assessment, evidenceRefs, artifactRefs, r.resultEvidenceDetails(ctx, evidenceRefs)),
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
	if err := r.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := r.evidence.Artifacts.Get(ctx, item.ID)
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
		if err := r.evidence.Artifacts.Update(ctx, existing); err != nil {
			return nil, err
		}
		return []common.ArtifactID{existing.ID}, r.publishArtifactCreated(ctx, existing)
	}
	return []common.ArtifactID{item.ID}, r.publishArtifactCreated(ctx, item)
}

func (r *resultSubsystem) recordWorkspaceDeliverableManifestArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	manifest string,
	evidenceRefs []string,
	now string,
) (common.ArtifactID, error) {
	if r == nil || run == nil || step == nil || r.evidence.Artifacts == nil {
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
	if err := r.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return "", err
		}
		existing, getErr := r.evidence.Artifacts.Get(ctx, item.ID)
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
		if err := r.evidence.Artifacts.Update(ctx, existing); err != nil {
			return "", err
		}
		return existing.ID, r.publishArtifactCreated(ctx, existing)
	}
	return item.ID, r.publishArtifactCreated(ctx, item)
}

func (r *resultSubsystem) recordPatchDiffArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	patchDetails []resultPatchDetail,
	now string,
) ([]common.ArtifactID, error) {
	if r == nil || run == nil || step == nil || r.evidence.Artifacts == nil || !hasPatchDeliverableContent(patchDetails) {
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
	if err := r.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := r.evidence.Artifacts.Get(ctx, item.ID)
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
		if err := r.evidence.Artifacts.Update(ctx, existing); err != nil {
			return nil, err
		}
		return []common.ArtifactID{existing.ID}, r.publishArtifactCreated(ctx, existing)
	}
	return []common.ArtifactID{item.ID}, r.publishArtifactCreated(ctx, item)
}

func (r *resultSubsystem) recordWebSourceIndexArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	sourceDetails []resultWebSourceDetail,
	now string,
) (common.ArtifactID, error) {
	if r == nil || run == nil || step == nil || r.evidence.Artifacts == nil || len(sourceDetails) == 0 {
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
	if err := r.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return "", err
		}
		existing, getErr := r.evidence.Artifacts.Get(ctx, item.ID)
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
		if err := r.evidence.Artifacts.Update(ctx, existing); err != nil {
			return "", err
		}
		return existing.ID, r.publishArtifactCreated(ctx, existing)
	}
	return item.ID, r.publishArtifactCreated(ctx, item)
}

func (r *resultSubsystem) recordGitOperationIndexArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	gitDetails []resultGitOperationDetail,
	now string,
) (common.ArtifactID, error) {
	if r == nil || run == nil || step == nil || r.evidence.Artifacts == nil || len(gitDetails) == 0 {
		return "", nil
	}
	evidenceRefs := resultGitOperationEvidenceRefs(gitDetails)
	item := &resultmodel.Artifact{
		ID:           gitOperationIndexArtifactID(run),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypeReport,
		Category:     resultcommon.ArtifactCategoryRuntimeEvidence,
		Name:         "Git Operation Index",
		Summary:      gitOperationIndexArtifactContent(gitDetails),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/git-operation-index", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: evidenceRefs,
		Metadata: map[string]any{
			"sourceType":   resultcommon.SourceTypeDoDAssessment,
			"sourceRef":    string(step.ID),
			"category":     resultcommon.ArtifactCategoryRuntimeEvidence,
			"artifactKind": "git_operation_index",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return "", err
		}
		existing, getErr := r.evidence.Artifacts.Get(ctx, item.ID)
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
		if err := r.evidence.Artifacts.Update(ctx, existing); err != nil {
			return "", err
		}
		return existing.ID, r.publishArtifactCreated(ctx, existing)
	}
	return item.ID, r.publishArtifactCreated(ctx, item)
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
		Diagnostics:           compactResultSummaryDiagnostics(assessment),
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

func compactResultSummaryDiagnostics(assessment map[string]any) map[string]any {
	if len(assessment) == 0 {
		return nil
	}
	ret := map[string]any{}
	if diagnostics := mapFromValue(assessment["runtimeDiagnostics"]); len(diagnostics) > 0 {
		ret["runtimeDiagnostics"] = compactLLMContextMap(diagnostics, 1)
	}
	for _, key := range []string{"sourceSummary", "rejectionReason", "gateFailureReason"} {
		if value := strings.TrimSpace(stringFromMap(assessment, key)); value != "" {
			ret[key] = truncateLLMContextString(value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (r *resultSubsystem) resultPatchDetails(ctx context.Context, runID common.RunID) []resultPatchDetail {
	if r == nil || r.evidence.Patches == nil || runID == "" {
		return nil
	}
	patches, err := r.evidence.Patches.ListByRun(ctx, runID)
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

func (r *resultSubsystem) resultWebSourceDetails(ctx context.Context, runID common.RunID) []resultWebSourceDetail {
	if r == nil || r.steps == nil || runID == "" {
		return nil
	}
	steps, err := r.steps.ListByRun(ctx, runID)
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

func (r *resultSubsystem) resultGitOperationDetails(ctx context.Context, runID common.RunID) []resultGitOperationDetail {
	if r == nil || r.steps == nil || runID == "" {
		return nil
	}
	steps, err := r.steps.ListByRun(ctx, runID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	ret := make([]resultGitOperationDetail, 0)
	for _, step := range steps {
		if step == nil || len(step.Output) == 0 {
			continue
		}
		if step.Kind != gitcommon.StepKindRead && step.Kind != gitcommon.StepKindWrite {
			continue
		}
		commit := mapFromValue(step.Output["commit"])
		ret = append(ret, resultGitOperationDetail{
			StepID:           step.ID,
			Kind:             step.Kind,
			Operation:        stringFromMap(step.Output, "operation"),
			Status:           stringFromMap(step.Output, "status"),
			Branch:           stringFromMap(step.Output, "branch"),
			HeadCommit:       stringFromMap(step.Output, "headCommit"),
			HeadCommitBefore: stringFromMap(step.Output, "headCommitBefore"),
			HeadCommitAfter:  stringFromMap(step.Output, "headCommitAfter"),
			CommitHash:       stringFromMap(commit, "hash"),
			CommitSubject:    stringFromMap(commit, "subject"),
			Summary:          stringFromMap(step.Output, "summary"),
			ChangedFiles:     resultGitChangedFilePaths(step.Output["changedFiles"]),
			StagedFiles:      resultGitChangedFilePaths(step.Output["stagedFiles"]),
			Truncated:        boolFromValue(step.Output["truncated"]),
			ZeroWork:         boolFromValue(step.Output["zeroWork"]),
			ErrorCode:        stringFromMap(step.Output, "errorCode"),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func resultGitChangedFilePaths(value any) []string {
	items := mapSliceFromValue(value)
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if path := stringFromMap(item, "path"); path != "" {
			ret = append(ret, path)
		}
	}
	return compactUniqueStrings(ret)
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

func gitOperationIndexArtifactID(run *runModel.Run) common.ArtifactID {
	if run == nil {
		return ""
	}
	return common.ArtifactID(fmt.Sprintf("%s/artifact/git-operation-index", run.ID))
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

func gitOperationIndexArtifactContent(gitDetails []resultGitOperationDetail) string {
	var builder strings.Builder
	builder.WriteString("# Git Operation Index\n\n")
	builder.WriteString("| Evidence | Kind | Operation | Status | Branch | Commit | Files | Summary |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, item := range gitDetails {
		commit := firstNonEmptyOrchestratorString(item.CommitHash, item.HeadCommitAfter, item.HeadCommit)
		files := item.ChangedFiles
		if len(files) == 0 {
			files = item.StagedFiles
		}
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s` | %s | %s | %s | %s | %s |\n",
			markdownInlineCode(string(item.StepID)),
			markdownInlineCode(item.Kind),
			markdownInlineCode(item.Operation),
			markdownTableCell(item.Status),
			markdownTableCell(item.Branch),
			markdownTableCell(shortCommit(commit)),
			markdownTableCell(strings.Join(files, ", ")),
			markdownTableCell(item.Summary),
		))
	}
	return strings.TrimSpace(builder.String())
}

func resultGitOperationEvidenceRefs(gitDetails []resultGitOperationDetail) []string {
	ret := make([]string, 0, len(gitDetails))
	for _, item := range gitDetails {
		if item.StepID != "" {
			ret = append(ret, string(item.StepID))
		}
	}
	return compactUniqueStrings(ret)
}

func shortCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
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

type resultGitOperationDetail struct {
	StepID           common.StepID
	Kind             string
	Operation        string
	Status           string
	Branch           string
	HeadCommit       string
	HeadCommitBefore string
	HeadCommitAfter  string
	CommitHash       string
	CommitSubject    string
	Summary          string
	ChangedFiles     []string
	StagedFiles      []string
	Truncated        bool
	ZeroWork         bool
	ErrorCode        string
}

func (r *resultSubsystem) resultEvidenceDetails(ctx context.Context, evidenceRefs []string) []resultEvidenceDetail {
	if r == nil || r.steps == nil || len(evidenceRefs) == 0 {
		return nil
	}
	ret := make([]resultEvidenceDetail, 0, len(evidenceRefs))
	for _, ref := range evidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		step, err := r.steps.Get(ctx, common.StepID(ref))
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
