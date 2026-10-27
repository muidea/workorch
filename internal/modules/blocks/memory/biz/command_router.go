package memory

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type memoryCommandFamily uint8

const (
	memoryCommandFamilyItem memoryCommandFamily = iota + 1
	memoryCommandFamilyRecallTrace
	memoryCommandFamilyWorkspaceDeletion
)

func memoryCommandTopics() []string {
	return []string{
		memoryevents.TopicMemoryCommandCreate,
		memoryevents.TopicMemoryCommandGet,
		memoryevents.TopicObservePreparationInventory,
		memoryevents.TopicMemoryCommandListByWorkspace,
		memoryevents.TopicMemoryCommandSearch,
		memoryevents.TopicMemoryCommandUpdate,
		memoryevents.TopicMemoryCommandDelete,
		memoryevents.TopicMemoryRecallTraceCommandUpsert,
		memoryevents.TopicMemoryRecallTraceCommandList,
		memoryevents.TopicMemoryCommandSealWorkspaceDeletion,
		memoryevents.TopicMemoryCommandPurgeWorkspace,
	}
}

func memoryCommandFamilyForTopic(topic string) (memoryCommandFamily, bool) {
	switch topic {
	case memoryevents.TopicMemoryCommandCreate,
		memoryevents.TopicObservePreparationInventory,
		memoryevents.TopicMemoryCommandGet,
		memoryevents.TopicMemoryCommandListByWorkspace,
		memoryevents.TopicMemoryCommandSearch,
		memoryevents.TopicMemoryCommandUpdate,
		memoryevents.TopicMemoryCommandDelete:
		return memoryCommandFamilyItem, true
	case memoryevents.TopicMemoryRecallTraceCommandUpsert,
		memoryevents.TopicMemoryRecallTraceCommandList:
		return memoryCommandFamilyRecallTrace, true
	case memoryevents.TopicMemoryCommandSealWorkspaceDeletion,
		memoryevents.TopicMemoryCommandPurgeWorkspace:
		return memoryCommandFamilyWorkspaceDeletion, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	_ = m.executeCommand(ev, result)
}

func (m *Biz) executeCommand(ev magicevent.Event, result magicevent.Result) error {
	if m == nil || m.service == nil || ev == nil {
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
	ctx := ev.Context()
	workspaceID := memoryCommandWorkspaceID(ev.Data())
	if workspaceID == "" {
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		setMemoryMagicResult(result, struct{}{}, err)
		return err
	}
	unlock := m.lockWorkspaceMutation(workspaceID)
	defer unlock()
	family, ok := memoryCommandFamilyForTopic(ev.ID())
	if !ok {
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
	switch family {
	case memoryCommandFamilyWorkspaceDeletion:
		return m.executeWorkspaceDeletionCommand(ctx, ev, result)
	case memoryCommandFamilyItem:
		err := m.workspaceDeletions.WithWorkspaceActiveFence(ctx, workspaceID, func(guarded context.Context) error {
			return m.executeMemoryItemCommand(guarded, ev, result)
		})
		if err != nil {
			setMemoryMagicResult(result, struct{}{}, err)
		}
		return err
	case memoryCommandFamilyRecallTrace:
		err := m.workspaceDeletions.WithWorkspaceActiveFence(ctx, workspaceID, func(guarded context.Context) error {
			return m.executeMemoryRecallTraceCommand(guarded, ev, result)
		})
		if err != nil {
			setMemoryMagicResult(result, struct{}{}, err)
		}
		return err
	default:
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
}

// memoryCommandWorkspaceID extracts only the durable command scope. It does
// not inspect component state, so the shared owner can serve concurrent
// Workspace lanes without a mutable "current workspace" field.
func memoryCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case memoryevents.CreateMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.GetMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.ObservePreparationInventoryCommand:
		return cmd.WorkspaceID
	case memoryevents.ListMemoryByWorkspaceCommand:
		return cmd.WorkspaceID
	case memoryevents.SearchMemoryCommand:
		return cmd.Query.WorkspaceID
	case memoryevents.UpdateMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.DeleteMemoryCommand:
		return cmd.WorkspaceID
	case memoryevents.UpsertMemoryRecallTraceCommand:
		return cmd.WorkspaceID
	case memoryevents.ListMemoryRecallTracesCommand:
		return cmd.Query.WorkspaceID
	case memoryevents.SealWorkspaceMemoryDeletionCommand:
		return cmd.WorkspaceID
	case memoryevents.PurgeWorkspaceMemoryCommand:
		return cmd.WorkspaceID
	default:
		return ""
	}
}

func setMemoryMagicResult[Result any](result magicevent.Result, data Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
