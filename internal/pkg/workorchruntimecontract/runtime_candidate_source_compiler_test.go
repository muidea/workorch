package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// All three consumers start from independently valid raw inputs. This fixture
// does not claim a completed aggregate or grant execution/acceptance credit.
func candidateSourceCompilerFixture(t testing.TB, count int) RuntimeStateRecord {
	t.Helper()
	var r RuntimeStateRecord
	r.Facts.WorkspaceID, r.Facts.RunID = "ws-candidate-sources", "run-candidate-sources"
	for i := range count {
		decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
			State: ConvergenceStateNeedStateChange,
			RequiredOutput: RequiredOutputContract{
				AcceptedKinds: []string{StepKindFileWrite}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs:  []common.CoverageWorkItemID{common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", i))}, TargetRefs: []string{"target.txt"},
			},
		})
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
		if err != nil {
			t.Fatal(err)
		}
		session := runtimeManifestBoundSessionForTest(t, manifest)
		session.LogicalRequestID = fmt.Sprintf("request-%d", i)
		session.Transaction = runtimeDispatchIntrinsicFixture(t, 0).Transaction
		session.Transaction.AuthoritySnapshotID = fmt.Sprintf("authority-%d", i)
		session.ExpectedDocument.LogicalRequestID = session.LogicalRequestID
		session.ExpectedDocument.Hash = StableRuntimeExpectedDocumentHash(*session.ExpectedDocument)
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.WorkspaceID, dispatch.RunID = r.Facts.WorkspaceID, r.Facts.RunID
		dispatch.Transaction = session.Transaction
		dispatch.ID = fmt.Sprintf("dispatch-%d", i)
		dispatch.Candidate.StateChange.Operations[0].Operation.BindingRefs = []string{manifest.Bindings[0].ID}
		dispatch.Candidate, err = DeriveRuntimeCandidateOperationAuthority(manifest, dispatch.Candidate)
		if err != nil {
			t.Fatal(err)
		}
		dispatch.CandidateHash = StableRuntimeCandidateHash(dispatch.Candidate)
		dispatch.CandidateManifestHash = manifest.Hash
		dispatch.CoverageBindings = CloneCoverageEvidenceBindings(manifest.Bindings)
		dispatch.CoverageBindingHash = StableRuntimeCoverageEvidenceBindingSnapshotHash(manifest.Bindings)
		dispatch.OperationTargetAuthorizations, err = RuntimeDispatchOperationTargetAuthorizationsFromManifest(manifest, dispatch.Candidate)
		if err != nil {
			t.Fatal(err)
		}
		dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
		attempt := RuntimeCandidateAttemptRecord{
			AuthorityContractID: StableAuthorityContractID(r.Facts.WorkspaceID, r.Facts.RunID, session.Transaction),
			Transaction:         session.Transaction, LogicalRequestID: session.LogicalRequestID, StablePrefixHash: session.StablePrefixHash,
			ManifestHash: manifest.Hash, State: RuntimeCandidateAttemptStateAccepted, DispatchID: dispatch.ID, CandidateHash: dispatch.CandidateHash,
			AdmissionTargetAuthorizations: CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations),
			TargetAuthorizationHash:       StableRuntimeDispatchOperationTargetAuthorizationsHash(dispatch.OperationTargetAuthorizations),
			AdmissionCoverageBindings:     CloneCoverageEvidenceBindings(manifest.Bindings), AdmissionCoverageBindingHash: dispatch.CoverageBindingHash,
			DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
		}
		r.CandidateAttempts = append(r.CandidateAttempts, attempt)
		r.CandidateRequestSessions = append(r.CandidateRequestSessions, session)
		r.CompletedDispatches = append(r.CompletedDispatches, dispatch)
	}
	if err := candidateSourceColdOracle(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func candidateSourceColdOracle(r RuntimeStateRecord) error {
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeCandidateRequestSessions(r.CandidateRequestSessions); err != nil {
		return err
	}
	return ValidateRuntimeAcceptedCandidateManifestLineage(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions)
}

func TestCandidateSourceCompilerFixture(t *testing.T) { candidateSourceCompilerFixture(t, 3) }

func BenchmarkCandidateSourceCompleteConsumers(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		r := candidateSourceCompilerFixture(b, count)
		b.Run(fmt.Sprintf("history_%d/cold_twice", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for range 2 {
					if err := candidateSourceColdOracle(r); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run(fmt.Sprintf("history_%d/lexical_twice", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s := newCandidateSourceTestSession()
				for range 2 {
					if err := candidateSourceSessionConsumers(r, s); err != nil {
						b.Fatal(err)
					}
				}
			}
		})

	}
}

func candidateSourceSessionConsumers(r RuntimeStateRecord, s *runtimeSemanticValidationSession) error {
	if err := validateRuntimeCandidateAttemptSourcesInSession(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts, s); err != nil {
		return err
	}
	if err := validateRuntimeCandidateRequestSourcesInSession(r.CandidateRequestSessions, s); err != nil {
		return err
	}
	return validateRuntimeCandidateManifestSourcesInSession(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions, s)
}

func TestCandidateSourceCompilerExactInputsAndColdErrorOrder(t *testing.T) {
	base := candidateSourceCompilerFixture(t, 3)
	mutations := map[string]func(*RuntimeStateRecord){
		"unchanged":         func(*RuntimeStateRecord) {},
		"workspace":         func(r *RuntimeStateRecord) { r.Facts.WorkspaceID = "another-workspace" },
		"run":               func(r *RuntimeStateRecord) { r.Facts.RunID = "another-run" },
		"attempt identity":  func(r *RuntimeStateRecord) { r.CandidateAttempts[0].LogicalRequestID = "drift" },
		"attempt scope":     func(r *RuntimeStateRecord) { r.CandidateAttempts[0].AuthorityContractID = "drift" },
		"attempt duplicate": func(r *RuntimeStateRecord) { r.CandidateAttempts = append(r.CandidateAttempts, r.CandidateAttempts[0]) },
		"attempt rehashed target": func(r *RuntimeStateRecord) {
			a := &r.CandidateAttempts[0]
			a.AdmissionTargetAuthorizations[0].Target.ExactTargetRefs[0] = "drift"
			a.TargetAuthorizationHash = StableRuntimeDispatchOperationTargetAuthorizationsHash(a.AdmissionTargetAuthorizations)
		},
		"attempt nil empty": func(r *RuntimeStateRecord) {
			r.CandidateAttempts[0].AdmissionValidationRecipes = []RuntimeValidationRecipe{}
		},
		"request duplicate": func(r *RuntimeStateRecord) {
			r.CandidateRequestSessions = append(r.CandidateRequestSessions, r.CandidateRequestSessions[0])
		},
		"request wire": func(r *RuntimeStateRecord) {
			e := r.CandidateRequestSessions[0].ExpectedDocument
			e.WireContractHash = "drift"
			e.Hash = StableRuntimeExpectedDocumentHash(*e)
		},
		"request raw same hash": func(r *RuntimeStateRecord) {
			r.CandidateRequestSessions[0].CandidateManifest.Bindings[0].DeclarationRef = "原文\x00\r\n changed"
		},
		"request rehashed body": func(r *RuntimeStateRecord) {
			m := r.CandidateRequestSessions[0].CandidateManifest
			m.Bindings[0].DeclarationRef = "drift"
			m.Hash = StableRuntimeCandidateContractManifestHash(*m)
		},
		"request nil empty": func(r *RuntimeStateRecord) {
			r.CandidateRequestSessions[0].CandidateManifest.Operations[0].Variants = []RuntimeOperationVariantContract{}
		},
		"request sequence": func(r *RuntimeStateRecord) { r.CandidateRequestSessions[0].NextSequence++ },
		"missing manifest": func(r *RuntimeStateRecord) { r.CandidateRequestSessions[0].CandidateManifest = nil },
		"missing attempt":  func(r *RuntimeStateRecord) { r.CandidateAttempts = r.CandidateAttempts[1:] },
		"missing request":  func(r *RuntimeStateRecord) { r.CandidateRequestSessions = r.CandidateRequestSessions[1:] },
		"missing dispatch": func(r *RuntimeStateRecord) { r.CompletedDispatches = r.CompletedDispatches[1:] },
		"duplicate dispatch": func(r *RuntimeStateRecord) {
			r.CompletedDispatches = append(r.CompletedDispatches, r.CompletedDispatches[0])
		},
		"active duplicate": func(r *RuntimeStateRecord) { r.ActiveDispatch = &r.CompletedDispatches[0] },
		"active move": func(r *RuntimeStateRecord) {
			d := r.CompletedDispatches[0]
			r.ActiveDispatch = &d
			r.CompletedDispatches = r.CompletedDispatches[1:]
		},
		"dispatch candidate": func(r *RuntimeStateRecord) {
			*r.CompletedDispatches[0].Candidate.StateChange.Operations[0].Operation.Parameters[0].String = "changed"
		},
		"dispatch target": func(r *RuntimeStateRecord) {
			r.CompletedDispatches[0].OperationTargetAuthorizations[0].Target.ExactTargetRefs[0] = "drift"
		},
		"dispatch binding": func(r *RuntimeStateRecord) { r.CompletedDispatches[0].CoverageBindings[0].DeclarationRef = "drift" },
		"dispatch recipe": func(r *RuntimeStateRecord) {
			r.CompletedDispatches[0].ValidationRecipes = []RuntimeValidationRecipe{{ID: "unproven", Command: "true"}}
		},
		"multiple errors": func(r *RuntimeStateRecord) {
			r.CandidateAttempts[0].AuthorityContractID = "drift"
			r.CandidateRequestSessions[0].NextSequence++
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			for _, method := range []string{"attempts", "requests", "lineage"} {
				s := newCandidateSourceTestSession()
				if err := candidateSourceSessionConsumers(base, s); err != nil {
					t.Fatal(err)
				}
				r := base.Clone()
				mutate(&r)
				before := r.Clone()
				var want error
				got := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
					switch method {
					case "attempts":
						want = ValidateRuntimeCandidateAttemptRecordsForScope(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts)
						return g.ValidateCandidateAttempts(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts)
					case "requests":
						want = ValidateRuntimeCandidateRequestSessions(r.CandidateRequestSessions)
						return g.ValidateCandidateRequests(r.CandidateRequestSessions)
					default:
						want = ValidateRuntimeAcceptedCandidateManifestLineage(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions)
						return g.ValidateCandidateManifestSources(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions)
					}
				})
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Fatalf("%s: got %v want %v", method, got, want)
				}
				if !reflect.DeepEqual(r, before) {
					t.Fatal("raw inputs changed")
				}
			}
		})
	}
}

