package runtime

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func sameFreshnessStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func TestPrerequisiteStaleInvalidatesProposalAndReacquiresWithoutCandidateDebt(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialFreshnessHash := initialized.Decision.AuthoritySnapshot.PrerequisiteFreshnessHash
	if initialFreshnessHash == "" {
		t.Fatal("fresh observation did not contribute to authority transaction")
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	invalidated, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || invalidated.ActiveDispatch != nil ||
		len(invalidated.Facts.Prerequisites.Pending) != 1 || len(invalidated.Facts.Prerequisites.Satisfied) != 0 ||
		len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 || len(invalidated.Facts.Resources.PrerequisiteObservations) != 0 ||
		invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("stale did not atomically return to prerequisite acquisition: %+v", invalidated)
	}
	if len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].Failure != nil || invalidated.CandidateAttempts[0].Proposal != nil ||
		len(invalidated.Facts.Evidence.Accepted) != 0 || len(invalidated.Facts.Evidence.Rejected) != 0 || invalidated.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("stale consumed candidate, evidence, or repair authority: %+v", invalidated)
	}
	saves := repository.saves
	duplicate, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil || duplicate.StateRevision != invalidated.StateRevision || repository.saves != saves || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate stale was not idempotent: aggregate=%+v saves=%d err=%v", duplicate, repository.saves, err)
	}
	if _, err := controller.commitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &inventory, Observation: &observation,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale prerequisite result commit = %v, want conflict", err)
	}
	newInventory := inventory
	newInventory.ObservedRevision = "tree-v2"
	newInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(newInventory)
	newObservation := observation
	newObservation.ContentHash = newInventory.SnapshotID
	newObservation.Dependencies[0].Revision = "tree-v2"
	newObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(newObservation)
	reacquired, err := controller.commitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &newInventory, Observation: &newObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		reacquired.Decision.AuthoritySnapshot.PrerequisiteFreshnessHash == initialFreshnessHash {
		t.Fatalf("fresh acquisition did not seal a new authority transaction: %+v", reacquired.Decision)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "fresh-prefix"); err != nil {
		t.Fatalf("new authority did not reopen candidate attempt 0: %v", err)
	}
}

func TestPrerequisiteStaleInvalidatesPendingCandidateRequestWithoutCandidateDebt(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	prerequisite, observation := addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-1", "run-1", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	invalidated, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-1", "run-1", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].Proposal != nil || invalidated.CandidateAttempts[0].Failure != nil ||
		invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || len(invalidated.Facts.Evidence.Accepted) != 0 ||
		len(invalidated.Facts.Evidence.Rejected) != 0 || invalidated.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("pending candidate stale consumed candidate/repair/evidence authority: %+v", invalidated)
	}
}

// A File target snapshot is a separate owner observation from manifest and
// admissibility. Its read-only freshness loss must retire only that snapshot,
// preserve the other recovery ledgers, and make the same prerequisite ID
// pending for a new owner observation. The candidate attempt is invalidated
// audit lineage, not candidate/repair debt.
func TestTargetSnapshotReadOnlyStaleReacquiresOnlyFileSnapshot(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	facts := controllerFacts()
	facts.Focus.RequireTargetGrounding = true
	requirement := normalizeRuntimeRequirements([]runtimecontract.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "target.txt", Reason: "grounded target requires owner snapshot",
	}})[0]
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, OwnerModuleID: requirement.OwnerModuleID,
		SubjectRef: requirement.SubjectRef, Reason: requirement.Reason,
	}
	snapshot := runtimecontract.TargetSnapshot{
		TargetRef: "target.txt", Content: "before\n", ContentHash: "target-v1",
		ContentState: runtimecontract.SnapshotContentStateComplete,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{snapshot}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}

	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(initialized.Facts.Prerequisites.Pending) != 0 {
		t.Fatalf("fresh target snapshot did not satisfy grounding prerequisite: %+v", initialized)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "target-snapshot-stale-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "target.txt", ExpectedRevision: "target-v1", ObservedRevision: "target-v2",
		Reason: "File owner observed a newer target revision",
	}
	invalidated, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(invalidated.Facts.Resources.TargetSnapshots) != 0 || len(invalidated.Facts.Resources.PrerequisiteObservations) != 0 ||
		len(invalidated.Facts.Prerequisites.Satisfied) != 0 || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
		invalidated.Facts.Prerequisites.Pending[0].ID != prerequisite.ID ||
		invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		invalidated.Facts.Repair.RepairAttempts != 0 || invalidated.Facts.Recovery.ProviderTransportAttempts != 0 || invalidated.ActiveDispatch != nil {
		t.Fatalf("target snapshot stale did not isolate prerequisite freshness: %+v", invalidated)
	}
	if len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].Proposal != nil || invalidated.CandidateAttempts[0].Failure != nil {
		t.Fatalf("target snapshot stale did not retain only invalidated candidate lineage: %+v", invalidated.CandidateAttempts)
	}

	saves := repository.saves
	duplicate, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil || duplicate.StateRevision != invalidated.StateRevision || repository.saves != saves ||
		duplicate.Facts.Recovery != invalidated.Facts.Recovery {
		t.Fatalf("duplicate target snapshot stale was not a no-op: aggregate=%+v saves=%d err=%v", duplicate, repository.saves, err)
	}
	if _, err := controller.commitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale target snapshot was accepted for re-acquisition: %v", err)
	}

	freshSnapshot := snapshot
	freshSnapshot.Content = "after\n"
	freshSnapshot.ContentHash = "target-v2"
	freshObservation := observation
	freshObservation.ContentHash = freshSnapshot.ContentHash
	freshObservation.Dependencies[0].Revision = freshSnapshot.ContentHash
	freshObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(freshObservation)
	reacquired, err := controller.commitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, TargetSnapshot: &freshSnapshot, Observation: &freshObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(reacquired.Facts.Resources.TargetSnapshots) != 1 ||
		reacquired.Facts.Resources.TargetSnapshots[0].ContentHash != "target-v2" || len(reacquired.Facts.Resources.PrerequisiteObservations) != 1 ||
		reacquired.Facts.Resources.PrerequisiteObservations[0].ObservationID != freshObservation.ObservationID ||
		reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("fresh target snapshot did not replace stale owner fact: %+v", reacquired)
	}
}

