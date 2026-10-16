package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"
)

// ValidateRuntimeExecutionProofMappings checks both directions against sealed
// Profile rules. A mapping cannot redirect an ordinary same-CWI rule, and an
// opted-in rule cannot silently fall back to its consumer when a mapping is absent.
func ValidateRuntimeExecutionProofMappings(inventory RuntimeSourceObligationInventory, profiles []RuntimeProfileContract) error {
	declared := false
	for _, profile := range profiles {
		for _, requirement := range profile.EvidenceRequirements {
			for _, rule := range requirement.LineageAssertions {
				declared = declared || rule.ExecutionSourceContractRef != ""
			}
		}
	}
	if !declared && len(inventory.ExecutionProofMappings) == 0 {
		return nil
	}
	if len(inventory.ExecutionProofMappings) > 512 {
		return fmt.Errorf("execution proof mappings exceed bound")
	}
	seen := map[string]bool{}
	goalSources := map[string]string{}
	for _, mapping := range inventory.ExecutionProofMappings {
		consumer, source := 0, 0
		var consumerItem, sourceItem RuntimeDefinitionSource
		for _, item := range inventory.Sources {
			if item.CandidateItemRef == mapping.ConsumerItemRef {
				consumer++
				consumerItem = item
			}
			if item.CandidateItemRef == mapping.SourceItemRef {
				source++
				sourceItem = item
			}
		}
		key := mapping.ConsumerItemRef + "\x00" + mapping.RuleRef
		if consumer != 1 || source != 1 || validateRuntimeExecutionProofMappingSource(mapping, consumerItem, sourceItem) != nil || seen[key] {
			return fmt.Errorf("execution proof mapping has invalid/ambiguous original source identity")
		}
		seen[key] = true
		if mapping.ContractRef == RuntimeGoalSourceExecutionProofV2 {
			if prior := goalSources[mapping.ConsumerItemRef]; prior != "" && prior != mapping.SourceItemRef {
				return fmt.Errorf("Goal rules require one unique original execution source")
			}
			goalSources[mapping.ConsumerItemRef] = mapping.SourceItemRef
		}
	}
	used := make([]bool, len(inventory.ExecutionProofMappings))
	for _, consumer := range inventory.Sources {
		policy, err := CompileRuntimeSourceEvidencePolicy(consumer.SourceScope, consumer.SourceID, profiles, nil)
		if err != nil {
			return err
		}
		rules := policy.LineageAssertions
		if consumer.SourceScope == "goal" {
			rules, err = runtimeGoalExecutionRules(consumer, profiles)
			if err != nil {
				return err
			}
		}
		for _, rule := range rules {
			if rule.ExecutionSourceContractRef == "" {
				continue
			}
			if err := rule.Validate(); err != nil {
				return err
			}
			matches := 0
			for index, mapping := range inventory.ExecutionProofMappings {
				if mapping.ConsumerItemRef == consumer.CandidateItemRef && mapping.RuleRef == rule.Ref && mapping.ContractRef == rule.ExecutionSourceContractRef {
					if mapping.ContractRef == RuntimeGoalSourceExecutionProofV2 {
						var source RuntimeDefinitionSource
						for _, value := range inventory.Sources {
							if value.CandidateItemRef == mapping.SourceItemRef {
								source = value
							}
						}
						ordinary, err := CompileRuntimeSourceEvidencePolicy(source.SourceScope, source.SourceID, profiles, nil)
						if err != nil || !slices.Contains(ordinary.RequiredEvidenceKinds, EvidenceKindValidation) || !runtimeOperationKindsContain(ordinary.AcceptedStepKinds, RuntimeBlockKindManagedValidationSession) {
							return fmt.Errorf("Goal execution source requires declared ordinary validation acceptance")
						}
					}
					matches++
					used[index] = true
				}
			}
			if matches != 1 {
				return fmt.Errorf("execution proof rule %q source %q/%q requires unique confirmed source mapping", rule.Ref, consumer.SourceScope, consumer.CandidateItemRef)
			}
		}
	}
	for index, matched := range used {
		if !matched {
			return fmt.Errorf("execution proof mapping %d has no exact opted-in Profile consumer", index)
		}
	}
	return nil
}

