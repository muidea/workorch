package orchestrator

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeAdapter interface {
	ExecuteStep(context.Context, common.RunID, common.StepID) (map[string]any, error)
}

type RuntimeOrchestrator struct {
	runtime RuntimeAdapter
}

func NewRuntimeOrchestrator(runtime RuntimeAdapter) *RuntimeOrchestrator {
	return &RuntimeOrchestrator{runtime: runtime}
}

func (o *RuntimeOrchestrator) StartRun(ctx context.Context, runID common.RunID) error {
	_ = ctx
	_ = runID
	return nil
}
func (o *RuntimeOrchestrator) DispatchNextStep(ctx context.Context, runID common.RunID) error {
	_ = ctx
	_ = runID
	return nil
}
func (o *RuntimeOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	_ = ctx
	_ = runID
	_ = ticketID
	return nil
}
func (o *RuntimeOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	_ = ctx
	_ = runID
	_ = approvalID
	return nil
}
func (o *RuntimeOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	_ = ctx
	_ = runID
	return nil
}
func (o *RuntimeOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	_ = ctx
	_ = runID
	return nil
}
func (o *RuntimeOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	_ = ctx
	_ = runID
	_ = reason
	return nil
}
