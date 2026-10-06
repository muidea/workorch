package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestImplementationWorkProspectiveTargetParametersUseFinalPolicy(t *testing.T) {
	decision, owner := targetParameterTestInputs(t, RuntimeBlockKindPatchApply, "file_write", []string{"new.txt"})
	catalog := owner.OperationAdmissibilities[0].Clone()
	catalog.ConstraintRefs = []string{"file_regular", "within_workspace"}
	catalog.SelectorRefs = []string{"work-scope"}
	catalog.OperationVariants = []RuntimeOperationAdmissibilityVariant{{Kind: RuntimeBlockKindPatchApply, VariantID: "file_write", AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: catalog.SelectorRefs, ConstraintRefs: catalog.ConstraintRefs}}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	binding := RuntimeImplementationWorkingBinding{
		OwnerCatalog:       catalog,
		CatalogObservation: RuntimePrerequisiteObservation{ObservationID: "work-catalog-observation"},
		Proposals:          []RuntimeSemanticProofProposal{{OperationKind: RuntimeBlockKindPatchApply, OperationVariantID: "file_write", Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"work-scope"}, ProspectiveConstraintRefs: catalog.ConstraintRefs, ProspectiveTargetRefs: []string{"new.txt"}}}},
	}
	binding.Hash = StableRuntimeImplementationWorkingBindingHash(binding)
	decision.RequiredOutput.CoverageWorkItemIDs = nil
	decision.RequiredOutput.ImplementationWorkBindingHash = binding.Hash
	decision.RequiredOutput.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	decision.RequiredOutput.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: RuntimeBlockKindPatchApply, VariantID: "file_write"}}
	decision.RequiredOutput.RequiredOperationSequence = []RuntimeRequiredOperationVariant{{RequirementID: "work-new", Kind: RuntimeBlockKindPatchApply, VariantID: "file_write", TargetRef: "new.txt"}}
	owner.OperationAdmissibilities = []RuntimeOperationAdmissibility{catalog}
	owner.ImplementationWorkingBinding = &binding
	manifest, err := BuildRuntimeCandidateContractManifest(decision, owner)
	if err != nil {
		t.Fatalf("prospective work target checked as existing before final policy: %v", err)
	}
	if _, err := BuildRuntimeCandidateWireContract(manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations) != 1 || len(manifest.Operations[0].TargetParameterBindings) != 1 || len(manifest.Operations[0].TargetParameterBindings[0].Values) != 0 {
		t.Fatal("prospective work invented an existing baseHash preimage")
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"requiredOperationRef":"work-new","parameters":[{"name":"content","string":"created"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, parameter := range candidate.StateChange.Operations[0].Operation.Parameters {
		if parameter.Name == "baseHash" {
			t.Fatal("prospective work operation fabricated a baseHash")
		}
	}
	if _, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"requiredOperationRef":"work-new","parameters":[{"name":"content","string":"created"},{"name":"baseHash","string":"invented"}]}]}`)); err == nil {
		t.Fatal("prospective work accepted a provider-authored preimage")
	}
}

func TestImplementationWorkMixedKindsKeepIndependentTargetPreimages(t *testing.T) {
	decision, owner := targetParameterTestInputs(t, StepKindFileEdit, "", []string{"session.txt", "session_manager.txt"})
	catalog := owner.OperationAdmissibilities[0].Clone()
	catalog.OperationKinds = []string{StepKindFileEdit, RuntimeBlockKindPatchApply}
	patchPreimages := catalog.TargetParameterBindings[0]
	patchPreimages.Kind = RuntimeBlockKindPatchApply
	catalog.TargetParameterBindings = append(catalog.TargetParameterBindings, patchPreimages)
	catalog = catalog.Normalized()
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	binding := RuntimeImplementationWorkingBinding{
		OwnerCatalog: catalog, CatalogObservation: RuntimePrerequisiteObservation{ObservationID: "work-catalog-observation"},
		Proposals: []RuntimeSemanticProofProposal{
			{OperationKind: StepKindFileEdit, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"session.txt"}}},
			{OperationKind: RuntimeBlockKindPatchApply, OperationVariantID: "text_replace", Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"session_manager.txt"}}},
		},
	}
	binding.Hash = StableRuntimeImplementationWorkingBindingHash(binding)
	decision.RequiredOutput.CoverageWorkItemIDs = nil
	decision.RequiredOutput.ImplementationWorkBindingHash = binding.Hash
	decision.RequiredOutput.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	decision.RequiredOutput.AcceptedKinds = []string{StepKindFileEdit, RuntimeBlockKindPatchApply}
	decision.RequiredOutput.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: StepKindFileEdit}, {Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace"}}
	decision.RequiredOutput.RequiredOperationSequence = []RuntimeRequiredOperationVariant{
		{RequirementID: "work-edit", Kind: StepKindFileEdit, TargetRef: "session.txt"},
		{RequirementID: "work-patch", Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace", TargetRef: "session_manager.txt"},
	}
	owner.OperationAdmissibilities = []RuntimeOperationAdmissibility{catalog}
	owner.ImplementationWorkingBinding = &binding
	manifest, err := BuildRuntimeCandidateContractManifest(decision, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRuntimeCandidateWireContract(manifest); err != nil {
		t.Fatalf("mixed work kinds retained another kind's target preimage: %v", err)
	}
	for _, operation := range manifest.Operations {
		if len(operation.TargetParameterBindings) != 1 || len(operation.TargetParameterBindings[0].Values) != 1 || len(operation.Target.AllowedTargetRefs()) != 1 || operation.TargetParameterBindings[0].Values[0].TargetRef != operation.Target.AllowedTargetRefs()[0] {
			t.Fatalf("kind-specific owner preimage drift: %+v", operation)
		}
	}
}

func TestImplementationWorkTargetParametersFollowFinalKindTarget(t *testing.T) {
	for _, kind := range []string{StepKindFileEdit, RuntimeBlockKindPatchApply} {
		t.Run(kind, func(t *testing.T) {
			variant := ""
			if kind == RuntimeBlockKindPatchApply {
				variant = "text_replace"
			}
			decision, owner := targetParameterTestInputs(t, kind, variant, []string{"session.txt", "session_manager.txt"})
			operation, err := runtimeCandidateOperationManifestForKind(kind, decision.State, decision.RequiredOutput, owner, RuntimeOperationContractRevision(), decision.Focus.CoverageEvidenceBindings)
			if err != nil {
				t.Fatal(err)
			}
			binding := RuntimeImplementationWorkingBinding{
				OwnerCatalog:       owner.OperationAdmissibilities[0],
				CatalogObservation: RuntimePrerequisiteObservation{ObservationID: "work-catalog-observation"},
				Proposals:          []RuntimeSemanticProofProposal{{OperationKind: kind, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"session.txt"}}}},
			}
			before := cloneRuntimeTargetParameterBindings(binding.OwnerCatalog.TargetParameterBindings)
			working, err := implementationWorkingOperationManifest(operation, binding)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateRuntimeCandidateTargetParameterBindings(working); err != nil {
				t.Fatalf("final singleton work target retained another kind's preimage: %v", err)
			}
			parameters, err := RuntimeCandidateTargetParameters(working, []string{"session.txt"})
			if err != nil || len(parameters) != 1 || parameters[0].Name != "baseHash" || parameters[0].String == nil || *parameters[0].String != "session-preimage" {
				t.Fatalf("exact owner baseHash derivation: %+v err=%v", parameters, err)
			}
			if !reflect.DeepEqual(before, binding.OwnerCatalog.TargetParameterBindings) {
				t.Fatal("work projection modified original owner catalog")
			}
		})
	}
}
