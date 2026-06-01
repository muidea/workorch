package appservice

import (
	"strings"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
)

func mapResults(items []*resultmodel.Result) []Result {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Result, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		artifactRefs := make([]string, 0, len(item.ArtifactRefs))
		for _, ref := range item.ArtifactRefs {
			if ref != "" {
				artifactRefs = append(artifactRefs, string(ref))
			}
		}
		ret = append(ret, Result{
			ID:            string(item.ID),
			WorkspaceID:   string(item.WorkspaceID),
			RunID:         string(item.RunID),
			GoalID:        string(item.GoalID),
			DoDID:         string(item.DoDID),
			State:         string(item.State),
			Title:         item.Title,
			Summary:       item.Summary,
			SourceType:    item.SourceType,
			SourceRef:     item.SourceRef,
			EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
			ArtifactRefs:  artifactRefs,
			DoDAssessment: mapDoDAssessment(item.DoDAssessment),
			SummaryDetail: mapResultSummary(item.SummaryDetail),
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResultSummary(item resultmodel.ResultSummary) *ResultSummary {
	if item.GoalOutcome == "" &&
		len(item.DoDItems) == 0 &&
		item.ImplementationSummary == "" &&
		item.VerificationSummary == "" &&
		len(item.EvidenceRefs) == 0 &&
		len(item.ArtifactRefs) == 0 &&
		len(item.KnownLimitations) == 0 &&
		len(item.ResidualRisks) == 0 &&
		len(item.NextActions) == 0 {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	ret := &ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          artifactRefs,
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
	}
	if len(item.DoDItems) > 0 {
		ret.DoDItems = make([]DoDAssessmentItem, 0, len(item.DoDItems))
		for _, assessmentItem := range item.DoDItems {
			ret.DoDItems = append(ret.DoDItems, DoDAssessmentItem{
				Item:          assessmentItem.Item,
				State:         assessmentItem.State,
				EvidenceRefs:  cloneReadModelStringSlice(assessmentItem.EvidenceRefs),
				FailureReason: assessmentItem.FailureReason,
				Confidence:    assessmentItem.Confidence,
				SourceNote:    assessmentItem.SourceNote,
			})
		}
	}
	return ret
}

func mapDoDAssessment(item resultmodel.DoDAssessment) *DoDAssessment {
	if item.Summary == "" &&
		len(item.Items) == 0 &&
		item.GoalCoverage == nil &&
		item.AcceptanceDimensions == nil &&
		item.DeliverableCoverage == nil &&
		!item.Satisfied {
		return nil
	}
	ret := &DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         mapDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: mapDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  mapDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, DoDAssessmentItem{
				Item:          assessmentItem.Item,
				State:         assessmentItem.State,
				EvidenceRefs:  cloneReadModelStringSlice(assessmentItem.EvidenceRefs),
				FailureReason: assessmentItem.FailureReason,
				Confidence:    assessmentItem.Confidence,
				SourceNote:    assessmentItem.SourceNote,
			})
		}
	}
	return ret
}

func mapDoDCoverage(item *resultmodel.DoDCoverage) *DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			artifactRefs := make([]string, 0, len(coverageItem.ArtifactRefs))
			for _, ref := range coverageItem.ArtifactRefs {
				if ref != "" {
					artifactRefs = append(artifactRefs, string(ref))
				}
			}
			ret.Items = append(ret.Items, DoDCoverageItem{
				ID:           coverageItem.ID,
				Deliverable:  coverageItem.Deliverable,
				Name:         coverageItem.Name,
				Path:         coverageItem.Path,
				Goal:         coverageItem.Goal,
				GoalItem:     coverageItem.GoalItem,
				Item:         coverageItem.Item,
				Requirement:  coverageItem.Requirement,
				State:        coverageItem.State,
				EvidenceRefs: cloneReadModelStringSlice(coverageItem.EvidenceRefs),
				ArtifactRefs: artifactRefs,
			})
		}
	}
	return ret
}

