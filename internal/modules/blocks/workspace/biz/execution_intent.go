package workspace

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceExecutionIngressSource = "application/workorchd"

type preparedWorkspaceExecutionIntent struct {
	ctx                context.Context
	command            workspaceevents.WorkspaceExecutionIntentCommand
	workspace          *workspacemodel.Workspace
	goal               *workspacemodel.Goal
	dod                *workspacemodel.DoD
	definitionRevision string
}

// handleExecutionIntentCommand is the Workspace owner ingress for both
// /run resume and /run restart.  It has no caller-specified target state: the
// owner derives the active Run selection and current Goal/DoD identity before
// forwarding a concrete fact to the Run owner.
func (m *Biz) handleExecutionIntentCommand(ev magicevent.Event, result magicevent.Result) {
	if m != nil {
		m.deletionIngressMu.RLock()
		defer m.deletionIngressMu.RUnlock()
	}
	response, err := m.executeExecutionIntent(ev)
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	if err := response.Validate(); err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) executeExecutionIntent(ev magicevent.Event) (workspaceevents.WorkspaceExecutionIntentResult, error) {
	prepared, err := m.prepareWorkspaceExecutionIntent(ev)
	if err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	if result, admitted := prepared.ownerAdmission(); admitted {
		return result, nil
	}
	if prepared.command.Kind == runtimecontract.RuntimeExecutionIntentRestart {
		definition := prepared.command.Bootstrap.Definition.Normalized()
		workspace, err := m.submitWorkspaceExecutionInternalEvent(prepared.ctx, workspaceExecutionInternalEvent{
			Kind:              workspaceExecutionInternalEventActivationProviderSelectionObserved,
			WorkspaceID:       prepared.workspace.ID,
			StartupInputHash:  definition.StartupInputHash,
			ProviderSelection: prepared.command.Bootstrap.Activation.ProviderSelection,
		})
		if err != nil {
			return workspaceevents.WorkspaceExecutionIntentResult{}, err
		}
		prepared.workspace = workspace
	}
	forward := prepared.runCommand()
	runResult, err := m.sendRunExecutionIntent(prepared.ctx, prepared.workspace.ID, forward)
	if err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	if err := validateRunExecutionIntentResult(runResult, prepared.workspace.ID, prepared.command.RequestID, forward.Kind, forward.RunID); err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, err
	}
	return workspaceevents.WorkspaceExecutionIntentResult(runResult.RuntimeExecutionIntentAdmission), nil
}

func (m *Biz) prepareWorkspaceExecutionIntent(ev magicevent.Event) (preparedWorkspaceExecutionIntent, error) {
	if m == nil || m.workspaces == nil || m.goals == nil || m.dods == nil || m.EventHub() == nil {
		return preparedWorkspaceExecutionIntent{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(workspaceevents.WorkspaceExecutionIntentCommand)
	if !ok || ev.Source() != workspaceExecutionIngressSource {
		return preparedWorkspaceExecutionIntent{}, common.ErrInvalidArgument
	}
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return preparedWorkspaceExecutionIntent{}, err
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return preparedWorkspaceExecutionIntent{}, err
	}
	workspace, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return preparedWorkspaceExecutionIntent{}, err
	}
	if err := workspaceDeletionRejectsMutation(workspace); err != nil {
		return preparedWorkspaceExecutionIntent{}, err
	}
	if cmd.Kind == runtimecontract.RuntimeExecutionIntentResume {
		return preparedWorkspaceExecutionIntent{ctx: ctx, command: cmd, workspace: workspace}, nil
	}
	goal, dod, err := m.currentExecutionDefinitions(ctx, workspace.ID)
	if err != nil {
		return preparedWorkspaceExecutionIntent{}, err
	}
	definitionRevision := workspacecontracts.ExecutionDefinitionRevision(
		string(goal.ID), goal.DefinitionRevision,
		string(dod.ID), dod.DefinitionRevision,
	)
	if definitionRevision == "" {
		return preparedWorkspaceExecutionIntent{}, fmt.Errorf("%w: Workspace Goal/DoD content revision is incomplete", common.ErrConflict)
	}
	if err := validateWorkspaceExecutionAdmissionPersistence(workspace); err != nil {
		return preparedWorkspaceExecutionIntent{}, fmt.Errorf("%w: Workspace activation admission persistence is invalid: %v", common.ErrConflict, err)
	}
	if err := validateRestartExecutionDefinition(cmd, goal, dod, definitionRevision); err != nil {
		return preparedWorkspaceExecutionIntent{}, err
	}
	return preparedWorkspaceExecutionIntent{
		ctx: ctx, command: cmd, workspace: workspace, goal: goal, dod: dod, definitionRevision: definitionRevision,
	}, nil
}

func validateWorkspaceExecutionAdmissionPersistence(workspace *workspacemodel.Workspace) error {
	_, _, err := workspace.ValidateActivationAdmissionPersistence()
	return err
}

func validateRestartExecutionDefinition(cmd workspaceevents.WorkspaceExecutionIntentCommand, goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string) error {
	if cmd.Kind != runtimecontract.RuntimeExecutionIntentRestart {
		return nil
	}
	definition := cmd.Bootstrap.Definition.Normalized()
	if definition.GoalID != goal.ID || definition.DoDID != dod.ID || definition.Revision != definitionRevision {
		return fmt.Errorf("%w: restart bootstrap does not match Workspace current definition", common.ErrConflict)
	}
	return nil
}

