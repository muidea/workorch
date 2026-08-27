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
	Schema                      json.RawMessage                               `json:"schema"`
	StructuredOutputInstruction string                                        `json:"structuredOutputInstruction"`
	ForbiddenTopLevelFields     []string                                      `json:"forbiddenTopLevelFields"`
	OperationContracts          []runtimeCandidateOperationContractProjection `json:"operationContracts,omitempty"`
}

type runtimeCandidateEffectPolicyProjection struct {
	ExecutableOperationsAllowed   bool                                 `json:"executableOperationsAllowed"`
	AllowedOperationEffects       []runtimecontract.RuntimeEffectScope `json:"allowedOperationEffects,omitempty"`
	AllowedAcceptedReceiptEffects []runtimecontract.RuntimeEffectScope `json:"allowedAcceptedReceiptEffects,omitempty"`
}

type runtimeCandidateOperationContractProjection struct {
	Kind                        string                                            `json:"kind"`
	OwnerModuleID               string                                            `json:"ownerModuleId,omitempty"`
	AllowedOperationFields      []string                                          `json:"allowedOperationFields,omitempty"`
	AllowedVariantIDs           []string                                          `json:"allowedVariantIds,omitempty"`
	Parameters                  []runtimeCandidateOperationParameterProjection    `json:"parameters"`
	RequireOneOfParameters      []string                                          `json:"requireOneOfParameters,omitempty"`
	TargetCardinality           runtimecontract.RuntimeOperationTargetCardinality `json:"targetCardinality,omitempty"`
	RequireTargetRefs           bool                                              `json:"requireTargetRefs,omitempty"`
	ForbiddenParameters         []string                                          `json:"forbiddenParameters,omitempty"`
	Variants                    []runtimeCandidateOperationVariantProjection      `json:"variants,omitempty"`
	Rules                       []string                                          `json:"rules,omitempty"`
	AllowsDynamicParameters     bool                                              `json:"allowsDynamicParameters,omitempty"`
	DynamicParameterPrefixes    []string                                          `json:"dynamicParameterPrefixes,omitempty"`
	RequiredOperationReferences []string                                          `json:"requiredOperationReferences,omitempty"`
	BindingAuthorizations       []runtimeCandidateBindingAuthorizationProjection  `json:"bindingAuthorizations,omitempty"`
	AcceptedEffectScopes        []runtimecontract.RuntimeEffectScope              `json:"acceptedEffectScopes,omitempty"`
	RequiredEffectScopes        []runtimecontract.RuntimeEffectScope              `json:"requiredEffectScopes,omitempty"`
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
	ID                  string                                               `json:"id"`
	Discriminators      []runtimecontract.RuntimeOperationParameterPredicate `json:"discriminators"`
	RequiredParameters  []string                                             `json:"requiredParameters,omitempty"`
	RequiredNonEmpty    []string                                             `json:"requiredNonEmpty,omitempty"`
	ForbiddenParameters []string                                             `json:"forbiddenParameters,omitempty"`
	EffectScope         runtimecontract.RuntimeEffectScope                   `json:"effectScope"`
	IdempotencyClass    runtimecontract.RuntimeOperationIdempotencyClass     `json:"idempotencyClass"`
}

type runtimeCandidateRetryProjection struct {
	PreviousFailure runtimeCandidateCorrectionFailureProjection `json:"previousFailure"`
	CorrectionInput runtimeCandidateCorrectionInputProjection   `json:"correctionInput"`
	Instruction     string                                      `json:"instruction"`
}

type runtimeCandidateCorrectionInputProjection struct {
	Objectives   []runtimecontract.RuntimeCandidateCorrectionObjective `json:"objectives"`
	ManifestHash string                                                `json:"manifestHash"`
	Witness      runtimeCandidateCorrectionWitnessProjection           `json:"witness"`
}

type runtimeCandidateCorrectionWitnessProjection struct {
	Dimension            runtimecontract.RuntimeCandidateCorrectionWitnessDimension `json:"dimension"`
	OperationKind        string                                                     `json:"operationKind,omitempty"`
	Path                 string                                                     `json:"path"`
	RejectedValue        string                                                     `json:"rejectedValue,omitempty"`
	AlternativeValue     string                                                     `json:"alternativeValue"`
	RejectedShapeHash    string                                                     `json:"rejectedShapeHash,omitempty"`
	AlternativeShapeHash string                                                     `json:"alternativeShapeHash,omitempty"`
}

