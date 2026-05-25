package appservice

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	appbootstrap "github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type ActivateWorkspaceExecutionCommand struct {
	GoalID                string            `json:"goalId,omitempty"`
	DoDID                 string            `json:"doDId,omitempty"`
	LLMProviderID         string            `json:"llmProviderId,omitempty"`
	SoulID                string            `json:"soulId,omitempty"`
	SoulBindings          map[string]string `json:"soulBindings,omitempty"`
	SkillIDs              []string          `json:"skillIds,omitempty"`
	MCPServerIDs          []string          `json:"mcpServerIds,omitempty"`
	CapabilityIDs         []string          `json:"capabilityIds,omitempty"`
	MemoryIDs             []string          `json:"memoryIds,omitempty"`
	DialogLanguage        string            `json:"dialogLanguage,omitempty"`
	Priority              int               `json:"priority,omitempty"`
	ExecutionContextPatch map[string]any    `json:"executionContextPatch,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}

func ActivateWorkspaceExecutionWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (ret WorkspaceExecutionActivation, err error) {
	exports, err := session.Exports()
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if exports.Services.RunService == nil ||
		exports.Repositories.Workspaces == nil ||
		exports.Repositories.Goals == nil ||
		exports.Repositories.DoDs == nil ||
		exports.Repositories.Runs == nil {
		return WorkspaceExecutionActivation{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)

	workspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if err := ensureWorkspaceManagedConfigs(workspace.InternalDirectory, workspace.ContextHints); err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if workspace.ActiveRunID != "" {
		run, err := exports.Repositories.Runs.Get(ctx, workspace.ActiveRunID)
		if err != nil {
			return WorkspaceExecutionActivation{}, err
		}
		if isActivationTerminalRunState(run.State) {
			workspace.ActiveRunID = ""
		} else {
			goal, dod, replaceErr := selectWorkspaceExecutionDefinitions(ctx, session, common.WorkspaceID(workspaceID), cmd)
			if replaceErr != nil {
				if strings.TrimSpace(cmd.GoalID) != "" || strings.TrimSpace(cmd.DoDID) != "" {
					return WorkspaceExecutionActivation{}, replaceErr
				}
			} else if activeRunDefinitionStale(ctx, session, workspace.ID, run, goal, dod) {
				executionContext, err := buildWorkspaceExecutionContextWithDefinitions(ctx, session, workspace, goal, dod, cmd)
				if err != nil {
					return WorkspaceExecutionActivation{}, err
				}
				if err := exports.Services.RunService.SupersedeRun(ctx, runmodule.SupersedeRunCommand{RunID: run.ID}); err != nil {
					return WorkspaceExecutionActivation{}, err
				}
				runID, err := createAndStartWorkspaceExecutionRun(ctx, session, common.WorkspaceID(workspaceID), goal.ID, dod.ID, executionContext, cmd.Priority, cmd.LLMProviderID)
				if err != nil {
					return WorkspaceExecutionActivation{}, err
				}
				startedRun, err := exports.Repositories.Runs.Get(ctx, runID)
				if err != nil {
					return WorkspaceExecutionActivation{}, err
				}
				snapshot := activationSnapshot(workspace, startedRun, goal, dod)
				return WorkspaceExecutionActivation{
					Workspace: snapshot.Workspace,
					Run:       mapRun(startedRun),
					Snapshot:  snapshot,
				}, nil
			}
			snapshot := activationSnapshot(workspace, run, goal, dod)
			return WorkspaceExecutionActivation{
				Workspace: snapshot.Workspace,
				Run:       mapRun(run),
				Snapshot:  snapshot,
			}, nil
		}
	}

	goal, dod, err := selectWorkspaceExecutionDefinitions(ctx, session, common.WorkspaceID(workspaceID), cmd)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	executionContext, err := buildWorkspaceExecutionContextWithDefinitions(ctx, session, workspace, goal, dod, cmd)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}

	runID, err := createAndStartWorkspaceExecutionRun(ctx, session, common.WorkspaceID(workspaceID), goal.ID, dod.ID, executionContext, cmd.Priority, cmd.LLMProviderID)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	startedRun, err := exports.Repositories.Runs.Get(ctx, runID)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	snapshot := activationSnapshot(workspace, startedRun, goal, dod)
	return WorkspaceExecutionActivation{
		Workspace: snapshot.Workspace,
		Run:       mapRun(startedRun),
		Snapshot:  snapshot,
	}, nil
}

func activationSnapshot(workspace *workspacemodel.Workspace, run *runmodel.Run, goal *workspacemodel.Goal, dod *workspacemodel.DoD) WorkspaceSnapshot {
	projectedWorkspace := mapWorkspaceWithRunProjection(workspace, run)
	if run == nil {
		ret := WorkspaceSnapshot{
			Workspace: projectedWorkspace,
		}
		if goal != nil {
			ret.Goals = []Goal{mapGoal(goal)}
		}
		if dod != nil {
			ret.DoDs = []DoD{mapDoD(dod)}
		}
		ret.Outcome = mapWorkspaceOutcome(ret)
		return ret
	}

	runView := mapRun(run)
	runSnapshot := RunSnapshot{
		Workspace: projectedWorkspace,
		Run:       runView,
	}
	runSnapshot.TaskProgress = mapTaskProgress(runSnapshot)
	runSnapshot.Outcome = mapRunOutcome(runSnapshot)

	ret := WorkspaceSnapshot{
		Workspace:    projectedWorkspace,
		Run:          &runView,
		Outcome:      mapWorkspaceOutcome(WorkspaceSnapshot{Workspace: projectedWorkspace, Runs: []Run{runView}, TaskProgress: runSnapshot.TaskProgress}),
		TaskProgress: runSnapshot.TaskProgress,
		Runs:         []Run{runView},
	}
	if goal != nil {
		ret.Goals = []Goal{mapGoal(goal)}
	}
	if dod != nil {
		ret.DoDs = []DoD{mapDoD(dod)}
	}
	return ret
}

func isActivationTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func latestWorkspaceGoal(items []*workspacemodel.Goal) *workspacemodel.Goal {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] != nil && strings.TrimSpace(items[i].Text) != "" {
			return items[i]
		}
	}
	return nil
}

func latestWorkspaceDoD(items []*workspacemodel.DoD) *workspacemodel.DoD {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] != nil && len(normalizeDoDItems(items[i].Items)) > 0 {
			return items[i]
		}
	}
	return nil
}

func orderedWorkspaceGoals(ctx context.Context, eventRecords repository.EventRecordRepository, workspaceID common.WorkspaceID, items []*workspacemodel.Goal) []*workspacemodel.Goal {
	return orderedWorkspaceDefinitions(ctx, eventRecords, workspaceID, events.TopicGoalDefined, items, func(item *workspacemodel.Goal) common.GoalID {
		if item == nil {
			return ""
		}
		return item.ID
	})
}

func orderedWorkspaceDoDs(ctx context.Context, eventRecords repository.EventRecordRepository, workspaceID common.WorkspaceID, items []*workspacemodel.DoD) []*workspacemodel.DoD {
	return orderedWorkspaceDefinitions(ctx, eventRecords, workspaceID, events.TopicDoDDefined, items, func(item *workspacemodel.DoD) common.DoDID {
		if item == nil {
			return ""
		}
		return item.ID
	})
}

func orderedWorkspaceDefinitions[T any, ID ~string](ctx context.Context, eventRecords repository.EventRecordRepository, workspaceID common.WorkspaceID, topic string, items []T, idOf func(T) ID) []T {
	ordered := append([]T(nil), items...)
	if eventRecords == nil || len(ordered) == 0 {
		return ordered
	}
	records, err := eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Topic: topic})
	if err != nil || len(records) == 0 {
		return ordered
	}
	byID := make(map[string]T, len(ordered))
	for _, item := range ordered {
		id := string(idOf(item))
		if id != "" {
			byID[id] = item
		}
	}
	type orderedRecord struct {
		id  string
		pos int
	}
	lastRecordByID := map[string]orderedRecord{}
	for idx, record := range records {
		if record == nil || record.SubjectID == "" {
			continue
		}
		if _, ok := byID[record.SubjectID]; !ok {
			continue
		}
		lastRecordByID[record.SubjectID] = orderedRecord{id: record.SubjectID, pos: idx}
	}
	eventOrder := make([]orderedRecord, 0, len(lastRecordByID))
	for _, record := range lastRecordByID {
		eventOrder = append(eventOrder, record)
	}
	sort.SliceStable(eventOrder, func(i, j int) bool {
		return eventOrder[i].pos < eventOrder[j].pos
	})
	ret := make([]T, 0, len(ordered))
	seen := map[string]struct{}{}
	for _, record := range eventOrder {
		item := byID[record.id]
		ret = append(ret, item)
		seen[record.id] = struct{}{}
	}
	for _, item := range ordered {
		id := string(idOf(item))
		if _, ok := seen[id]; ok {
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func workspaceDefinitionRevisionContexts(ctx context.Context, eventRecords repository.EventRecordRepository, workspaceID common.WorkspaceID, topic string) []map[string]any {
	if eventRecords == nil {
		return nil
	}
	records, err := eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Topic: topic})
	if err != nil || len(records) == 0 {
		return nil
	}
	type revision struct {
		id             string
		count          int
		lastEventID    string
		lastOccurredAt string
		lastPos        int
	}
	byID := map[string]*revision{}
	for idx, record := range records {
		if record == nil || record.SubjectID == "" {
			continue
		}
		item := byID[record.SubjectID]
		if item == nil {
			item = &revision{id: record.SubjectID}
			byID[record.SubjectID] = item
		}
		item.count++
		item.lastEventID = string(record.ID)
		item.lastOccurredAt = strings.TrimSpace(record.OccurredAt)
		item.lastPos = idx
	}
	items := make([]*revision, 0, len(byID))
	for _, item := range byID {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].lastPos < items[j].lastPos
	})
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.count <= 1 {
			continue
		}
		ret = append(ret, map[string]any{
			"id":             item.id,
			"revisionCount":  item.count,
			"lastEventId":    item.lastEventID,
			"lastOccurredAt": item.lastOccurredAt,
		})
	}
	return ret
}

func effectiveWorkspaceGoal(ctx context.Context, goals repository.GoalRepository, eventRecords repository.EventRecordRepository, workspaceID common.WorkspaceID, selected *workspacemodel.Goal) (*workspacemodel.Goal, []map[string]any, error) {
	if goals == nil || selected == nil {
		return selected, nil, nil
	}
	items, err := goals.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	items = orderedWorkspaceGoals(ctx, eventRecords, workspaceID, items)
	contexts := make([]map[string]any, 0, len(items))
	parts := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		contexts = append(contexts, goalContext(item))
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	effective := *selected
	if latest := latestWorkspaceGoal(items); latest != nil {
		effective = *latest
	}
	if len(parts) > 0 {
		effective.Text = strings.Join(parts, "\n")
	}
	return &effective, contexts, nil
}

func effectiveWorkspaceDoD(ctx context.Context, dods repository.DoDRepository, eventRecords repository.EventRecordRepository, workspaceID common.WorkspaceID, selected *workspacemodel.DoD) (*workspacemodel.DoD, []map[string]any, error) {
	if dods == nil || selected == nil {
		return selected, nil, nil
	}
	items, err := dods.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	items = orderedWorkspaceDoDs(ctx, eventRecords, workspaceID, items)
	contexts := make([]map[string]any, 0, len(items))
	mergedItems := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || (item.State != common.DoDStateReady && item.State != common.DoDStateSatisfied) {
			continue
		}
		normalized := normalizeDoDItems(item.Items)
		if len(normalized) == 0 {
			continue
		}
		contexts = append(contexts, dodContext(item))
		for _, entry := range normalized {
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			mergedItems = append(mergedItems, entry)
		}
	}
	effective := *selected
	if latest := latestWorkspaceDoD(items); latest != nil {
		effective = *latest
	}
	if len(mergedItems) > 0 {
		effective.Items = mergedItems
		effective.State = common.DoDStateReady
	}
	return &effective, contexts, nil
}

func activeRunDefinitionStale(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, run *runmodel.Run, selectedGoal *workspacemodel.Goal, selectedDoD *workspacemodel.DoD) bool {
	if run == nil || selectedGoal == nil || selectedDoD == nil {
		return true
	}
	if run.GoalID != selectedGoal.ID || run.DoDID != selectedDoD.ID {
		return true
	}
	if len(run.ExecutionContext) == 0 {
		return true
	}
	exports, err := session.Exports()
	if err != nil || exports.Repositories.Goals == nil || exports.Repositories.DoDs == nil {
		return true
	}
	effectiveGoal, _, err := effectiveWorkspaceGoal(ctx, exports.Repositories.Goals, exports.Repositories.EventRecords, workspaceID, selectedGoal)
	if err != nil || effectiveGoal == nil {
		return true
	}
	effectiveDoD, _, err := effectiveWorkspaceDoD(ctx, exports.Repositories.DoDs, exports.Repositories.EventRecords, workspaceID, selectedDoD)
	if err != nil || effectiveDoD == nil {
		return true
	}
	current := map[string]any{
		"goalId": string(effectiveGoal.ID),
		"goal":   goalContext(effectiveGoal),
		"doDId":  string(effectiveDoD.ID),
		"dod":    dodContext(effectiveDoD),
	}
	existing := map[string]any{
		"goalId": run.ExecutionContext["goalId"],
		"goal":   run.ExecutionContext["goal"],
		"doDId":  run.ExecutionContext["doDId"],
		"dod":    run.ExecutionContext["dod"],
	}
	return stableContextJSON(existing) != stableContextJSON(current)
}

func stableContextJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func ActivateWorkspaceExecutionWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
}

func ActivateWorkspaceExecutionFromEnv(ctx context.Context, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
}

func selectWorkspaceExecutionDefinitions(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, cmd ActivateWorkspaceExecutionCommand) (*workspacemodel.Goal, *workspacemodel.DoD, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}

	var goal *workspacemodel.Goal
	var dod *workspacemodel.DoD
	err = activationContextReadTasks(
		func() error {
			var err error
			goal, err = selectWorkspaceExecutionGoal(ctx, exports.Repositories.Goals.ListByWorkspaceID, exports.Repositories.EventRecords, workspaceID, common.GoalID(cmd.GoalID))
			return err
		},
		func() error {
			var err error
			dod, err = selectWorkspaceExecutionDoD(ctx, exports.Repositories.DoDs.ListByWorkspaceID, exports.Repositories.EventRecords, workspaceID, common.DoDID(cmd.DoDID))
			return err
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return goal, dod, nil
}

func selectWorkspaceExecutionGoal(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error),
	eventRecords repository.EventRecordRepository,
	workspaceID common.WorkspaceID,
	requestedID common.GoalID,
) (*workspacemodel.Goal, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	items = orderedWorkspaceGoals(ctx, eventRecords, workspaceID, items)
	latest := latestWorkspaceGoal(items)
	if requestedID != "" {
		for _, item := range items {
			if item != nil && item.ID == requestedID {
				if latest != nil {
					return latest, nil
				}
				return item, nil
			}
		}
		return nil, common.ErrConflict
	}
	if latest == nil {
		return nil, common.ErrInvalidArgument
	}
	return latest, nil
}

func selectWorkspaceExecutionDoD(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error),
	eventRecords repository.EventRecordRepository,
	workspaceID common.WorkspaceID,
	requestedID common.DoDID,
) (*workspacemodel.DoD, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	items = orderedWorkspaceDoDs(ctx, eventRecords, workspaceID, items)
	readyItems := make([]*workspacemodel.DoD, 0, len(items))
	for _, item := range items {
		if item != nil && (item.State == common.DoDStateReady || item.State == common.DoDStateSatisfied) {
			readyItems = append(readyItems, item)
		}
	}
	latest := latestWorkspaceDoD(readyItems)
	if requestedID != "" {
		for _, item := range readyItems {
			if item.ID == requestedID {
				if latest != nil {
					return latest, nil
				}
				return item, nil
			}
		}
		return nil, common.ErrConflict
	}
	if latest == nil {
		return nil, common.ErrInvalidArgument
	}
	return latest, nil
}

func buildWorkspaceExecutionContext(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, cmd ActivateWorkspaceExecutionCommand) (map[string]any, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	var goal *workspacemodel.Goal
	var dod *workspacemodel.DoD
	var workspace *workspacemodel.Workspace
	if err := activationContextReadTasks(
		func() error {
			var err error
			goal, err = exports.Repositories.Goals.Get(ctx, goalID)
			return err
		},
		func() error {
			var err error
			dod, err = exports.Repositories.DoDs.Get(ctx, dodID)
			return err
		},
		func() error {
			var err error
			workspace, err = exports.Repositories.Workspaces.Get(ctx, workspaceID)
			return err
		},
	); err != nil {
		return nil, err
	}
	return buildWorkspaceExecutionContextWithDefinitions(ctx, session, workspace, goal, dod, cmd)
}

func buildWorkspaceExecutionContextWithDefinitions(ctx context.Context, session *RuntimeSession, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, cmd ActivateWorkspaceExecutionCommand) (map[string]any, error) {
	if workspace == nil || goal == nil || dod == nil || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID {
		return nil, common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	effectiveGoal, workspaceGoals, err := effectiveWorkspaceGoal(ctx, exports.Repositories.Goals, exports.Repositories.EventRecords, workspace.ID, goal)
	if err != nil {
		return nil, err
	}
	effectiveDoD, workspaceDoDs, err := effectiveWorkspaceDoD(ctx, exports.Repositories.DoDs, exports.Repositories.EventRecords, workspace.ID, dod)
	if err != nil {
		return nil, err
	}
	goalRevisions := workspaceDefinitionRevisionContexts(ctx, exports.Repositories.EventRecords, workspace.ID, events.TopicGoalDefined)
	doDRevisions := workspaceDefinitionRevisionContexts(ctx, exports.Repositories.EventRecords, workspace.ID, events.TopicDoDDefined)
	contextData := map[string]any{
		"activation":  "workspace_execution",
		"workspaceId": string(workspace.ID),
		"goalId":      string(effectiveGoal.ID),
		"doDId":       string(effectiveDoD.ID),
	}
	contextData["executionContract"] = workspaceExecutionContract(ctx, exports.Repositories.WorkspaceTemplates, workspace)
	contextData["goal"] = goalContext(effectiveGoal)
	if len(workspaceGoals) > 0 {
		contextData["goals"] = workspaceGoals
		contextData["goalDefinitionPolicy"] = "workspace_goals_are_cumulative_latest_refines_previous"
	}
	contextData["dod"] = dodContext(effectiveDoD)
	if len(workspaceDoDs) > 0 {
		contextData["dods"] = workspaceDoDs
		contextData["doDDefinitionPolicy"] = "workspace_dods_are_cumulative_latest_refines_previous"
	}
	definitionChangeImpact := definitionChangeImpactFromDefinitions(effectiveGoal, workspaceGoals, goalRevisions, effectiveDoD, workspaceDoDs, doDRevisions)
	if len(definitionChangeImpact) > 0 {
		contextData["definitionChangeImpact"] = definitionChangeImpact
	}
	dialogLanguage := languageNameFromLocale(cmd.DialogLanguage)
	if dialogLanguage == "" {
		dialogLanguage = defaultDialogLanguage()
	}
	if dialogLanguage != "" {
		contextData["dialogLanguage"] = dialogLanguage
	}
	defaultMCPServerIDs := defaultMCPServerIDsFromContextHints(workspace.ContextHints)
	defaultCapabilityIDs := defaultCapabilityIDsFromContextHints(workspace.ContextHints)
	mcpServerIDs := appendDefaultIDs(normalizeStringIDs(cmd.MCPServerIDs), defaultMCPServerIDs)
	capabilityIDs := appendDefaultIDs(normalizeStringIDs(cmd.CapabilityIDs), defaultCapabilityIDs)
	memoryIDs := normalizeStringIDs(cmd.MemoryIDs)

	var defaultSoul *soulmodel.Soul
	var soulRouting SoulRouting
	var memories []*memorymodel.MemoryItem
	var retrievalEvidence map[string]any
	var governanceHistory []map[string]any
	err = activationContextReadTasks(
		func() error {
			var err error
			defaultSoul, soulRouting, _, err = resolveWorkspaceSoulRouting(ctx, exports, workspace, cmd)
			return err
		},
		func() error {
			var err error
			memories, retrievalEvidence, err = selectedMemoryItems(ctx, workspace.ID, exports.Services.MemoryService, memoryIDs, effectiveGoal, effectiveDoD)
			return err
		},
		func() error {
			governanceHistory = workspaceGovernanceHistory(ctx, exports.Repositories.Tickets, exports.Repositories.Resolutions, exports.Repositories.Runs, workspace.ID, effectiveGoal.ID, effectiveDoD.ID, 10)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	if defaultSoul != nil {
		contextData["soulId"] = string(defaultSoul.ID)
		contextData["soul"] = soulContext(defaultSoul)
	}
	contextData["soulRouting"] = soulRouting

	skills, skillIDs, skillSelection, err := selectSkillsForWorkspaceExecution(ctx, session, workspace.ID, effectiveGoal, effectiveDoD, cmd.SkillIDs, defaultSoul)
	if err != nil {
		return nil, err
	}
	contextData["skillSelection"] = skillSelection
	if len(skills) > 0 {
		contextData["skillIds"] = skillIDs
		contextData["skills"] = skillContexts(skills)
	}

	requiredMCPServerIDs := requiredMCPServerRefs(skills)
	requiredCapabilityIDs := requiredCapabilityRefs(skills)
	autoBoundMCPServerIDs := make([]string, 0)
	for _, id := range requiredMCPServerIDs {
		if !slices.Contains(mcpServerIDs, id) {
			mcpServerIDs = append(mcpServerIDs, id)
			autoBoundMCPServerIDs = append(autoBoundMCPServerIDs, id)
		}
	}
	autoBoundCapabilityIDs := make([]string, 0)
	for _, id := range requiredCapabilityIDs {
		if !slices.Contains(capabilityIDs, id) {
			capabilityIDs = append(capabilityIDs, id)
			autoBoundCapabilityIDs = append(autoBoundCapabilityIDs, id)
		}
	}
	contextData["mcpSelection"] = map[string]any{
		"policy":                 "template_default_and_skill_required_mcp_auto_bind",
		"requestedMCPServerIds":  normalizeStringIDs(cmd.MCPServerIDs),
		"defaultMCPServerIds":    defaultMCPServerIDs,
		"requiredMCPServerIds":   requiredMCPServerIDs,
		"autoBoundMCPServerIds":  autoBoundMCPServerIDs,
		"selectedMCPServerIds":   mcpServerIDs,
		"requestedCapabilityIds": normalizeStringIDs(cmd.CapabilityIDs),
		"defaultCapabilityIds":   defaultCapabilityIDs,
		"requiredCapabilityIds":  requiredCapabilityIDs,
		"autoBoundCapabilityIds": autoBoundCapabilityIDs,
		"selectedCapabilityIds":  capabilityIDs,
	}
	var mcpServers []*mcpmodel.Server
	var mcpCapabilities []*mcpmodel.Capability
	err = activationContextReadTasks(
		func() error {
			var err error
			mcpServers, err = selectedMCPServers(ctx, exports.Services.MCPService, mcpServerIDs, workspace)
			return err
		},
		func() error {
			var err error
			mcpCapabilities, err = selectedMCPCapabilities(ctx, exports.Services.MCPService, capabilityIDs)
			return err
		},
	)
	if err != nil {
		return nil, err
	}
	if len(mcpServers) > 0 {
		contextData["mcpServerIds"] = mcpServerIDs
		contextData["mcpServers"] = mcpServerContexts(mcpServers)
	}
	if len(mcpCapabilities) > 0 {
		contextData["capabilityIds"] = capabilityIDs
		contextData["mcpCapabilities"] = mcpCapabilityContexts(mcpCapabilities)
	}

	if err := validateSoulRoutingCapabilitySelection(ctx, exports.Services.SoulService, soulRouting, skillIDs, mcpServerIDs, capabilityIDs, capabilityCategories(mcpCapabilities), requiredToolKinds(skills)); err != nil {
		return nil, err
	}

	if retrievalEvidence != nil {
		contextData["memoryRetrieval"] = retrievalEvidence
	}
	if len(memories) > 0 {
		contextData["memoryIds"] = memoryItemIDs(memories)
		contextData["memories"] = memoryContexts(memories)
	}
	if len(governanceHistory) > 0 {
		contextData["governanceHistory"] = governanceHistory
		if !definitionChangeImpactChanged(definitionChangeImpact) {
			if replanContinuation := replanContinuationFromGovernanceHistory(governanceHistory); len(replanContinuation) > 0 {
				contextData["replanContinuation"] = replanContinuation
			}
		} else {
			contextData["definitionChangeContinuationPolicy"] = "definition_change_supersedes_prior_actionable_continuation"
		}
	}
	if !definitionChangeImpactChanged(definitionChangeImpact) {
		rejectedContinuation, err := rejectedResultContinuationForActivation(ctx, exports.Repositories.Results, workspace.ID, effectiveGoal.ID, effectiveDoD.ID)
		if err != nil {
			return nil, err
		}
		if len(rejectedContinuation) > 0 {
			contextData["rejectedResultContinuation"] = rejectedContinuation
		}
	} else {
		contextData["definitionChangeContinuationPolicy"] = "definition_change_supersedes_prior_actionable_continuation"
	}
	if len(workspace.EvidencePolicies) > 0 {
		contextData["evidencePolicies"] = cloneContextMapSlice(workspace.EvidencePolicies)
	}
	mergeActivationExecutionContextPatch(contextData, cmd.ExecutionContextPatch)

	return contextData, nil
}

func workspaceExecutionContract(ctx context.Context, templates repository.WorkspaceTemplateRepository, workspace *workspacemodel.Workspace) map[string]any {
	contract := map[string]any{
		"stateChangePolicy": "allowed_but_not_required",
	}
	if workspace == nil {
		return contract
	}
	hints := workspaceExecutionContractHints(ctx, templates, workspace)
	if value, ok := hints["stateChangePolicy"].(string); ok && strings.TrimSpace(value) != "" {
		contract["stateChangePolicy"] = strings.TrimSpace(value)
	}
	if profileContracts, ok := hints["profileContracts"].(map[string]any); ok && len(profileContracts) > 0 {
		contract["profileContracts"] = cloneContextValue(profileContracts)
	}
	if policies, ok := hints["capabilityUsagePolicies"].([]any); ok && len(policies) > 0 {
		contract["capabilityUsagePolicies"] = cloneContextValue(policies)
	}
	return contract
}

func workspaceExecutionContractHints(ctx context.Context, templates repository.WorkspaceTemplateRepository, workspace *workspacemodel.Workspace) map[string]any {
	if workspace == nil {
		return nil
	}
	workspaceHints := cloneContextMap(workspace.ContextHints)
	templateHints := matchingWorkspaceTemplateContextHints(ctx, templates, workspace)
	if len(templateHints) == 0 {
		return workspaceHints
	}
	return mergeTemplateWorkspaceContextHintsForExecutionContract(templateHints, workspaceHints)
}

func matchingWorkspaceTemplateContextHints(ctx context.Context, templates repository.WorkspaceTemplateRepository, workspace *workspacemodel.Workspace) map[string]any {
	if ctx == nil || templates == nil || workspace == nil {
		return nil
	}
	if templateID := strings.TrimSpace(workspace.TemplateID); templateID != "" {
		item, err := templates.Get(ctx, templateID)
		if err == nil && item != nil {
			return cloneContextMap(item.ContextHints)
		}
		return nil
	}
	items, err := templates.List(ctx)
	if err != nil {
		return nil
	}
	var selected map[string]any
	for _, item := range items {
		if item == nil || len(item.ContextHints) == 0 {
			continue
		}
		if !workspaceMatchesTemplateRouting(workspace, item) {
			continue
		}
		selected = cloneContextMap(item.ContextHints)
	}
	return selected
}

func workspaceMatchesTemplateRouting(workspace *workspacemodel.Workspace, template *repository.WorkspaceTemplate) bool {
	if workspace == nil || template == nil {
		return false
	}
	if strings.TrimSpace(template.DefaultSoulID) == "" && len(template.SoulBindings) == 0 {
		return false
	}
	if string(workspace.DefaultSoulID) != strings.TrimSpace(template.DefaultSoulID) {
		return false
	}
	if len(workspace.SoulBindings) != len(template.SoulBindings) {
		return false
	}
	for stage, templateSoulID := range template.SoulBindings {
		if string(workspace.SoulBindings[stage]) != strings.TrimSpace(templateSoulID) {
			return false
		}
	}
	return true
}

func mergeTemplateWorkspaceContextHintsForExecutionContract(templateHints map[string]any, workspaceHints map[string]any) map[string]any {
	if len(templateHints) == 0 {
		return cloneContextMap(workspaceHints)
	}
	ret := cloneContextMap(templateHints)
	for key, value := range workspaceHints {
		if key == "profileContracts" {
			ret[key] = mergeTemplateWorkspaceProfileContracts(contextMapValue(templateHints[key]), contextMapValue(value))
			continue
		}
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func mergeTemplateWorkspaceProfileContracts(templateProfiles map[string]any, workspaceProfiles map[string]any) map[string]any {
	if len(templateProfiles) == 0 {
		if cloned := cloneContextValue(workspaceProfiles); cloned != nil {
			if mapped, ok := cloned.(map[string]any); ok {
				return mapped
			}
		}
		return nil
	}
	ret := map[string]any{}
	for profileID, profile := range templateProfiles {
		ret[profileID] = cloneContextValue(profile)
	}
	for profileID, workspaceProfile := range workspaceProfiles {
		templateProfile, exists := ret[profileID]
		if !exists {
			ret[profileID] = cloneContextValue(workspaceProfile)
			continue
		}
		ret[profileID] = mergeTemplateWorkspaceProfileContract(contextMapValue(templateProfile), contextMapValue(workspaceProfile))
	}
	return ret
}

func mergeTemplateWorkspaceProfileContract(templateProfile map[string]any, workspaceProfile map[string]any) map[string]any {
	if len(templateProfile) == 0 {
		if cloned := cloneContextValue(workspaceProfile); cloned != nil {
			if mapped, ok := cloned.(map[string]any); ok {
				return mapped
			}
		}
		return nil
	}
	ret := map[string]any{}
	for key, value := range templateProfile {
		ret[key] = cloneContextValue(value)
	}
	for key, value := range workspaceProfile {
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func contextMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func mergeActivationExecutionContextPatch(contextData map[string]any, patch map[string]any) {
	if len(contextData) == 0 || len(patch) == 0 {
		return
	}
	reserved := map[string]struct{}{
		"activation": {}, "workspaceId": {}, "goalId": {}, "doDId": {},
		"goal": {}, "goals": {}, "goalDefinitionPolicy": {},
		"dod": {}, "dods": {}, "doDDefinitionPolicy": {},
		"definitionChangeImpact":             {},
		"definitionChangeContinuationPolicy": {},
		"rejectedResultContinuation":         {},
		"replanContinuation":                 {},
	}
	for key, value := range patch {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := reserved[key]; ok {
			continue
		}
		contextData[key] = value
	}
}

func definitionChangeImpactFromDefinitions(effectiveGoal *workspacemodel.Goal, goals []map[string]any, goalRevisions []map[string]any, effectiveDoD *workspacemodel.DoD, dods []map[string]any, doDRevisions []map[string]any) map[string]any {
	goalIDs := goalIDsFromContexts(goals)
	doDIDs := doDIDsFromContexts(dods)
	effectiveGoalID := ""
	if effectiveGoal != nil {
		effectiveGoalID = string(effectiveGoal.ID)
	}
	effectiveDoDID := ""
	if effectiveDoD != nil {
		effectiveDoDID = string(effectiveDoD.ID)
	}
	changed := len(goalIDs) > 1 || len(doDIDs) > 1 || len(goalRevisions) > 0 || len(doDRevisions) > 0
	if !changed {
		return nil
	}
	return map[string]any{
		"version":                        "workorch.definition_change_impact.v1",
		"changed":                        true,
		"policy":                         "workspace_goal_dod_cumulative_latest_refines_previous",
		"requiredPreExecutionAssessment": true,
		"goalChange": map[string]any{
			"effectiveGoalId":        effectiveGoalID,
			"latestGoalId":           latestDefinitionID(goalIDs, effectiveGoalID),
			"previousGoalIds":        previousDefinitionIDs(goalIDs, effectiveGoalID),
			"allGoalIds":             goalIDs,
			"definitionRevisions":    cloneContextMapSlice(goalRevisions),
			"changedInPlace":         len(goalRevisions) > 0,
			"allGoalsAreConstraints": true,
		},
		"doDChange": map[string]any{
			"effectiveDoDId":                effectiveDoDID,
			"latestDoDId":                   latestDefinitionID(doDIDs, effectiveDoDID),
			"previousDoDIds":                previousDefinitionIDs(doDIDs, effectiveDoDID),
			"allDoDIds":                     doDIDs,
			"definitionRevisions":           cloneContextMapSlice(doDRevisions),
			"changedInPlace":                len(doDRevisions) > 0,
			"allDoDItemsAreAcceptanceGates": true,
		},
		"strategyRequirements": []string{
			"preserve still-valid prior Goal and DoD constraints",
			"identify additive requirements and conflicts before state-changing work",
			"choose state-change, evidence, and verification targets before state-changing work",
			"do not weaken prior acceptance evidence or constraints",
		},
	}
}

func definitionChangeImpactChanged(impact map[string]any) bool {
	if len(impact) == 0 {
		return false
	}
	value, ok := impact["changed"].(bool)
	return ok && value
}

func latestDefinitionID(ids []string, effectiveID string) string {
	if strings.TrimSpace(effectiveID) != "" {
		return strings.TrimSpace(effectiveID)
	}
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}

func previousDefinitionIDs(ids []string, effectiveID string) []string {
	ret := make([]string, 0, len(ids))
	effectiveID = strings.TrimSpace(effectiveID)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == effectiveID {
			continue
		}
		ret = append(ret, id)
	}
	return ret
}

func activationContextReadTasks(tasks ...func() error) error {
	return runReadModelTasks(tasks...)
}

func workspaceGovernanceHistory(
	ctx context.Context,
	tickets repository.DecisionTicketRepository,
	resolutions repository.DecisionResolutionRepository,
	runs repository.RunRepository,
	workspaceID common.WorkspaceID,
	goalID common.GoalID,
	dodID common.DoDID,
	limit int,
) []map[string]any {
	if tickets == nil || resolutions == nil || runs == nil || workspaceID == "" || limit <= 0 {
		return nil
	}
	workspaceTickets, err := tickets.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil
	}
	ret := make([]map[string]any, 0, min(limit, len(workspaceTickets)))
	for _, ticket := range workspaceTickets {
		if ticket == nil || ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		run, err := runs.Get(ctx, ticket.RunID)
		if err != nil || run == nil || run.WorkspaceID != workspaceID {
			continue
		}
		if goalID != "" && run.GoalID != goalID {
			continue
		}
		if dodID != "" && run.DoDID != dodID {
			continue
		}
		resolution, err := resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil || resolution == nil {
			continue
		}
		ret = append(ret, common.RedactSensitiveMap(map[string]any{
			"ticketId":          string(ticket.ID),
			"runId":             string(ticket.RunID),
			"stepId":            string(ticket.StepID),
			"decisionKind":      ticket.DecisionKind,
			"reason":            ticket.Reason,
			"question":          ticket.Question,
			"ticketContext":     compactActivationFailureContext(ticket.Context),
			"resolutionId":      string(resolution.ID),
			"resolutionType":    resolution.Type,
			"resolutionPayload": compactActivationDecisionPayload(resolution.Payload),
		}))
		if len(ret) >= limit {
			break
		}
	}
	return ret
}

func selectedSkills(ctx context.Context, session *RuntimeSession, ids []string, workspaceID string) ([]Skill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Repositories.Skills == nil {
		return nil, common.ErrInvalidArgument
	}
	type skillResult struct {
		item Skill
		err  error
	}
	results := make([]skillResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, sid string) {
			defer wg.Done()
			item, err := exports.Repositories.Skills.Get(ctx, common.SkillID(sid))
			if err == nil {
				results[idx].item = mapSkill(item)
				return
			}
			if !errors.Is(err, common.ErrNotFound) {
				results[idx].err = err
				return
			}
			results[idx].item, results[idx].err = readSkillHubSkill(ctx, session, sid, SkillQuery{WorkspaceID: workspaceID})
		}(i, id)
	}
	wg.Wait()
	ret := make([]Skill, 0, len(ids))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		ret = append(ret, r.item)
	}
	return ret, nil
}

func selectedMCPServers(ctx context.Context, service interface {
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
}, ids []string, workspace *workspacemodel.Workspace) ([]*mcpmodel.Server, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if service == nil {
		return nil, common.ErrInvalidArgument
	}
	type serverResult struct {
		item *mcpmodel.Server
		err  error
	}
	results := make([]serverResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, sid string) {
			defer wg.Done()
			results[idx].item, results[idx].err = service.GetServer(ctx, common.MCPServerID(sid))
		}(i, id)
	}
	wg.Wait()
	ret := make([]*mcpmodel.Server, 0, len(ids))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if !r.item.Enabled {
			return nil, common.ErrConflict
		}
		ret = append(ret, mcpServerForWorkspace(r.item, workspace))
	}
	return ret, nil
}

func mcpServerForWorkspace(item *mcpmodel.Server, workspace *workspacemodel.Workspace) *mcpmodel.Server {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Args = cloneStrings(item.Args)
	cloned.AllowedTools = cloneStrings(item.AllowedTools)
	cloned.AllowedResources = cloneStrings(item.AllowedResources)
	cloned.AllowedPrompts = cloneStrings(item.AllowedPrompts)
	cloned.Env = cloneMCPEnvForWorkspace(item.Env, workspace)
	return &cloned
}

func cloneMCPEnvForWorkspace(env map[string]string, workspace *workspacemodel.Workspace) map[string]string {
	if env == nil {
		return nil
	}
	ret := make(map[string]string, len(env))
	for key, value := range env {
		ret[key] = expandWorkspaceContextPlaceholders(value, workspace)
	}
	return ret
}

func expandWorkspaceContextPlaceholders(value string, workspace *workspacemodel.Workspace) string {
	if workspace == nil {
		return value
	}
	replacements := map[string]string{
		"{workspaceId}":                   string(workspace.ID),
		"{workspaceDeliverableDirectory}": workspace.DeliverableDirectory,
		"{workspaceInternalDirectory}":    workspace.InternalDirectory,
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func selectedMCPCapabilities(ctx context.Context, service interface {
	GetCapability(context.Context, common.MCPCapabilityID) (*mcpmodel.Capability, error)
}, ids []string) ([]*mcpmodel.Capability, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if service == nil {
		return nil, common.ErrInvalidArgument
	}
	type capResult struct {
		item *mcpmodel.Capability
		err  error
	}
	results := make([]capResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, cid string) {
			defer wg.Done()
			results[idx].item, results[idx].err = service.GetCapability(ctx, common.MCPCapabilityID(cid))
		}(i, id)
	}
	wg.Wait()
	ret := make([]*mcpmodel.Capability, 0, len(ids))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if !r.item.Enabled {
			return nil, common.ErrConflict
		}
		ret = append(ret, r.item)
	}
	return ret, nil
}

func defaultMCPServerIDsFromContextHints(contextHints map[string]any) []string {
	return defaultIDsFromContextHints(contextHints, "defaultMCPServerIds")
}

func defaultCapabilityIDsFromContextHints(contextHints map[string]any) []string {
	return defaultIDsFromContextHints(contextHints, "defaultCapabilityIds")
}

func defaultIDsFromContextHints(contextHints map[string]any, key string) []string {
	if len(contextHints) == 0 {
		return nil
	}
	ret := make([]string, 0)
	for _, value := range contextHints {
		config, ok := value.(map[string]any)
		if !ok {
			continue
		}
		ret = appendDefaultIDs(ret, normalizeStringIDs(stringSliceFromAny(config[key])))
	}
	return ret
}

func appendDefaultIDs(base []string, defaults []string) []string {
	ret := cloneStrings(base)
	for _, id := range defaults {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(ret, id) {
			continue
		}
		ret = append(ret, id)
	}
	return ret
}

func selectedMemoryItems(ctx context.Context, workspaceID common.WorkspaceID, service interface {
	GetMemoryItem(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	ListMemoryByWorkspace(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}, ids []string, goal *workspacemodel.Goal, dod *workspacemodel.DoD) ([]*memorymodel.MemoryItem, map[string]any, error) {
	if service == nil {
		if len(ids) > 0 {
			return nil, nil, common.ErrInvalidArgument
		}
		return nil, nil, nil
	}
	if len(ids) > 0 {
		type memResult struct {
			item *memorymodel.MemoryItem
			err  error
		}
		results := make([]memResult, len(ids))
		var wg sync.WaitGroup
		for i, id := range ids {
			wg.Add(1)
			go func(idx int, mid string) {
				defer wg.Done()
				results[idx].item, results[idx].err = service.GetMemoryItem(ctx, common.MemoryID(mid))
			}(i, id)
		}
		wg.Wait()
		ret := make([]*memorymodel.MemoryItem, 0, len(ids))
		for _, r := range results {
			if r.err != nil {
				return nil, nil, r.err
			}
			if r.item.WorkspaceID != workspaceID {
				return nil, nil, common.ErrConflict
			}
			if activationMemoryVisibleToLLM(r.item) {
				ret = append(ret, r.item)
			}
		}
		status := "selected"
		if len(ret) == 0 && len(results) > 0 {
			status = "none_selected"
		}
		return ret, map[string]any{
			"policy":             "explicit_memory_selection",
			"status":             status,
			"retrievedMemoryIds": memoryItemIDs(ret),
		}, nil
	}
	semanticItems, semanticEvidence, semanticHandled := semanticMemoryItems(ctx, workspaceID, service, goal, dod)
	if semanticHandled {
		selected := activationVisibleMemoryItems(semanticItems, 8)
		if semanticEvidence == nil {
			semanticEvidence = map[string]any{}
		}
		semanticEvidence["retrievedMemoryIds"] = memoryItemIDs(selected)
		if len(selected) == 0 && len(semanticItems) > 0 {
			semanticEvidence["status"] = "none_selected"
		}
		return selected, semanticEvidence, nil
	}
	items, err := service.ListMemoryByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	selected := activationVisibleMemoryItems(items, 8)
	status := "selected"
	if len(selected) == 0 && len(items) > 0 {
		status = "none_selected"
	}
	return selected, map[string]any{
		"policy":             "deterministic_workspace_list",
		"status":             status,
		"candidateMemoryIds": memoryItemIDs(items),
		"excludedMemoryIds":  memoryItemIDs(activationExcludedMemoryItems(items, selected)),
		"retrievedMemoryIds": memoryItemIDs(selected),
	}, nil
}

func activationVisibleMemoryItems(items []*memorymodel.MemoryItem, limit int) []*memorymodel.MemoryItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	ret := make([]*memorymodel.MemoryItem, 0, min(len(items), limit))
	for _, item := range items {
		if len(ret) >= limit {
			break
		}
		if activationMemoryVisibleToLLM(item) {
			ret = append(ret, item)
		}
	}
	return ret
}

func activationExcludedMemoryItems(candidates []*memorymodel.MemoryItem, selected []*memorymodel.MemoryItem) []*memorymodel.MemoryItem {
	if len(candidates) == 0 {
		return nil
	}
	selectedIDs := map[common.MemoryID]struct{}{}
	for _, item := range selected {
		if item != nil && item.ID != "" {
			selectedIDs[item.ID] = struct{}{}
		}
	}
	ret := make([]*memorymodel.MemoryItem, 0, len(candidates)-len(selectedIDs))
	for _, item := range candidates {
		if item == nil || item.ID == "" {
			continue
		}
		if _, ok := selectedIDs[item.ID]; ok {
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func activationMemoryVisibleToLLM(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(item.MemoryType) != "tool_feedback" {
		return true
	}
	if activationMemoryHasTag(item, "accepted_input") {
		return true
	}
	return !activationMemoryHasTag(item, "provider_retry")
}

func activationMemoryHasTag(item *memorymodel.MemoryItem, tag string) bool {
	if item == nil {
		return false
	}
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return false
	}
	for _, candidate := range item.Tags {
		if strings.TrimSpace(strings.ToLower(candidate)) == tag {
			return true
		}
	}
	return false
}

func semanticMemoryItems(ctx context.Context, workspaceID common.WorkspaceID, service interface {
	SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}, goal *workspacemodel.Goal, dod *workspacemodel.DoD) ([]*memorymodel.MemoryItem, map[string]any, bool) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return nil, map[string]any{
			"policy": "semantic_vector_search",
			"status": "degraded",
			"reason": err.Error(),
		}, true
	}
	providerID := strings.TrimSpace(cfg.LLM.DefaultEmbeddingProvider)
	if providerID == "" {
		return nil, nil, false
	}
	queryText := memoryRetrievalQueryText(goal, dod)
	evidence := map[string]any{
		"policy":              "semantic_vector_search",
		"providerId":          providerID,
		"operation":           llmcommon.ProviderOperationEmbeddings,
		"queryTextPreview":    previewRetrievalQuery(queryText),
		"queryTextSha256":     sha256Hex(queryText),
		"minScore":            0.1,
		"limit":               8,
		"retrievedMemoryIds":  []string{},
		"retrievedMemoryHits": []map[string]any{},
	}
	defaultProviderID, providerConfigs, err := llmbiz.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	providerConfig, ok := providerConfigs[providerID]
	if !ok {
		evidence["status"] = "degraded"
		evidence["reason"] = fmt.Sprintf("embedding provider %q is not configured", providerID)
		return nil, evidence, true
	}
	if providerConfig.Unavailable {
		evidence["status"] = "degraded"
		evidence["reason"] = providerConfig.UnavailableReason
		return nil, evidence, true
	}
	adapter := llmbiz.NewRuntimeAdapterFromConfigs(defaultProviderID, providerConfigs)
	embedding, err := adapter.GenerateEmbedding(ctx, providerID, queryText)
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	items, err := service.SearchMemory(ctx, repository.MemorySearchQuery{
		WorkspaceID: workspaceID,
		Vector:      embedding.Vector,
		MinScore:    0.1,
		Limit:       8,
	})
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	evidence["status"] = "selected"
	evidence["embeddingModel"] = embedding.Model
	evidence["vectorDimension"] = len(embedding.Vector)
	evidence["retrievedMemoryIds"] = memoryItemIDs(items)
	evidence["retrievedMemoryHits"] = memoryRetrievalHits(items)
	return items, evidence, true
}

func normalizeStringIDs(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		item = strings.TrimSpace(item)
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

func requiredToolKinds(skills []Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, kind := range skill.RequiredToolKinds {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			ret = append(ret, kind)
		}
	}
	return ret
}

func requiredMCPServerRefs(skills []Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, id := range skill.RequiredMCPServerRefs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ret = append(ret, id)
		}
	}
	return ret
}

func requiredCapabilityRefs(skills []Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, id := range skill.RequiredCapabilityRefs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ret = append(ret, id)
		}
	}
	return ret
}

func capabilityCategories(items []*mcpmodel.Capability) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range items {
		category := strings.TrimSpace(item.Category)
		if category == "" {
			continue
		}
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		ret = append(ret, category)
	}
	return ret
}

func memoryItemIDs(items []*memorymodel.MemoryItem) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, string(item.ID))
	}
	return ret
}

func memoryRetrievalHits(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":              string(item.ID),
			"summary":         item.Summary,
			"vectorScore":     item.VectorScore,
			"vectorDimension": len(item.Vector),
		})
	}
	return ret
}

func memoryRetrievalQueryText(goal *workspacemodel.Goal, dod *workspacemodel.DoD) string {
	parts := []string{}
	if goal != nil && strings.TrimSpace(goal.Text) != "" {
		parts = append(parts, "Goal: "+strings.TrimSpace(goal.Text))
	}
	if dod != nil && len(dod.Items) > 0 {
		parts = append(parts, "Definition of Done:")
		for _, item := range dod.Items {
			item = strings.TrimSpace(item)
			if item != "" {
				parts = append(parts, "- "+item)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func previewRetrievalQuery(text string) string {
	text = strings.TrimSpace(text)
	const limit = 240
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

func goalContext(item *workspacemodel.Goal) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":          string(item.ID),
		"workspaceId": string(item.WorkspaceID),
		"text":        item.Text,
	}
}

func dodContext(item *workspacemodel.DoD) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":          string(item.ID),
		"workspaceId": string(item.WorkspaceID),
		"items":       cloneStrings(item.Items),
		"state":       string(item.State),
	}
}

func soulContext(item *soulmodel.Soul) map[string]any {
	return map[string]any{
		"id":                   string(item.ID),
		"name":                 item.Name,
		"description":          item.Description,
		"businessDomains":      cloneStrings(item.BusinessDomains),
		"soulMd":               item.SoulMD,
		"allowedSkillRefs":     cloneStrings(item.AllowedSkillRefs),
		"allowedMCPServerRefs": cloneStrings(item.AllowedMCPServerRefs),
		"allowedToolKinds":     cloneStrings(item.AllowedToolKinds),
	}
}

func skillContexts(items []Skill) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":                     item.ID,
			"name":                   item.Name,
			"version":                item.Version,
			"description":            item.Description,
			"triggerRules":           cloneStrings(item.TriggerRules),
			"instruction":            item.Instruction,
			"referenceRefs":          cloneStrings(item.ReferenceRefs),
			"requiredToolKinds":      cloneStrings(item.RequiredToolKinds),
			"requiredMCPServerRefs":  cloneStrings(item.RequiredMCPServerRefs),
			"requiredCapabilityRefs": cloneStrings(item.RequiredCapabilityRefs),
			"constraints":            cloneStrings(item.Constraints),
			"source":                 item.Source,
			"scope":                  item.Scope,
			"workspaceId":            item.WorkspaceID,
			"status":                 item.Status,
			"sourceRepository":       item.SourceRepository,
			"sourcePath":             item.SourcePath,
			"targetPath":             item.TargetPath,
			"localPath":              item.LocalPath,
			"repoPath":               item.RepoPath,
			"globalPath":             item.GlobalPath,
			"projectPath":            item.ProjectPath,
			"contentHash":            item.ContentHash,
			"sourceHash":             item.SourceHash,
			"appliedHash":            item.AppliedHash,
			"actualHash":             item.ActualHash,
			"agent":                  item.Agent,
			"managedBy":              item.ManagedBy,
			"sourceSkillId":          item.SourceSkillID,
		})
	}
	return ret
}

func mcpServerContexts(items []*mcpmodel.Server) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":                    string(item.ID),
			"name":                  item.Name,
			"transport":             item.Transport,
			"enabled":               item.Enabled,
			"command":               item.Command,
			"args":                  cloneStrings(item.Args),
			"env":                   redactedMCPEnvContext(item.Env),
			"allowedTools":          cloneStrings(item.AllowedTools),
			"allowedResources":      cloneStrings(item.AllowedResources),
			"defaultApprovalPolicy": item.DefaultApprovalPolicy,
			"timeoutMillis":         item.TimeoutMillis,
		})
	}
	return ret
}

func redactedMCPEnvContext(env map[string]string) map[string]any {
	if len(env) == 0 {
		return nil
	}
	ret := make(map[string]any, len(env))
	for key, value := range env {
		ret[key] = map[string]any{
			"present":  value != "",
			"redacted": value != "",
		}
	}
	return ret
}

func mcpCapabilityContexts(items []*mcpmodel.Capability) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":             string(item.ID),
			"serverId":       string(item.ServerID),
			"kind":           item.Kind,
			"name":           item.Name,
			"displayName":    item.DisplayName,
			"description":    item.Description,
			"category":       item.Category,
			"tags":           cloneStrings(item.Tags),
			"usageHint":      item.UsageHint,
			"inputSummary":   item.InputSummary,
			"outputSummary":  item.OutputSummary,
			"stageHints":     cloneStrings(item.StageHints),
			"riskLevel":      item.RiskLevel,
			"uri":            item.URI,
			"enabled":        item.Enabled,
			"approvalPolicy": item.ApprovalPolicy,
			"status":         item.Status,
		})
	}
	return ret
}

func rejectedResultContinuationForActivation(ctx context.Context, results repository.ResultRepository, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) (map[string]any, error) {
	if results == nil || workspaceID == "" || goalID == "" || dodID == "" {
		return nil, nil
	}
	items, err := results.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	latest := latestTerminalResultForDefinitions(items, goalID, dodID)
	if latest == nil || latest.State != common.ResultStateRejected {
		return nil, nil
	}
	return rejectedResultContinuationContext(latest), nil
}

func latestTerminalResultForDefinitions(items []*resultmodel.Result, goalID common.GoalID, dodID common.DoDID) *resultmodel.Result {
	var latest *resultmodel.Result
	latestKey := ""
	for idx, item := range items {
		if item == nil || item.GoalID != goalID || item.DoDID != dodID || !isTerminalResultState(item.State) {
			continue
		}
		key := resultOrderingKey(item, idx)
		if latest == nil || key > latestKey {
			latest = item
			latestKey = key
		}
	}
	return latest
}

func isTerminalResultState(state common.ResultState) bool {
	switch state {
	case common.ResultStateAccepted, common.ResultStateRejected:
		return true
	default:
		return false
	}
}

func resultOrderingKey(item *resultmodel.Result, index int) string {
	if item == nil {
		return fmt.Sprintf("%020d", index)
	}
	timestamp := strings.TrimSpace(item.UpdatedAt)
	if timestamp == "" {
		timestamp = strings.TrimSpace(item.CreatedAt)
	}
	return fmt.Sprintf("%s|%020d", timestamp, index)
}

func rejectedResultContinuationContext(item *resultmodel.Result) map[string]any {
	if item == nil {
		return nil
	}
	ret := map[string]any{
		"mode":                "rejected_result_continuation",
		"predecessorRunId":    string(item.RunID),
		"resultId":            string(item.ID),
		"resultState":         string(item.State),
		"sourceType":          strings.TrimSpace(item.SourceType),
		"sourceRef":           strings.TrimSpace(item.SourceRef),
		"summary":             compactActivationContextValue(item.Summary, 1),
		"goalOutcome":         compactActivationContextValue(item.SummaryDetail.GoalOutcome, 1),
		"satisfiedDoDItems":   rejectedContinuationDoDItems(item.DoDAssessment.Items, true),
		"unsatisfiedDoDItems": rejectedContinuationDoDItems(item.DoDAssessment.Items, false),
		"coverageGaps": map[string]any{
			"missingGoalCoverage":         rejectedContinuationCoverageGaps(item.DoDAssessment.GoalCoverage, "goal"),
			"missingAcceptanceDimensions": rejectedContinuationCoverageGaps(item.DoDAssessment.AcceptanceDimensions, "acceptance"),
			"missingDeliverableCoverage":  rejectedContinuationCoverageGaps(item.DoDAssessment.DeliverableCoverage, "deliverable"),
		},
		"requiredEvidence":   rejectedContinuationRequiredEvidence(item.SummaryDetail),
		"resultEvidenceRefs": compactStringRefs(item.EvidenceRefs, 20),
		"artifactRefs":       compactArtifactRefs(item.ArtifactRefs, 20),
	}
	if diagnostics := rejectedContinuationDiagnostics(item.SummaryDetail.Diagnostics); len(diagnostics) > 0 {
		ret["diagnostics"] = diagnostics
	}
	if refs := compactStringRefs(item.SummaryDetail.EvidenceRefs, 20); len(refs) > 0 {
		ret["summaryEvidenceRefs"] = refs
	}
	if refs := compactArtifactRefs(item.SummaryDetail.ArtifactRefs, 20); len(refs) > 0 {
		ret["summaryArtifactRefs"] = refs
	}
	return stripRuntimeLLMBindingFromActivationContext(common.RedactSensitiveMap(ret))
}

func rejectedContinuationDiagnostics(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"gateFailureReason", "rejectionReason", "sourceSummary"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			ret[key] = compactActivationContextValue(value, 1)
		}
	}
	if diagnostics := contextMapValue(input["runtimeDiagnostics"]); len(diagnostics) > 0 {
		ret["runtimeDiagnostics"] = compactActivationContextValue(diagnostics, 1)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func rejectedContinuationDoDItems(items []resultmodel.DoDAssessmentItem, satisfied bool) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		state := strings.TrimSpace(item.State)
		isSatisfied := strings.EqualFold(state, "satisfied")
		if satisfied != isSatisfied {
			continue
		}
		mapped := map[string]any{
			"item":         compactActivationContextValue(item.Item, 1),
			"evidenceRefs": compactStringRefs(item.EvidenceRefs, 10),
		}
		if !satisfied {
			mapped["state"] = state
			mapped["reason"] = compactActivationContextValue(item.FailureReason, 1)
		}
		ret = append(ret, mapped)
		if len(ret) >= 20 {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func rejectedContinuationCoverageGaps(coverage *resultmodel.DoDCoverage, scope string) []map[string]any {
	if coverage == nil || len(coverage.Items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(coverage.Items))
	for _, item := range coverage.Items {
		if rejectedContinuationCoverageItemSatisfied(item, scope) {
			continue
		}
		mapped := map[string]any{}
		for key, value := range rejectedContinuationCoverageIdentity(item) {
			mapped[key] = compactActivationContextValue(value, 1)
		}
		mapped["state"] = strings.TrimSpace(item.State)
		if refs := compactStringRefs(item.EvidenceRefs, 10); len(refs) > 0 {
			mapped["evidenceRefs"] = refs
		}
		if refs := compactArtifactRefs(item.ArtifactRefs, 10); len(refs) > 0 {
			mapped["artifactRefs"] = refs
		}
		ret = append(ret, mapped)
		if len(ret) >= 20 {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func rejectedContinuationCoverageItemSatisfied(item resultmodel.DoDCoverageItem, scope string) bool {
	if !strings.EqualFold(strings.TrimSpace(item.State), "satisfied") {
		return false
	}
	if scope == "deliverable" {
		return len(compactStringRefs(item.EvidenceRefs, 1)) > 0 || len(compactArtifactRefs(item.ArtifactRefs, 1)) > 0
	}
	return len(compactStringRefs(item.EvidenceRefs, 1)) > 0
}

func rejectedContinuationCoverageIdentity(item resultmodel.DoDCoverageItem) map[string]string {
	ret := map[string]string{}
	for key, value := range map[string]string{
		"id":          item.ID,
		"deliverable": item.Deliverable,
		"name":        item.Name,
		"path":        item.Path,
		"goal":        item.Goal,
		"goalItem":    item.GoalItem,
		"item":        item.Item,
		"requirement": item.Requirement,
	} {
		if strings.TrimSpace(value) != "" {
			ret[key] = strings.TrimSpace(value)
		}
	}
	return ret
}

func rejectedContinuationRequiredEvidence(summary resultmodel.ResultSummary) []any {
	values := make([]string, 0, len(summary.NextActions)+len(summary.ResidualRisks)+len(summary.KnownLimitations))
	values = append(values, summary.NextActions...)
	values = append(values, summary.ResidualRisks...)
	values = append(values, summary.KnownLimitations...)
	return compactStringRefs(values, 20)
}

func compactStringRefs(input []string, limit int) []any {
	if len(input) == 0 || limit <= 0 {
		return nil
	}
	ret := make([]any, 0, min(len(input), limit))
	seen := map[string]struct{}{}
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, compactActivationContextValue(item, 1))
		if len(ret) >= limit {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactArtifactRefs(input []common.ArtifactID, limit int) []any {
	if len(input) == 0 || limit <= 0 {
		return nil
	}
	ret := make([]any, 0, min(len(input), limit))
	seen := map[common.ArtifactID]struct{}{}
	for _, item := range input {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, compactActivationContextValue(string(item), 1))
		if len(ret) >= limit {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func replanContinuationFromGovernanceHistory(history []map[string]any) map[string]any {
	for idx := len(history) - 1; idx >= 0; idx-- {
		item := history[idx]
		if len(item) == 0 {
			continue
		}
		resolutionType := strings.TrimSpace(stringFromDecisionPayload(item, "resolutionType"))
		resolutionPayload := decisionPayloadMap(item["resolutionPayload"])
		recoveryAction := strings.TrimSpace(stringFromDecisionPayload(resolutionPayload, "recoveryAction"))
		if resolutionType != "replan" && recoveryAction != "replan" {
			continue
		}
		replan := map[string]any{
			"predecessorRunId": stringFromDecisionPayload(item, "runId"),
			"reason":           "resolved_governance_replan",
			"ticketId":         stringFromDecisionPayload(item, "ticketId"),
			"ticketStepId":     stringFromDecisionPayload(item, "stepId"),
			"ticketReason":     stringFromDecisionPayload(item, "reason"),
			"resolutionId":     stringFromDecisionPayload(item, "resolutionId"),
			"resolutionType":   resolutionType,
		}
		if len(resolutionPayload) > 0 {
			replan["resolutionPayload"] = compactActivationDecisionPayload(resolutionPayload)
		}
		if failure := replanFailureContextFromGovernanceItem(item); len(failure) > 0 {
			replan["failureContext"] = failure
			replan["ticketContext"] = failure
		}
		return replan
	}
	return nil
}

func replanFailureContextFromGovernanceItem(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	context := decisionPayloadMap(item["ticketContext"])
	if len(context) == 0 {
		context = decisionPayloadMap(item["context"])
	}
	if len(context) > 0 {
		return compactActivationFailureContext(context)
	}
	ret := map[string]any{}
	for _, key := range []string{
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"repairAttempts",
		"failedStepOutput",
		"recoveryPolicy",
	} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = compactActivationContextValue(value, 1)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactActivationDecisionPayload(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"answer",
		"note",
		"operator",
		"recoveryAction",
		"resumeInstruction",
		"resolution",
		"summary",
		"reason",
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"attempts",
		"maxAttempts",
		"repairOfRootStepId",
		"recoveryPolicy",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactActivationContextValue(value, 1)
		}
	}
	for _, key := range []string{"failureContext", "ticketContext", "context"} {
		if context := decisionPayloadMap(input[key]); len(context) > 0 {
			ret[key] = compactActivationFailureContext(context)
		}
	}
	return stripRuntimeLLMBindingFromActivationContext(common.RedactSensitiveMap(ret))
}

func compactActivationFailureContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"failureReason",
		"repairObjective",
		"attempts",
		"maxAttempts",
		"repairAttempts",
		"repairOfRootStepId",
		"runtimeRepairAttempt",
		"maxRuntimeRepairAttempts",
		"recoveryPolicy",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactActivationContextValue(value, 1)
		}
	}
	if failedOutput := decisionPayloadMap(input["failedStepOutput"]); len(failedOutput) > 0 {
		ret["failedStepOutput"] = compactActivationFailedOutput(failedOutput)
	}
	if targets := compactActivationTargetSnapshots(input); len(targets) > 0 {
		ret["targetSnapshots"] = targets
	}
	return stripRuntimeLLMBindingFromActivationContext(common.RedactSensitiveMap(ret))
}

func compactActivationFailedOutput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"stage",
		"status",
		"error",
		"reason",
		"failureClass",
		"failureSignature",
		"failureCategory",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactActivationContextValue(value, 1)
		}
	}
	if graph := decisionPayloadMap(input["executionGraph"]); len(graph) > 0 {
		if nodes, ok := graph["nodes"].([]any); ok {
			ret["rejectedExecutionGraph"] = map[string]any{"nodeCount": len(nodes)}
		} else if nodes, ok := graph["nodes"].([]map[string]any); ok {
			ret["rejectedExecutionGraph"] = map[string]any{"nodeCount": len(nodes)}
		}
	}
	if _, ok := input["rawContentPreview"]; ok {
		ret["rawContentPreviewOmitted"] = true
	}
	if _, ok := input["repairRawContentPreview"]; ok {
		ret["repairRawContentPreviewOmitted"] = true
	}
	if _, ok := input["streamEvents"]; ok {
		ret["streamEventsOmitted"] = true
	}
	if ret["rawContentPreviewOmitted"] == true ||
		ret["repairRawContentPreviewOmitted"] == true ||
		ret["streamEventsOmitted"] == true {
		ret["omittedContentReason"] = "invalid or rejected LLM decision content is not reusable repair input; rebuild from current context, target snapshots, or fresh reads"
	}
	return ret
}

func compactActivationTargetSnapshots(input map[string]any) []map[string]any {
	var ret []map[string]any
	seen := map[string]bool{}
	add := func(raw any, source string) {
		for _, item := range activationMapSlice(raw) {
			targetRef := strings.TrimSpace(stringFromDecisionPayload(item, "targetRef"))
			if targetRef == "" || seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			snapshot := map[string]any{}
			for _, key := range []string{
				"targetRef",
				"exists",
				"contentHash",
				"contentComplete",
				"safeForFileWrite",
				"redacted",
				"truncated",
				"isBinary",
				"size",
				"source",
				"error",
			} {
				if value, ok := item[key]; ok && value != nil {
					snapshot[key] = compactActivationContextValue(value, 1)
				}
			}
			if source != "" {
				snapshot["contextSource"] = source
			}
			ret = append(ret, snapshot)
			if len(ret) >= 20 {
				return
			}
		}
	}
	add(input["targetSnapshots"], "lineage_target")
	if len(ret) < 20 {
		add(input["failureTargetRepairContext"], "failure_target")
	}
	if len(ret) < 20 {
		add(input["patchRepairContext"], "patch_target")
	}
	return ret
}

func activationMapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func compactActivationContextValue(value any, depth int) any {
	value = common.RedactSensitiveValue(value)
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		typed = strings.TrimSpace(typed)
		if strings.Contains(typed, "[REDACTED]") {
			return map[string]any{
				"omitted":                         true,
				"containedRuntimeRedactionMarker": true,
				"bytes":                           len(typed),
				"reason":                          "redacted runtime marker content is not reusable repair input; rebuild from current target snapshots or fresh reads",
			}
		}
		if len(typed) > 4096 {
			return typed[:4096] + "..."
		}
		return typed
	case map[string]any:
		if depth >= 3 {
			return map[string]any{"entries": len(typed)}
		}
		ret := map[string]any{}
		for key, item := range typed {
			if activationContextKeySkipped(key) {
				continue
			}
			ret[key] = compactActivationContextValue(item, depth+1)
			if len(ret) >= 32 {
				break
			}
		}
		return ret
	case []map[string]any:
		if depth >= 3 {
			return map[string]any{"items": len(typed)}
		}
		limit := min(len(typed), 20)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactActivationContextValue(typed[idx], depth+1))
		}
		return ret
	case []any:
		if depth >= 3 {
			return map[string]any{"items": len(typed)}
		}
		limit := min(len(typed), 20)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactActivationContextValue(typed[idx], depth+1))
		}
		return ret
	case []string:
		limit := min(len(typed), 20)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactActivationContextValue(typed[idx], depth+1))
		}
		return ret
	default:
		return typed
	}
}

func activationContextKeySkipped(key string) bool {
	switch strings.TrimSpace(key) {
	case "executionContext",
		"executionContextPackage",
		"failedStepInput",
		"previousStepOutput",
		"workspaceFileManifest",
		"failureTargetRepairContext",
		"patchRepairContext",
		"content",
		"diff",
		"rawContentPreview",
		"repairRawContentPreview",
		"streamEvents",
		"llmProviderId",
		"providerId",
		"provider",
		"model":
		return true
	default:
		return false
	}
}

func stripRuntimeLLMBindingFromActivationContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	for _, key := range []string{
		"llmProviderId",
		"providerId",
		"provider",
		"model",
	} {
		delete(input, key)
	}
	for key, value := range input {
		input[key] = stripRuntimeLLMBindingValueFromActivationContext(value)
	}
	return input
}

func stripRuntimeLLMBindingValueFromActivationContext(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return stripRuntimeLLMBindingFromActivationContext(typed)
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, stripRuntimeLLMBindingFromActivationContext(item))
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, stripRuntimeLLMBindingValueFromActivationContext(item))
		}
		return ret
	default:
		return value
	}
}

func memoryContexts(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":          string(item.ID),
			"memoryType":  item.MemoryType,
			"summary":     item.Summary,
			"content":     item.Content,
			"tags":        cloneStrings(item.Tags),
			"importance":  item.Importance,
			"confidence":  item.Confidence,
			"visibility":  item.Visibility,
			"provenance":  activationMemoryProvenance(item),
			"vectorScore": item.VectorScore,
		})
	}
	return ret
}

func activationMemoryProvenance(item *memorymodel.MemoryItem) string {
	if item == nil || item.RunID == "" {
		return "workspace"
	}
	return "historical_run"
}

func createAndStartWorkspaceExecutionRun(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any, priority int, llmProviderID string) (common.RunID, error) {
	exports, err := session.Exports()
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 8; attempt++ {
		runID, err := nextWorkspaceExecutionRunID(ctx, exports.Repositories.Runs)
		if err != nil {
			return "", err
		}
		createExecutionContext := executionContextForCreatedWorkspaceRun(runID, workspaceID, goalID, dodID, executionContext, priority)
		if err := exports.Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
			RunID:            runID,
			WorkspaceID:      workspaceID,
			GoalID:           goalID,
			DoDID:            dodID,
			InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
			ExecutionContext: createExecutionContext,
			Priority:         priority,
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return "", err
		}
		if err := recordActivationMemoryRecallTrace(ctx, exports, runID, workspaceID, createExecutionContext); err != nil {
			return "", err
		}
		startCtx := common.ContextWithRuntimeLLMProviderID(ctx, llmProviderID)
		if err := exports.Services.RunService.EnsureStarted(startCtx, runmodule.StartRunCommand{RunID: runID}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				run, getErr := exports.Repositories.Runs.Get(ctx, runID)
				if getErr != nil {
					return "", getErr
				}
				if isActivationObservableRunState(run.State) {
					return runID, nil
				}
			}
			return "", err
		}
		return runID, nil
	}
	return "", common.ErrConflict
}

func nextWorkspaceExecutionRunID(ctx context.Context, runs repository.RunRepository) (common.RunID, error) {
	if runs == nil {
		return "", common.ErrInvalidArgument
	}
	items, err := runs.List(ctx)
	if err != nil {
		return "", err
	}
	maxSeq := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		if seq, ok := parseNumericRunID(item.ID); ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	return common.RunID(formatNumericRunID(maxSeq + 1)), nil
}

func parseNumericRunID(runID common.RunID) (int, bool) {
	raw := strings.TrimSpace(string(runID))
	if !strings.HasPrefix(raw, "run-") {
		return 0, false
	}
	suffix := strings.TrimPrefix(raw, "run-")
	if suffix == "" {
		return 0, false
	}
	seq, err := strconv.Atoi(suffix)
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}

func formatNumericRunID(seq int) string {
	if seq < 1 {
		seq = 1
	}
	return fmt.Sprintf("run-%03d", seq)
}

func isActivationObservableRunState(state common.RunState) bool {
	switch state {
	case common.RunStateRunning,
		common.RunStateValidating,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval,
		common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func recordActivationMemoryRecallTrace(ctx context.Context, exports appbootstrap.Exports, runID common.RunID, workspaceID common.WorkspaceID, executionContext map[string]any) error {
	if exports.Repositories.MemoryRecallTraces == nil || runID == "" || workspaceID == "" {
		return nil
	}
	retrieval, _ := executionContext["memoryRetrieval"].(map[string]any)
	if len(retrieval) == 0 {
		return nil
	}
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/activation", runID))
	trace := &memorymodel.MemoryRecallTrace{
		ID:                  traceID,
		WorkspaceID:         workspaceID,
		RunID:               runID,
		Stage:               "planning",
		Source:              "activation",
		Policy:              traceStringValue(retrieval["policy"]),
		Status:              traceStringValue(retrieval["status"]),
		ProviderID:          traceStringValue(retrieval["providerId"]),
		Operation:           traceStringValue(retrieval["operation"]),
		QueryTextPreview:    traceStringValue(retrieval["queryTextPreview"]),
		QueryTextSha256:     traceStringValue(retrieval["queryTextSha256"]),
		MinScore:            traceFloatValue(retrieval["minScore"]),
		Limit:               traceIntValue(retrieval["limit"]),
		EmbeddingModel:      traceStringValue(retrieval["embeddingModel"]),
		VectorDimension:     traceIntValue(retrieval["vectorDimension"]),
		RefreshSource:       "activation",
		RetrievedMemoryIDs:  traceMemoryIDs(retrieval["retrievedMemoryIds"]),
		RetrievedMemoryHits: traceMemoryHits(retrieval["retrievedMemoryHits"]),
		Reason:              traceStringValue(retrieval["reason"]),
		OccurredAt:          time.Now().UTC().Format(time.RFC3339Nano),
		EvidenceRef:         string(traceID),
	}
	if len(trace.RetrievedMemoryIDs) == 0 {
		trace.RetrievedMemoryIDs = traceMemoryIDs(executionContext["memoryIds"])
	}
	trace.SelectedMemoryIDs = append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...)
	if err := exports.Repositories.MemoryRecallTraces.Upsert(ctx, trace); err != nil {
		return err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil
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

func traceMemoryIDs(value any) []common.MemoryID {
	switch typed := value.(type) {
	case []common.MemoryID:
		return append([]common.MemoryID(nil), typed...)
	case []string:
		ret := make([]common.MemoryID, 0, len(typed))
		for _, item := range typed {
			if item != "" {
				ret = append(ret, common.MemoryID(item))
			}
		}
		return ret
	case []any:
		ret := make([]common.MemoryID, 0, len(typed))
		for _, item := range typed {
			if text := traceStringValue(item); text != "" {
				ret = append(ret, common.MemoryID(text))
			}
		}
		return ret
	default:
		return nil
	}
}

func traceMemoryHits(value any) []memorymodel.MemoryRecallHit {
	items := traceMapSlice(value)
	ret := make([]memorymodel.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		id := common.MemoryID(traceStringValue(item["id"]))
		if id == "" {
			continue
		}
		ret = append(ret, memorymodel.MemoryRecallHit{
			ID:              id,
			Summary:         traceStringValue(item["summary"]),
			VectorScore:     traceFloatValue(item["vectorScore"]),
			VectorDimension: traceIntValue(item["vectorDimension"]),
		})
	}
	return ret
}

func traceMapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func traceStringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

func traceIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func traceFloatValue(value any) float64 {
	switch typed := value.(type) {
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return float64(typed)
	case int8:
		return float64(typed)
	case int16:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func executionContextForCreatedWorkspaceRun(runID common.RunID, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any, priority int) map[string]any {
	createdRun := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           goalID,
		DoDID:            dodID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: cloneContextMap(executionContext),
		State:            common.RunStateCreated,
		Priority:         priority,
	}
	ret := cloneContextMap(executionContext)
	if ret == nil {
		ret = map[string]any{}
	}
	if compressed := compressedContextForRunStart(createdRun); compressed != nil {
		ret["compressedContext"] = compressed
	}
	return ret
}

func readWorkspaceSnapshotFromExports(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID) (WorkspaceSnapshot, error) {
	return readWorkspaceSnapshotFromExportsWithRunSnapshot(ctx, session, workspaceID, nil)
}

func readWorkspaceSnapshotFromExportsWithRunSnapshot(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, runSnapshot *RunSnapshot) (WorkspaceSnapshot, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return readWorkspaceSnapshot(ctx, deps, workspaceID, runSnapshot)
}

func readRunSnapshotFromExports(ctx context.Context, session *RuntimeSession, runID common.RunID) (RunSnapshot, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunSnapshot{}, err
	}
	return readRunSnapshot(ctx, deps, runID)
}
