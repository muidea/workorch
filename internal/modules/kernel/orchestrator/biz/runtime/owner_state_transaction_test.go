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
	return aggregate.Validate()
}

func TestOwnerStateTransactionCompleteImagesMatchIndependentGates(t *testing.T) {
	for _, arm := range []string{"valid", "raw feedback", "raw scope", "converted feedback", "semantic", "source", "budget", "dispatch", "header order", "owner remainder"} {
		t.Run(arm, func(t *testing.T) {
			a := stateChangeAggregate(t)
			r := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)
			originalA, originalR := a.CloneRaw(), r.Clone()
			err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
				compiler := &ownerStateInvariantCompiler{}
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
							checked.CompletedDispatches = append(checked.CompletedDispatches, SealedDispatch{})
						case "header order":
							raw.Facts.RunID = ""
							checked.ResultFeedbackVersion = 0
						case "owner remainder":
							checked.Decision.AuthoritySnapshot.RunID = "wrong-run"
						}
					}
					got := validate(ownerStateSourceImageInCompiler(record, checked, compiler))
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
					bad.AfterSemantic = func(*runtimecontract.RuntimeOwnerInvariantSourceGate) error { return sentinel }
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
	request               RuntimeTransitionPreimageRequest
	preimageMutation      func(*runtimecontract.RuntimeStateRecord, *RuntimeAggregate)
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
func (r *ownerStateTransactionTestRepository) LoadTransitionPreimage(_ context.Context, request RuntimeTransitionPreimageRequest) (RuntimeTransitionPreimage, error) {
	r.rereads++
	r.request = request
	raw, a := r.raw.Clone(), r.previous.CloneRaw()
	if r.preimageMutation != nil {
		r.preimageMutation(&raw, &a)
	}
	return request.CompileOwnerRecord(raw, a)
}
func (r *ownerStateTransactionTestRepository) Save(_ context.Context, a RuntimeAggregate, _ uint64) error {
	r.saves++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.previous, r.raw = a.CloneRaw(), runtimeStateRecordForTransactionTest(a)
	return nil
}
func runtimeStateRecordForTransactionTest(a RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	// Only fields used by this fixture; do not normalize malformed raw sources.
	return runtimecontract.RuntimeStateRecord{ResultFeedbackVersion: a.ResultFeedbackVersion, StateRevision: a.StateRevision, EvidenceSequence: a.EvidenceSequence,
		Definition: a.Definition, Facts: a.Facts, SemanticPlanning: a.SemanticPlanning, ImplementationWork: a.ImplementationWork, Reconciliation: a.Reconciliation,
		CoverageProofLineage: a.CoverageProofLineage, Decision: a.Decision, ExecutionBudget: a.ExecutionBudget, CompletedDispatches: sealedDispatchesToRuntimeRecords(a.CompletedDispatches)}
}

func TestOwnerStateTransactionPrivateEventReadsAgainAndDoesNotEscape(t *testing.T) {
	for _, scenario := range []string{"normal", "bad raw", "stale", "scope", "save failure"} {
		t.Run(scenario, func(t *testing.T) {
			a := stateChangeAggregate(t)
			repo := &ownerStateTransactionTestRepository{previous: a, raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)}
			switch scenario {
			case "bad raw":
				repo.preimageMutation = func(r *runtimecontract.RuntimeStateRecord, _ *RuntimeAggregate) { r.ResultFeedbackVersion = 0 }
			case "stale":
				repo.preimageMutation = func(r *runtimecontract.RuntimeStateRecord, a *RuntimeAggregate) { r.StateRevision++; a.StateRevision++ }
			case "scope":
				repo.preimageMutation = func(_ *runtimecontract.RuntimeStateRecord, a *RuntimeAggregate) { a.Facts.RunID = "wrong" }
			case "save failure":
				repo.saveErr = errors.New("exact CAS failure")
			}
			c, _ := NewRuntimeController(repo)
			failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "runtime-drive-failed", Message: "exact owner fact"}
			event := RuntimeInternalEvent{Kind: RuntimeInternalEventFailureObserved, Scope: RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{FailureFact: &failure}}
			result, err := c.ApplyInternalEvent(context.Background(), event)
			if repo.loads != 1 || repo.rereads != 1 {
				t.Fatalf("actual read counts=%d/%d err=%v", repo.loads, repo.rereads, err)
			}
			if scenario == "normal" {
				if err != nil || repo.saves != 1 || result.Aggregate.Validate() != nil {
					t.Fatalf("valid event: %v saves=%d", err, repo.saves)
				}
			} else if err == nil {
				t.Fatal("invalid transaction accepted")
			}
			if scenario != "normal" && scenario != "save failure" && repo.saves != 0 {
				t.Fatal("drift reached CAS")
			}
			if c.transitions.ownerTransaction != nil {
				t.Fatal("shared controller retained transaction")
			}
			if _, err := repo.request.CompileOwnerRecord(repo.raw, repo.previous); err == nil {
				t.Fatal("request survived event")
			}
			if repo.request.call.compiler.Load() != nil {
				t.Fatal("closed request retained raw invariant compiler")
			}
			if scenario == "normal" {
				if _, err := c.ApplyInternalEvent(context.Background(), event); err != nil || repo.saves != 1 || repo.loads != 2 || repo.rereads != 1 {
					t.Fatalf("duplicate changed CAS/budget/effect: %v", err)
				}
				repo.raw.ResultFeedbackVersion = 0
				if _, err := c.ApplyInternalEvent(context.Background(), event); err == nil || repo.saves != 1 || repo.rereads != 1 {
					t.Fatal("new event borrowed previous success")
				}
			}
		})
	}
}

