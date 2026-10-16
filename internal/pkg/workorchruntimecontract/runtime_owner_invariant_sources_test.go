package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestOwnerInvariantSourceGateCompleteColdOracle(t *testing.T) {
	ledger, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	for _, arm := range []string{"valid", "manifest raw", "catalog raw", "document raw", "batch hash", "batch identity", "owner", "missing owner", "duplicate owner", "nil empty", "obligation"} {
		t.Run(arm, func(t *testing.T) {
			for _, method := range []string{"batch", "document", "feasibility", "obligation"} {
				s := newRuntimeSemanticValidationSession()
				// Warm complete original inputs, never a supplied success flag.
				if err := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
					_, _, err := g.CompileAcceptedBatch(*ledger.Manifest, ledger.Batches[0], *ledger.Document, ledger.Feasibilities)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				m, b, d := cloneRuntimeSemanticPlanManifestRaw(*ledger.Manifest), ledger.Batches[0].Clone(), cloneRuntimeSemanticPlanDocumentRaw(*ledger.Document)
				facts := cloneRuntimeStateRawSlice(ledger.Feasibilities, func(f RuntimeSemanticProofFeasibility) RuntimeSemanticProofFeasibility { return f.Clone() })
				switch arm {
				case "manifest raw":
					m.ProofObligations[0].DeclarationRef = "same hash drift"
				case "catalog raw":
					m.OwnerCatalogs[0].SnapshotID = "same hash drift"
				case "document raw":
					d.Proposals[0].DeclarationRef = "same hash drift"
				case "batch hash":
					b.Hash = "drift"
				case "batch identity":
					b.RequestLogicalID = ""
					b.Hash = StableRuntimeSemanticPlanBatchHash(b)
				case "owner":
					facts[0].OwnerModuleID = "wrong-owner"
				case "missing owner":
					facts = nil
				case "duplicate owner":
					facts = append(facts, facts[0])
				case "nil empty":
					m.OwnerFactDependencyCatalogs = []RuntimeOperationAdmissibility{}
				case "obligation":
					m.ProofObligations[0].RequiredEvidenceKinds = nil
				}
				before, _ := json.Marshal(struct {
					M RuntimeSemanticPlanManifest
					B RuntimeSemanticPlanBatch
					D RuntimeSemanticPlanDocument
					F []RuntimeSemanticProofFeasibility
				}{m, b, d, facts})
				var want error
				got := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
					switch method {
					case "batch":
						wf, wc, err := CompileRuntimeSemanticAcceptedBatchBindings(m, b, d, facts)
						want = err
						gf, gc, err := g.CompileAcceptedBatch(m, b, d, facts)
						if !reflect.DeepEqual(gf, wf) || !reflect.DeepEqual(gc, wc) {
							t.Fatal("full batch products differ")
						}
						return err
					case "document":
						want = d.ValidateAgainstManifest(m)
						return g.ValidateDocument(m, d)
					case "feasibility":
						want = ValidateRuntimeSemanticFeasibilitySet(m, d, facts, true)
						return g.ValidateFeasibilities(m, d, facts, true)
					default:
						w, err := ResolveRuntimeSemanticEvidenceObligation(m.ProofObligations[0])
						want = err
						v, err := g.ResolveObligation(m.ProofObligations[0])
						if !reflect.DeepEqual(v, w) {
							t.Fatal("full obligation products differ")
						}
						return err
					}
				})
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Fatalf("%s first error: %v != %v", method, got, want)
				}
				after, _ := json.Marshal(struct {
					M RuntimeSemanticPlanManifest
					B RuntimeSemanticPlanBatch
					D RuntimeSemanticPlanDocument
					F []RuntimeSemanticProofFeasibility
				}{m, b, d, facts})
				if !reflect.DeepEqual(before, after) {
					t.Fatal("validator changed raw inputs")
				}
			}
		})
	}
}

