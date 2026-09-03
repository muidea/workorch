package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeCandidateOutputContractProjection struct {
	State                       runtimecontract.ConvergenceState              `json:"state"`
	EffectPolicy                runtimeCandidateEffectPolicyProjection        `json:"effectPolicy"`
	ManifestHash                string                                        `json:"manifestHash,omitempty"`
	WireContractHash            string                                        `json:"wireContractHash,omitempty"`
	MaxOperationCount           int                                           `json:"maxOperationCount,omitempty"`
	OperationSetPolicy          runtimeCandidateOperationSetPolicyProjection  `json:"operationSetPolicy,omitempty"`
	Schema                      json.RawMessage                               `json:"schema"`
	StructuredOutputInstruction string                                        `json:"structuredOutputInstruction"`
	CanonicalWireLayout         string                                        `json:"canonicalWireLayout,omitempty"`
	WireRules                   []runtimeCandidateWireRuleProjection          `json:"wireRules,omitempty"`
	ForbiddenTopLevelFields     []string                                      `json:"forbiddenTopLevelFields"`
	OperationContracts          []runtimeCandidateOperationContractProjection `json:"operationContracts,omitempty"`
}

type runtimeCandidateOperationSetPolicyProjection struct {
	MaxOperationsPerOwnerTarget           int  `json:"maxOperationsPerOwnerTarget,omitempty"`
	AllowRequiredOperationSequenceOverlap bool `json:"allowRequiredOperationSequenceOverlap,omitempty"`
}

type runtimeCandidateWireRuleProjection struct {
	RequiredOperationRef           string   `json:"requiredOperationRef,omitempty"`
	CompiledKind                   string   `json:"compiledKind"`
	CompiledVariantID              string   `json:"compiledVariantId,omitempty"`
	ProviderFields                 []string `json:"providerFields"`
	RequiredProviderFields         []string `json:"requiredProviderFields,omitempty"`
	ProviderParameterNames         []string `json:"providerParameterNames,omitempty"`
	RequiredProviderParameterNames []string `json:"requiredProviderParameterNames,omitempty"`
	RequiredNonEmptyParameterNames []string `json:"requiredNonEmptyParameterNames,omitempty"`
	LocallyDerivedFields           []string `json:"locallyDerivedFields,omitempty"`
}

type runtimeCandidateWireLayoutOperation struct {
	RequiredOperationRef string                                `json:"requiredOperationRef,omitempty"`
	Kind                 string                                `json:"kind,omitempty"`
	BindingRefs          []string                              `json:"bindingRefs,omitempty"`
	TargetRefs           []string                              `json:"targetRefs,omitempty"`
	Parameters           []runtimeCandidateWireLayoutParameter `json:"parameters,omitempty"`
}

type runtimeCandidateWireLayoutParameter struct {
	Name       string   `json:"name"`
	String     *string  `json:"string,omitempty"`
	Integer    *int64   `json:"integer,omitempty"`
	Boolean    *bool    `json:"boolean,omitempty"`
	StringList []string `json:"stringList,omitempty"`
}

type runtimeCandidateWireLayoutPayload struct {
	Operations []runtimeCandidateWireLayoutOperation `json:"operations"`
}

type runtimeCandidateEffectPolicyProjection struct {
	ExecutableOperationsAllowed   bool                                 `json:"executableOperationsAllowed"`
	AllowedOperationEffects       []runtimecontract.RuntimeEffectScope `json:"allowedOperationEffects,omitempty"`
	AllowedAcceptedReceiptEffects []runtimecontract.RuntimeEffectScope `json:"allowedAcceptedReceiptEffects,omitempty"`
}

type runtimeCandidateOperationContractProjection struct {
	Kind                        string                                                `json:"kind"`
	OwnerModuleID               string                                                `json:"ownerModuleId,omitempty"`
	AllowedOperationFields      []string                                              `json:"allowedOperationFields,omitempty"`
	RequiredOperationFields     []string                                              `json:"requiredOperationFields,omitempty"`
	AllowedVariantIDs           []string                                              `json:"allowedVariantIds,omitempty"`
	Parameters                  []runtimeCandidateOperationParameterProjection        `json:"parameters"`
	RequireOneOfParameters      []string                                              `json:"requireOneOfParameters,omitempty"`
	TargetCardinality           runtimecontract.RuntimeOperationTargetCardinality     `json:"targetCardinality,omitempty"`
	RequireTargetRefs           bool                                                  `json:"requireTargetRefs,omitempty"`
	ForbiddenParameters         []string                                              `json:"forbiddenParameters,omitempty"`
	CandidateConstraints        []runtimecontract.RuntimeOperationCandidateConstraint `json:"candidateConstraints,omitempty"`
	Variants                    []runtimeCandidateOperationVariantProjection          `json:"variants,omitempty"`
	Rules                       []string                                              `json:"rules,omitempty"`
	AllowsDynamicParameters     bool                                                  `json:"allowsDynamicParameters,omitempty"`
	DynamicParameterPrefixes    []string                                              `json:"dynamicParameterPrefixes,omitempty"`
	RequiredOperationReferences []string                                              `json:"requiredOperationReferences,omitempty"`
	BindingAuthorizations       []runtimeCandidateBindingAuthorizationProjection      `json:"bindingAuthorizations,omitempty"`
	AcceptedEffectScopes        []runtimecontract.RuntimeEffectScope                  `json:"acceptedEffectScopes,omitempty"`
	RequiredEffectScopes        []runtimecontract.RuntimeEffectScope                  `json:"requiredEffectScopes,omitempty"`
	// Target is the sealed TargetPolicy authorization compiled by the single
	// manifest compiler. Renderer/schema/instruction must consume it instead of
	// re-deriving owner inventory or inventing a second allow-list.
	Target                    runtimeCandidateOperationTargetProjection `json:"target"`
	PrerequisiteSnapshotRefs  []string                                  `json:"prerequisiteSnapshotRefs,omitempty"`
	AdmissibilitySnapshotRefs []string                                  `json:"admissibilitySnapshotRefs,omitempty"`
	// ValidationRecipeIDs exposes only selection identity. Full command, args
	// and timeout remain activation-sealed owner facts consumed by Shell; a
	// provider must never receive a second command-construction interface.
	ValidationRecipeIDs       []string `json:"validationRecipeIds,omitempty"`
	OperationContractRevision string   `json:"operationContractRevision,omitempty"`
}

