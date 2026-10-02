package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (p *runtimeApplicationPorts) configureDefinitionPorts() {
	ports := &p.ports
	ports.Definition.ReadGoal = func(ctx context.Context, _, _, workspaceID, goalID string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadGoalWithSession(ctx, session, workspaceID, goalID)
			return err
		})
		return ret, err
	}
	ports.Definition.ListGoals = func(ctx context.Context, _, _, workspaceID string) ([]workorchmanagementcontract.Goal, error) {
		var ret []workorchmanagementcontract.Goal
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListGoalsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	ports.Definition.ReadDoD = func(ctx context.Context, _, _, workspaceID, dodID string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadDoDWithSession(ctx, session, workspaceID, dodID)
			return err
		})
		return ret, err
	}
	ports.Definition.ListDoDs = func(ctx context.Context, _, _, workspaceID string) ([]workorchmanagementcontract.DoD, error) {
		var ret []workorchmanagementcontract.DoD
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListDoDsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
}
