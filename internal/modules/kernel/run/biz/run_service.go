package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
	hub magicevent.Hub
}

func NewServiceWithEventHub(runs repository.RunRepository, commandHub magicevent.Hub) *Service {
	return newServiceWithPorts(runs, workspaceEventReader{hub: commandHub}, eventRunStateNotifier{hub: commandHub})
}

func newServiceWithPorts(runs repository.RunRepository, workspaces workspaceReader, notifier runStateNotifier) *Service {
	return &Service{runs: runs, workspaces: workspaces, notifier: notifier}
}

func (s *Service) CreateRun(ctx context.Context, cmd runcontracts.CreateRunCommand) error {
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
	run := &runModel.Run{
		ID:                 cmd.RunID,
		WorkspaceID:        cmd.WorkspaceID,
		GoalID:             cmd.GoalID,
		DoDID:              cmd.DoDID,
		Lineage:            normalizeCreateRunLineage(cmd.Lineage),
		DefinitionRevision: definitionRevision,
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

func (s *Service) EnsureRun(ctx context.Context, cmd runcontracts.CreateRunCommand) error {
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

func normalizeCreateRunLineage(lineage runModel.RunLineage) runModel.RunLineage {
	lineage.PredecessorRunID = common.RunID(strings.TrimSpace(string(lineage.PredecessorRunID)))
	lineage.PredecessorResultID = common.ResultID(strings.TrimSpace(string(lineage.PredecessorResultID)))
	lineage.SuccessorRunID = common.RunID(strings.TrimSpace(string(lineage.SuccessorRunID)))
	lineage.LineageState = strings.TrimSpace(lineage.LineageState)
	if lineage.LineageState == "" {
		lineage.LineageState = "active"
	}
	// Create must not invent successor links; successor is linked via LinkRunSuccessor.
	lineage.SuccessorRunID = ""
	return lineage
}

// LinkRunSuccessor CAS-updates predecessor.SuccessorRunID.
// Empty expected means only empty successor is allowed; same successor is idempotent success.
func (s *Service) LinkRunSuccessor(ctx context.Context, cmd runcontracts.LinkRunSuccessorCommand) error {
	if s == nil || s.runs == nil || cmd.WorkspaceID == "" || cmd.PredecessorRunID == "" || cmd.SuccessorRunID == "" {
		return common.ErrInvalidArgument
	}
	if cmd.PredecessorRunID == cmd.SuccessorRunID {
		return common.ErrInvalidArgument
	}
	predecessor, err := s.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.PredecessorRunID)
	if err != nil {
		return err
	}
	successor, err := s.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.SuccessorRunID)
	if err != nil {
		return err
	}
	// Successor must already record this predecessor (create-time lineage).
	if successor.Lineage.PredecessorRunID != cmd.PredecessorRunID {
		return fmt.Errorf("%w: successor predecessor lineage mismatch", common.ErrConflict)
	}
	current := common.RunID(strings.TrimSpace(string(predecessor.Lineage.SuccessorRunID)))
	expected := common.RunID(strings.TrimSpace(string(cmd.ExpectedSuccessorRunID)))
	if current == cmd.SuccessorRunID {
		return nil // idempotent
	}
	if expected == "" {
		if current != "" {
			return fmt.Errorf("%w: predecessor already has successor %s", common.ErrConflict, current)
		}
	} else if current != expected {
		return fmt.Errorf("%w: predecessor successor CAS mismatch have=%s expected=%s", common.ErrConflict, current, expected)
	}
	predecessor.Lineage.SuccessorRunID = cmd.SuccessorRunID
	if strings.TrimSpace(predecessor.Lineage.LineageState) == "" {
		predecessor.Lineage.LineageState = "active"
	}
	if err := s.runs.Update(ctx, predecessor); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunStateChanged, predecessor)
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
	if n.hub == nil || run == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedDataFromModel(run))
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: "run",
		SubjectID:   string(run.ID),
		WorkspaceID: string(run.WorkspaceID),
		RunID:       string(run.ID),
	}))
	n.hub.Post(event)
	return nil
}
