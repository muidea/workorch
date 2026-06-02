package appservice

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const (
	readModelEventRecordLimit       = 100
	readModelLLMUsageRecordLimit    = 5000
	readModelStepPageLimit          = 100
	readModelMaxStepPageLimit       = 500
	readModelToolCallPageLimit      = 100
	readModelMaxToolCallPageLimit   = 500
	readModelCheckpointPageLimit    = 100
	readModelMaxCheckpointPageLimit = 500
	readModelResultPageLimit        = 100
	readModelMaxResultPageLimit     = 500
	readModelPatchPageLimit         = 100
	readModelMaxPatchPageLimit      = 500
	readModelValidationPageLimit    = 100
	readModelMaxValidationPageLimit = 500
	readModelArtifactPageLimit      = 100
	readModelMaxArtifactPageLimit   = 500
	readModelDecisionPageLimit      = 100
	readModelMaxDecisionPageLimit   = 500
	readModelApprovalPageLimit      = 100
	readModelMaxApprovalPageLimit   = 500
	eventPayloadStringLimit         = 512
	eventPayloadListLimit           = 16
)

func canonicalReadModelStepKind(kind string) string {
	return llmcommon.CanonicalRuntimeBlockKind(kind)
}

type readModelDependencies struct {
	workspaces            repository.WorkspaceRepository
	goals                 repository.GoalRepository
	dods                  repository.DoDRepository
	runs                  repository.RunRepository
	steps                 repository.StepRepository
	results               repository.ResultRepository
	artifacts             repository.ArtifactRepository
	artifactContinuations repository.ArtifactContinuationRepository
	eventRecords          repository.EventRecordRepository
	memoryRecalls         repository.MemoryRecallTraceRepository
	toolCalls             repository.ToolCallRepository
	mcpCapabilities       repository.MCPCapabilityRepository
	patches               repository.PatchRepository
	validations           repository.ValidationResultRepository
	checkpoints           repository.RuntimeCheckpointRepository
	autoGovernanceTraces  repository.AutoGovernanceTraceRepository
	tickets               repository.DecisionTicketRepository
	resolutions           repository.DecisionResolutionRepository
	approvals             repository.ApprovalRepository
	published             []events.WorkorchEvent
	loadConfig            func() (workorchconfig.Config, error)
}

func readModelDependenciesFromSession(session *RuntimeSession) (readModelDependencies, error) {
	exports, err := session.Exports()
	if err != nil {
		return readModelDependencies{}, err
	}
	if exports.Repositories.Workspaces == nil ||
		exports.Repositories.Runs == nil ||
		exports.Repositories.Steps == nil ||
		exports.Repositories.Results == nil ||
		exports.Repositories.Artifacts == nil ||
		exports.Repositories.ArtifactContinuations == nil ||
		exports.Repositories.EventRecords == nil ||
		exports.Repositories.MemoryRecallTraces == nil ||
		exports.Repositories.ToolCalls == nil ||
		exports.Repositories.MCPCapabilities == nil ||
		exports.Repositories.Patches == nil ||
		exports.Repositories.ValidationResults == nil ||
		exports.Repositories.RuntimeCheckpoints == nil ||
		exports.Repositories.Tickets == nil ||
		exports.Repositories.Resolutions == nil ||
		exports.Repositories.Approvals == nil {
		return readModelDependencies{}, common.ErrInvalidArgument
	}

	var published []events.WorkorchEvent
	if session.Runtime != nil {
		published = session.Runtime.PublishedWorkorchEvents()
	}

	return readModelDependencies{
		workspaces:            exports.Repositories.Workspaces,
		goals:                 exports.Repositories.Goals,
		dods:                  exports.Repositories.DoDs,
		runs:                  exports.Repositories.Runs,
		steps:                 exports.Repositories.Steps,
		results:               exports.Repositories.Results,
		artifacts:             exports.Repositories.Artifacts,
		artifactContinuations: exports.Repositories.ArtifactContinuations,
		eventRecords:          exports.Repositories.EventRecords,
		memoryRecalls:         exports.Repositories.MemoryRecallTraces,
		toolCalls:             exports.Repositories.ToolCalls,
		mcpCapabilities:       exports.Repositories.MCPCapabilities,
		patches:               exports.Repositories.Patches,
		validations:           exports.Repositories.ValidationResults,
		checkpoints:           exports.Repositories.RuntimeCheckpoints,
		autoGovernanceTraces:  exports.Repositories.AutoGovernanceTraces,
		tickets:               exports.Repositories.Tickets,
		resolutions:           exports.Repositories.Resolutions,
		approvals:             exports.Repositories.Approvals,
		published:             published,
		loadConfig:            configLoaderFromExports(exports),
	}, nil
}

