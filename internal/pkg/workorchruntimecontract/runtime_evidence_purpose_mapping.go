package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeCrossCWIEvidencePurposeV1 = "runtime/cross-cwi-evidence-purpose/v1"
const RuntimeProfileDeliverableEvidencePurposeV2 = "runtime/profile-deliverable-evidence-purpose/v2"

// Candidate refs are sealed before activation. This is not an execution-source
// guard, a receipt, a current binding, or a grant of terminal credit.
type RuntimeEvidencePurposeMapping struct {
	ContractRef             string
	HistoricalItemRef       string
	AcceptanceItemRef       string
	HistoricalProfileID     string `json:",omitempty"`
	HistoricalDeliverableID string `json:",omitempty"`
	HistoricalProfileHash   string `json:",omitempty"`
	HistoricalText          string `json:",omitempty"`
}

// Detached immutable endpoints for a later SM-2/SM-3 consumer. Resolving these
// values proves the confirmed source identity, not behavior or current proof.
type RuntimeEvidencePurposeEndpoints struct {
	Mapping       RuntimeEvidencePurposeMapping
	InventoryHash string
	Historical    RuntimeCoverageItem
	Acceptance    RuntimeCoverageItem
}

func ResolveRuntimeEvidencePurposeEndpoints(definition RuntimeDefinitionContext, execution RuntimeExecutionContract, seed RuntimeDefinitionSeed, historicalID common.CoverageWorkItemID) (RuntimeEvidencePurposeEndpoints, error) {
	invalid := func() (RuntimeEvidencePurposeEndpoints, error) {
		return RuntimeEvidencePurposeEndpoints{}, fmt.Errorf("evidence purpose endpoints lack unique sealed original Definition mapping")
	}
	inventory := definition.SourceObligations
	if historicalID == "" || inventory == nil || inventory.Validate() != nil ||
		inventory.DefinitionRevision != definition.Revision || inventory.StartupInputHash != definition.StartupInputHash ||
		!SameRuntimeSourceObligationInventory(inventory, seed.SourceObligations) || seed.SchemaVersion != RuntimeDefinitionSeedSchemaCurrent ||
		seed.DefinitionRevision != definition.Revision || seed.Hash != StableRuntimeDefinitionSeedHash(seed) ||
		seed.PolicyHash != StableRuntimeCandidatePrefixHashWithExecutionContract(definition, execution) ||
		ValidateRuntimeSourceObligationApplicability(inventory, execution.ProfileContracts) != nil ||
		ValidateRuntimeEvidencePurposeMappingCoverage(*inventory, seed.CoverageItems) != nil ||
		ValidateRuntimeSourceObligationSeed(seed) != nil {
		return invalid()
	}
	var history RuntimeCoverageItem
	histories := 0
	for _, item := range seed.CoverageItems {
		if item.ID == historicalID {
			history, histories = item, histories+1
		}
	}
	if histories != 1 {
		return invalid()
	}
	historyRef, historySources := "", 0
	for _, source := range RuntimeEvidencePurposeSources(*inventory) {
		if source.SourceScope == history.SourceScope && source.SourceID == history.SourceID && strings.TrimSpace(source.Text) == history.TargetText {
			historyRef, historySources = source.CandidateItemRef, historySources+1
		}
	}
	if historySources != 1 {
		return invalid()
	}
	var mapping RuntimeEvidencePurposeMapping
	matches := 0
	for _, candidate := range inventory.EvidencePurposeMappings {
		if candidate.HistoricalItemRef == historyRef {
			mapping, matches = candidate, matches+1
		}
	}
	if matches != 1 {
		return invalid()
	}
	var acceptanceSource RuntimeDefinitionSource
	for _, source := range inventory.Sources {
		if source.CandidateItemRef == mapping.AcceptanceItemRef {
			acceptanceSource = source
		}
	}
	var acceptance RuntimeCoverageItem
	acceptances := 0
	for _, item := range seed.CoverageItems {
		if item.SourceScope == acceptanceSource.SourceScope && item.SourceID == acceptanceSource.SourceID && item.TargetText == strings.TrimSpace(acceptanceSource.Text) {
			acceptance, acceptances = item, acceptances+1
		}
	}
	if acceptances != 1 || acceptance.ID == history.ID || !slices.Contains(acceptance.PrerequisiteCoverageWorkItemIDs, history.ID) {
		return invalid()
	}
	return RuntimeEvidencePurposeEndpoints{Mapping: mapping, InventoryHash: inventory.Hash, Historical: history.Normalized(), Acceptance: acceptance.Normalized()}, nil
}

