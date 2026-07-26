package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCandidateContractManifestRevision identifies the canonical pure
// projection used by the provider renderer, strict output boundary and
// correction renderer. It is deliberately separate from the executable
// operation-contract revision: this value tracks the candidate wire shape,
// while RuntimeOperationContractRevision tracks executable owner semantics.
const RuntimeCandidateContractManifestRevision = "candidate-contract-manifest-v1"

// RuntimeCandidateContractManifest is an immutable-value projection of one
// sealed RuntimeDecision. It contains no provider request, prompt content,
// aggregate mutation or action selection. Consumers must derive their schema,
// output instruction and typed diagnostics from this value rather than keep a
// second operation-field or parameter whitelist.
type RuntimeCandidateContractManifest struct {
	Revision                   string
	Hash                       string
	Action                     RuntimeAction
	RequiredTopLevelFields     []string
	AllowedTopLevelFields      []string
	ForbiddenTopLevelFields    []string
	RequireCoverageWorkItemIDs bool
	CoverageWorkItemIDs        []common.CoverageWorkItemID
	TargetRefs                 []string
	DependenciesAllowed        bool
	RequiredOperationSequence  []RuntimeRequiredOperationVariant
	Operations                 []RuntimeCandidateOperationManifest
}

// RuntimeCandidateOperationManifest is the canonical operation-level contract
// for an allowed kind in the current sealed decision.
type RuntimeCandidateOperationManifest struct {
	Kind                        string
	AllowedOperationFields      []string
	AllowedVariantIDs           []string
	Parameters                  []RuntimeOperationParameterContract
	RequireOneOfParameters      []string
	RequireTargetRefs           bool
	ForbiddenParameters         []string
	Variants                    []RuntimeOperationVariantContract
	Rules                       []string
	AllowsDynamicParameters     bool
	DynamicParameterPrefixes    []string
	RequiredOperationReferences []string
}

// BuildRuntimeCandidateContractManifest creates the single pure contract
// projection for a sealed decision. TargetSelection can only narrow operation
// kinds for a File owner creation/scope observation; it cannot create a target
// or change the decision.
func BuildRuntimeCandidateContractManifest(decision RuntimeDecision, selection *RuntimeTargetSelection) (RuntimeCandidateContractManifest, error) {
	required := decision.RequiredOutput.Normalized()
	manifest := RuntimeCandidateContractManifest{
		Revision:                  RuntimeCandidateContractManifestRevision,
		Action:                    decision.Action,
		CoverageWorkItemIDs:       append([]common.CoverageWorkItemID(nil), required.CoverageWorkItemIDs...),
		TargetRefs:                append([]string(nil), required.TargetRefs...),
		RequiredOperationSequence: cloneRuntimeRequiredOperationVariants(required.RequiredOperationSequence),
	}
	manifest.RequiredTopLevelFields, manifest.AllowedTopLevelFields, manifest.ForbiddenTopLevelFields = runtimeCandidateEnvelopeFields(decision.Action)
	switch decision.Action {
	case RuntimeActionProduceStateChange, RuntimeActionProduceValidation:
		if required.Kind == RequiredOutputKindNone {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("candidate manifest action %q requires a sealed required output", decision.Action)
		}
		manifest.RequireCoverageWorkItemIDs = true
		manifest.DependenciesAllowed = runtimeRequiredOperationSequenceHasDependency(required.RequiredOperationSequence)
		kinds := RuntimeCandidateAllowedOperationKinds(decision, selection)
		if len(kinds) == 0 {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("candidate manifest action %q has no allowed operation kinds", decision.Action)
		}
		for _, kind := range kinds {
			operation, err := runtimeCandidateOperationManifestForKind(kind, required)
			if err != nil {
				return RuntimeCandidateContractManifest{}, err
			}
			manifest.Operations = append(manifest.Operations, operation)
		}
	case RuntimeActionProduceAssessment, RuntimeActionRequestGovernance:
		// These actions have a closed non-operation envelope. The renderer still
		// consumes the manifest so its top-level field rules are not duplicated.
	default:
		return RuntimeCandidateContractManifest{}, fmt.Errorf("runtime action %q does not accept a candidate manifest", decision.Action)
	}
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
	return manifest, nil
}

