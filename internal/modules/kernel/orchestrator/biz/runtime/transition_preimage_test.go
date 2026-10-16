package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func transitionPreimageRequestForTest(aggregate RuntimeAggregate) RuntimeTransitionPreimageRequest {
	return RuntimeTransitionPreimageRequest{call: &runtimeTransitionPreimageCall{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, revision: aggregate.StateRevision,
	}}
}

type transitionPreimageTestRepository struct {
	transitionCoordinatorRepository
	request RuntimeTransitionPreimageRequest
	product RuntimeTransitionPreimage
	mode    string
}

func (r *transitionPreimageTestRepository) LoadTransitionPreimage(ctx context.Context, request RuntimeTransitionPreimageRequest) (RuntimeTransitionPreimage, error) {
	r.request = request
	workspaceID, runID, _ := request.Scope()
	aggregate, err := r.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeTransitionPreimage{}, err
	}
	if r.mode == "zero" {
		return RuntimeTransitionPreimage{}, nil
	}
	if r.mode == "reuse" {
		return r.product, nil
	}
	r.product, err = request.Compile(aggregate)
	return r.product, err
}

func TestRuntimeTransitionPreimageCompleteDetachedOriginal(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.ProcessedEventIDs = []string{" raw\x00事件 "}
	original := aggregate.CloneRaw()
	request := transitionPreimageRequestForTest(aggregate)
	product, err := request.Compile(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aggregate, original) {
		t.Fatal("compiler rewrote original input")
	}
	aggregate.ProcessedEventIDs[0] = "changed caller"
	aggregate.Facts.Coverage[0].TargetText = "changed nested caller"
	got, err := request.consume(product)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("complete original not detached: %v", err)
	}
	got.ProcessedEventIDs[0] = "changed consumer"
	if aggregate.ProcessedEventIDs[0] != "changed caller" {
		t.Fatal("consumer aliases caller")
	}
}

func TestRuntimeTransitionPreimageCannotForgeReplayOrSerialize(t *testing.T) {
	for _, scenario := range []string{"zero request", "zero product", "wrong call", "version", "json", "closed", "consumed", "recompile", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			aggregate := stateChangeAggregate(t)
			request := transitionPreimageRequestForTest(aggregate)
			if scenario == "zero request" {
				request = RuntimeTransitionPreimageRequest{}
			} else if scenario == "invalid" {
				aggregate.Decision.AuthoritySnapshot.RunID = "invalid"
			}
			product, err := request.Compile(aggregate)
			if scenario == "zero request" || scenario == "invalid" {
				if err == nil {
					t.Fatal("invalid compiler input accepted")
				}
				if _, err := request.consume(product); err == nil {
					t.Fatal("failed compilation supplied a product")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "zero product":
				product = RuntimeTransitionPreimage{}
			case "wrong call":
				request = transitionPreimageRequestForTest(aggregate)
			case "version":
				product.version = "unrecognized"
			case "json":
				wire, marshalErr := json.Marshal(product)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				product = RuntimeTransitionPreimage{}
				if err := json.Unmarshal(wire, &product); err != nil {
					t.Fatal(err)
				}
			case "closed":
				request.call.state.Store(4)
			case "consumed":
				if _, err := request.consume(product); err != nil {
					t.Fatal(err)
				}
			case "recompile":
				if _, err := request.Compile(aggregate); err == nil {
					t.Fatal("duplicate compilation accepted")
				}
				return
			}
			if _, err := request.consume(product); err == nil {
				t.Fatal("forged/replayed product accepted")
			}
		})
	}
}

func TestRuntimeTransitionPreimageCopiesHaveOneConsumer(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	request := transitionPreimageRequestForTest(aggregate)
	product, err := request.Compile(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if _, err := request.consume(product); err == nil {
				successes.Add(1)
			}
		})
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("consumers=%d", successes.Load())
	}
}

