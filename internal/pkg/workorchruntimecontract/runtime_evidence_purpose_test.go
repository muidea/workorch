package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func historicalEffectPurposeFixture() EvidenceFact {
	reads, _ := ownerDependencyUnionFixture()
	post := *CloneRuntimeOwnerRevisionSet(&reads)
	post.Entries[0].Revision = "first-actual-postimage"
	post = post.WithHash()
	return EvidenceFact{ID: "original-effect", Kind: EvidenceKindStateChange, OwnerRevisionSet: &reads,
		ApplicabilityDependencies: &RuntimeEvidenceApplicabilityDependencies{
			ProtocolRef: RuntimeEvidenceApplicabilityDependenciesV2, OwnerRevisionSet: post,
			PurposeRef: RuntimeHistoricalEffectProofV1, OwnerModuleID: RuntimeModuleFile,
			OperationKind: StepKindFileEdit, OwnerContractRef: RuntimeFileHistoricalEffectProofV1,
		}}
}

func TestHistoricalEffectPurposeDoesNotPromotePostimageToCurrent(t *testing.T) {
	fact := historicalEffectPurposeFixture()
	original := cloneEvidenceFactRaw(fact)
	current := CloneRuntimeOwnerRevisionSet(&fact.ApplicabilityDependencies.OwnerRevisionSet)
	current.Entries[0].Revision = "second-actual-postimage"
	*current = current.WithHash()
	observation := RuntimeEvidenceApplicabilityObservation{Request: RuntimeEvidenceApplicabilityRequest{
		SourceEvidenceID: fact.ID, SourceEvidenceHash: StableRuntimeEvidenceFactHash(fact), EvidenceRevision: "2"}, CurrentDependencies: current}
	observations := []RuntimeEvidenceApplicabilityObservation{observation}
	if !RuntimeEvidenceIsHistoricalEffect(fact) || !RuntimeEvidenceHasStaleApplicability(fact, observations, "2") ||
		RuntimeEvidenceHasFreshApplicability(fact, observations, "2", "") ||
		!RuntimeEvidenceHasFreshProofApplicability(fact, observations, "2", "") {
		t.Fatal("historical behavior eligibility altered original resource currentness")
	}
	if RuntimeHistoricalEffectsHaveCurrentAcceptanceProof([]EvidenceFact{fact}) {
		t.Fatal("historical effect alone supplies current ordinary acceptance")
	}
	validation := EvidenceFact{Kind: EvidenceKindValidation, Validation: &RuntimeValidationObservation{}, OwnerRevisionSet: CloneRuntimeOwnerRevisionSet(current),
		ApplicabilityDependencies: &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesV1, OwnerRevisionSet: *current}}
	if !RuntimeHistoricalEffectsHaveCurrentAcceptanceProof([]EvidenceFact{fact, validation}) {
		t.Fatal("separately selected current ordinary proof was not conjoined")
	}
	validation.ApplicabilityDependencies = nil
	if RuntimeHistoricalEffectsHaveCurrentAcceptanceProof([]EvidenceFact{fact, validation}) {
		t.Fatal("unsampled legacy validation filled current resources")
	}
	if _, err := CompileRuntimeOwnerRevisionSetUnion(current.DefinitionRevision, fact.ApplicabilityDependencies.OwnerRevisionSet, *current); err == nil {
		t.Fatal("purpose relaxed conflicting resource versions")
	}
	body, _ := json.Marshal(fact)
	var reload EvidenceFact
	if json.Unmarshal(body, &reload) != nil || !reflect.DeepEqual(reload, fact) || !RuntimeEvidenceIsHistoricalEffect(reload) || !reflect.DeepEqual(fact, original) {
		t.Fatal("purpose reload changed historical input/postcondition")
	}
	legacy := cloneEvidenceFactRaw(fact)
	legacy.ApplicabilityDependencies = &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesV1, OwnerRevisionSet: fact.ApplicabilityDependencies.OwnerRevisionSet}
	if RuntimeEvidenceIsHistoricalEffect(legacy) || RuntimeEvidenceHasFreshProofApplicability(legacy, nil, "2", "") {
		t.Fatal("old receipt was silently upgraded or stale credit restored")
	}
}

