package workspace

import (
	"context"
	"reflect"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func (m *Biz) reservePreparationRegistration(ctx context.Context, current *workspace.Workspace, index int, c workspaceevents.InitializeWorkspacePreparationCommand) (workspaceevents.InitializeWorkspacePreparationResult, error) {
	empty := workspaceevents.InitializeWorkspacePreparationResult{}
	r := current.PreparationResourceInitializations[index]
	if !r.Matches(current) || !r.FileObserved() || r.Registration == nil || r.Registration.Plan.Home != c.RegistrationHome || index != len(current.PreparationResourceInitializations)-1 {
		return empty, common.ErrConflict
	}
	g := r.Registration
	if g.Succeeded() {
		return preparationInitializationResult(current, c)
	}
	if len(g.Attempts) >= skillhubpreparation.RegistrationAttemptLimit || g.Status() == "failed" {
		return preparationInitializationResult(current, c)
	}
	next := *current
	next.Revision++
	next.PreparationResourceInitializations = workspace.ClonePreparationInitializations(current.PreparationResourceInitializations)
	updated := &next.PreparationResourceInitializations[index]
	updated.CommittedWorkspaceRevision = next.Revision
	updated.Registration.Attempts = append(updated.Registration.Attempts, workspace.PreparationRegistrationAttempt{PreparedAt: time.Now().UTC(), Query: len(g.Attempts) > 0})
	if err := m.workspaces.CompareAndSwap(ctx, &next, current.Revision); err != nil {
		return empty, err
	}
	out, err := preparationInitializationResult(&next, c)
	if err != nil {
		return empty, err
	}
	out.Registration = &workspaceevents.PreparationRegistrationDispatch{Plan: g.Plan, Attempt: len(g.Attempts) + 1, Query: len(g.Attempts) > 0, CommittedWorkspaceRevision: next.Revision}
	return out, out.ValidateFor(c)
}

func (m *Biz) handlePreparationRegistrationSettle(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceevents.CommandSource || ev.ID() != workspaceevents.TopicWorkspacePreparationRegistrationSettle {
		result.Set(nil, cd.NewError(cd.IllegalParam, "invalid registration settlement source"))
		return
	}
	c, ok := ev.Data().(workspaceevents.SettlePreparationRegistrationCommand)
	if !ok {
		result.Set(nil, cd.NewError(cd.IllegalParam, "invalid registration settlement"))
		return
	}
	out, err := m.settlePreparationRegistration(ev.Context(), c)
	if err != nil {
		result.Set(nil, cd.WrapError(cd.Unexpected, err, "registration receipt unavailable; reconcile original identity"))
		return
	}
	result.Set(out, nil)
}
func (m *Biz) settlePreparationRegistration(ctx context.Context, c workspaceevents.SettlePreparationRegistrationCommand) (workspaceevents.InitializeWorkspacePreparationResult, error) {
	empty := workspaceevents.InitializeWorkspacePreparationResult{}
	if m == nil || ctx == nil || c.Original.Validate() != nil || c.Dispatch.Plan.Validate() != nil || c.Observation.Validate() != nil {
		return empty, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, c.Original.WorkspaceID)
	if err != nil {
		return empty, err
	}
	current, err := m.workspaces.Get(bound, c.Original.WorkspaceID)
	if err != nil {
		return empty, err
	}
	if workspaceDeletionRejectsMutation(current) != nil || workspace.ValidatePreparationInitializations(current) != nil || len(current.PreparationResourceInitializations) == 0 {
		return empty, common.ErrConflict
	}
	index := len(current.PreparationResourceInitializations) - 1
	r := current.PreparationResourceInitializations[index]
	g := r.Registration
	if r.Plan.RequestID != c.Original.RequestID || r.ExpectedWorkspaceRevision != c.Original.ExpectedWorkspaceRevision || !r.Matches(current) || g == nil || g.Plan != c.Dispatch.Plan || g.Plan.Home != c.Original.RegistrationHome || len(g.Attempts) != c.Dispatch.Attempt || len(g.Attempts) == 0 || g.Attempts[len(g.Attempts)-1].Query != c.Dispatch.Query {
		return empty, common.ErrConflict
	}
	a := g.Attempts[len(g.Attempts)-1]
	if a.Result != nil {
		if !reflect.DeepEqual(*a.Result, c.Observation) {
			return empty, common.ErrConflict
		}
		return preparationInitializationResult(current, c.Original)
	}
	if c.Dispatch.CommittedWorkspaceRevision != current.Revision || c.Observation.StartedAt.Before(a.PreparedAt) {
		return empty, common.ErrConflict
	}
	next := *current
	next.Revision++
	next.PreparationResourceInitializations = workspace.ClonePreparationInitializations(current.PreparationResourceInitializations)
	updated := &next.PreparationResourceInitializations[index]
	updated.CommittedWorkspaceRevision = next.Revision
	saved := c.Observation.Clone()
	updated.Registration.Attempts[len(g.Attempts)-1].Result = &saved
	if err := m.workspaces.CompareAndSwap(bound, &next, current.Revision); err != nil {
		return empty, err
	}
	return preparationInitializationResult(&next, c.Original)
}
