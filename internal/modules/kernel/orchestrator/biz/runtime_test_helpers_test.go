package orchestrator

import (
	"context"
	"fmt"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Test-only fixture helpers. Biz exposes only typed admission
// transitions in production so a caller cannot turn an expected rejection into
// a string-classified drive error.
func (o *Biz) RecordRuntimeCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.RuntimeAggregate, error) {
	result, err := o.AdmitRuntimeCandidateProposalForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s", common.ErrConflict, result.Admission.Failure.Code)
	}
	return result.Aggregate, nil
}

func (o *Biz) AcceptRuntimeCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.RuntimeAggregate, error) {
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
func (o *Biz) BeginRuntimeCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (runtime.RuntimeAggregate, error) {
	aggregate, err := o.loadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
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
		return o.CompleteRuntimeCandidateRequestDocument(ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID)
	}
	return o.loadRuntime(ctx, workspaceID, runID)
}

func (o *Biz) AdmitRuntimeCandidateProposalForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, stablePrefixHash, err := runtimeCandidateAttemptTestBindingWithPrefix(ctx, o, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	candidate, err = runtimemodel.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	return o.AdmitRuntimeCandidateProposal(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, logicalRequestID, candidate)
}

func (o *Biz) AdmitAndSealRuntimeCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	rawCandidate := candidate
	manifest, logicalRequestID, stablePrefixHash, err := runtimeCandidateAttemptTestBindingWithPrefix(ctx, o, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	candidate, err = runtimemodel.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	aggregate, err := o.loadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimemodel.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	existing, found := runtimeCandidateAttemptRecordForTest(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	needsProposal := !found || (existing.State != runtimemodel.RuntimeCandidateAttemptStateAccepted &&
		existing.State != runtimemodel.RuntimeCandidateAttemptStateFailed &&
		(existing.State != runtimemodel.RuntimeCandidateAttemptStatePending || existing.Proposal == nil))
	if needsProposal {
		proposal, proposalErr := o.AdmitRuntimeCandidateProposal(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, logicalRequestID, candidate)
		if proposalErr != nil || !proposal.Admission.Accepted() {
			return proposal, proposalErr
		}
		manifest, logicalRequestID, _, err = runtimeCandidateAttemptTestBindingWithPrefix(ctx, o, workspaceID, runID, attempt)
		if err != nil {
			return runtime.CandidateAdmissionCommitResult{}, err
		}
		candidate = bindRuntimeTestCandidateToManifest(manifest, rawCandidate)
		candidate, err = runtimemodel.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			return runtime.CandidateAdmissionCommitResult{}, err
		}
	} else if existing.State == runtimemodel.RuntimeCandidateAttemptStatePending && existing.Proposal != nil {
		candidate = *existing.Proposal
	}
	return o.AdmitAndSealRuntimeCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func runtimeCandidateAttemptRecordForTest(records []runtimemodel.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string, attempt int) (runtimemodel.RuntimeCandidateAttemptRecord, bool) {
	for _, record := range records {
		if runtimemodel.CandidateAttemptAuthorityContractID(workspaceID, runID, record) == contractID && record.Attempt == attempt {
			return record, true
		}
	}
	return runtimemodel.RuntimeCandidateAttemptRecord{}, false
}

func pendingRuntimeCandidateAttemptForTest(records []runtimemodel.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string, attempt int) (int, runtimemodel.RuntimeCandidateAttemptRecord, bool) {
	for index, record := range records {
		if runtimemodel.CandidateAttemptAuthorityContractID(workspaceID, runID, record) == contractID && record.Attempt == attempt && record.State == runtimemodel.RuntimeCandidateAttemptStatePending {
			return index, record, true
		}
	}
	return -1, runtimemodel.RuntimeCandidateAttemptRecord{}, false
}

func runtimeCandidateAttemptTestBinding(ctx context.Context, o *Biz, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, error) {
	manifest, logicalRequestID, _, err := runtimeCandidateAttemptTestBindingWithPrefix(ctx, o, workspaceID, runID, attempt)
	return manifest, logicalRequestID, err
}

func runtimeCandidateAttemptTestBindingWithPrefix(ctx context.Context, o *Biz, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, string, error) {
	aggregate, err := o.loadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return runtimemodel.RuntimeCandidateContractManifest{}, "", "", err
	}
	return resolveRuntimeCandidateAttemptTestBinding(aggregate, attempt)
}