func TestFeasibilityPrerequisiteInvalidatesAttemptWithoutRetryOrRepairDebt(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, TargetRefs: []string{"target.txt"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	admissibility := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: admissibility.ID, Kind: admissibility.Kind, OwnerModuleID: admissibility.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{admissibility}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	gap := runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, Reason: "validation build failed"}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 1, ActiveGap: &gap}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("initial decision = %+v", initialized.Decision)
	}
	targetRef, patchType, oldText, newText := "target.txt", "text_replace", "old", "new"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "replace-target", Kind: runtimecontract.RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{targetRef},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	request := runtimecontract.RuntimeOperationPrerequisiteRequest{OperationID: "replace-target", Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "target.txt", Reason: "content-bound file operation requires a current owner target snapshot"}
	failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_replay_proof_unavailable", Signature: "file-preview-content-conflict", Message: "current target content cannot prove the proposed patch", TargetRefs: []string{"target.txt"}}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, runtimecontract.RuntimeFeedbackDomainFeasibility, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent), runtimecontract.RuntimeModuleFile, failure)
	if err != nil {
		t.Fatal(err)
	}
	ownerObservation, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromOutcome(runtimecontract.RuntimeOwnerFeasibilityPrerequisiteRequired, &feedback, nil, &request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordCandidateFeasibility(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: "replace-target", AdmissibilitySnapshotID: inventory.SnapshotID, Observation: &ownerObservation,
	}); err != nil {
		t.Fatal(err)
	}
	invalidated, err := controller.invalidateCandidateAttemptForFeasibilityPrerequisite(ctx, "ws-v2", "run-v2", 0, request)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(invalidated.Facts.Prerequisites.Pending) != 1 || invalidated.Facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindTargetSnapshot {
		t.Fatalf("feasibility prerequisite did not enter acquire state: %+v", invalidated)
	}
	if invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || invalidated.CandidateAttempts[0].PrerequisiteRequest == nil || invalidated.Facts.Repair.RepairAttempts != 1 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("feasibility prerequisite consumed retry, repair, or stale budget: %+v", invalidated)
	}
	beforeAuthority := initialized.Decision.AuthoritySnapshot.ID
	prerequisite := invalidated.Facts.Prerequisites.Pending[0]
	snapshot := runtimecontract.TargetSnapshot{TargetRef: "target.txt", Content: "package current", ContentHash: "target-v2", ContentState: runtimecontract.SnapshotContentStateComplete}
	snapshotObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash}},
	}
	snapshotObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(snapshotObservation)
	resumed, err := controller.commitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &snapshotObservation})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || resumed.Decision.AuthoritySnapshot.ID == beforeAuthority || len(resumed.Facts.Resources.TargetSnapshots) != 1 || resumed.Facts.Repair.RepairAttempts != 1 {
		t.Fatalf("snapshot commit did not seal new state-change authority while retaining repair cycle: %+v", resumed)
	}
	saves := repository.saves
	if duplicate, err := controller.invalidateCandidateAttemptForFeasibilityPrerequisite(ctx, "ws-v2", "run-v2", 0, request); err != nil || duplicate.StateRevision != resumed.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate feasibility prerequisite was not idempotent: %v", err)
	}
}

// An ordinary owner infeasibility is neither a candidate correction nor a
// prerequisite refresh. It must leave an immutable invalidated-attempt
// lineage, re-reduce SM-2 with the factual failure, and make a fresh attempt
// 0 possible only under the resulting authority.
func TestOwnerInfeasibilityInvalidatesAttemptAndFormsNewAuthorityWithoutCorrection(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeAuthority := initialized.Decision.AuthoritySnapshot.ID
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-target", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, facts.WorkspaceID, facts.RunID, 0, "owner-infeasible-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordCandidateProposal(ctx, facts.WorkspaceID, facts.RunID, 0, candidate); err != nil {
		t.Fatal(err)
	}
	failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "file/target-not-regular", Message: "owner rejected non-regular target", TargetRefs: []string{"target.txt"}}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: beforeAuthority,
	}, runtimecontract.RuntimeFeedbackDomainFeasibility, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular), runtimecontract.RuntimeModuleFile, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = "write-target"
	feedback.OriginalDiagnostic.TargetRefs = []string{"target.txt", "target.txt", "secondary.txt"}
	feedback.OriginalDiagnostic.ExpectedRevision = "workspace-v1"
	feedback.OriginalDiagnostic.ObservedRevision = "workspace-v2"
	feedback.OriginalDiagnostic.FieldViolations = []runtimecontract.RuntimeFieldViolation{
		{Path: "operations[0].targetRefs", Actual: "target.txt", Allowed: []string{"secondary.txt", "target.txt", "target.txt"}},
		{Path: "operations[0].parameters.mode", Actual: "replace", Required: []string{"create", "replace"}, Forbidden: []string{"delete", "delete"}},
	}
	feedback.OriginalDiagnostic.DetailsRef = "workorch://owner/file/feasibility-1"
	feedback.OriginalDiagnostic.DetailsHash = "sha256:diagnostic"
	feedback.OriginalDiagnostic.DetailsBytes = 257
	feedback.OriginalDiagnostic.ContentType = "application/json"
	feedback.OriginalDiagnostic.Truncated = true
	feedback.OriginalDiagnostic.Redacted = true
	expectedDiagnostic := feedback.OriginalDiagnostic.CloneOriginal()
	ownerObservation := runtimecontract.RuntimeOwnerFeasibilityObservation{
		Outcome: runtimecontract.RuntimeOwnerFeasibilityInfeasible, Feedback: &feedback,
	}
	if _, err := controller.recordCandidateFeasibility(ctx, facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: "write-target", AdmissibilitySnapshotID: inventory.SnapshotID, Observation: &ownerObservation,
	}); err != nil {
		t.Fatal(err)
	}
	ownerEvent, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventOwnerFeasibilityObserved, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, Attempt: 0, OwnerFeasibility: &ownerObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidated := ownerEvent.Aggregate
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || invalidated.Decision.AuthoritySnapshot.ID == beforeAuthority ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].OwnerFeasibility == nil || invalidated.CandidateAttempts[0].Failure != nil ||
		invalidated.Facts.Failure == nil || invalidated.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible || invalidated.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("owner infeasibility did not form a clean new authority: %+v", invalidated)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, facts.WorkspaceID, facts.RunID, 1, "owner-infeasible-prefix"); err == nil {
		t.Fatal("owner infeasibility incorrectly opened correction attempt 1")
	}
	// Recreate the controller over the same persisted aggregate. No process-local
	// attempt map may be needed to distinguish the old invalidated contract from
	// this newly sealed authority.
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	reloadedDiagnostic := reloaded.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic
	if !reflect.DeepEqual(reloadedDiagnostic, expectedDiagnostic) {
		t.Fatalf("owner diagnostic changed across persisted reload:\n got=%+v\nwant=%+v", reloadedDiagnostic, expectedDiagnostic)
	}
	if _, err := restarted.BeginCandidateAttemptForTest(ctx, facts.WorkspaceID, facts.RunID, 0, "owner-infeasible-new-authority"); err != nil {
		t.Fatalf("new authority did not permit fresh attempt 0: %v", err)
	}
	saves := repository.saves
	duplicateEvent, err := restarted.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventOwnerFeasibilityObserved, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, Attempt: 0, OwnerFeasibility: &ownerObservation,
	})
	if err != nil || duplicateEvent.Aggregate.StateRevision != repository.aggregate.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate owner observation rewrote aggregate: duplicate=%+v saves=%d err=%v", duplicateEvent.Aggregate, repository.saves, err)
	}
}

