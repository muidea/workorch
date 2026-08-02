package orchestrator

import (
	"context"
	"fmt"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Test-only fixture helpers. RuntimeOrchestrator exposes only typed admission
// transitions in production so a caller cannot turn an expected rejection into
// a string-classified drive error.
func (o *RuntimeOrchestrator) RecordRuntimeCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.RuntimeAggregate, error) {
	result, err := o.AdmitRuntimeCandidateProposalForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}

func (o *RuntimeOrchestrator) AcceptRuntimeCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.RuntimeAggregate, error) {
	result, err := o.AdmitAndSealRuntimeCandidateAttemptForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}

// Test-only facades construct the same complete session/manifest binding used
// by production before invoking the strict controller entry points. They are
// intentionally unavailable from non-test builds.
func (o *RuntimeOrchestrator) BeginRuntimeCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (runtime.RuntimeAggregate, error) {
	aggregate, err := o.LoadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	manifest, err := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimemodel.RuntimeCandidateManifestOwnerFactsFromResources((aggregate).Facts.Resources))
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	_, session, err := o.EnsureRuntimeCandidateRequestSession(ctx, workspaceID, runID, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if session.State != runtimemodel.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimemodel.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := o.MarkRuntimeCandidateRequestSubmitted(ctx, workspaceID, runID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		if _, err := o.CompleteRuntimeCandidateRequestDocument(ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
	}
	return o.BeginRuntimeCandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, session.LogicalRequestID)
}

func (o *RuntimeOrchestrator) AdmitRuntimeCandidateProposalForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := runtimeCandidateAttemptTestBinding(ctx, o, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	return o.AdmitRuntimeCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func (o *RuntimeOrchestrator) AdmitAndSealRuntimeCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := runtimeCandidateAttemptTestBinding(ctx, o, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	return o.AdmitAndSealRuntimeCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func runtimeCandidateAttemptTestBinding(ctx context.Context, o *RuntimeOrchestrator, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, error) {
	aggregate, err := o.LoadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return runtimemodel.RuntimeCandidateContractManifest{}, "", err
	}
	var fallback *runtimemodel.RuntimeCandidateAttemptRecord
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt {
			continue
		}
		if record.Transaction == aggregate.Decision.AuthorityTransaction() {
			manifest, manifestErr := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimemodel.RuntimeCandidateManifestOwnerFactsFromResources((aggregate).Facts.Resources))
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

func candidateTargetSelection(aggregate runtime.RuntimeAggregate) *runtimemodel.RuntimeTargetSelection {
	if aggregate.Facts.Resources.TargetSelection != nil {
		selection := aggregate.Facts.Resources.TargetSelection.Normalized()
		return &selection
	}
	return nil
}

func beginRuntimeCandidateAttemptForTest(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (runtime.RuntimeAggregate, error) {
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	manifest, err := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimemodel.RuntimeCandidateManifestOwnerFactsFromResources((aggregate).Facts.Resources))
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	_, session, err := controller.EnsureCandidateRequestSession(ctx, workspaceID, runID, runtimemodel.RuntimeProviderDocumentOperations, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if session.State != runtimemodel.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimemodel.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := controller.MarkCandidateRequestSubmitted(ctx, workspaceID, runID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		if _, err := controller.CompleteCandidateRequestDocument(ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
	}
	return controller.BeginCandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, session.LogicalRequestID)
}

func admitRuntimeCandidateProposalForTest(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := directRuntimeCandidateAttemptTestBinding(ctx, controller, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	return controller.AdmitCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func admitAndSealRuntimeCandidateAttemptForTest(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, err := directRuntimeCandidateAttemptTestBinding(ctx, controller, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	return controller.AdmitAndSealCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func directRuntimeCandidateAttemptTestBinding(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, error) {
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
			manifest, manifestErr := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimemodel.RuntimeCandidateManifestOwnerFactsFromResources((aggregate).Facts.Resources))
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

func evaluateRuntimeCandidateAdmissionForTest(aggregate runtime.RuntimeAggregate, candidate runtimemodel.RuntimeCandidate) runtime.CandidateAdmissionResult {
	manifest, err := runtimemodel.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimemodel.RuntimeCandidateManifestOwnerFactsFromResources((aggregate).Facts.Resources))
	if err != nil {
		return runtime.CandidateAdmissionResult{Disposition: runtime.CandidateAdmissionRejectedContract}
	}
	return runtime.EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
}