func TestHistoricalEffectPurposeRejectsWrongOwnerKindProtocolAndSource(t *testing.T) {
	fact := historicalEffectPurposeFixture()
	for _, name := range []string{"owner", "kind", "purpose", "contract", "protocol", "reads", "definition", "hash", "evidence_kind"} {
		t.Run(name, func(t *testing.T) {
			changed := cloneEvidenceFactRaw(fact)
			value := changed.ApplicabilityDependencies
			switch name {
			case "owner":
				value.OwnerModuleID = RuntimeModuleShell
			case "kind":
				value.OperationKind = StepKindFileRead
			case "purpose":
				value.PurposeRef += " "
			case "contract":
				value.OwnerContractRef += " "
			case "protocol":
				value.ProtocolRef = RuntimeEvidenceApplicabilityDependenciesV1
			case "reads":
				changed.OwnerRevisionSet = nil
			case "definition":
				value.OwnerRevisionSet.DefinitionRevision = "other"
				value.OwnerRevisionSet = value.OwnerRevisionSet.WithHash()
			case "hash":
				value.OwnerRevisionSet.SetHash = "wrong"
			case "evidence_kind":
				changed.Kind = EvidenceKindValidation
			}
			if RuntimeEvidenceIsHistoricalEffect(changed) || StableRuntimeEvidenceFactHash(changed) == StableRuntimeEvidenceFactHash(fact) {
				t.Fatal("invalid or unsealed historical proof purpose")
			}
		})
	}
	if ValidateRuntimeEvidencePurposeForOperation(fact.ApplicabilityDependencies, StepKindFileRead, EvidenceKindStateChange) == nil ||
		ValidateRuntimeEvidencePurposeForOperation(fact.ApplicabilityDependencies, StepKindFileEdit, EvidenceKindValidation) == nil {
		t.Fatal("declared purpose bypassed independent sealed operation")
	}
}

func TestHistoricalEffectPurposeRequiresExactSealedAllOfBinding(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	operation := RuntimeOperation{ID: "effect", Kind: StepKindFileWrite, BindingRefs: []string{binding.ID}}
	want := RuntimeFileHistoricalEffectProofV1
	if got := RuntimeHistoricalEffectProofContractForBindings([]CoverageEvidenceBinding{binding}, operation); got != want {
		t.Fatalf("complete all-of purpose = %q", got)
	}
	for _, name := range []string{"no_binding", "wrong_ref", "duplicate", "no_validation", "invalid_hash", "read_only"} {
		t.Run(name, func(t *testing.T) {
			bindings := []CoverageEvidenceBinding{binding.Clone()}
			op := operation.Clone()
			switch name {
			case "no_binding":
				op.BindingRefs = nil
			case "wrong_ref":
				op.BindingRefs[0] = "other"
			case "duplicate":
				bindings = append(bindings, binding.Clone())
			case "no_validation":
				bindings[0].Proofs = bindings[0].Proofs[:1]
				bindings[0].Hash = StableCoverageEvidenceBindingHash(bindings[0])
			case "invalid_hash":
				bindings[0].Hash = "wrong"
			case "read_only":
				op.Kind = StepKindFileRead
			}
			if RuntimeHistoricalEffectProofContractForBindings(bindings, op) != "" {
				t.Fatal("purpose invented without exact independent acceptance obligation")
			}
		})
	}
}

func TestHistoricalEffectPurposeDoesNotUpgradeIntermediateDispatch(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	operation := RuntimeOperation{ID: "effect", Kind: StepKindFileWrite, BindingRefs: []string{binding.ID}}
	dispatch := RuntimeDispatchRecord{Transaction: AuthorityTransaction{State: ConvergenceStateNeedStateChange}, CoverageBindings: []CoverageEvidenceBinding{binding}}
	if RuntimeHistoricalEffectProofContractForDispatch(dispatch, operation) != RuntimeFileHistoricalEffectProofV1 {
		t.Fatal("ordinary all-of dispatch lost its declared behavior proof")
	}
	for _, name := range []string{"repair", "repair_hash", "work", "validation", "assessment"} {
		t.Run(name, func(t *testing.T) {
			changed := dispatch
			switch name {
			case "repair":
				changed.RepairCommitment = &RuntimeDispatchRepairCommitment{}
			case "repair_hash":
				changed.RepairCommitmentHash = "original-repair"
			case "work":
				changed.ImplementationWorkCommitment = &RuntimeImplementationWorkRound{}
			case "validation":
				changed.Transaction.State = ConvergenceStateNeedValidation
			case "assessment":
				changed.Transaction.State = ConvergenceStateNeedAssessment
			}
			if RuntimeHistoricalEffectProofContractForDispatch(changed, operation) != "" {
				t.Fatal("intermediate/non-effect dispatch acquired historical acceptance purpose")
			}
		})
	}
}
