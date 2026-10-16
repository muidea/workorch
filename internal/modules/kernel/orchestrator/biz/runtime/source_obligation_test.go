package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeSharedSourceObligationFixture(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	t.Helper()
	facts, definition := controllerFacts(), controllerDefinition()
	inventory := (runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent, CandidateHash: "candidate", SpecificationHash: "specification", DefinitionRevision: definition.Revision, StartupInputHash: definition.StartupInputHash,
		Sources: []runtimecontract.RuntimeDefinitionSource{
			{CandidateItemRef: "goal", ItemRef: "formal-goal", SourceScope: "goal", SourceID: "goal", Text: definition.GoalDefinition},
			{CandidateItemRef: "dod", ItemRef: "formal-dod", SourceScope: "dodItem", SourceID: "dod-v2", Text: "deliver"},
		},
		Requirements: []runtimecontract.RuntimeSourceRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{"dod"}, Text: "完整交付"}},
		Obligations: []runtimecontract.RuntimeSourceObligation{
			{ID: "A", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: "A 独立判据"},
			{ID: "B", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: "B 独立判据"},
		},
		Verifications: []runtimecontract.RuntimeSourceVerification{
			{ID: "verify-A", ObligationID: "A", Declaration: "验证 A", ProfileID: "profile", EvidenceRequirementID: "dod-v2", Scope: "dodItem"},
			{ID: "verify-B", ObligationID: "B", Declaration: "验证 B", ProfileID: "profile", EvidenceRequirementID: "dod-v2", Scope: "dodItem"},
		},
	}).WithHash()
	definition.SourceObligations = &inventory
	facts.DefinitionSeed.SourceObligations = &inventory
	checks, err := runtimecontract.CompileRuntimeSourceObligationChecks(inventory, facts.DefinitionSeed.CoverageItems[0])
	if err != nil {
		t.Fatal(err)
	}
	facts.DefinitionSeed.CoverageItems[0].SourceObligationChecks = checks
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.RequireAssessment = true
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	facts.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "profile",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "dod-v2", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			ValidationRecipeID: runtimeManagedValidationEvidencePolicy().ID,
		}},
	}}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	return controller, repository, aggregate
}

func TestRuntimeSharedSourceObligationFreshAggregateRejectsRehashedForeignDeclaration(t *testing.T) {
	_, _, aggregate := runtimeSharedSourceObligationFixture(t)
	for _, name := range []string{"Profile", "EvidenceRequirement", "Scope"} {
		t.Run(name, func(t *testing.T) {
			changed := aggregate.CloneRaw()
			inventory := changed.Facts.DefinitionSeed.SourceObligations.Clone()
			switch name {
			case "Profile":
				inventory.Verifications[0].ProfileID = "foreign"
			case "EvidenceRequirement":
				inventory.Verifications[0].EvidenceRequirementID = "foreign"
			case "Scope":
				inventory.Verifications[0].Scope = "goal"
			}
			inventory = inventory.WithHash()
			if err := inventory.Validate(); err != nil {
				t.Fatalf("反例需先满足库存自洽: %v", err)
			}
			changed.Definition.SourceObligations = &inventory
			changed.Facts.DefinitionSeed.SourceObligations = &inventory
			for index, item := range changed.Facts.DefinitionSeed.CoverageItems {
				checks, err := runtimecontract.CompileRuntimeSourceObligationChecks(inventory, item)
				if err != nil {
					t.Fatal(err)
				}
				changed.Facts.DefinitionSeed.CoverageItems[index].SourceObligationChecks = checks
			}
			changed.Facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(changed.Facts.DefinitionSeed)
			if err := changed.Facts.DefinitionSeed.Validate(); err != nil {
				t.Fatalf("反例需先满足 seed 自洽: %v", err)
			}
			encoded, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeAggregate
			if err := json.Unmarshal(encoded, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err == nil || !strings.Contains(err.Error(), "source applicability") {
				t.Fatalf("fresh aggregate 没有独立反证 declaration: %v", err)
			}
		})
	}
}

func TestSourceObligationTerminalSealRejectsCoordinatedInventoryDeletion(t *testing.T) {
	ctx := context.Background()
	controller, _, aggregate := runtimeSharedSourceObligationFixture(t)
	binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
	proof, _ := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	observation := runtimecontract.RuntimeValidationObservation{RecipeID: proof.ValidationRecipeID}
	for _, attestation := range proof.ValidationCheckAttestations {
		observation.ObservationProtocolRef = attestation.ObservationProtocolRef
		observation.ExecutedChecks = append(observation.ExecutedChecks, attestation.NativeCheckRef)
		observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRef)
	}
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "source-validation", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{binding.ID}, TargetRefs: []string{"target.txt"}, Parameters: runtimeManagedValidationParameters(),
	}}}}}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	reads := runtimeSourceObligationFixtureReads(sealed)
	completed, err := controller.executeActiveDispatch(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, &qualityOperationExecutor{owner: controller, evidence: runtimecontract.RuntimeOperationEvidence{Validation: &observation, OwnerRevisionSet: &reads}})
	if err != nil {
		t.Fatal(err)
	}
	completed, err = runtimeTestCommitSatisfiedCoverageAssessment(ctx, controller, completed)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := terminalMaterialSealFromAggregate(completed)
	if err != nil {
		t.Fatal(err)
	}
	next := completed.CloneRaw()
	next.StateRevision++
	next.TerminalMaterialSeal = &seal
	if err := validateTerminalMaterialSealTransition(completed, next); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coordinated delete B", "remove whole inventory"} {
		t.Run(name, func(t *testing.T) {
			forged := next.CloneRaw()
			closeout := forged.TerminalMaterialSeal.Material.RuntimeCloseout
			if name == "remove whole inventory" {
				closeout.SourceObligations = nil
				for index := range closeout.RequirementOutcomes {
					closeout.RequirementOutcomes[index].SourceObligations = nil
				}
			} else {
				inventory := closeout.SourceObligations.Clone()
				inventory.Obligations = inventory.Obligations[:1]
				inventory.Verifications = inventory.Verifications[:1]
				inventory = inventory.WithHash()
				closeout.SourceObligations = &inventory
				for index := range closeout.RequirementOutcomes {
					outcome := &closeout.RequirementOutcomes[index]
					outcome.SourceObligations = outcome.SourceObligations[:1]
					check := &outcome.SourceObligations[0].Check
					check.InventoryHash = inventory.Hash
					check.CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef(inventory.DefinitionRevision, outcome.CoverageWorkItemID, *check)
				}
			}
			body := closeout.WithCloseoutHash()
			forged.TerminalMaterialSeal.Material.RuntimeCloseout = &body
			if err := body.ValidateRaw(); err != nil {
				t.Fatalf("反例应先通过 standalone 自洽 closeout: %v", err)
			}
			forged.TerminalMaterialSeal.MaterialHash, err = forged.TerminalMaterialSeal.Material.CanonicalHash()
			if err != nil {
				t.Fatal(err)
			}
			if err := validateTerminalMaterialSealTransition(completed, forged); err == nil {
				t.Fatal("协调删来源绕过独立生成前像")
			}
			encoded, err := json.Marshal(forged)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeAggregate
			if err := json.Unmarshal(encoded, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := runtimecontract.ValidateRuntimeTerminalMaterialSeal(reloaded.TerminalMaterialSeal, terminalMaterialSource(reloaded)); err == nil {
				t.Fatal("fresh-load 接纳被协调删除的来源材料")
			}
		})
	}
}

