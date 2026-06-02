package run

import (
	"context"
	"errors"
	"reflect"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Orchestrator interface {
	StartRun(context.Context, common.RunID) error
	BlockOnDecision(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval(context.Context, common.RunID, common.ApprovalID) error
	ResumeRun(context.Context, common.RunID) error
	ValidateRun(context.Context, common.RunID) error
	CompleteRun(context.Context, common.RunID) error
	CancelRun(context.Context, common.RunID) error
	SupersedeRun(context.Context, common.RunID) error
	FailRun(context.Context, common.RunID, string) error
}

type ExecutionContextPatchOrchestrator interface {
	ResumeRunWithExecutionContextPatch(context.Context, common.RunID, map[string]any) error
}

type LoadedRunExecutionContextPatchOrchestrator interface {
	ResumeLoadedRunWithExecutionContextPatch(context.Context, *runModel.Run, map[string]any) error
}

type Service struct {
	runs         repository.RunRepository
	workspaces   repository.WorkspaceRepository
	orchestrator Orchestrator
	eventHub     events.Hub
}

func NewService(runs repository.RunRepository, workspaces repository.WorkspaceRepository, orchestrator Orchestrator, eventHub events.Hub) *Service {
	return &Service{runs: runs, workspaces: workspaces, orchestrator: orchestrator, eventHub: eventHub}
}

func (s *Service) CreateRun(ctx context.Context, cmd runservice.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if _, err := s.workspaces.Get(ctx, cmd.WorkspaceID); err != nil {
		return err
	}

	definitionRevision := firstNonEmptyString(cmd.DefinitionRevision, runDefinitionRevisionFromExecutionContext(cmd.ExecutionContext))
	run := &runModel.Run{
		ID:                 cmd.RunID,
		WorkspaceID:        cmd.WorkspaceID,
		GoalID:             cmd.GoalID,
		DoDID:              cmd.DoDID,
		DefinitionRevision: definitionRevision,
		InitialStepKind:    cmd.InitialStepKind,
		ExecutionContext:   cloneExecutionContext(cmd.ExecutionContext),
		State:              common.RunStateCreated,
		Priority:           cmd.Priority,
		FailureReason:      "",
	}
	if err := s.runs.Create(ctx, run); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, events.TopicRunCreated, run)
}

func (s *Service) EnsureRun(ctx context.Context, cmd runservice.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}

	run, err := s.runs.Get(ctx, cmd.RunID)
	if err == nil {
		definitionRevision := firstNonEmptyString(cmd.DefinitionRevision, runDefinitionRevisionFromExecutionContext(cmd.ExecutionContext))
		if run.WorkspaceID != cmd.WorkspaceID ||
			run.GoalID != cmd.GoalID ||
			run.DoDID != cmd.DoDID ||
			run.DefinitionRevision != definitionRevision ||
			run.InitialStepKind != cmd.InitialStepKind ||
			run.Priority != cmd.Priority ||
			!reflect.DeepEqual(run.ExecutionContext, cmd.ExecutionContext) {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateRun(ctx, cmd)
}

func runDefinitionRevisionFromExecutionContext(input map[string]any) string {
	revision, _ := input["definitionRevision"].(map[string]any)
	if len(revision) == 0 {
		return ""
	}
	if id := strings.TrimSpace(stringFromMap(revision, "id")); id != "" {
		return id
	}
	goalID := strings.TrimSpace(stringFromMap(revision, "effectiveGoalId"))
	doDID := strings.TrimSpace(stringFromMap(revision, "effectiveDoDId"))
	if goalID == "" && doDID == "" {
		return ""
	}
	return "goal:" + goalID + "|dod:" + doDID
}

func stringFromMap(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneExecutionContext(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func (s *Service) StartRun(ctx context.Context, cmd runservice.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.StartRun(ctx, cmd.RunID)
}

func (s *Service) EnsureStarted(ctx context.Context, cmd runservice.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateCompleted {
		return nil
	}
	if isTerminalRunState(r.State) || r.State == common.RunStateValidating || r.State == common.RunStateBlockedOnDecision || r.State == common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return s.StartRun(ctx, cmd)
}

func (s *Service) ResumeRun(ctx context.Context, cmd runservice.ResumeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	if cmd.LoadedRun != nil {
		if cmd.LoadedRun.ID != cmd.RunID {
			return common.ErrConflict
		}
		if patcher, ok := s.orchestrator.(LoadedRunExecutionContextPatchOrchestrator); ok {
			return patcher.ResumeLoadedRunWithExecutionContextPatch(ctx, cmd.LoadedRun, cloneExecutionContext(cmd.ExecutionContextPatch))
		}
	} else if s.runs != nil {
		loaded, err := s.runs.Get(ctx, cmd.RunID)
		if err != nil {
			return err
		}
		if loaded.State == common.RunStateRunning {
			return common.ErrConflict
		}
		if patcher, ok := s.orchestrator.(LoadedRunExecutionContextPatchOrchestrator); ok {
			return patcher.ResumeLoadedRunWithExecutionContextPatch(ctx, loaded, cloneExecutionContext(cmd.ExecutionContextPatch))
		}
	}
	if len(cmd.ExecutionContextPatch) > 0 {
		if patcher, ok := s.orchestrator.(ExecutionContextPatchOrchestrator); ok {
			return patcher.ResumeRunWithExecutionContextPatch(ctx, cmd.RunID, cloneExecutionContext(cmd.ExecutionContextPatch))
		}
	}
	return s.orchestrator.ResumeRun(ctx, cmd.RunID)
}

func (s *Service) EnsureResumed(ctx context.Context, cmd runservice.ResumeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if isTerminalRunState(r.State) {
		return nil
	}
	if r.State != common.RunStateBlockedOnDecision &&
		r.State != common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return s.ResumeRun(ctx, cmd)
}

func (s *Service) BlockOnDecision(ctx context.Context, cmd runservice.BlockOnDecisionCommand) error {
	if cmd.RunID == "" || cmd.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
}

func (s *Service) EnsureBlockedOnDecision(ctx context.Context, cmd runservice.BlockOnDecisionCommand) error {
	if cmd.RunID == "" || cmd.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateBlockedOnDecision && r.BlockingTicketID == cmd.TicketID {
		return nil
	}
	if r.State == common.RunStateBlockedOnDecision && r.BlockingTicketID != "" && r.BlockingTicketID != cmd.TicketID {
		return common.ErrConflict
	}
	if r.State == common.RunStateBlockedOnApproval && r.BlockingApprovalID != "" {
		return common.ErrConflict
	}
	if isTerminalRunState(r.State) {
		return nil
	}
	return s.BlockOnDecision(ctx, cmd)
}

func (s *Service) BlockOnApproval(ctx context.Context, cmd runservice.BlockOnApprovalCommand) error {
	if cmd.RunID == "" || cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
}

func (s *Service) EnsureBlockedOnApproval(ctx context.Context, cmd runservice.BlockOnApprovalCommand) error {
	if cmd.RunID == "" || cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State == common.RunStateBlockedOnApproval && r.BlockingApprovalID == cmd.ApprovalID {
		return nil
	}
	if r.State == common.RunStateBlockedOnApproval && r.BlockingApprovalID != "" && r.BlockingApprovalID != cmd.ApprovalID {
		return common.ErrConflict
	}
	if r.State == common.RunStateBlockedOnDecision && r.BlockingTicketID != "" {
		return common.ErrConflict
	}
	if isTerminalRunState(r.State) {
		return nil
	}
	return s.BlockOnApproval(ctx, cmd)
}

func (s *Service) ValidateRun(ctx context.Context, cmd runservice.ValidateRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.ValidateRun(ctx, cmd.RunID)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateValidating
	r.FailureReason = ""
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, events.TopicRunStateChanged, r)
}

func (s *Service) CompleteRun(ctx context.Context, cmd runservice.CompleteRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.CompleteRun(ctx, cmd.RunID)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateRunning && r.State != common.RunStateValidating {
		return common.ErrConflict
	}
	r.State = common.RunStateCompleted
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, events.TopicRunStateChanged, r)
}

func (s *Service) CancelRun(ctx context.Context, cmd runservice.CancelRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.CancelRun(ctx, cmd.RunID)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if !canCancelRun(r.State) {
		return common.ErrConflict
	}
	r.State = common.RunStateCancelled
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, events.TopicRunStateChanged, r)
}

func (s *Service) SupersedeRun(ctx context.Context, cmd runservice.SupersedeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.SupersedeRun(ctx, cmd.RunID)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if !canSupersedeRun(r.State) {
		return common.ErrConflict
	}
	r.State = common.RunStateSuperseded
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, events.TopicRunStateChanged, r)
}