func TestCandidateSourceCompilerDetachedBoundedAndNotRetained(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 3)
	s := newCandidateSourceTestSession()
	if err := candidateSourceSessionConsumers(r, s); err != nil {
		t.Fatal(err)
	}
	old := *s.candidateSources
	detached := r.Clone()
	if err := candidateSourceSessionConsumers(detached, s); err != nil {
		t.Fatal(err)
	}
	if old != *s.candidateSources {
		t.Fatal("exact duplicate did not reuse the complete consumer results")
	}
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(r.CandidateAttempts), reflect.ValueOf(old.attempts.values), "attempts")
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(r.CandidateRequestSessions), reflect.ValueOf(old.requests.values), "requests")
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(r.CompletedDispatches), reflect.ValueOf(old.lineage.completed), "completed")
	wire, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var reload RuntimeStateRecord
	if err = json.Unmarshal(wire, &reload); err != nil {
		t.Fatal(err)
	}
	if err = candidateSourceSessionConsumers(reload, s); err != nil {
		t.Fatal(err)
	}
	if old != *s.candidateSources {
		t.Fatal("fresh exact reload did not reuse source results")
	}
	// Raw nil and empty differ even when the original validator accepts both.
	r.CandidateAttempts[0].AdmissionValidationRecipes = []RuntimeValidationRecipe{}
	if err = validateRuntimeCandidateAttemptSourcesInSession(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts, s); err != nil {
		t.Fatal(err)
	}
	if s.candidateSources.attempts == old.attempts {
		t.Fatal("nil/empty were collapsed before the source check")
	}
	r.CandidateRequestSessions[0].CandidateManifest.Bindings[0].DeclarationRef = "source mutation"
	if err = validateRuntimeCandidateRequestSourcesInSession(r.CandidateRequestSessions, s); err == nil {
		t.Fatal("same hash hid source mutation")
	}
	if s.candidateSources.requests != old.requests {
		t.Fatal("failed source was cached")
	}
	retained := newRuntimeSemanticValidationSession()
	if retained.candidateSources != nil {
		t.Fatal("current candidate results survived the lexical transaction")
	}
	for _, count := range []int{1, 4, 2} {
		next := candidateSourceCompilerFixture(t, count)
		if err = candidateSourceSessionConsumers(next, s); err != nil {
			t.Fatal(err)
		}
		if len(s.candidateSources.lineage.completed) != count {
			t.Fatal("source compiler retained an older complete image")
		}
	}
	// Cold public calls cannot inherit another consumer's or transaction's result.
	if candidateSourceColdOracle(r) == nil {
		t.Fatal("cold validator accepted a warm source mutation")
	}
}

