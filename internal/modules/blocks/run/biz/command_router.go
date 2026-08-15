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
	runCommandFamilyCreate runCommandFamily = iota + 1
	runCommandFamilyRunQuery
	runCommandFamilyStepQuery
	runCommandFamilyCheckpointQuery
	runCommandFamilyRuntimeStateStore
	runCommandFamilyResourceCleanup
)

func runCommandTopics() []string {
	return []string{
		runevents.TopicRunCommandCreate,
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
	case runevents.TopicRunCommandCreate:
		return runCommandFamilyCreate, true
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

func (m *Biz) executeCommand(ev magicevent.Event) (runevents.RunCommandResult, error) {
	if m == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.Service == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	workspaceID := workspaceScopeFromRunCommand(ev.Data())
	if workspaceID != "" {
		var err error
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
	}
	family, ok := runCommandFamilyForTopic(ev.ID())
	if !ok {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	switch family {
	case runCommandFamilyCreate:
		return m.executeCreateRunCommand(ctx, ev)
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
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
}

func workspaceScopeFromRunCommand(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case runevents.CreateRunCommand:
		return cmd.WorkspaceID
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

func setRunMagicResult(result magicevent.Result, data runevents.RunCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data.Clone(), nil)
}