func runReadModelTasks(tasks ...func() error) error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var ret error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if ret == nil {
			ret = err
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			recordErr(task())
		}(task)
	}
	wg.Wait()
	return ret
}

func mapPatches(items []*runmodel.Patch) []Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectPatch(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readModelPatchLineStats(item *runmodel.Patch) (int, int) {
	if item == nil {
		return 0, 0
	}
	text := item.Diff
	if strings.TrimSpace(text) == "" {
		text = item.Content
	}
	added := 0
	deleted := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			deleted++
		}
	}
	return added, deleted
}

func mergeWorkspaceSnapshotPatches(existing []Patch, latest []Patch) []Patch {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]Patch, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func mergeWorkspaceSnapshotArtifactContinuations(existing []ArtifactContinuation, latest []ArtifactContinuation) []ArtifactContinuation {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]ArtifactContinuation, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func mergeWorkspaceLLMUsageTraces(existing []LLMUsageTrace, latest []LLMUsageTrace) []LLMUsageTrace {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]LLMUsageTrace, 0, len(existing)+len(latest))
	for _, item := range existing {
		key := firstNonEmptyReadModel(item.ID, item.StepID+"|"+item.OccurredAt)
		seen[key] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		key := firstNonEmptyReadModel(item.ID, item.StepID+"|"+item.OccurredAt)
		if idx, ok := seen[key]; ok {
			out[idx] = item
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OccurredAt == out[j].OccurredAt {
			return out[i].ID < out[j].ID
		}
		return out[i].OccurredAt < out[j].OccurredAt
	})
	return out
}

