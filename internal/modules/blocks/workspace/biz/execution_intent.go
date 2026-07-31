package workspace

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceExecutionIngressSource = "application/workorchd"

// handleExecutionIntentCommand is the Workspace owner ingress for both
// /run resume and /run restart.  It has no caller-specified target state: the
// owner derives the active Run selection and current Goal/DoD identity before
// forwarding a concrete fact to the Run owner.
func (m *Biz) handleExecutionIntentCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeExecutionIntent(ev)
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) executeExecutionIntent(ev magicevent.Event) (workspaceevents.WorkspaceExecutionIntentResult, error) {
	if m == nil || m.workspaces == nil || m.goals == nil || m.dods == nil || m.EventHub() == nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(workspaceevents.WorkspaceExecutionIntentCommand)
	if !ok || ev.Source() != workspaceExecutionIngressSource {
		return workspaceevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	workspace, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	if cmd.Kind == workspaceevents.WorkspaceExecutionIntentRestart {
		workspace, err = m.materializeDefaultLLMTargetFromBootstrap(ctx, workspace, cmd.Bootstrap)
		if err != nil {
			return workspaceevents.WorkspaceExecutionIntentResult{}, err
		}
	}
	goal, dod, err := m.currentExecutionDefinitions(ctx, workspace.ID)
	if err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	definitionRevision := workspaceExecutionDefinitionRevision(goal.ID, dod.ID)
	if cmd.Kind == workspaceevents.WorkspaceExecutionIntentRestart {
		definition := cmd.Bootstrap.Definition.Normalized()
		if definition.GoalID != goal.ID || definition.DoDID != dod.ID || definition.Revision != definitionRevision {
			return workspaceevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: restart bootstrap does not match Workspace current definition", common.ErrConflict)
		}
	}

	resultValue := workspaceevents.WorkspaceExecutionIntentResult{
		WorkspaceID: workspace.ID,
		RequestID:   cmd.RequestID,
		Kind:        cmd.Kind,
	}
	if cmd.Kind == workspaceevents.WorkspaceExecutionIntentResume && workspace.ActiveRunID == "" {
		resultValue.Disposition = "restart_required"
		resultValue.Diagnostic = "workspace has no active run"
		return resultValue, nil
	}

	runKind := runevents.WorkspaceExecutionIntentResume
	if cmd.Kind == workspaceevents.WorkspaceExecutionIntentRestart {
		runKind = runevents.WorkspaceExecutionIntentRestart
	}
	forward := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID:        workspace.ID,
		RequestID:          cmd.RequestID,
		Kind:               runKind,
		GoalID:             goal.ID,
		DoDID:              dod.ID,
		DefinitionRevision: definitionRevision,
		Priority:           cmd.Priority,
	}
	if runKind == runevents.WorkspaceExecutionIntentResume {
		forward.RunID = workspace.ActiveRunID
	} else {
		bootstrap := cmd.Bootstrap.Clone()
		forward.Bootstrap = &bootstrap
	}

	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandWorkspaceExecutionIntent,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		forward,
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "run", SubjectID: string(forward.RunID), WorkspaceID: string(workspace.ID), RunID: string(forward.RunID),
	}))
	response := m.SendEvent(event)
	if response == nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return workspaceevents.WorkspaceExecutionIntentResult{}, err.Cause
		}
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, resultErr
	}
	runResult, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if !ok || runResult.WorkspaceID != workspace.ID || runResult.RequestID != cmd.RequestID || runResult.Kind != runKind {
		return workspaceevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	resultValue.RunID = runResult.RunID
	resultValue.Disposition = strings.TrimSpace(runResult.Disposition)
	resultValue.Diagnostic = strings.TrimSpace(runResult.Diagnostic)
	return resultValue, nil
}

// materializeDefaultLLMTargetFromBootstrap is an owner-local Workspace
// metadata transition. The value is derived only from the validated sealed
// bootstrap of a successful restart admission; resume never changes the
// next-run preference and application ingress never writes Workspace state.
func (m *Biz) materializeDefaultLLMTargetFromBootstrap(ctx context.Context, workspace *workspacemodel.Workspace, bootstrap *runtimecontract.RuntimeBootstrap) (*workspacemodel.Workspace, error) {
	if workspace == nil || bootstrap == nil {
		return nil, common.ErrInvalidArgument
	}
	if workspace.DefaultLLMTarget != nil {
		return workspace, nil
	}
	target := bootstrap.Activation.ProviderTarget.Normalized()
	if err := target.Validate(); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		current := workspace
		if attempt > 0 {
			var err error
			current, err = m.workspaces.Get(ctx, workspace.ID)
			if err != nil {
				return nil, err
			}
			if current.DefaultLLMTarget != nil {
				return current, nil
			}
		}
		next := *current
		next.DefaultLLMTarget = &target
		if err := m.workspaces.CompareAndSwap(ctx, &next, current.Revision); err != nil {
			if err == common.ErrConflict {
				continue
			}
			return nil, err
		}
		m.publishWorkspaceUpdated(ctx, &next)
		return &next, nil
	}
	return nil, common.ErrConflict
}

// currentExecutionDefinitions is deliberately owner-local.  A Workspace may
// have only one live leaf for each definition lineage; ambiguous historical
// records are rejected rather than guessed by an ingress or a Run.
func (m *Biz) currentExecutionDefinitions(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Goal, *workspacemodel.DoD, error) {
	goals, err := m.goals.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	goal := currentGoalLeaf(goals, workspaceID)
	dod := currentDoDLeaf(dods, workspaceID)
	if goal == nil || dod == nil {
		return nil, nil, fmt.Errorf("%w: Workspace current Goal/DoD selection is unavailable or ambiguous", common.ErrConflict)
	}
	return goal, dod, nil
}

func currentGoalLeaf(items []*workspacemodel.Goal, workspaceID common.WorkspaceID) *workspacemodel.Goal {
	predecessors := make(map[common.GoalID]struct{}, len(items))
	for _, item := range items {
		if item != nil && item.WorkspaceID == workspaceID && item.PreviousGoalID != "" {
			predecessors[item.PreviousGoalID] = struct{}{}
		}
	}
	var selected *workspacemodel.Goal
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || strings.TrimSpace(item.LineageState) != "active" {
			continue
		}
		if _, superseded := predecessors[item.ID]; superseded {
			continue
		}
		if selected != nil && selected.ID != item.ID {
			return nil
		}
		selected = item
	}
	return selected
}

func currentDoDLeaf(items []*workspacemodel.DoD, workspaceID common.WorkspaceID) *workspacemodel.DoD {
	predecessors := make(map[common.DoDID]struct{}, len(items))
	for _, item := range items {
		if item != nil && item.WorkspaceID == workspaceID && item.PreviousDoDID != "" {
			predecessors[item.PreviousDoDID] = struct{}{}
		}
	}
	var selected *workspacemodel.DoD
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || strings.TrimSpace(item.LineageState) != "active" || (item.State != common.DoDStateReady && item.State != common.DoDStateSatisfied) {
			continue
		}
		if _, superseded := predecessors[item.ID]; superseded {
			continue
		}
		if selected != nil && selected.ID != item.ID {
			return nil
		}
		selected = item
	}
	return selected
}

func workspaceExecutionDefinitionRevision(goalID common.GoalID, dodID common.DoDID) string {
	return "goal:" + strings.TrimSpace(string(goalID)) + "|dod:" + strings.TrimSpace(string(dodID))
}
