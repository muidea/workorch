package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These test-only conveniences keep fixture setup concise without preserving a
// second production admission API. Production callers must consume the typed
// CandidateAdmissionCommitResult transitions directly.
func (c *RuntimeController) recordCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	result, err := c.AdmitCandidateProposalForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s: %s", common.ErrConflict, result.Admission.Failure.Code, result.Admission.Failure.Reason)
	}
	return result.Aggregate, nil
}

func (c *RuntimeController) AcceptCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	result, err := c.AdmitAndSealCandidateAttemptForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s: %s", common.ErrConflict, result.Admission.Failure.Code, result.Admission.Failure.Reason)
	}
	return result.Aggregate, nil
}

// RecordFixtureCandidateFeasibilities simulates only the already-completed
// read-only owner probes required by an explicit fixture inventory.  It is
// intentionally test-only: production obtains these facts from the owner via
// the typed feasibility command before SM-3 sealing.
func RecordFixtureCandidateFeasibilities(ctx context.Context, c *RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) error {
	if c == nil {
		return common.ErrInvalidArgument
	}
	aggregate, err := c.Load(ctx, workspaceID, runID)
	if err != nil {
		return err
	}
	requiresProbe := false
	for _, operation := range candidateRuntimeOperations(candidate) {
		descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if known && runtimeOperationAdmissibilityRequiredForOperation(aggregate.Facts.Prerequisites, descriptor.OwnerModuleID, operation.Kind) {
			requiresProbe = true
			break
		}
	}
	if !requiresProbe {
		return nil
	}
	if _, err := c.recordCandidateProposal(ctx, workspaceID, runID, attempt, candidate); err != nil {
		return err
	}
	aggregate, err = c.Load(ctx, workspaceID, runID)
	if err != nil {
		return err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	_, attemptRecord, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found {
		return fmt.Errorf("fixture candidate attempt %d is not pending", attempt)
	}
	for _, operation := range candidateRuntimeOperations(candidate) {
		operation = operation.Normalized()
		descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !known || !runtimeOperationAdmissibilityRequiredForOperation(aggregate.Facts.Prerequisites, descriptor.OwnerModuleID, operation.Kind) {
			continue
		}
		inventory, found := runtimeOperationAdmissibilityForKind(aggregate.Facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
		if !found {
			return fmt.Errorf("fixture operation %q has no matching owner admissibility inventory", operation.ID)
		}
		if _, err := c.recordCandidateFeasibility(ctx, workspaceID, runID, attempt, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			CandidateManifestHash: attemptRecord.ManifestHash, Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
		}); err != nil {
			return err
		}
	}
	return nil
}

func runtimeTestPendingCandidateManifestHash(t *testing.T, c *RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) string {
	t.Helper()
	aggregate, err := c.Load(context.Background(), workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	_, record, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found || record.ManifestHash == "" {
		t.Fatalf("fixture candidate attempt %d has no manifest identity", attempt)
	}
	return record.ManifestHash
}