func mapValidationResults(items []*runmodel.ValidationResult) []ValidationResult {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectValidationResult(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, RuntimeCheckpoint{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			CheckpointType:     string(item.CheckpointType),
			CurrentStepID:      string(item.CurrentStepID),
			RunStateSnapshot:   string(item.RunStateSnapshot),
			BlockingTicketID:   string(item.BlockingTicketID),
			BlockingApprovalID: string(item.BlockingApprovalID),
			ThreadRef:          item.ThreadRef,
			OccurredAt:         item.OccurredAt,
			RestorableUntil:    item.RestorableUntil,
			Data:               cloneReadModelMap(item.Data),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapDoDEvidence(snapshot RunSnapshot) []DoDEvidenceTrace {
	evidenceIndex := runEvidenceRefIndex(snapshot)
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	ret := make([]DoDEvidenceTrace, 0, len(results))
	for _, item := range results {
		if item.DoDAssessment == nil {
			continue
		}
		if snapshot.Run.DoDID != "" && item.DoDID != snapshot.Run.DoDID {
			continue
		}
		trace := DoDEvidenceTrace{
			ResultID:  item.ID,
			DoDID:     item.DoDID,
			Satisfied: item.DoDAssessment.Satisfied,
			Summary:   item.DoDAssessment.Summary,
		}
		assessmentItems := item.DoDAssessment.Items
		if len(assessmentItems) == 0 && item.SummaryDetail != nil && len(item.SummaryDetail.DoDItems) > 0 {
			assessmentItems = item.SummaryDetail.DoDItems
		}
		if len(assessmentItems) > 0 {
			trace.Items = make([]DoDEvidenceItem, 0, len(assessmentItems))
			for _, assessmentItem := range assessmentItems {
				refs := compactUniqueStrings(cloneReadModelStringSlice(assessmentItem.EvidenceRefs))
				missing := make([]string, 0)
				for _, ref := range refs {
					if _, ok := evidenceIndex[ref]; !ok {
						missing = append(missing, ref)
					}
				}
				status := "missing"
				if len(refs) > 0 && len(missing) == 0 {
					status = "linked"
				} else if len(refs) > 0 && len(missing) < len(refs) {
					status = "partial"
				}
				trace.Items = append(trace.Items, DoDEvidenceItem{
					Item:                assessmentItem.Item,
					State:               assessmentItem.State,
					FailureReason:       assessmentItem.FailureReason,
					Confidence:          assessmentItem.Confidence,
					SourceNote:          assessmentItem.SourceNote,
					EvidenceRefs:        refs,
					EvidenceStatus:      status,
					MissingEvidenceRefs: missing,
				})
			}
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResultQuality(snapshot RunSnapshot) []ResultQualityTrace {
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	evidenceIndex := runEvidenceRefIndex(snapshot)
	weakValidationRefs := weakValidationEvidenceRefIndex(snapshot)
	requiredDoDLeafItems := runRequiredDoDLeafItems(snapshot)
	ret := make([]ResultQualityTrace, 0, len(results))
	for _, item := range results {
		quality := ResultQualityTrace{
			ResultID:     item.ID,
			DoDID:        item.DoDID,
			State:        "passed",
			Summary:      strings.TrimSpace(item.Summary),
			EvidenceRefs: compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs)),
		}
		addIssue := func(code string, severity string, message string, suggestion string, refs ...string) {
			issue := ResultQualityIssue{
				Code:         code,
				Severity:     severity,
				Message:      message,
				Suggestion:   suggestion,
				EvidenceRefs: compactUniqueStrings(refs),
			}
			quality.Issues = append(quality.Issues, issue)
			if severity == "error" {
				quality.State = "failed"
			} else if quality.State != "failed" {
				quality.State = "warning"
			}
		}
		var evidenceRefs []string
		var goalOutcome string
		var verificationSummary string
		var knownLimitations []string
		var residualRisks []string
		var nextActions []string
		if item.SummaryDetail != nil {
			goalOutcome = strings.TrimSpace(item.SummaryDetail.GoalOutcome)
			verificationSummary = strings.TrimSpace(item.SummaryDetail.VerificationSummary)
			knownLimitations = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.KnownLimitations))
			residualRisks = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.ResidualRisks))
			nextActions = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.NextActions))
			evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.EvidenceRefs))
		}
		if len(evidenceRefs) == 0 {
			evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs))
		}
		quality.EvidenceRefs = evidenceRefs
		if goalOutcome == "" {
			addIssue("missing_goal_outcome", "error", "result is missing a concrete goal outcome summary", "populate summaryDetail.goalOutcome with the delivered outcome", evidenceRefs...)
		}
		if verificationSummary == "" {
			addIssue("missing_verification_summary", "error", "result is missing verification summary", "populate summaryDetail.verificationSummary with verification scope and conclusions", evidenceRefs...)
		}
		if len(evidenceRefs) == 0 {
			addIssue("missing_evidence_refs", "error", "result is missing evidence references", "attach validation, patch, tool call or artifact evidence refs")
		}
		if len(knownLimitations) == 0 && len(residualRisks) == 0 {
			addIssue("missing_risk_disclosure", "error", "result is missing known limitations and residual risks", "populate summaryDetail.knownLimitations or summaryDetail.residualRisks", evidenceRefs...)
		}
		if len(nextActions) == 0 {
			addIssue("missing_next_actions", "warning", "result is missing next actions", "populate summaryDetail.nextActions for follow-up guidance", evidenceRefs...)
		}
		if item.DoDAssessment == nil || len(item.DoDAssessment.Items) == 0 {
			addIssue("missing_dod_item_assessment", "warning", "result is missing DoD item-level assessment", "attach DoD item states and evidence refs", evidenceRefs...)
		} else {
			if missingLeaf := missingRequiredDoDLeafItems(requiredDoDLeafItems, item.DoDAssessment.Items); len(missingLeaf) > 0 {
				addIssue(
					"missing_dod_leaf_coverage",
					"error",
					fmt.Sprintf("result dodAssessment does not cover required DoD leaf items: %s", strings.Join(missingLeaf, "; ")),
					"include each missing DoD leaf item in dodAssessment.items with concrete evidenceRefs",
					evidenceRefs...,
				)
			}
			for _, assessmentItem := range item.DoDAssessment.Items {
				if strings.TrimSpace(assessmentItem.State) != "satisfied" {
					continue
				}
				itemName := strings.TrimSpace(assessmentItem.Item)
				refs := compactUniqueStrings(cloneReadModelStringSlice(assessmentItem.EvidenceRefs))
				if len(refs) == 0 {
					addIssue(
						"dod_item_missing_evidence_refs",
						"error",
						fmt.Sprintf("satisfied DoD item %q has no evidence refs", itemName),
						"attach concrete validation/patch/tool evidence refs for every satisfied DoD item",
					)
					continue
				}
				missingRefs := make([]string, 0, len(refs))
				for _, ref := range refs {
					if _, ok := evidenceIndex[ref]; !ok {
						missingRefs = append(missingRefs, ref)
					}
				}
				if len(missingRefs) > 0 {
					addIssue(
						"dod_item_unlinked_evidence_refs",
						"error",
						fmt.Sprintf("satisfied DoD item %q references unlinked evidence: %s", itemName, strings.Join(missingRefs, ", ")),
						"use evidence refs that exist in runtime steps, validations, patches, tool calls, checkpoints, or artifacts",
						refs...,
					)
				}
				if allEvidenceRefsWeakValidation(refs, weakValidationRefs) {
					addIssue(
						"dod_item_weak_verification_evidence",
						"error",
						fmt.Sprintf("satisfied DoD item %q relies only on weak verification evidence", itemName),
						"add effective assertions/tests or mark the item failed/skipped with impact and residual risk",
						refs...,
					)
				}
			}
		}
		ret = append(ret, quality)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func pageRunSteps(items []*runmodel.Step, query StepQuery) StepPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelStepPageLimit
	}
	if limit > readModelMaxStepPageLimit {
		limit = readModelMaxStepPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]presenter.StepView, 0, min(limit, len(items)))
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
		mapped := presenter.MapStep(item)
		if mapped == nil {
			continue
		}
		selected = append(selected, projectStepView(*mapped))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return StepPage{
		RunID:             strings.TrimSpace(query.RunID),
		Steps:             selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectStepView(item presenter.StepView) presenter.StepView {
	kind := canonicalReadModelStepKind(item.Kind)
	return presenter.StepView{
		ID:     item.ID,
		Kind:   kind,
		State:  item.State,
		Output: projectStepOutput(kind, item.Output),
	}
}

func projectStepOutput(kind string, output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"approvalId":                   {},
		"assessmentOnly":               {},
		"assessmentOnlyReason":         {},
		"blockingApprovalId":           {},
		"blockingTicketId":             {},
		"capabilityId":                 {},
		"category":                     {},
		"completionState":              {},
		"contextBudget":                {},
		"decisionId":                   {},
		"dodAssessment":                {},
		"error":                        {},
		"failureClass":                 {},
		"failureReason":                {},
		"llmCallRefs":                  {},
		"llmUsage":                     {},
		"memoryRefs":                   {},
		"nextKind":                     {},
		"operation":                    {},
		"patchId":                      {},
		"prompt":                       {},
		"reason":                       {},
		"repairAttempt":                {},
		"resultId":                     {},
		"serverId":                     {},
		"stage":                        {},
		"state":                        {},
		"status":                       {},
		"streamDiagnostics":            {},
		"summary":                      {},
		"target":                       {},
		"targetRef":                    {},
		"tool":                         {},
		"toolCallId":                   {},
		"transport":                    {},
		"uri":                          {},
		"validationId":                 {},
		"workspaceExecutionSummary":    {},
		"workspaceExecutionSummaryRef": {},
	}
	ret := map[string]any{}
	for key, value := range output {
		if _, ok := allowed[key]; !ok {
			continue
		}
		ret[key] = value
	}
	if strings.TrimSpace(kind) == "mcp/tool_call" {
		if result := projectMCPToolCallResult(output["result"]); len(result) > 0 {
			ret["result"] = result
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectMCPToolCallResult(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	ret := map[string]any{}
	if structured := projectMCPStructuredContent(raw["structuredContent"]); len(structured) > 0 {
		ret["structuredContent"] = structured
	}
	if isError, ok := raw["isError"].(bool); ok {
		ret["isError"] = isError
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectMCPStructuredContent(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"arguments": {},
		"name":      {},
		"ok":        {},
		"tool":      {},
		"transport": {},
	}
	ret := map[string]any{}
	for key, item := range raw {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if projected, ok := projectReadModelScalarMapValue(item); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectReadModelScalarMapValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		if len(typed) <= eventPayloadStringLimit {
			return typed, true
		}
		return typed[:eventPayloadStringLimit] + "...", true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case map[string]any:
		ret := map[string]any{}
		for key, item := range typed {
			if projected, ok := projectReadModelScalarMapValue(item); ok {
				ret[key] = projected
			}
		}
		if len(ret) == 0 {
			return nil, false
		}
		return ret, true
	default:
		return nil, false
	}
}

func pageRunToolCalls(items []*runmodel.ToolCall, query ToolCallQuery) ToolCallPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelToolCallPageLimit
	}
	if limit > readModelMaxToolCallPageLimit {
		limit = readModelMaxToolCallPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]ToolCall, 0, min(limit, len(items)))
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
		selected = append(selected, projectToolCall(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ToolCallPage{
		RunID:             strings.TrimSpace(query.RunID),
		ToolCalls:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectToolCall(item *runmodel.ToolCall) ToolCall {
	if item == nil {
		return ToolCall{}
	}
	return ToolCall{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
		ToolName:    item.ToolName,
		State:       string(item.State),
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}

func ProjectRunSnapshotDetail(snapshot RunSnapshot) RunSnapshot {
	ret := snapshot
	if ret.Step != nil {
		projected := projectStepView(*ret.Step)
		ret.Step = &projected
	}
	if len(ret.Steps) > 0 {
		limit := min(len(ret.Steps), readModelStepPageLimit)
		projected := make([]presenter.StepView, 0, limit)
		for _, item := range ret.Steps[:limit] {
			projected = append(projected, projectStepView(item))
		}
		ret.Steps = projected
	}
	if len(ret.ToolCalls) > 0 {
		limit := min(len(ret.ToolCalls), readModelToolCallPageLimit)
		projected := make([]ToolCall, 0, limit)
		for _, item := range ret.ToolCalls[:limit] {
			projected = append(projected, projectToolCallView(item))
		}
		ret.ToolCalls = projected
	}
	if len(ret.RuntimeCheckpoints) > 0 {
		limit := min(len(ret.RuntimeCheckpoints), readModelCheckpointPageLimit)
		projected := make([]RuntimeCheckpoint, 0, limit)
		for _, item := range ret.RuntimeCheckpoints[:limit] {
			projected = append(projected, projectRuntimeCheckpointView(item))
		}
		ret.RuntimeCheckpoints = projected
	}
	if len(ret.Results) > 0 {
		limit := min(len(ret.Results), readModelResultPageLimit)
		projected := make([]Result, 0, limit)
		for _, item := range ret.Results[:limit] {
			projected = append(projected, projectResultView(item))
		}
		ret.Results = projected
	}
	if len(ret.Patches) > 0 {
		limit := min(len(ret.Patches), readModelPatchPageLimit)
		projected := make([]Patch, 0, limit)
		for _, item := range ret.Patches[:limit] {
			projected = append(projected, projectPatchView(item))
		}
		ret.Patches = projected
	}
	if len(ret.ValidationResults) > 0 {
		limit := min(len(ret.ValidationResults), readModelValidationPageLimit)
		projected := make([]ValidationResult, 0, limit)
		for _, item := range ret.ValidationResults[:limit] {
			projected = append(projected, projectValidationResultView(item))
		}
		ret.ValidationResults = projected
	}
	if len(ret.Artifacts) > 0 {
		limit := min(len(ret.Artifacts), readModelArtifactPageLimit)
		projected := make([]Artifact, 0, limit)
		for _, item := range ret.Artifacts[:limit] {
			projected = append(projected, projectArtifactView(item))
		}
		ret.Artifacts = projected
	}
	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), readModelDecisionPageLimit)
		projected := make([]Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, projectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), readModelApprovalPageLimit)
		projected := make([]Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, projectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

func ProjectWorkspaceSnapshotDetail(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	ret := snapshot
	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), readModelDecisionPageLimit)
		projected := make([]Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, projectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), readModelApprovalPageLimit)
		projected := make([]Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, projectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

func projectToolCallView(item ToolCall) ToolCall {
	return ToolCall{
		ID:          item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		ToolName:    item.ToolName,
		State:       item.State,
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}

func pageRunRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint, query RuntimeCheckpointQuery) RuntimeCheckpointPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelCheckpointPageLimit
	}
	if limit > readModelMaxCheckpointPageLimit {
		limit = readModelMaxCheckpointPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]RuntimeCheckpoint, 0, min(limit, len(items)))
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
		selected = append(selected, projectRuntimeCheckpoint(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return RuntimeCheckpointPage{
		RunID:              strings.TrimSpace(query.RunID),
		RuntimeCheckpoints: selected,
		Limit:              limit,
		CursorAfterID:      cursorAfterID,
		NextCursorAfterID:  nextCursor,
		HasMore:            hasMore,
	}
}

func projectRuntimeCheckpoint(item *runmodel.RuntimeCheckpoint) RuntimeCheckpoint {
	if item == nil {
		return RuntimeCheckpoint{}
	}
	return RuntimeCheckpoint{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		RunID:              string(item.RunID),
		CheckpointType:     string(item.CheckpointType),
		CurrentStepID:      string(item.CurrentStepID),
		RunStateSnapshot:   string(item.RunStateSnapshot),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}

func projectRuntimeCheckpointView(item RuntimeCheckpoint) RuntimeCheckpoint {
	return RuntimeCheckpoint{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		CheckpointType:     item.CheckpointType,
		CurrentStepID:      item.CurrentStepID,
		RunStateSnapshot:   item.RunStateSnapshot,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}

func pageRunPatches(items []*runmodel.Patch, query PatchQuery) PatchPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelPatchPageLimit
	}
	if limit > readModelMaxPatchPageLimit {
		limit = readModelMaxPatchPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]Patch, 0, min(limit, len(items)))
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
		selected = append(selected, projectPatch(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return PatchPage{
		RunID:             strings.TrimSpace(query.RunID),
		Patches:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectPatch(item *runmodel.Patch) Patch {
	if item == nil {
		return Patch{}
	}
	addedLines, deletedLines := readModelPatchLineStats(item)
	return Patch{
		ID:               string(item.ID),
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.RunID),
		StepID:           string(item.StepID),
		ToolCallID:       string(item.ToolCallID),
		ApprovalID:       string(item.ApprovalID),
		RevertApprovalID: string(item.RevertApprovalID),
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            string(item.State),
		AddedLines:       addedLines,
		DeletedLines:     deletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}

func projectPatchView(item Patch) Patch {
	return Patch{
		ID:               item.ID,
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ToolCallID:       item.ToolCallID,
		ApprovalID:       item.ApprovalID,
		RevertApprovalID: item.RevertApprovalID,
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            item.State,
		AddedLines:       item.AddedLines,
		DeletedLines:     item.DeletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}

func pageRunValidationResults(items []*runmodel.ValidationResult, query ValidationResultQuery) ValidationResultPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelValidationPageLimit
	}
	if limit > readModelMaxValidationPageLimit {
		limit = readModelMaxValidationPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]ValidationResult, 0, min(limit, len(items)))
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
		selected = append(selected, projectValidationResult(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ValidationResultPage{
		RunID:             strings.TrimSpace(query.RunID),
		ValidationResults: selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectValidationResult(item *runmodel.ValidationResult) ValidationResult {
	if item == nil {
		return ValidationResult{}
	}
	return ValidationResult{
		ID:              string(item.ID),
		WorkspaceID:     string(item.WorkspaceID),
		RunID:           string(item.RunID),
		StepID:          string(item.StepID),
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           string(item.State),
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: string(item.EvidenceQuality),
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}

func projectValidationResultView(item ValidationResult) ValidationResult {
	return ValidationResult{
		ID:              item.ID,
		WorkspaceID:     item.WorkspaceID,
		RunID:           item.RunID,
		StepID:          item.StepID,
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           item.State,
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: item.EvidenceQuality,
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}

func cloneReadModelMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneReadModelStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	return append([]string(nil), input...)
}

func readModelStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func readModelBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func readModelIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	default:
		return 0
	}
	return 0
}

func readModelFloatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func readModelMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

func readModelMapSliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneReadModelMap(item))
		}
		return ret
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, cloneReadModelMap(mapped))
			}
		}
		return ret
	default:
		return nil
	}
}

func readModelStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func readModelPolicyDecision(value any) string {
	policy := readModelMapValue(value)
	if len(policy) == 0 {
		return ""
	}
	return readModelStringValue(policy["decision"])
}

func mapRunStatus(snapshot RunSnapshot) RunStatus {
	ret := RunStatus{
		ID:                 snapshot.Run.ID,
		WorkspaceID:        snapshot.Run.WorkspaceID,
		State:              snapshot.Run.State,
		StepCount:          len(snapshot.Steps),
		DecisionCount:      len(snapshot.Decisions),
		ApprovalCount:      len(snapshot.Approvals),
		BlockingTicketID:   snapshot.Run.BlockingTicketID,
		BlockingApprovalID: snapshot.Run.BlockingApprovalID,
		FailureReason:      snapshot.Run.FailureReason,
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = canonicalReadModelStepKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
	}
	return ret
}

func mapRunObservability(snapshot RunSnapshot) RunObservability {
	timeline := runtimeTimeline(snapshot)
	return RunObservability{
		Run:                  snapshot.Run,
		Outcome:              snapshot.Outcome,
		Progress:             runtimeProgress(snapshot),
		TaskProgress:         snapshot.TaskProgress,
		RepairLoop:           snapshot.RepairLoop,
		TaskDecompositions:   snapshot.TaskDecompositions,
		TaskPlans:            snapshot.TaskPlans,
		ExecutionSections:    snapshot.ExecutionSections,
		LLMContextBudgets:    snapshot.LLMContextBudgets,
		LLMUsageTraces:       snapshot.LLMUsageTraces,
		LLMUsageSummary:      snapshot.LLMUsageSummary,
		MemoryRecallTraces:   snapshot.MemoryRecallTraces,
		GitOperations:        snapshot.GitOperations,
		WebSources:           snapshot.WebSources,
		ClosureTraces:        snapshot.ClosureTraces,
		DoDEvidence:          snapshot.DoDEvidence,
		ResultQuality:        snapshot.ResultQuality,
		Evidence:             runtimeEvidenceSummary(snapshot, len(timelineEvents(timeline))),
		LatestCheckpoint:     latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		SelectedCapabilities: snapshot.SelectedCapabilities,
		UsedCapabilities:     snapshot.UsedCapabilities,
		Timeline:             timeline,
	}
}
