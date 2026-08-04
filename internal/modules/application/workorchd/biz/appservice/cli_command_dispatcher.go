package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// CLICommandDispatcher owns the operation-specific CLI EventHub contracts.
type CLICommandDispatcher struct {
	basebiz.Base
	session    *runtimecore.Session
	viewReader WorkspaceViewReader
	typedReady bool
}

type WorkspaceViewReader interface {
	ListWorkspaceView(ctx context.Context) ([]appmodelcore.Workspace, error)
	ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error)
	// ReadWorkspaceActivity returns the store-owned bounded activity feed.
	// Optional: implementations that do not yet persist activity may return
	// ok=false and RuntimeView will fall back to content rebuild with revision 0.
	ReadWorkspaceActivity(ctx context.Context, workspaceID string) (workorchprojectioncontract.WorkspaceActivityFeed, bool, error)
}

func NewCLICommandDispatcher(session *runtimecore.Session, commandHub magicevent.Hub, viewReader WorkspaceViewReader) *CLICommandDispatcher {
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

func (d *CLICommandDispatcher) borrow() *runtimecore.Session {
	if d == nil || d.session == nil {
		return nil
	}
	return d.session.Borrow()
}