func validateRuntimeEvidencePurposeMappingSources(inventory RuntimeSourceObligationInventory) error {
	if len(inventory.EvidencePurposeMappings) > 512 {
		return fmt.Errorf("evidence purpose mappings exceed bound")
	}
	sources := map[string]RuntimeDefinitionSource{}
	for _, source := range inventory.Sources {
		if _, duplicate := sources[source.CandidateItemRef]; duplicate {
			return fmt.Errorf("evidence purpose mapping has ambiguous original source")
		}
		sources[source.CandidateItemRef] = source
	}
	seen := map[string]bool{}
	for _, mapping := range inventory.EvidencePurposeMappings {
		history, acceptance := sources[mapping.HistoricalItemRef], sources[mapping.AcceptanceItemRef]
		validHistory := mapping.ContractRef == RuntimeCrossCWIEvidencePurposeV1 && history.SourceScope == "dodItem" && !runtimeEvidencePurposeHasProfileFields(mapping)
		if mapping.ContractRef == RuntimeProfileDeliverableEvidencePurposeV2 {
			validHistory = ValidateRuntimeProfileEvidencePurposeMappingDeclaration(mapping) == nil && history.CandidateItemRef == ""
		}
		if !validHistory || acceptance.SourceScope != "dodItem" ||
			mapping.HistoricalItemRef == mapping.AcceptanceItemRef || seen[mapping.HistoricalItemRef] ||
			!runtimeSourceIdentity(mapping.HistoricalItemRef, 1024) || !runtimeSourceIdentity(mapping.AcceptanceItemRef, 1024) {
			return fmt.Errorf("evidence purpose mapping requires unique distinct original DoD endpoints")
		}
		seen[mapping.HistoricalItemRef] = true
	}
	// An acceptance endpoint cannot itself defer acceptance. This excludes both
	// cycles and transitive guessing about a terminal ordinary proof source.
	for _, mapping := range inventory.EvidencePurposeMappings {
		if seen[mapping.AcceptanceItemRef] {
			return fmt.Errorf("evidence purpose acceptance source must be ordinary, not another deferred source")
		}
	}
	return nil
}

// Pure pre-seal applicability check. Mapping is an explicit specification
// opt-in, not inferred from paths, operation names, or criterion prose. Both
// original declarations remain unchanged and must independently be executable.
func ValidateRuntimeEvidencePurposeMappings(inventory RuntimeSourceObligationInventory, profiles []RuntimeProfileContract) error {
	if err := validateRequiredRuntimeEvidencePurposes(inventory.EvidencePurposeMappings, profiles); err != nil {
		return err
	}
	if len(inventory.EvidencePurposeMappings) == 0 {
		return nil
	}
	if err := validateRuntimeEvidencePurposeMappingSources(inventory); err != nil {
		return err
	}
	sources := map[string]RuntimeDefinitionSource{}
	for _, source := range inventory.Sources {
		sources[source.CandidateItemRef] = source
	}
	for _, mapping := range inventory.EvidencePurposeMappings {
		history, acceptance := sources[mapping.HistoricalItemRef], sources[mapping.AcceptanceItemRef]
		if mapping.ContractRef == RuntimeProfileDeliverableEvidencePurposeV2 {
			if err := validateRuntimeProfileEvidencePurposeSource(mapping, profiles); err != nil {
				return err
			}
			history = runtimeProfileEvidencePurposeSource(mapping)
		}
		if err := validateRuntimeEvidencePurposePolicies(mapping, history, acceptance, profiles); err != nil {
			return err
		}
	}
	return nil
}