type runtimeCandidateBindingAuthorizationProjection struct {
	RequiredOperationRef string   `json:"requiredOperationRef,omitempty"`
	VariantID            string   `json:"variantId,omitempty"`
	AllowedBindingRefs   []string `json:"allowedBindingRefs,omitempty"`
}

// runtimeCandidateOperationTargetProjection is the provider-facing pure view of
// one sealed TargetPolicy. It carries membership facts only; no raw owner
// error text and no domain heuristics.
type runtimeCandidateOperationTargetProjection struct {
	Policy                    runtimecontract.RuntimeOperationTargetPolicy `json:"policy"`
	ExactTargetRefs           []string                                     `json:"exactTargetRefs,omitempty"`
	OwnerScopeTargetRefs      []string                                     `json:"ownerScopeTargetRefs,omitempty"`
	OwnerSelectorRefs         []string                                     `json:"ownerSelectorRefs,omitempty"`
	ProspectiveConstraintRefs []string                                     `json:"prospectiveConstraintRefs,omitempty"`
	ProspectiveTargetRefs     []string                                     `json:"prospectiveTargetRefs,omitempty"`
	AllowedTargetRefs         []string                                     `json:"allowedTargetRefs,omitempty"`
}

type runtimeCandidateOperationParameterProjection struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type runtimeCandidateOperationVariantProjection struct {
	ID                   string                                                `json:"id"`
	Discriminators       []runtimecontract.RuntimeOperationParameterPredicate  `json:"discriminators"`
	RequiredParameters   []string                                              `json:"requiredParameters,omitempty"`
	RequiredNonEmpty     []string                                              `json:"requiredNonEmpty,omitempty"`
	ForbiddenParameters  []string                                              `json:"forbiddenParameters,omitempty"`
	CandidateConstraints []runtimecontract.RuntimeOperationCandidateConstraint `json:"candidateConstraints,omitempty"`
	EffectScope          runtimecontract.RuntimeEffectScope                    `json:"effectScope"`
	IdempotencyClass     runtimecontract.RuntimeOperationIdempotencyClass      `json:"idempotencyClass"`
}

type runtimeCandidateRetryProjection struct {
	PreviousFailure runtimecontract.RuntimeCandidateCorrectionObservation `json:"previousFailure"`
	CorrectionInput runtimeCandidateCorrectionInputProjection             `json:"correctionInput"`
	Instruction     string                                                `json:"instruction"`
}

type runtimeCandidateCorrectionInputProjection struct {
	Objectives   []runtimecontract.RuntimeCandidateCorrectionObjective `json:"objectives"`
	ManifestHash string                                                `json:"manifestHash"`
	Witness      runtimeCandidateCorrectionWitnessProjection           `json:"witness"`
}

type runtimeCandidateCorrectionWitnessProjection struct {
	Dimension               runtimecontract.RuntimeCandidateCorrectionWitnessDimension `json:"dimension"`
	OperationKind           string                                                     `json:"operationKind,omitempty"`
	Path                    string                                                     `json:"path"`
	RejectedValue           string                                                     `json:"rejectedValue,omitempty"`
	AlternativeValue        string                                                     `json:"alternativeValue"`
	RejectedShapeHash       string                                                     `json:"rejectedShapeHash,omitempty"`
	AlternativeShapeHash    string                                                     `json:"alternativeShapeHash,omitempty"`
	ConstraintRef           string                                                     `json:"constraintRef,omitempty"`
	ParameterEntryEncodings []runtimeCandidateParameterEntryEncodingProjection         `json:"parameterEntryEncodings,omitempty"`
	OperationSetViolation   *runtimecontract.RuntimeCandidateOperationSetViolation     `json:"operationSetViolation,omitempty"`
}

type runtimeCandidateParameterEntryEncodingProjection struct {
	OperationIndex         int                                                `json:"operationIndex"`
	ParameterIndex         int                                                `json:"parameterIndex"`
	OperationKind          string                                             `json:"operationKind"`
	SourceEntryPath        string                                             `json:"sourceEntryPath"`
	ContainerParameterName string                                             `json:"containerParameterName"`
	Entries                []runtimeCandidateParameterEncodingEntryProjection `json:"entries"`
}

type runtimeCandidateParameterEncodingEntryProjection struct {
	Name           string                                        `json:"name"`
	ContractType   runtimecontract.RuntimeOperationParameterType `json:"contractType"`
	WireValueField string                                        `json:"wireValueField"`
}