// ValidateRuntimeCandidateContractManifestForDecision verifies that a manifest
// carried across an owner boundary is the exact pure projection of the sealed
// decision and its persisted target-selection fact. It is an integrity check,
// not a second schema compiler: renderer, decoder and admission consume the
// supplied manifest after this check succeeds.
func ValidateRuntimeCandidateContractManifestForDecision(manifest RuntimeCandidateContractManifest, decision RuntimeDecision, selection *RuntimeTargetSelection) error {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return fmt.Errorf("runtime candidate contract manifest is invalid")
	}
	expected, err := BuildRuntimeCandidateContractManifest(decision, selection)
	if err != nil {
		return err
	}
	if manifest.Hash != expected.Hash {
		return fmt.Errorf("runtime candidate contract manifest does not match sealed authority")
	}
	return nil
}

// RuntimeCandidateAllowedOperationKinds is the owner-neutral C3 narrowing
// shared by every candidate-contract consumer. A File creation observation
// only admits durable File kinds. A File scope additionally admits a kind
// whose descriptor explicitly declares that it can act on an existing,
// File-owner-grounded target. It never grants a target or chooses an action.
func RuntimeCandidateAllowedOperationKinds(decision RuntimeDecision, selection *RuntimeTargetSelection) []string {
	required := decision.RequiredOutput.Normalized()
	kinds := append([]string(nil), required.AcceptedKinds...)
	if decision.Action != RuntimeActionProduceStateChange || selection == nil {
		return normalizeStrings(kinds)
	}
	value := selection.Normalized()
	if value.OwnerModuleID != RuntimeModuleFile ||
		(value.Disposition != RuntimeTargetSelectionCreate && value.Disposition != RuntimeTargetSelectionScope) {
		return normalizeStrings(kinds)
	}
	seen := make(map[string]struct{}, len(kinds)+len(required.RequiredOperationSequence))
	ret := make([]string, 0, len(kinds)+len(required.RequiredOperationSequence))
	for _, raw := range kinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(raw)
		if !ok || !RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
			continue
		}
		allowsScopeKind := value.Disposition == RuntimeTargetSelectionScope && descriptor.AllowsGroundedWorkspaceTarget
		if descriptor.OwnerModuleID != RuntimeModuleFile && !allowsScopeKind {
			continue
		}
		if _, duplicate := seen[descriptor.Kind]; duplicate {
			continue
		}
		seen[descriptor.Kind] = struct{}{}
		ret = append(ret, descriptor.Kind)
	}
	for _, requirement := range required.RequiredOperationSequence {
		descriptor, ok := RuntimeOperationDescriptorForKind(requirement.Kind)
		if !ok {
			continue
		}
		if _, duplicate := seen[descriptor.Kind]; duplicate {
			continue
		}
		seen[descriptor.Kind] = struct{}{}
		ret = append(ret, descriptor.Kind)
	}
	sort.Strings(ret)
	return ret
}

func (m RuntimeCandidateContractManifest) Normalized() RuntimeCandidateContractManifest {
	m.Revision = strings.TrimSpace(m.Revision)
	m.Hash = strings.TrimSpace(m.Hash)
	m.Action = RuntimeAction(strings.TrimSpace(string(m.Action)))
	m.RequiredTopLevelFields = normalizeStrings(m.RequiredTopLevelFields)
	m.AllowedTopLevelFields = normalizeStrings(m.AllowedTopLevelFields)
	m.ForbiddenTopLevelFields = normalizeStrings(m.ForbiddenTopLevelFields)
	m.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(m.CoverageWorkItemIDs)
	m.TargetRefs = normalizeStrings(m.TargetRefs)
	m.RequiredOperationSequence = cloneRuntimeRequiredOperationVariants(normalizeRequiredOperationVariants(m.RequiredOperationSequence))
	operations := make([]RuntimeCandidateOperationManifest, 0, len(m.Operations))
	seen := make(map[string]struct{}, len(m.Operations))
	for _, operation := range m.Operations {
		operation = operation.Normalized()
		if operation.Kind == "" {
			continue
		}
		if _, exists := seen[operation.Kind]; exists {
			continue
		}
		seen[operation.Kind] = struct{}{}
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(left, right int) bool { return operations[left].Kind < operations[right].Kind })
	m.Operations = operations
	return m
}

