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
	aggregate, err := c.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	candidate, err = runtimeTestDeriveCandidateAuthority(aggregate, candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
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
	aggregate, err := c.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	candidate, err = runtimeTestDeriveCandidateAuthority(aggregate, candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	result, err := c.AdmitAndSealCandidateAttemptForTest(ctx, workspaceID, runID, attempt, candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !result.Admission.Accepted() {
		return result.Aggregate, fmt.Errorf("%w: candidate admission rejected with %s: %s", common.ErrConflict, result.Admission.Failure.Code, result.Admission.Failure.Reason)
	}
	return result.Aggregate, nil
}

func runtimeTestDeriveCandidateAuthority(aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) (runtimecontract.RuntimeCandidate, error) {
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return runtimecontract.RuntimeCandidate{}, err
	}
	return runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
}

func runtimeTestMustDeriveCandidateAuthority(t *testing.T, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	t.Helper()
	derived, err := runtimeTestDeriveCandidateAuthority(aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	return derived
}

func runtimeTestMustDeriveCandidateForManifest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	t.Helper()
	derived, err := runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	return derived
}

func runtimeTestCandidateOperationID(t *testing.T, candidate runtimecontract.RuntimeCandidate, ordinal int) string {
	t.Helper()
	operations := candidateRuntimeOperations(candidate)
	if ordinal < 0 || ordinal >= len(operations) || operations[ordinal].ID == "" {
		t.Fatalf("fixture candidate operation ordinal %d is unavailable: %+v", ordinal, operations)
	}
	return operations[ordinal].ID
}

func runtimeTestDerivedOperationID(t *testing.T, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate, ordinal int) string {
	t.Helper()
	return runtimeTestCandidateOperationID(t, runtimeTestMustDeriveCandidateAuthority(t, aggregate, candidate), ordinal)
}

func runtimeTestActiveOperationID(t *testing.T, aggregate RuntimeAggregate, ordinal int) string {
	t.Helper()
	if aggregate.ActiveDispatch == nil {
		t.Fatal("fixture has no active dispatch")
	}
	return runtimeTestCandidateOperationID(t, aggregate.ActiveDispatch.Candidate, ordinal)
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
	candidate, err = runtimeTestDeriveCandidateAuthority(aggregate, candidate)
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

// runtimeTestCurrentCandidateFeasibilities projects the read-only feasible
// owner results that a direct seal fixture would already have persisted before
// entering SM-3. The projection is rooted in the current typed owner catalogs
// and the exact pre-seal candidate manifest; it never invents an owner,
// snapshot, acquisition binding or operation identity.
func runtimeTestCurrentCandidateFeasibilities(t *testing.T, aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) []runtimecontract.RuntimeCandidateFeasibilityRecord {
	t.Helper()
	candidate, err := runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatalf("derive candidate authority for fixture feasibility: %v", err)
	}
	ret := make([]runtimecontract.RuntimeCandidateFeasibilityRecord, 0)
	for _, operation := range candidateRuntimeOperations(candidate) {
		operation = operation.Normalized()
		descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !known || descriptor.OwnerModuleID == "" {
			t.Fatalf("fixture operation %q has no canonical owner", operation.Kind)
		}
		if !runtimeOperationAdmissibilityRequiredForOperation(
			aggregate.Facts.Prerequisites, descriptor.OwnerModuleID, operation.Kind,
		) {
			continue
		}
		inventory, found := runtimeOperationAdmissibilityForKind(
			aggregate.Facts.Resources.OperationAdmissibilities,
			descriptor.OwnerModuleID,
			operation.Kind,
		)
		if !found {
			t.Fatalf("fixture operation %q has no current persisted owner catalog", operation.ID)
		}
		ret = append(ret, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID,
			AcquisitionBinding: inventory.AcquisitionBinding, CandidateManifestHash: manifest.Hash,
			Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
		})
	}
	if err := runtimecontract.ValidateRuntimeCandidateFeasibilityRecords(ret); err != nil {
		t.Fatalf("fixture candidate feasibility projection: %v", err)
	}
	return ret
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