// runtimeTargetSelectionInstruction converts the persisted File-owner
// selection disposition into provider-facing C3 guidance. A create selection
// carries the SM-2 binding-sealed prospective path; a complete scope does not.
// Only a durable File-owner operation is admissible for creation, and the File
// owner remains the final workspace-boundary authority.
func runtimeTargetSelectionInstruction(selection *runtimecontract.RuntimeTargetSelection, state runtimecontract.ConvergenceState) string {
	if selection == nil || state != runtimecontract.ConvergenceStateNeedStateChange {
		return ""
	}
	value := selection.Normalized()
	if value.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		return ""
	}
	switch value.Disposition {
	case runtimecontract.RuntimeTargetSelectionCreate:
		if value.ProspectiveTargetRef == "" {
			return ""
		}
		return fmt.Sprintf("The File owner observed an empty workspace creation scope with SM-2 binding-sealed prospective target %q. Propose at least one durable File-owner operation whose sole targetRefs entry is exactly that path. Do not duplicate the target as an operation parameter, substitute another path, leave it blank, or use this scope to authorize Git, Shell, MCP, or another owner. The File owner will validate the path before execution.", value.ProspectiveTargetRef)
	case runtimecontract.RuntimeTargetSelectionScope:
		return "The File owner provided a bounded workspace feasibility scope, not a selected repair target. Follow any concrete target already sealed in the Focus, FailureTarget, current failure, or RepairDirective. Only when none is present, propose at least one durable File-owner operation with a concrete workspace-relative targetRefs entry; do not duplicate it as an operation parameter. That target may be a new file and remains subject to File-owner feasibility. Do not use this scope to authorize Git, Shell, MCP, or another owner."
	default:
		return ""
	}
}

func runtimeCandidateContractCatalog() string {
	return strings.Join([]string{
		"Return exactly one JSON object for the requested Runtime convergence state; it must be complete, compact, and finished in this response. The JSON object itself is the candidate; never wrap it in another object.",
		"The C3 current runtime authority's outputContract is the sole structured-output contract. Its manifestHash, wireContractHash, schema, structuredOutputInstruction, canonicalWireLayout, wireRules, and operationContracts define the complete proposal surface for this request.",
		"Emit only schema fields and manifest-authorized operations. Do not infer or add unknown fields, unrequested operations, prose, preface, suffix, or markdown fences. Do not repeat the Goal, DoD, manifest, authority, or input context.",
		"For every operation, emit only the fields listed by its wireRule. Fields listed as locallyDerivedFields are compiled by Runtime from the sealed manifest and are forbidden in provider output. Keep parameters as the last member of each operation object and close the operation and root only after parameters are complete.",
		"context.requiredState is the sole canonical decision authority. Do not invent or modify convergence state, focus, authority, transaction, or output mode.",
		"When context.repairDirective is present, obey its sealed state, effect, evidence and operation constraints. The exact fields and values remain in outputContract and the current runtime authority.",
		"context.requiredEvidenceKinds declares the current evidence obligation; it never overrides context.requiredState, context.repairDirective, or outputContract.",
		"When rejectedOperationShapes is present in the C4 execution-tail object, every listed operation hash and exact structural operation is forbidden until later accepted evidence supersedes it. Choose an admissible kind and change kind, targets, or execution-semantic parameters.",
	}, "\n")
}

func runtimeCandidateOutputContractForManifest(manifest runtimecontract.RuntimeCandidateContractManifest, wire runtimecontract.RuntimeCandidateWireContract) runtimeCandidateOutputContractProjection {
	manifest = manifest.Normalized()
	wire = wire.Normalized()
	return runtimeCandidateOutputContractProjection{
		State: manifest.State,
		EffectPolicy: runtimeCandidateEffectPolicyProjection{
			ExecutableOperationsAllowed:   manifest.EffectPolicy.ExecutableOperationsAllowed,
			AllowedOperationEffects:       append([]runtimecontract.RuntimeEffectScope(nil), manifest.EffectPolicy.AllowedOperationEffects...),
			AllowedAcceptedReceiptEffects: append([]runtimecontract.RuntimeEffectScope(nil), manifest.EffectPolicy.AllowedAcceptedReceiptEffects...),
		},
		ManifestHash:      manifest.Hash,
		WireContractHash:  wire.Hash,
		MaxOperationCount: manifest.MaxOperationCount,
		OperationSetPolicy: runtimeCandidateOperationSetPolicyProjection{
			MaxOperationsPerOwnerTarget:           manifest.OperationSetPolicy.MaxOperationsPerOwnerTarget,
			AllowRequiredOperationSequenceOverlap: manifest.OperationSetPolicy.AllowRequiredOperationSequenceOverlap,
		},
		Schema:                      json.RawMessage(runtimeCandidateSchemaForManifest(manifest)),
		StructuredOutputInstruction: runtimeCandidateStructuredOutputInstruction(manifest, wire),
		CanonicalWireLayout:         runtimeCandidateCanonicalWireLayout(manifest, wire),
		WireRules:                   runtimeCandidateWireRuleProjections(wire),
		ForbiddenTopLevelFields:     append([]string(nil), manifest.ForbiddenTopLevelFields...),
		OperationContracts:          runtimeCandidateOperationContractsForManifest(manifest.Operations, wire),
	}
}