func TestRuntimeOwnerStateJointPreimageCannotForgeReplayOrOutliveCall(t *testing.T) {
	for _, scenario := range []string{"zero request", "zero product", "wrong call", "version", "json", "closed", "consumed", "recompile", "invalid raw", "invalid aggregate"} {
		t.Run(scenario, func(t *testing.T) {
			aggregate := stateChangeAggregate(t)
			record := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, aggregate)
			request := transitionPreimageRequestForTest(aggregate)
			switch scenario {
			case "zero request":
				request = RuntimeTransitionPreimageRequest{}
			case "invalid raw":
				record.ResultFeedbackVersion = 0
			case "invalid aggregate":
				aggregate.Decision.AuthoritySnapshot.RunID = "invalid"
			}
			product, err := request.CompileOwnerRecord(record, aggregate)
			if scenario == "zero request" || scenario == "invalid raw" || scenario == "invalid aggregate" {
				if err == nil {
					t.Fatal("invalid joint input accepted")
				}
				if _, err := request.consume(product); err == nil {
					t.Fatal("failed compilation supplied a product")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "zero product":
				product = RuntimeTransitionPreimage{}
			case "wrong call":
				request = transitionPreimageRequestForTest(aggregate)
			case "version":
				product.version = "unrecognized"
			case "json":
				wire, _ := json.Marshal(product)
				product = RuntimeTransitionPreimage{}
				if err := json.Unmarshal(wire, &product); err != nil {
					t.Fatal(err)
				}
			case "closed":
				request.call.state.Store(4)
			case "consumed":
				if _, err := request.consume(product); err != nil {
					t.Fatal(err)
				}
			case "recompile":
				if _, err := request.CompileOwnerRecord(record, aggregate); err == nil {
					t.Fatal("joint product recompiled")
				}
				return
			}
			if _, err := request.consume(product); err == nil {
				t.Fatal("forged/replayed/closed joint product accepted")
			}
		})
	}
}

func TestRuntimeOwnerStateJointPreimageDetachedAndOneConsumer(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	record := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, aggregate)
	original := aggregate.CloneRaw()
	request := transitionPreimageRequestForTest(aggregate)
	product, err := request.CompileOwnerRecord(record, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	record.Facts.Coverage[0].TargetText = "changed DTO"
	aggregate.Facts.Coverage[0].TargetText = "changed aggregate"
	if !reflect.DeepEqual(product.aggregate, original) {
		t.Fatal("joint product aliases DTO or converted aggregate")
	}
	var successes atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if _, err := request.consume(product); err == nil {
				successes.Add(1)
			}
		})
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("joint consumers=%d", successes.Load())
	}
}

func TestRuntimeTransitionCoordinatorPreimageRetainsIndependentTerminalTransitionGate(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	previous.StateRevision++
	previous.TerminalMaterialSeal = &seal
	next := previous.CloneRaw()
	next.StateRevision++
	next.TerminalMaterialSeal = nil
	// Both complete states are independently valid. Only the cross-image gate
	// can reject deleting a committed terminal seal.
	if err := previous.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := next.Validate(); err != nil {
		t.Fatal(err)
	}
	original := previous.CloneRaw()
	for _, complete := range []bool{false, true} {
		base := transitionCoordinatorRepository{previous: &previous}
		var repository RuntimeStateRepository = &base
		if complete {
			repository = &transitionPreimageTestRepository{transitionCoordinatorRepository: base}
		}
		coordinator, err := NewRuntimeTransitionCoordinator(repository)
		if err != nil {
			t.Fatal(err)
		}
		if err := coordinator.Commit(context.Background(), next, previous.StateRevision); err == nil || !strings.Contains(err.Error(), "terminal material seal") {
			t.Fatalf("complete=%v bypassed cross-image terminal invariant: %v", complete, err)
		}
		if !reflect.DeepEqual(previous, original) {
			t.Fatal("transition rejection rewrote original source")
		}
	}
}

