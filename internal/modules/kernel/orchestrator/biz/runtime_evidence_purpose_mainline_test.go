package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
)

func runtimeAssertHistoricalEffectActualReadRemainsStale(t *testing.T, ctx context.Context, owner *Biz, aggregate runtime.RuntimeAggregate) {
	t.Helper()
	before := aggregate.CloneRaw()
	stale := 0
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if !runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) {
			continue
		}
		request, err := runtime.CompileRuntimeEvidenceApplicabilityRequest(aggregate, fact.ID)
		if err != nil || request.PurposeRef != runtimecontract.RuntimeHistoricalEffectProofV1 {
			t.Fatalf("historical request lost original source purpose: %v", err)
		}
		current, observed, err := owner.queryRuntimeFileMutationApplicabilityFromAggregate(ctx, aggregate, request.DispatchID, request.OperationID, request.SourceReceiptHash, request.RequestID)
		if err != nil || !observed || current == nil {
			t.Fatalf("actual File historical readback: observed=%v err=%v", observed, err)
		}
		observation := runtimecontract.RuntimeEvidenceApplicabilityObservation{Request: request, RecordedAtStateRevision: request.IssuedAtStateRevision + 1, CurrentDependencies: current}
		if err := runtimecontract.ValidateRuntimeEvidenceApplicabilityObservationSource(observation, aggregate.Facts, aggregate.OperationReceipts); err != nil {
			t.Fatalf("actual readback differs from immutable source: %v", err)
		}
		if runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, []runtimecontract.RuntimeEvidenceApplicabilityObservation{observation}, aggregate.Facts.EvidenceRevision) {
			stale++
			if runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, []runtimecontract.RuntimeEvidenceApplicabilityObservation{observation}, aggregate.Facts.EvidenceRevision, aggregate.Reconciliation.RequestHash) ||
				!runtimecontract.RuntimeEvidenceHasFreshProofApplicability(fact, []runtimecontract.RuntimeEvidenceApplicabilityObservation{observation}, aggregate.Facts.EvidenceRevision, aggregate.Reconciliation.RequestHash) {
				t.Fatal("actual obsolete postimage was revived or original behavior was lost")
			}
		}
	}
	if stale == 0 || !reflect.DeepEqual(before, aggregate) {
		t.Fatal("actual readback did not expose the old postimage or mutated terminal authority")
	}
	loaded, err := owner.loadRuntime(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || !reflect.DeepEqual(before, loaded) {
		t.Fatalf("read-only historical query changed receipt/effect/budget/material: %v", err)
	}
}

// Actual producer/material oracle: old File postimages must survive verbatim
// on formal items, but cannot supply a current resource union or fresh proof.
func runtimeAssertHistoricalEffectTerminalSeparation(t *testing.T, source runtimecontract.RuntimeStateRecord, closeout runtimecontract.RuntimeResultCloseout, currentBytes []byte) {
	t.Helper()
	digest := sha256.Sum256(currentBytes)
	currentHash := hex.EncodeToString(digest[:])
	effects, oldPostimages := 0, 0
	for _, fact := range source.Facts.Evidence.Accepted {
		if !runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) {
			continue
		}
		effects++
		if fact.OwnerRevisionSet == nil || fact.ApplicabilityDependencies == nil {
			t.Fatal("actual historical producer lost its original read/postimage pair")
		}
		for _, original := range fact.ApplicabilityDependencies.OwnerRevisionSet.Entries {
			if original.Revision != currentHash {
				oldPostimages++
			}
			for _, current := range closeout.OwnerRevisionSet.Entries {
				if current.OwnerKind == original.OwnerKind && current.OwnerInstanceID == original.OwnerInstanceID &&
					current.ResourceKind == original.ResourceKind && current.ResourceID == original.ResourceID {
					t.Fatal("historical File postimage entered the terminal current union")
				}
			}
		}
		preserved := false
		for _, outcome := range closeout.RequirementOutcomes {
			for _, item := range outcome.EvidenceItems {
				if item.EvidenceRef != string(fact.ID) {
					continue
				}
				if !reflect.DeepEqual(item.OwnerRevisionSet, fact.OwnerRevisionSet) || !reflect.DeepEqual(item.ApplicabilityDependencies, fact.ApplicabilityDependencies) {
					t.Fatal("formal evidence item rewrote historical reads or purpose/postimage")
				}
				preserved = true
			}
		}
		if !preserved {
			t.Fatal("formal material silently omitted an actual historical effect")
		}
	}
	if effects != 2 || oldPostimages == 0 {
		t.Fatalf("shared-target oracle lacks both real historical effects and an obsolete postimage: effects=%d old=%d", effects, oldPostimages)
	}
}