func resolveRuntimeCandidateAttemptTestBinding(aggregate runtime.RuntimeAggregate, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, string, error) {
	transaction := aggregate.Decision.AuthorityTransaction()
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.Transaction != transaction || record.State != runtimemodel.RuntimeCandidateAttemptStatePending {
			continue
		}
		manifest, manifestErr := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
		if manifestErr != nil {
			return runtimemodel.RuntimeCandidateContractManifest{Hash: record.ManifestHash}, record.LogicalRequestID, record.StablePrefixHash, nil
		}
		return manifest, record.LogicalRequestID, record.StablePrefixHash, nil
	}
	for i := len(aggregate.CandidateRequestSessions) - 1; i >= 0; i-- {
		session := aggregate.CandidateRequestSessions[i]
		if session.DocumentKind != runtimemodel.RuntimeProviderDocumentOperations ||
			session.Transaction != transaction ||
			session.State != runtimemodel.RuntimeCandidateRequestSessionDocumentComplete ||
			session.ExpectedDocument == nil ||
			session.ExpectedDocument.CorrectionAttempt != attempt {
			continue
		}
		manifest, manifestErr := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
		if manifestErr != nil {
			return runtimemodel.RuntimeCandidateContractManifest{Hash: session.ManifestHash}, session.LogicalRequestID, session.StablePrefixHash, nil
		}
		return manifest, session.LogicalRequestID, session.StablePrefixHash, nil
	}
	var fallback *runtimemodel.RuntimeCandidateAttemptRecord
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt {
			continue
		}
		if record.Transaction == transaction {
			manifest, manifestErr := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
			if manifestErr != nil {
				return runtimemodel.RuntimeCandidateContractManifest{Hash: record.ManifestHash}, record.LogicalRequestID, record.StablePrefixHash, nil
			}
			return manifest, record.LogicalRequestID, record.StablePrefixHash, nil
		}
		copyRecord := record
		fallback = &copyRecord
	}
	if fallback != nil {
		return runtimemodel.RuntimeCandidateContractManifest{Hash: fallback.ManifestHash}, fallback.LogicalRequestID, fallback.StablePrefixHash, nil
	}
	return runtimemodel.RuntimeCandidateContractManifest{}, "", "", fmt.Errorf("%w: candidate attempt %d has no test binding", common.ErrConflict, attempt)
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
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	_, session, err := runtimeTestControllerEnsureCandidateRequest(controller, ctx, workspaceID, runID, runtimemodel.RuntimeProviderDocumentOperations, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if session.State != runtimemodel.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimemodel.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := runtimeTestControllerMarkCandidateRequestSubmitted(controller, ctx, workspaceID, runID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		return runtimeTestControllerCompleteCandidateDocument(controller, ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID)
	}
	return controller.Load(ctx, workspaceID, runID)
}

func admitRuntimeCandidateProposalForTest(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	manifest, logicalRequestID, stablePrefixHash, err := directRuntimeCandidateAttemptTestBindingWithPrefix(ctx, controller, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	candidate, err = runtimemodel.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	return runtimeTestControllerAdmitCandidateProposal(controller, ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, logicalRequestID, candidate)
}

func admitAndSealRuntimeCandidateAttemptForTest(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	rawCandidate := candidate
	manifest, logicalRequestID, stablePrefixHash, err := directRuntimeCandidateAttemptTestBindingWithPrefix(ctx, controller, workspaceID, runID, attempt)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	candidate, err = runtimemodel.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtime.CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimemodel.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	existing, found := runtimeCandidateAttemptRecordForTest(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	needsProposal := !found || (existing.State != runtimemodel.RuntimeCandidateAttemptStateAccepted &&
		existing.State != runtimemodel.RuntimeCandidateAttemptStateFailed &&
		(existing.State != runtimemodel.RuntimeCandidateAttemptStatePending || existing.Proposal == nil))
	if needsProposal {
		proposal, proposalErr := runtimeTestControllerAdmitCandidateProposal(controller, ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, logicalRequestID, candidate)
		if proposalErr != nil || !proposal.Admission.Accepted() {
			return proposal, proposalErr
		}
		manifest, logicalRequestID, _, err = directRuntimeCandidateAttemptTestBindingWithPrefix(ctx, controller, workspaceID, runID, attempt)
		if err != nil {
			return runtime.CandidateAdmissionCommitResult{}, err
		}
		candidate = bindRuntimeTestCandidateToManifest(manifest, rawCandidate)
		candidate, err = runtimemodel.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			return runtime.CandidateAdmissionCommitResult{}, err
		}
	} else if existing.State == runtimemodel.RuntimeCandidateAttemptStatePending && existing.Proposal != nil {
		candidate = *existing.Proposal
	}
	return runtimeTestControllerAdmitAndSealCandidate(controller, ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func directRuntimeCandidateAttemptTestBinding(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, error) {
	manifest, logicalRequestID, _, err := directRuntimeCandidateAttemptTestBindingWithPrefix(ctx, controller, workspaceID, runID, attempt)
	return manifest, logicalRequestID, err
}

func directRuntimeCandidateAttemptTestBindingWithPrefix(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (runtimemodel.RuntimeCandidateContractManifest, string, string, error) {
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtimemodel.RuntimeCandidateContractManifest{}, "", "", err
	}
	return resolveRuntimeCandidateAttemptTestBinding(aggregate, attempt)
}

func evaluateRuntimeCandidateAdmissionForTest(aggregate runtime.RuntimeAggregate, candidate runtimemodel.RuntimeCandidate) runtime.CandidateAdmissionResult {
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return runtime.CandidateAdmissionResult{Disposition: runtime.CandidateAdmissionRejectedContract}
	}
	return runtime.EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
}
