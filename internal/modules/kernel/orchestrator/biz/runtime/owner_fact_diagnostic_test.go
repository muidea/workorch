package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFactExhaustionSealsExactCauseAndPreservesRestartBudget(t *testing.T) {
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, OriginalClass: "validation_readiness", OriginalCode: "native_check_assertion_unresolved",
		OriginalMessage: "  exact native check cannot resolve an assertion  ", Signature: "owner-readiness/cwi-v2", TargetRefs: []string{"checks/cwi.test"}}
	controller, repository, _ := prepareOwnerFactAssessmentWithDiagnostic(t, false, diagnostic)
	expected := missingOwnerFactDiagnostic(repository.aggregate.Facts)
	if expected == nil {
		t.Fatal("fixture lost exact owner preparation")
	}
	before := repository.saves
	failed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "owner_fact_convergence_exhausted" || !reflect.DeepEqual(failed.Decision.Terminal.OriginalDiagnostic, expected) {
		t.Fatal("SM-2 did not seal exact owner cause")
	}
	if repository.saves != before+1 || len(failed.Facts.Repair.NoProgressOperationShapes) != 1 || len(failed.Facts.Coverage[0].ReopenLineage) != 0 {
		t.Fatal("diagnostic changed CAS/acquisition boundary")
	}
	if err := failed.Validate(); err != nil {
		t.Fatal(err)
	}
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := controller.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Decision.Terminal.OriginalDiagnostic, expected) {
		t.Fatal("restart lost original cause")
	}
	saves := repository.saves
	_, err = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{}})
	if !errors.Is(err, common.ErrConflict) || repository.saves != saves {
		t.Fatalf("duplicate terminal event mutated state: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"message drift":  func(a *RuntimeAggregate) { a.Decision.Terminal.OriginalDiagnostic.OriginalMessage = "different" },
		"revision drift": func(a *RuntimeAggregate) { a.Decision.Terminal.OriginalDiagnostic.ObservedRevision = "stale" },
		"wrong owner": func(a *RuntimeAggregate) {
			a.Decision.Terminal.OriginalDiagnostic.OwnerModuleID = runtimecontract.RuntimeModuleFile
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := reloaded
			terminal := *reloaded.Decision.Terminal
			terminal.OriginalDiagnostic = terminal.OriginalDiagnostic.CloneOriginal()
			changed.Decision.Terminal = &terminal
			mutate(&changed)
			if changed.Validate() == nil {
				t.Fatal("accepted mismatched cause")
			}
		})
	}
}

func TestOwnerFactDiagnosticRejectsStaleForeignAndAmbiguousPreparations(t *testing.T) {
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, OriginalClass: "validation_readiness", OriginalCode: "check_missing",
		OriginalMessage: "exact native check is missing", Signature: "owner-readiness/cwi-v2"}
	_, repository, _ := prepareOwnerFactAssessmentWithDiagnostic(t, false, diagnostic)
	for name, mutate := range map[string]func(*runtimecontract.RuntimeValidationProofPreparation){
		"stale definition":  func(p *runtimecontract.RuntimeValidationProofPreparation) { p.DefinitionRevision = "stale" },
		"other requirement": func(p *runtimecontract.RuntimeValidationProofPreparation) { p.RequirementHash = "other" },
		"stale catalog": func(p *runtimecontract.RuntimeValidationProofPreparation) {
			p.OriginalDiagnostic.ObservedRevision = "stale"
		},
		"other owner": func(p *runtimecontract.RuntimeValidationProofPreparation) {
			p.OriginalDiagnostic.OwnerModuleID = runtimecontract.RuntimeModuleFile
		},
	} {
		t.Run(name, func(t *testing.T) {
			facts := repository.aggregate.Facts.Normalized()
			for ci := range facts.Resources.OperationAdmissibilities {
				for si := range facts.Resources.OperationAdmissibilities[ci].ValidationSurfaces {
					for pi := range facts.Resources.OperationAdmissibilities[ci].ValidationSurfaces[si].ProofRequirementPreparations {
						p := &facts.Resources.OperationAdmissibilities[ci].ValidationSurfaces[si].ProofRequirementPreparations[pi]
						if p.OriginalDiagnostic != nil {
							mutate(p)
						}
					}
				}
			}
			if got := missingOwnerFactDiagnostic(facts); got != nil {
				t.Fatalf("non-current diagnostic was sealed: %+v", got)
			}
		})
	}
	facts := repository.aggregate.Facts.Normalized()
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, facts.Resources.OperationAdmissibilities...)
	if got := missingOwnerFactDiagnostic(facts); got != nil {
		t.Fatalf("ambiguous owner cause was selected: %+v", got)
	}
}