func mapArtifacts(items []*resultmodel.Artifact) []Artifact {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Artifact, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, Artifact{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			ResultID:           string(item.ResultID),
			StepID:             string(item.StepID),
			ToolCallID:         string(item.ToolCallID),
			PatchID:            string(item.PatchID),
			ValidationResultID: string(item.ValidationResultID),
			ArtifactType:       item.ArtifactType,
			Category:           item.Category,
			Name:               item.Name,
			Summary:            item.Summary,
			URI:                item.URI,
			ContentType:        item.ContentType,
			State:              string(item.State),
			EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
			Metadata:           cloneReadModelMap(item.Metadata),
			CreatedAt:          item.CreatedAt,
			UpdatedAt:          item.UpdatedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) []ArtifactContinuation {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ArtifactContinuation, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectArtifactContinuation(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectArtifactContinuation(item *orchestratormodel.ArtifactContinuation) ArtifactContinuation {
	if item == nil {
		return ArtifactContinuation{}
	}
	progress := map[string]any{}
	if item.ExpectedSequence > 0 {
		progress["expectedSequence"] = item.ExpectedSequence
	}
	if len(item.ReceivedSequences) > 0 {
		progress["receivedCount"] = len(item.ReceivedSequences)
		progress["lastReceivedSequence"] = item.ReceivedSequences[len(item.ReceivedSequences)-1]
	}
	if item.BytesWritten > 0 {
		progress["bytesWritten"] = item.BytesWritten
	}
	if len(progress) == 0 {
		progress = nil
	}
	return ArtifactContinuation{
		ID:                string(item.ID),
		WorkspaceID:       string(item.WorkspaceID),
		RunID:             string(item.RunID),
		RootStepID:        string(item.RootStepID),
		ArtifactID:        item.ArtifactID,
		TargetRef:         item.TargetRef,
		Operation:         item.Operation,
		State:             string(item.State),
		ExpectedSequence:  item.ExpectedSequence,
		ReceivedSequences: append([]int(nil), item.ReceivedSequences...),
		FinalReceived:     item.FinalReceived,
		BytesWritten:      item.BytesWritten,
		RollingHash:       item.RollingHash,
		LastChunkHash:     item.LastChunkHash,
		CoverageTargets:   cloneReadModelStringSlice(item.CoverageTargets),
		EvidenceRefs:      cloneReadModelStringSlice(item.EvidenceRefs),
		FailureReason:     artifactContinuationFailureReason(item),
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
		Progress:          progress,
	}
}

func artifactContinuationFailureReason(item *orchestratormodel.ArtifactContinuation) string {
	if item == nil || len(item.Metadata) == 0 {
		return ""
	}
	if failure := readModelStringValue(item.Metadata["failureError"]); strings.TrimSpace(failure) != "" {
		return strings.TrimSpace(failure)
	}
	if failure := readModelStringValue(item.Metadata["applyFailure"]); strings.TrimSpace(failure) != "" {
		return strings.TrimSpace(failure)
	}
	return ""
}

func pageRunResults(items []*resultmodel.Result, query ResultQuery) ResultPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelResultPageLimit
	}
	if limit > readModelMaxResultPageLimit {
		limit = readModelMaxResultPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]Result, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectResult(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ResultPage{
		RunID:             strings.TrimSpace(query.RunID),
		Results:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectResult(item *resultmodel.Result) Result {
	if item == nil {
		return Result{}
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return Result{
		ID:           string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		GoalID:       string(item.GoalID),
		DoDID:        string(item.DoDID),
		State:        string(item.State),
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: artifactRefs,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func projectResultView(item Result) Result {
	return Result{
		ID:           item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		GoalID:       item.GoalID,
		DoDID:        item.DoDID,
		State:        item.State,
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: cloneReadModelStringSlice(item.ArtifactRefs),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func pageRunArtifacts(items []*resultmodel.Artifact, query ArtifactQuery) ArtifactPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelArtifactPageLimit
	}
	if limit > readModelMaxArtifactPageLimit {
		limit = readModelMaxArtifactPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]Artifact, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectArtifact(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ArtifactPage{
		RunID:             strings.TrimSpace(query.RunID),
		Artifacts:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectArtifact(item *resultmodel.Artifact) Artifact {
	if item == nil {
		return Artifact{}
	}
	return Artifact{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		RunID:              string(item.RunID),
		ResultID:           string(item.ResultID),
		StepID:             string(item.StepID),
		ToolCallID:         string(item.ToolCallID),
		PatchID:            string(item.PatchID),
		ValidationResultID: string(item.ValidationResultID),
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              string(item.State),
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func projectArtifactView(item Artifact) Artifact {
	return Artifact{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		ResultID:           item.ResultID,
		StepID:             item.StepID,
		ToolCallID:         item.ToolCallID,
		PatchID:            item.PatchID,
		ValidationResultID: item.ValidationResultID,
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              item.State,
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}