// runtimeCandidateCorrectionFailureProjection is the only representation of
// an earlier rejection allowed into C4. It intentionally has no Reason field:
// raw provider/parser/owner text remains in persisted audit, never in the next
// model prompt.
type runtimeCandidateCorrectionFailureProjection struct {
	Class      runtimecontract.RuntimeCandidateFailureClass       `json:"class"`
	Code       string                                             `json:"code,omitempty"`
	Signature  string                                             `json:"signature,omitempty"`
	TargetRefs []string                                           `json:"targetRefs,omitempty"`
	Diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic `json:"diagnostic,omitempty"`
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
		"The current runtime authority's context.outputContract is the sole structured-output contract. Its manifestHash, schema, structuredOutputInstruction, and operationContracts define the complete allowed fields, kinds, variants, parameters, targets, coverage and dependencies for this request.",
		"Emit only schema fields and manifest-authorized operations. Do not infer or add unknown fields, unrequested operations, prose, preface, suffix, or markdown fences. Do not repeat the Goal, DoD, manifest, authority, or input context.",
		"context.requiredState is the sole canonical decision authority. Do not invent or modify convergence state, focus, authority, transaction, or output mode.",
		"When context.repairDirective is present, obey its sealed state, effect, evidence and operation constraints. The exact fields and values remain in context.outputContract and current runtime authority.",
		"context.missingEvidenceKinds is completion-audit information only and must never override context.requiredState or context.repairDirective.",
		"When context.rejectedOperationShapes is present, every listed operation hash and exact structural operation is forbidden until later accepted evidence supersedes it. Choose an admissible kind and change kind, targets, or execution-semantic parameters.",
	}, "\n")
}

