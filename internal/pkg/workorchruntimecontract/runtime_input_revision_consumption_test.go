package workorchruntimecontract

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestInputRevisionCurrentStateMatchesOriginalSourceRules(t *testing.T) {
	v, original := inputRevisionFixture(t, true)
	g := RuntimeSemanticPlanningGeneration{Reason: RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: 10, Snapshot: original}
	semantic := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{g}}
	definition := RuntimeDefinitionContext{Revision: v.Binding.DefinitionRevision}
	facts := RuntimeFacts{WorkspaceID: v.WorkspaceID, RunID: v.RunID, DefinitionSeed: RuntimeDefinitionSeed{Hash: v.DefinitionSeedHash},
		Focus:             CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{v.Binding.CoverageWorkItemID}},
		DefinitionFocuses: []RuntimeDefinitionFocus{{CoverageWorkItemIDs: []common.CoverageWorkItemID{v.Binding.CoverageWorkItemID}, ImplementationWorkPolicy: &RuntimeImplementationWorkPolicy{OwnerModuleID: v.OwnerModuleID, InputRevisionProtocolRef: RuntimeValidationInputRevisionProtocolV1}}},
	}
	scenarios := map[string]func(*RuntimeFacts, *RuntimeImplementationWorkLedger, *RuntimeSemanticPlanningLedger){
		"pending": func(*RuntimeFacts, *RuntimeImplementationWorkLedger, *RuntimeSemanticPlanningLedger) {},
		"completed": func(_ *RuntimeFacts, w *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			w.Rounds = []RuntimeImplementationWorkRound{{ScopeRequest: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}, CompletedReceiptSetHash: "real-collective"}}
		},
		"failed": func(_ *RuntimeFacts, w *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			w.Rounds = []RuntimeImplementationWorkRound{{ScopeRequest: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}, CompletedReceiptSetHash: "real-collective", FailedReceiptHash: "rejected"}}
		},
		"pending lost focus": func(f *RuntimeFacts, _ *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			f.Focus.CoverageWorkItemIDs = nil
		},
		"missing policy": func(f *RuntimeFacts, _ *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			f.DefinitionFocuses = nil
		},
		"wrong startup": func(f *RuntimeFacts, _ *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			f.DefinitionSeed.Hash = "foreign"
		},
		"duplicate consumption": func(_ *RuntimeFacts, w *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			r := RuntimeImplementationWorkRound{ScopeRequest: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}, CompletedReceiptSetHash: "real-collective"}
			w.Rounds = []RuntimeImplementationWorkRound{r, r}
		},
		"preview lost source": func(_ *RuntimeFacts, w *RuntimeImplementationWorkLedger, s *RuntimeSemanticPlanningLedger) {
			w.ScopePreview = &RuntimeImplementationWorkScopePreview{Request: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}}
			s.History = nil
		},
		"preview forged report": func(_ *RuntimeFacts, w *RuntimeImplementationWorkLedger, _ *RuntimeSemanticPlanningLedger) {
			bad := v.Clone()
			bad.Report.Checks[0].Reason = "forged"
			w.ScopePreview = &RuntimeImplementationWorkScopePreview{Request: RuntimeImplementationWorkScopeRequest{InputRevision: bad}}
		},
		"future report": func(_ *RuntimeFacts, _ *RuntimeImplementationWorkLedger, s *RuntimeSemanticPlanningLedger) {
			consumer := RuntimeSemanticPlanningGeneration{RetiredAtStateRevision: 9, Snapshot: RuntimeSemanticPlanningLedger{Manifest: &RuntimeSemanticPlanManifest{ProofObligations: []RuntimeSemanticProofObligation{{ImplementationWork: &RuntimeSemanticImplementationWork{Scope: RuntimeImplementationWorkScopeObservation{Request: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}}}}}}}}
			s.History = append(s.History, consumer)
		},
	}
	for name, mutate := range scenarios {
		t.Run(name, func(t *testing.T) {
			f, s, w := (RuntimeStateRecord{Facts: facts}).Clone().Facts, semantic.CloneRaw(), RuntimeImplementationWorkLedger{}
			mutate(&f, &w, &s)
			encoded, _ := json.Marshal(struct {
				F RuntimeFacts
				W RuntimeImplementationWorkLedger
				S RuntimeSemanticPlanningLedger
			}{f, w, s})
			got := validateRuntimeInputRevisionState(definition, f, w, s, newRuntimeSemanticValidationSession())
			want := beforeValidateRuntimeInputRevisionState(definition, f, w, s, newRuntimeSemanticValidationSession())
			if (got == nil) != (want == nil) {
				t.Fatalf("current applicability changed: got=%v want=%v", got, want)
			}
			current, err := PendingRuntimeValidationInputRevision(s, w, v.Binding.CoverageWorkItemID)
			previous, oldErr := beforePendingRuntimeValidationInputRevision(s, w, v.Binding.CoverageWorkItemID)
			if (err == nil) != (oldErr == nil) || (err == nil && !reflect.DeepEqual(current, previous)) {
				t.Fatalf("pending source changed: %v / %v", err, oldErr)
			}
			after, _ := json.Marshal(struct {
				F RuntimeFacts
				W RuntimeImplementationWorkLedger
				S RuntimeSemanticPlanningLedger
			}{f, w, s})
			if string(encoded) != string(after) {
				t.Fatal("source consumption mutated input")
			}
		})
	}
}