// An undeclared owner code is a capability contract defect, not a factual
// infeasibility that may trigger a fresh authority or another provider call.
// The original code/message remains in the persisted owner feedback while the
// SM-2/SM-3 owner-local handoff terminates it with zero dispatch/effect work.
func TestOwnerContractDefectFailsFastWithoutCorrectionOrEffect(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-target", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, "owner-contract-defect-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordCandidateProposal(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, candidate); err != nil {
		t.Fatal(err)
	}
	original := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateContractFailure, Code: "file_new_unmapped_code",
		Signature: "file/owner-contract-defect", Message: "file owner emitted an unregistered feasibility code", TargetRefs: []string{"target.txt"},
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID, AuthoritySnapshotID: initialized.Decision.AuthoritySnapshot.ID,
	}, runtimecontract.RuntimeFeedbackDomainFeasibility, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect), runtimecontract.RuntimeModuleFile, original)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = "write-target"
	ownerObservation, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromOutcome(runtimecontract.RuntimeOwnerFeasibilityContractDefect, &feedback, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordCandidateFeasibility(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: "write-target", AdmissibilitySnapshotID: "inventory-file", Observation: &ownerObservation,
	}); err != nil {
		t.Fatal(err)
	}
	committed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventOwnerFeasibilityObserved, WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID, Attempt: 0, OwnerFeasibility: &ownerObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate := committed.Aggregate
	if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.ActiveDispatch != nil ||
		len(aggregate.CandidateAttempts) != 1 || aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		aggregate.CandidateAttempts[0].OwnerFeasibility == nil || aggregate.CandidateAttempts[0].OwnerFeasibility.Outcome != runtimecontract.RuntimeOwnerFeasibilityContractDefect ||
		aggregate.CandidateAttempts[0].OwnerFeasibility.Feedback == nil || aggregate.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic == nil ||
		aggregate.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalCode != original.Code ||
		aggregate.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalMessage != original.Message ||
		aggregate.Facts.Failure == nil || aggregate.Facts.Failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment || aggregate.Facts.Failure.Code != "runtime_owner_contract_defect" ||
		len(aggregate.OperationReceipts) != 0 || len(aggregate.Facts.Evidence.Accepted) != 0 || len(aggregate.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("owner contract defect did not fail fast with retained original diagnostic: %+v", aggregate)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, 1, "owner-contract-defect-prefix"); err == nil {
		t.Fatal("owner contract defect opened a correction attempt")
	}
	saves := repository.saves
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventOwnerFeasibilityObserved, WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID, Attempt: 0, OwnerFeasibility: &ownerObservation,
	})
	if err != nil || duplicate.Aggregate.StateRevision != aggregate.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate owner contract defect rewrote aggregate: aggregate=%+v saves=%d err=%v", duplicate.Aggregate, repository.saves, err)
	}
}

func TestMCPOwnerCatalogStaleRestartDoesNotReuseFailureTarget(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "mcp-catalog-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall}}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-catalog-v1", TargetRefs: []string{"implementation.go"}}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-failure", Message: "observed failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "mcp-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.AuthoritySnapshot.PrerequisiteFreshnessHash == "" {
		t.Fatal("MCP owner-catalog observation was not sealed into authority")
	}
	// The failure above is fixture input for the catalog-freshness path, not an
	// already-persisted repair continuation. It predates the semantic focus
	// projection, so discard the placeholder-focus directive before the helper
	// seals the canonical binding and derives a current-focus directive.
	repository.aggregate.Facts.RepairDirective = nil
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID, Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", ExpectedRevision: "mcp-catalog-v1", ObservedRevision: "mcp-catalog-v2", Reason: "mcp catalog changed"}
	invalidated, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != invalidated.StateRevision || loaded.Facts.Resources.FailureTarget != nil || loaded.Facts.Failure == nil || len(loaded.Facts.Failure.TargetRefs) != 0 ||
		len(loaded.Facts.Prerequisites.Pending) != 1 || loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID || loaded.Facts.RepairDirective != nil ||
		!sameRuntimeTargetRefs(GroundedRepairTargets(loaded.Facts), []string{"target.txt"}) ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
		t.Fatalf("restart retained stale MCP failure authority: loaded=%+v invalidated=%+v err=%v", loaded, invalidated, err)
	}
}

func TestActiveDispatchPrerequisiteStaleInvalidatesWithoutRejectedEvidence(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	prerequisite, observation := addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	execution := &repository.aggregate.ActiveDispatch.OperationExecutions[0]
	execution.Fence = 1
	execution.State = runtimecontract.OperationExecutionOwnerCommitted
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		PrerequisiteStale: &stale,
	}
	receipt.Feedback = runtimeOperationFeedbackForTest(t, repository.aggregate, execution.OperationID, runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "prerequisite_stale", Signature: "stale-file-tree", Message: "workspace changed",
	})
	receipt = receipt.Normalized()
	execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	repository.aggregate.OperationReceipts = append(repository.aggregate.OperationReceipts, receipt)
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	invalidated, err := controller.invalidateActiveDispatchForPrerequisiteStale(ctx, "ws-1", "run-1", sealed.ActiveDispatch.ID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.ActiveDispatch != nil || len(invalidated.CompletedDispatches) != 1 ||
		invalidated.CompletedDispatches[0].State != DispatchStateInvalidated ||
		invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(invalidated.Facts.Evidence.Accepted) != 0 || len(invalidated.Facts.Evidence.Rejected) != 0 ||
		invalidated.Facts.Repair.RepairAttempts != 0 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("generic stale did not return to acquisition cleanly: %+v", invalidated)
	}
	if len(invalidated.Facts.Prerequisites.Pending) != 1 || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 {
		t.Fatalf("stale did not clear fresh prerequisite facts: %+v", invalidated.Facts)
	}
	stateRevision := invalidated.StateRevision
	duplicate, err := controller.invalidateActiveDispatchForPrerequisiteStale(ctx, "ws-1", "run-1", sealed.ActiveDispatch.ID, receipt)
	if err != nil || duplicate.StateRevision != stateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate active stale changed authority: aggregate=%+v err=%v", duplicate, err)
	}
}

