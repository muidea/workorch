package workorchruntimecontract

import "testing"

func TestRuntimeOperationCorrectionMetadataIsDescriptorAndContractDeclared(t *testing.T) {
	fileReadContract, found := RuntimeOperationContractForKind(StepKindFileRead)
	if !found {
		t.Fatal("file read contract missing")
	}
	fileReadDescriptor, found := RuntimeOperationDescriptorForKind(StepKindFileRead)
	if !found {
		t.Fatal("file read descriptor missing")
	}
	fileRead := RuntimeCandidateOperationManifest{
		Kind: StepKindFileRead, TargetCardinality: fileReadDescriptor.TargetCardinality, Parameters: fileReadContract.Parameters,
	}
	target, found, err := RuntimeOperationCorrectionFieldForRole(fileRead, RuntimeOperationCorrectionRoleTarget)
	if err != nil || !found || target.Source != RuntimeOperationCorrectionFieldOperation || target.WitnessDimension() != RuntimeCandidateCorrectionWitnessTarget || target.CandidatePath(3) != "$.operations[3].targetRefs" {
		t.Fatalf("descriptor-declared target metadata=%+v found=%v", target, found)
	}

	fileListContract, found := RuntimeOperationContractForKind(StepKindFileList)
	if !found {
		t.Fatal("file list contract missing")
	}
	fileListDescriptor, found := RuntimeOperationDescriptorForKind(StepKindFileList)
	if !found {
		t.Fatal("file list descriptor missing")
	}
	fileList := RuntimeCandidateOperationManifest{
		Kind: StepKindFileList, TargetCardinality: fileListDescriptor.TargetCardinality, Parameters: fileListContract.Parameters,
	}
	if _, found, err := RuntimeOperationCorrectionFieldForRole(fileList, RuntimeOperationCorrectionRoleTarget); err != nil || found {
		t.Fatal("a capability path parameter was inferred as outer target metadata")
	}
	closed, err := RuntimeOperationCorrectionFieldsForRole(fileList, RuntimeOperationCorrectionRoleClosedParameter)
	if err != nil || len(closed) != 1 || closed[0].Name != "recursive" || closed[0].Source != RuntimeOperationCorrectionFieldParameter || closed[0].WitnessDimension() != RuntimeCandidateCorrectionWitnessClosedParameter {
		t.Fatalf("contract-declared closed metadata=%+v err=%v", closed, err)
	}

	validationContract, found := RuntimeOperationContractForKind(RuntimeBlockKindManagedValidationSession)
	if !found {
		t.Fatal("managed validation contract missing")
	}
	validation := RuntimeCandidateOperationManifest{Kind: RuntimeBlockKindManagedValidationSession, Parameters: validationContract.Parameters}
	recipe, found, err := RuntimeOperationCorrectionFieldForRole(validation, RuntimeOperationCorrectionRoleValidationRecipe)
	if err != nil || !found || recipe.Source != RuntimeOperationCorrectionFieldParameter || recipe.WitnessDimension() != RuntimeCandidateCorrectionWitnessValidationRecipe || recipe.CandidatePath(1) != "$.operations[1].parameters[name=recipeId]" {
		t.Fatalf("contract-declared recipe metadata=%+v found=%v", recipe, found)
	}
	bindingManifest := RuntimeCandidateOperationManifest{
		Kind: StepKindFileRead,
		BindingAuthorizations: []RuntimeOperationBindingAuthorization{{
			AllowedBindingRefs: []string{"binding/test/cwi-1"},
		}},
	}
	binding, found, err := RuntimeOperationCorrectionFieldForRole(bindingManifest, RuntimeOperationCorrectionRoleEvidenceBinding)
	if err != nil || !found || binding.Source != RuntimeOperationCorrectionFieldOperation || binding.WitnessDimension() != RuntimeCandidateCorrectionWitnessEvidenceBinding || binding.CandidatePath(2) != "$.operations[2].bindingRefs" {
		t.Fatalf("manifest-declared evidence-binding metadata=%+v found=%v err=%v", binding, found, err)
	}
}

func TestRuntimeOperationCorrectionMetadataRejectsAmbiguousOrIllTypedRoles(t *testing.T) {
	cases := []struct {
		name       string
		parameters []RuntimeOperationParameterContract
	}{
		{
			name: "closed string", parameters: []RuntimeOperationParameterContract{{
				Name: "mode", Type: RuntimeOperationParameterTypeString, CorrectionRole: RuntimeOperationCorrectionRoleClosedParameter,
			}},
		},
		{
			name: "duplicate recipe", parameters: []RuntimeOperationParameterContract{
				{Name: "recipeA", Type: RuntimeOperationParameterTypeString, CorrectionRole: RuntimeOperationCorrectionRoleValidationRecipe},
				{Name: "recipeB", Type: RuntimeOperationParameterTypeString, CorrectionRole: RuntimeOperationCorrectionRoleValidationRecipe},
			},
		},
		{
			name: "unknown role", parameters: []RuntimeOperationParameterContract{{
				Name: "mode", Type: RuntimeOperationParameterTypeString, CorrectionRole: RuntimeOperationCorrectionRole("unknown"),
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			operation := RuntimeCandidateOperationManifest{Kind: StepKindFileList, Parameters: tc.parameters}
			if _, err := RuntimeOperationCorrectionFields(operation); err == nil {
				t.Fatal("invalid correction metadata was accepted")
			}
			manifest := RuntimeCandidateContractManifest{
				Revision: RuntimeCandidateContractManifestRevision,
				State:    ConvergenceStateNeedValidation,
				EffectPolicy: RuntimeConvergenceEffectPolicy{
					State: ConvergenceStateNeedValidation, ExecutableOperationsAllowed: true,
					AllowedOperationEffects:       []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
					AllowedAcceptedReceiptEffects: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
				},
				Operations: []RuntimeCandidateOperationManifest{operation},
			}.Normalized()
			manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
			if err := ValidateRuntimeCandidateOperationCorrectionMetadata(manifest); err == nil {
				t.Fatal("manifest correction metadata validation accepted invalid role declaration")
			}
			if err := ValidateRuntimeCandidateAgainstManifest(manifest, RuntimeCandidate{}); err == nil {
				t.Fatal("candidate manifest admission did not fail closed on invalid correction metadata")
			}
		})
	}
}

func TestRuntimeCandidateManifestHashIncludesCorrectionRole(t *testing.T) {
	base := RuntimeCandidateContractManifest{
		Revision: RuntimeCandidateContractManifestRevision,
		State:    ConvergenceStateNeedValidation,
		Operations: []RuntimeCandidateOperationManifest{{
			Kind: StepKindFileList,
			Parameters: []RuntimeOperationParameterContract{{
				Name: "recursive", Type: RuntimeOperationParameterTypeBoolean,
			}},
		}},
	}.Normalized()
	changed := base.Clone()
	changed.Operations[0].Parameters[0].CorrectionRole = RuntimeOperationCorrectionRoleClosedParameter
	if StableRuntimeCandidateContractManifestHash(base) == StableRuntimeCandidateContractManifestHash(changed) {
		t.Fatal("candidate manifest hash ignored correction role metadata")
	}
}