func TestCandidateSourceGateRevocationFailureAndConcurrency(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 1)
	var captured *RuntimeOwnerInvariantSourceGate
	s := newCandidateSourceTestSession()
	err := withRuntimeOwnerInvariantSourceGate(s, func(g *RuntimeOwnerInvariantSourceGate) error {
		captured = g
		g.state.mu.Lock()
		if g.ValidateCandidateRequests(r.CandidateRequestSessions) == nil {
			t.Fatal("concurrent gate admitted")
		}
		g.state.mu.Unlock()
		if err := g.ValidateCandidateRequests(r.CandidateRequestSessions); err != nil {
			return err
		}
		bad := r.Clone()
		bad.CandidateRequestSessions[0].NextSequence++
		failure := g.ValidateCandidateRequests(bad.CandidateRequestSessions)
		if failure == nil {
			t.Fatal("bad request admitted")
		}
		if next := g.ValidateCandidateAttempts(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); fmt.Sprint(next) != fmt.Sprint(failure) {
			t.Fatal("failure not sticky")
		}
		return nil
	})
	if err == nil {
		t.Fatal("ignored failure admitted")
	}
	if captured.ValidateCandidateRequests(r.CandidateRequestSessions) == nil || captured.ValidateCandidateAttempts(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts) == nil || captured.ValidateCandidateManifestSources(nil, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions) == nil {
		t.Fatal("revoked candidate gate admitted")
	}
	var cold *RuntimeOwnerInvariantSourceGate
	if err = cold.ValidateCandidateRequests(r.CandidateRequestSessions); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			local := newCandidateSourceTestSession()
			if err := candidateSourceSessionConsumers(r, local); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

