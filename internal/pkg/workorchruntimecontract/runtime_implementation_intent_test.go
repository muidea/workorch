package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func acceptedImplementationIntentFixture(t *testing.T) (RuntimeImplementationWorkRound, RuntimeSemanticPlanningLedger) {
	t.Helper()
	m, d := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	r := RuntimeImplementationWorkRound{WorkspaceID: m.WorkspaceID, RunID: m.RunID, DefinitionSeedHash: m.DefinitionSeedHash, CoverageWorkItemID: w.Scope.Request.CoverageWorkItemID, ScopeRequest: w.Scope.Request.Clone(), ScopeObservation: &w.Scope}
	s := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{Reason: RuntimeSemanticPlanningRetirementImplementationPurpose, Snapshot: RuntimeSemanticPlanningLedger{Phase: RuntimeSemanticPlanningOrderAccepted, Manifest: &m, CoverageOrderDocument: &d, DocumentHash: StableRuntimeSemanticPlanDocumentHash(d), LogicalRequestID: "original-purpose"}}}}
	return r, s
}

func TestImplementationIntentOriginalSourceJoinAndDetachedClone(t *testing.T) {
	r, s := acceptedImplementationIntentFixture(t)
	b, err := ResolveRuntimeImplementationWorkIntent(r, s)
	if err != nil {
		t.Fatal(err)
	}
	cloned := b.Clone()
	cloned.Intent.SelectedRequirementRefs[0] = "foreign"
	if reflect.DeepEqual(b, cloned) {
		t.Fatal("intent clone aliases source")
	}
	for name, mutate := range map[string]func(*RuntimeImplementationWorkRound, *RuntimeSemanticPlanningLedger){
		"missing": func(_ *RuntimeImplementationWorkRound, s *RuntimeSemanticPlanningLedger) { s.History = nil },
		"duplicate": func(_ *RuntimeImplementationWorkRound, s *RuntimeSemanticPlanningLedger) {
			s.History = append(s.History, s.History[0])
		},
		"workspace": func(r *RuntimeImplementationWorkRound, _ *RuntimeSemanticPlanningLedger) { r.WorkspaceID = "other" },
		"run":       func(r *RuntimeImplementationWorkRound, _ *RuntimeSemanticPlanningLedger) { r.RunID = "other" },
		"cwi": func(r *RuntimeImplementationWorkRound, _ *RuntimeSemanticPlanningLedger) {
			r.CoverageWorkItemID = "other"
		},
		"scope": func(r *RuntimeImplementationWorkRound, _ *RuntimeSemanticPlanningLedger) {
			r.ScopeRequest.RequestID = "other"
		},
		"content": func(r *RuntimeImplementationWorkRound, _ *RuntimeSemanticPlanningLedger) {
			r.ScopeObservation.Content.Snapshots[0].Content += "drift"
		},
		"unaccepted": func(_ *RuntimeImplementationWorkRound, s *RuntimeSemanticPlanningLedger) {
			s.History[0].Snapshot.Phase = RuntimeSemanticPlanningRequired
		},
		"document": func(_ *RuntimeImplementationWorkRound, s *RuntimeSemanticPlanningLedger) {
			s.History[0].Snapshot.CoverageOrderDocument.ImplementationIntent.RemainingWork = "tampered"
		},
	} {
		t.Run(name, func(t *testing.T) {
			rr, ss := r.Clone(), s.Clone()
			mutate(&rr, &ss)
			if _, err := ResolveRuntimeImplementationWorkIntent(rr, ss); err == nil {
				t.Fatal("foreign/missing original accepted")
			}
		})
	}
	// A fully valid different semantic proposal is distinguishable from the
	// original persisted binding, even if its author recomputes every hash.
	ss := s.Clone()
	d := ss.History[0].Snapshot.CoverageOrderDocument
	d.ImplementationIntent.RemainingWork = "another task"
	ss.History[0].Snapshot.DocumentHash = StableRuntimeSemanticPlanDocumentHash(*d)
	changed, err := ResolveRuntimeImplementationWorkIntent(r, ss)
	if err != nil || sameImplementationIntentBinding(b, changed) {
		t.Fatal("self-rehashed intent must not equal original", err)
	}
}

