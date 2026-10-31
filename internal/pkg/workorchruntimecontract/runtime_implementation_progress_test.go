package workorchruntimecontract

import (
	"encoding/json"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"reflect"
	"strings"
	"testing"
)

func progressPurposeFixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument) {
	t.Helper()
	m := semanticWorkValidationManifest(t)
	w := m.ProofObligations[0].ImplementationWork
	w.PurposeChoiceOnly, w.EffectAvailable = true, true
	w.Scope.Request.RequestID = implementationWorkScopeRequestID(w.Scope.Request)
	w.Scope.Selection.PrerequisiteID = w.Scope.Request.RequestID
	w.Scope.Selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(w.Scope.Selection)
	w.Scope.Observation.PrerequisiteID = w.Scope.Request.RequestID
	w.Scope.Observation.ContentHash = w.Scope.Selection.SnapshotID
	w.Scope.Observation.ObservationID = StableRuntimePrerequisiteObservationID(w.Scope.Observation)
	w.PolicyHash = StableRuntimeImplementationWorkPolicyHash(w.Scope.Request.Policy)
	w.Scope.Content = &RuntimeImplementationWorkContent{ProtocolRef: RuntimeImplementationWorkContentV1, ManifestRevision: w.Scope.Selection.ObservedRevision}
	for _, target := range w.Scope.Selection.TargetRefs {
		w.Scope.Content.Snapshots = append(w.Scope.Content.Snapshots, workContentSnapshot(target, "unchanged original content\n"))
	}
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	decl := m.ProofObligations[0].DeclarationRef
	d := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: m.Hash, DefinitionSeedHash: m.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, ImplementationPurpose: RuntimeImplementationWorkPurposeEffect,
		ImplementationIntent: &RuntimeImplementationIntent{DeclarationRef: decl, ContentHash: w.Scope.Content.Hash(), RemainingWork: "implement the original missing behavior", Reason: "current original content does not implement it"}}
	for _, item := range m.CoverageItems {
		d.CoverageOrder = append(d.CoverageOrder, item.ID)
	}
	for _, ref := range RuntimeImplementationRequirementRefs(m, *w, decl) {
		d.ImplementationIntent.RequirementReviews = append(d.ImplementationIntent.RequirementReviews, RuntimeImplementationRequirementReview{RequirementRef: ref, WorkRequired: true})
		d.ImplementationIntent.SelectedRequirementRefs = append(d.ImplementationIntent.SelectedRequirementRefs, ref)
	}
	return m, d
}

func TestImplementationProgressExactWireReloadAndPurposeConsistency(t *testing.T) {
	m, d := progressPurposeFixture(t)
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, raw)
	if err != nil || StableRuntimeSemanticPlanDocumentHash(decoded) != StableRuntimeSemanticPlanDocumentHash(d) {
		t.Fatal("exact replay failed", err)
	}
	schema, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, "", 0, nil)
	if err != nil || !strings.Contains(schema, "requirementReviews") {
		t.Fatal("missing progress schema", err)
	}
	for name, mutate := range map[string]func(*RuntimeSemanticPlanDocument){
		"missing-review": func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent.RequirementReviews = nil },
		"foreign": func(v *RuntimeSemanticPlanDocument) {
			v.ImplementationIntent.RequirementReviews[0].RequirementRef = "foreign"
		},
		"duplicate": func(v *RuntimeSemanticPlanDocument) {
			v.ImplementationIntent.RequirementReviews = append(v.ImplementationIntent.RequirementReviews, v.ImplementationIntent.RequirementReviews[0])
		},
		"no-gap": func(v *RuntimeSemanticPlanDocument) {
			v.ImplementationIntent.RequirementReviews[0].WorkRequired = false
		},
		"no-selection": func(v *RuntimeSemanticPlanDocument) { v.ImplementationIntent.SelectedRequirementRefs = nil },
		"duplicate-selection": func(v *RuntimeSemanticPlanDocument) {
			v.ImplementationIntent.SelectedRequirementRefs = append(v.ImplementationIntent.SelectedRequirementRefs, v.ImplementationIntent.SelectedRequirementRefs[0])
		},
		"contradictory-terminal": func(v *RuntimeSemanticPlanDocument) {
			v.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := d.Clone()
			mutate(&v)
			if v.ValidateAgainstManifest(m) == nil {
				t.Fatal("inconsistent progress accepted")
			}
			if err := d.ValidateAgainstManifest(m); err != nil {
				t.Fatal("clone aliases original", err)
			}
		})
	}
	d.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
	d.ImplementationIntent.SelectedRequirementRefs = nil
	for n := range d.ImplementationIntent.RequirementReviews {
		d.ImplementationIntent.RequirementReviews[n].WorkRequired = false
	}
	if err := d.ValidateAgainstManifest(m); err != nil {
		t.Fatal("independent validation purpose rejected", err)
	}
	raw, _ = json.Marshal(d)
	var loaded RuntimeSemanticPlanDocument
	if json.Unmarshal(raw, &loaded) != nil || loaded.ValidateAgainstManifest(m) != nil {
		t.Fatal("restart changed progress")
	}
}

