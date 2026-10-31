package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"strings"
	"time"
)

// All child budgets inherit this single request deadline. A reserved final
// observation phase is a sibling of initial reads, never their child. Freshness
// is still the original 30 seconds; these are execution limits, not leases.
const preparationTotalBudget = 45 * time.Second
const preparationInitialBudget = 12 * time.Second
const preparationFinalBudget = 10 * time.Second
const preparationSettlementBudget = 15 * time.Second

// Phase state is request-local and contains no observation or readiness cache.
type preparationObservationPhases struct {
	request     context.Context
	initial     context.Context
	stopInitial context.CancelFunc
}

func newPreparationObservationPhases(ctx context.Context) preparationObservationPhases {
	initial, stop := context.WithTimeout(ctx, preparationInitialBudget)
	return preparationObservationPhases{request: ctx, initial: initial, stopInitial: stop}
}
func (p preparationObservationPhases) beginFinal() (context.Context, context.CancelFunc) {
	p.stopInitial()
	return context.WithTimeout(p.request, preparationFinalBudget)
}

type preparationObservationError struct {
	stage    string
	cause    error
	deadline time.Time
}

func (e *preparationObservationError) Error() string { return e.cause.Error() }
func (e *preparationObservationError) Unwrap() error { return e.cause }
func preparationStageError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &preparationObservationError{stage: stage, cause: err}
}

func preparationStageContextError(ctx context.Context, stage string, err error) error {
	if err == nil {
		return nil
	}
	deadline, _ := ctx.Deadline()
	return &preparationObservationError{stage: stage, cause: err, deadline: deadline}
}

// Caller supplies complete=true only after validating the actual typed result.
// A complete known result is retained even if cancellation arrives afterwards.
func preparationDiagnostic(ctx context.Context, phase, owner, stage string, started time.Time, attempted, complete bool, err error) workspace.PreparationObservationDiagnostic {
	d := workspace.PreparationObservationDiagnostic{Phase: phase, Owner: owner, Stage: stage, StartedAt: started, CompletedAt: time.Now().UTC(), Attempted: attempted, Code: "incomplete"}
	d.Deadline, _ = ctx.Deadline()
	var staged *preparationObservationError
	if errors.As(err, &staged) {
		d.Stage = staged.stage
		for cause := err; cause != nil; cause = errors.Unwrap(cause) {
			if e, ok := cause.(*preparationObservationError); ok && !e.deadline.IsZero() {
				d.Deadline = e.deadline
				break
			}
		}
	}
	switch {
	case complete && err == nil:
		d.Code = "observed"
	case !attempted && errors.Is(err, context.DeadlineExceeded):
		d.Code = "budget_exhausted_before_call"
	case errors.Is(err, context.DeadlineExceeded):
		d.Code = "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		d.Code = "cancelled"
	case err != nil:
		d.Code = "owner_rejected"
		var coded *cd.Error
		if errors.As(err, &coded) && coded.Code == cd.Timeout {
			d.Code = "deadline_exceeded"
		}
	}
	if strings.Contains(d.Stage, "drift") {
		d.Code = "source_drift"
	}
	if strings.HasPrefix(d.Stage, "validate") || d.Stage == "result_type" {
		d.Code = "invalid_result"
	}
	if err != nil {
		raw := []byte(err.Error())
		hash := sha256.Sum256(raw)
		d.ErrorByteCount, d.ErrorSHA256 = len(raw), hex.EncodeToString(hash[:])
		if len(raw) > workspace.PreparationObservationErrorLimit {
			d.NotSavedReason = "error_exceeds_storage_limit"
		} else {
			d.ErrorBytes = raw
		}
	}
	return d
}
