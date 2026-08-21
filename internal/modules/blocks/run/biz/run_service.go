package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceReader interface {
	Get(context.Context, common.WorkspaceID) (*workspaceModel.Workspace, error)
}

type Service struct {
	runs       repository.RunRepository
	workspaces workspaceReader
	notifier   runStateNotifier
}

type runStateNotifier interface {
	PublishRunState(context.Context, string, *runModel.Run) error
}

type eventRunStateNotifier struct {
	post func(magicevent.Event)
}

func newServiceWithPorts(runs repository.RunRepository, workspaces workspaceReader, notifier runStateNotifier) *Service {
	return &Service{runs: runs, workspaces: workspaces, notifier: notifier}
}

func (s *Service) CreateRun(ctx context.Context, cmd runevents.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if _, err := runWorkspaceScope(ctx, cmd.WorkspaceID); err != nil {
		return err
	}
	if _, err := s.workspaces.Get(ctx, cmd.WorkspaceID); err != nil {
		return err
	}

	definitionRevision := strings.TrimSpace(cmd.DefinitionRevision)
	if err := validateRuntimeBootstrapForRun(cmd.RuntimeBootstrap, cmd.GoalID, cmd.DoDID, definitionRevision); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	startupInputHash := ""
	if cmd.RuntimeBootstrap != nil {
		startupInputHash = cmd.RuntimeBootstrap.Definition.Normalized().StartupInputHash
	}
	run := &runModel.Run{
		ID:                 cmd.RunID,
		WorkspaceID:        cmd.WorkspaceID,
		GoalID:             cmd.GoalID,
		DoDID:              cmd.DoDID,
		Lineage:            createRunLineage(cmd.PredecessorRunID, cmd.PredecessorResultID),
		DefinitionRevision: definitionRevision,
		StartupInputHash:   startupInputHash,
		InitialStepKind:    cmd.InitialStepKind,
		RuntimeBootstrap:   cloneRuntimeBootstrap(cmd.RuntimeBootstrap),
		State:              common.RunStatePending,
		Priority:           cmd.Priority,
		FailureReason:      "",
		CreatedAt:          now,
	}
	if err := s.runs.Create(ctx, run); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunCreated, run)
}

func validateRuntimeBootstrapForRun(bootstrap *runtimemodel.RuntimeBootstrap, goalID common.GoalID, dodID common.DoDID, definitionRevision string) error {
	if bootstrap == nil {
		return nil
	}
	value := bootstrap.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Definition.GoalID != goalID || value.Definition.DoDID != dodID || value.Definition.Revision != definitionRevision {
		return fmt.Errorf("%w: runtime bootstrap definition does not match run", common.ErrConflict)
	}
	return nil
}

func (s *Service) EnsureRun(ctx context.Context, cmd runevents.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}

	run, err := s.loadRun(ctx, cmd.RunID, cmd.WorkspaceID)
	if err == nil {
		definitionRevision := strings.TrimSpace(cmd.DefinitionRevision)
		if run.WorkspaceID != cmd.WorkspaceID ||
			run.GoalID != cmd.GoalID ||
			run.DoDID != cmd.DoDID ||
			run.DefinitionRevision != definitionRevision ||
			run.StartupInputHash != startupInputHashForBootstrap(cmd.RuntimeBootstrap) ||
			run.InitialStepKind != cmd.InitialStepKind ||
			run.Priority != cmd.Priority ||
			!sameRuntimeBootstrap(run.RuntimeBootstrap, cmd.RuntimeBootstrap) {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateRun(ctx, cmd)
}

func startupInputHashForBootstrap(bootstrap *runtimemodel.RuntimeBootstrap) string {
	if bootstrap == nil {
		return ""
	}
	return bootstrap.Definition.Normalized().StartupInputHash
}

func createRunLineage(predecessorRunID common.RunID, predecessorResultID common.ResultID) runModel.RunLineage {
	return runModel.RunLineage{
		PredecessorRunID:    common.RunID(strings.TrimSpace(string(predecessorRunID))),
		PredecessorResultID: common.ResultID(strings.TrimSpace(string(predecessorResultID))),
		LineageState:        "active",
	}
}

func cloneRuntimeBootstrap(value *runtimemodel.RuntimeBootstrap) *runtimemodel.RuntimeBootstrap {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
}

func sameRuntimeBootstrap(left *runtimemodel.RuntimeBootstrap, right *runtimemodel.RuntimeBootstrap) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return runtimemodel.SameRuntimeBootstrap(*left, *right)
}

func (s *Service) loadRun(ctx context.Context, runID common.RunID, explicitWorkspaceID common.WorkspaceID) (*runModel.Run, error) {
	if s == nil || s.runs == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID, err := runWorkspaceScope(ctx, explicitWorkspaceID)
	if err != nil {
		return nil, err
	}
	if workspaceID != "" {
		return s.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	}
	return s.runs.Get(ctx, runID)
}

func runWorkspaceScope(ctx context.Context, explicitWorkspaceID common.WorkspaceID) (common.WorkspaceID, error) {
	contextWorkspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if explicitWorkspaceID != "" && contextWorkspaceID != "" && explicitWorkspaceID != contextWorkspaceID {
		return "", common.ErrConflict
	}
	if explicitWorkspaceID != "" {
		return explicitWorkspaceID, nil
	}
	return contextWorkspaceID, nil
}

func (s *Service) publishRunEvent(ctx context.Context, topic string, run *runModel.Run) error {
	if s.notifier == nil || run == nil {
		return nil
	}
	return s.notifier.PublishRunState(ctx, topic, run)
}

func (n eventRunStateNotifier) PublishRunState(ctx context.Context, topic string, run *runModel.Run) error {
	if n.post == nil || run == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedDataFromModel(run))
	event.BindLaneKey(events.EventHubLaneKeyFor(runevents.Contracts(), topic, eventcontract.LaneScope{
		SubjectType: "run",
		SubjectID:   string(run.ID),
		WorkspaceID: string(run.WorkspaceID),
		RunID:       string(run.ID),
	}))
	n.post(event)
	return nil
}
