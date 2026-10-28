package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (p *runtimeApplicationPorts) configureWorkspacePorts() {
	ports := &p.ports
	preparationCommand := func(recoverOriginal bool) func(context.Context, string, workorchmanagementcontract.PreparationCheckRequest) (workorchmanagementcontract.PreparationOperationView, error) {
		return func(ctx context.Context, workspaceID string, request workorchmanagementcontract.PreparationCheckRequest) (workorchmanagementcontract.PreparationOperationView, error) {
			var view workorchmanagementcontract.PreparationOperationView
			err := p.withSession(ctx, func(session *runtimecore.Session) error {
				hub, err := session.RequiredCommandHub()
				if err != nil {
					return err
				}
				b := &Biz{Base: basebiz.New(ModuleID, hub, nil), preparationSkillHubRunner: p.preparationSkillHubRunner}
				// Use the real process accessor, not ConfigLoader's empty-config
				// compatibility fallback. Absence remains an unobserved config.
				exports, exportErr := session.Exports()
				if exportErr != nil {
					return exportErr
				}
				observationCtx := configcore.ContextWithLoader(ctx, exports.Infrastructure.LoadConfig)
				view, err = b.CheckStartupPreparation(observationCtx, common.WorkspaceID(workspaceID), request, recoverOriginal)
				return err
			})
			return view, err
		}
	}
	initializationCommand := func(recoverOriginal bool) func(context.Context, string, workorchmanagementcontract.PreparationCheckRequest) (workorchmanagementcontract.PreparationResourceInitializationOperation, error) {
		return func(ctx context.Context, id string, request workorchmanagementcontract.PreparationCheckRequest) (workorchmanagementcontract.PreparationResourceInitializationOperation, error) {
			var view workorchmanagementcontract.PreparationResourceInitializationOperation
			err := p.withSession(ctx, func(session *runtimecore.Session) error {
				hub, err := session.RequiredCommandHub()
				if err != nil {
					return err
				}
				b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
				view, err = b.InitializeStartupPreparationResources(ctx, common.WorkspaceID(id), request, recoverOriginal)
				return err
			})
			return view, err
		}
	}
	ports.Workspace.InitializeStartupPreparationResources = initializationCommand(false)
	ports.Workspace.RecoverStartupPreparationResources = initializationCommand(true)
	ports.Workspace.CheckStartupPreparation = preparationCommand(false)
	ports.Workspace.RecoverStartupPreparation = preparationCommand(true)
	ports.Workspace.ReadStartupPreparation = func(ctx context.Context, workspaceID string) (workorchmanagementcontract.StartupPreparationView, error) {
		var view workorchmanagementcontract.StartupPreparationView
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			hub, err := session.RequiredCommandHub()
			if err != nil {
				return err
			}
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			view, err = b.ReadStartupPreparationView(ctx, common.WorkspaceID(workspaceID))
			return err
		})
		return view, err
	}
	ports.Workspace.DeleteWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) error {
		return p.withSession(ctx, func(session *runtimecore.Session) error {
			return appservice.DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd)
		})
	}
	ports.Workspace.ReadWorkspace = func(ctx context.Context, _, _, workspaceID string) (appmodelcore.Workspace, error) {
		var ret appmodelcore.Workspace
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	ports.Workspace.ReadWorkspaceSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
		var ret readmodelcore.WorkspaceSnapshotState
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	ports.Workspace.ReadWorkspaceDetailSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
		var ret readmodelcore.WorkspaceSnapshotState
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceDetailSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	ports.Workspace.ReadProjectionSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
		var ret readmodelcore.WorkspaceSnapshotState
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceProjectionSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	ports.Workspace.ReadWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		var ret workorchmanagementcontract.WorkspaceSoulBindings
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadWorkspaceSoulBindingsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	ports.Workspace.UpdateWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string, cmd workorchmanagementcontract.WorkspaceSoulBindings) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		var ret workorchmanagementcontract.WorkspaceSoulBindings
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateWorkspaceSoulBindingsWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	ports.Workspace.ListWorkspaces = func(ctx context.Context, _, _ string) ([]appmodelcore.Workspace, error) {
		var ret []appmodelcore.Workspace
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListWorkspacesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Workspace.UpdateWorkspaceLLMSelection = func(ctx context.Context, workspaceID string, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (appmodelcore.Workspace, error) {
		var ret appmodelcore.Workspace
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateWorkspaceLLMSelectionWithSession(ctx, session, workspaceID, selection, expectedRevision)
			return err
		})
		return ret, err
	}
}