// Compilation adds only the exact declared predecessor; it cannot pick another
// source, create a CWI, or invent a missing continuation endpoint. Existing
// dependency validation rejects cycles, including cycles across several rules.
func CompileRuntimeExecutionProofMappingCoverage(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) ([]RuntimeCoverageItem, error) {
	if inventory.Validate() != nil {
		return nil, fmt.Errorf("execution proof mapping coverage requires valid confirmed inventory")
	}
	result := slices.Clone(items)
	for index := range result {
		result[index] = result[index].Normalized()
	}
	for _, mapping := range inventory.ExecutionProofMappings {
		resolve := func(ref string) (int, error) {
			var source RuntimeDefinitionSource
			for _, candidate := range inventory.Sources {
				if candidate.CandidateItemRef == ref {
					source = candidate
				}
			}
			index, matches := -1, 0
			for n, item := range result {
				if item.SourceScope == source.SourceScope && item.SourceID == source.SourceID && item.TargetText == strings.TrimSpace(source.Text) {
					index, matches = n, matches+1
				}
			}
			if matches != 1 {
				return -1, fmt.Errorf("execution proof mapping requires unique original coverage endpoint")
			}
			return index, nil
		}
		consumer, err := resolve(mapping.ConsumerItemRef)
		if mapping.ContractRef == RuntimeGoalSourceExecutionProofV2 {
			// Goal has no independent work CWI. Its guard is consumed with the
			// mapped source's ordinary acceptance, not a fabricated predecessor.
			if _, err := resolve(mapping.SourceItemRef); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		source, err := resolve(mapping.SourceItemRef)
		if err != nil {
			return nil, err
		}
		if consumer == source {
			return nil, fmt.Errorf("execution proof mapping cannot target itself")
		}
		if !slices.Contains(result[consumer].PrerequisiteCoverageWorkItemIDs, result[source].ID) {
			result[consumer].PrerequisiteCoverageWorkItemIDs = append(result[consumer].PrerequisiteCoverageWorkItemIDs, result[source].ID)
		}
		result[consumer] = result[consumer].Normalized()
	}
	if err := ValidateRuntimeCoverageItemDependencies(result); err != nil {
		return nil, err
	}
	return result, nil
}

func ValidateRuntimeExecutionProofMappingCoverage(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) error {
	if len(inventory.ExecutionProofMappings) == 0 {
		return nil
	}
	compiled, err := CompileRuntimeExecutionProofMappingCoverage(inventory, items)
	if err != nil {
		return err
	}
	for index, item := range compiled {
		if !slices.Equal(item.PrerequisiteCoverageWorkItemIDs, items[index].PrerequisiteCoverageWorkItemIDs) {
			return fmt.Errorf("execution proof mapping omitted exact original source predecessor")
		}
	}
	return nil
}

// Resolve only from immutable confirmed candidate refs, exact DefinitionSeed
// source text and the SM-2 current binding reference's original accepted
// semantic batch. A historical repair binding or the consumer's binding is
// never substituted for a missing ordinary source binding.
func runtimeExecutionProofSourceBinding(assertion RuntimeLineageAssertion, sources RuntimeLineageAssertionSources) (CoverageEvidenceBinding, error) {
	return runtimeExecutionProofSourceBindingInSession(assertion, sources, nil)
}

func runtimeExecutionProofSourceBindingInSession(assertion RuntimeLineageAssertion, sources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) (CoverageEvidenceBinding, error) {
	invalid := func() (CoverageEvidenceBinding, error) {
		return CoverageEvidenceBinding{}, fmt.Errorf("execution proof lacks unique confirmed original source binding")
	}
	inventory := sources.Definition.SourceObligations
	if (assertion.Rule.ExecutionSourceContractRef != RuntimeCrossSourceExecutionProofV1 && assertion.Rule.ExecutionSourceContractRef != RuntimeGoalSourceExecutionProofV2) || inventory == nil || inventory.Validate() != nil ||
		inventory.StartupInputHash != sources.Definition.StartupInputHash || inventory.DefinitionRevision != sources.Definition.Revision ||
		!SameRuntimeSourceObligationInventory(inventory, sources.Facts.DefinitionSeed.SourceObligations) ||
		ValidateRuntimeExecutionProofMappings(*inventory, sources.Facts.Activation.ExecutionContract.ProfileContracts) != nil {
		return invalid()
	}
	consumerRef, consumers := "", 0
	for _, source := range inventory.Sources {
		text := strings.TrimSpace(source.Text)
		if assertion.Rule.ExecutionSourceContractRef == RuntimeGoalSourceExecutionProofV2 {
			text = source.Text
		}
		if source.SourceScope == assertion.SourceScope && source.SourceID == assertion.SourceID && RuntimeLineageSourceTextHash(text) == assertion.SourceTextHash {
			consumerRef, consumers = source.CandidateItemRef, consumers+1
		}
	}
	if consumers != 1 {
		return invalid()
	}
	sourceRef, mappings := "", 0
	for _, mapping := range inventory.ExecutionProofMappings {
		if mapping.ConsumerItemRef == consumerRef && mapping.RuleRef == assertion.Rule.Ref && mapping.ContractRef == assertion.Rule.ExecutionSourceContractRef {
			sourceRef, mappings = mapping.SourceItemRef, mappings+1
		}
	}
	if mappings != 1 {
		return invalid()
	}
	var original RuntimeDefinitionSource
	originals := 0
	for _, source := range inventory.Sources {
		if source.CandidateItemRef == sourceRef {
			original, originals = source, originals+1
		}
	}
	if originals != 1 || original.SourceScope != "dodItem" {
		return invalid()
	}
	var item RuntimeCoverageItem
	items := 0
	for _, candidate := range sources.Facts.DefinitionSeed.CoverageItems {
		if candidate.SourceScope == original.SourceScope && candidate.SourceID == original.SourceID && candidate.TargetText == strings.TrimSpace(original.Text) {
			item, items = candidate, items+1
		}
	}
	goalConsumer := assertion.Rule.ExecutionSourceContractRef == RuntimeGoalSourceExecutionProofV2
	if items != 1 || (!goalConsumer && item.ID == assertion.CoverageWorkItemID) || (goalConsumer && item.ID != assertion.CoverageWorkItemID) {
		return invalid()
	}
	var current RuntimeCoverageItemFact
	currentItems := 0
	for _, candidate := range sources.Facts.Coverage {
		if candidate.ID == item.ID && candidate.SourceScope == item.SourceScope && candidate.SourceID == item.SourceID && candidate.TargetText == item.TargetText {
			current, currentItems = candidate, currentItems+1
		}
	}
	if currentItems != 1 || current.CurrentBindingRef == nil || current.CurrentBindingRef.Validate() != nil {
		return invalid()
	}
	entries, err := session.preimages(sources.Semantic)
	if err != nil {
		return invalid()
	}
	var binding CoverageEvidenceBinding
	matches := 0
	for _, entry := range entries {
		if entry.semanticManifestHash != current.CurrentBindingRef.SemanticManifestHash {
			continue
		}
		purpose, err := resolveRuntimeSemanticEvidenceObligationInSession(entry.obligation, session.semanticSession())
		if err != nil || purpose.Purpose != RuntimeSemanticEvidenceObligationDefinitionProof || entry.obligation.CoverageWorkItemID != item.ID {
			continue
		}
		for _, candidate := range entry.bindings {
			if runtimeLineageBindingMatches(candidate, item.ID, *current.CurrentBindingRef) {
				binding, matches = candidate, matches+1
			}
		}
	}
	if matches != 1 || binding.Validate() != nil || binding.DefinitionRevision != sources.Definition.Revision || binding.DeclarationRef != CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID) {
		return invalid()
	}
	if goalConsumer {
		if ValidateRuntimeGoalExecutionAssertionSeed(sources.Facts.DefinitionSeed, sources.Facts.Activation.ExecutionContract.ProfileContracts) != nil || !slices.Contains(binding.LineageAssertions, assertion) {
			return invalid()
		}
		ordinary := false
		for _, proof := range binding.Proofs {
			ordinary = ordinary || proof.EvidenceKind == EvidenceKindValidation
		}
		if !ordinary {
			return invalid()
		}
	}
	// The private index may be used by another guard in this same invocation.
	// Its backing slices must never escape through the resolver's result.
	return cloneCoverageEvidenceBindingRaw(binding), nil
}
