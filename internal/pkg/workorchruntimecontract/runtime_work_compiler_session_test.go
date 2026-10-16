package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

type workCompilerSessionInput struct {
	Definition RuntimeDefinitionContext
	Sources    RuntimeImplementationWorkLedgerSources
	Execution  RuntimeExecutionContract
	Catalogs   []RuntimeOperationAdmissibility
	Round      RuntimeImplementationWorkRound
	Prior      []RuntimeImplementationWorkRound
}

func workCompilerSessionFixture(t *testing.T) workCompilerSessionInput {
	t.Helper()
	ledger, sources := implementationWorkLedgerFixture(t)
	round := ledger.Rounds[0].Clone()
	catalogs := []RuntimeOperationAdmissibility{round.WorkingBinding.OwnerCatalog.Clone()}
	catalogs[0].OperationKinds = []string{StepKindFileWrite, RuntimeBlockKindPatchApply}
	catalogs[0] = catalogs[0].Normalized()
	catalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalogs[0])
	round.WorkingBinding = nil
	round.Hash = StableRuntimeImplementationWorkRoundHash(round)
	return workCompilerSessionInput{
		Definition: RuntimeDefinitionContext{Revision: sources.Seed.DefinitionRevision, StartupInputHash: sources.StartupInputHash,
			GoalID: "goal-1", GoalDefinition: "original goal", DoDID: "dod-1", DoDDefinition: "original criterion",
			CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindPatchApply, RuntimeBlockKindManagedValidationSession}},
		Sources: sources, Round: round, Catalogs: catalogs,
		Execution: RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision(),
			StateChangePolicy: RuntimeStateChangeAllowedButNotRequired, ProfileContracts: sources.Profiles},
	}
}

func (in workCompilerSessionInput) compile(purpose, reference bool, session *runtimeSemanticValidationSession) (RuntimeSemanticPlanManifest, error) {
	s := in.Sources
	if purpose {
		preview := RuntimeImplementationWorkScopePreview{Request: in.Round.ScopeRequest, Observation: in.Round.ScopeObservation}
		if reference {
			return referenceWorkPurposeCompiler(s.WorkspaceID, s.RunID, in.Definition, s.Seed, s.Focuses, in.Execution, in.Catalogs, preview, in.Prior)
		}
		if session != nil {
			return compileRuntimeWorkPurposeInSession(s.WorkspaceID, s.RunID, in.Definition, s.Seed, s.Focuses, in.Execution, in.Catalogs, preview, in.Prior, session)
		}
		return CompileRuntimeSemanticImplementationPurposeManifest(s.WorkspaceID, s.RunID, in.Definition, s.Seed, s.Focuses, in.Execution, in.Catalogs, preview, in.Prior)
	}
	if reference {
		return referenceWorkManifestCompiler(s.WorkspaceID, s.RunID, in.Definition, s.Seed, s.Focuses, in.Execution, in.Catalogs, in.Round)
	}
	if session != nil {
		return compileRuntimeWorkManifestInSession(s.WorkspaceID, s.RunID, in.Definition, s.Seed, s.Focuses, in.Execution, in.Catalogs, in.Round, session)
	}
	return CompileRuntimeSemanticPlanManifestForImplementationWork(s.WorkspaceID, s.RunID, in.Definition, s.Seed, s.Focuses, in.Execution, in.Catalogs, in.Round)
}

func compareWorkCompilerSession(t *testing.T, in workCompilerSessionInput, purpose bool, session *runtimeSemanticValidationSession) {
	t.Helper()
	before, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	got, gotErr := in.compile(purpose, false, session)
	want, wantErr := in.compile(purpose, true, nil)
	if cwiFocusBindingsError(gotErr) != cwiFocusBindingsError(wantErr) || !reflect.DeepEqual(got, want) {
		t.Fatalf("compiler product/first error differs: %v != %v", gotErr, wantErr)
	}
	after, err := json.Marshal(in)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("compiler changed original source bytes")
	}
	if gotErr == nil {
		got.ProofObligations[0].ImplementationWork.MutationInputCatalogs[0].TargetRefs[0] = "caller-change"
		got.OwnerCatalogs[0].TargetRefs[0] = "caller-change"
		got.ProofObligations[0].ImplementationWork.Scope.Selection.TargetRefs[0] = "caller-change"
		after, _ = json.Marshal(in)
		if !bytes.Equal(before, after) {
			t.Fatal("compiler output aliases source")
		}
		again, againErr := in.compile(purpose, false, session)
		if againErr != nil || !reflect.DeepEqual(again, want) {
			t.Fatal("output mutation polluted later invocation")
		}
	}
}