func TestAcceptedReceiptResourceChangeRetiresPrerequisitesButKeepsRemainingDispatch(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	_, _ = addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: patchApplyTestOperation("write-1", "main.go")},
		{Operation: func() runtimecontract.RuntimeOperation {
			operation := patchApplyTestOperation("write-2", "other.go")
			// Only write-1 is bound to the main.go CWI. The second write exists to
			// exercise continuation after invalidation, not to claim coverage.
			operation.BindingRefs = nil
			operation.DependsOn = []string{"write-1"}
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-freshness-resource-change", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	receipt := evidence.Receipt.Clone()
	receipt.ResourceChanges = []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
	}}
	receipt = receipt.Normalized()
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	for idx := range repository.aggregate.OperationReceipts {
		if repository.aggregate.OperationReceipts[idx].ExecutionKey == receipt.ExecutionKey && repository.aggregate.OperationReceipts[idx].Fence == receipt.Fence {
			repository.aggregate.OperationReceipts[idx] = receipt
		}
	}
	for idx := range repository.aggregate.ActiveDispatch.OperationExecutions {
		if repository.aggregate.ActiveDispatch.OperationExecutions[idx].OperationID == receipt.OperationID {
			repository.aggregate.ActiveDispatch.OperationExecutions[idx].OwnerReceiptHash = hash
		}
	}
	evidence.Receipt = &receipt
	evidence.OwnerReceiptHash = hash
	committed, err := coordinator.commitEvidence(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.DispatchCompleted || committed.Aggregate.ActiveDispatch == nil || len(committed.Aggregate.CompletedDispatches) != 0 ||
		committed.Aggregate.ActiveDispatch.ID != sealed.ActiveDispatch.ID ||
		len(committed.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 || committed.Aggregate.ActiveDispatch.CompletedOperationIDs[0] != "write-1" ||
		len(committed.Aggregate.Facts.Evidence.Accepted) != 1 || len(committed.Aggregate.Facts.Evidence.Rejected) != 0 ||
		committed.Aggregate.Facts.Repair.RepairAttempts != 0 || committed.Aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(committed.Aggregate.Facts.FreshnessReacquireCoverage) != 0 {
		t.Fatalf("accepted resource change did not preserve sealed dispatch: %+v", committed.Aggregate)
	}
	if len(committed.Aggregate.Facts.Prerequisites.Pending) != 1 || len(committed.Aggregate.Facts.Resources.OperationAdmissibilities) != 0 ||
		len(committed.Aggregate.PrerequisiteInvalidations) != 1 {
		t.Fatalf("resource change did not invalidate intersecting observation: %+v", committed.Aggregate)
	}
	if changes := committed.Aggregate.PrerequisiteInvalidations[0].ResourceChanges; len(changes) != 1 ||
		changes[0].Domain != runtimecontract.RuntimeResourceWorkspaceTree || changes[0].AfterRevision != "tree-v2" {
		t.Fatalf("invalidation did not retain owner resource-change source: %#v", changes)
	}
	secondEvidence := ExecutionEvidence{
		EventID: "event-freshness-resource-change-second", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-2",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &secondEvidence)
	completed, err := coordinator.commitEvidence(ctx, secondEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.DispatchCompleted || completed.Aggregate.ActiveDispatch != nil || len(completed.Aggregate.CompletedDispatches) != 1 ||
		completed.Aggregate.CompletedDispatches[0].State != DispatchStateCommitted ||
		completed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(completed.Aggregate.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("completed resource-changing dispatch did not defer SM-2 until refresh: %+v", completed.Aggregate)
	}
	duplicate, err := coordinator.commitEvidence(ctx, evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("duplicate resource-change evidence changed refresh budget: aggregate=%+v err=%v", duplicate.Aggregate, err)
	}
}

func TestAcceptedReceiptResourceChangesAdvanceInvalidationWithoutRecoveryBudget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	prerequisite, _ := addFreshFileAdmissibilityObservation(t, &aggregate)
	for revision := 2; revision <= 10; revision++ {
		changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-"+strconv.Itoa(revision), "write-"+strconv.Itoa(revision), runtimecontract.RuntimeOperationReceipt{
			ResourceChanges: []runtimecontract.RuntimeResourceChange{{
				Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v" + strconv.Itoa(revision-1), AfterRevision: "tree-v" + strconv.Itoa(revision),
			}},
		})
		if err != nil || !changed {
			t.Fatalf("resource-change invalidation #%d = changed:%v err:%v", revision, changed, err)
		}
		if revision < 10 {
			// Re-acquisition restores the owner fact before the next durable
			// operation changes that same resource again.
			addFreshFileAdmissibilityObservation(t, &aggregate)
		}
	}
	if aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 || aggregate.Facts.Failure != nil ||
		runtimecontract.RuntimePrerequisiteInvalidationGeneration(aggregate.PrerequisiteInvalidations, prerequisite.ID) != 9 {
		t.Fatalf("resource changes must be progress, not recovery exhaustion: facts=%+v invalidations=%+v", aggregate.Facts, aggregate.PrerequisiteInvalidations)
	}
}