func TestInputRevisionFormalRequestStillRejectsChangedSource(t *testing.T) {
	v, l := inputRevisionFixture(t)
	l.StablePrefixHash = "prefix"
	doc := l.BatchDocuments[0]
	request := RuntimeCandidateRequestSession{LogicalRequestID: v.SourceLogicalRequestID, DocumentKind: RuntimeProviderDocumentSemanticPlan, ManifestHash: v.SourceManifestHash, StablePrefixHash: l.StablePrefixHash, State: RuntimeCandidateRequestSessionTerminal,
		ExpectedDocument: &RuntimeExpectedDocumentContract{LogicalRequestID: v.SourceLogicalRequestID, ManifestHash: v.SourceManifestHash, DefinitionSeedHash: doc.DefinitionSeedHash, PlanStep: doc.PlanStep, BatchID: doc.BatchID, BatchSequence: doc.BatchSequence, BatchCoverageWorkItemIDs: doc.BatchCoverageWorkItemIDs},
		LastOutcome:      RuntimeCandidateRequestOutcome{Class: RuntimeCandidateRequestOutcomePrerequisiteStale, Code: RuntimeValidationInputRevisionRetirementOutcome, Signature: RuntimeValidationInputRevisionRetirementOutcome + "/" + v.SourceManifestHash + "/" + v.SourceLogicalRequestID},
	}
	semantic := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{Reason: RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: 10, Snapshot: l}}}
	original := RuntimeStateRecord{SemanticPlanning: semantic, CandidateRequestSessions: []RuntimeCandidateRequestSession{request}}
	session := newRuntimeSemanticValidationSession()
	session.committedPreimage = &original
	if err := validateRuntimeInputRevisionRequestSourcesInSession(semantic, original.CandidateRequestSessions, session); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*[]RuntimeCandidateRequestSession){
		"missing":   func(s *[]RuntimeCandidateRequestSession) { *s = nil },
		"duplicate": func(s *[]RuntimeCandidateRequestSession) { *s = append(*s, (*s)[0]) },
		"pending": func(s *[]RuntimeCandidateRequestSession) {
			(*s)[0].State = RuntimeCandidateRequestSessionDocumentComplete
		},
		"different manifest": func(s *[]RuntimeCandidateRequestSession) { (*s)[0].ManifestHash = "foreign" },
		"unsettled":          func(s *[]RuntimeCandidateRequestSession) { (*s)[0].ProviderSubmitted = true },
		"different criteria": func(s *[]RuntimeCandidateRequestSession) { (*s)[0].ExpectedDocument.DefinitionSeedHash = "foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := original.Clone().CandidateRequestSessions
			mutate(&changed)
			if err := validateRuntimeInputRevisionRequestSourcesInSession(semantic, changed, session); err == nil {
				t.Fatal("formal generation hid a changed request source")
			}
		})
	}
}

func TestInputRevisionConsumersDoNotReAdmitSemanticDocument(t *testing.T) {
	for name, blocked := range map[string]map[string]bool{
		"runtime_validation_input_revision.go": {"ValidateAgainstManifest": true},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if s, ok := call.Fun.(*ast.SelectorExpr); ok && blocked[s.Sel.Name] {
				t.Fatal("input revision consumer re-admits the full semantic document")
			}
			return true
		})
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "validateRuntimeInputRevisionState" {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "PendingRuntimeValidationInputRevision" {
						t.Fatal("state consumer recursively recompiles the full history per source")
					}
				}
				return true
			})
		}
	}
}