func TestCandidateSourceNestedLineageKeepsCompleteColdGates(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 1)
	dispatch := r.CompletedDispatches[0]
	dispatch.Transaction.AuthoritySnapshotID = StableAuthoritySnapshotIDForTransaction(dispatch.WorkspaceID, dispatch.RunID, dispatch.Transaction)
	dispatch.ID = StableRuntimeDispatchID(dispatch.Transaction, dispatch.CandidateHash)
	r.CandidateRequestSessions[0].Transaction = dispatch.Transaction
	r.CandidateAttempts[0].Transaction = dispatch.Transaction
	r.CandidateAttempts[0].DispatchID = dispatch.ID
	r.CandidateAttempts[0].AuthorityContractID = StableAuthorityContractID(dispatch.WorkspaceID, dispatch.RunID, dispatch.Transaction)
	execution := &dispatch.OperationExecutions[0]
	execution.OperationID = dispatch.Candidate.StateChange.Operations[0].Operation.ID
	execution.OperationHash = StableRuntimeOperationHash(dispatch.Candidate.StateChange.Operations[0].Operation)
	execution.ExecutionKey = StableRuntimeOperationExecutionKey(dispatch.WorkspaceID, dispatch.RunID, dispatch.ID, execution.OperationID)
	execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(*execution)
	dispatch.OperationAuthorityHash = StableRuntimeOperationExecutionAuthoritiesHash(dispatch.OperationExecutions)
	dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
	r.CandidateAttempts[0].DispatchAuthorityHash = dispatch.DispatchAuthorityHash
	r.ActiveDispatch, r.CompletedDispatches = &dispatch, nil
	sources := historicalSessionArchiveSources(r)
	if err := runtimeValidateLineageOperationsInSemanticSession(sources, nil); err != nil {
		t.Fatal("invalid complete lineage fixture", err)
	}
	s := newCandidateSourceTestSession()
	if err := validateRuntimeCandidateManifestSourcesInSession(sources.Active, sources.Completed, sources.Attempts, sources.Sessions, s); err != nil {
		t.Fatal(err)
	}
	preimage := s.candidateSources.lineage
	for range 2 {
		if err := runtimeValidateLineageOperationsInSemanticSession(sources, s); err != nil {
			t.Fatal(err)
		}
		if s.candidateSources.lineage != preimage {
			t.Fatal("nested consumer rebuilt the identical complete manifest preimage")
		}
	}
	for arm := uint8(0); arm < 12; arm++ {
		changed := runtimeLineageOperationSourceRecord(sources).Clone()
		mutateLineageOperationSource(&changed, arm, "unchanged hash with raw source drift")
		if arm == 11 {
			changed.ActiveDispatch.CandidateManifestHash = "unchanged hash with raw source drift"
		}
		input := historicalSessionArchiveSources(changed)
		got := runtimeValidateLineageOperationsInSemanticSession(input, s)
		want := runtimeValidateLineageOperationsInSemanticSession(input, nil)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("nested source %d changed cold first error: %v != %v", arm, got, want)
		}
	}
}

