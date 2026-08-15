package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraft"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Goal/DoD/quality/setup-draft adapters form one definition-focused slice.
func (s *serverState) configureDefinitionOperations(session *runtimecore.Session) {
	s.defineGoal = func(ctx context.Context, _, _, workspaceID, text string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.DefineGoalWithSession(ctx, session, workspaceID, text)
			return err
		})
		return ret, err
	}
	s.updateGoal = func(ctx context.Context, _, _, goalID, text string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateGoalWithSession(ctx, session, goalID, text)
			return err
		})
		return ret, err
	}
	s.readGoal = func(ctx context.Context, _, _, goalID string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadGoalWithSession(ctx, session, goalID)
			return err
		})
		return ret, err
	}
	s.listGoals = func(ctx context.Context, _, _, workspaceID string) ([]workorchmanagementcontract.Goal, error) {
		var ret []workorchmanagementcontract.Goal
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListGoalsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.defineDoD = func(ctx context.Context, _, _, workspaceID string, items []string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.DefineDoDWithSession(ctx, session, workspaceID, items)
			return err
		})
		return ret, err
	}
	s.updateDoD = func(ctx context.Context, _, _, dodID string, items []string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateDoDWithSession(ctx, session, dodID, items)
			return err
		})
		return ret, err
	}
	s.readDoD = func(ctx context.Context, _, _, dodID string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadDoDWithSession(ctx, session, dodID)
			return err
		})
		return ret, err
	}
	s.listDoDs = func(ctx context.Context, _, _, workspaceID string) ([]workorchmanagementcontract.DoD, error) {
		var ret []workorchmanagementcontract.DoD
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListDoDsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.checkGoalDoDQuality = func(ctx context.Context, _, _ string, cmd definitionquality.Command) (definitionquality.Check, error) {
		var ret definitionquality.Check
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CheckGoalDoDQualityWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.generateWorkspaceSetupDraft = func(ctx context.Context, _, _ string, cmd setupdraft.GenerationCommand) (setupdraft.Draft, error) {
		var ret setupdraft.Draft
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
			return err
		})
		return ret, err
	}
}
