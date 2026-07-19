package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/common"
)

// CLICommandDispatcher owns the operation-specific CLI EventHub contracts.
type CLICommandDispatcher struct {
	basebiz.Base
	session    *RuntimeSession
	viewReader WorkspaceViewReader
	typedReady bool
}

type WorkspaceViewReader interface {
	ListWorkspaceView(ctx context.Context) ([]Workspace, error)
	ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error)
	// ReadWorkspaceActivity returns the store-owned bounded activity feed.
	// Optional: implementations that do not yet persist activity may return
	// ok=false and RuntimeView will fall back to content rebuild with revision 0.
	ReadWorkspaceActivity(ctx context.Context, workspaceID string) (WorkspaceActivityFeed, bool, error)
}

func NewCLICommandDispatcher(session *RuntimeSession, commandHub magicevent.Hub, viewReader WorkspaceViewReader) *CLICommandDispatcher {
	return &CLICommandDispatcher{
		Base:       basebiz.New(modulecommon.ModuleID, commandHub, nil),
		session:    session,
		viewReader: viewReader,
	}
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