func runtimeCandidateWireRuleProjections(wire runtimecontract.RuntimeCandidateWireContract) []runtimeCandidateWireRuleProjection {
	ret := make([]runtimeCandidateWireRuleProjection, 0, len(wire.OperationRules))
	for _, rule := range wire.OperationRules {
		rule = rule.Normalized()
		derived := make([]string, 0, 3+len(rule.DerivedParameters))
		if rule.RequiredSlot() {
			derived = append(derived, "kind")
		}
		if len(rule.DerivedBindingRefs) != 0 {
			derived = append(derived, "bindingRefs")
		}
		if len(rule.DerivedTargetRefs) != 0 {
			derived = append(derived, "targetRefs")
		}
		for _, name := range rule.DerivedParameterNames() {
			derived = append(derived, "parameters[name="+name+"]")
		}
		ret = append(ret, runtimeCandidateWireRuleProjection{
			RequiredOperationRef: rule.RequiredOperationRef, CompiledKind: rule.Kind, CompiledVariantID: rule.VariantID,
			ProviderFields: append([]string(nil), rule.ProviderFields...), RequiredProviderFields: append([]string(nil), rule.RequiredProviderFields...),
			ProviderParameterNames: append([]string(nil), rule.ProviderParameterNames...), RequiredProviderParameterNames: append([]string(nil), rule.RequiredProviderParameterNames...),
			RequiredNonEmptyParameterNames: append([]string(nil), rule.RequiredNonEmptyParameterNames...),
			LocallyDerivedFields:           derived,
		})
	}
	return ret
}

func runtimeCandidateCanonicalWireLayout(manifest runtimecontract.RuntimeCandidateContractManifest, wire runtimecontract.RuntimeCandidateWireContract) string {
	requiredRules := make([]runtimecontract.RuntimeCandidateWireOperationRule, 0, len(wire.OperationRules))
	for _, rule := range wire.OperationRules {
		if rule.RequiredSlot() {
			requiredRules = append(requiredRules, rule)
		}
	}
	if len(requiredRules) == 0 {
		return ""
	}
	operations := make([]runtimeCandidateWireLayoutOperation, 0, len(requiredRules))
	for _, rule := range requiredRules {
		rule = rule.Normalized()
		value := runtimeCandidateWireLayoutOperation{}
		if rule.RequiredSlot() {
			value.RequiredOperationRef = rule.RequiredOperationRef
		} else {
			value.Kind = rule.Kind
		}
		if runtimeStringIn(rule.RequiredProviderFields, "bindingRefs") && len(rule.AllowedBindingRefs) != 0 {
			value.BindingRefs = []string{rule.AllowedBindingRefs[0]}
		}
		if runtimeStringIn(rule.RequiredProviderFields, "targetRefs") {
			value.TargetRefs = []string{"<manifest-authorized-target>"}
		}
		operation, _ := runtimeCandidateOperationManifestForKind(manifest.Operations, rule.Kind)
		layoutParameterNames := append([]string(nil), rule.RequiredProviderParameterNames...)
		if len(rule.RequireOneOfProviderParameters) != 0 {
			layoutParameterNames = append(layoutParameterNames, rule.RequireOneOfProviderParameters[0])
		}
		for _, name := range runtimeSortedUniqueStrings(layoutParameterNames) {
			parameter, found := runtimeCandidateOperationParameterForName(operation.Parameters, name)
			if !found {
				continue
			}
			placeholder := runtimeCandidateParameterPlaceholder(parameter)
			value.Parameters = append(value.Parameters, placeholder)
		}
		operations = append(operations, value)
	}
	encoded, _ := json.Marshal(runtimeCandidateWireLayoutPayload{Operations: operations})
	return "Layout only; replace placeholder values from C1-C3 and never emit locally-derived fields: " + string(encoded)
}

func runtimeCandidateOperationParameterForName(values []runtimecontract.RuntimeOperationParameterContract, name string) (runtimecontract.RuntimeOperationParameterContract, bool) {
	for _, parameter := range values {
		if parameter.Name == name {
			return parameter, true
		}
	}
	return runtimecontract.RuntimeOperationParameterContract{}, false
}

func runtimeCandidateParameterPlaceholder(parameter runtimecontract.RuntimeOperationParameterContract) runtimeCandidateWireLayoutParameter {
	value := runtimeCandidateWireLayoutParameter{Name: parameter.Name}
	switch parameter.Type {
	case runtimecontract.RuntimeOperationParameterTypeInteger:
		placeholder := int64(0)
		value.Integer = &placeholder
	case runtimecontract.RuntimeOperationParameterTypeBoolean:
		placeholder := false
		value.Boolean = &placeholder
	case runtimecontract.RuntimeOperationParameterTypeStringList:
		value.StringList = []string{"<value>"}
	default:
		placeholder := "<value>"
		value.String = &placeholder
	}
	return value
}

