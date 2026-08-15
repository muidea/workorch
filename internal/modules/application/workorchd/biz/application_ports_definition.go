package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraft"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (p *runtimeApplicationPorts) configureDefinitionPorts() {
	ports := &p.ports
	ports.Definition.DefineGoal = func(ctx context.Context, _, _, workspaceID, text string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.DefineGoalWithSession(ctx, session, workspaceID, text)
			return err
		})
		return ret, err
	}
	ports.Definition.UpdateGoal = func(ctx context.Context, _, _, goalID, text string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateGoalWithSession(ctx, session, goalID, text)
			return err
		})
		return ret, err
	}
	ports.Definition.ReadGoal = func(ctx context.Context, _, _, goalID string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadGoalWithSession(ctx, session, goalID)
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
	ports.Definition.DefineDoD = func(ctx context.Context, _, _, workspaceID string, items []string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.DefineDoDWithSession(ctx, session, workspaceID, items)
			return err
		})
		return ret, err
	}
	ports.Definition.UpdateDoD = func(ctx context.Context, _, _, dodID string, items []string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateDoDWithSession(ctx, session, dodID, items)
			return err
		})
		return ret, err
	}
	ports.Definition.ReadDoD = func(ctx context.Context, _, _, dodID string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadDoDWithSession(ctx, session, dodID)
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
	ports.Definition.CheckGoalDoDQuality = func(ctx context.Context, _, _ string, cmd definitionquality.Command) (definitionquality.Check, error) {
		var ret definitionquality.Check
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CheckGoalDoDQualityWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Definition.GenerateWorkspaceSetupDraft = func(ctx context.Context, _, _ string, cmd setupdraft.GenerationCommand) (setupdraft.Draft, error) {
		var ret setupdraft.Draft
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
			return err
		})
		return ret, err
	}
}
