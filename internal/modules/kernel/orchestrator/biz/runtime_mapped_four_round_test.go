package orchestrator

import (
	"fmt"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Real File/Shell/Git owners and Run CAS; source declaration is a neutral private
// fixture. Public authoring is independently tested through its HTTP producer.
func TestMappedProfileHistoricalSourceFourRealWorkRoundsThenOrdinaryAcceptance(t *testing.T) {
	for _, cut := range []string{"uninterrupted", "after_acceptance_stale"} {
		t.Run(cut, func(t *testing.T) { runRuntimeFileBackedFreshFileShellGitAcceptance(t, cut, true, true) })
	}
}
func runtimeMappedFourRoundBootstrap(t *testing.T, b runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	t.Helper()
	policy := runtimecontract.RuntimeImplementationWorkPolicy{ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "profile/test-file-shell-git", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: nil, MaxRoundsPerCWI: 8, MaxTargetsPerRound: 4, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true}
	p := &b.Activation.ExecutionContract.ProfileContracts[0]
	p.DeliverableExpectations[0].RequiredEvidencePurposeContractRef = runtimecontract.RuntimeProfileDeliverableEvidencePurposeV2
	p.EvidenceRequirements[1].ImplementationWorkPolicy = &policy
	p.EvidenceRequirements[1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	p.EvidenceRequirements[1].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	b.DefinitionFocuses[1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	b.DefinitionFocuses[1].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	b.Slices[1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	b.DefinitionFocuses[1].ImplementationWorkPolicy = &policy
	inventory := b.Definition.SourceObligations.Clone()
	inventory.EvidencePurposeMappings[0].HistoricalProfileHash = runtimecontract.StableRuntimeProfileContractsHash([]runtimecontract.RuntimeProfileContract{*p})
	inventory = inventory.WithHash()
	b.Definition.SourceObligations = &inventory
	items, err := runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(inventory, b.CoverageItems)
	if err != nil {
		t.Fatal(err)
	}
	for n := range items {
		items[n].SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(inventory, items[n])
		if err != nil {
			t.Fatal(err)
		}
	}
	b.CoverageItems = items
	b = runtimeTestBootstrapWithBindings(b)
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	return b
}
func runtimeMappedFourRoundSemantic(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
	var work *runtimecontract.RuntimeSemanticImplementationWork
	for _, o := range command.SemanticManifest.ProofObligations {
		if o.ImplementationWork != nil {
			work = o.ImplementationWork
		}
	}
	if work == nil {
		if command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && len(command.SemanticContext.BatchCoverageWorkItemIDs) == 1 && command.SemanticContext.BatchCoverageWorkItemIDs[0] == "cwi-edit" {
			m := command.SemanticManifest
			proposals, err := runtimeFileShellGitSemanticProposals(m, "cwi-edit")
			if err != nil {
				return llmevents.RequestRuntimeCandidateResult{}, err
			}
			filtered := []runtimecontract.RuntimeSemanticProofProposal{}
			for _, p := range proposals {
				if p.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession {
					filtered = append(filtered, p)
				}
			}
			d := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: m.Hash, DefinitionSeedHash: m.DefinitionSeedHash, PlanStep: command.SemanticContext.PlanStep, BatchID: command.SemanticContext.BatchID, BatchSequence: command.SemanticContext.BatchSequence, BatchCoverageWorkItemIDs: command.SemanticContext.BatchCoverageWorkItemIDs, Proposals: filtered}
			raw, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
			if err != nil {
				return llmevents.RequestRuntimeCandidateResult{}, err
			}
			return llmevents.RequestRuntimeCandidateResult{DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, raw)}, nil
		}
		return runtimeFileShellGitSemanticResult(command)
	}
	m := command.SemanticManifest
	d := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: m.Hash, DefinitionSeedHash: m.DefinitionSeedHash, PlanStep: command.SemanticContext.PlanStep}
	if command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		for _, item := range m.CoverageItems {
			d.CoverageOrder = append(d.CoverageOrder, item.ID)
		}
		if work.PurposeChoiceOnly {
			d.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeTerminal
			d.ImplementationIntent = implementationWorkIntentForTest(m)
			if work.Ordinal <= 4 {
				d.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeEffect
			}
		}
	} else {
		d.BatchID, d.BatchSequence, d.BatchCoverageWorkItemIDs = command.SemanticContext.BatchID, command.SemanticContext.BatchSequence, command.SemanticContext.BatchCoverageWorkItemIDs
		o := m.ProofObligations[0]
		d.Proposals = []runtimecontract.RuntimeSemanticProofProposal{{CoverageWorkItemID: o.CoverageWorkItemID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileEdit,
			Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"main.go"}}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: runtimecontract.RuntimeSemanticCurrentDeclarationRef(o)}}
	}
	raw, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	return llmevents.RequestRuntimeCandidateResult{DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, raw)}, nil
}
func runtimeMappedWorkCandidate(command llmevents.RequestRuntimeCandidateCommand, ordinal int) (runtimecontract.RuntimeCandidate, error) {
	old := "package main\n"
	if ordinal > 1 {
		old = fmt.Sprintf("package main\n// work %d\n", ordinal-1)
	}
	next := fmt.Sprintf("package main\n// work %d\n", ordinal)
	if ordinal == 4 {
		next = "package main\n// edited\n"
	}
	base, required := "", ""
	for _, o := range command.CandidateManifest.Operations {
		for _, p := range o.TargetParameterBindings {
			if p.Name == "baseHash" {
				for _, v := range p.Values {
					if v.TargetRef == "main.go" {
						base = v.Value
					}
				}
			}
		}
	}
	for _, r := range command.CandidateManifest.RequiredOperationSequence {
		if r.TargetRef == "main.go" {
			required = r.RequirementID
		}
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: fmt.Sprintf("mapped-work-%d", ordinal), Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.go"}, RequiredOperationRef: required, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &old}, {Name: "newText", String: &next}, {Name: "baseHash", String: &base}}}}}}}, nil
}