func runtimeCandidateOperationContractsForManifest(values []runtimecontract.RuntimeCandidateOperationManifest, wire runtimecontract.RuntimeCandidateWireContract) []runtimeCandidateOperationContractProjection {
	ret := make([]runtimeCandidateOperationContractProjection, 0, len(values))
	for _, operation := range values {
		operation = operation.Normalized()
		allowedWireFields, requiredWireFields := runtimeCandidateWireFieldsForKind(wire, operation.Kind)
		if len(allowedWireFields) == 0 {
			allowedWireFields = append([]string(nil), operation.AllowedOperationFields...)
			requiredWireFields = append([]string(nil), operation.RequiredOperationFields...)
		}
		providerParameterNames := runtimeCandidateWireParameterNamesForKind(wire, operation.Kind)
		auxiliaryRule, hasAuxiliaryRule := runtimeCandidateWireAuxiliaryRuleForKind(wire, operation.Kind)
		auxiliaryVariantSelectable := hasAuxiliaryRule && auxiliaryRule.VariantID == ""
		target := operation.Target.Normalized()
		projection := runtimeCandidateOperationContractProjection{
			Kind:                        operation.Kind,
			OwnerModuleID:               operation.OwnerModuleID,
			AllowedOperationFields:      allowedWireFields,
			RequiredOperationFields:     requiredWireFields,
			RequireOneOfParameters:      runtimeCandidateWireOneOfParameterNamesForKind(wire, operation.Kind),
			TargetCardinality:           operation.TargetCardinality,
			RequireTargetRefs:           runtimeStringIn(requiredWireFields, "targetRefs"),
			ForbiddenParameters:         append([]string(nil), operation.ForbiddenParameters...),
			CandidateConstraints:        runtimeCandidateOperationConstraints(operation.Variants),
			Rules:                       append([]string(nil), operation.Rules...),
			AllowsDynamicParameters:     operation.AllowsDynamicParameters,
			DynamicParameterPrefixes:    append([]string(nil), operation.DynamicParameterPrefixes...),
			RequiredOperationReferences: append([]string(nil), operation.RequiredOperationReferences...),
			BindingAuthorizations:       make([]runtimeCandidateBindingAuthorizationProjection, 0, len(operation.BindingAuthorizations)),
			AcceptedEffectScopes:        append([]runtimecontract.RuntimeEffectScope(nil), operation.AcceptedEffectScopes...),
			RequiredEffectScopes:        append([]runtimecontract.RuntimeEffectScope(nil), operation.RequiredEffectScopes...),
			Target: runtimeCandidateOperationTargetProjection{
				Policy:                    target.Policy,
				ExactTargetRefs:           append([]string(nil), target.ExactTargetRefs...),
				OwnerScopeTargetRefs:      append([]string(nil), target.OwnerScopeTargetRefs...),
				OwnerSelectorRefs:         append([]string(nil), target.OwnerSelectorRefs...),
				ProspectiveConstraintRefs: append([]string(nil), target.ProspectiveConstraintRefs...),
				ProspectiveTargetRefs:     append([]string(nil), target.ProspectiveTargetRefs...),
				AllowedTargetRefs:         target.AllowedTargetRefs(),
			},
			PrerequisiteSnapshotRefs:  append([]string(nil), operation.PrerequisiteSnapshotRefs...),
			AdmissibilitySnapshotRefs: append([]string(nil), operation.AdmissibilitySnapshotRefs...),
			ValidationRecipeIDs:       runtimeValidationRecipeIDs(operation.ValidationRecipes),
			OperationContractRevision: operation.OperationContractRevision,
			Parameters:                make([]runtimeCandidateOperationParameterProjection, 0, len(operation.Parameters)),
		}
		if auxiliaryVariantSelectable {
			projection.AllowedVariantIDs = append([]string(nil), operation.AllowedVariantIDs...)
			projection.RequireOneOfParameters = append([]string(nil), operation.RequireOneOfParameters...)
		}
		for _, authorization := range operation.BindingAuthorizations {
			if !runtimeCandidateWireKindSelectsBinding(wire, operation.Kind, authorization.RequiredOperationRef) ||
				(authorization.RequiredOperationRef == "" && len(operation.Variants) != 0 && !runtimeCandidateManifestHasVariant(operation.Variants, authorization.VariantID)) {
				continue
			}
			projection.BindingAuthorizations = append(projection.BindingAuthorizations, runtimeCandidateBindingAuthorizationProjection{
				RequiredOperationRef: authorization.RequiredOperationRef,
				VariantID:            authorization.VariantID,
				AllowedBindingRefs:   append([]string(nil), authorization.AllowedBindingRefs...),
			})
		}
		for _, parameter := range operation.Parameters {
			if !runtimeStringIn(providerParameterNames, parameter.Name) {
				continue
			}
			projection.Parameters = append(projection.Parameters, runtimeCandidateOperationParameterProjection{
				Name: parameter.Name, Type: string(parameter.Type), Required: parameter.Required, Description: parameter.Description,
			})
		}
		if auxiliaryVariantSelectable {
			for _, variant := range operation.Variants {
				projection.Variants = append(projection.Variants, runtimeCandidateOperationVariantProjection{
					ID: variant.ID, Discriminators: append([]runtimecontract.RuntimeOperationParameterPredicate(nil), variant.Discriminators...),
					RequiredParameters: append([]string(nil), variant.RequiredParameters...), RequiredNonEmpty: append([]string(nil), variant.RequiredNonEmpty...),
					ForbiddenParameters: append([]string(nil), variant.ForbiddenParameters...), CandidateConstraints: append([]runtimecontract.RuntimeOperationCandidateConstraint(nil), variant.CandidateConstraints...), EffectScope: variant.EffectScope, IdempotencyClass: variant.IdempotencyClass,
				})
			}
		}
		ret = append(ret, projection)
	}
	return ret
}

func runtimeCandidateOperationConstraints(variants []runtimecontract.RuntimeOperationVariantContract) []runtimecontract.RuntimeOperationCandidateConstraint {
	ret := make([]runtimecontract.RuntimeOperationCandidateConstraint, 0)
	for _, variant := range variants {
		ret = append(ret, variant.CandidateConstraints...)
	}
	return runtimecontract.NormalizeRuntimeOperationCandidateConstraints(ret)
}

func runtimeCandidateWireParameterNamesForKind(wire runtimecontract.RuntimeCandidateWireContract, kind string) []string {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	values := make([]string, 0)
	for _, rule := range wire.OperationRules {
		if rule.Kind == kind {
			values = append(values, rule.ProviderParameterNames...)
		}
	}
	return runtimeSortedUniqueStrings(values)
}

func runtimeCandidateWireOneOfParameterNamesForKind(wire runtimecontract.RuntimeCandidateWireContract, kind string) []string {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	values := make([]string, 0)
	for _, rule := range wire.OperationRules {
		if rule.Kind == kind {
			values = append(values, rule.RequireOneOfProviderParameters...)
		}
	}
	return runtimeSortedUniqueStrings(values)
}

