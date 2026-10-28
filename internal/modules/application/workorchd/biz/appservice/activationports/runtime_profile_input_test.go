package activationports

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	definitionadmission "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProfileInputAcceptsEveryBuiltinTemplate(t *testing.T) {
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		t.Run(template.ID, func(t *testing.T) {
			if err := validateRuntimePresealTargetDeclarations(template.ContextHints, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuiltinValidationGuidanceSurvivesTypedActivationAndReload(t *testing.T) {
	template := softwareDevProfileTemplateForTest(t)
	raw := template.ContextHints.Map()
	if err := validateRuntimePresealTargetDeclarations(raw, nil); err != nil {
		t.Fatal(err)
	}
	contract := runtimeExecutionContract(raw)
	if len(contract.ValidationPolicies) != 1 || !contract.ValidationPolicies[0].ValidationScratchRequired || contract.ValidationPolicies[0].ValidationSafetyClass != "bounded" {
		t.Fatal("typed execution contract lost effective scratch/bounded policy")
	}
	for _, expected := range []string{"isolated scratch", "bounded foreground", "do not use detached background", "must clean up"} {
		if !strings.Contains(strings.Join(contract.ValidationPolicies[0].Notes, "\n"), expected) {
			t.Fatalf("guidance %q disappeared before typed activation", expected)
		}
	}
	// Normal source serialization and a fresh adapter invocation must retain
	// the same policy, without backfilling the removed unused Profile object.
	encoded, err := json.Marshal(template.ContextHints)
	if err != nil {
		t.Fatal(err)
	}
	var restored workorcheventpayload.Payload
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimePresealTargetDeclarations(restored.Map(), nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract, runtimeExecutionContract(restored.Map())) {
		t.Fatal("reloaded source changed typed policy")
	}
}

func TestRuntimeProfileInputRejectsLossyCollectionShapes(t *testing.T) {
	for _, key := range []string{"acceptanceDimensions", "deliverableExpectations", "evidenceRequirements", "coverageDependencies", "bootstrapRecipes"} {
		for _, bad := range []any{nil, "not-an-array", []any{false}, []any{nil}, []map[string]any{nil}} {
			input := map[string]any{"profileContracts": map[string]any{"profile": map[string]any{key: bad}}}
			if err := validateRuntimePresealTargetDeclarations(input, nil); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("%s=%#v: got %v", key, bad, err)
			}
		}
	}
	for _, input := range []map[string]any{
		{"profileContracts": false},
		{"profileContracts": map[string]any{"profile": nil}},
		{"profileContracts": map[string]any{"profile": map[string]any{"bootstrapRecipes": []any{map[string]any{"artifacts": []any{false}}}}}},
		{"profileContracts": map[string]any{"profile": map[string]any{"evidenceRequirements": []any{map[string]any{"requiredOwnerFacts": []any{false}}}}}},
	} {
		if err := validateRuntimePresealTargetDeclarations(input, nil); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("malformed Profile input accepted: %#v, %v", input, err)
		}
	}
	for _, input := range []map[string]any{nil, {}, {"profileContracts": map[string]any{}}, {"profileContracts": map[string]any{"profile": map[string]any{"acceptanceDimensions": []any{}}}}} {
		if err := validateRuntimePresealTargetDeclarations(input, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeProfileInputRejectsLossyValuesAndUnknownFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile map[string]any
	}{
		{"required_string", map[string]any{"acceptanceDimensions": []any{map[string]any{"id": "required", "required": "true"}}}},
		{"required_null", map[string]any{"deliverableExpectations": []any{map[string]any{"id": "required", "required": nil}}}},
		{"quality_string", map[string]any{"evidenceRequirements": []any{map[string]any{"requireDirectEvidence": "true"}}}},
		{"mixed_checks", map[string]any{"evidenceRequirements": []any{map[string]any{"requiredCheckRefs": []any{"check", true}}}}},
		{"null_kinds", map[string]any{"evidenceRequirements": []any{map[string]any{"acceptedStepKinds": nil}}}},
		{"fractional_budget", map[string]any{"evidenceRequirements": []any{map[string]any{"ownerFactConvergences": []any{map[string]any{"maxAcquisitionAttempts": 1.75}}}}}},
		{"unknown_profile_contract", map[string]any{"workIterationPolicy": map[string]any{"maxRounds": 2}}},
		{"unknown_requirement_contract", map[string]any{"evidenceRequirements": []any{map[string]any{"implementationWorkConvergence": map[string]any{"maxRounds": 2}}}}},
		{"wrong_object_scope", map[string]any{"evidenceRequirements": []any{map[string]any{"acceptanceDimensions": []any{}}}}},
		{"misspelled_selector", map[string]any{"evidenceRequirements": []any{map[string]any{"targetSelecterRef": "required-selector"}}}},
		{"unknown_owner_fact", map[string]any{"evidenceRequirements": []any{map[string]any{"requiredOwnerFacts": []any{map[string]any{"state": "ready"}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"profileContracts": map[string]any{"profile": tc.profile}}
			if err := validateRuntimePresealTargetDeclarations(input, nil); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("unsupported or lossy Profile input accepted: %v", err)
			}
		})
	}
}

func TestRuntimeProfileInputShapeRejectsEveryWrongPrimitiveTypeWithoutMutation(t *testing.T) {
	for _, kind := range []string{"dimension", "deliverable", "dependency", "recipe", "artifact", "evidence", "variant", "capability", "ownerFact", "convergence"} {
		fields := runtimeProfileFields(kind)
		for _, group := range []struct {
			fields  string
			valid   any
			invalid []any
		}{
			{fields.text, "literal", []any{nil, true, 3, []any{"literal"}}},
			{fields.boolean, true, []any{nil, "true", 1, []any{true}}},
			{fields.stringList, []any{"one", "two"}, []any{nil, "one", []any{"one", nil}, []any{"one", 2}, []any{false}, []any{"one", ""}, []string{"one", " "}}},
			{fields.integer, float64(1), []any{nil, "1", true, 1.75, math.Inf(1), math.NaN(), math.Ldexp(1, strconv.IntSize-1)}},
		} {
			for _, key := range strings.Fields(group.fields) {
				t.Run(kind+"/"+key, func(t *testing.T) {
					object := map[string]any{key: group.valid}
					if err := validateRuntimeProfileObject(object, kind, kind); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(object, map[string]any{key: group.valid}) {
						t.Fatal("shape check mutated input")
					}
					for _, bad := range group.invalid {
						if err := validateRuntimeProfileObject(map[string]any{key: bad}, kind, kind); !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), kind+"."+key) {
							t.Fatalf("%T did not receive scoped invalid-input rejection: %v", bad, err)
						}
					}
				})
			}
		}
	}
}

func TestRuntimeProfileInputRejectsBeforeMergeAndFinalActivation(t *testing.T) {
	for _, field := range []string{"required", "unknown"} {
		for _, source := range []string{"template", "workspace"} {
			t.Run(field+"/"+source, func(t *testing.T) {
				builtin := softwareDevProfileTemplateForTest(t)
				valid := map[string]any{"id": "required-source", "required": true}
				invalid := map[string]any{"id": "required-source", "required": "true"}
				if field == "unknown" {
					invalid = map[string]any{"id": "required-source", "required": true, "workIterationPolicy": map[string]any{"maxRounds": 2}}
				}
				hints := func(item map[string]any) map[string]any {
					return map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{"acceptanceDimensions": []any{item}}}}
				}
				bad, err := workorcheventpayload.FromMap(hints(invalid))
				if err != nil {
					t.Fatal(err)
				}
				good, err := workorcheventpayload.FromMap(hints(valid))
				if err != nil {
					t.Fatal(err)
				}
				workspace := &workspacemodel.Workspace{ID: "workspace-profile", TemplateID: builtin.ID, ContextHints: good}
				builtin.ContextHints = bad
				if source == "workspace" {
					builtin.ContextHints, workspace.ContextHints = good, bad
				}
				before := workspace.ContextHints.Clone()
				if _, _, _, _, _, err := definitionAdmissionProfileBindings(workspace, builtin); !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("invalid %s policy disappeared before owner admission: %v", source, err)
				}
				if !reflect.DeepEqual(before, workspace.ContextHints) {
					t.Fatal("admission rewrote source payload")
				}
				_, err = RuntimeActivationContext(runtimecontract.ProviderInferenceSelection{}, "", nil, nil, appmodelcore.SoulRouting{}, nil, nil, nil, nil, nil, nil, hints(invalid), runtimecontract.RuntimeContinuationContext{})
				if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "profileContracts") {
					t.Fatalf("final activation did not independently reject Profile input: %v", err)
				}
			})
		}
	}
}

