package orchestrator

import (
	"context"
	"fmt"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Test-only fixture helpers. RuntimeOrchestrator exposes only typed admission
// transitions in production so a caller cannot turn an expected rejection into
// a string-classified drive error.
func (o *RuntimeOrchestrator) RecordRuntimeV2CandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.RuntimeAggregate, error) {
	result, err := o.AdmitRuntimeV2CandidateProposal(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}

func (o *RuntimeOrchestrator) AcceptRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.RuntimeAggregate, error) {
	result, err := o.AdmitAndSealRuntimeV2CandidateAttempt(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}
