package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

// CLICommandDispatcher owns /cli/command handling for workorchd. The CLI/TUI
// backend publishes command envelopes only; owner state is read or changed here.
type CLICommandDispatcher struct {
	session    *RuntimeSession
	eventHub   events.Hub
	viewReader WorkspaceViewReader
	core       *clicore.Dispatcher
}

type WorkspaceViewReader interface {
	ListWorkspaceView(ctx context.Context) ([]Workspace, error)
	ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error)
}

func NewCLICommandDispatcher(session *RuntimeSession, eventHub events.Hub, viewReader WorkspaceViewReader) *CLICommandDispatcher {
	ret := &CLICommandDispatcher{session: session, eventHub: eventHub, viewReader: viewReader}
	ret.core = clicore.NewDispatcher(eventHub, func(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
		return ret.handleCommand(ContextWithEventHub(ctx, eventHub), cmd)
	})
	return ret
}

func (d *CLICommandDispatcher) Run() error {
	if d == nil || d.session == nil || d.core == nil {
		return common.ErrInvalidArgument
	}
	return d.core.Run()
}

func (d *CLICommandDispatcher) Close() {
	if d == nil {
		return
	}
	d.core.Close()
}

func (d *CLICommandDispatcher) handleCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	if cmd.WorkspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
	}
	switch cmd.Command {
	case events.CommandWorkspaceCreate, events.CommandWorkspaceGet, events.CommandWorkspaceList,
		events.CommandWorkspaceSnapshot, events.CommandWorkspaceDelete:
		return d.handleWorkspaceCommand(ctx, cmd)
	case events.CommandGoalDefine, events.CommandGoalList, events.CommandGoalGet, events.CommandGoalUpdate,
		events.CommandDoDDefine, events.CommandDoDList, events.CommandDoDGet, events.CommandDoDUpdate:
		return d.handleDefinitionCommand(ctx, cmd)
	case events.CommandRunActivate, events.CommandRunResume, events.CommandRunGet, events.CommandRunList,
		events.CommandRunStatus:
		return d.handleRunCommand(ctx, cmd)
	case events.CommandDecisionResolve, events.CommandDecisionList, events.CommandDecisionGet, events.CommandDecisionBatch,
		events.CommandApprovalApprove, events.CommandApprovalReject, events.CommandApprovalList, events.CommandApprovalGet,
		events.CommandApprovalBatch:
		return d.handleGovernanceCommand(ctx, cmd)
	case events.CommandSoulCreate, events.CommandSoulGet, events.CommandSoulList, events.CommandSoulUpdate,
		events.CommandSoulDelete, events.CommandSkillCreate, events.CommandSkillGet, events.CommandSkillList,
		events.CommandMCPServerCreate, events.CommandMCPServerGet, events.CommandMCPServerList,
		events.CommandMCPServerUpdate, events.CommandMCPCapabilityList:
		return d.handleContextRegistryCommand(ctx, cmd)
	case events.CommandMemoryCreate, events.CommandMemoryGet, events.CommandMemoryUpdate, events.CommandMemoryDelete,
		events.CommandMemoryList, events.CommandMemorySearch:
		return d.handleMemoryCommand(ctx, cmd)
	case events.CommandTemplateCreate, events.CommandTemplateGet, events.CommandTemplateList,
		events.CommandTemplateUpdate, events.CommandTemplateDelete:
		return d.handleTemplateCommand(ctx, cmd)
	case events.CommandConfigurationGet:
		item, err := ReadServiceConfiguration()
		return clicore.ServiceConfigResourceMap(item), nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

func (d *CLICommandDispatcher) borrow() *RuntimeSession {
	if d == nil || d.session == nil {
		return nil
	}
	return d.session.Borrow()
}
