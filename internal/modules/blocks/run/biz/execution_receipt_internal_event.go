package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type executionReceiptObserved struct {
	Receipt runmodel.WorkspaceExecutionIntentReceipt
	Command runevents.WorkspaceExecutionIntentCommand
}

// Known inability to preserve an original response must fail before a new
// effect. Replay preserves its original response and never backfills one.
func (m *Biz) requireFreshExecutionReceiptSink(ctx context.Context, c runevents.WorkspaceExecutionIntentCommand) error {
	reader, ok := m.runs.(repository.RunExecutionIntentReceiptReader)
	if !ok {
		return fmt.Errorf("%w: execution receipt complete reader unavailable", common.ErrConflict)
	}
	prior, err := reader.ReadWorkspaceExecutionIntentReceipt(ctx, c.WorkspaceID, c.RequestID)
	if err != nil {
		return err
	}
	if prior != nil {
		return fmt.Errorf("%w: original receipt exists outside the selected fresh source", common.ErrConflict)
	}
	return nil
}

func reduceExecutionReceiptObserved(current *runmodel.Run, fact executionReceiptObserved) (*runmodel.Run, bool, error) {
	r := fact.Receipt
	identity, err := runevents.WorkspaceExecutionIntentPayloadHash(fact.Command)
	if current == nil || err != nil || identity != r.PayloadHash || fact.Command.WorkspaceID != r.Admission.WorkspaceID || fact.Command.RequestID != r.Admission.RequestID || r.Validate() != nil || current.WorkspaceID != r.Admission.WorkspaceID || current.ID != r.Admission.RunID || runmodel.ValidateExecutionIntentReceipts(current) != nil {
		return nil, false, common.ErrConflict
	}
	for _, prior := range current.ExecutionIntentReceipts {
		if prior.Admission.RequestID == r.Admission.RequestID {
			if prior.PayloadHash != r.PayloadHash {
				return nil, false, common.ErrConflict
			}
			// An explicit restart replay can observe a later lifecycle fact.
			// It must never replace the immutable original response used by reads.
			return current, true, nil
		}
	}
	// Bind the proposed original response to the actual immutable acceptance
	// source, not merely to an arbitrary Run with the same Workspace scope.
	requested := replacementIntentFromCommand(fact.Command)
	if r.Admission.Reason == runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
		if current.ActivationRequestID != r.Admission.RequestID || !replacementRunMatchesIntent(current, requested) {
			return nil, false, common.ErrConflict
		}
	} else if !replacementIntentMatches(current.ReplacementIntent, replacementIntentForPredecessor(requested, current)) {
		return nil, false, common.ErrConflict
	}
	if len(current.ExecutionIntentReceipts) >= runmodel.WorkspaceExecutionIntentReceiptLimit {
		return nil, false, common.ErrConflict
	}
	next := *current
	next.ExecutionIntentReceipts = append(append([]runmodel.WorkspaceExecutionIntentReceipt(nil), current.ExecutionIntentReceipts...), r)
	return &next, false, nil
}

// This records a response actually selected by the Run owner. It never
// reconstructs one from a pending/active/terminal lifecycle snapshot. A crash
// before this CAS leaves unknown; read-only reconciliation cannot fill the cut.
func (m *Biz) recordExecutionIntentResponse(ctx context.Context, command runevents.WorkspaceExecutionIntentCommand, response runevents.WorkspaceExecutionIntentResult) error {
	identity, err := runevents.WorkspaceExecutionIntentPayloadHash(command)
	if err != nil || response.Validate() != nil || response.WorkspaceID != command.WorkspaceID || response.RequestID != command.RequestID || response.Kind != command.Kind || response.RunID == "" {
		return common.ErrConflict
	}
	return m.submitExecutionReceiptObserved(ctx, executionReceiptObserved{Command: command.Normalized(), Receipt: runmodel.WorkspaceExecutionIntentReceipt{
		Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: identity, RecordedAt: time.Now().UTC(), Admission: response.RuntimeExecutionIntentAdmission,
	}})
}

func (m *Biz) submitExecutionReceiptObserved(ctx context.Context, fact executionReceiptObserved) error {
	command := fact.Command
	identity, err := runevents.WorkspaceExecutionIntentPayloadHash(command)
	if err != nil || fact.Receipt.Validate() != nil || fact.Receipt.PayloadHash != identity || fact.Receipt.Admission.WorkspaceID != command.WorkspaceID || fact.Receipt.Admission.RequestID != command.RequestID {
		return common.ErrConflict
	}
	// The same request may later be represented by its replacement successor.
	// Look across the complete original inventory before selecting any sink.
	reader, ok := m.runs.(repository.RunExecutionIntentReceiptReader)
	if !ok {
		return fmt.Errorf("%w: execution receipt complete reader unavailable", common.ErrConflict)
	}
	prior, err := reader.ReadWorkspaceExecutionIntentReceipt(ctx, command.WorkspaceID, command.RequestID)
	if err != nil {
		return fmt.Errorf("execution receipt original lookup: %w", err)
	}
	if prior != nil {
		if prior.Validate() != nil || prior.Admission.WorkspaceID != command.WorkspaceID || prior.Admission.RequestID != command.RequestID || prior.PayloadHash != identity {
			return common.ErrConflict
		}
		return nil
	}
	for attempt := 0; attempt < 4; attempt++ {
		current, err := m.runs.GetByWorkspaceID(ctx, command.WorkspaceID, fact.Receipt.Admission.RunID)
		if err != nil {
			return err
		}
		next, noop, err := reduceExecutionReceiptObserved(current, fact)
		if err != nil || noop {
			if err != nil {
				return fmt.Errorf("execution receipt reduce: %w", err)
			}
			return nil
		}
		if err := m.runs.CompareAndSwap(ctx, next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return fmt.Errorf("execution receipt CAS: %w", err)
		}
		return nil
	}
	return common.ErrConflict
}