func TestInputRevisionPendingUsesCurrentFactsAfterCommittedSource(t *testing.T) {
	v, l := inputRevisionFixture(t)
	s := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{Reason: RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: 10, Snapshot: l}}}
	f := RuntimeFacts{WorkspaceID: v.WorkspaceID, RunID: v.RunID, DefinitionSeed: RuntimeDefinitionSeed{Hash: v.DefinitionSeedHash}, Focus: CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{v.Binding.CoverageWorkItemID}}, DefinitionFocuses: []RuntimeDefinitionFocus{{CoverageWorkItemIDs: []common.CoverageWorkItemID{v.Binding.CoverageWorkItemID}, ImplementationWorkPolicy: &RuntimeImplementationWorkPolicy{OwnerModuleID: v.OwnerModuleID, InputRevisionProtocolRef: RuntimeValidationInputRevisionProtocolV1}}}}
	previous := RuntimeStateRecord{Facts: f, SemanticPlanning: s}
	session := newRuntimeSemanticValidationSession()
	session.committedPreimage = &previous
	definition := RuntimeDefinitionContext{Revision: v.Binding.DefinitionRevision}
	if err := validateRuntimeInputRevisionState(definition, f, RuntimeImplementationWorkLedger{}, s, session); err != nil {
		t.Fatal(err)
	}
	f.Focus.CoverageWorkItemIDs = nil
	if err := validateRuntimeInputRevisionState(definition, f, RuntimeImplementationWorkLedger{}, s, session); err == nil {
		t.Fatal("formal source hid the loss of current pending focus")
	}
	w := RuntimeImplementationWorkLedger{Rounds: []RuntimeImplementationWorkRound{{ScopeRequest: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}, CompletedReceiptSetHash: "real-collective"}}}
	if err := validateRuntimeInputRevisionState(definition, f, w, s, session); err != nil {
		t.Fatal("completed effect retained stale pending focus", err)
	}
	w.Rounds[0].FailedReceiptHash = "rejected"
	if err := validateRuntimeInputRevisionState(definition, f, w, s, session); err == nil {
		t.Fatal("failed effect settled pending revision")
	}
}

func TestInputRevisionFormalObservationStillRequiresCurrentSource(t *testing.T) {
	v, facts := inputObservationFixture(t)
	semantic := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{
		Reason: RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: 10, OwnerInputRevision: v,
	}}}
	previous := RuntimeStateRecord{SemanticPlanning: semantic, Facts: facts}
	session := newRuntimeSemanticValidationSession()
	session.committedPreimage = &previous
	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"unchanged": func(*RuntimeStateRecord) {},
		"retired observation": func(r *RuntimeStateRecord) {
			r.Facts.Resources.RetiredPrerequisiteObservations = r.Facts.Resources.PrerequisiteObservations
			r.Facts.Resources.PrerequisiteObservations = nil
		},
		"invalidation retains source": func(r *RuntimeStateRecord) {
			o := v.OwnerObservation.Observation
			r.Facts.Resources.PrerequisiteObservations = nil
			r.PrerequisiteInvalidations = []RuntimePrerequisiteInvalidationRecord{{PrerequisiteIDs: []string{o.PrerequisiteID}, ObservationIDs: []string{o.ObservationID}}}
		},
		"missing observation": func(r *RuntimeStateRecord) { r.Facts.Resources.PrerequisiteObservations = nil },
		"changed observation": func(r *RuntimeStateRecord) {
			r.Facts.Resources.PrerequisiteObservations[0].ContentHash = "foreign"
		},
		"unrelated invalidation": func(r *RuntimeStateRecord) {
			r.Facts.Resources.PrerequisiteObservations = nil
			r.PrerequisiteInvalidations = []RuntimePrerequisiteInvalidationRecord{{PrerequisiteIDs: []string{"foreign"}, ObservationIDs: []string{v.OwnerObservation.Observation.ObservationID}}}
		},
		"changed original catalog": func(r *RuntimeStateRecord) {
			r.SemanticPlanning.History[0].OwnerInputRevision.OwnerObservation.Catalog.ObservedRevision = "foreign"
		},
		"semantic report substituted": func(r *RuntimeStateRecord) {
			r.SemanticPlanning.History[0].OwnerInputRevision.Report.SubjectHash = "invented"
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := previous.Clone()
			mutate(&r)
			body, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var restored RuntimeStateRecord
			if err := json.Unmarshal(body, &restored); err != nil {
				t.Fatal(err)
			}
			cold := ValidateRuntimeInputRevisionObservationSources(restored.SemanticPlanning, restored.Facts, restored.PrerequisiteInvalidations)
			formal := validateRuntimeInputRevisionObservationSourcesInSession(restored.SemanticPlanning, restored.Facts, restored.PrerequisiteInvalidations, session)
			if (formal == nil) != (cold == nil) {
				t.Fatalf("formal source hid a current owner change: formal=%v cold=%v", formal, cold)
			}
			wantPass := name == "unchanged" || name == "retired observation" || name == "invalidation retains source"
			if (formal == nil) != wantPass {
				t.Fatalf("current owner source disposition: %v", formal)
			}
			after, err := json.Marshal(restored)
			if err != nil || string(body) != string(after) {
				t.Fatal("observation consumption changed original facts")
			}
		})
	}
}
