package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func historicalAcceptanceSemanticFixture(t *testing.T) (RuntimeSourceObligationInventory, RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, []RuntimeSemanticProofFeasibility) {
	t.Helper()
	manifest, document, _, source := semanticIndependentProofSetFixture(t)
	inventory, _, items := evidencePurposeMappingFixture()
	inventory.DefinitionRevision = manifest.DefinitionRevision
	inventory = inventory.WithHash()
	items[0].ID = manifest.CoverageItems[0].ID
	items, err := CompileRuntimeEvidencePurposeMappingCoverage(inventory, items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].SourceObligationChecks, err = CompileRuntimeSourceObligationChecks(inventory, items[0])
	if err != nil {
		t.Fatal(err)
	}
	manifest.CoverageItems = items[:1]
	source.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	source.AcceptedStepKinds = []string{StepKindFileWrite}
	source.ValidationRecipeID, source.RequiredCheckRefs = "", nil
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document.ManifestHash = manifest.Hash
	document.Proposals = document.Proposals[:1]
	facts := []RuntimeSemanticProofFeasibility{semanticFeasibilityForTest(t, manifest, document.Proposals[0], []string{"main.go"}, "", nil)}
	return inventory, manifest, document, facts
}

func TestHistoricalAcceptanceSourceReachesSemanticAndBindingWithoutCredit(t *testing.T) {
	inventory, manifest, document, facts := historicalAcceptanceSemanticFixture(t)
	before, _ := json.Marshal(manifest)
	declaration := manifest.CoverageItems[0].HistoricalAcceptance
	if declaration == nil || declaration.InventoryHash != inventory.Hash || manifest.ProofObligations[0].HistoricalAcceptance == nil ||
		!reflect.DeepEqual(declaration, manifest.ProofObligations[0].HistoricalAcceptance) ||
		manifest.ProofObligations[0].ValidationProofRequirement != nil {
		t.Fatal("historical source silently acquired local validation or lost confirmed consumer")
	}
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 {
		t.Fatal("historical source did not retain one exact proof binding")
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	if !sameRuntimeHistoricalAcceptanceSource(binding.HistoricalAcceptance, declaration) || len(binding.Proofs) != 1 || binding.Proofs[0].EvidenceKind != EvidenceKindStateChange {
		t.Fatal("binding changed historical source or invented current ordinary proof")
	}
	// Availability recognizes the version, but this binding still contains no
	// owner receipt, historical completion or ordinary acceptance proof.
	if err := ValidateRuntimeEvidencePurposeMappingAvailability(&inventory); err != nil {
		t.Fatal(err)
	}
	var reload CoverageEvidenceBinding
	body, _ := json.Marshal(binding)
	if err := json.Unmarshal(body, &reload); err != nil || reload.Validate() != nil || !reflect.DeepEqual(reload, binding) {
		t.Fatal("original mapped binding changed on reload")
	}
	clone := binding.Clone()
	clone.HistoricalAcceptance.AcceptanceItemRef = "changed"
	raw := cloneCoverageEvidenceBindingRaw(binding)
	raw.HistoricalAcceptance.InventoryHash = "changed"
	obligation := cloneRuntimeSemanticProofObligationRaw(manifest.ProofObligations[0])
	obligation.HistoricalAcceptance.AcceptanceChecksHash = "changed"
	item := cloneRuntimeCoverageItemRaw(manifest.CoverageItems[0])
	item.HistoricalAcceptance.AcceptanceTargetTextHash = "changed"
	after, _ := json.Marshal(manifest)
	if !reflect.DeepEqual(before, after) || binding.HistoricalAcceptance.AcceptanceItemRef != "acceptance" {
		t.Fatal("clone/raw reload aliases immutable source projection")
	}
	legacy := binding.Clone()
	legacy.HistoricalAcceptance = nil
	if StableCoverageEvidenceBindingHash(legacy) == binding.Hash {
		t.Fatal("source mapping omitted from binding authority identity")
	}
}

func TestHistoricalAcceptanceDeclarationRejectsIntermediateAndCoordinatedSourceSubstitution(t *testing.T) {
	inventory, manifest, _, _ := historicalAcceptanceSemanticFixture(t)
	for name, mutate := range map[string]func(*RuntimeSemanticProofObligation){
		"wrong historical CWI": func(o *RuntimeSemanticProofObligation) { o.HistoricalAcceptance.HistoricalCoverageWorkItemID = "other" },
		"self": func(o *RuntimeSemanticProofObligation) {
			o.HistoricalAcceptance.AcceptanceCoverageWorkItemID = o.CoverageWorkItemID
		},
		"version":           func(o *RuntimeSemanticProofObligation) { o.HistoricalAcceptance.ContractRef += " " },
		"incomplete checks": func(o *RuntimeSemanticProofObligation) { o.HistoricalAcceptance.AcceptanceChecksHash = "checks" },
		"work":              func(o *RuntimeSemanticProofObligation) { o.ImplementationWork = &RuntimeSemanticImplementationWork{} },
		"repair": func(o *RuntimeSemanticProofObligation) {
			o.ValidationFailureRepair = &RuntimeSemanticValidationFailureRepair{}
		},
		"acquisition": func(o *RuntimeSemanticProofObligation) {
			o.OwnerFactAcquisition = &RuntimeSemanticOwnerFactAcquisition{}
		},
		"mixed evidence": func(o *RuntimeSemanticProofObligation) {
			o.RequiredEvidenceKinds = append(o.RequiredEvidenceKinds, EvidenceKindValidation)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := manifest.ProofObligations[0].Clone()
			mutate(&changed)
			if changed.Validate() == nil {
				t.Fatal("intermediate/invalid source acquired deferred Definition proof")
			}
		})
	}
	_, _, original := evidencePurposeMappingFixture()
	original[0].ID = manifest.CoverageItems[0].ID
	original, err := CompileRuntimeEvidencePurposeMappingCoverage(inventory, original)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"acceptance", "inventory", "criterion", "checks", "omission"} {
		changed := NormalizeRuntimeCoverageItems(original)
		value := changed[0].HistoricalAcceptance
		switch field {
		case "acceptance":
			value.AcceptanceCoverageWorkItemID = "another-consumer"
		case "inventory":
			value.InventoryHash = "another-inventory"
		case "criterion":
			value.HistoricalTargetTextHash = runtimeHistoricalAcceptanceTextHash("summary")
		case "checks":
			value.AcceptanceChecksHash = runtimeHistoricalAcceptanceChecksHash(nil)
		case "omission":
			changed[0].HistoricalAcceptance = nil
		}
		if sameRuntimeCoverageItem(changed[0], original[0]) || ValidateRuntimeEvidencePurposeMappingCoverage(inventory, changed) == nil {
			t.Fatalf("coordinated hash can replace original source field %s", field)
		}
	}
}

