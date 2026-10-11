package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func assertRuntimeValidationSurfaceSinglePass(t testing.TB, surface RuntimeValidationSurface) {
	t.Helper()
	before, err := json.Marshal(surface)
	if err != nil {
		t.Fatal(err)
	}
	want := referenceRuntimeValidationSurfaceValidate(surface)
	for range 2 {
		got := surface.Validate()
		if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
			t.Fatalf("surface 接受／拒绝顺序不等价: got=%v want=%v", got, want)
		}
	}
	after, _ := json.Marshal(surface)
	if string(before) != string(after) {
		t.Fatal("校验改写原 surface bytes")
	}
}

func TestRuntimeValidationSurfaceSinglePassMatchesIndependentReference(t *testing.T) {
	ready := runtimeExactProofCloneReadySurfaceFixture("recipe").Normalized()
	preparing := runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()
	for _, original := range []RuntimeValidationSurface{ready, preparing} {
		if err := original.Validate(); err != nil {
			t.Fatalf("合法 fixture 被拒绝: %v", err)
		}
		for _, scenario := range []string{"valid", "raw", "identity", "owner", "operation", "effect", "idempotency", "readiness", "applicability", "duplicate_check", "duplicate_proof", "definition", "recipe", "attestation", "constraint", "input_schema", "mixed", "cross_coverage_native"} {
			t.Run(string(original.Readiness)+"/"+scenario, func(t *testing.T) {
				value := original.Clone()
				switch scenario {
				case "raw":
					value.ID = " " + value.ID + " "
				case "identity":
					value.ID = ""
				case "owner":
					value.OwnerModuleID = "unowned"
				case "operation":
					value.OperationKind = StepKindFileRead
				case "effect":
					value.EffectScope = "unknown-effect"
				case "idempotency":
					value.IdempotencyClass = "unknown-idempotency"
				case "readiness":
					value.Readiness = "unknown-readiness"
				case "applicability":
					value.ProofApplicability = RuntimeValidationProofAuxiliary
				case "duplicate_check":
					value.CheckRefs = []string{"check", " check "}
				case "mixed":
					value.ProofRequirementPreparations = []RuntimeValidationProofPreparation{preparing.ProofRequirementPreparations[0].Clone()}
					value.ProofRequirementBindings = []RuntimeValidationProofRequirementBinding{ready.ProofRequirementBindings[0].Clone()}
				default:
					if len(value.ProofRequirementBindings) != 0 {
						proof := &value.ProofRequirementBindings[0]
						switch scenario {
						case "duplicate_proof":
							value.ProofRequirementBindings = append(value.ProofRequirementBindings, proof.Clone())
						case "definition":
							proof.DefinitionRevision = ""
						case "recipe":
							proof.ValidationRecipeID = "other"
						case "attestation":
							proof.CheckAttestations[0].RequiredCheckRef = "other"
						case "constraint":
							proof.RequiredCheckRefs = append(proof.RequiredCheckRefs, proof.RequiredCheckRefs[0])
						case "input_schema":
							value.InputSchemaHash = ""
						case "cross_coverage_native":
							other := proof.Clone()
							other.CoverageWorkItemID = "cwi-other"
							other.RequirementHash = "requirement-other"
							value.ProofRequirementBindings = append(value.ProofRequirementBindings, other)
						}
					} else {
						proof := &value.ProofRequirementPreparations[0]
						switch scenario {
						case "duplicate_proof":
							value.ProofRequirementPreparations = append(value.ProofRequirementPreparations, proof.Clone())
						case "definition":
							proof.DefinitionRevision = ""
						case "recipe":
							proof.ValidationRecipeID = "other"
						case "attestation":
							proof.CheckAttestations = append(proof.CheckAttestations, proof.CheckAttestations[0])
						case "constraint":
							proof.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Kind = "unknown"
						case "input_schema":
							proof.ProspectiveTargetSatisfactionConstraint.InputSchemaHash = "other"
						case "cross_coverage_native":
							other := proof.Clone()
							other.CoverageWorkItemID = "cwi-other"
							other.RequirementHash = "requirement-other"
							value.ProofRequirementPreparations = append(value.ProofRequirementPreparations, other)
						}
					}
				}
				if err := referenceRuntimeValidationSurfaceValidate(value); (err == nil) != (scenario == "valid" || scenario == "raw") {
					t.Fatalf("独立场景未命中预期: %s: %v", scenario, err)
				}
				assertRuntimeValidationSurfaceSinglePass(t, value)
				body, _ := json.Marshal(value)
				var reloaded RuntimeValidationSurface
				if err := json.Unmarshal(body, &reloaded); err != nil {
					t.Fatal(err)
				}
				assertRuntimeValidationSurfaceSinglePass(t, reloaded)
			})
		}
	}
	manifest, _, _ := semanticInputReviewV2Fixture(t)
	for _, catalog := range manifest.OwnerCatalogs {
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		for _, surface := range catalog.ValidationSurfaces {
			assertRuntimeValidationSurfaceSinglePass(t, surface)
			changed := surface.Clone()
			changed.ProofRequirementBindings[0].InputReview.Files[0].ContentHash = "different"
			assertRuntimeValidationSurfaceSinglePass(t, changed)
			if reflect.DeepEqual(changed, surface) {
				t.Fatal("嵌套变更未命中")
			}
		}
	}
	assertRuntimeValidationSurfaceSinglePass(t, RuntimeValidationSurface{})
}