func (s *Service) FailRun(ctx context.Context, cmd runservice.FailRunCommand) error {
	if cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.FailRun(ctx, cmd.RunID, cmd.Reason)
	}

	r, err := s.runs.Get(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if !canFailRun(r.State) {
		return common.ErrConflict
	}
	r.State = common.RunStateFailed
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = cmd.Reason
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, events.TopicRunStateChanged, r)
}

func isTerminalRunState(state common.RunState) bool {
	return state == common.RunStateCompleted ||
		state == common.RunStateFailed ||
		state == common.RunStateCancelled ||
		state == common.RunStateSuperseded
}

func canCancelRun(state common.RunState) bool {
	switch state {
	case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
		return true
	default:
		return false
	}
}

func canSupersedeRun(state common.RunState) bool {
	switch state {
	case common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
		return true
	default:
		return false
	}
}

func canFailRun(state common.RunState) bool {
	switch state {
	case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
		return true
	default:
		return false
	}
}

func (s *Service) publishRunEvent(ctx context.Context, topic string, run *runModel.Run) error {
	if s.eventHub == nil || run == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "run",
		SubjectID:    string(run.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		Payload: events.RunPayload(
			run.GoalID,
			run.DoDID,
			run.State,
			run.BlockingTicketID,
			run.BlockingApprovalID,
			run.FailureReason,
		),
	})
}