// Receipt-driven invalidation is normal progress, while loss of a local
// prerequisite remains bounded stale/no-progress recovery.
func TestMixedOwnerPrerequisiteRefreshLedgersRemainIndependent(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	filePrerequisite, _ := addFreshFileAdmissibilityObservation(t, &aggregate)
	scratchPrerequisite, scratchObservation := addFreshValidationScratchObservation(t, &aggregate, "scratch-v1")
	aggregate.Facts = aggregate.Facts.Normalized()
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = facts
	aggregate.Decision = decision

	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-file", "write-main", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws-1/run-1/dispatch-file/write-main", Fence: 1, OperationID: "write-main",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
		}},
	})
	if err != nil || !changed {
		t.Fatalf("external resource-change invalidation = changed:%t err:%v", changed, err)
	}
	if aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		len(aggregate.Facts.Prerequisites.Pending) != 1 || aggregate.Facts.Prerequisites.Pending[0].ID != filePrerequisite.ID ||
		len(aggregate.Facts.Prerequisites.Satisfied) != 1 || aggregate.Facts.Prerequisites.Satisfied[0].ID != scratchPrerequisite.ID ||
		aggregate.Facts.Resources.ValidationScratch == nil || len(aggregate.Facts.Resources.PrerequisiteObservations) != 1 ||
		aggregate.Facts.Resources.PrerequisiteObservations[0].PrerequisiteID != scratchPrerequisite.ID {
		t.Fatalf("external receipt did not retire only File owner facts: %+v", aggregate.Facts)
	}
	commitFreshnessInvalidationForTest(t, &aggregate)

	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: scratchPrerequisite.ID, ObservationID: scratchObservation.ObservationID,
		Domain: runtimecontract.RuntimeResourceLocalResource, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		SubjectRef: "validation-scratch/ws-1/run-1/cwi-1", ExpectedRevision: "scratch-v1", ObservedRevision: "scratch-v2", Reason: "local scratch disappeared",
	}
	invalidated, err := controller.invalidatePrerequisitesForReadOnlyStale(ctx, "ws-1", "run-1", stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		len(invalidated.Facts.Prerequisites.Pending) != 2 || len(invalidated.Facts.Prerequisites.Satisfied) != 0 ||
		invalidated.Facts.Resources.ValidationScratch != nil || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
		len(invalidated.CandidateRequestSessions) != 0 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
		len(invalidated.GovernanceRequests) != 0 || len(invalidated.OperationReceipts) != 0 {
		t.Fatalf("local stale contaminated the external receipt path: %+v", invalidated)
	}
	if !sameFreshnessStrings([]string{invalidated.Facts.Prerequisites.Pending[0].ID, invalidated.Facts.Prerequisites.Pending[1].ID}, []string{filePrerequisite.ID, scratchPrerequisite.ID}) {
		t.Fatalf("mixed owner pending prerequisites = %+v", invalidated.Facts.Prerequisites.Pending)
	}
	revision := invalidated.StateRevision
	duplicate, err := controller.invalidatePrerequisitesForReadOnlyStale(ctx, "ws-1", "run-1", stale)
	if err != nil || duplicate.StateRevision != revision || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate local stale retried mixed-owner refresh: aggregate=%+v err=%v", duplicate, err)
	}

	// An unrelated external change is not an excuse to consume either ledger or
	// to ask either owner again.
	unchanged, err := applyReceiptResourceChangeInvalidation(&duplicate, "dispatch-file", "write-other", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws-1/run-1/dispatch-file/write-other", Fence: 1, OperationID: "write-other",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "unrelated.go", BeforeRevision: "v1", AfterRevision: "v2",
		}},
	})
	if err != nil || unchanged || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("unrelated external change mutated mixed-owner ledger: changed=%t aggregate=%+v err=%v", unchanged, duplicate, err)
	}
}

func TestRuntimeAggregateCurrentCandidateWorkIgnoresRetiredAuthorityLineage(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	current := aggregate.Decision.AuthorityTransaction()
	historical := current
	historical.AuthoritySnapshotID = "authority-historical"
	aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{{
		LogicalRequestID: "historical-request", Transaction: historical,
		State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete,
	}}
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: "historical-contract", Transaction: historical, Attempt: 0,
		State: runtimecontract.RuntimeCandidateAttemptStateAccepted,
	}}
	if runtimeAggregateHasCurrentCandidateWork(aggregate) {
		t.Fatalf("historical candidate lineage was treated as current work: %+v", aggregate)
	}
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: "current-request", Transaction: current,
		State: runtimecontract.RuntimeCandidateRequestSessionPrepared,
	})
	if !runtimeAggregateHasCurrentCandidateWork(aggregate) {
		t.Fatalf("current candidate session was not fenced: %+v", aggregate)
	}
	aggregate.CandidateRequestSessions[1].State = runtimecontract.RuntimeCandidateRequestSessionTerminal
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: "current-contract", Transaction: current, Attempt: 0,
		State: runtimecontract.RuntimeCandidateAttemptStatePending,
	})
	if !runtimeAggregateHasCurrentCandidateWork(aggregate) {
		t.Fatalf("current pending candidate attempt was not fenced: %+v", aggregate)
	}
}

func addFreshValidationScratchObservation(t *testing.T, aggregate *RuntimeAggregate, revision string) (runtimecontract.RuntimePrerequisite, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "validation-scratch:orchestrator", Kind: runtimecontract.PrerequisiteKindValidationScratch,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
	}
	scratch := &runtimecontract.ValidationScratch{Reference: ".workorch-internal/validation-scratch/ws-1/run-1/cwi-1", Revision: revision}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: revision,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceLocalResource, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
			SubjectRef: "validation-scratch/ws-1/run-1/cwi-1", Revision: revision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate.Facts.Resources.ValidationScratch = scratch
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, observation)
	return prerequisite, observation
}

func commitFreshnessInvalidationForTest(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("mixed-owner freshness fixture invalid: %v", err)
	}
}

