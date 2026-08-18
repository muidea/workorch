package run

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runCommandFamily uint8

const (
	runCommandFamilyRunQuery runCommandFamily = iota + 1
	runCommandFamilyStepQuery
	runCommandFamilyCheckpointQuery
	runCommandFamilyRuntimeStateStore
	runCommandFamilyResourceCleanup
)

func runCommandTopics() []string {
	return []string{
		runevents.TopicRunCommandGet,
		runevents.TopicRunCommandList,
		runevents.TopicRunCommandListByWorkspace,
		runevents.TopicRunCommandGetStep,
		runevents.TopicRunCommandListStepsByRun,
		runevents.TopicRunCommandListStepSummariesByRun,
		runevents.TopicRunCommandGetRuntimeCheckpoint,
		runevents.TopicRunCommandListCheckpointsByRun,
		runevents.TopicRunCommandPurgeRuntimeResources,
		runevents.TopicRunCommandStoreRuntimeStateRecord,
		runevents.TopicRunCommandDelete,
	}
}

func runCommandFamilyForTopic(topic string) (runCommandFamily, bool) {
	switch topic {
	case runevents.TopicRunCommandGet,
		runevents.TopicRunCommandList,
		runevents.TopicRunCommandListByWorkspace:
		return runCommandFamilyRunQuery, true
	case runevents.TopicRunCommandGetStep,
		runevents.TopicRunCommandListStepsByRun,
		runevents.TopicRunCommandListStepSummariesByRun:
		return runCommandFamilyStepQuery, true
	case runevents.TopicRunCommandGetRuntimeCheckpoint,
		runevents.TopicRunCommandListCheckpointsByRun:
		return runCommandFamilyCheckpointQuery, true
	case runevents.TopicRunCommandStoreRuntimeStateRecord:
		return runCommandFamilyRuntimeStateStore, true
	case runevents.TopicRunCommandPurgeRuntimeResources,
		runevents.TopicRunCommandDelete:
		return runCommandFamilyResourceCleanup, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setRunMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (any, error) {
	if m == nil {
		return nil, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.Service == nil {
		return nil, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	workspaceID := workspaceScopeFromRunCommand(ev.Data())
	if workspaceID != "" {
		var err error
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
	}
	family, ok := runCommandFamilyForTopic(ev.ID())
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	switch family {
	case runCommandFamilyRunQuery:
		return m.executeRunQueryCommand(ctx, ev)
	case runCommandFamilyStepQuery:
		return m.executeStepQueryCommand(ctx, ev)
	case runCommandFamilyCheckpointQuery:
		return m.executeCheckpointQueryCommand(ctx, ev)
	case runCommandFamilyRuntimeStateStore:
		return m.executeRuntimeStateStoreCommand(ctx, ev)
	case runCommandFamilyResourceCleanup:
		return m.executeRunResourceCleanupCommand(ctx, ev)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func workspaceScopeFromRunCommand(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case runevents.GetRunCommand:
		return cmd.WorkspaceID
	case runevents.ListRunsByWorkspaceCommand:
		return cmd.WorkspaceID
	case runevents.GetStepCommand:
		return cmd.WorkspaceID
	case runevents.ListStepsByRunCommand:
		return cmd.WorkspaceID
	case runevents.ListStepSummariesByRunCommand:
		return cmd.WorkspaceID
	case runevents.GetRuntimeCheckpointCommand:
		return cmd.WorkspaceID
	case runevents.ListRuntimeCheckpointsByRunCommand:
		return cmd.WorkspaceID
	case runevents.PurgeRuntimeResourcesCommand:
		return cmd.WorkspaceID
	case runevents.StoreRuntimeStateRecordCommand:
		return cmd.WorkspaceID
	case runevents.DeleteRunCommand:
		return cmd.WorkspaceID
	default:
		return ""
	}
}

func (m *Biz) validateRunReadScope(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if runID == "" || workspaceID == "" {
		return nil
	}
	_, err := m.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	return err
}

func setRunMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	cloned, ok := cloneRunCommandResult(data)
	if !ok {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	result.Set(cloned, nil)
}

func cloneRunCommandResult(data any) (any, bool) {
	switch value := data.(type) {
	case runevents.GetRunResult:
		return value.Clone(), true
	case runevents.ListRunsResult:
		return value.Clone(), true
	case runevents.ListRunsByWorkspaceResult:
		return value.Clone(), true
	case runevents.GetStepResult:
		return value.Clone(), true
	case runevents.ListStepsByRunResult:
		return value.Clone(), true
	case runevents.ListStepSummariesByRunResult:
		return value.Clone(), true
	case runevents.GetRuntimeCheckpointResult:
		return value.Clone(), true
	case runevents.ListRuntimeCheckpointsByRunResult:
		return value.Clone(), true
	case runevents.PurgeRuntimeResourcesResult:
		return value, true
	case runevents.StoreRuntimeStateRecordResult:
		return value.Clone(), true
	case runevents.DeleteRunResult:
		return value, true
	default:
		return nil, false
	}
}
