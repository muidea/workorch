package run

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeRunQueryCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	switch ev.ID() {
	case runevents.TopicQueryWorkspaceExecutionIntentReceipt:
		return m.queryWorkspaceExecutionIntentReceipt(ctx, ev)
	case runevents.TopicObserveWorkspacePreparationHistory:
		return m.observeWorkspacePreparationHistory(ctx, ev)
	case runevents.TopicRunCommandGet:
		cmd, ok := ev.Data().(runevents.GetRunCommand)
		if !ok || cmd.RunID == "" {
			return nil, common.ErrInvalidArgument
		}
		var item *runmodel.Run
		var err error
		preCloned := false
		if cmd.WorkspaceID != "" {
			if snapRepo, ok := m.runs.(runtimeStateSnapshotRepository); ok {
				item, err = snapRepo.GetRuntimeStateSnapshot(ctx, cmd.WorkspaceID, cmd.RunID)
				preCloned = true
			}
			if !preCloned {
				item, err = m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
			}
		} else {
			item, err = m.runs.Get(ctx, cmd.RunID)
		}
		if err != nil {
			return nil, err
		}
		if retired, err := m.workspaceRunsRetired(ctx, item.WorkspaceID); err != nil {
			return nil, err
		} else if retired {
			return nil, common.ErrNotFound
		}
		res := runevents.GetRunResult{Run: item}
		if preCloned {
			return preClonedRunCommandResult{value: res}, nil
		}
		return res, nil
	case runevents.TopicRunCommandList:
		cmd, ok := ev.Data().(runevents.ListRunsCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		if cmd.Compact {
			facts, err := m.listRunInventoryForQuery(ctx, "", cmd.States)
			return runevents.ListRunsResult{Inventory: facts}, err
		}
		items, err := m.runs.List(ctx)
		if err != nil {
			return nil, err
		}
		items, err = m.filterRetiredWorkspaceRuns(ctx, items)
		if err != nil {
			return nil, err
		}
		return runevents.ListRunsResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States))}, nil
	case runevents.TopicRunCommandListByWorkspace:
		cmd, ok := ev.Data().(runevents.ListRunsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return nil, common.ErrInvalidArgument
		}
		if retired, err := m.workspaceRunsRetired(ctx, cmd.WorkspaceID); err != nil {
			return nil, err
		} else if retired {
			return runevents.ListRunsByWorkspaceResult{}, nil
		}
		if cmd.Compact {
			facts, err := m.listRunInventoryForQuery(ctx, cmd.WorkspaceID, cmd.States)
			return runevents.ListRunsByWorkspaceResult{Inventory: facts}, err
		}
		items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return runevents.ListRunsByWorkspaceResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States))}, nil
	case runevents.TopicRunCommandLifecycleFactInventory:
		cmd, ok := ev.Data().(runevents.QueryRunLifecycleFactInventoryCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		items, err := m.runs.ListInventory(ctx, "")
		if err != nil {
			return nil, err
		}
		response, err := buildRunLifecycleFacts(items, cmd)
		if err != nil {
			return nil, err
		}
		return response, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) listRunInventoryForQuery(ctx context.Context, workspaceID common.WorkspaceID, states []common.RunState) ([]runmodel.RunInventoryItem, error) {
	facts, err := m.runs.ListInventory(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var retired map[common.WorkspaceID]bool
	if workspaceID == "" {
		retirements, err := m.runs.ListWorkspaceRetirements(ctx)
		if err != nil {
			return nil, err
		}
		retired = make(map[common.WorkspaceID]bool, len(retirements))
		for _, fence := range retirements {
			if fence.IsRetired() {
				retired[fence.WorkspaceID] = true
			}
		}
	}
	filter := runStateFilter(states)
	ret := make([]runmodel.RunInventoryItem, 0, len(facts))
	for _, fact := range facts {
		if retired[fact.WorkspaceID] || workspaceID != "" && fact.WorkspaceID != workspaceID {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filter[fact.State]; !ok {
				continue
			}
		}
		ret = append(ret, fact.Clone())
	}
	return ret, nil
}

func (m *Biz) workspaceRunsRetired(ctx context.Context, workspaceID common.WorkspaceID) (bool, error) {
	if m == nil || m.runs == nil || workspaceID == "" {
		return false, common.ErrInvalidArgument
	}
	fence, err := m.runs.GetWorkspaceRetirement(ctx, workspaceID)
	if errors.Is(err, common.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return fence.IsRetired(), nil
}

func (m *Biz) filterRetiredWorkspaceRuns(ctx context.Context, items []*runmodel.Run) ([]*runmodel.Run, error) {
	retirements, err := m.runs.ListWorkspaceRetirements(ctx)
	if err != nil {
		return nil, err
	}
	retired := make(map[common.WorkspaceID]struct{}, len(retirements))
	for _, fence := range retirements {
		if fence.IsRetired() {
			retired[fence.WorkspaceID] = struct{}{}
		}
	}
	ret := make([]*runmodel.Run, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if _, hidden := retired[item.WorkspaceID]; !hidden {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func buildRunLifecycleFactInventory(items []*runmodel.Run, cmd runevents.QueryRunLifecycleFactInventoryCommand) (runevents.QueryRunLifecycleFactInventoryResult, error) {
	facts := make([]runmodel.RunInventoryItem, 0, len(items))
	for _, item := range items {
		if item != nil {
			facts = append(facts, runmodel.InventoryFromRun(item))
		}
	}
	return buildRunLifecycleFacts(facts, cmd)
}

func buildRunLifecycleFacts(items []runmodel.RunInventoryItem, cmd runevents.QueryRunLifecycleFactInventoryCommand) (runevents.QueryRunLifecycleFactInventoryResult, error) {
	limit := cmd.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cursor := strings.TrimSpace(cmd.Cursor)
	type entry struct {
		key  string
		fact runevents.RunStateChangedData
	}
	entries := make([]entry, 0, len(items))
	activeWorkspace := map[common.WorkspaceID]common.RunID{}
	for _, item := range items {
		if runStateIsTerminal(item.State) {
			continue
		}
		fact := runevents.RunStateChangedDataFromInventory(item)
		switch fact.State {
		case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		default:
			return runevents.QueryRunLifecycleFactInventoryResult{}, common.ErrInvalidArgument
		}
		if err := fact.Validate(); err != nil {
			return runevents.QueryRunLifecycleFactInventoryResult{}, err
		}
		if existing := activeWorkspace[fact.WorkspaceID]; existing != "" && existing != fact.RunID {
			return runevents.QueryRunLifecycleFactInventoryResult{}, fmt.Errorf("%w: Workspace %s has multiple current non-terminal Runs", common.ErrConflict, fact.WorkspaceID)
		}
		activeWorkspace[fact.WorkspaceID] = fact.RunID
		key := runevents.RunLifecycleFactInventoryCursorKey(fact)
		if cursor != "" && key <= cursor {
			continue
		}
		entries = append(entries, entry{key: key, fact: fact})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	result := runevents.QueryRunLifecycleFactInventoryResult{}
	pageSize := len(entries)
	if pageSize > limit {
		pageSize = limit
	}
	result.Items = make([]runevents.RunStateChangedData, 0, pageSize)
	for idx := 0; idx < pageSize; idx++ {
		result.Items = append(result.Items, entries[idx].fact)
	}
	if len(entries) > limit {
		result.NextCursor = entries[limit-1].key
	}
	if err := result.Validate(); err != nil {
		return runevents.QueryRunLifecycleFactInventoryResult{}, err
	}
	return result, nil
}

func runStateFilter(states []common.RunState) map[common.RunState]struct{} {
	if len(states) == 0 {
		return nil
	}
	ret := map[common.RunState]struct{}{}
	for _, state := range states {
		if state != "" {
			ret[state] = struct{}{}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectRunCommandListItems(items []*runmodel.Run, states map[common.RunState]struct{}) []*runmodel.Run {
	if len(items) == 0 {
		return items
	}
	ret := make([]*runmodel.Run, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if len(states) > 0 {
			if _, ok := states[item.State]; !ok {
				continue
			}
		}
		ret = append(ret, item)
	}
	return ret
}