func TestFileTargetChangeInvalidatesShellValidationObservation(t *testing.T) {
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1",
	}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	shellInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:shell", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.StepKindShellCommand}, ObservedRevision: "shell-policy-v1",
	}
	shellInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellInventory)
	filePrerequisite := runtimecontract.RuntimePrerequisite{ID: fileInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}}
	shellPrerequisite := runtimecontract.RuntimePrerequisite{ID: shellInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}}
	fileObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: filePrerequisite.ID, Kind: filePrerequisite.Kind, OwnerModuleID: filePrerequisite.OwnerModuleID, ContentHash: fileInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	fileObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(fileObservation)
	shellObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: shellPrerequisite.ID, Kind: shellPrerequisite.Kind, OwnerModuleID: shellPrerequisite.OwnerModuleID, ContentHash: shellInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: "shell-policy-v1"},
			{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "main.go", Revision: "missing"},
		},
	}
	shellObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(shellObservation)
	facts := runtimecontract.RuntimeFacts{
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{filePrerequisite, shellPrerequisite}},
		Resources: runtimecontract.RuntimeResourceFacts{
			OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{fileInventory, shellInventory},
			PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{fileObservation, shellObservation},
		},
	}
	// Shell observes only its policy and the selected File-owned validation
	// target. A workspace-tree change elsewhere, or a Git repository-state
	// change, must not retire Shell's inventory merely because both facts belong
	// to the same workspace.
	for _, test := range []struct {
		change             runtimecontract.RuntimeResourceChange
		wantFileRetirement bool
	}{
		{change: runtimecontract.RuntimeResourceChange{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2"}, wantFileRetirement: true},
		{change: runtimecontract.RuntimeResourceChange{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", BeforeRevision: "repository-v1", AfterRevision: "repository-v2"}},
	} {
		updated, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(facts, []runtimecontract.RuntimeResourceChange{test.change})
		if test.wantFileRetirement {
			if err != nil || len(updated.Resources.OperationAdmissibilities) != 1 || updated.Resources.OperationAdmissibilities[0].PrerequisiteID != shellPrerequisite.ID ||
				len(updated.Resources.PrerequisiteObservations) != 1 || updated.Resources.PrerequisiteObservations[0].PrerequisiteID != shellPrerequisite.ID ||
				len(updated.Prerequisites.Satisfied) != 1 || updated.Prerequisites.Satisfied[0].ID != shellPrerequisite.ID ||
				len(prerequisiteIDs) != 1 || prerequisiteIDs[0] != filePrerequisite.ID || len(observationIDs) != 1 || observationIDs[0] != fileObservation.ObservationID ||
				len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != filePrerequisite.ID {
				t.Fatalf("workspace tree change did not retire only File inventory: facts=%+v prerequisites=%v observations=%v err=%v", updated, prerequisiteIDs, observationIDs, err)
			}
		} else if err != nil || len(prerequisiteIDs) != 0 || len(observationIDs) != 0 || len(updated.Prerequisites.Pending) != 0 ||
			len(updated.Prerequisites.Satisfied) != 2 || len(updated.Resources.OperationAdmissibilities) != 2 || len(updated.Resources.PrerequisiteObservations) != 2 {
			t.Fatalf("Git change unexpectedly retired an admissibility inventory: facts=%+v prerequisites=%v observations=%v err=%v", updated, prerequisiteIDs, observationIDs, err)
		}
	}
	updated, prerequisites, observations, _, err := invalidatePrerequisiteObservationsForChanges(facts, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "main.go", AfterRevision: "main-v1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisites) != 1 || prerequisites[0] != shellPrerequisite.ID || len(observations) != 1 || observations[0] != shellObservation.ObservationID {
		t.Fatalf("File target change affected wrong prerequisites/observations: %v / %v", prerequisites, observations)
	}
	if len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != shellPrerequisite.ID ||
		len(updated.Prerequisites.Satisfied) != 1 || updated.Prerequisites.Satisfied[0].ID != filePrerequisite.ID ||
		len(updated.Resources.OperationAdmissibilities) != 1 || updated.Resources.OperationAdmissibilities[0].PrerequisiteID != filePrerequisite.ID {
		t.Fatalf("File target change did not precisely invalidate Shell observation: %+v", updated)
	}

	// The same exact intersection must preserve the separate resource-change
	// ledger when the File owner's accepted receipt changes the validation
	// target. This is forward progress, not a read-only stale refresh.
	aggregate := RuntimeAggregate{Facts: facts}
	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-file", "write-target", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch-file/write-target", Fence: 1, OperationID: "write-target",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "main.go", BeforeRevision: "missing", AfterRevision: "main-v1",
		}},
	})
	if err != nil || !changed || aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		len(aggregate.Facts.Prerequisites.Pending) != 1 || aggregate.Facts.Prerequisites.Pending[0].ID != shellPrerequisite.ID ||
		len(aggregate.Facts.Prerequisites.Satisfied) != 1 || aggregate.Facts.Prerequisites.Satisfied[0].ID != filePrerequisite.ID ||
		len(aggregate.Facts.Resources.OperationAdmissibilities) != 1 || aggregate.Facts.Resources.OperationAdmissibilities[0].PrerequisiteID != filePrerequisite.ID ||
		len(aggregate.PrerequisiteInvalidations) != 1 || aggregate.PrerequisiteInvalidations[0].DispatchID != "dispatch-file" || aggregate.PrerequisiteInvalidations[0].OperationID != "write-target" {
		t.Fatalf("accepted File receipt did not retire only Shell validation inventory: aggregate=%+v changed=%t err=%v", aggregate, changed, err)
	}
	duplicate, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-file", "write-target", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch-file/write-target", Fence: 1, OperationID: "write-target",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "main.go", BeforeRevision: "missing", AfterRevision: "main-v1",
		}},
	})
	if err != nil || duplicate || aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 || len(aggregate.PrerequisiteInvalidations) != 1 {
		t.Fatalf("duplicate File resource change consumed Shell refresh again: aggregate=%+v changed=%t err=%v", aggregate, duplicate, err)
	}
}

func TestGitAdmissibilityReadOnlyStaleRetiresOnlyGitInventory(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	gitInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:git", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		OperationKinds: []string{runtimecontract.StepKindGitWrite}, ObservedRevision: "repository-v1",
		AcquisitionBinding: "capability-state/ws-v2/run-v2/git/0",
	}
	gitInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(gitInventory)
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "target-v1",
	}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	gitPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: gitInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite},
	}
	filePrerequisite := runtimecontract.RuntimePrerequisite{
		ID: fileInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	gitObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: gitPrerequisite.ID, Kind: gitPrerequisite.Kind, OwnerModuleID: gitPrerequisite.OwnerModuleID,
		ContentHash: gitInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit,
			SubjectRef: "workspace_deliverable", Revision: gitInventory.ObservedRevision,
		}},
	}
	gitObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(gitObservation)
	fileObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: filePrerequisite.ID, Kind: filePrerequisite.Kind, OwnerModuleID: filePrerequisite.OwnerModuleID,
		ContentHash: fileInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "target.txt", Revision: fileInventory.ObservedRevision,
		}},
	}
	fileObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(fileObservation)
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{filePrerequisite, gitPrerequisite}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileInventory, gitInventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{fileObservation, gitObservation}
	if _, err := controller.initialize(ctx, facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "git-admissibility-stale-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: gitPrerequisite.ID, ObservationID: gitObservation.ObservationID,
		Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "repository-v1", ObservedRevision: "repository-v2",
		Reason: "Git owner observed a newer repository state",
	}
	invalidated, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(invalidated.Facts.Resources.OperationAdmissibilities) != 1 || invalidated.Facts.Resources.OperationAdmissibilities[0].PrerequisiteID != filePrerequisite.ID ||
		len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 || invalidated.Facts.Resources.PrerequisiteObservations[0].PrerequisiteID != filePrerequisite.ID ||
		len(invalidated.Facts.Prerequisites.Pending) != 1 || invalidated.Facts.Prerequisites.Pending[0].ID != gitPrerequisite.ID ||
		len(invalidated.Facts.Prerequisites.Satisfied) != 1 || invalidated.Facts.Prerequisites.Satisfied[0].ID != filePrerequisite.ID ||
		invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.ActiveDispatch != nil || len(invalidated.GovernanceRequests) != 0 {
		t.Fatalf("Git read-only stale did not isolate Git inventory: %+v", invalidated)
	}
	saves := repository.saves
	duplicate, err := controller.invalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil || duplicate.StateRevision != invalidated.StateRevision || repository.saves != saves || duplicate.Facts.Recovery != invalidated.Facts.Recovery {
		t.Fatalf("duplicate Git stale was not a no-op: aggregate=%+v saves=%d err=%v", duplicate, repository.saves, err)
	}
	if _, err := controller.commitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: gitPrerequisite.ID, OperationAdmissibility: &gitInventory, Observation: &gitObservation,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale Git inventory was accepted for re-acquisition: %v", err)
	}
}

