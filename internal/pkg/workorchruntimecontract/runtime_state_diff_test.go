package workorchruntimecontract

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func mutateDistinctDiffValue(v reflect.Value) bool {
	if !v.IsValid() || !v.CanSet() {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString("mutated")
		return true
	case reflect.Bool:
		v.SetBool(true)
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
		return true
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
		return true
	case reflect.Ptr:
		elem := reflect.New(v.Type().Elem())
		_ = mutateDistinctDiffValue(elem.Elem())
		v.Set(elem)
		return true
	case reflect.Slice:
		slice := reflect.MakeSlice(v.Type(), 1, 1)
		_ = mutateDistinctDiffValue(slice.Index(0))
		v.Set(slice)
		return true
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		val := reflect.New(v.Type().Elem()).Elem()
		_ = mutateDistinctDiffValue(key)
		_ = mutateDistinctDiffValue(val)
		m.SetMapIndex(key, val)
		v.Set(m)
		return true
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			if mutateDistinctDiffValue(f) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func diffHasAnyChange(diff RuntimeStateTransitionDiff) bool {
	val := reflect.ValueOf(diff)
	for i := 0; i < val.NumField(); i++ {
		if val.Type().Field(i).Name == "PreviousAvailable" {
			continue
		}
		if val.Field(i).Bool() {
			return true
		}
	}
	return false
}

func TestRuntimeStateTransitionDiffReflectionExhaustiveness(t *testing.T) {
	base := RuntimeStateRecord{}
	same := ComputeRuntimeStateTransitionDiff(base, base)
	if !same.PreviousAvailable || diffHasAnyChange(same) {
		t.Fatalf("identical records produced non-zero diff: %+v", same)
	}

	full := FullRuntimeStateTransitionDiff()
	if full.PreviousAvailable {
		t.Fatalf("FullRuntimeStateTransitionDiff must set PreviousAvailable=false")
	}
	fullVal := reflect.ValueOf(full)
	for i := 0; i < fullVal.NumField(); i++ {
		field := fullVal.Type().Field(i)
		if field.Name == "PreviousAvailable" {
			continue
		}
		if !fullVal.Field(i).Bool() {
			t.Fatalf("FullRuntimeStateTransitionDiff left field %s false", field.Name)
		}
	}

	recordType := reflect.TypeOf(RuntimeStateRecord{})
	for i := 0; i < recordType.NumField(); i++ {
		field := recordType.Field(i)
		if !field.IsExported() || field.Name == "Facts" {
			continue
		}
		mutated := RuntimeStateRecord{}
		fv := reflect.ValueOf(&mutated).Elem().Field(i)
		if !mutateDistinctDiffValue(fv) {
			t.Fatalf("could not mutate RuntimeStateRecord field %s", field.Name)
		}
		got := ComputeRuntimeStateTransitionDiff(base, mutated)
		if !diffHasAnyChange(got) {
			t.Fatalf("mutating RuntimeStateRecord.%s did not flip any diff flag: %+v", field.Name, got)
		}
	}

	factsType := reflect.TypeOf(RuntimeFacts{})
	for i := 0; i < factsType.NumField(); i++ {
		field := factsType.Field(i)
		if !field.IsExported() {
			continue
		}
		mutated := RuntimeStateRecord{}
		fv := reflect.ValueOf(&mutated.Facts).Elem().Field(i)
		if !mutateDistinctDiffValue(fv) {
			t.Fatalf("could not mutate RuntimeFacts field %s", field.Name)
		}
		got := ComputeRuntimeStateTransitionDiff(base, mutated)
		if !diffHasAnyChange(got) || !got.FactsAny() {
			t.Fatalf("mutating RuntimeFacts.%s did not flip any diff/FactsAny flag: %+v", field.Name, got)
		}
	}

	// Granular sub-field checks used by owner reload admission & transition validators.
	t.Run("granular-subfields", func(t *testing.T) {
		term := RuntimeStateRecord{}
		term.Decision.Terminal = &RuntimeTerminalDecision{Code: "done"}
		dTerm := ComputeRuntimeStateTransitionDiff(base, term)
		if !dTerm.DecisionTerminal || !dTerm.Decision {
			t.Fatalf("Decision.Terminal mutation missed: %+v", dTerm)
		}

		prof := RuntimeStateRecord{}
		prof.Facts.Activation.ExecutionContract.ProfileContracts = []RuntimeProfileContract{{ID: "p-1"}}
		dProf := ComputeRuntimeStateTransitionDiff(base, prof)
		if !dProf.FactsActivationProfileContracts || !dProf.FactsActivation() || !dProf.FactsCore() {
			t.Fatalf("Activation ProfileContracts mutation missed: %+v", dProf)
		}

		pol := RuntimeStateRecord{}
		pol.Facts.Activation.EvidencePolicies = []RuntimeEvidencePolicy{{ID: "ep-1"}}
		dPol := ComputeRuntimeStateTransitionDiff(base, pol)
		if !dPol.FactsActivationEvidencePolicies || !dPol.FactsActivation() || !dPol.FactsCore() {
			t.Fatalf("Activation EvidencePolicies mutation missed: %+v", dPol)
		}

		actOther := RuntimeStateRecord{}
		actOther.Facts.Activation.ExecutionContract.StateChangePolicy = RuntimeStateChangeAllowedButNotRequired
		dActOther := ComputeRuntimeStateTransitionDiff(base, actOther)
		if !dActOther.FactsActivationOther || dActOther.FactsActivationProfileContracts || dActOther.FactsActivationEvidencePolicies {
			t.Fatalf("Activation other mutation misclassified: %+v", dActOther)
		}

		adm := RuntimeStateRecord{}
		adm.Facts.Resources.OperationAdmissibilities = []RuntimeOperationAdmissibility{{PrerequisiteID: "ad-1"}}
		dAdm := ComputeRuntimeStateTransitionDiff(base, adm)
		if !dAdm.FactsResourcesOperationAdmissibilities || !dAdm.FactsResources() || dAdm.FactsResourcesOther {
			t.Fatalf("Resources.OperationAdmissibilities mutation misclassified: %+v", dAdm)
		}

		resOther := RuntimeStateRecord{}
		resOther.Facts.Resources.Manifest = &WorkspaceManifest{Revision: "rev-1"}
		dResOther := ComputeRuntimeStateTransitionDiff(base, resOther)
		if !dResOther.FactsResourcesOther || !dResOther.FactsResources() || dResOther.FactsResourcesOperationAdmissibilities {
			t.Fatalf("Resources other mutation misclassified: %+v", dResOther)
		}

		evAcc := RuntimeStateRecord{}
		evAcc.Facts.Evidence.Accepted = []EvidenceFact{{ID: "ev-1"}}
		if d := ComputeRuntimeStateTransitionDiff(base, evAcc); !d.FactsEvidenceAccepted || d.FactsEvidenceRejected || !d.FactsEvidence() {
			t.Fatalf("Evidence.Accepted mutation misclassified: %+v", d)
		}

		evRej := RuntimeStateRecord{}
		evRej.Facts.Evidence.Rejected = []EvidenceFact{{ID: "ev-2"}}
		if d := ComputeRuntimeStateTransitionDiff(base, evRej); !d.FactsEvidenceRejected || d.FactsEvidenceAccepted || !d.FactsEvidence() {
			t.Fatalf("Evidence.Rejected mutation misclassified: %+v", d)
		}
	})
}

func TestRuntimeStateTransitionDiffCommittedHistoryAndCompileEquivalence(t *testing.T) {
	before := RuntimeStateRecord{
		SemanticPlanning: RuntimeSemanticPlanningLedger{
			History: []RuntimeSemanticPlanningGeneration{{RetiredAtStateRevision: 8, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated}},
		},
		ImplementationWork: RuntimeImplementationWorkLedger{
			Rounds: []RuntimeImplementationWorkRound{{CompletedReceiptSetHash: "collective", Hash: "original"}},
		},
		Facts: RuntimeFacts{
			WorkspaceID: "ws-diff", RunID: "run-diff",
			Evidence: EvidenceLedger{Accepted: []EvidenceFact{{ID: "z", Summary: "original"}}},
		},
	}
	after := before.Clone()
	after.StateRevision = 9
	after.Facts.Evidence.Accepted = append([]EvidenceFact{{ID: "a", Summary: "new"}}, after.Facts.Evidence.Accepted...)
	diff := ComputeRuntimeStateTransitionDiff(before, after)
	if err := ValidateRuntimeCommittedHistoryTransitionWithDiff(before, after, diff); err != nil {
		t.Fatalf("valid with-diff transition failed: %v", err)
	}
	if err := ValidateRuntimeCommittedHistoryTransitionWithDiff(before, after, FullRuntimeStateTransitionDiff()); err != nil {
		t.Fatalf("valid full-diff transition failed: %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"removed semantic history": func(r *RuntimeStateRecord) { r.SemanticPlanning.History = nil },
		"rewritten semantic history": func(r *RuntimeStateRecord) {
			r.SemanticPlanning.History[0].Reason = RuntimeSemanticPlanningRetirementInputRevision
		},
		"rehash completed work round": func(r *RuntimeStateRecord) { r.ImplementationWork.Rounds[0].Hash = "tampered" },
		"rewritten original evidence": func(r *RuntimeStateRecord) { r.Facts.Evidence.Accepted[1].Summary = "tampered" },
		"cross workspace":             func(r *RuntimeStateRecord) { r.Facts.WorkspaceID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := after.Clone()
			mutate(&changed)
			withDiffErr := ValidateRuntimeCommittedHistoryTransitionWithDiff(before, changed, ComputeRuntimeStateTransitionDiff(before, changed))
			fullErr := ValidateRuntimeCommittedHistoryTransitionWithDiff(before, changed, FullRuntimeStateTransitionDiff())
			if !errors.Is(withDiffErr, common.ErrConflict) || !errors.Is(fullErr, common.ErrConflict) {
				t.Fatalf("expected conflict for both paths, withDiff=%v full=%v", withDiffErr, fullErr)
			}
			// CompileRuntimeOwnerStateTransition requires valid full records; verify nil guard too.
			badImage := RuntimeOwnerStateSourceImage{Converted: nil}
			if err := CompileRuntimeOwnerStateTransition(before, badImage); err == nil {
				t.Fatal("CompileRuntimeOwnerStateTransition accepted nil Converted image")
			}
		})
	}
	_, _, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		baseValid := run.RuntimeState
		if err := baseValid.Validate(); err != nil {
			t.Fatalf("archive fixture invalid: %v", err)
		}
		validNext := baseValid.Clone()
		validNext.StateRevision++
		cDiff := CompileRuntimeOwnerStateTransition(baseValid, RuntimeOwnerStateSourceImage{
			Converted: &validNext, AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error { return nil },
		})
		cFull := CompileRuntimeOwnerStateTransitionWithDiff(baseValid, RuntimeOwnerStateSourceImage{
			Converted: &validNext, AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error { return nil },
		}, FullRuntimeStateTransitionDiff())
		if fmt.Sprint(cDiff) != fmt.Sprint(cFull) {
			t.Fatalf("CompileRuntimeOwnerStateTransition valid mismatch: %v != %v", cDiff, cFull)
		}
		brokenNext := validNext.Clone()
		brokenNext.Facts.DefinitionRevision = ""
		bDiff := CompileRuntimeOwnerStateTransition(baseValid, RuntimeOwnerStateSourceImage{
			Converted: &brokenNext, AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error { return nil },
		})
		bFull := CompileRuntimeOwnerStateTransitionWithDiff(baseValid, RuntimeOwnerStateSourceImage{
			Converted: &brokenNext, AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error { return nil },
		}, FullRuntimeStateTransitionDiff())
		if bDiff == nil || fmt.Sprint(bDiff) != fmt.Sprint(bFull) {
			t.Fatalf("CompileRuntimeOwnerStateTransition invalid mismatch: %v != %v", bDiff, bFull)
		}
	}
}
