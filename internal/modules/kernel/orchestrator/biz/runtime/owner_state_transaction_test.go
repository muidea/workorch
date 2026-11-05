package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func independentOwnerStateImage(record *runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate) error {
	if record != nil {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("load Runtime state: %w", err)
		}
	}
	if record != nil && !reflect.DeepEqual(*record, ownerStateRecord(aggregate)) {
		return fmt.Errorf("runtime owner record conversion changed original data")
	}
	return aggregate.Validate()
}

func TestOwnerStateTransactionCompleteImagesMatchIndependentGates(t *testing.T) {
	for _, arm := range []string{"valid", "raw feedback", "raw scope", "converted feedback", "semantic", "source", "budget", "dispatch", "header order", "owner remainder"} {
		t.Run(arm, func(t *testing.T) {
			a := stateChangeAggregate(t)
			r := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)
			originalA, originalR := a.CloneRaw(), r.Clone()
			err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
				// Full first Load, after-image and actual Commit reload all use
				// the same compiler, including their complete owner continuations.
				for phase := range 3 {
					checked := a.CloneRaw()
					raw := r.Clone()
					var record *runtimecontract.RuntimeStateRecord
					if phase != 1 {
						record = &raw
					} else {
						checked.StateRevision++
					}
					if phase == 2 {
						switch arm {
						case "raw feedback":
							raw.ResultFeedbackVersion = 0
						case "raw scope":
							raw.Facts.RunID = ""
						case "converted feedback":
							checked.ResultFeedbackVersion = 0
						case "semantic":
							checked.SemanticPlanning.DefinitionSeed.PolicyHash = "same-hash-raw-drift"
						case "source":
							checked.EvidenceSequence++
						case "budget":
							checked.ExecutionBudget.Policy.PolicyRevision = "drift"
						case "dispatch":
							checked.CompletedDispatches = append(checked.CompletedDispatches, runtimecontract.RuntimeDispatchRecord{})
						case "header order":
							raw.Facts.RunID = ""
							checked.ResultFeedbackVersion = 0
						case "owner remainder":
							checked.Decision.AuthoritySnapshot.RunID = "wrong-run"
						}
					}
					got := validate(ownerStateSourceImageInCompiler(record, checked))
					want := independentOwnerStateImage(record, checked)
					if fmt.Sprint(got) != fmt.Sprint(want) {
						t.Fatalf("phase %d first error differs: %v != %v", phase, got, want)
					}
					if got != nil {
						return got
					}
				}
				return nil
			})
			if arm == "valid" && err != nil {
				t.Fatal(err)
			}
			if arm != "valid" && err == nil {
				t.Fatal("drift borrowed a prior success")
			}
			if !reflect.DeepEqual(a, originalA) || !reflect.DeepEqual(r, originalR) {
				t.Fatal("transaction changed original inputs")
			}
		})
	}
}

func TestOwnerStateTransactionRevokesFullGateAndRetainsErrorOrder(t *testing.T) {
	a := stateChangeAggregate(t)
	image := ownerStateSourceImage(nil, a)
	var retained func(runtimecontract.RuntimeOwnerStateSourceImage) error
	sentinel := errors.New("exact owner invariant")
	for _, exit := range []string{"normal", "failure", "panic"} {
		func() {
			defer func() {
				if recovered := recover(); (exit == "panic") != (recovered == "owner panic") {
					t.Fatalf("panic changed: %v", recovered)
				}
			}()
			err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
				retained = validate
				if err := validate(image); err != nil {
					return err
				}
				if exit == "panic" {
					panic("owner panic")
				}
				if exit == "failure" {
					bad := image
					bad.AggregateJoins = func(*runtimecontract.RuntimeOwnerInvariantSourceGate) error { return sentinel }
					if !errors.Is(validate(bad), sentinel) {
						t.Fatal("remaining invariant skipped")
					}
					if !errors.Is(validate(image), sentinel) {
						t.Fatal("failed transaction became reusable")
					}
					return fmt.Errorf("original owner prefix: %w", sentinel)
				}
				return nil
			})
			if exit == "normal" && err != nil {
				t.Fatal(err)
			}
			if exit == "failure" && (!errors.Is(err, sentinel) || !strings.HasPrefix(err.Error(), "original owner prefix:")) {
				t.Fatalf("owner error wrapping changed: %v", err)
			}
		}()
		if err := retained(image); err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("retained gate survived %s: %v", exit, err)
		}
	}
	if err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error { return validate(image) }); err != nil {
		t.Fatal("fresh transaction inherited failure", err)
	}
}

type ownerStateTransactionTestRepository struct {
	previous              RuntimeAggregate
	raw                   runtimecontract.RuntimeStateRecord
	loads, rereads, saves int
	beforeCAS             func()
	responseLost          bool
	saveErr               error
}

func (r *ownerStateTransactionTestRepository) Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error) {
	r.loads++
	a := r.previous.CloneRaw()
	return a, independentOwnerStateImage(&r.raw, a)
}
func (r *ownerStateTransactionTestRepository) LoadOwnerStateOriginal(context.Context, common.WorkspaceID, common.RunID) (runtimecontract.RuntimeStateRecord, RuntimeAggregate, error) {
	r.loads++
	return r.raw.Clone(), r.previous.CloneRaw(), nil
}

