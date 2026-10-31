package runtimeprofiledecode

import "testing"

func TestDecodePreservesNestedProfileContractValues(t *testing.T) {
	repairRoute := map[string]any{
		"id": "route", "requireInputAddition": true, "requireInputRepair": true,
		"triggerOwnerModuleId": "trigger", "triggerOperationKind": "operation", "triggerDiagnosticClass": "diagnostic",
		"triggerEvidenceTargetType": "target", "triggerOwnerDiagnosticCode": "code", "repairEvidenceKind": "repair",
		"repairOwnerModuleId": "repair-owner", "repairOperationKinds": []any{"operation"}, "repairTargetPolicy": "target",
		"repairTargetRole": "role", "repairDeclarationTargetSelectorRef": "declaration",
		"repairDeclarationTargetSelectorOwnerRef": "owner", "repairDeclarationTargetSelectorSubjectRef": "subject",
		"repairDeclarationTargetSelectorCandidateRefs": []any{"candidate"}, "repairOperationTargetSelectorRef": "operation",
	}
	implementation := map[string]any{
		"protocolRef": "protocol", "ownerModuleId": "owner", "inputRevisionProtocolRef": "validation/input-revision/v1",
		"allowedOperations":     []any{map[string]any{"kind": "operation", "variantId": "variant"}},
		"allowedTargetPolicies": []any{"existing"}, "targetSelectorRef": "selector", "targetSelectorOwnerRef": "selector-owner",
		"targetSelectorSubjectRef": "selector-subject", "targetSelectorCandidateRefs": []any{"candidate"},
		"prospectiveTargetAllOfConstraintRefs": []any{"constraint"}, "maxRoundsPerCWI": 2, "maxTargetsPerRound": 3,
		"requireFreshObservation": true, "requireFreshValidation": true, "protectValidationInputs": true,
	}
	convergence := map[string]any{
		"ownerModuleId": "owner", "factKind": "fact", "operationKind": "operation", "requirePreexisting": true,
		"missingTrigger": "missing", "acquisitionOwnerModuleId": "acq-owner", "acquisitionOperationKind": "acq",
		"acquisitionOperationVariantId": "variant", "acquisitionTargetPolicy": "target", "acquisitionTargetSelectorRef": "acq-selector",
		"prospectiveSatisfactionContractRef": "contract", "targetSelectorRef": "selector", "targetSelectorOwnerRef": "selector-owner",
		"targetSelectorSubjectRef": "selector-subject", "reobserveOwnerModuleId": "reobserve", "reobserveFactKind": "fact",
		"maxAcquisitionAttempts": 2, "dependsOnRequirementRefs": []any{"dependency"},
	}
	evidence := map[string]any{
		"id": "evidence", "scope": "dodItem", "evidenceRole": "role", "targetRole": "source", "targetRefs": []any{"ref"},
		"targetDiscoverySources": []any{"source"}, "targetSelectorRef": "selector", "targetSelectorOwnerRef": "owner", "targetSelectorSubjectRef": "subject",
		"targetSelectorCandidateRefs": []any{"candidate"}, "prospectiveTargetAllOfConstraintRefs": []any{"constraint"}, "requireTargetGrounding": true,
		"requiredEvidenceKinds": []any{"validation"}, "requireDirectEvidence": true, "requireClaimEvidence": true,
		"requireExecutableBehaviorEvidence": true, "requireCoverageTargets": true, "acceptedStepKinds": []any{"step"},
		"acceptedOperationVariants": []any{map[string]any{"kind": "operation", "variantId": "variant"}}, "validationRecipeId": "recipe",
		"requiredCheckRefs": []any{"check"}, "metricConstraints": []any{"metric"}, "artifactRefs": []any{"artifact"},
		"requiredOwnerFacts":                 []any{map[string]any{"ownerModuleId": "owner", "factKind": "fact", "operationKind": "operation"}},
		"ownerFactConvergences":              []any{convergence},
		"validationFailureRepairConvergence": map[string]any{"id": "repair", "routes": []any{repairRoute}, "requireFreshValidation": true, "maxAttempts": 3},
		"implementationWorkPolicy":           implementation,
		"capabilityStateRequirements":        []any{map[string]any{"ownerModuleId": "owner", "capabilityStateKind": "state", "subjectRef": "subject"}},
		"rejectedStepKinds":                  []any{"reject"}, "description": "evidence description",
	}
	profile := map[string]any{
		"acceptanceDimensions":    []any{map[string]any{"id": "dimension", "required": true, "description": "describe"}},
		"deliverableExpectations": []any{map[string]any{"id": "deliverable", "required": true, "evidenceRole": "role", "description": "output"}},
		"coverageDependencies":    []any{map[string]any{"id": "dep", "sourceScope": "goal", "sourceRequirementId": "source", "dependentScope": "dodItem", "dependentRequirementId": "dependent", "requireSourceSettled": true, "triggerEvidenceKinds": []any{"kind"}}},
		"bootstrapRecipes":        []any{map[string]any{"id": "recipe", "requiredGoalTerms": []any{"goal"}, "requiredDoDTerms": []any{"dod"}, "artifacts": []any{map[string]any{"id": "artifact", "targetRole": "target", "allowedOperationKinds": []any{"write"}, "dependsOnArtifactIds": []any{"prior"}}}}},
		"evidenceRequirements":    []any{evidence},
	}
	input := map[string]any{"profileContracts": map[string]any{"profile-a": profile}}
	got := Decode(input["profileContracts"])
	if len(got) != 1 || got[0].ID != "profile-a" || len(got[0].AcceptanceDimensions) != 1 || len(got[0].DeliverableExpectations) != 1 || len(got[0].CoverageDependencies) != 1 || len(got[0].BootstrapRecipes) != 1 || len(got[0].EvidenceRequirements) != 1 {
		t.Fatalf("Profile fields lost: %+v", got)
	}
	decodedEvidence := got[0].EvidenceRequirements[0]
	if decodedEvidence.ValidationFailureRepairConvergence == nil || decodedEvidence.ImplementationWorkPolicy == nil ||
		decodedEvidence.ValidationFailureRepairConvergence.Routes[0].RepairDeclarationTargetSelectorCandidateRefs[0] != "candidate" ||
		decodedEvidence.ImplementationWorkPolicy.AllowedOperations[0].VariantID != "variant" ||
		decodedEvidence.ImplementationWorkPolicy.InputRevisionProtocolRef != "validation/input-revision/v1" ||
		got[0].BootstrapRecipes[0].Artifacts[0].DependsOnArtifactIDs[0] != "prior" ||
		decodedEvidence.OwnerFactConvergences[0].MaxAcquisitionAttempts != 2 {
		t.Fatalf("nested Profile policies lost: %+v", got)
	}
	if err := ValidateInputShape(input); err != nil {
		t.Fatalf("valid Profile rejected: %v", err)
	}
	if decodedEvidence.ValidationFailureRepairConvergence.Routes[0].TriggerDiagnosticClass != "diagnostic" {
		t.Fatal("nested repair policy field was lost")
	}
}