func TestRuntimeSharedSourceObligationReceiptCannotCompleteOnlyA(t *testing.T) {
	for _, complete := range []bool{false, true} {
		name := "only A"
		if complete {
			name = "A and B"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			controller, repository, aggregate := runtimeSharedSourceObligationFixture(t)
			binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
			proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
			if !found {
				t.Fatal("sealed source validation proof missing")
			}
			checks := aggregate.Facts.DefinitionSeed.CoverageItems[0].SourceObligationChecks
			observation := runtimecontract.RuntimeValidationObservation{RecipeID: proof.ValidationRecipeID}
			for _, attestation := range proof.ValidationCheckAttestations {
				observation.ObservationProtocolRef = attestation.ObservationProtocolRef
				observation.ExecutedChecks = append(observation.ExecutedChecks, attestation.NativeCheckRef)
				if complete || attestation.RequiredCheckRef != checks[1].CheckRef {
					observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRef)
				}
			}
			candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "source-validation", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{binding.ID}, TargetRefs: []string{"target.txt"}, Parameters: runtimeManagedValidationParameters(),
			}}}}}
			sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
			if err != nil {
				t.Fatal(err)
			}
			reads := runtimeSourceObligationFixtureReads(sealed)
			executor := &qualityOperationExecutor{owner: controller, evidence: runtimecontract.RuntimeOperationEvidence{Summary: "独立 owner 检查观察", Validation: &observation, OwnerRevisionSet: &reads}}
			completed, err := controller.executeActiveDispatch(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, executor)
			if err != nil {
				t.Fatal(err)
			}
			if executor.called != 1 || len(completed.OperationReceipts) != 1 {
				t.Fatal("fixture did not exercise persisted owner receipt")
			}
			if !complete {
				if completed.Decision.State == runtimecontract.ConvergenceStateComplete || completed.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied {
					t.Fatal("only A closed shared source CWI")
				}
				for _, fact := range completed.Facts.Evidence.Accepted {
					if len(fact.CoverageWorkItemIDs) != 0 {
						t.Fatal("only A obtained source CWI credit")
					}
				}
			} else {
				completed, err = runtimeTestCommitSatisfiedCoverageAssessment(ctx, controller, completed)
				if err != nil {
					t.Fatal(err)
				}
				if completed.Decision.State != runtimecontract.ConvergenceStateComplete || completed.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied {
					t.Fatal("all_of source receipt did not complete after assessment")
				}
				material, err := runtimecontract.DeriveRuntimeTerminalResultMaterialFromState(terminalMaterialSource(completed))
				if err != nil || material.RuntimeCloseout.ValidateRaw() != nil || len(material.RuntimeCloseout.RequirementOutcomes[0].SourceObligations) != 2 {
					t.Fatalf("complete source closeout missing: %v", err)
				}
			}
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.repository.Load(ctx, completed.Facts.WorkspaceID, completed.Facts.RunID)
			if err != nil || loaded.Validate() != nil || loaded.Decision.State != completed.Decision.State || len(loaded.Facts.DefinitionSeed.CoverageItems[0].SourceObligationChecks) != 2 {
				t.Fatalf("restart lost source authority: %v", err)
			}
			if complete && len(loaded.Facts.Coverage[0].EvidenceRefs) != 1 {
				t.Fatalf("restart duplicated shared CWI credit: %+v", loaded.Facts.Coverage)
			}
		})
	}
}

// Explicit technical mock read basis. Real byte sampling is verified by the
// File/Shell owner mainline, not by this private reducer fixture.
func runtimeSourceObligationFixtureReads(aggregate RuntimeAggregate) runtimecontract.RuntimeOwnerRevisionSet {
	return (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: aggregate.Facts.DefinitionRevision, Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: runtimecontract.RuntimeModuleShell + "/" + string(aggregate.Facts.WorkspaceID), ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: "target.txt", Revision: "fixture-input-bytes", DependencyHash: "fixture-native-selector"}}}).WithHash()
}