func reviewedImplementationPurposeFixture(t *testing.T, exhausted bool) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument) {
	t.Helper()
	m, d := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	r, s := acceptedImplementationIntentFixture(t)
	b, err := ResolveRuntimeImplementationWorkIntent(r, s)
	if err != nil {
		t.Fatal(err)
	}
	w.Ordinal = 2
	if exhausted {
		w.RepeatedWorkStates = w.Scope.Request.Policy.MaxRepeatedWorkStates
		w.Ordinal = w.RepeatedWorkStates + 1
	}
	w.EffectAvailable = !exhausted
	w.PrecedingReceiptSetHash = "previous-receipts"
	w.PreviousRound = &RuntimeImplementationRoundReviewSource{Ordinal: w.Ordinal - 1, DispatchID: "previous-dispatch", ReceiptSetHash: w.PrecedingReceiptSetHash, IntentBinding: b, Before: w.Scope.Content.Clone().Snapshots}
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	d.ManifestHash = m.Hash
	d.ImplementationIntent.PreviousRoundReview = &RuntimeImplementationRoundReview{DispatchID: w.PreviousRound.DispatchID, ReceiptSetHash: w.PrecedingReceiptSetHash, Outcome: "partial", Reason: "the owner edit changed another branch; original behavior still missing"}
	if exhausted {
		d.ImplementationPurpose = RuntimeImplementationWorkPurposeStalled
		d.ImplementationIntent.SelectedRequirementRefs = nil
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	return m, d
}

func assertImplementationSchemaAndParser(t *testing.T, m RuntimeSemanticPlanManifest, d RuntimeSemanticPlanDocument, want bool) {
	t.Helper()
	rawSchema, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(rawSchema))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("https://workorch.invalid/review.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("https://workorch.invalid/review.json")
	if err != nil {
		t.Fatal(err)
	}
	wire := map[string]any{"documentKind": RuntimeProviderDocumentSemanticPlan, "schemaVersion": d.SchemaVersion, "manifestHash": d.ManifestHash, "definitionSeedHash": d.DefinitionSeedHash, "planStep": d.PlanStep, "coverageOrder": d.CoverageOrder, "proposals": []any{}, "implementationPurpose": d.ImplementationPurpose, "implementationIntent": d.ImplementationIntent}
	encoded, _ := json.Marshal(wire)
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	schemaErr := schema.Validate(v)
	_, parseErr := DecodeRuntimeSemanticPlanPayloadWithManifest(m, encoded)
	if (schemaErr == nil) != want || (parseErr == nil) != want {
		t.Fatalf("schema/parser agreement want=%t schema=%v parser=%v", want, schemaErr, parseErr)
	}
}

func TestImplementationPreviousIntentReviewAndTruthfulBudgetOutcome(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		m, d := reviewedImplementationPurposeFixture(t, exhausted)
		assertImplementationSchemaAndParser(t, m, d, true)
		for _, outcome := range []string{"partial", "diverged"} {
			v := d.Clone()
			v.ImplementationIntent.PreviousRoundReview.Outcome = RuntimeImplementationRoundReviewOutcome(outcome)
			assertImplementationSchemaAndParser(t, m, v, true)
			v.ImplementationIntent.RequirementReviews[0].WorkRequired = false
			assertImplementationSchemaAndParser(t, m, v, false)
		}
		for name, mutate := range map[string]func(*RuntimeSemanticPlanDocument){
			"missing": func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent.PreviousRoundReview = nil },
			"stale": func(v *RuntimeSemanticPlanDocument) {
				v.ImplementationIntent.PreviousRoundReview.ReceiptSetHash = "other"
			},
			"unknown": func(v *RuntimeSemanticPlanDocument) {
				v.ImplementationIntent.PreviousRoundReview.Outcome = "inconclusive"
			},
		} {
			t.Run(name, func(t *testing.T) { v := d.Clone(); mutate(&v); assertImplementationSchemaAndParser(t, m, v, false) })
		}
		v := d.Clone()
		v.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
		v.ImplementationIntent.SelectedRequirementRefs = nil
		assertImplementationSchemaAndParser(t, m, v, true)
		v = d.Clone()
		v.ImplementationPurpose = RuntimeImplementationWorkPurposeInconclusive
		v.ImplementationIntent.SelectedRequirementRefs = nil
		v.ImplementationIntent.PreviousRoundReview.Outcome = "inconclusive"
		assertImplementationSchemaAndParser(t, m, v, true)
		v = d.Clone()
		v.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
		v.ImplementationIntent.SelectedRequirementRefs = nil
		v.ImplementationIntent.PreviousRoundReview.Outcome = "addressed"
		for n := range v.ImplementationIntent.RequirementReviews {
			v.ImplementationIntent.RequirementReviews[n].WorkRequired = false
		}
		assertImplementationSchemaAndParser(t, m, v, true)
		if exhausted {
			v = d.Clone()
			v.ImplementationPurpose = RuntimeImplementationWorkPurposeEffect
			v.ImplementationIntent.SelectedRequirementRefs = []string{v.ImplementationIntent.RequirementReviews[0].RequirementRef}
			assertImplementationSchemaAndParser(t, m, v, false)
		}
	}
}