func TestDefinitionAdmissionRejectsDuplicateProfileObligationsBeforeCoverage(t *testing.T) {
	for _, source := range []string{"template", "workspace"} {
		t.Run(source, func(t *testing.T) {
			builtin := softwareDevProfileTemplateForTest(t)
			workspace := &workspacemodel.Workspace{ID: "workspace-profile", DeliverableDirectory: "/workspace", TemplateID: builtin.ID}
			hints := map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{"deliverableExpectations": []any{map[string]any{"id": "extra", "required": true}, map[string]any{"id": "extra", "required": false}}}}}
			payload, err := workorcheventpayload.FromMap(hints)
			if err != nil {
				t.Fatal(err)
			}
			if source == "template" {
				builtin.ContextHints = payload
			} else {
				workspace.ContextHints = payload
			}
			goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: workspace.ID, Text: "deliver", DefinitionRevision: "goal-r1"}
			dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: workspace.ID, Items: []string{"prove behavior"}, DefinitionRevision: "dod-r1"}
			candidate, err := definitionAdmissionCandidate(workspace, goal, dod, builtin)
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, p := range candidate.RuntimeProfileContracts {
				for _, d := range p.DeliverableExpectations {
					if d.ID == "extra" {
						found++
					}
				}
			}
			if found != 2 {
				t.Fatalf("source %s lost duplicate before owner: %d", source, found)
			}
			result, err := definitionadmission.Compile(candidate)
			if err != nil || result.Disposition == workspacemodel.DefinitionAdmissionAdmittedAllOf || result.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid {
				t.Fatalf("duplicate admitted: %+v %v", result, err)
			}
			// The final activation compiler independently rejects the same snapshot.
			activation := runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: candidate.RuntimeProfileContracts, AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{}}}
			if _, err := activationcore.RuntimeBootstrap(goal, dod, "definition", "startup", nil, activation); err == nil || !strings.Contains(err.Error(), "runtime profile proof closure") {
				t.Fatalf("bootstrap failed to reject Profile at its own gate: %v", err)
			}
		})
	}
}

func TestDefinitionProfileShapeIsCheckedBeforeConfigurationMerge(t *testing.T) {
	for _, source := range []string{"template", "workspace"} {
		t.Run(source, func(t *testing.T) {
			builtin := softwareDevProfileTemplateForTest(t)
			workspace := &workspacemodel.Workspace{ID: "workspace-profile", TemplateID: builtin.ID}
			bad, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"software-dev": map[string]any{"deliverableExpectations": []any{false}}}})
			if err != nil {
				t.Fatal(err)
			}
			if source == "workspace" {
				workspace.ContextHints = bad
			} else {
				workspace.ContextHints = builtin.ContextHints
				builtin.ContextHints = bad
			}
			if _, _, _, _, _, err := definitionAdmissionProfileBindings(workspace, builtin); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("malformed %s disappeared in merge: %v", source, err)
			}
		})
	}
}