func (p preparedWorkspaceExecutionIntent) ownerAdmission() (workspaceevents.WorkspaceExecutionIntentResult, bool) {
	if p.command.Kind == runtimecontract.RuntimeExecutionIntentRestart {
		if p.workspace.StartupInput == nil || p.workspace.ActivationAdmission == nil {
			return workspaceevents.WorkspaceExecutionIntentResult{
				WorkspaceID: p.workspace.ID, RequestID: p.command.RequestID, Kind: p.command.Kind,
				Outcome: runtimecontract.RuntimeExecutionIntentFreshActivationRequired,
				Reason:  runtimecontract.RuntimeExecutionIntentReasonDefinitionAdmissionMissing,
			}, true
		}
		startup := p.workspace.StartupInput.Normalized()
		admission := p.workspace.ActivationAdmission.Normalized()
		if admission.State == workspacemodel.WorkspaceActivationPendingClarificationResolution {
			return workspaceevents.WorkspaceExecutionIntentResult{
				WorkspaceID: p.workspace.ID, RequestID: p.command.RequestID, Kind: p.command.Kind,
				Outcome: runtimecontract.RuntimeExecutionIntentGovernanceRequired,
				Reason:  runtimecontract.RuntimeExecutionIntentReasonDefinitionClarificationPending,
			}, true
		}
		definition := p.command.Bootstrap.Definition.Normalized()
		if startup.Validate() != nil || admission.Validate(startup) != nil || admission.State != workspacemodel.WorkspaceActivationEligible || definition.Revision != startup.DefinitionRevision || definition.StartupInputHash != startup.StartupInputHash || runtimecontract.StableRuntimeProfileContractsHash(p.command.Bootstrap.Activation.ExecutionContract.ProfileContracts) != startup.RuntimeProfileContractsHash {
			return workspaceevents.WorkspaceExecutionIntentResult{
				WorkspaceID: p.workspace.ID, RequestID: p.command.RequestID, Kind: p.command.Kind,
				Outcome: runtimecontract.RuntimeExecutionIntentFreshActivationRequired,
				Reason:  runtimecontract.RuntimeExecutionIntentReasonDefinitionAdmissionMismatch,
			}, true
		}
	}
	if p.command.Kind != runtimecontract.RuntimeExecutionIntentResume || p.workspace.ActiveRunID != "" {
		return workspaceevents.WorkspaceExecutionIntentResult{}, false
	}
	return workspaceevents.WorkspaceExecutionIntentResult{
		WorkspaceID: p.workspace.ID,
		RequestID:   p.command.RequestID,
		Kind:        p.command.Kind,
		Outcome:     runtimecontract.RuntimeExecutionIntentRestartRequired,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent,
	}, true
}

func (p preparedWorkspaceExecutionIntent) runCommand() runevents.WorkspaceExecutionIntentCommand {
	runKind := runtimecontract.RuntimeExecutionIntentResume
	if p.command.Kind == runtimecontract.RuntimeExecutionIntentRestart {
		runKind = runtimecontract.RuntimeExecutionIntentRestart
	}
	forward := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: p.workspace.ID,
		RequestID:   p.command.RequestID,
		Kind:        runKind,
	}
	if runKind == runtimecontract.RuntimeExecutionIntentResume {
		forward.RunID = p.workspace.ActiveRunID
	} else {
		forward.GoalID = p.goal.ID
		forward.DoDID = p.dod.ID
		forward.DefinitionRevision = p.definitionRevision
		forward.StartupInputHash = p.command.Bootstrap.Definition.Normalized().StartupInputHash
		bootstrap := p.command.Bootstrap.Clone()
		forward.Bootstrap = &bootstrap
	}
	return forward
}

func (m *Biz) sendRunExecutionIntent(ctx context.Context, workspaceID common.WorkspaceID, forward runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandWorkspaceExecutionIntent,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		forward,
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "run", SubjectID: string(forward.RunID), WorkspaceID: string(workspaceID), RunID: string(forward.RunID),
	}))
	response := m.SendEvent(event)
	if response == nil {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err.Cause
		}
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return runevents.WorkspaceExecutionIntentResult{}, resultErr
	}
	runResult, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if !ok {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	return runResult, nil
}

func validateRunExecutionIntentResult(runResult runevents.WorkspaceExecutionIntentResult, workspaceID common.WorkspaceID, requestID string, kind runtimecontract.RuntimeExecutionIntentKind, selectedRunID common.RunID) error {
	if runResult.WorkspaceID != workspaceID || runResult.RequestID != requestID || runResult.Kind != kind {
		return common.ErrConflict
	}
	if kind == runtimecontract.RuntimeExecutionIntentResume && (selectedRunID == "" || runResult.RunID != selectedRunID) {
		return fmt.Errorf("%w: Run execution intent result changed the Workspace-selected active Run", common.ErrConflict)
	}
	if err := runResult.Validate(); err != nil {
		return fmt.Errorf("%w: Run execution intent result: %v", common.ErrConflict, err)
	}
	return nil
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
		if item == nil || item.WorkspaceID != workspaceID || strings.TrimSpace(item.LineageState) != "active" || item.State != common.DoDStateReady {
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