func TestRuntimeValidationSurfaceSinglePassRevalidatesLaterMutation(t *testing.T) {
	for _, surface := range []RuntimeValidationSurface{runtimeExactProofCloneReadySurfaceFixture("recipe"), runtimeExactProofClonePreparingSurfaceFixture("recipe")} {
		if err := surface.Validate(); err != nil {
			t.Fatal(err)
		}
		if len(surface.ProofRequirementBindings) != 0 {
			surface.ProofRequirementBindings[0].RequirementHash = ""
		} else {
			surface.ProofRequirementPreparations[0].RequirementHash = ""
		}
		if err := surface.Validate(); err == nil {
			t.Fatal("后续调用复用了前次成功，未拒绝新 nested 变更")
		}
		assertRuntimeValidationSurfaceSinglePass(t, surface)
	}
}

func TestRuntimeValidationSurfaceSinglePassUnavailability(t *testing.T) {
	requirement := runtimeValidationProofRequirementForTest(t, "definition", RuntimeCoverageItem{}, "", "recipe", nil)
	surface := bindRuntimeValidationSurfaceForTest(runtimeExactProofCloneReadySurfaceFixture("recipe"), requirement)
	surface.ProofRequirementBindings, surface.CheckRefs = nil, nil
	surface.ProofRequirementUnavailabilities = []RuntimeValidationProofUnavailability{{
		Requirement: requirement, ValidationRecipeID: "recipe",
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{
			SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell,
			ObservedRevision: "owner-revision", OriginalClass: "validation_readiness", OriginalCode: "owner_input_invalid",
			OriginalMessage: "  exact owner cause\r\n原文  ", Signature: "owner-input/cwi", TargetRefs: []string{"checks/input"},
		},
	}}
	for name, mutate := range map[string]func(*RuntimeValidationSurface){
		"valid": func(s *RuntimeValidationSurface) {},
		"duplicate": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities = append(s.ProofRequirementUnavailabilities, s.ProofRequirementUnavailabilities[0])
		},
		"ready_and_unavailable": func(s *RuntimeValidationSurface) {
			s.ProofRequirementBindings = bindRuntimeValidationSurfaceForTest(*s, requirement).ProofRequirementBindings
		},
		"owner": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities[0].OriginalDiagnostic.OwnerModuleID = RuntimeModuleFile
		},
		"recipe": func(s *RuntimeValidationSurface) { s.ProofRequirementUnavailabilities[0].ValidationRecipeID = "other" },
		"definition": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities[0].Requirement.DefinitionRevision = "other"
		},
		"diagnostic":    func(s *RuntimeValidationSurface) { s.ProofRequirementUnavailabilities[0].OriginalDiagnostic = nil },
		"applicability": func(s *RuntimeValidationSurface) { s.ProofApplicability = RuntimeValidationProofAuxiliary },
	} {
		t.Run(name, func(t *testing.T) {
			value := surface.Clone()
			mutate(&value)
			if err := referenceRuntimeValidationSurfaceValidate(value); (err == nil) != (name == "valid") {
				t.Fatalf("unavailable 场景未命中预期: %s: %v", name, err)
			}
			assertRuntimeValidationSurfaceSinglePass(t, value)
			body, _ := json.Marshal(value)
			var reloaded RuntimeValidationSurface
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertRuntimeValidationSurfaceSinglePass(t, reloaded)
		})
	}
}

