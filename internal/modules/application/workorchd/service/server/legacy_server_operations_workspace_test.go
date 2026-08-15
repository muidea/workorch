package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Workspace reads/writes are kept together as the application boundary for
// workspace-owned data; no Runtime transition is selected here.
func (s *serverState) configureWorkspaceOperations(session *runtimecore.Session) {
	s.createWorkspace = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.WorkspaceCreateRequest) (appmodelcore.Workspace, error) {
		var ret appmodelcore.Workspace
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateWorkspaceWithTemplateWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.deleteWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) error {
		return withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			return appservice.DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd)
		})
	}
	s.readWorkspace = func(ctx context.Context, _, _, workspaceID string) (appmodelcore.Workspace, error) {
		var ret appmodelcore.Workspace
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
		var ret readmodelcore.WorkspaceSnapshotState
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceDetailSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
		var ret readmodelcore.WorkspaceSnapshotState
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceDetailSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readProjectionSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
		var ret readmodelcore.WorkspaceSnapshotState
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceProjectionSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		var ret workorchmanagementcontract.WorkspaceSoulBindings
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceSoulBindingsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.updateWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string, cmd workorchmanagementcontract.WorkspaceSoulBindings) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		var ret workorchmanagementcontract.WorkspaceSoulBindings
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateWorkspaceSoulBindingsWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	s.listWorkspaces = func(ctx context.Context, _, _ string) ([]appmodelcore.Workspace, error) {
		var ret []appmodelcore.Workspace
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListWorkspacesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
}