// Explicit archived input supplements the built-in positive/negative fixtures;
// no live read, provider call or owner write occurs in this diagnostic test.
func TestCandidateSourceCompilerReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		if err := candidateSourceColdOracle(r); err != nil {
			t.Fatal(err)
		}
		s := newCandidateSourceTestSession()
		for range 2 {
			if err := candidateSourceSessionConsumers(r, s); err != nil {
				t.Fatal(err)
			}
		}
		if len(r.CandidateRequestSessions) == 0 || len(r.CandidateAttempts) == 0 || len(r.CompletedDispatches) == 0 {
			t.Fatal("archive has no candidate source history")
		}
		sources := historicalSessionArchiveSources(r)
		beforeLineage := s.candidateSources.lineage
		if err := runtimeValidateLineageOperationsInSemanticSession(sources, s); err != nil {
			t.Fatal(err)
		}
		if s.candidateSources.lineage != beforeLineage {
			t.Fatal("nested operation-lineage consumer rebuilt exact manifest sources")
		}
		bad := cloneRuntimeLineageSourcesRaw(sources)
		bad.Sessions[0].LogicalRequestID = "changed request identity"
		gotNested := runtimeValidateLineageOperationsInSemanticSession(bad, s)
		wantNested := runtimeValidateLineageOperationsInSemanticSession(bad, nil)
		if fmt.Sprint(gotNested) != fmt.Sprint(wantNested) {
			t.Fatalf("nested cold error %v != %v", gotNested, wantNested)
		}
		r.CandidateRequestSessions[0].StablePrefixHash = "same original hash, changed prefix"
		got, want := candidateSourceSessionConsumers(r, s), candidateSourceColdOracle(r)
		if got == nil || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("archive mutation first error %v != %v", got, want)
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkCandidateSourceCompleteArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for name, run := range runs {
		for _, mode := range []string{"cold_twice", "lexical_twice"} {
			b.Run(name+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					s := newCandidateSourceTestSession()
					for range 2 {
						var err error
						if mode == "cold_twice" {
							err = candidateSourceColdOracle(run.RuntimeState)
						} else {
							err = candidateSourceSessionConsumers(run.RuntimeState, s)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}

// Frozen prior collection consumer: full raw session validation followed by
// full normalization used only to obtain the identity for duplicate detection.
func referenceCandidateRequestCollection(values []RuntimeCandidateRequestSession) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("runtime candidate request session: %w", err)
		}
		value = value.Normalized()
		if _, found := seen[value.LogicalRequestID]; found {
			return fmt.Errorf("runtime candidate request session %q is duplicated", value.LogicalRequestID)
		}
		seen[value.LogicalRequestID] = struct{}{}
	}
	return nil
}

func TestCandidateRequestCollectionIdentityProjectionPreservesCompleteGate(t *testing.T) {
	base := candidateSourceCompilerFixture(t, 2).CandidateRequestSessions
	for _, arm := range []string{"valid", "empty", "duplicate", "normalized duplicate", "raw manifest", "wire", "multiple errors", "nil empty"} {
		t.Run(arm, func(t *testing.T) {
			values := cloneRuntimeStateRawSlice(base, cloneRuntimeCandidateRequestSessionRaw)
			switch arm {
			case "empty":
				values = nil
			case "duplicate":
				values = append(values, values[0])
			case "normalized duplicate":
				values[1] = cloneRuntimeCandidateRequestSessionRaw(values[0])
				values[1].LogicalRequestID = " \t" + values[1].LogicalRequestID + "\r\n"
			case "raw manifest":
				values[0].CandidateManifest.Bindings[0].DeclarationRef = "changed same hash"
			case "wire":
				values[0].ExpectedDocument.WireContractHash = "changed"
			case "multiple errors":
				values = append(values, values[0])
				values[0].NextSequence++
			case "nil empty":
				values[0].CandidateManifest.TargetRefs = []string{}
			}
			before := cloneRuntimeStateRawSlice(values, cloneRuntimeCandidateRequestSessionRaw)
			if got, want := ValidateRuntimeCandidateRequestSessions(values), referenceCandidateRequestCollection(values); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("first error %v != %v", got, want)
			}
			if !reflect.DeepEqual(before, values) {
				t.Fatal("collection validation changed original inputs")
			}
		})
	}
	old := testing.AllocsPerRun(10, func() {
		if err := referenceCandidateRequestCollection(base); err != nil {
			t.Fatal(err)
		}
	})
	current := testing.AllocsPerRun(10, func() {
		if err := ValidateRuntimeCandidateRequestSessions(base); err != nil {
			t.Fatal(err)
		}
	})
	if current >= old {
		t.Fatalf("identity projection retained unnecessary full clone: %v >= %v", current, old)
	}
}