func TestHistoricalAcceptancePurposeAndEquivalentIdentityRetainOriginalSource(t *testing.T) {
	_, manifest, document, facts := historicalAcceptanceSemanticFixture(t)
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	operation := RuntimeOperation{ID: "historical-effect", Kind: StepKindFileWrite, BindingRefs: []string{binding.ID}}
	dispatch := RuntimeDispatchRecord{Transaction: AuthorityTransaction{State: ConvergenceStateNeedStateChange}, CoverageBindings: []CoverageEvidenceBinding{binding}}
	if RuntimeHistoricalEffectProofContractForDispatch(dispatch, operation) != RuntimeFileHistoricalEffectProofV1 {
		t.Fatal("exact confirmed mapped binding did not publish receiving-owner purpose")
	}
	value := historicalEffectPurposeFixture().ApplicabilityDependencies
	value.OperationKind = operation.Kind
	if err := ValidateRuntimeHistoricalEffectReceiptPurpose(dispatch, operation, value); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "legacy", "purpose", "owner contract", "repair", "work", "invalid original mapping"} {
		t.Run(name, func(t *testing.T) {
			changed := dispatch
			changed.CoverageBindings = CloneCoverageEvidenceBindings(dispatch.CoverageBindings)
			declared := CloneRuntimeEvidenceApplicabilityDependencies(value)
			switch name {
			case "missing":
				declared = nil
			case "legacy":
				declared.ProtocolRef = RuntimeEvidenceApplicabilityDependenciesProtocol
				declared.PurposeRef, declared.OwnerContractRef, declared.OwnerModuleID, declared.OperationKind = "", "", "", ""
			case "purpose":
				declared.PurposeRef = ""
			case "owner contract":
				declared.OwnerContractRef = "other"
			case "repair":
				changed.RepairCommitment = &RuntimeDispatchRepairCommitment{}
			case "work":
				changed.ImplementationWorkCommitment = &RuntimeImplementationWorkRound{}
			case "invalid original mapping":
				changed.CoverageBindings[0].HistoricalAcceptance.AcceptanceCoverageWorkItemID = binding.CoverageWorkItemID
				changed.CoverageBindings[0].Hash = StableCoverageEvidenceBindingHash(changed.CoverageBindings[0])
			}
			if ValidateRuntimeHistoricalEffectReceiptPurpose(changed, operation, declared) == nil {
				t.Fatal("mapped receipt lost original purpose or upgraded intermediate effect")
			}
		})
	}
	identity, err := DeriveCoverageEvidenceProofIdentity(binding, EvidenceKindStateChange)
	if err != nil || identity.HistoricalAcceptance == nil {
		t.Fatalf("historical acceptance source absent from proof identity: %v", err)
	}
	changed := binding.Clone()
	changed.HistoricalAcceptance.AcceptanceChecksHash = runtimeHistoricalAcceptanceChecksHash(nil)
	changed.Hash = StableCoverageEvidenceBindingHash(changed)
	different, err := DeriveCoverageEvidenceProofIdentity(changed, EvidenceKindStateChange)
	if err != nil || SameCoverageEvidenceProofIdentity(identity, different) {
		t.Fatal("equivalent-binding rollover ignored original consumer all-of source")
	}
	changed = binding.Clone()
	changed.HistoricalAcceptance = nil
	changed.Hash = StableCoverageEvidenceBindingHash(changed)
	unmapped, err := DeriveCoverageEvidenceProofIdentity(changed, EvidenceKindStateChange)
	if err != nil || SameCoverageEvidenceProofIdentity(identity, unmapped) || RuntimeHistoricalEffectProofContractForBindings([]CoverageEvidenceBinding{changed}, operation) != "" {
		t.Fatal("unmapped old state-change-only proof was automatically upgraded")
	}
	clone := identity.Clone()
	clone.HistoricalAcceptance.InventoryHash = "changed"
	raw := cloneCoverageEvidenceProofIdentityRaw(identity)
	raw.HistoricalAcceptance.AcceptanceItemRef = "changed"
	if identity.HistoricalAcceptance.InventoryHash != binding.HistoricalAcceptance.InventoryHash || identity.HistoricalAcceptance.AcceptanceItemRef != "acceptance" {
		t.Fatal("proof identity clone aliases original declaration")
	}
}