func TestFileTargetChangeDoesNotInvalidateGitAdmissibilityObservation(t *testing.T) {
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"}, ObservedRevision: "target-v1",
	}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	gitInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:git", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		OperationKinds: []string{runtimecontract.StepKindGitWrite}, ObservedRevision: "repository-v1", AcquisitionBinding: "capability-state/ws/run/git/0",
	}
	gitInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(gitInventory)
	filePrerequisite := runtimecontract.RuntimePrerequisite{ID: fileInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}}
	gitPrerequisite := runtimecontract.RuntimePrerequisite{ID: gitInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}}
	fileObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: filePrerequisite.ID, Kind: filePrerequisite.Kind, OwnerModuleID: filePrerequisite.OwnerModuleID, ContentHash: fileInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "main.go", Revision: "target-v1"}},
	}
	fileObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(fileObservation)
	gitObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: gitPrerequisite.ID, Kind: gitPrerequisite.Kind, OwnerModuleID: gitPrerequisite.OwnerModuleID, ContentHash: gitInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", Revision: "repository-v1"},
			{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"},
		},
	}
	gitObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(gitObservation)
	facts := runtimecontract.RuntimeFacts{
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{filePrerequisite, gitPrerequisite}},
		Resources: runtimecontract.RuntimeResourceFacts{
			OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{fileInventory, gitInventory},
			PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{fileObservation, gitObservation},
		},
	}
	updated, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(facts, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "main.go", BeforeRevision: "target-v1", AfterRevision: "target-v2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisiteIDs) != 1 || prerequisiteIDs[0] != filePrerequisite.ID || len(observationIDs) != 1 || observationIDs[0] != fileObservation.ObservationID ||
		len(updated.Resources.OperationAdmissibilities) != 1 || updated.Resources.OperationAdmissibilities[0].PrerequisiteID != gitPrerequisite.ID ||
		len(updated.Resources.PrerequisiteObservations) != 1 || updated.Resources.PrerequisiteObservations[0].PrerequisiteID != gitPrerequisite.ID ||
		len(updated.Prerequisites.Satisfied) != 1 || updated.Prerequisites.Satisfied[0].ID != gitPrerequisite.ID ||
		len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != filePrerequisite.ID {
		t.Fatalf("File target change overwrote Git admissibility observation: %+v", updated)
	}
}

// TestThreeOwnerAdmissibilityStaleIntersection keeps the A-3M merge rule
// explicit: SM-2 retires observations by exact dependency identity, never by
// owner name, inventory order, or a workspace-wide string match. Git's bound
// capability-state observation is deliberately included because it shares the
// same repository-state dependency as the Git inventory.
func TestThreeOwnerAdmissibilityStaleIntersection(t *testing.T) {
	filePrerequisite := runtimecontract.RuntimePrerequisite{ID: "operation_admissibility:file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}}
	gitStatePrerequisite := runtimecontract.RuntimePrerequisite{ID: "capability_state:git", Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable"}
	gitPrerequisite := runtimecontract.RuntimePrerequisite{ID: "operation_admissibility:git", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}}
	shellPrerequisite := runtimecontract.RuntimePrerequisite{ID: "operation_admissibility:shell", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}}
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: filePrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"source.go"}, ObservedRevision: "source-v1"}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	gitState := runtimecontract.RuntimeCapabilityStateSnapshot{RequirementID: gitStatePrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", SnapshotID: "git-state-v1", AcquisitionKey: "capability-state/ws/run/git/0", AcquisitionBinding: "capability-state/ws/run/git/0", ObservedRevision: "repository-v1"}
	gitInventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: gitPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleGit, OperationKinds: []string{runtimecontract.StepKindGitWrite}, ObservedRevision: "repository-v1", AcquisitionBinding: gitState.AcquisitionBinding}
	gitInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(gitInventory)
	shellInventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: shellPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKinds: []string{runtimecontract.StepKindShellCommand}, ObservedRevision: "shell-policy-v1"}
	shellInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellInventory)
	fileObservation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: filePrerequisite.ID, Kind: filePrerequisite.Kind, OwnerModuleID: filePrerequisite.OwnerModuleID, ContentHash: fileInventory.SnapshotID, Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "source.go", Revision: "source-v1"}}}
	fileObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(fileObservation)
	gitStateObservation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: gitStatePrerequisite.ID, Kind: gitStatePrerequisite.Kind, OwnerModuleID: gitStatePrerequisite.OwnerModuleID, ContentHash: "git-state-v1", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", Revision: "repository-v1"}}}
	gitStateObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(gitStateObservation)
	gitObservation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: gitPrerequisite.ID, Kind: gitPrerequisite.Kind, OwnerModuleID: gitPrerequisite.OwnerModuleID, ContentHash: gitInventory.SnapshotID, Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", Revision: "repository-v1"}}}
	gitObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(gitObservation)
	shellObservation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: shellPrerequisite.ID, Kind: shellPrerequisite.Kind, OwnerModuleID: shellPrerequisite.OwnerModuleID, ContentHash: shellInventory.SnapshotID, Dependencies: []runtimecontract.RuntimeFreshnessDependency{
		{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: "shell-policy-v1"},
		{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "validation.go", Revision: "validation-v1"},
	}}
	shellObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(shellObservation)
	base := runtimecontract.RuntimeFacts{Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{filePrerequisite, gitStatePrerequisite, gitPrerequisite, shellPrerequisite}}, Resources: runtimecontract.RuntimeResourceFacts{
		CapabilityStates:         []runtimecontract.RuntimeCapabilityStateSnapshot{gitState},
		OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{fileInventory, gitInventory, shellInventory},
		PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{fileObservation, gitStateObservation, gitObservation, shellObservation},
	}}

	unchanged, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(base, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "unrelated.go", BeforeRevision: "other-v1", AfterRevision: "other-v2",
	}})
	if err != nil || len(prerequisiteIDs) != 0 || len(observationIDs) != 0 || len(unchanged.Prerequisites.Satisfied) != 4 ||
		len(unchanged.Resources.OperationAdmissibilities) != 3 || len(unchanged.Resources.CapabilityStates) != 1 {
		t.Fatalf("unrelated three-owner change was not a no-op: facts=%+v prerequisites=%v observations=%v err=%v", unchanged, prerequisiteIDs, observationIDs, err)
	}

	for _, test := range []struct {
		name              string
		stale             runtimecontract.RuntimePrerequisiteStale
		wantPending       []string
		wantInventories   []string
		wantCapabilityGit bool
	}{
		{name: "file_target", stale: runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: filePrerequisite.ID, ObservationID: fileObservation.ObservationID, Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "source.go", ExpectedRevision: "source-v1", ObservedRevision: "source-v2", Reason: "File target changed"}, wantPending: []string{filePrerequisite.ID}, wantInventories: []string{gitPrerequisite.ID, shellPrerequisite.ID}, wantCapabilityGit: true},
		{name: "git_repository", stale: runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: gitPrerequisite.ID, ObservationID: gitObservation.ObservationID, Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: runtimecontract.RuntimeModuleGit, SubjectRef: "workspace_deliverable", ExpectedRevision: "repository-v1", ObservedRevision: "repository-v2", Reason: "Git repository changed"}, wantPending: []string{gitStatePrerequisite.ID, gitPrerequisite.ID}, wantInventories: []string{filePrerequisite.ID, shellPrerequisite.ID}, wantCapabilityGit: false},
		{name: "shell_policy", stale: runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: shellPrerequisite.ID, ObservationID: shellObservation.ObservationID, Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", ExpectedRevision: "shell-policy-v1", ObservedRevision: "shell-policy-v2", Reason: "Shell policy changed"}, wantPending: []string{shellPrerequisite.ID}, wantInventories: []string{filePrerequisite.ID, gitPrerequisite.ID}, wantCapabilityGit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, pending, _, _, err := invalidatePrerequisiteObservationsForStale(base, test.stale)
			if err != nil || !sameFreshnessStrings(pending, test.wantPending) || len(updated.Resources.OperationAdmissibilities) != len(test.wantInventories) ||
				(len(updated.Resources.CapabilityStates) == 1) != test.wantCapabilityGit {
				t.Fatalf("three-owner stale intersection mismatch: updated=%+v pending=%v err=%v", updated, pending, err)
			}
			for _, want := range test.wantInventories {
				found := false
				for _, inventory := range updated.Resources.OperationAdmissibilities {
					found = found || inventory.PrerequisiteID == want
				}
				if !found {
					t.Fatalf("three-owner stale removed unrelated inventory %q: %+v", want, updated.Resources.OperationAdmissibilities)
				}
			}
		})
	}
}

