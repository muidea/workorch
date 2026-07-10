package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

// CLICommandDispatcher owns the typed /cli/command contract for workorchd.
// The CLI/TUI backend sends the owner-defined command through magicCommon EventHub;
// owner state is read or changed here.
type CLICommandDispatcher struct {
	session    *RuntimeSession
	eventHub   events.Hub
	commandHub magicevent.Hub
	viewReader WorkspaceViewReader
	core       *clicore.Dispatcher
}

type WorkspaceViewReader interface {
	ListWorkspaceView(ctx context.Context) ([]Workspace, error)
	ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error)
}

func NewCLICommandDispatcher(session *RuntimeSession, eventHub events.Hub, commandHub magicevent.Hub, viewReader WorkspaceViewReader) *CLICommandDispatcher {
	ret := &CLICommandDispatcher{session: session, eventHub: eventHub, commandHub: commandHub, viewReader: viewReader}
	ret.core = clicore.NewDispatcher(commandHub, func(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
		return ret.handleCommand(ContextWithEventHubs(ctx, eventHub, commandHub), cmd)
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

func (d *CLICommandDispatcher) handleCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	if cmd.WorkspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
	}
	switch cmd.Command {
	case clicore.CommandWorkspaceCreate, clicore.CommandWorkspaceGet, clicore.CommandWorkspaceList,
		clicore.CommandWorkspaceSnapshot, clicore.CommandWorkspaceDelete:
		return d.handleWorkspaceCommand(ctx, cmd)
	case clicore.CommandGoalDefine, clicore.CommandGoalList, clicore.CommandGoalGet, clicore.CommandGoalUpdate,
		clicore.CommandDoDDefine, clicore.CommandDoDList, clicore.CommandDoDGet, clicore.CommandDoDUpdate:
		return d.handleDefinitionCommand(ctx, cmd)
	case clicore.CommandRunActivate, clicore.CommandRunResume, clicore.CommandRunGet, clicore.CommandRunList,
		clicore.CommandRunStatus:
		return d.handleRunCommand(ctx, cmd)
	case clicore.CommandDecisionResolve, clicore.CommandDecisionList, clicore.CommandDecisionGet, clicore.CommandDecisionBatch,
		clicore.CommandApprovalApprove, clicore.CommandApprovalReject, clicore.CommandApprovalList, clicore.CommandApprovalGet,
		clicore.CommandApprovalBatch:
		return d.handleGovernanceCommand(ctx, cmd)
	case clicore.CommandSoulCreate, clicore.CommandSoulGet, clicore.CommandSoulList, clicore.CommandSoulUpdate,
		clicore.CommandSoulDelete, clicore.CommandSkillCreate, clicore.CommandSkillGet, clicore.CommandSkillList,
		clicore.CommandMCPServerCreate, clicore.CommandMCPServerGet, clicore.CommandMCPServerList,
		clicore.CommandMCPServerUpdate, clicore.CommandMCPCapabilityList:
		return d.handleContextRegistryCommand(ctx, cmd)
	case clicore.CommandMemoryCreate, clicore.CommandMemoryGet, clicore.CommandMemoryUpdate, clicore.CommandMemoryDelete,
		clicore.CommandMemoryList, clicore.CommandMemorySearch:
		return d.handleMemoryCommand(ctx, cmd)
	case clicore.CommandTemplateCreate, clicore.CommandTemplateGet, clicore.CommandTemplateList,
		clicore.CommandTemplateUpdate, clicore.CommandTemplateDelete:
		return d.handleTemplateCommand(ctx, cmd)
	case clicore.CommandConfigurationGet:
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