func TestRuntimeTransitionCoordinatorSingleCommitPreimageGuards(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "scope", "revision", "invariant", "zero", "reuse", "save error", "response loss"} {
		t.Run(scenario, func(t *testing.T) {
			previous := stateChangeAggregate(t)
			next := previous.CloneRaw()
			next.StateRevision++
			repository := &transitionPreimageTestRepository{transitionCoordinatorRepository: transitionCoordinatorRepository{previous: &previous}}
			switch scenario {
			case "missing":
				repository.previous = nil
			case "scope":
				previous.Facts.RunID = "another-run"
			case "revision":
				previous.StateRevision++
			case "invariant":
				previous.Decision.AuthoritySnapshot.RunID = "invalid"
			case "zero":
				repository.mode = "zero"
			case "reuse":
				other := transitionPreimageRequestForTest(previous)
				var err error
				repository.product, err = other.Compile(previous)
				if err != nil {
					t.Fatal(err)
				}
				repository.mode = "reuse"
			case "save error", "response loss":
				repository.saveErr = errors.New("interrupted")
				repository.mutateBeforeError = scenario == "response loss"
			}
			coordinator, err := NewRuntimeTransitionCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			err = coordinator.Commit(context.Background(), next, next.StateRevision-1)
			wantSaves := 0
			if scenario == "valid" || scenario == "save error" || scenario == "response loss" {
				wantSaves = 1
			}
			if (err == nil) != (scenario == "valid") || repository.loads != 1 || repository.saves != wantSaves {
				t.Fatalf("err=%v loads=%d saves=%d", err, repository.loads, repository.saves)
			}
			if scenario == "revision" {
				var conflict *RuntimeCASConflictError
				if !errors.As(err, &conflict) || conflict.ObservedRevision != previous.StateRevision {
					t.Fatalf("lost exact revision conflict: %v", err)
				}
			}
			if repository.request.call.state.Load() != 4 {
				t.Fatal("request outlived Commit")
			}
			if _, err := repository.request.Compile(next); err == nil {
				t.Fatal("completed Commit request compiled another preimage")
			}
			if scenario == "valid" {
				repository.mode = "reuse"
				if err := coordinator.Commit(context.Background(), next, next.StateRevision-1); err == nil || repository.loads != 2 || repository.saves != 1 {
					t.Fatalf("new Commit borrowed old success: %v", err)
				}
			}
		})
	}
}

func FuzzRuntimeTransitionPreimageCompleteOriginalGate(f *testing.F) {
	f.Add(uint8(0), " raw\x00事件 ")
	f.Add(uint8(1), "wrong-run")
	f.Add(uint8(2), "\xff\x00")
	f.Add(uint8(3), "changed-budget")
	f.Fuzz(func(t *testing.T, field uint8, value string) {
		aggregate := stateChangeAggregate(t)
		switch field % 5 {
		case 0:
			aggregate.ProcessedEventIDs = []string{value}
		case 1:
			aggregate.Decision.AuthoritySnapshot.RunID = common.RunID(value)
		case 2:
			aggregate.Facts.DefinitionSeed.Hash = value
		case 3:
			aggregate.ExecutionBudget.Policy.PolicyHash = value
		case 4:
			aggregate.Definition.Revision = value
		}
		original := aggregate.CloneRaw()
		wantErr := aggregate.CloneRaw().Validate()
		request := transitionPreimageRequestForTest(aggregate)
		product, gotErr := request.Compile(aggregate)
		if (wantErr == nil) != (gotErr == nil) || (wantErr != nil && wantErr.Error() != gotErr.Error()) {
			t.Fatalf("complete gate mismatch: want=%v got=%v", wantErr, gotErr)
		}
		if !reflect.DeepEqual(aggregate, original) {
			t.Fatal("compiler rewrote original")
		}
		if gotErr == nil {
			got, err := request.consume(product)
			if err != nil || !reflect.DeepEqual(got, original) {
				t.Fatalf("original changed: %v", err)
			}
		}
	})
}