func runtimeCandidateWireAuxiliaryRuleForKind(wire runtimecontract.RuntimeCandidateWireContract, kind string) (runtimecontract.RuntimeCandidateWireOperationRule, bool) {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, rule := range wire.OperationRules {
		if rule.Kind == kind && !rule.RequiredSlot() {
			return rule.Normalized(), true
		}
	}
	return runtimecontract.RuntimeCandidateWireOperationRule{}, false
}

func runtimeCandidateManifestHasVariant(values []runtimecontract.RuntimeOperationVariantContract, variantID string) bool {
	for _, variant := range values {
		if variant.ID == strings.TrimSpace(variantID) {
			return true
		}
	}
	return false
}

func runtimeCandidateWireFieldsForKind(wire runtimecontract.RuntimeCandidateWireContract, kind string) ([]string, []string) {
	allowed := make([]string, 0)
	var required []string
	matched := 0
	for _, rule := range wire.OperationRules {
		if rule.Kind != runtimecontract.CanonicalRuntimeBlockKind(kind) {
			continue
		}
		matched++
		allowed = append(allowed, rule.ProviderFields...)
		if required == nil {
			required = append([]string(nil), rule.RequiredProviderFields...)
			continue
		}
		filtered := required[:0]
		for _, field := range required {
			if runtimeStringIn(rule.RequiredProviderFields, field) {
				filtered = append(filtered, field)
			}
		}
		required = filtered
	}
	if matched == 0 {
		return nil, nil
	}
	return runtimeSortedUniqueStrings(allowed), runtimeSortedUniqueStrings(required)
}

func runtimeCandidateWireKindSelectsBinding(wire runtimecontract.RuntimeCandidateWireContract, kind, requiredOperationRef string) bool {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	requiredOperationRef = strings.TrimSpace(requiredOperationRef)
	for _, rule := range wire.OperationRules {
		if rule.Kind == kind && rule.RequiredOperationRef == requiredOperationRef && runtimeStringIn(rule.ProviderFields, "bindingRefs") {
			return true
		}
	}
	return false
}

func runtimeValidationRecipeIDs(recipes []runtimecontract.RuntimeValidationRecipe) []string {
	recipes = runtimecontract.NormalizeRuntimeValidationRecipes(recipes)
	ret := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		ret = append(ret, recipe.ID)
	}
	return ret
}

// runtimeCandidateStructuredOutputInstruction renders the request-specific
// instruction only from the sealed manifest. It deliberately does not inspect
// operation descriptors, candidate context, rejected shapes or error strings:
// those inputs have already been normalized into the manifest or belong to
// C4 retry context. Keeping this projection adjacent to the provider renderer
// makes the LLM-facing wording a consumer rather than a second contract owner.
func runtimeManifestHasAllowedBindingRefs(manifest runtimecontract.RuntimeCandidateContractManifest) bool {
	for _, operation := range manifest.Operations {
		if len(runtimeCandidateBindingAuthorizationRefs(operation.BindingAuthorizations)) != 0 {
			return true
		}
	}
	return false
}