func (r *ownerStateTransactionTestRepository) Save(_ context.Context, a RuntimeAggregate, expected uint64) error {
	if r.beforeCAS != nil {
		r.beforeCAS()
	}
	if r.previous.StateRevision != expected {
		return &RuntimeCASConflictError{ExpectedRevision: expected, ObservedRevision: r.previous.StateRevision}
	}
	r.saves++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.previous, r.raw = a.CloneRaw(), runtimeStateRecordForTransactionTest(a)
	if r.responseLost {
		r.responseLost = false
		return errors.New("response lost after CAS")
	}
	return nil
}
func runtimeStateRecordForTransactionTest(a RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	return RuntimeStateRecordFromAggregate(a)
}

func TestOwnerStateTransactionUsesOnePreimageAndAtomicCAS(t *testing.T) {
	for _, scenario := range []string{"normal", "bad raw", "stale CAS", "scope", "save failure", "response loss"} {
		t.Run(scenario, func(t *testing.T) {
			a := stateChangeAggregate(t)
			repo := &ownerStateTransactionTestRepository{previous: a, raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)}
			switch scenario {
			case "bad raw":
				repo.raw.ResultFeedbackVersion = 0
			case "scope":
				repo.raw.Facts.RunID = "other"
			case "stale CAS":
				repo.beforeCAS = func() { repo.previous.StateRevision++ }
			case "save failure":
				repo.saveErr = errors.New("exact CAS failure")
			case "response loss":
				repo.responseLost = true
			}
			c, _ := NewRuntimeController(repo)
			failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "runtime-drive-failed", Message: "exact owner fact"}
			event := RuntimeInternalEvent{Kind: RuntimeInternalEventFailureObserved, Scope: RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{FailureFact: &failure}}
			_, err := c.ApplyInternalEvent(context.Background(), event)
			if repo.loads != 1 || repo.rereads != 0 {
				t.Fatalf("reads=%d/%d: %v", repo.loads, repo.rereads, err)
			}
			if scenario == "normal" {
				if err != nil || repo.saves != 1 {
					t.Fatal("valid event", err)
				}
			} else if err == nil {
				t.Fatal("invalid/unacknowledged transaction accepted")
			}
			if (scenario == "bad raw" || scenario == "scope" || scenario == "stale CAS") && repo.saves != 0 {
				t.Fatal("invalid preimage overwrote CAS state")
			}
			if c.transitions.ownerTransaction != nil {
				t.Fatal("shared controller retained transaction")
			}
			if scenario == "normal" || scenario == "response loss" {
				fresh, _ := NewRuntimeController(repo)
				if _, err := fresh.ApplyInternalEvent(context.Background(), event); err != nil || repo.saves != 1 {
					t.Fatalf("recovery/duplicate repeated CAS: %v saves=%d", err, repo.saves)
				}
				repo.raw.ResultFeedbackVersion = 0
				if _, err := fresh.ApplyInternalEvent(context.Background(), event); err == nil || repo.saves != 1 {
					t.Fatal("new event borrowed previous success")
				}
			}
		})
	}
}

func TestOwnerStateTransactionRejectsReentrantOrConcurrentGate(t *testing.T) {
	a := stateChangeAggregate(t)
	if err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
		image := ownerStateSourceImage(nil, a)
		originalBefore := image.AggregateJoins
		image.AggregateJoins = func(gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
			if err := validate(ownerStateSourceImage(nil, a)); err == nil {
				t.Fatal("recursive gate admitted")
			}
			result := make(chan error, 1)
			go func() { result <- validate(ownerStateSourceImage(nil, a)) }()
			if err := <-result; err == nil {
				t.Fatal("concurrent gate admitted")
			}
			return originalBefore(gate)
		}
		return validate(image)
	}); err != nil {
		t.Fatal(err)
	}
}

func FuzzOwnerStateTransactionImageOracle(f *testing.F) {
	for arm := uint8(0); arm < 8; arm++ {
		f.Add(arm, "原\x00文")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		a := stateChangeAggregate(t)
		r := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)
		err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
			if err := validate(ownerStateSourceImageInCompiler(&r, a)); err != nil {
				return err
			}
			switch arm % 8 {
			case 0:
				r.Definition.StartupInputHash = raw
			case 1:
				r.Facts.DefinitionRevision = raw
			case 2:
				a.SemanticPlanning.DefinitionSeed.PolicyHash = raw
			case 3:
				a.ExecutionBudget.Policy.PolicyRevision = raw
			case 4:
				a.Decision.AuthoritySnapshot.RunID = common.RunID(raw)
			case 5:
				a.Facts.Coverage[0].TargetText = raw
			case 6:
				a.Facts.Focus.ID = raw
			case 7:
				a.ProcessedEventIDs = append(a.ProcessedEventIDs, raw)
			}
			got := validate(ownerStateSourceImageInCompiler(&r, a))
			want := independentOwnerStateImage(&r, a)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("raw drift first error: %v != %v", got, want)
			}
			return got
		})
		_ = err
	})
}