func mutateWorkCompilerSession(in *workCompilerSessionInput, arm uint8, value string) {
	switch arm % 16 {
	case 0:
	case 1:
		in.Catalogs[0].TargetRefs[0] = value
	case 2:
		in.Catalogs[0].SnapshotID = value
	case 3:
		in.Definition.StartupInputHash = value
	case 4:
		in.Round.ScopeRequest.RequestID = value
	case 5:
		in.Execution.OperationContractRevision = value
	case 6:
		in.Catalogs = nil
	case 7:
		in.Catalogs = []RuntimeOperationAdmissibility{}
	case 8:
		in.Catalogs = append(in.Catalogs, in.Catalogs[0].Clone())
	case 9:
		other := in.Catalogs[0].Clone()
		other.PrerequisiteID = "other-prerequisite"
		other.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(other)
		in.Catalogs = append(in.Catalogs, other, RuntimeOperationAdmissibility{OwnerModuleID: value})
	case 10:
		in.Round.ScopeObservation = nil
	case 11:
		in.Round.Ordinal = 0
		in.Round.Hash = StableRuntimeImplementationWorkRoundHash(in.Round)
	case 12:
		in.Prior = []RuntimeImplementationWorkRound{{CoverageWorkItemID: in.Round.CoverageWorkItemID}}
	case 13:
		in.Sources.Focuses[0].AssessmentText = value
	case 14:
		in.Catalogs[0].TargetRefs = []string{value, value}
	case 15:
		in.Round.ScopeObservation.Selection.ObservedRevision = value
	}
}

func TestWorkCompilerSessionOriginalProductsAndRejections(t *testing.T) {
	for _, purpose := range []bool{false, true} {
		for arm := uint8(0); arm < 16; arm++ {
			in := workCompilerSessionFixture(t)
			session := newRuntimeSemanticValidationSession()
			if _, err := in.compile(purpose, false, session); err != nil {
				t.Fatal(err)
			}
			mutateWorkCompilerSession(&in, arm, "changed raw value")
			compareWorkCompilerSession(t, in, purpose, session)
			compareWorkCompilerSession(t, in, purpose, nil)
			raw, _ := json.Marshal(in)
			var loaded workCompilerSessionInput
			if err := json.Unmarshal(raw, &loaded); err != nil {
				t.Fatal(err)
			}
			compareWorkCompilerSession(t, loaded, purpose, newRuntimeSemanticValidationSession())
		}
	}
}

func TestWorkCompilerCatalogSessionPreservesCollectionErrorOrder(t *testing.T) {
	for arm := uint8(0); arm < 16; arm++ {
		in := workCompilerSessionFixture(t)
		session := newRuntimeSemanticValidationSession()
		if err := validateRuntimeWorkCompilerCatalogsInSession(in.Catalogs, session); err != nil {
			t.Fatal(err)
		}
		mutateWorkCompilerSession(&in, arm, "raw")
		got := validateRuntimeWorkCompilerCatalogsInSession(in.Catalogs, session)
		want := validateRuntimeSemanticOwnerCatalogCollection(in.Catalogs)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("collection error order: %v != %v", got, want)
		}
	}
}

func FuzzWorkCompilerSessionMatchesOriginal(f *testing.F) {
	for arm := uint8(0); arm < 16; arm++ {
		f.Add(arm, "raw")
	}
	f.Fuzz(func(t *testing.T, arm uint8, value string) {
		if len(value) > 128 {
			t.Skip()
		}
		for _, purpose := range []bool{false, true} {
			in := workCompilerSessionFixture(t)
			session := newRuntimeSemanticValidationSession()
			if _, err := in.compile(purpose, false, session); err != nil {
				t.Fatal(err)
			}
			mutateWorkCompilerSession(&in, arm, value)
			compareWorkCompilerSession(t, in, purpose, session)
		}
	})
}

func BenchmarkWorkCompilerSessionCompleteCall(b *testing.B) {
	in := workCompilerSessionFixture(&testing.T{})
	for _, reference := range []bool{true, false} {
		name := "session"
		if reference {
			name = "original"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				session := newRuntimeSemanticValidationSession()
				for range 4 {
					if _, err := in.compile(true, reference, session); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
