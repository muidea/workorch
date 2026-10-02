package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Goal/DoD 与 quality 的 owner 适配仅供局部测试；HTTP 不注册退休定义写路由。
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
	s.updateGoal = func(ctx context.Context, _, _, workspaceID, goalID, text string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateGoalWithSession(ctx, session, workspaceID, goalID, text)
			return err
		})
		return ret, err
	}
	s.readGoal = func(ctx context.Context, _, _, workspaceID, goalID string) (workorchmanagementcontract.Goal, error) {
		var ret workorchmanagementcontract.Goal
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadGoalWithSession(ctx, session, workspaceID, goalID)
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
	s.updateDoD = func(ctx context.Context, _, _, workspaceID, dodID string, items []string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateDoDWithSession(ctx, session, workspaceID, dodID, items)
			return err
		})
		return ret, err
	}
	s.readDoD = func(ctx context.Context, _, _, workspaceID, dodID string) (workorchmanagementcontract.DoD, error) {
		var ret workorchmanagementcontract.DoD
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadDoDWithSession(ctx, session, workspaceID, dodID)
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
}
