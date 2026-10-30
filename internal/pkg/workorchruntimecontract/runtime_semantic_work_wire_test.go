package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func workWireFixture(t *testing.T, prospective bool) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument) {
	t.Helper()
	m := semanticWorkValidationManifest(t)
	c := &m.OwnerCatalogs[0]
	c.SelectorRefs = []string{"native-existing", "native-new"}
	w := m.ProofObligations[0].ImplementationWork
	c.ConstraintRefs = append([]string(nil), w.Scope.Request.Policy.ProspectiveTargetAllOfConstraintRefs...)
	c.OperationVariants = []RuntimeOperationAdmissibilityVariant{{Kind: StepKindFileWrite, AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: c.SelectorRefs, ConstraintRefs: c.ConstraintRefs}}
	*c = c.Normalized()
	c.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*c)
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	p := RuntimeSemanticProofProposal{CoverageWorkItemID: m.CoverageItems[0].ID, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite, DeclarationRef: RuntimeSemanticCurrentDeclarationRef(m.ProofObligations[0]), RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"resource-a"}}}
	if prospective {
		p.Target = RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"native-new"}, ProspectiveTargetRefs: []string{"resource-b"}, ProspectiveConstraintRefs: c.ConstraintRefs}
	}
	d := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: m.Hash, DefinitionSeedHash: m.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "work-batch", BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{p.CoverageWorkItemID}, Proposals: []RuntimeSemanticProofProposal{p}}
	return m, d
}

func TestSemanticWorkRealWireSchemaDecoderAndEvaluator(t *testing.T) {
	for _, prospective := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "prospective"}[prospective], func(t *testing.T) {
			m, d := workWireFixture(t, prospective)
			before, _ := json.Marshal(m)
			rawSchema, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, d.BatchID, d.BatchSequence, d.BatchCoverageWorkItemIDs)
			if err != nil {
				t.Fatal(err)
			}
			schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(rawSchema))
			if err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("https://workorch.invalid/work.json", schemaDoc); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile("https://workorch.invalid/work.json")
			if err != nil {
				t.Fatal(err)
			}
			wire, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
			if err != nil {
				t.Fatal(err)
			}
			value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(wire)))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); err != nil {
				t.Fatalf("actual wire rejected: %v\n%s", err, wire)
			}
			decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := decoded.ValidateAgainstManifest(m); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.Proposals[0].RequiredEffectScopes, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}) || strings.Contains(string(wire), "requiredEffectScopes") {
				t.Fatal("effect derivation leaked into wire")
			}
			mutations := map[string]func(map[string]any){
				"internal effect": func(p map[string]any) { p["requiredEffectScopes"] = []any{"workspace_durable"} },
				"scope selector": func(p map[string]any) {
					p["target"].(map[string]any)["ownerSelectorRefs"] = []any{m.ProofObligations[0].ImplementationWork.Scope.Request.Policy.TargetSelectorRef}
				},
				"wrong owner":       func(p map[string]any) { p["ownerModuleId"] = RuntimeModuleShell },
				"unknown operation": func(p map[string]any) { p["operationKind"] = "foreign/op" },
				"cross CWI":         func(p map[string]any) { p["coverageWorkItemId"] = "other-cwi" },
			}
			if prospective {
				mutations["missing all-of"] = func(p map[string]any) {
					p["target"].(map[string]any)["prospectiveConstraintRefs"] = []any{"constraint-a"}
				}
			} else {
				mutations["outside observed scope"] = func(p map[string]any) { p["target"].(map[string]any)["ownerScopeTargetRefs"] = []any{"resource-b"} }
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					var v map[string]any
					_ = json.Unmarshal([]byte(wire), &v)
					mutate(v["proposals"].([]any)[0].(map[string]any))
					if schema.Validate(v) == nil {
						t.Fatal("schema admitted invalid wire")
					}
					b, _ := json.Marshal(v)
					got, e := DecodeRuntimeSemanticPlanPayloadWithManifest(m, b)
					if e == nil && got.ValidateAgainstManifest(m) == nil {
						t.Fatal("decoder/evaluator admitted invalid wire")
					}
				})
			}
			after, _ := json.Marshal(m)
			if string(before) != string(after) {
				t.Fatal("compiler mutated source")
			}
		})
	}
}

func TestSemanticWorkRequestContractRejectsEmptyOwnerIntersection(t *testing.T) {
	m, d := workWireFixture(t, false)
	m.OwnerCatalogs[0].OperationVariants[0].AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget}
	m.OwnerCatalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(m.OwnerCatalogs[0])
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	var conflict *RuntimeSemanticRequestContractError
	if err := ValidateRuntimeSemanticRequestContract(m, d.PlanStep, d.BatchCoverageWorkItemIDs); !errors.As(err, &conflict) {
		t.Fatalf("expected typed technical conflict: %v", err)
	}
	if _, err := RuntimeSemanticPlanJSONSchemaForRequest(m, d.PlanStep, d.BatchID, 0, d.BatchCoverageWorkItemIDs); !errors.As(err, &conflict) {
		t.Fatalf("schema did not run same gate: %v", err)
	}
}