func (m RuntimeCandidateOperationManifest) Normalized() RuntimeCandidateOperationManifest {
	m.Kind = CanonicalRuntimeBlockKind(m.Kind)
	m.AllowedOperationFields = normalizeStrings(m.AllowedOperationFields)
	m.AllowedVariantIDs = normalizeStrings(m.AllowedVariantIDs)
	m.Parameters = cloneRuntimeOperationParameterContracts(m.Parameters)
	m.RequireOneOfParameters = cloneRuntimeCandidateContractStrings(m.RequireOneOfParameters)
	m.ForbiddenParameters = cloneRuntimeCandidateContractStrings(m.ForbiddenParameters)
	m.Variants = cloneRuntimeOperationVariantContracts(m.Variants)
	m.Rules = cloneRuntimeCandidateContractStrings(m.Rules)
	m.DynamicParameterPrefixes = normalizeStrings(m.DynamicParameterPrefixes)
	m.RequiredOperationReferences = normalizeStrings(m.RequiredOperationReferences)
	return m
}

func (m RuntimeCandidateContractManifest) Clone() RuntimeCandidateContractManifest {
	return m.Normalized()
}

// StableRuntimeCandidateContractManifestHash excludes the Hash field itself
// and uses only normalized typed values. It is safe to expose in request audit
// metadata; it contains neither provider payload nor workspace content.
func StableRuntimeCandidateContractManifestHash(manifest RuntimeCandidateContractManifest) string {
	manifest = manifest.Normalized()
	manifest.Hash = ""
	h := sha256.New()
	writeHashString(h, manifest.Revision)
	writeHashString(h, string(manifest.Action))
	writeHashStrings(h, manifest.RequiredTopLevelFields)
	writeHashStrings(h, manifest.AllowedTopLevelFields)
	writeHashStrings(h, manifest.ForbiddenTopLevelFields)
	writeHashString(h, fmt.Sprintf("%t", manifest.RequireCoverageWorkItemIDs))
	coverage := make([]string, 0, len(manifest.CoverageWorkItemIDs))
	for _, id := range manifest.CoverageWorkItemIDs {
		coverage = append(coverage, string(id))
	}
	writeHashStrings(h, coverage)
	writeHashStrings(h, manifest.TargetRefs)
	writeHashString(h, fmt.Sprintf("%t", manifest.DependenciesAllowed))
	for _, requirement := range manifest.RequiredOperationSequence {
		writeHashString(h, requirement.RequirementID)
		writeHashString(h, requirement.Kind)
		writeHashString(h, requirement.VariantID)
		writeHashStrings(h, requirement.DependsOnRequirementIDs)
		writeHashString(h, requirement.Reason)
	}
	for _, operation := range manifest.Operations {
		writeHashString(h, operation.Kind)
		writeHashStrings(h, operation.AllowedOperationFields)
		writeHashStrings(h, operation.AllowedVariantIDs)
		for _, parameter := range operation.Parameters {
			writeHashString(h, parameter.Name)
			writeHashString(h, string(parameter.Type))
			writeHashString(h, fmt.Sprintf("%t", parameter.Required))
			writeHashString(h, parameter.Description)
		}
		writeHashStrings(h, operation.RequireOneOfParameters)
		writeHashString(h, fmt.Sprintf("%t", operation.RequireTargetRefs))
		writeHashStrings(h, operation.ForbiddenParameters)
		for _, variant := range operation.Variants {
			writeHashString(h, variant.ID)
			for _, discriminator := range variant.Discriminators {
				writeHashString(h, discriminator.Name)
				writeHashString(h, discriminator.EqualsString)
			}
			writeHashStrings(h, variant.RequiredParameters)
			writeHashStrings(h, variant.RequiredNonEmpty)
			writeHashStrings(h, variant.ForbiddenParameters)
			for _, pair := range variant.DistinctParameterPairs {
				writeHashString(h, pair.Left)
				writeHashString(h, pair.Right)
			}
			writeHashString(h, string(variant.EffectScope))
			writeHashString(h, string(variant.IdempotencyClass))
		}
		writeHashStrings(h, operation.Rules)
		writeHashString(h, fmt.Sprintf("%t", operation.AllowsDynamicParameters))
		writeHashStrings(h, operation.DynamicParameterPrefixes)
		writeHashStrings(h, operation.RequiredOperationReferences)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func runtimeCandidateEnvelopeFields(action RuntimeAction) (required, allowed, forbidden []string) {
	forbidden = []string{"candidate", "candidates", "executionGraph", "convergenceState", "focus", "authority", "transaction", "outputMode", "result", "runtimeState"}
	switch action {
	case RuntimeActionProduceStateChange, RuntimeActionProduceValidation:
		return []string{"operations"}, []string{"operations"}, append(forbidden, "id", "kind", "requiredOperationRef", "dependsOn", "coverageWorkItemIds", "targetRefs", "parameters")
	case RuntimeActionProduceAssessment:
		return []string{"focusId", "coverageWorkItemIds", "disposition", "summary"}, []string{"focusId", "sliceId", "coverageWorkItemIds", "disposition", "items", "gap", "summary", "evidenceRefs", "artifactRefs"}, forbidden
	case RuntimeActionRequestGovernance:
		return []string{"externalDependency"}, []string{"externalDependency"}, append(forbidden, "kind", "subjectRef", "reason")
	default:
		return nil, nil, forbidden
	}
}

func runtimeCandidateOperationManifestForKind(kind string, required RequiredOutputContract) (RuntimeCandidateOperationManifest, error) {
	contract, ok := RuntimeOperationContractForKind(kind)
	if !ok {
		return RuntimeCandidateOperationManifest{}, fmt.Errorf("candidate manifest operation kind %q has no operation contract", kind)
	}
	variants, allowedVariantIDs, err := runtimeCandidateManifestVariants(contract, required)
	if err != nil {
		return RuntimeCandidateOperationManifest{}, err
	}
	references := make([]string, 0)
	for _, requirement := range required.RequiredOperationSequence {
		if CanonicalRuntimeBlockKind(requirement.Kind) == contract.Kind && strings.TrimSpace(requirement.RequirementID) != "" {
			references = append(references, requirement.RequirementID)
		}
	}
	fields := []string{"id", "kind", "coverageWorkItemIds", "targetRefs", "parameters"}
	if len(references) != 0 {
		fields = append(fields, "requiredOperationRef")
	}
	if runtimeRequiredOperationSequenceHasDependency(required.RequiredOperationSequence) {
		fields = append(fields, "dependsOn")
	}
	return RuntimeCandidateOperationManifest{
		Kind:                        contract.Kind,
		AllowedOperationFields:      fields,
		AllowedVariantIDs:           allowedVariantIDs,
		Parameters:                  cloneRuntimeOperationParameterContracts(contract.Parameters),
		RequireOneOfParameters:      append([]string(nil), contract.RequireOneOfParameters...),
		RequireTargetRefs:           contract.RequireTargetRefs,
		ForbiddenParameters:         append([]string(nil), contract.ForbiddenParameters...),
		Variants:                    variants,
		Rules:                       append([]string(nil), contract.Rules...),
		AllowsDynamicParameters:     runtimeCandidateManifestAllowsDynamicParameters(contract.Kind),
		DynamicParameterPrefixes:    runtimeCandidateManifestDynamicParameterPrefixes(contract.Kind),
		RequiredOperationReferences: references,
	}.Normalized(), nil
}

func runtimeCandidateManifestVariants(contract RuntimeOperationContract, required RequiredOutputContract) ([]RuntimeOperationVariantContract, []string, error) {
	allowed := make(map[string]struct{})
	for _, ref := range required.AcceptedOperationVariants {
		if ref.Kind == contract.Kind {
			allowed[ref.VariantID] = struct{}{}
		}
	}
	for _, requirement := range required.RequiredOperationSequence {
		if CanonicalRuntimeBlockKind(requirement.Kind) == contract.Kind && strings.TrimSpace(requirement.VariantID) != "" {
			allowed[strings.TrimSpace(requirement.VariantID)] = struct{}{}
		}
	}
	variants := make([]RuntimeOperationVariantContract, 0, len(contract.Variants))
	for _, variant := range contract.Variants {
		if len(allowed) != 0 {
			if _, ok := allowed[variant.ID]; !ok {
				continue
			}
		}
		variants = append(variants, cloneRuntimeOperationVariantContract(variant))
	}
	if len(allowed) != 0 && len(variants) != len(allowed) {
		return nil, nil, fmt.Errorf("candidate manifest operation kind %q references an unknown sealed variant", contract.Kind)
	}
	ids := make([]string, 0, len(variants))
	for _, variant := range variants {
		ids = append(ids, variant.ID)
	}
	return variants, normalizeStrings(ids), nil
}

func runtimeRequiredOperationSequenceHasDependency(values []RuntimeRequiredOperationVariant) bool {
	for _, value := range values {
		if len(value.DependsOnRequirementIDs) != 0 {
			return true
		}
	}
	return false
}

func runtimeCandidateManifestAllowsDynamicParameters(kind string) bool {
	return len(runtimeCandidateManifestDynamicParameterPrefixes(kind)) != 0
}

func runtimeCandidateManifestDynamicParameterPrefixes(kind string) []string {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindMCPToolCall:
		return []string{"input."}
	case StepKindMCPPromptGet:
		return []string{"arguments."}
	default:
		return nil
	}
}

func cloneRuntimeOperationParameterContracts(values []RuntimeOperationParameterContract) []RuntimeOperationParameterContract {
	return append([]RuntimeOperationParameterContract(nil), values...)
}

func cloneRuntimeOperationVariantContract(value RuntimeOperationVariantContract) RuntimeOperationVariantContract {
	value.Discriminators = append([]RuntimeOperationParameterPredicate(nil), value.Discriminators...)
	value.RequiredParameters = cloneRuntimeCandidateContractStrings(value.RequiredParameters)
	value.RequiredNonEmpty = cloneRuntimeCandidateContractStrings(value.RequiredNonEmpty)
	value.ForbiddenParameters = cloneRuntimeCandidateContractStrings(value.ForbiddenParameters)
	value.DistinctParameterPairs = append([]RuntimeOperationParameterPair(nil), value.DistinctParameterPairs...)
	return value
}

func cloneRuntimeOperationVariantContracts(values []RuntimeOperationVariantContract) []RuntimeOperationVariantContract {
	ret := make([]RuntimeOperationVariantContract, 0, len(values))
	for _, value := range values {
		ret = append(ret, cloneRuntimeOperationVariantContract(value))
	}
	return ret
}

func cloneRuntimeCandidateContractStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func cloneRuntimeRequiredOperationVariants(values []RuntimeRequiredOperationVariant) []RuntimeRequiredOperationVariant {
	ret := make([]RuntimeRequiredOperationVariant, 0, len(values))
	for _, value := range values {
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		value.DependsOnRequirementIDs = normalizeStrings(value.DependsOnRequirementIDs)
		value.Reason = strings.TrimSpace(value.Reason)
		ret = append(ret, value)
	}
	return ret
}