func TestImplementationExhaustionDoesNotBecomeDefinitionTicket(t *testing.T) {
	m, _ := reviewedImplementationPurposeFixture(t, true)
	_, expected, gap := runtimeDefinitionGapDocumentForTest(t)
	expected.ManifestHash, expected.DefinitionSeedHash = m.Hash, m.DefinitionSeedHash
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	gap.ManifestHash, gap.DefinitionSeedHash = m.Hash, m.DefinitionSeedHash
	gap.OffendingItemRefs = []string{string(m.CoverageItems[0].ID)}
	gap.GapKind = RuntimeDefinitionGapSemanticAmbiguity
	original, _ := json.Marshal(gap)
	admission := EvaluateRuntimeDefinitionGapAdmission(m, expected, gap)
	if admission.Admitted() || admission.Code != "implementation_work_exhausted_review_missing" {
		t.Fatalf("internal exhaustion opened business ticket: %+v", admission)
	}
	after, _ := json.Marshal(gap)
	if string(after) != string(original) {
		t.Fatal("original diagnostic rewritten")
	}
	// A genuine alternatives gap remains the existing business clarification.
	gap.GapKind = RuntimeDefinitionGapImplicitOneOf
	if a := EvaluateRuntimeDefinitionGapAdmission(m, expected, gap); !a.Admitted() {
		t.Fatalf("genuine definition gap blocked: %+v", a)
	}
}

func TestImplementationReviewMaySelectAnotherUnitButCannotDropOriginalGap(t *testing.T) {
	m, d := reviewedImplementationPurposeFixture(t, false)
	work := m.ProofObligations[0].ImplementationWork
	d.ImplementationIntent.RequirementReviews = append(d.ImplementationIntent.RequirementReviews, RuntimeImplementationRequirementReview{RequirementRef: "other-original", WorkRequired: true})
	d.ImplementationIntent.SelectedRequirementRefs = []string{"other-original"}
	if err := validateImplementationRoundReview(d.ImplementationIntent, work); err != nil {
		t.Fatal("next bounded work unit was rejected despite retaining the original gap", err)
	}
	d.ImplementationIntent.RequirementReviews[0].WorkRequired = false
	if err := validateImplementationRoundReview(d.ImplementationIntent, work); err == nil {
		t.Fatal("unfulfilled original gap disappeared")
	}
	d.ImplementationIntent.RequirementReviews[0].WorkRequired = true
	if err := validateImplementationRoundReview(d.ImplementationIntent, work); err != nil {
		t.Fatal("necessary continuation was rejected", err)
	}
}

func TestImplementationObservationFailurePreservesOriginalReason(t *testing.T) {
	for _, purpose := range []RuntimeImplementationWorkPurpose{RuntimeImplementationWorkPurposeStalled, RuntimeImplementationWorkPurposeInconclusive} {
		reason := "  原始判断\n不改写也不截断  "
		choice := RuntimeImplementationWorkPurposeChoice{Purpose: purpose, SemanticManifestHash: "manifest", DocumentHash: "original-document"}
		failure := RuntimeImplementationWorkObservationFailure(choice, &RuntimeImplementationIntent{Reason: reason})
		cloned := failure.Clone()
		if cloned.OriginalDiagnostic == nil || cloned.OriginalDiagnostic.OriginalMessage != reason || cloned.OriginalDiagnostic.Signature != choice.DocumentHash {
			t.Fatal("failure authority normalization altered original semantic reason")
		}
		if err := cloned.OriginalDiagnostic.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