func runtimeCandidateOutputContractForManifest(manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateOutputContractProjection {
	manifest = manifest.Normalized()
	return runtimeCandidateOutputContractProjection{
		State: manifest.State,
		EffectPolicy: runtimeCandidateEffectPolicyProjection{
			ExecutableOperationsAllowed:   manifest.EffectPolicy.ExecutableOperationsAllowed,
			AllowedOperationEffects:       append([]runtimecontract.RuntimeEffectScope(nil), manifest.EffectPolicy.AllowedOperationEffects...),
			AllowedAcceptedReceiptEffects: append([]runtimecontract.RuntimeEffectScope(nil), manifest.EffectPolicy.AllowedAcceptedReceiptEffects...),
		},
		ManifestHash:                manifest.Hash,
		Schema:                      json.RawMessage(runtimeCandidateSchemaForManifest(manifest)),
		StructuredOutputInstruction: runtimeCandidateStructuredOutputInstruction(manifest),
		ForbiddenTopLevelFields:     append([]string(nil), manifest.ForbiddenTopLevelFields...),
		OperationContracts:          runtimeCandidateOperationContractsForManifest(manifest.Operations),
	}
}

func runtimeCandidateOperationContractsForManifest(values []runtimecontract.RuntimeCandidateOperationManifest) []runtimeCandidateOperationContractProjection {
	ret := make([]runtimeCandidateOperationContractProjection, 0, len(values))
	for _, operation := range values {
		operation = operation.Normalized()
		target := operation.Target.Normalized()
		projection := runtimeCandidateOperationContractProjection{
			Kind:                        operation.Kind,
			OwnerModuleID:               operation.OwnerModuleID,
			AllowedOperationFields:      append([]string(nil), operation.AllowedOperationFields...),
			AllowedVariantIDs:           append([]string(nil), operation.AllowedVariantIDs...),
			RequireOneOfParameters:      append([]string(nil), operation.RequireOneOfParameters...),
			TargetCardinality:           operation.TargetCardinality,
			RequireTargetRefs:           operation.RequireTargetRefs,
			ForbiddenParameters:         append([]string(nil), operation.ForbiddenParameters...),
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
		for _, authorization := range operation.BindingAuthorizations {
			projection.BindingAuthorizations = append(projection.BindingAuthorizations, runtimeCandidateBindingAuthorizationProjection{
				RequiredOperationRef: authorization.RequiredOperationRef,
				VariantID:            authorization.VariantID,
				AllowedBindingRefs:   append([]string(nil), authorization.AllowedBindingRefs...),
			})
		}
		for _, parameter := range operation.Parameters {
			projection.Parameters = append(projection.Parameters, runtimeCandidateOperationParameterProjection{
				Name: parameter.Name, Type: string(parameter.Type), Required: parameter.Required, Description: parameter.Description,
			})
		}
		for _, variant := range operation.Variants {
			projection.Variants = append(projection.Variants, runtimeCandidateOperationVariantProjection{
				ID: variant.ID, Discriminators: append([]runtimecontract.RuntimeOperationParameterPredicate(nil), variant.Discriminators...),
				RequiredParameters: append([]string(nil), variant.RequiredParameters...), RequiredNonEmpty: append([]string(nil), variant.RequiredNonEmpty...),
				ForbiddenParameters: append([]string(nil), variant.ForbiddenParameters...), EffectScope: variant.EffectScope, IdempotencyClass: variant.IdempotencyClass,
			})
		}
		ret = append(ret, projection)
	}
	return ret
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

func runtimeCandidateStructuredOutputInstruction(manifest runtimecontract.RuntimeCandidateContractManifest) string {
	manifest = manifest.Normalized()
	lines := []string{
		"Sealed candidate manifest hash: " + manifest.Hash + ".",
		"Return a candidate for convergence state " + string(manifest.State) + " with required top-level fields: " + runtimeCandidateContractFieldList(manifest.RequiredTopLevelFields) + ".",
		runtimecontract.RuntimeConvergenceEffectInstruction(manifest.EffectPolicy.State),
		"The only allowed top-level fields are: " + runtimeCandidateContractFieldList(manifest.AllowedTopLevelFields) + ".",
	}
	if len(manifest.ForbiddenTopLevelFields) > 0 {
		lines = append(lines, "Forbidden top-level fields: "+runtimeCandidateContractFieldList(manifest.ForbiddenTopLevelFields)+".")
	}
	if runtimeManifestHasAllowedBindingRefs(manifest) {
		lines = append(lines, "Every operation kind with a sealed binding authorization must select exactly one bindingRefs value from that authorization; never emit coverageWorkItemIds. Operation kinds without binding authorization are non-crediting and must omit bindingRefs.")
	}
	if len(manifest.RequiredOperationSequence) > 0 {
		refs := make([]string, 0, len(manifest.RequiredOperationSequence))
		for _, requirement := range manifest.RequiredOperationSequence {
			value := requirement.RequirementID + "=" + requirement.Kind + "/" + requirement.VariantID
			if requirement.TargetRef != "" {
				value += "@" + requirement.TargetRef
			}
			refs = append(refs, value)
		}
		lines = append(lines, "Required operation sequence references: "+strings.Join(refs, ", ")+".")
	}
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		line := "Operation kind " + operation.Kind + " allows fields: " + runtimeCandidateContractFieldList(operation.AllowedOperationFields) + "."
		if len(operation.AllowedVariantIDs) > 0 {
			line += " Allowed variants: " + runtimeCandidateContractFieldList(operation.AllowedVariantIDs) + "."
		}
		if len(operation.Parameters) > 0 {
			parameters := make([]string, 0, len(operation.Parameters))
			for _, parameter := range operation.Parameters {
				name := parameter.Name + ":" + string(parameter.Type)
				if parameter.Required {
					name += "(required)"
				}
				parameters = append(parameters, name)
			}
			line += " Parameters: " + strings.Join(parameters, ", ") + "."
		}
		if len(operation.RequireOneOfParameters) > 0 {
			line += " Require one non-empty parameter from: " + runtimeCandidateContractFieldList(operation.RequireOneOfParameters) + "."
		}
		if operation.TargetCardinality != "" {
			line += " targetRefs cardinality=" + string(operation.TargetCardinality) + "."
		}
		if operation.RequireTargetRefs {
			line += " targetRefs is required and must name the concrete target changed by this operation."
		}
		target := operation.Target.Normalized()
		if target.Policy != "" {
			line += " TargetPolicy=" + string(target.Policy) + "."
			if allowed := target.AllowedTargetRefs(); len(allowed) > 0 {
				line += " Allowed targets: " + runtimeCandidateContractFieldList(allowed) + "."
			}
			if len(target.ProspectiveTargetRefs) != 0 {
				line += " Prospective targets: " + runtimeCandidateContractFieldList(target.ProspectiveTargetRefs) + "."
			}
			if len(target.OwnerSelectorRefs) > 0 {
				line += " Owner selectors: " + runtimeCandidateContractFieldList(target.OwnerSelectorRefs) + "."
			}
		}
		if len(operation.RequiredOperationReferences) > 0 {
			line += " requiredOperationRef must be one of: " + runtimeCandidateContractFieldList(operation.RequiredOperationReferences) + "."
		}
		for _, authorization := range operation.BindingAuthorizations {
			authorization = authorization.Normalized()
			identity := authorization.VariantID
			if authorization.RequiredOperationRef != "" {
				identity += "/" + authorization.RequiredOperationRef
			}
			if len(authorization.AllowedBindingRefs) > 0 {
				line += " Variant/ref " + identity + " requires exactly one bindingRefs value from: " + runtimeCandidateContractFieldList(authorization.AllowedBindingRefs) + "."
			} else {
				line += " Variant/ref " + identity + " is auxiliary and must omit bindingRefs."
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
	if !manifest.DependenciesAllowed {
		lines = append(lines, "dependsOn is not an allowed operation field for this sealed contract.")
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

func runtimeCandidateManifestVariantForID(values []runtimecontract.RuntimeOperationVariantContract, id string) (runtimecontract.RuntimeOperationVariantContract, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationVariantContract{}, false
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

func runtimeCandidateRetryForManifest(attempt int, failure *runtimecontract.RuntimeCandidateFailure, manifest runtimecontract.RuntimeCandidateContractManifest, correction *llmmodels.RuntimeCandidateCorrectionInput) *runtimeCandidateRetryProjection {
	if attempt != 1 || failure == nil || correction == nil {
		return nil
	}
	if correction.Validate(manifest.Hash) != nil {
		return nil
	}
	previous := runtimeCandidateCorrectionFailureProjection{
		Class:      failure.Class,
		Code:       strings.TrimSpace(failure.Code),
		Signature:  strings.TrimSpace(failure.Signature),
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if diagnostic := failure.Diagnostic.Clone(); diagnostic != nil {
		// P3-B2a has already sealed the complete diagnostic. C4 only projects
		// that immutable, manifest-bound delta; it must not infer a missing hash,
		// rejected shape, or correction objective from the failure text/signature.
		// Attempt-1 eligibility belongs to SM-3 and is fenced before this LLM
		// renderer is called.
		previous.Diagnostic = diagnostic.Clone()
	}
	instruction := "Retry the exact same sealed authority contract. Return only the legal top-level object for the requested action."
	manifest = manifest.Normalized()
	if manifest.Hash != "" {
		instruction += " Re-read context.outputContract.structuredOutputInstruction and schema for sealed manifest hash " + manifest.Hash + "; they are the only source of allowed fields, kinds, variants, parameters, targets, coverage and dependencies."
	}
	if previous.Class == runtimecontract.RuntimeCandidateFailureClassSchema || previous.Class == runtimecontract.RuntimeCandidateFailureClassContract {
		instruction += " The previous candidate shape was rejected. Remove every field or value not permitted by that sealed manifest instead of adding a wrapper, narrative, summary, explanation or convenience parameter."
	}
	if runtimeCandidateFailureIsOperationShapeReplay(previous.Code) {
		instruction += " The previous candidate replayed a rejected or no-progress operation shape. Inspect context.rejectedOperationShapes and context.noProgressOperationShapes. Changing only an operation ID, narrative, or cosmetic parameter is not sufficient."
	}
	input := correction.Normalized()
	instruction += fmt.Sprintf(" Apply the exact correction alternative %q at %s (%s); an unrelated valid change does not satisfy this retry.", input.Witness.AlternativeValue, input.Witness.Path, input.Witness.Dimension)
	witness := input.Witness.Normalized()
	projection := runtimeCandidateCorrectionInputProjection{
		Objectives:   append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		ManifestHash: input.ManifestHash,
		Witness: runtimeCandidateCorrectionWitnessProjection{
			Dimension: witness.Dimension, OperationKind: witness.OperationKind, Path: witness.Path,
			RejectedValue: witness.RejectedValue, AlternativeValue: witness.AlternativeValue,
			RejectedShapeHash: witness.RejectedShapeHash, AlternativeShapeHash: witness.AlternativeShapeHash,
		},
	}
	return &runtimeCandidateRetryProjection{PreviousFailure: previous, CorrectionInput: projection, Instruction: instruction}
}

func runtimeCandidateFailureIsOperationShapeReplay(code string) bool {
	switch strings.TrimSpace(code) {
	case "candidate_replays_rejected_shape", "candidate_partially_replays_rejected_shape", "candidate_replays_no_progress_shape":
		return true
	default:
		return false
	}
}
