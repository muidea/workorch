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
	case runevents.TopicRunCommandGet:
		cmd, ok := ev.Data().(runevents.GetRunCommand)
		if !ok || cmd.RunID == "" {
			return nil, common.ErrInvalidArgument
		}
		var item *runmodel.Run
		var err error
		if cmd.WorkspaceID != "" {
			item, err = m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
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
		return runevents.GetRunResult{Run: item}, nil
	case runevents.TopicRunCommandList:
		cmd, ok := ev.Data().(runevents.ListRunsCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		items, err := m.runs.List(ctx)
		if err != nil {
			return nil, err
		}
		items, err = m.filterRetiredWorkspaceRuns(ctx, items)
		if err != nil {
			return nil, err
		}
		return runevents.ListRunsResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States), cmd.Compact)}, nil
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
		items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return runevents.ListRunsByWorkspaceResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States), cmd.Compact)}, nil
	case runevents.TopicRunCommandLifecycleFactInventory:
		cmd, ok := ev.Data().(runevents.QueryRunLifecycleFactInventoryCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		items, err := m.runs.List(ctx)
		if err != nil {
			return nil, err
		}
		response, err := buildRunLifecycleFactInventory(items, cmd)
		if err != nil {
			return nil, err
		}
		return response, nil
	default:
		return nil, common.ErrInvalidArgument
	}
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
		if item == nil || runStateIsTerminal(item.State) {
			continue
		}
		fact := runevents.RunStateChangedDataFromModel(item)
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

func projectRunCommandListItems(items []*runmodel.Run, states map[common.RunState]struct{}, compact bool) []*runmodel.Run {
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
		if compact {
			ret = append(ret, compactRunListItem(item))
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func compactRunListItem(item *runmodel.Run) *runmodel.Run {
	if item == nil {
		return nil
	}
	return &runmodel.Run{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		Lineage:            item.Lineage,
		DefinitionRevision: item.DefinitionRevision,
		StartupInputHash:   item.StartupInputHash,
		InitialStepKind:    item.InitialStepKind,
		State:              item.State,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		FailureReason:      item.FailureReason,
		CreatedAt:          item.CreatedAt,
		StartedAt:          item.StartedAt,
		EndedAt:            item.EndedAt,
	}
}
