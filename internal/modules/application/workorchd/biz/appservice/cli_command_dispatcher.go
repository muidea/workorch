package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

// CLICommandDispatcher owns the operation-specific CLI EventHub contracts.
type CLICommandDispatcher struct {
	session    *RuntimeSession
	commandHub magicevent.Hub
	viewReader WorkspaceViewReader
	typed      magicevent.SimpleObserver
}

type WorkspaceViewReader interface {
	ListWorkspaceView(ctx context.Context) ([]Workspace, error)
	ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error)
}

func NewCLICommandDispatcher(session *RuntimeSession, commandHub magicevent.Hub, viewReader WorkspaceViewReader) *CLICommandDispatcher {
	return &CLICommandDispatcher{session: session, commandHub: commandHub, viewReader: viewReader}
}

func (d *CLICommandDispatcher) Run() error {
	if d == nil || d.session == nil {
		return common.ErrInvalidArgument
	}
	return d.runTypedCLI()
}

func (d *CLICommandDispatcher) Close() {
	if d == nil {
		return
	}
	d.closeTypedCLI()
}

func (d *CLICommandDispatcher) borrow() *RuntimeSession {
	if d == nil || d.session == nil {
		return nil
	}
	return d.session.Borrow()
}