func TestImplementationProgressCoversCompleteOriginalCheckSet(t *testing.T) {
	m, d := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	// The public document test above exercises the complete manifest gate. Here
	// isolate set coverage without inventing a different validated inventory.
	m.CoverageItems[0].SourceObligationChecks = []RuntimeSourceObligationCheck{{CheckRef: "original-a"}, {CheckRef: "original-b"}}
	refs := RuntimeImplementationRequirementRefs(m, *w, d.ImplementationIntent.DeclarationRef)
	if !reflect.DeepEqual(refs, []string{"original-a", "original-b"}) {
		t.Fatal(refs)
	}
	d.ImplementationIntent.RequirementReviews = []RuntimeImplementationRequirementReview{{RequirementRef: "original-a", WorkRequired: true}, {RequirementRef: "original-b"}}
	d.ImplementationIntent.SelectedRequirementRefs = []string{"original-a"}
	if err := validateRuntimeImplementationProgress(d, m, w, d.ImplementationIntent.DeclarationRef); err != nil {
		t.Fatal(err)
	}
	d.ImplementationIntent.RequirementReviews = d.ImplementationIntent.RequirementReviews[:1]
	if validateRuntimeImplementationProgress(d, m, w, d.ImplementationIntent.DeclarationRef) == nil {
		t.Fatal("omitted original accepted")
	}
}

func TestImplementationProgressWireRequiresExplicitBoolean(t *testing.T) {
	m, d := progressPurposeFixture(t)
	d.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
	d.ImplementationIntent.SelectedRequirementRefs = nil
	for n := range d.ImplementationIntent.RequirementReviews {
		d.ImplementationIntent.RequirementReviews[n].WorkRequired = false
	}
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []string{"", `,"workRequired":null`, `,"workRequired":"false"`} {
		bad := strings.Replace(string(raw), `,"workRequired":false`, replacement, 1)
		if bad == string(raw) {
			t.Fatal("test did not mutate workRequired")
		}
		if _, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, []byte(bad)); err == nil {
			t.Fatal("missing/null/wrong-type interpreted as no remaining gap")
		}
	}
}

func TestImplementationProgressSchemaMatchesPurposeAndRequiredShape(t *testing.T) {
	m, d := progressPurposeFixture(t)
	rawSchema, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(rawSchema))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://workorch.invalid/progress.json", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("https://workorch.invalid/progress.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil || schema.Validate(v) != nil {
		t.Fatal("positive grammar mismatch", err, schema.Validate(v))
	}
	var base map[string]any
	json.Unmarshal(raw, &base)
	for _, kind := range []string{"missing-boolean", "terminal-gap", "selected-no-gap", "no-selection", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			var v map[string]any
			json.Unmarshal(raw, &v)
			intent := v["implementationIntent"].(map[string]any)
			reviews := intent["requirementReviews"].([]any)
			review := reviews[0].(map[string]any)
			switch kind {
			case "missing-boolean":
				delete(review, "workRequired")
			case "terminal-gap":
				v["implementationPurpose"] = string(RuntimeImplementationWorkPurposeTerminal)
			case "selected-no-gap":
				review["workRequired"] = false
			case "no-selection":
				delete(intent, "selectedRequirementRefs")
			case "foreign":
				review["requirementRef"] = "foreign"
			}
			if schema.Validate(v) == nil {
				t.Fatal("schema permits contradictory/incomplete purpose")
			}
		})
	}
	d.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
	d.ImplementationIntent.SelectedRequirementRefs = nil
	for n := range d.ImplementationIntent.RequirementReviews {
		d.ImplementationIntent.RequirementReviews[n].WorkRequired = false
	}
	raw, err = EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	v, err = jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil || schema.Validate(v) != nil {
		t.Fatal("terminal grammar mismatch", err, schema.Validate(v))
	}
}

func TestImplementationProgressRequiresOwnerContentWithoutProfileVersionSwitch(t *testing.T) {
	m, d := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	w.Scope.Content = nil
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	if _, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, "", 0, nil); err == nil {
		t.Fatal("missing current owner content reached provider schema")
	}
	m, d = progressPurposeFixture(t)
	if err := validateRuntimeImplementationProgress(d, m, m.ProofObligations[0].ImplementationWork, d.ImplementationIntent.DeclarationRef); err != nil {
		t.Fatal("current progress contract must not require a Profile version switch", err)
	}
}
