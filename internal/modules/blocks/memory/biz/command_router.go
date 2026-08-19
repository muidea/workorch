package memory

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type memoryCommandFamily uint8

const (
	memoryCommandFamilyItem memoryCommandFamily = iota + 1
	memoryCommandFamilyRecallTrace
)

func memoryCommandTopics() []string {
	return []string{
		memoryevents.TopicMemoryCommandCreate,
		memoryevents.TopicMemoryCommandGet,
		memoryevents.TopicMemoryCommandListByWorkspace,
		memoryevents.TopicMemoryCommandSearch,
		memoryevents.TopicMemoryCommandUpdate,
		memoryevents.TopicMemoryCommandDelete,
		memoryevents.TopicMemoryRecallTraceCommandUpsert,
		memoryevents.TopicMemoryRecallTraceCommandGet,
		memoryevents.TopicMemoryRecallTraceCommandList,
		memoryevents.TopicMemoryRecallTraceCommandDelete,
	}
}

func memoryCommandFamilyForTopic(topic string) (memoryCommandFamily, bool) {
	switch topic {
	case memoryevents.TopicMemoryCommandCreate,
		memoryevents.TopicMemoryCommandGet,
		memoryevents.TopicMemoryCommandListByWorkspace,
		memoryevents.TopicMemoryCommandSearch,
		memoryevents.TopicMemoryCommandUpdate,
		memoryevents.TopicMemoryCommandDelete:
		return memoryCommandFamilyItem, true
	case memoryevents.TopicMemoryRecallTraceCommandUpsert,
		memoryevents.TopicMemoryRecallTraceCommandGet,
		memoryevents.TopicMemoryRecallTraceCommandList,
		memoryevents.TopicMemoryRecallTraceCommandDelete:
		return memoryCommandFamilyRecallTrace, true
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
	family, ok := memoryCommandFamilyForTopic(ev.ID())
	if !ok {
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
	switch family {
	case memoryCommandFamilyItem:
		return m.executeMemoryItemCommand(ctx, ev, result)
	case memoryCommandFamilyRecallTrace:
		return m.executeMemoryRecallTraceCommand(ctx, ev, result)
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
	case memoryevents.GetMemoryRecallTraceCommand:
		return cmd.WorkspaceID
	case memoryevents.ListMemoryRecallTracesCommand:
		return cmd.Query.WorkspaceID
	case memoryevents.DeleteMemoryRecallTraceCommand:
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