// Adds only the explicit historical predecessor of the confirmed acceptance
// source. It never adds the inverse (which would create an acceptance cycle).
func CompileRuntimeEvidencePurposeMappingCoverage(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) ([]RuntimeCoverageItem, error) {
	inventory = inventory.Clone()
	if err := inventory.Validate(); err != nil {
		return nil, err
	}
	return compileRuntimeEvidencePurposeMappingCoverageValidated(inventory, items)
}

// Only the exact detached inventory already validated in this invocation may
// reach this helper. No validation proof escapes to callers or future loads.
func compileRuntimeEvidencePurposeMappingCoverageValidated(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) ([]RuntimeCoverageItem, error) {
	result := slices.Clone(items)
	for n := range result {
		result[n] = result[n].Normalized()
		// Recompile the entire declaration from the original inventory; never
		// trust a caller-supplied source projection, even with a new seed hash.
		result[n].HistoricalAcceptance = nil
	}
	sources := map[string]RuntimeDefinitionSource{}
	for _, source := range RuntimeEvidencePurposeSources(inventory) {
		sources[source.CandidateItemRef] = source
	}
	resolve := func(ref string) (int, error) {
		source := sources[ref]
		index, matches := -1, 0
		for n, item := range result {
			if item.SourceScope == source.SourceScope && item.SourceID == source.SourceID && item.TargetText == strings.TrimSpace(source.Text) {
				index, matches = n, matches+1
			}
		}
		if matches != 1 {
			return -1, fmt.Errorf("evidence purpose mapping lacks unique exact original coverage endpoint")
		}
		return index, nil
	}
	for _, mapping := range inventory.EvidencePurposeMappings {
		history, err := resolve(mapping.HistoricalItemRef)
		if err != nil {
			return nil, err
		}
		acceptance, err := resolve(mapping.AcceptanceItemRef)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(result[acceptance].PrerequisiteCoverageWorkItemIDs, result[history].ID) {
			result[acceptance].PrerequisiteCoverageWorkItemIDs = append(result[acceptance].PrerequisiteCoverageWorkItemIDs, result[history].ID)
		}
		result[acceptance] = result[acceptance].Normalized()
		checks, err := compileRuntimeSourceObligationChecksValidated(inventory, result[acceptance])
		if err != nil {
			return nil, err
		}
		result[history].HistoricalAcceptance = &RuntimeHistoricalAcceptanceSource{
			ContractRef: mapping.ContractRef, InventoryHash: inventory.Hash,
			HistoricalItemRef: mapping.HistoricalItemRef, AcceptanceItemRef: mapping.AcceptanceItemRef,
			HistoricalCoverageWorkItemID: result[history].ID, AcceptanceCoverageWorkItemID: result[acceptance].ID,
			HistoricalTargetTextHash: runtimeHistoricalAcceptanceTextHash(sources[mapping.HistoricalItemRef].Text),
			AcceptanceTargetTextHash: runtimeHistoricalAcceptanceTextHash(sources[mapping.AcceptanceItemRef].Text),
			AcceptanceChecksHash:     runtimeHistoricalAcceptanceChecksHash(checks),
		}
	}
	if err := ValidateRuntimeCoverageItemDependencies(result); err != nil {
		return nil, err
	}
	if err := validateRuntimeHistoricalAcceptanceDependencyClosure(result); err != nil {
		return nil, err
	}
	return result, nil
}

