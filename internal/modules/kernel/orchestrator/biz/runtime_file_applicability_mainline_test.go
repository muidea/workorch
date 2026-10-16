package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Real File execution/query, receipt adapter and SM-2 controller. The bounded
// catalog/semantic pre-seal input, Workspace directory and CAS store are explicit
// fixtures. No provider or acceptance oracle is substituted for the actual bytes.
func TestRuntimeFileMutationApplicabilityPairsActualReceiptAndSM2(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	directory := t.TempDir()
	beforeBytes, afterBytes := "old\r\n原文\n", "new\r\n原文\n"
	path := filepath.Join(directory, "target.txt")
	if err := os.WriteFile(path, []byte(beforeBytes), 0600); err != nil {
		t.Fatal(err)
	}
	workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workspaceevents.GetWorkspaceCommand)
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID, DeliverableDirectory: directory}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet); err != nil {
			t.Error(err)
		}
	})
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})
	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts = runtimeCandidateSealExactFileEditFacts(t, "target.txt")
	digest := sha256.Sum256([]byte(beforeBytes))
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{TargetRef: "target.txt", Content: beforeBytes, ContentHash: hex.EncodeToString(digest[:]), ContentState: runtimecontract.SnapshotContentStateComplete}}
	repository := &runtimeReceiptSourceGuardRepository{aggregate: aggregate}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	owner.runtimeController = controller
	receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	for _, topic := range []string{orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt} {
		if err := receipts.Subscribe(topic, owner.handleRuntimeOperationReceipt); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := receipts.Unsubscribe(topic); err != nil {
				t.Error(err)
			}
		})
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, aggregate)
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "applicability-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &beforeBytes}, {Name: "newText", String: &afterBytes}}}}}}}
	proposal, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("proposal: %v %+v", err, proposal.Admission)
	}
	operation := proposal.Aggregate.CandidateAttempts[len(proposal.Aggregate.CandidateAttempts)-1].Proposal.StateChange.Operations[0].Operation
	catalog := aggregate.Facts.Resources.OperationAdmissibilities[0]
	if _, err := runtimeTestControllerRecordCandidateFeasibility(controller, ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: operation.ID, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding, CandidateManifestHash: runtimePendingCandidateManifestHashForBizTest(t, ctx, controller, "ws-v2", "run-v2", 0), Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}}); err != nil {
		t.Fatal(err)
	}
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() {
		t.Fatalf("seal: %v %+v", err, sealed.Admission)
	}
	aggregate, err = runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregate.OperationReceipts) != 1 || len(aggregate.Facts.Evidence.Accepted) != 1 || aggregate.ActiveDispatch != nil {
		t.Fatal("original real receipt transaction incomplete")
	}
	fact := aggregate.Facts.Evidence.Accepted[0]
	if fact.ApplicabilityDependencies == nil || aggregate.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatal("unobserved postcondition was used as current proof")
	}
	readDigest, postDigest := sha256.Sum256([]byte(beforeBytes)), sha256.Sum256([]byte(afterBytes))
	if fact.OwnerRevisionSet.Entries[0].Revision != hex.EncodeToString(readDigest[:]) || fact.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision != hex.EncodeToString(postDigest[:]) {
		t.Fatal("File lost original input or actual postcondition bytes")
	}
	original := aggregate.CloneRaw()
	request, required, err := runtime.NextRuntimeEvidenceApplicabilityRequest(aggregate)
	if err != nil || !required || request.SourceEvidenceID != fact.ID {
		t.Fatalf("exact current query obligation: required=%v err=%v", required, err)
	}
	updated, changed, err := owner.driveRuntimeEvidenceApplicability(ctx, aggregate)
	if err != nil || !changed {
		t.Fatalf("real owner query/SM-2 handoff: %v", err)
	}
	if len(updated.Facts.EvidenceApplicabilityObservations) != 1 || !reflect.DeepEqual(updated.Facts.EvidenceApplicabilityObservations[0].CurrentDependencies, &fact.ApplicabilityDependencies.OwnerRevisionSet) || !reflect.DeepEqual(updated.OperationReceipts, original.OperationReceipts) || !reflect.DeepEqual(updated.Facts.Evidence, original.Facts.Evidence) || !reflect.DeepEqual(updated.ExecutionBudget, original.ExecutionBudget) {
		t.Fatal("observation changed original receipt/effect/budget or invented postcondition")
	}
	if _, required, err := runtime.NextRuntimeEvidenceApplicabilityRequest(updated); err != nil || required {
		t.Fatalf("same revision repeated owner query: %v", err)
	}
	for _, scenario := range []string{"matching", "changed", "missing"} {
		for _, afterCAS := range []bool{false, true} {
			t.Run(fmt.Sprintf("real_query_restart/%s/after_cas_%v", scenario, afterCAS), func(t *testing.T) {
				if err := os.WriteFile(path, []byte(afterBytes), 0600); err != nil {
					t.Fatal(err)
				}
				if scenario == "changed" {
					if err := os.WriteFile(path, []byte("later\x00\xffbytes"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if scenario == "missing" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				repository.aggregate = original.CloneRaw()
				saves := repository.saves
				fault := &runtimeApplicabilityFaultRepository{delegate: repository, afterCAS: afterCAS}
				controller, err := runtime.NewRuntimeController(fault)
				if err != nil {
					t.Fatal(err)
				}
				owner.runtimeController = controller
				if _, _, err := owner.driveRuntimeEvidenceApplicability(ctx, original); err == nil || !fault.fired {
					t.Fatal("actual query CAS fault was not reached")
				}
				// Rebuild actual File owner and Runtime controller; restore the stored
				// opaque aggregate through raw JSON, never from a process-local cache.
				if err := file.Teardown(ctx); err != nil {
					t.Fatal(err)
				}
				file, err = filebiz.New(hub, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Start(); err != nil {
					t.Fatal(err)
				}
				body, err := json.Marshal(repository.aggregate)
				if err != nil {
					t.Fatal(err)
				}
				var reload runtime.RuntimeAggregate
				if err := json.Unmarshal(body, &reload); err != nil || reload.Validate() != nil {
					t.Fatalf("reload lost original transaction: %v", err)
				}
				repository.aggregate = reload
				controller, err = runtime.NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				owner.runtimeController = controller
				result, changed, err := owner.driveRuntimeEvidenceApplicability(ctx, reload)
				if err != nil || changed == afterCAS || repository.saves != saves+1 || len(result.Facts.EvidenceApplicabilityObservations) != 1 {
					t.Fatalf("recovery repeated or lost query observation CAS: changed=%v err=%v", changed, err)
				}
				if result.Facts.EvidenceApplicabilityObservations[0].Missing != (scenario == "missing") || runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, result.Facts.EvidenceApplicabilityObservations, result.Facts.EvidenceRevision) != (scenario != "matching") ||
					!reflect.DeepEqual(result.OperationReceipts, original.OperationReceipts) || !reflect.DeepEqual(result.Facts.Evidence, original.Facts.Evidence) || !reflect.DeepEqual(result.ExecutionBudget, original.ExecutionBudget) {
					t.Fatal("recovery changed original receipt/effect/budget or currentness result")
				}
			})
		}
	}
	if err := os.WriteFile(path, []byte(afterBytes), 0600); err != nil {
		t.Fatal(err)
	}
	// Changed bytes and missing are fresh actual owner results. Neither may
	// revive the original source; an I/O failure cannot create a missing fact.
	for _, scenario := range []string{"changed", "missing", "read_error"} {
		t.Run(scenario, func(t *testing.T) {
			body, _ := json.Marshal(original)
			var reload runtime.RuntimeAggregate
			if err := json.Unmarshal(body, &reload); err != nil {
				t.Fatal(err)
			}
			repository.aggregate = reload
			saves := repository.saves
			if scenario == "changed" {
				if err := os.WriteFile(path, []byte{0, 0xff, '\n'}, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if scenario == "missing" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "read_error" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, changed, err := owner.driveRuntimeEvidenceApplicability(ctx, reload)
			if scenario == "read_error" {
				if err == nil || changed || repository.saves != saves || !reflect.DeepEqual(repository.aggregate, reload) {
					t.Fatal("read error became missing fact or CAS")
				}
				return
			}
			if err != nil || !changed || !runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, result.Facts.EvidenceApplicabilityObservations, result.Facts.EvidenceRevision) || repository.saves != saves+1 {
				t.Fatalf("actual stale observation: %v", err)
			}
			// This source has never earned CWI credit. Reopening an already-open
			// row is not a second acceptance/reopen event; stale credit is tested
			// separately against an explicit settled preimage.
			if result.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied || runtimecontract.RuntimeEvidenceHasObservedCurrentApplicability(fact, result.Facts.EvidenceApplicabilityObservations, result.Facts.EvidenceRevision) || !reflect.DeepEqual(result.OperationReceipts, original.OperationReceipts) || !reflect.DeepEqual(result.ExecutionBudget, original.ExecutionBudget) {
				t.Fatal("stale observation retained credit or discarded original effect responsibility")
			}
		})
	}
}

type runtimeApplicabilityFaultRepository struct {
	delegate runtime.RuntimeStateRepository
	afterCAS bool
	fired    bool
}

func (r *runtimeApplicabilityFaultRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeApplicabilityFaultRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	cut := !r.fired && len(aggregate.Facts.EvidenceApplicabilityObservations) != 0
	if cut && !r.afterCAS {
		r.fired = true
		return fmt.Errorf("test interrupted before actual File applicability CAS")
	}
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if cut {
		r.fired = true
		return fmt.Errorf("test lost response after actual File applicability CAS")
	}
	return nil
}