// Test oracle for the legitimate successor generation, not an alternative
// production selector/admission path. Every trigger must reverse-prove a real
// stale ordinary source and the original retired binding identity.
func runtimeTestProofReopenHasExactStaleSource(record runtimecontract.RuntimeStateRecord, coverageID common.CoverageWorkItemID, oldFocus string) bool {
	for _, item := range record.Facts.Coverage {
		if item.ID != coverageID {
			continue
		}
		for _, reopen := range item.ReopenLineage {
			if reopen.PreviousBindingRef.FocusID != oldFocus {
				continue
			}
			for _, observed := range record.Facts.EvidenceApplicabilityObservations {
				if observed.Request.RequestID != reopen.TriggerRef {
					continue
				}
				for _, fact := range record.Facts.Evidence.Accepted {
					if fact.ID != observed.Request.SourceEvidenceID || fact.Kind != runtimecontract.EvidenceKindValidation ||
						runtimecontract.StableRuntimeEvidenceFactHash(fact) != observed.Request.SourceEvidenceHash ||
						!containsRuntimeCoverageID(fact.CoverageWorkItemIDs, coverageID) || !containsRuntimeString(fact.BindingRefs, reopen.PreviousBindingRef.BindingID) {
						continue
					}
					if runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, []runtimecontract.RuntimeEvidenceApplicabilityObservation{observed}, observed.Request.EvidenceRevision) {
						return true
					}
				}
			}
		}
	}
	return false
}

func runtimeTestCurrentProofReopenHasFreshTarget(record runtimecontract.RuntimeStateRecord, coverageID common.CoverageWorkItemID, target string) bool {
	focus := record.Facts.Focus
	if focus.TargetSelectorRef != "" || !containsRuntimeCoverageID(focus.CoverageWorkItemIDs, coverageID) || !containsRuntimeString(focus.TargetRefs, target) {
		return false
	}
	for _, item := range record.Facts.Coverage {
		if item.ID != coverageID || len(item.ReopenLineage) == 0 {
			continue
		}
		last := item.ReopenLineage[len(item.ReopenLineage)-1]
		if !runtimeTestProofReopenHasExactStaleSource(record, coverageID, last.PreviousBindingRef.FocusID) {
			return false
		}
		// Retired selector identities can be consumed/reacquired by earlier
		// generations. Prove the exact original sealed target, not a selector
		// label surviving in a mutable prerequisite collection.
		originalTarget := false
		for _, dispatch := range record.CompletedDispatches {
			for _, binding := range dispatch.CoverageBindings {
				if binding.Validate() == nil && binding.ID == last.PreviousBindingRef.BindingID &&
					binding.Hash == last.PreviousBindingRef.BindingHash && binding.FocusID == last.PreviousBindingRef.FocusID &&
					binding.CoverageWorkItemID == coverageID && containsRuntimeString(binding.TargetRefs, target) {
					originalTarget = true
				}
			}
		}
		if !originalTarget {
			return false
		}
		for _, prerequisite := range record.Facts.Prerequisites.Satisfied {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.FocusID != focus.ID ||
				prerequisite.Scope != runtimecontract.RuntimePrerequisiteScopeFocus || prerequisite.SubjectRef != target {
				continue
			}
			for _, observed := range record.Facts.Resources.PrerequisiteObservations {
				if observed.PrerequisiteID != prerequisite.ID || observed.Kind != prerequisite.Kind || observed.OwnerModuleID != prerequisite.OwnerModuleID {
					continue
				}
				for _, snapshot := range record.Facts.Resources.TargetSnapshots {
					if snapshot.TargetRef == target && snapshot.Complete() && observed.ContentHash == snapshot.ContentHash {
						return true
					}
				}
			}
		}
	}
	return false
}