func runtimeCandidateStructuredOutputInstruction(manifest runtimecontract.RuntimeCandidateContractManifest, wire runtimecontract.RuntimeCandidateWireContract) string {
	manifest = manifest.Normalized()
	wire = wire.Normalized()
	lines := []string{
		"Sealed candidate manifest hash: " + manifest.Hash + ".",
		"Sealed candidate wire contract hash: " + wire.Hash + ".",
		"Return a candidate for convergence state " + string(manifest.State) + " with required top-level fields: " + runtimeCandidateContractFieldList(manifest.RequiredTopLevelFields) + ".",
		runtimecontract.RuntimeConvergenceEffectInstruction(manifest.EffectPolicy.State),
		"The only allowed top-level fields are: " + runtimeCandidateContractFieldList(manifest.AllowedTopLevelFields) + ".",
	}
	if len(manifest.ForbiddenTopLevelFields) > 0 {
		lines = append(lines, "Forbidden top-level fields: "+runtimeCandidateContractFieldList(manifest.ForbiddenTopLevelFields)+".")
	}
	if manifest.MaxOperationCount > 0 {
		lines = append(lines, fmt.Sprintf("Emit at most %d operations in this complete candidate document.", manifest.MaxOperationCount))
	}
	if manifest.OperationSetPolicy.MaxOperationsPerOwnerTarget > 0 {
		lines = append(lines, fmt.Sprintf("Emit at most %d operation for the same ownerModuleId and targetRef. Consolidate multiple edits to one owner target into one atomic operation; only distinct slots in requiredOperationSequence may overlap.", manifest.OperationSetPolicy.MaxOperationsPerOwnerTarget))
	}
	if runtimeManifestHasAllowedBindingRefs(manifest) {
		lines = append(lines, "Emit bindingRefs only when the matching wire rule lists it as a provider field. Runtime compiles a singleton sealed binding locally; echoing that fixed binding is forbidden. When bindingRefs remains selectable, choose exactly one listed value. Never emit coverageWorkItemIds.")
	}
	if len(manifest.RequiredOperationSequence) > 0 {
		refs := make([]string, 0, len(manifest.RequiredOperationSequence))
		for _, requirement := range manifest.RequiredOperationSequence {
			value := requirement.RequirementID + "=" + requirement.Kind + "/" + requirement.VariantID
			if requirement.TargetRef != "" {
				value += "@" + requirement.TargetRef
			}
			if len(requirement.DependsOnRequirementIDs) != 0 {
				value += "[requires=" + strings.Join(requirement.DependsOnRequirementIDs, "+") + "]"
			}
			refs = append(refs, value)
		}
		lines = append(lines, "Required operation sequence references: "+strings.Join(refs, ", ")+". Select each slot only by requiredOperationRef; kind, singleton target/binding and variant discriminators listed as locally derived must be omitted.")
	}
	for _, rule := range wire.OperationRules {
		rule = rule.Normalized()
		operation, found := runtimeCandidateOperationManifestForKind(manifest.Operations, rule.Kind)
		if !found {
			continue
		}
		line := "Wire rule"
		if rule.RequiredSlot() {
			line += " requiredOperationRef=" + rule.RequiredOperationRef + " compiles kind=" + rule.Kind + " variant=" + rule.VariantID + "."
		} else {
			line += " auxiliary kind=" + rule.Kind + "."
			if rule.VariantID != "" {
				line += " Runtime compiles the sole sealed variant=" + rule.VariantID + "."
			}
		}
		line += " Emit only fields in this order: " + runtimeCandidateContractFieldList(rule.ProviderFields) + ". Required provider fields: " + runtimeCandidateContractFieldList(rule.RequiredProviderFields) + "."
		derived := runtimeCandidateWireRuleProjections(runtimecontract.RuntimeCandidateWireContract{OperationRules: []runtimecontract.RuntimeCandidateWireOperationRule{rule}})
		if len(derived) == 1 && len(derived[0].LocallyDerivedFields) != 0 {
			line += " Locally derived and forbidden in provider output: " + runtimeCandidateContractFieldList(derived[0].LocallyDerivedFields) + "."
		}
		if len(rule.ProviderParameterNames) != 0 {
			parameters := make([]string, 0, len(rule.ProviderParameterNames))
			for _, name := range rule.ProviderParameterNames {
				parameter, found := runtimeCandidateOperationParameterForName(operation.Parameters, name)
				if !found {
					continue
				}
				parameters = append(parameters, parameter.Name+":"+string(parameter.Type))
			}
			line += " Provider parameters: " + runtimeCandidateContractFieldList(parameters) + "."
		}
		if len(rule.RequiredProviderParameterNames) != 0 {
			line += " Required provider parameters: " + runtimeCandidateContractFieldList(rule.RequiredProviderParameterNames) + "."
		}
		if len(rule.RequiredNonEmptyParameterNames) != 0 {
			line += " These provider parameters must be non-empty: " + runtimeCandidateContractFieldList(rule.RequiredNonEmptyParameterNames) + "."
		}
		if len(rule.RequireOneOfProviderParameters) != 0 {
			line += " Require one non-empty provider parameter from: " + runtimeCandidateContractFieldList(rule.RequireOneOfProviderParameters) + "."
		}
		if runtimeStringIn(rule.ProviderFields, "bindingRefs") {
			if runtimeStringIn(rule.RequiredProviderFields, "bindingRefs") {
				line += " bindingRefs is required and requires exactly one value from: " + runtimeCandidateContractFieldList(rule.AllowedBindingRefs) + "."
			} else {
				line += " Optional bindingRefs must select exactly one value from: " + runtimeCandidateContractFieldList(rule.AllowedBindingRefs) + "."
			}
		}
		if runtimeStringIn(rule.ProviderFields, "targetRefs") {
			line += " targetRefs cardinality=" + string(operation.TargetCardinality) + " and must satisfy TargetPolicy=" + string(operation.Target.Policy) + "."
			if runtimeStringIn(rule.RequiredProviderFields, "targetRefs") {
				line += " targetRefs is required."
			}
			if allowed := operation.Target.AllowedTargetRefs(); len(allowed) != 0 {
				line += " Allowed targets: " + runtimeCandidateContractFieldList(allowed) + "."
			}
		}
		if len(operation.ForbiddenParameters) > 0 {
			line += " Forbidden parameters: " + runtimeCandidateContractFieldList(operation.ForbiddenParameters) + "."
		}
		if ids := runtimeValidationRecipeIDs(operation.ValidationRecipes); len(ids) > 0 {
			line += " recipeId must select exactly one sealed validation recipe: " + runtimeCandidateContractFieldList(ids) + ". Do not include command, args, timeoutMillis, or a test-name filter."
		}
		if operation.AllowsDynamicParameters {
			line += " Dynamic parameters are allowed only by this operation contract."
		}
		lines = append(lines, line)
	}
	if len(wire.OperationRules) != 0 {
		lines = append(lines, "Within every operation object, emit requiredOperationRef first when present, then kind/bindingRefs/targetRefs only when listed, and parameters last. Do not close the operation or root before parameters are complete. Operation id and dependsOn are derived by Runtime; never emit them.")
	}
	return strings.Join(lines, "\n")
}

func runtimeCandidateBindingAuthorizationRefs(values []runtimecontract.RuntimeOperationBindingAuthorization) []string {
	seen := make(map[string]struct{})
	ret := make([]string, 0)
	for _, value := range values {
		for _, ref := range value.Normalized().AllowedBindingRefs {
			if _, duplicate := seen[ref]; duplicate {
				continue
			}
			seen[ref] = struct{}{}
			ret = append(ret, ref)
		}
	}
	sort.Strings(ret)
	return ret
}

func runtimeSortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func runtimeCandidateContractFieldList(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func runtimeCandidateRetryForManifest(attempt int, observation *runtimecontract.RuntimeCandidateCorrectionObservation, manifest runtimecontract.RuntimeCandidateContractManifest, correction *llmmodels.RuntimeCandidateCorrectionInput) *runtimeCandidateRetryProjection {
	if attempt != 1 || observation == nil || correction == nil {
		return nil
	}
	if observation.Validate() != nil || correction.Validate(manifest.Hash) != nil {
		return nil
	}
	previous := observation.Clone()
	input := correction.Normalized()
	// Operation identities are derived after provider decoding and are not part
	// of the provider wire. The operation-set witness uses stable kind/index
	// coordinates, so its C4 projection must not expose the local diagnostic ID.
	if input.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOperationSetCardinality && previous.Diagnostic != nil {
		previous.Diagnostic.OperationID = ""
	}
	instruction := "Retry the exact same sealed authority contract. Return only the legal top-level object for the requested action."
	manifest = manifest.Normalized()
	if manifest.Hash != "" {
		instruction += " Re-read outputContract.structuredOutputInstruction and schema in the C3 current runtime authority for sealed manifest hash " + manifest.Hash + "; they are the only source of allowed fields, kinds, variants, parameters, targets, coverage and dependencies."
	}
	if input.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding {
		instruction += " The previous candidate encoded manifest-declared logical parameters as sibling keys inside another parameters[] entry. Re-emit a fresh complete document: every listed logical parameter must be its own parameters[] item containing exactly name plus the listed typed wireValueField. Keep the container parameter as its own item; do not place logical parameter names beside its value field. Re-generate values from C3 authority and do not copy this diagnostic as output data."
	} else if input.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField && input.Witness.AlternativeValue == runtimecontract.RuntimeCandidateCorrectionFieldPresent {
		instruction += " The previous document omitted, emptied, set null, or used the wrong JSON type for a required field. Rebuild the entire document and emit that field exactly once with the type and nested shape required by outputContract.schema; field presence without schema conformance is still invalid."
	} else if input.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint {
		instruction += " The capability owner rejected one candidate parameter against a constraint already sealed in outputContract.candidateConstraints. Rebuild that parameter from the sealed required shape and the current C3 owner observation; do not copy the rejected value, parser output, or diagnostic text."
	} else if input.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOperationSetCardinality {
		instruction += " The previous candidate contained multiple ordinary durable operations for the same capability owner and canonical target. Consolidate every required change for that exact target into one operation; do not use dependsOn, a new operation ID, or a cosmetic parameter change to preserve the duplicate effect."
	} else if previous.Class == runtimecontract.RuntimeCandidateFailureClassSchema || previous.Class == runtimecontract.RuntimeCandidateFailureClassContract {
		instruction += " The previous candidate shape was rejected. Remove every field or value not permitted by that sealed manifest instead of adding a wrapper, narrative, summary, explanation or convenience parameter."
	}
	if runtimeCandidateFailureIsOperationShapeReplay(previous.Code) {
		instruction += " The previous candidate replayed a rejected or no-progress operation shape. Inspect rejectedOperationShapes and noProgressOperationShapes in the C4 execution-tail object. Changing only an operation ID, narrative, or cosmetic parameter is not sufficient."
	}
	instruction += fmt.Sprintf(" Apply the exact correction alternative %q at %s (%s); an unrelated valid change does not satisfy this retry.", input.Witness.AlternativeValue, input.Witness.Path, input.Witness.Dimension)
	witness := input.Witness.Normalized()
	projection := runtimeCandidateCorrectionInputProjection{
		Objectives:   append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		ManifestHash: input.ManifestHash,
		Witness: runtimeCandidateCorrectionWitnessProjection{
			Dimension: witness.Dimension, OperationKind: witness.OperationKind, Path: witness.Path,
			RejectedValue: witness.RejectedValue, AlternativeValue: witness.AlternativeValue,
			RejectedShapeHash: witness.RejectedShapeHash, AlternativeShapeHash: witness.AlternativeShapeHash,
			ConstraintRef:           witness.ConstraintRef,
			ParameterEntryEncodings: runtimeCandidateParameterEntryEncodingProjections(witness.ParameterEntryEncodings),
			OperationSetViolation:   witness.OperationSetViolation.Clone(),
		},
	}
	return &runtimeCandidateRetryProjection{PreviousFailure: previous, CorrectionInput: projection, Instruction: instruction}
}

func runtimeCandidateParameterEntryEncodingProjections(values []runtimecontract.RuntimeCandidateParameterEntryEncodingViolation) []runtimeCandidateParameterEntryEncodingProjection {
	ret := make([]runtimeCandidateParameterEntryEncodingProjection, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		projection := runtimeCandidateParameterEntryEncodingProjection{
			OperationIndex: value.OperationIndex, ParameterIndex: value.ParameterIndex,
			OperationKind:          value.OperationKind,
			SourceEntryPath:        fmt.Sprintf("$.operations[%d].parameters[%d]", value.OperationIndex, value.ParameterIndex),
			ContainerParameterName: value.ContainerParameterName,
			Entries:                make([]runtimeCandidateParameterEncodingEntryProjection, 0, len(value.MisplacedParameters)),
		}
		for _, parameter := range value.MisplacedParameters {
			wireField, found := runtimecontract.RuntimeOperationParameterWireValueField(parameter.Type)
			if !found {
				continue
			}
			projection.Entries = append(projection.Entries, runtimeCandidateParameterEncodingEntryProjection{
				Name: parameter.Name, ContractType: parameter.Type, WireValueField: wireField,
			})
		}
		ret = append(ret, projection)
	}
	return ret
}

func runtimeCandidateFailureIsOperationShapeReplay(code string) bool {
	switch strings.TrimSpace(code) {
	case "candidate_replays_rejected_shape", "candidate_partially_replays_rejected_shape", "candidate_replays_no_progress_shape":
		return true
	default:
		return false
	}
}