func FuzzRuntimeValidationSurfaceSinglePassNested(f *testing.F) {
	for selector := uint8(0); selector < 24; selector++ {
		f.Add(selector, "", uint8(0))
		f.Add(selector, "  changed\r\n原文  ", uint8(1))
	}
	f.Fuzz(func(t *testing.T, selector uint8, value string, preparing uint8) {
		var surface RuntimeValidationSurface
		if preparing%2 == 0 {
			surface = runtimeExactProofCloneReadySurfaceFixture("recipe").Normalized()
			proof := &surface.ProofRequirementBindings[0]
			switch selector % 12 {
			case 0:
				proof.DefinitionRevision = value
			case 1:
				proof.RequirementHash = value
			case 2:
				proof.ValidationRecipeID = value
			case 3:
				proof.CheckAttestations[0].RequiredCheckRef = value
			case 4:
				proof.RequiredCheckRefs = append(proof.RequiredCheckRefs, value)
			case 5:
				proof.CheckAttestations = append(proof.CheckAttestations, proof.CheckAttestations[0])
			case 6:
				surface.ProofRequirementBindings = append(surface.ProofRequirementBindings, proof.Clone())
			case 7:
				surface.CheckRefs = append(surface.CheckRefs, value)
			case 8:
				surface.InputSchemaHash = value
			case 9:
				surface.ID = value
			case 10:
				surface.ReadinessReason = value
			case 11:
				surface.ProofRequirementPreparations = runtimeExactProofClonePreparingSurfaceFixture("recipe").ProofRequirementPreparations
			}
		} else {
			surface = runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()
			proof := &surface.ProofRequirementPreparations[0]
			switch selector % 12 {
			case 0:
				proof.DefinitionRevision = value
			case 1:
				proof.RequirementHash = value
			case 2:
				proof.ValidationRecipeID = value
			case 3:
				proof.CheckAttestations[0].RequiredCheckRef = value
			case 4:
				proof.ProspectiveTargetSatisfactionConstraint.InputSchemaHash = value
			case 5:
				proof.CheckAttestations = append(proof.CheckAttestations, proof.CheckAttestations[0])
			case 6:
				surface.ProofRequirementPreparations = append(surface.ProofRequirementPreparations, proof.Clone())
			case 7:
				proof.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Kind = RuntimeProspectiveTargetPredicateKind(value)
			case 8:
				proof.ProspectiveTargetSatisfactionConstraint.ValidationSurfaceID = value
			case 9:
				surface.ID = value
			case 10:
				surface.ReadinessReason = value
			case 11:
				surface.ProofRequirementBindings = runtimeExactProofCloneReadySurfaceFixture("recipe").ProofRequirementBindings
			}
		}
		assertRuntimeValidationSurfaceSinglePass(t, surface)
	})
}

func FuzzRuntimeValidationSurfaceSinglePass(f *testing.F) {
	for _, surface := range []RuntimeValidationSurface{runtimeExactProofCloneReadySurfaceFixture("recipe"), runtimeExactProofClonePreparingSurfaceFixture("recipe"), {}} {
		body, _ := json.Marshal(surface)
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		var surface RuntimeValidationSurface
		if json.Unmarshal(body, &surface) != nil {
			return
		}
		assertRuntimeValidationSurfaceSinglePass(t, surface)
	})
}

func BenchmarkRuntimeValidationSurfaceSinglePass(b *testing.B) {
	for _, original := range []RuntimeValidationSurface{runtimeExactProofCloneReadySurfaceFixture("recipe").Normalized(), runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()} {
		b.Run(string(original.Readiness), func(b *testing.B) {
			for _, optimized := range []bool{false, true} {
				name := "reference"
				if optimized {
					name = "single_pass"
				}
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var err error
						if optimized {
							err = original.Validate()
						} else {
							err = referenceRuntimeValidationSurfaceValidate(original)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