func TestCandidateSourceNilGateDoesNotCompileUnusedPreimages(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 8)
	var gate *RuntimeOwnerInvariantSourceGate
	projected := testing.AllocsPerRun(20, func() {
		if err := gate.ValidateCandidateAttempts(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); err != nil {
			t.Fatal(err)
		}
		if err := gate.ValidateCandidateRequests(r.CandidateRequestSessions); err != nil {
			t.Fatal(err)
		}
		if err := gate.ValidateCandidateManifestSources(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions); err != nil {
			t.Fatal(err)
		}
	})
	compiled := testing.AllocsPerRun(20, func() {
		if err := candidateSourceSessionConsumers(r, newCandidateSourceTestSession()); err != nil {
			t.Fatal(err)
		}
	})
	// Compare against actually building all complete preimages. Exact cold
	// allocation equality is not a contract: map growth and race wrappers may
	// add small variation independently of any source clone.
	if projected >= compiled {
		t.Fatalf("nil gate did not avoid unused complete preimages: %v >= %v", projected, compiled)
	}
}

func newCandidateSourceTestSession() *runtimeSemanticValidationSession {
	s := newRuntimeSemanticValidationSession()
	s.candidateSources = &runtimeCandidateSourceCompiler{}
	return s
}

func TestCandidateSourceOrdinarySemanticSessionStaysCold(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 1)
	s := newRuntimeSemanticValidationSession()
	if err := candidateSourceSessionConsumers(r, s); err != nil {
		t.Fatal(err)
	}
	if s.candidateSources != nil {
		t.Fatal("ordinary cold validator retained candidate consumer preimages")
	}
}

func TestCandidateSourceOwnerTransactionReleasesLexicalProducts(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 1)
	for _, arm := range []string{"success", "error", "panic"} {
		t.Run(arm, func(t *testing.T) {
			s := newRuntimeSemanticValidationSession()
			func() {
				defer func() {
					if v := recover(); v != nil && arm != "panic" {
						panic(v)
					}
				}()
				err := compileRuntimeOwnerStateTransaction(func(func(RuntimeOwnerStateSourceImage) error) error {
					if s.candidateSources == nil {
						t.Fatal("owner transaction did not enable lexical consumers")
					}
					if err := candidateSourceSessionConsumers(r, s); err != nil {
						return err
					}
					if s.candidateSources.lineage == nil {
						t.Fatal("complete source result was not compiled")
					}
					switch arm {
					case "panic":
						panic("test owner unwind")
					case "error":
						return fmt.Errorf("test owner failure")
					}
					return nil
				}, s)
				if (err != nil) != (arm == "error") {
					t.Fatalf("transaction result %v", err)
				}
			}()
			if s.candidateSources != nil {
				t.Fatal("transaction return/error/panic retained candidate sources")
			}
		})
	}
}