func TestOwnerStateInvariantCompilerRequiresExactDetachedRawAndSeparateGates(t *testing.T) {
	a := stateChangeAggregate(t)
	c := &ownerStateInvariantCompiler{}
	before, after := 0, 0
	for range 2 {
		if err := c.validate(a, false, func() error { before++; return nil }); err != nil {
			t.Fatal(err)
		}
		if err := c.validate(a, true, func() error { after++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if before != 1 || after != 1 {
		t.Fatalf("ordered continuations did not independently compile once: %d/%d", before, after)
	}
	original := a.CloneRaw()
	a.Facts.Coverage[0].TargetText = "same hash raw mutation"
	sentinel := errors.New("actual changed subtree rejected")
	for range 2 {
		if err := c.validate(a, true, func() error { after++; return sentinel }); !errors.Is(err, sentinel) {
			t.Fatal("raw mutation borrowed prior success")
		}
	}
	if after != 3 || !reflect.DeepEqual(c.images[0].raw, original) {
		t.Fatal("failure cached or successful preimage aliases caller")
	}
	// A successful before-semantic result cannot grant the after-semantic gate.
	if err := c.validate(a, false, func() error { before++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := c.validate(a, true, func() error { after++; return sentinel }); !errors.Is(err, sentinel) || after != 4 {
		t.Fatal("one ordered gate substituted for another")
	}
}

func TestOwnerStateTransactionRejectsReentrantOrConcurrentGate(t *testing.T) {
	a := stateChangeAggregate(t)
	if err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
		image := ownerStateSourceImage(nil, a)
		originalBefore := image.BeforeSemantic
		image.BeforeSemantic = func() error {
			if err := validate(ownerStateSourceImage(nil, a)); err == nil {
				t.Fatal("recursive gate admitted")
			}
			result := make(chan error, 1)
			go func() { result <- validate(ownerStateSourceImage(nil, a)) }()
			if err := <-result; err == nil {
				t.Fatal("concurrent gate admitted")
			}
			return originalBefore()
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
			compiler := &ownerStateInvariantCompiler{}
			if err := validate(ownerStateSourceImageInCompiler(&r, a, compiler)); err != nil {
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
			got := validate(ownerStateSourceImageInCompiler(&r, a, compiler))
			want := independentOwnerStateImage(&r, a)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("raw drift first error: %v != %v", got, want)
			}
			return got
		})
		_ = err
	})
}