func TestMergeTemplateWorkspaceProfileContractClonesAndOverlaysByIdentity(t *testing.T) {
	template := map[string]any{"evidenceRequirements": []any{
		map[string]any{"scope": "dodItem", "id": "same", "description": "template"},
		map[string]any{"scope": "goal", "id": "kept"},
	}}
	workspace := map[string]any{"evidenceRequirements": []any{
		map[string]any{"scope": "dodItem", "id": "same", "description": "workspace"},
		map[string]any{"scope": "dodItem", "id": "new"},
	}}
	merged := MergeTemplateWorkspaceProfileContract(template, workspace)
	items := MapSlice(merged["evidenceRequirements"])
	if len(items) != 3 || items[0]["id"] != "new" || items[1]["description"] != "workspace" || items[2]["id"] != "kept" {
		t.Fatalf("unexpected merge: %#v", items)
	}
	items[0]["description"] = "changed"
	if MapSlice(template["evidenceRequirements"])[0]["description"] != "template" {
		t.Fatal("merge result aliases template input")
	}
}

func TestProfileShapeRejectsRetiredProgressVersionSwitch(t *testing.T) {
	for _, field := range []string{"progressProtocolRef", "schemaVersion", "compatibilityMode"} {
		t.Run(field, func(t *testing.T) {
			work := map[string]any{field: "implementation-work-progress/v1"}
			input := map[string]any{"profileContracts": map[string]any{"project": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "work", "implementationWorkPolicy": work}}}}}
			if err := ValidateInputShape(input); err == nil {
				t.Fatalf("retired management field %s accepted", field)
			}
		})
	}
}