func TestOwnerCatalogChangeInvalidatesBoundFailureTargetObservation(t *testing.T) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "mcp-server-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	}
	prerequisiteObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "mcp-server-v1",
		}},
	}
	prerequisiteObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(prerequisiteObservation)
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
	}
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-failure", Message: "observed failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation,
	}
	facts := runtimecontract.RuntimeFacts{
		Failure: failure,
		RepairDirective: &runtimecontract.RuntimeRepairDirective{
			ID: "repair", FocusID: "focus", Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation,
			RequiredState: runtimecontract.ConvergenceStateNeedStateChange, CurrentFailure: failure, TargetRefs: []string{"implementation.go"},
		},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{prerequisite}},
		Resources: runtimecontract.RuntimeResourceFacts{
			FailureTarget:            &runtimecontract.FailureTarget{Signature: "mcp-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation},
			OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{inventory},
			PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{prerequisiteObservation},
		},
	}
	updated, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(facts, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", BeforeRevision: "mcp-server-v1", AfterRevision: "mcp-server-v2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisiteIDs) != 1 || prerequisiteIDs[0] != prerequisite.ID || len(observationIDs) != 1 || observationIDs[0] != prerequisiteObservation.ObservationID ||
		updated.Resources.FailureTarget != nil || updated.Failure == nil || len(updated.Failure.TargetRefs) != 0 || updated.Failure.TargetObservation != nil || updated.RepairDirective != nil {
		t.Fatalf("stale owner observation was retained: facts=%+v prerequisites=%v observations=%v", updated, prerequisiteIDs, observationIDs)
	}
	if len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != prerequisite.ID || len(updated.Resources.OperationAdmissibilities) != 0 {
		t.Fatalf("owner catalog prerequisite was not reacquired: %+v", updated)
	}
	duplicate, duplicatePrerequisites, duplicateObservations, _, err := invalidatePrerequisiteObservationsForChanges(updated, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", BeforeRevision: "mcp-server-v1", AfterRevision: "mcp-server-v2",
	}})
	if err != nil || len(duplicatePrerequisites) != 0 || len(duplicateObservations) != 0 || len(duplicate.Prerequisites.Pending) != 1 ||
		duplicate.Resources.FailureTarget != nil || duplicate.RepairDirective != nil {
		t.Fatalf("duplicate owner catalog invalidation was not idempotent: facts=%+v prerequisites=%v observations=%v err=%v", duplicate, duplicatePrerequisites, duplicateObservations, err)
	}
}

func TestWorkspaceTreeChangeInvalidatesOwnerTargetSelectionOnce(t *testing.T) {
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "target_selection:blocks/file:deliverable:file/unique_workspace_entry:::0", Kind: runtimecontract.PrerequisiteKindTargetSelection,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "deliverable", TargetSelectorRef: "file/unique_workspace_entry",
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/unique_workspace_entry",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1, ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts := runtimecontract.RuntimeFacts{
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{prerequisite}},
		Resources:     runtimecontract.RuntimeResourceFacts{TargetSelection: &selection, PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{observation}},
	}
	changes := []runtimecontract.RuntimeResourceChange{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2"}}
	updated, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(facts, changes)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Resources.TargetSelection != nil || len(updated.Prerequisites.Satisfied) != 0 || len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != prerequisite.ID || len(prerequisiteIDs) != 1 || prerequisiteIDs[0] != prerequisite.ID || len(observationIDs) != 1 || observationIDs[0] != observation.ObservationID {
		t.Fatalf("target selection was not precisely invalidated: updated=%+v prerequisites=%v observations=%v", updated, prerequisiteIDs, observationIDs)
	}
	duplicate, duplicatePrerequisites, duplicateObservations, _, err := invalidatePrerequisiteObservationsForChanges(updated, changes)
	if err != nil || duplicate.Resources.TargetSelection != nil || len(duplicatePrerequisites) != 0 || len(duplicateObservations) != 0 || len(duplicate.Prerequisites.Pending) != 1 {
		t.Fatalf("duplicate target selection invalidation was not idempotent: facts=%+v prerequisites=%v observations=%v err=%v", duplicate, duplicatePrerequisites, duplicateObservations, err)
	}
}

func addFreshFileAdmissibilityObservation(t *testing.T, aggregate *RuntimeAggregate) (runtimecontract.RuntimePrerequisite, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	aggregate.Facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go", "other.go"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	return prerequisite, observation
}
