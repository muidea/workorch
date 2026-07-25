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
	result, err := o.AdmitRuntimeV2CandidateProposalForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}

func (o *RuntimeOrchestrator) AcceptRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.RuntimeAggregate, error) {
	result, err := o.AdmitAndSealRuntimeV2CandidateAttemptForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}

// Test-only facades construct the same complete session/manifest binding used
// by production before invoking the strict controller entry points. They are
// intentionally unavailable from non-test builds.
func (o *RuntimeOrchestrator) BeginRuntimeV2CandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
	aggregate, err := o.LoadRuntimeV2(ctx, workspaceID, runID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	manifest, err := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, candidateTargetSelection(aggregate))
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	_, session, err := o.EnsureRuntimeV2CandidateRequestSession(ctx, workspaceID, runID, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if session.State != runtimemodel.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimemodel.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := o.MarkRuntimeV2CandidateRequestSubmitted(ctx, workspaceID, runID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		if _, err := o.CompleteRuntimeV2CandidateRequestDocument(ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
	}
	return o.BeginRuntimeV2CandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, session.LogicalRequestID)
}

func (o *RuntimeOrchestrator) AdmitRuntimeV2CandidateProposalForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := runtimeV2CandidateAttemptTestBinding(ctx, o, workspaceID, runID, attempt)
	if err != nil {
		return runtimev2.CandidateAdmissionCommitResult{}, err
	}
	return o.AdmitRuntimeV2CandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func (o *RuntimeOrchestrator) AdmitAndSealRuntimeV2CandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := runtimeV2CandidateAttemptTestBinding(ctx, o, workspaceID, runID, attempt)
	if err != nil {
		return runtimev2.CandidateAdmissionCommitResult{}, err
	}
	return o.AdmitAndSealRuntimeV2CandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func runtimeV2CandidateAttemptTestBinding(ctx context.Context, o *RuntimeOrchestrator, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, error) {
	aggregate, err := o.LoadRuntimeV2(ctx, workspaceID, runID)
	if err != nil {
		return runtimemodel.RuntimeCandidateContractManifest{}, "", err
	}
	var fallback *runtimemodel.RuntimeCandidateAttemptRecord
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt {
			continue
		}
		if record.Transaction == aggregate.Decision.AuthorityTransaction() {
			manifest, manifestErr := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, candidateTargetSelection(aggregate))
			if manifestErr != nil {
				return runtimemodel.RuntimeCandidateContractManifest{Hash: record.ManifestHash}, record.LogicalRequestID, nil
			}
			return manifest, record.LogicalRequestID, nil
		}
		copyRecord := record
		fallback = &copyRecord
	}
	if fallback != nil {
		return runtimemodel.RuntimeCandidateContractManifest{Hash: fallback.ManifestHash}, fallback.LogicalRequestID, nil
	}
	return runtimemodel.RuntimeCandidateContractManifest{}, "", fmt.Errorf("%w: candidate attempt %d has no test binding", common.ErrConflict, attempt)
}

func candidateTargetSelection(aggregate runtimev2.RuntimeAggregate) *runtimemodel.RuntimeTargetSelection {
	if aggregate.Facts.Resources.TargetSelection != nil {
		selection := aggregate.Facts.Resources.TargetSelection.Normalized()
		return &selection
	}
	return nil
}

func beginRuntimeV2CandidateAttemptForTest(ctx context.Context, controller *runtimev2.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	manifest, err := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, candidateTargetSelection(aggregate))
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	_, session, err := controller.EnsureCandidateRequestSession(ctx, workspaceID, runID, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if session.State != runtimemodel.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimemodel.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := controller.MarkCandidateRequestSubmitted(ctx, workspaceID, runID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		if _, err := controller.CompleteCandidateRequestDocument(ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
	}
	return controller.BeginCandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, session.LogicalRequestID)
}

func admitRuntimeV2CandidateProposalForTest(ctx context.Context, controller *runtimev2.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := directRuntimeV2CandidateAttemptTestBinding(ctx, controller, workspaceID, runID, attempt)
	if err != nil {
		return runtimev2.CandidateAdmissionCommitResult{}, err
	}
	return controller.AdmitCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func admitAndSealRuntimeV2CandidateAttemptForTest(ctx context.Context, controller *runtimev2.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := directRuntimeV2CandidateAttemptTestBinding(ctx, controller, workspaceID, runID, attempt)
	if err != nil {
		return runtimev2.CandidateAdmissionCommitResult{}, err
	}
	return controller.AdmitAndSealCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func directRuntimeV2CandidateAttemptTestBinding(ctx context.Context, controller *runtimev2.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, error) {
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtimemodel.RuntimeCandidateContractManifest{}, "", err
	}
	var fallback *runtimemodel.RuntimeCandidateAttemptRecord
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt {
			continue
		}
		if record.Transaction == aggregate.Decision.AuthorityTransaction() {
			manifest, manifestErr := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, candidateTargetSelection(aggregate))
			if manifestErr != nil {
				return runtimemodel.RuntimeCandidateContractManifest{Hash: record.ManifestHash}, record.LogicalRequestID, nil
			}
			return manifest, record.LogicalRequestID, nil
		}
		copyRecord := record
		fallback = &copyRecord
	}
	if fallback != nil {
		return runtimemodel.RuntimeCandidateContractManifest{Hash: fallback.ManifestHash}, fallback.LogicalRequestID, nil
	}
	return runtimemodel.RuntimeCandidateContractManifest{}, "", fmt.Errorf("%w: candidate attempt %d has no test binding", common.ErrConflict, attempt)
}

func evaluateRuntimeV2CandidateAdmissionForTest(aggregate runtimev2.RuntimeAggregate, candidate runtimemodel.RuntimeCandidate) runtimev2.CandidateAdmissionResult {
	manifest, err := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, candidateTargetSelection(aggregate))
	if err != nil {
		return runtimev2.CandidateAdmissionResult{Disposition: runtimev2.CandidateAdmissionRejectedContract}
	}
	return runtimev2.EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
}