// The ordinary source graph alone misses a credit cycle: H -> intermediate ->
// consumer is acyclic, but intermediate requires H's terminal acceptance while
// H can only settle after consumer. Only the exact mapped consumer may consume
// H's behavior progress. Check the two-purpose dependency graph without changing
// the original dependencies or granting other consumers behavioral credit.
func validateRuntimeHistoricalAcceptanceDependencyClosure(items []RuntimeCoverageItem) error {
	type node struct {
		id       common.CoverageWorkItemID
		behavior bool
	}
	consumers := make(map[common.CoverageWorkItemID]common.CoverageWorkItemID)
	for _, item := range items {
		if item.HistoricalAcceptance != nil {
			consumers[item.ID] = item.HistoricalAcceptance.AcceptanceCoverageWorkItemID
		}
	}
	if len(consumers) == 0 {
		return nil
	}
	dependencies := make(map[node][]node)
	for _, item := range items {
		execution := node{id: item.ID}
		if consumer, mapped := consumers[item.ID]; mapped {
			execution.behavior = true
			dependencies[node{id: item.ID}] = []node{execution, {id: consumer}}
		}
		for _, prerequisite := range item.PrerequisiteCoverageWorkItemIDs {
			dependency := node{id: prerequisite}
			if consumers[prerequisite] == item.ID {
				dependency.behavior = true
			}
			dependencies[execution] = append(dependencies[execution], dependency)
		}
	}
	visited, visiting := make(map[node]bool), make(map[node]bool)
	var visit func(node) bool
	visit = func(current node) bool {
		if visiting[current] {
			return false
		}
		if visited[current] {
			return true
		}
		visiting[current] = true
		for _, prerequisite := range dependencies[current] {
			if !visit(prerequisite) {
				return false
			}
		}
		delete(visiting, current)
		visited[current] = true
		return true
	}
	for _, item := range items {
		if !visit(node{id: item.ID}) {
			return fmt.Errorf("evidence purpose mapping contains deferred acceptance dependency cycle")
		}
	}
	return nil
}

func ValidateRuntimeEvidencePurposeMappingCoverage(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) error {
	// Retain the legacy empty-mapping branch: it checks only that no coverage
	// declares an unconfirmed historical source. Nonempty mapping validates
	// the entire inventory before any compilation.
	if len(inventory.EvidencePurposeMappings) != 0 {
		inventory = inventory.Clone()
		if err := inventory.Validate(); err != nil {
			return err
		}
	}
	return validateRuntimeEvidencePurposeMappingCoverageValidated(inventory, items)
}

func validateRuntimeEvidencePurposeMappingCoverageValidated(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) error {
	if len(inventory.EvidencePurposeMappings) == 0 {
		for _, item := range items {
			if item.HistoricalAcceptance != nil {
				return fmt.Errorf("historical acceptance source has no confirmed mapping")
			}
		}
		return nil
	}
	compiled, err := compileRuntimeEvidencePurposeMappingCoverageValidated(inventory, items)
	if err != nil {
		return err
	}
	for n := range items {
		left, right := compiled[n].HistoricalAcceptance, items[n].HistoricalAcceptance
		if (left == nil) != (right == nil) || (left != nil && *left != *right) {
			return fmt.Errorf("evidence purpose source differs from confirmed original mapping")
		}
		if !slices.Equal(compiled[n].PrerequisiteCoverageWorkItemIDs, items[n].PrerequisiteCoverageWorkItemIDs) {
			return fmt.Errorf("evidence purpose mapping omitted exact historical predecessor")
		}
	}
	return nil
}

