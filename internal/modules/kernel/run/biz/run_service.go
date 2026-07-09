package run

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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

type WorkspaceReader interface {
	Get(context.Context, common.WorkspaceID) (*workspaceModel.Workspace, error)
}

type workspaceRepositoryReader struct {
	repository.WorkspaceRepository
}

type Service struct {
	runs         repository.RunRepository
	workspaces   WorkspaceReader
	orchestrator Orchestrator
	eventHub     events.Hub
}

func NewService(runs repository.RunRepository, workspaces repository.WorkspaceRepository, orchestrator Orchestrator, eventHub events.Hub) *Service {
	var reader WorkspaceReader
	if workspaces != nil {
		reader = workspaceRepositoryReader{WorkspaceRepository: workspaces}
	}
	return NewServiceWithWorkspaceReader(runs, reader, orchestrator, eventHub)
}

func NewServiceWithWorkspaceReader(runs repository.RunRepository, workspaces WorkspaceReader, orchestrator Orchestrator, eventHub events.Hub) *Service {
	return &Service{runs: runs, workspaces: workspaces, orchestrator: orchestrator, eventHub: eventHub}
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

	definitionRevision := firstNonEmptyString(cmd.DefinitionRevision, runDefinitionRevisionFromExecutionContext(cmd.ExecutionContext))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	run := &runModel.Run{
		ID:                 cmd.RunID,
		WorkspaceID:        cmd.WorkspaceID,
		GoalID:             cmd.GoalID,
		DoDID:              cmd.DoDID,
		Lineage:            runModel.RunLineage{LineageState: "active"},
		DefinitionRevision: definitionRevision,
		InitialStepKind:    cmd.InitialStepKind,
		ExecutionContext:   cloneExecutionContext(cmd.ExecutionContext),
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

func (s *Service) EnsureRun(ctx context.Context, cmd runcontracts.CreateRunCommand) error {
	if cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}

	run, err := s.loadRun(ctx, cmd.RunID, cmd.WorkspaceID)
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

func (s *Service) StartRun(ctx context.Context, cmd runcontracts.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.StartRun(ctx, cmd.RunID)
}

func (s *Service) EnsureStarted(ctx context.Context, cmd runcontracts.StartRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
	if err != nil {
		return err
	}
	if r.State == common.RunStateCompleted {
		return nil
	}
	if isTerminalRunState(r.State) || r.State == common.RunStateActive || r.State == common.RunStateBlockedOnDecision || r.State == common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return s.StartRun(ctx, cmd)
}

func (s *Service) ResumeRun(ctx context.Context, cmd runcontracts.ResumeRunCommand) error {
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
		if cmd.WorkspaceID != "" && cmd.LoadedRun.WorkspaceID != cmd.WorkspaceID {
			return common.ErrNotFound
		}
		if patcher, ok := s.orchestrator.(LoadedRunExecutionContextPatchOrchestrator); ok {
			return patcher.ResumeLoadedRunWithExecutionContextPatch(ctx, cmd.LoadedRun, cloneExecutionContext(cmd.ExecutionContextPatch))
		}
	} else if s.runs != nil {
		loaded, err := s.loadRun(ctx, cmd.RunID, cmd.WorkspaceID)
		if err != nil {
			return err
		}
		if loaded.State == common.RunStateActive {
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

func (s *Service) EnsureResumed(ctx context.Context, cmd runcontracts.ResumeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.loadRun(ctx, cmd.RunID, cmd.WorkspaceID)
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

func (s *Service) BlockOnDecision(ctx context.Context, cmd runcontracts.BlockOnDecisionCommand) error {
	if cmd.RunID == "" || cmd.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
}

func (s *Service) EnsureBlockedOnDecision(ctx context.Context, cmd runcontracts.BlockOnDecisionCommand) error {
	if cmd.RunID == "" || cmd.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
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

func (s *Service) BlockOnApproval(ctx context.Context, cmd runcontracts.BlockOnApprovalCommand) error {
	if cmd.RunID == "" || cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}
	return s.orchestrator.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
}

func (s *Service) EnsureBlockedOnApproval(ctx context.Context, cmd runcontracts.BlockOnApprovalCommand) error {
	if cmd.RunID == "" || cmd.ApprovalID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator == nil {
		return common.ErrInvalidArgument
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
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

func (s *Service) ValidateRun(ctx context.Context, cmd runcontracts.ValidateRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.ValidateRun(ctx, cmd.RunID)
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
	if err != nil {
		return err
	}
	if r.State != common.RunStateActive {
		return common.ErrConflict
	}
	r.State = common.RunStateActive
	r.FailureReason = ""
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunStateChanged, r)
}

func (s *Service) CompleteRun(ctx context.Context, cmd runcontracts.CompleteRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.CompleteRun(ctx, cmd.RunID)
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
	if err != nil {
		return err
	}
	if r.State != common.RunStateActive {
		return common.ErrConflict
	}
	r.State = common.RunStateCompleted
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if strings.TrimSpace(r.EndedAt) == "" {
		r.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunStateChanged, r)
}

func (s *Service) CancelRun(ctx context.Context, cmd runcontracts.CancelRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.CancelRun(ctx, cmd.RunID)
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
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
	if strings.TrimSpace(r.EndedAt) == "" {
		r.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunStateChanged, r)
}

func (s *Service) SupersedeRun(ctx context.Context, cmd runcontracts.SupersedeRunCommand) error {
	if cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.SupersedeRun(ctx, cmd.RunID)
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
	if err != nil {
		return err
	}
	if !canSupersedeRun(r.State) {
		return common.ErrConflict
	}
	r.State = common.RunStateSuperseded
	if strings.TrimSpace(r.Lineage.LineageState) == "" || r.Lineage.LineageState == "active" {
		r.Lineage.LineageState = "superseded"
	}
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if strings.TrimSpace(r.EndedAt) == "" {
		r.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunStateChanged, r)
}

func (s *Service) FailRun(ctx context.Context, cmd runcontracts.FailRunCommand) error {
	if cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}
	if s.orchestrator != nil {
		return s.orchestrator.FailRun(ctx, cmd.RunID, cmd.Reason)
	}

	r, err := s.loadRun(ctx, cmd.RunID, "")
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
	if strings.TrimSpace(r.EndedAt) == "" {
		r.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := s.runs.Update(ctx, r); err != nil {
		return err
	}
	return s.publishRunEvent(ctx, runevents.TopicRunStateChanged, r)
}

func isTerminalRunState(state common.RunState) bool {
	return common.RunStateIsTerminal(state)
}

func canCancelRun(state common.RunState) bool {
	return common.RunStateCanTransition(state, common.RunStateCancelled)
}

func canSupersedeRun(state common.RunState) bool {
	return common.RunStateCanTransition(state, common.RunStateSuperseded)
}

func canFailRun(state common.RunState) bool {
	return common.RunStateCanTransition(state, common.RunStateFailed)
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
	if s.eventHub == nil || run == nil {
		return nil
	}
	return events.PublishByContract(ctx, s.eventHub, events.Envelope{
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
