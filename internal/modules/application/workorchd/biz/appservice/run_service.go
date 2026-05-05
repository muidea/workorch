package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Run struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	GoalID             string         `json:"goalId,omitempty"`
	DoDID              string         `json:"doDId,omitempty"`
	InitialStepKind    string         `json:"initialStepKind,omitempty"`
	ExecutionContext   map[string]any `json:"executionContext,omitempty"`
	State              string         `json:"state"`
	Priority           int            `json:"priority,omitempty"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	FailureReason      string         `json:"failureReason,omitempty"`
}

func CreateRunWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string, goalID string, dodID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	if err := validateRunDefinitions(ctx, session, workspaceID, goalID, dodID); err != nil {
		return Run{}, err
	}
	var executionContext map[string]any
	if goalID != "" || dodID != "" {
		executionContext, err = buildWorkspaceExecutionContext(
			ctx,
			session,
			common.WorkspaceID(workspaceID),
			common.GoalID(goalID),
			common.DoDID(dodID),
			ActivateWorkspaceExecutionCommand{},
		)
		if err != nil {
			return Run{}, err
		}
	}

	id := common.RunID(runID)
	if err := service.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:            id,
		WorkspaceID:      common.WorkspaceID(workspaceID),
		GoalID:           common.GoalID(goalID),
		DoDID:            common.DoDID(dodID),
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: executionContext,
	}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func CreateRunWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
}

func CreateRunFromEnv(ctx context.Context, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
}

func StartRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.RunID(runID)
	if err := attachCompressedContextToRun(ctx, session, id); err != nil {
		return Run{}, err
	}
	if err := service.EnsureStarted(ctx, runmodule.StartRunCommand{RunID: id}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func StartRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return StartRunWithSession(ctx, session, runID)
}

func StartRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return StartRunWithSession(ctx, session, runID)
}

func ResumeRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.RunID(runID)
	patch, loadedRun, err := compressedContextPatchForRun(ctx, session, id)
	if err != nil {
		return Run{}, err
	}
	if err := service.ResumeRun(ctx, runmodule.ResumeRunCommand{RunID: id, LoadedRun: loadedRun, ExecutionContextPatch: patch}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func ResumeRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return ResumeRunWithSession(ctx, session, runID)
}

func ResumeRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return ResumeRunWithSession(ctx, session, runID)
}

func ReadRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	_, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.RunID(runID))
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func ReadRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return ReadRunWithSession(ctx, session, runID)
}

func ReadRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return ReadRunWithSession(ctx, session, runID)
}

func ListRunsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []Run, err error) {
	_, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Run, 0, len(items))
	for _, item := range items {
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		ret = append(ret, mapRun(item))
	}
	return ret, nil
}

func ListRunsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListRunsWithSession(ctx, session, workspaceID)
}

func ListRunsFromEnv(ctx context.Context, workspaceID string) ([]Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListRunsWithSession(ctx, session, workspaceID)
}

func attachCompressedContextToRun(ctx context.Context, session *RuntimeSession, runID common.RunID) error {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return err
	}
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return attachCompressedContextToLoadedRun(ctx, deps, item)
}

func compressedContextPatchForRun(ctx context.Context, session *RuntimeSession, runID common.RunID) (map[string]any, *runmodel.Run, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, nil, err
	}
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	compressed, err := compressedContextForLoadedRun(ctx, deps, item)
	if err != nil {
		return nil, nil, err
	}
	if err := recordRecoveryMemoryRecallTrace(ctx, session, deps, item, compressed); err != nil {
		return nil, nil, err
	}
	return map[string]any{"compressedContext": compressed}, item, nil
}

func recordRecoveryMemoryRecallTrace(ctx context.Context, session *RuntimeSession, deps readModelDependencies, item *runmodel.Run, compressed map[string]any) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || deps.memoryRecalls == nil || len(compressed) == 0 {
		return nil
	}
	sourceRefs := stringSliceFromAny(compressed["sourceSectionRefs"])
	if !containsString(sourceRefs, "memory_recall") {
		return nil
	}
	existing, err := deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{RunID: item.ID})
	if err != nil {
		return err
	}
	memoryIDs := make([]common.MemoryID, 0)
	sourceTraceIDs := make([]string, 0)
	status := "inherited"
	for _, trace := range existing {
		if trace == nil || trace.Source == "recovery" {
			continue
		}
		sourceTraceIDs = append(sourceTraceIDs, string(trace.ID), trace.EvidenceRef)
		memoryIDs = append(memoryIDs, trace.RetrievedMemoryIDs...)
		if trace.Status == "degraded" {
			status = "degraded"
		}
	}
	memoryIDs = compactMemoryIDs(memoryIDs)
	if len(memoryIDs) == 0 && status != "degraded" {
		return nil
	}
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/recovery", item.ID))
	trace := &memorymodel.MemoryRecallTrace{
		ID:                 traceID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.ID,
		Stage:              "recovery",
		Source:             "recovery",
		Policy:             "inherit_compressed_context",
		Status:             status,
		RetrievedMemoryIDs: compactMemoryIDs(memoryIDs),
		Reason:             "compressed context inherited memory recall evidence",
		OccurredAt:         time.Now().UTC().Format(time.RFC3339Nano),
		EvidenceRef:        string(traceID),
	}
	if len(sourceTraceIDs) > 0 {
		trace.Reason += ": " + strings.Join(compactUniqueStrings(sourceTraceIDs), ", ")
	}
	if err := deps.memoryRecalls.Upsert(ctx, trace); err != nil {
		return err
	}
	exports, err := session.Exports()
	if err != nil || exports.Infrastructure.EventHub == nil {
		return err
	}
	return exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicMemoryRecallRecorded,
		OccurredAt:   trace.OccurredAt,
		SourceModule: modulecommon.EventSourceModuleID,
		SubjectType:  "memory_recall",
		SubjectID:    string(trace.ID),
		WorkspaceID:  string(trace.WorkspaceID),
		RunID:        string(trace.RunID),
		Payload: events.MemoryRecallPayload(
			trace.ID,
			trace.Stage,
			trace.Source,
			trace.Policy,
			trace.Status,
			trace.ProviderID,
			trace.Operation,
			trace.RetrievedMemoryIDs,
			trace.Reason,
		),
	})
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func compactMemoryIDs(input []common.MemoryID) []common.MemoryID {
	if len(input) == 0 {
		return nil
	}
	ret := make([]common.MemoryID, 0, len(input))
	seen := map[common.MemoryID]struct{}{}
	for _, item := range input {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func attachCompressedContextToLoadedRun(ctx context.Context, deps readModelDependencies, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	compressed, err := compressedContextForLoadedRun(ctx, deps, item)
	if err != nil {
		return err
	}
	executionContext := cloneContextMap(item.ExecutionContext)
	if executionContext == nil {
		executionContext = map[string]any{}
	}
	executionContext["compressedContext"] = compressed
	item.ExecutionContext = executionContext
	return deps.runs.Update(ctx, item)
}

func compressedContextForLoadedRun(ctx context.Context, deps readModelDependencies, item *runmodel.Run) (map[string]any, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	compressed := compressedContextForRunStart(item)
	if compressed != nil {
		return compressed, nil
	}
	compressed, err := compressedContextForBlockedRunResume(ctx, deps, item)
	if err != nil {
		return nil, err
	}
	if compressed != nil {
		return compressed, nil
	}
	snapshot, err := readRunRecoverySnapshot(ctx, deps, item.ID)
	if err != nil {
		return nil, err
	}
	return compressedContextMap(compressedContextFromRunSnapshot(snapshot)), nil
}

func compressedContextForRunStart(item *runmodel.Run) map[string]any {
	if item == nil || item.State != common.RunStateCreated || item.ID == "" || item.WorkspaceID == "" {
		return nil
	}
	language := runDialogLanguage(item)
	return compressedContextMap(CompressedContext{
		Version:          "workorch.compressedContext.v1",
		Mode:             "deterministic_recovery_summary",
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.ID),
		Summary:          localizedCreatedRunSummary(language),
		CurrentMilestone: "execution_steps",
		Milestones: []ProgressMilestone{
			{ID: "run_created", Title: localizedMilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{string(item.ID)}},
			{ID: "execution_steps", Title: localizedMilestoneTitle(language, "execution_steps"), State: "pending"},
			{ID: "governance", Title: localizedMilestoneTitle(language, "governance"), State: "pending"},
			{ID: "validation", Title: localizedMilestoneTitle(language, "validation"), State: "pending"},
			{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: "pending"},
		},
		Recovery: RecoveryPlan{
			Mode:       "continue",
			NextAction: localizedRecoveryNextAction(language),
			RunID:      string(item.ID),
		},
		SourceSectionRefs:  []string{"workspace", "run", "task_progress", "recovery"},
		SourceEvidenceRefs: []string{string(item.ID)},
		RetentionPolicy:    localizedRetentionPolicy(language),
	})
}

func compressedContextForBlockedRunResume(ctx context.Context, deps readModelDependencies, item *runmodel.Run) (map[string]any, error) {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return nil, nil
	}
	language := runDialogLanguage(item)
	var blocker ProgressBlocker
	recovery := RecoveryPlan{
		Mode:       "continue",
		NextAction: "continue_run",
		RunID:      string(item.ID),
	}
	summary := ""
	switch item.State {
	case common.RunStateBlockedOnDecision:
		if item.BlockingTicketID == "" || deps.tickets == nil || deps.resolutions == nil {
			return nil, nil
		}
		ticket, err := deps.tickets.Get(ctx, item.BlockingTicketID)
		if err != nil {
			return nil, err
		}
		if ticket.State != common.DecisionTicketStateResolved {
			return nil, nil
		}
		resolution, err := deps.resolutions.GetByTicketID(ctx, item.BlockingTicketID)
		if err != nil {
			return nil, err
		}
		if resolution.Type != "provide_missing_info" {
			return nil, nil
		}
		blocker = ProgressBlocker{
			ID:          string(item.BlockingTicketID),
			Kind:        "decision",
			State:       string(common.DecisionTicketStateResolved),
			Reason:      localizedBlockerReason(language, "decision", "resolved"),
			EvidenceRef: string(item.BlockingTicketID),
		}
		recovery.BlockingTicketID = string(item.BlockingTicketID)
		recovery.Reason = localizedBlockerReason(language, "decision", "resolved")
		summary = localizedBlockerReason(language, "decision", "resumed")
	case common.RunStateBlockedOnApproval:
		if item.BlockingApprovalID == "" || deps.approvals == nil {
			return nil, nil
		}
		approval, err := deps.approvals.Get(ctx, item.BlockingApprovalID)
		if err != nil {
			return nil, err
		}
		if approval.State != common.ApprovalStateApproved {
			return nil, nil
		}
		blocker = ProgressBlocker{
			ID:          string(item.BlockingApprovalID),
			Kind:        "approval",
			State:       string(common.ApprovalStateApproved),
			Reason:      localizedBlockerReason(language, "approval", "approved"),
			EvidenceRef: string(item.BlockingApprovalID),
		}
		recovery.BlockingApprovalID = string(item.BlockingApprovalID)
		recovery.Reason = localizedBlockerReason(language, "approval", "approved")
		summary = localizedBlockerReason(language, "approval", "resumed")
	default:
		return nil, nil
	}
	return compressedContextMap(CompressedContext{
		Version:          "workorch.compressedContext.v1",
		Mode:             "deterministic_recovery_summary",
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.ID),
		Summary:          summary,
		CurrentMilestone: "execution_steps",
		Milestones: []ProgressMilestone{
			{ID: "run_created", Title: localizedMilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{string(item.ID)}},
			{ID: "execution_steps", Title: localizedMilestoneTitle(language, "execution_steps"), State: "in_progress"},
			{ID: "governance", Title: localizedMilestoneTitle(language, "governance"), State: "in_progress", EvidenceRefs: []string{blocker.ID}},
			{ID: "validation", Title: localizedMilestoneTitle(language, "validation"), State: "pending"},
			{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: "pending"},
		},
		Blockers:           []ProgressBlocker{blocker},
		Recovery:           recovery,
		SourceSectionRefs:  []string{"run", "governance", "task_progress"},
		SourceEvidenceRefs: []string{string(item.ID), blocker.ID},
		RetentionPolicy:    localizedRetentionPolicy(language),
	}), nil
}

func readRunContextBundle(ctx context.Context, deps readModelDependencies, runID common.RunID) (ContextBundle, error) {
	snapshot, err := readRunSnapshot(ctx, deps, runID)
	if err != nil {
		return ContextBundle{}, err
	}
	return mapRunContextBundle(snapshot), nil
}

func compressedContextMap(input CompressedContext) map[string]any {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var ret map[string]any
	if err := json.Unmarshal(payload, &ret); err != nil {
		return nil
	}
	return ret
}

func runDependenciesFromSession(session *RuntimeSession) (runmodule.Service, repository.RunRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Services.RunService == nil || exports.Repositories.Runs == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.RunService, exports.Repositories.Runs, nil
}

func validateRunDefinitions(ctx context.Context, session *RuntimeSession, workspaceID string, goalID string, dodID string) error {
	if goalID == "" && dodID == "" {
		return nil
	}
	if goalID == "" || dodID == "" {
		return common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	if exports.Repositories.Workspaces == nil || exports.Repositories.Goals == nil || exports.Repositories.DoDs == nil {
		return common.ErrInvalidArgument
	}
	var (
		goal    *workspacemodel.Goal
		dod     *workspacemodel.DoD
		wsErr   error
		goalErr error
		dodErr  error
	)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, wsErr = exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	}()
	go func() {
		defer wg.Done()
		goal, goalErr = exports.Repositories.Goals.Get(ctx, common.GoalID(goalID))
	}()
	go func() {
		defer wg.Done()
		dod, dodErr = exports.Repositories.DoDs.Get(ctx, common.DoDID(dodID))
	}()
	wg.Wait()
	if wsErr != nil {
		return wsErr
	}
	if goalErr != nil {
		return goalErr
	}
	if goal.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	if dodErr != nil {
		return dodErr
	}
	if dod.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	if dod.State != common.DoDStateReady && dod.State != common.DoDStateSatisfied {
		return common.ErrConflict
	}
	return nil
}

func cloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func mapRun(item *runmodel.Run) Run {
	if item == nil {
		return Run{}
	}
	return Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		GoalID:             string(item.GoalID),
		DoDID:              string(item.DoDID),
		InitialStepKind:    item.InitialStepKind,
		ExecutionContext:   cloneContextMap(item.ExecutionContext),
		State:              string(item.State),
		Priority:           item.Priority,
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
}

func runDialogLanguage(item *runmodel.Run) string {
	if item == nil || item.ExecutionContext == nil {
		return defaultDialogLanguage()
	}
	if lang, ok := item.ExecutionContext["dialogLanguage"].(string); ok {
		if normalized := languageNameFromLocale(lang); normalized != "" {
			return normalized
		}
	}
	return defaultDialogLanguage()
}
