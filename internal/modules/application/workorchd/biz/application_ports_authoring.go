package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (p *runtimeApplicationPorts) configureAuthoringPorts() {
	withAuthoring := func(ctx context.Context, fn func(*Biz, *runtimecore.Session) error) error {
		return p.withSession(ctx, func(session *runtimecore.Session) error {
			hub, err := session.RequiredCommandHub()
			if err != nil {
				return err
			}
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			return fn(b, session)
		})
	}
	authoring := server.DefinitionAuthoringPorts{
		Create: func(ctx context.Context, _, _ string, request workorchmanagementcontract.DefinitionAuthoringCreate) (view workorchmanagementcontract.DefinitionAuthoringView, err error) {
			err = withAuthoring(ctx, func(b *Biz, s *runtimecore.Session) error {
				view, err = b.CreateDefinitionAuthoringView(ctx, s.RepositoryBackend(), s.RepositoryDataDir(), request)
				return err
			})
			return
		},
		Read: func(ctx context.Context, id string) (view workorchmanagementcontract.DefinitionAuthoringView, err error) {
			err = withAuthoring(ctx, func(b *Biz, _ *runtimecore.Session) error {
				view, err = b.ReadDefinitionAuthoringView(ctx, common.WorkspaceID(id))
				return err
			})
			return
		},
		Save: func(ctx context.Context, id string, request workorchmanagementcontract.DefinitionAuthoringSave) (view workorchmanagementcontract.DefinitionAuthoringView, err error) {
			err = withAuthoring(ctx, func(b *Biz, _ *runtimecore.Session) error {
				view, err = b.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(id), request)
				return err
			})
			return
		},
		Compile: func(ctx context.Context, id string) (view workorchmanagementcontract.DefinitionAuthoringView, err error) {
			err = withAuthoring(ctx, func(b *Biz, _ *runtimecore.Session) error {
				if _, err = b.DeriveDefinitionAuthoringView(ctx, common.WorkspaceID(id)); err != nil {
					return err
				}
				view, err = b.CompileDefinitionAuthoringView(ctx, common.WorkspaceID(id))
				return err
			})
			return
		},
		Confirm: func(ctx context.Context, id string, request workorchmanagementcontract.DefinitionAuthoringConfirm) (view workorchmanagementcontract.DefinitionAuthoringView, err error) {
			err = withAuthoring(ctx, func(b *Biz, _ *runtimecore.Session) error {
				view, err = b.ConfirmDefinitionAuthoringView(ctx, common.WorkspaceID(id), request)
				return err
			})
			return
		},
	}
	p.ports.Definition.CreateDefinitionAuthoring = authoring.Create
	p.ports.Definition.ReadDefinitionAuthoring = authoring.Read
	p.ports.Definition.SaveDefinitionAuthoring = authoring.Save
	p.ports.Definition.CompileDefinitionAuthoring = authoring.Compile
	p.ports.Definition.ConfirmDefinitionAuthoring = authoring.Confirm
}