func validateRuntimeEvidencePurposePolicies(mapping RuntimeEvidencePurposeMapping, history, acceptance RuntimeDefinitionSource, profiles []RuntimeProfileContract) error {
	effect, err := CompileRuntimeSourceEvidencePolicy(history.SourceScope, history.SourceID, profiles, nil)
	if err != nil {
		return err
	}
	current, err := CompileRuntimeSourceEvidencePolicy(acceptance.SourceScope, acceptance.SourceID, profiles, nil)
	if err != nil {
		return err
	}
	if !slices.Equal(effect.RequiredEvidenceKinds, []string{EvidenceKindStateChange}) || effect.ImplementationWorkPolicy != nil ||
		effect.ValidationFailureRepairConvergence != nil || len(effect.AcceptedStepKinds) == 0 {
		return fmt.Errorf("evidence purpose historical source must retain an ordinary state-change-only declaration")
	}
	for _, kind := range effect.AcceptedStepKinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.HistoricalEffectProofContractRef == "" {
			return fmt.Errorf("evidence purpose historical source lacks published owner purpose contract")
		}
	}
	if !slices.Contains(current.RequiredEvidenceKinds, EvidenceKindValidation) || (current.ImplementationWorkPolicy != nil && mapping.ContractRef != RuntimeProfileDeliverableEvidencePurposeV2) ||
		!slices.Contains(current.AcceptedStepKinds, RuntimeBlockKindManagedValidationSession) {
		return fmt.Errorf("evidence purpose acceptance source requires independent ordinary owner-attested validation")
	}
	return nil
}

// RuntimeEvidencePurposeAcceptanceEligible qualifies a single proposed edge's
// declarations only. Required-set completeness and full graph are separate gates.
func RuntimeEvidencePurposeAcceptanceEligible(m RuntimeEvidencePurposeMapping, profiles []RuntimeProfileContract, sourceID string) bool {
	if m.ContractRef != RuntimeProfileDeliverableEvidencePurposeV2 || validateRuntimeProfileEvidencePurposeSource(m, profiles) != nil || sourceID == "" {
		return false
	}
	return validateRuntimeEvidencePurposePolicies(m, runtimeProfileEvidencePurposeSource(m), RuntimeDefinitionSource{SourceScope: "dodItem", SourceID: sourceID}, profiles) == nil
}

// ValidateRuntimeEvidencePurposePresealCoverage checks the full dependency graph
// before admission. No compiled binding, proof, or validation token is returned.
func ValidateRuntimeEvidencePurposePresealCoverage(i RuntimeSourceObligationInventory, profiles []RuntimeProfileContract, items []RuntimeCoverageItem) error {
	if err := ValidateRuntimeEvidencePurposeMappings(i, profiles); err != nil {
		return err
	}
	result := slices.Clone(items)
	for n := range result {
		result[n] = result[n].Normalized()
	}
	sources := map[string]RuntimeDefinitionSource{}
	for _, s := range RuntimeEvidencePurposeSources(i) {
		sources[s.CandidateItemRef] = s
	}
	resolve := func(ref string) (int, error) {
		source := sources[ref]
		idx, count := -1, 0
		for n, item := range result {
			if item.SourceScope == source.SourceScope && item.SourceID == source.SourceID && item.TargetText == strings.TrimSpace(source.Text) {
				idx = n
				count++
			}
		}
		if count != 1 {
			return -1, fmt.Errorf("purpose dependency graph requires unique original source %s", ref)
		}
		return idx, nil
	}
	for n := range result {
		result[n].HistoricalAcceptance = nil
	}
	for _, m := range i.EvidencePurposeMappings {
		h, err := resolve(m.HistoricalItemRef)
		if err != nil {
			return err
		}
		a, err := resolve(m.AcceptanceItemRef)
		if err != nil {
			return err
		}
		if !slices.Contains(result[a].PrerequisiteCoverageWorkItemIDs, result[h].ID) {
			result[a].PrerequisiteCoverageWorkItemIDs = append(result[a].PrerequisiteCoverageWorkItemIDs, result[h].ID)
		}
		// Only the dependency endpoint is needed for this local two-purpose graph.
		// No inventory hash, checks, binding or historical proof is manufactured.
		result[h].HistoricalAcceptance = &RuntimeHistoricalAcceptanceSource{AcceptanceCoverageWorkItemID: result[a].ID}
	}
	if err := ValidateRuntimeCoverageItemDependencies(result); err != nil {
		return err
	}
	return validateRuntimeHistoricalAcceptanceDependencyClosure(result)
}