func TestOwnerInvariantDefinitionSourcesFullOracleAndIsolation(t *testing.T) {
	for arm := uint8(0); arm < 9; arm++ {
		r := semanticDefinitionSessionFixture(t)
		s := newRuntimeSemanticValidationSession()
		if err := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
			return g.ValidateDefinitionSources(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		}); err != nil {
			t.Fatal(err)
		}
		mutateSemanticDefinitionSession(&r, arm, "原文\x00 same hash drift")
		want := referenceSemanticLedgerDefinitionGate(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		got := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
			return g.ValidateDefinitionSources(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		})
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("arm %d: %v != %v", arm, got, want)
		}
	}
	ledger, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	want, wc, err := CompileRuntimeSemanticAcceptedBatchBindings(*ledger.Manifest, ledger.Batches[0], *ledger.Document, ledger.Feasibilities)
	if err != nil {
		t.Fatal(err)
	}
	s := newRuntimeSemanticValidationSession()
	for range 2 {
		if err := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
			got, gc, err := g.CompileAcceptedBatch(*ledger.Manifest, ledger.Batches[0], *ledger.Document, ledger.Feasibilities)
			if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gc, wc) {
				t.Fatalf("detached products: %v", err)
			}
			got[0].CoverageEvidenceBindings[0].Proofs[0].EvidenceKind = "mutated return"
			gc.DefinitionRevision = "mutated return"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOwnerInvariantSourceGateLifetimeFailureAndContention(t *testing.T) {
	var captured *RuntimeOwnerInvariantSourceGate
	var copied RuntimeOwnerInvariantSourceGate
	sentinel := errors.New("owner contextual first error")
	s := newRuntimeSemanticValidationSession()
	err := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
		captured = g
		copied = *g
		g.state.mu.Lock()
		if err := g.ValidateDefinitionSources(RuntimeSemanticPlanningLedger{}, RuntimeDefinitionSeed{}, nil); err == nil {
			t.Fatal("concurrent/recursive gate admitted")
		}
		g.state.mu.Unlock()
		_, err := g.ResolveObligation(RuntimeSemanticProofObligation{})
		if err == nil {
			t.Fatal("bad obligation admitted")
		}
		if next := g.ValidateDefinitionSources(RuntimeSemanticPlanningLedger{}, RuntimeDefinitionSeed{}, nil); fmt.Sprint(next) != fmt.Sprint(err) {
			t.Fatal("first failure was not sticky")
		}
		return nil // Owner cannot ignore the failed complete validator.
	})
	if err == nil {
		t.Fatal("ignored gate failure admitted image")
	}
	if captured.state.semantic != nil || !captured.state.closed {
		t.Fatal("captured gate retained compilation")
	}
	if err := captured.ValidateDefinitionSources(RuntimeSemanticPlanningLedger{}, RuntimeDefinitionSeed{}, nil); err == nil {
		t.Fatal("captured closed gate admitted")
	}
	if err := copied.ValidateDefinitionSources(RuntimeSemanticPlanningLedger{}, RuntimeDefinitionSeed{}, nil); err == nil {
		t.Fatal("copied gate escaped revocation")
	}
	wire, err := json.Marshal(copied)
	var restored RuntimeOwnerInvariantSourceGate
	if err != nil || json.Unmarshal(wire, &restored) != nil || restored.ValidateDefinitionSources(RuntimeSemanticPlanningLedger{}, RuntimeDefinitionSeed{}, nil) == nil {
		t.Fatal("serialization exported a live gate")
	}
	if err := (&RuntimeOwnerInvariantSourceGate{}).ValidateDefinitionSources(RuntimeSemanticPlanningLedger{}, RuntimeDefinitionSeed{}, nil); err == nil {
		t.Fatal("forged zero gate admitted")
	}
	if err := withRuntimeOwnerInvariantSourceGate(s, func(*RuntimeOwnerInvariantSourceGate) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal("owner error changed")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic swallowed")
			}
		}()
		_ = withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error { captured = g; panic(sentinel) })
	}()
	if captured.state.semantic != nil || !captured.state.closed {
		t.Fatal("panic retained compilation")
	}
	if err := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- g.validate(func(*runtimeSemanticValidationSession) error { close(entered); <-release; return nil })
		}()
		<-entered
		_, rejected := g.ResolveObligation(RuntimeSemanticProofObligation{})
		close(release)
		if err := <-done; err != nil || rejected == nil {
			t.Fatal("concurrent source gate was not rejected")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerInvariantAssessmentSourcesCompleteLedgerAndJoins(t *testing.T) {
	base, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	purpose, err := ResolveRuntimeAssessmentPurposePreimage(base, bindings)
	if err != nil {
		t.Fatal(err)
	}
	for _, arm := range []string{"valid", "ledger", "binding", "purpose", "authority", "missing dispatch", "duplicate dispatch", "duplicate session", "attempt", "document", "owner"} {
		t.Run(arm, func(t *testing.T) {
			s := newRuntimeSemanticValidationSession()
			for pass := 0; pass < 2; pass++ {
				ledger := cloneRuntimeSemanticPlanningLedgerRaw(base)
				manifest := RuntimeCandidateContractManifest{Bindings: CloneCoverageEvidenceBindings(bindings), AssessmentPurpose: purpose.Purpose, AssessmentPurposeManifestHash: purpose.SemanticManifestHash}
				d := RuntimeDispatchRecord{ID: "assessment", Candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{}}, AssessmentAuthorityInput: &RuntimeAssessmentAuthorityInput{Purpose: purpose.Purpose}}
				completed := []RuntimeDispatchRecord{d}
				attempts := []RuntimeCandidateAttemptRecord{{State: RuntimeCandidateAttemptStateAccepted, DispatchID: d.ID, LogicalRequestID: "assessment-request"}}
				sessions := []RuntimeCandidateRequestSession{{LogicalRequestID: "assessment-request", CandidateManifest: &manifest}}
				if pass == 1 {
					switch arm {
					case "ledger":
						ledger.DefinitionSeed.Hash = "same hash drift"
					case "binding":
						manifest.Bindings[0].CoverageWorkItemID = "other"
					case "purpose":
						manifest.AssessmentPurposeManifestHash = "other"
					case "authority":
						completed[0].AssessmentAuthorityInput = nil
					case "missing dispatch":
						completed = nil
					case "duplicate dispatch":
						completed = append(completed, d)
					case "duplicate session":
						sessions = append(sessions, sessions[0])
					case "attempt":
						attempts[0].LogicalRequestID = "other"
					case "document":
						ledger.BatchDocuments[0].Proposals[0].DeclarationRef = "same hash drift"
					case "owner":
						ledger.Feasibilities[0].OwnerModuleID = "other"
					}
				}
				want := ValidateRuntimeAcceptedAssessmentPurposePreimages(ledger, nil, completed, attempts, sessions)
				got := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
					return g.ValidateAssessmentSources(ledger, nil, completed, attempts, sessions)
				})
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Fatalf("pass %d first error: %v != %v", pass, got, want)
				}
				if pass == 0 && got != nil {
					t.Fatal(got)
				}
			}
		})
	}
}

func FuzzOwnerInvariantDefinitionSources(f *testing.F) {
	for arm := uint8(0); arm < 9; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		r := semanticDefinitionSessionFixture(t)
		s := newRuntimeSemanticValidationSession()
		_ = withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
			return g.ValidateDefinitionSources(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		})
		mutateSemanticDefinitionSession(&r, arm, raw)
		want := referenceSemanticLedgerDefinitionGate(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		got := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
			return g.ValidateDefinitionSources(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		})
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("first error: %v != %v", got, want)
		}
	})
}
