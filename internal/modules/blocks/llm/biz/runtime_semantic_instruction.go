package llm

import (
	"encoding/json"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// compactSemanticPlanPromptSchema removes prose-only JSON Schema metadata from
// the provider prompt. The full schema remains sealed in ProviderRequest.Schema
// and is still used by the local decoder/evaluator; only the redundant prompt
// projection is compacted.
func compactSemanticPlanPromptSchema(raw []byte) []byte {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	var compact func(any) any
	compact = func(item any) any {
		switch typed := item.(type) {
		case map[string]any:
			ret := make(map[string]any, len(typed))
			for key, child := range typed {
				switch key {
				case "description", "title", "$comment", "examples":
					continue
				default:
					ret[key] = compact(child)
				}
			}
			return ret
		case []any:
			ret := make([]any, len(typed))
			for idx, child := range typed {
				ret[idx] = compact(child)
			}
			return ret
		default:
			return item
		}
	}
	value = compact(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}

func runtimeSemanticPlanContractCatalog() string {
	return "Return exactly one complete compact JSON object and finish it in this response. Emit only schema fields; never add prose, a preface or suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. You propose a bounded semantic proof plan for a sealed WorkOrch definition. The proposal is not authority and has no side effects. Use only the owner modules, operation kinds, variants, target policies, evidence kinds, declarations, and selectable identities supplied in C3. Echo each proofObligation.declarationRef exactly; it is sealed per CWI and is never your choice. A validationProofRequirement fixes its requiredCheckRefs; when its validationRecipeId is non-empty that recipe is also fixed, otherwise select only a recipe that the same ownerCatalog publishes as ready/direct for the exact requirement. RequiredCheckRefs are owner-verified receipt requirements and must never be emitted or replaced as proposal fields. A RequiredOwnerFacts declaration in C2 is a postcondition, not a current owner fact: only a matching selectable identity in the same owner's C3 catalog establishes current availability, and you must not invent or claim that fact. Operation kind, variant and target policy must belong to the same ownerCatalog fact; prospective/create-only authority cannot be used for an existing owner target. Each proposal is one independently feasible proof and has exactly one evidence kind. Group proofs by coverageWorkItemId: return one proof for every required evidence kind of each coverage item, with no extra CWI or duplicate evidence kind. Validation evidence is receipt-backed and may use only mcp/tool_call or validation/managed_session with a validationRecipeId published by that same ownerCatalog; each published recipe is already owner-attested, input-bound and non-durable. File/read, file/write, shell/command, git/write, and patch/apply are never validation proof operations. If the sealed Definition cannot be represented as a complete all-of proof plan because it implies alternatives, omits a prerequisite, or remains semantically ambiguous, return only the schema's definition_clarification_required factual document with the exact current planStep, an allowed gapKind, manifest-authorized offendingItemRefs, and a short safe diagnosticMessage. For gapKind=undeclared_prerequisite, also return one or more prerequisiteClaims with exact coverageWorkItemId, ownerModuleId, factKind and operationKind values covering every offending CWI; never call a requirement undeclared when the same exact RequiredOwnerFacts entry already exists in C2, including when C3 shows it currently selectable or carries ownerFactAcquisition. That factual document must not propose a relation, action, ticket, target state, or replacement. Never emit operation execution, runtime action, coverage state or credit, terminal/retry/governance decisions, receipts, or owner observations. Implementation-work rule: C3 implementationPurpose with purposeChoiceOnly=true asks only for a semantic purpose proposal in the existing coverage_order document. Emit the single implementationPurpose schema field as implementation_effect only when effectAvailable=true, or terminal_proof; when inputRevision is present only implementation_effect is allowed, using its complete original input and conflict findings to propose a criterion-preserving correction; neither value grants effects, terminal credit, or a Runtime action. The exact policy, scope, exclusions and remaining ordinal are volatile C3 constraints, never inferred from file names or terminal recipe availability. For evidencePurpose=implementation_work in a proof batch, implementationWork replaces only the current proof proposal vocabulary: use its immutable policy, exact owner scope and excludedTargetRefs, propose bounded unique targets and state_change/workspace_durable proofs, and never also propose terminal validation evidence or a validationRecipeId/input review for that work. Its terminalRequiredEvidenceKinds and original Goal/DoD remain unchanged obligations for later independently reviewed terminal proof. Multiple distinct work targets may contribute to the same original CWI; do not split that CWI, claim work completion, edit protected validation inputs outside an explicitly supplied inputRevision, bypass exact validation repair, or select targets outside the complete policy/scope intersection."
}

func runtimeSemanticBaseOutputInstructionWithoutWorkContent(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest) string {
	for _, obligation := range manifest.ProofObligations {
		work := obligation.ImplementationWork
		if work == nil {
			continue
		}
		if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder && work.PurposeChoiceOnly {
			if work.Scope.Request.InputRevision != nil {
				return "Return one complete semantic_plan JSON object with planStep=coverage_order, the complete coverageOrder, proposals=[], and implementationPurpose=implementation_effect. The exact C3 inputRevision retains the original input, immutable requirement and conflict findings. A real bounded criterion-preserving input modification is required before fresh independent review; unchanged re-review is not an allowed purpose. This proposal grants no effect or validation credit. Use only the sealed Profile and exact owner scope. Emit no operation, Runtime decision, new criterion, receipt or authority."
			}
			return "Return exactly one complete compact semantic_plan JSON object with planStep=coverage_order, the complete coverageOrder, proposals=[], and the single schema-required implementationPurpose field. Select only a value allowed by this exact schema: implementation_effect requires C3 implementationPurpose.effectAvailable=true; terminal_proof only requests the original terminal-proof transaction and does not claim completion. Use the complete C3 implementationPurpose policy and scope to judge a bounded implementation proposal, without guessing targets or validation availability. Emit no operation, target proposal, recipe, feasibility, runtime action, credit, terminal decision, retry, governance, ticket, or receipt. A genuine Definition gap uses only the existing definition_clarification_required schema branch and never borrows implementation purpose authority."
		}
		if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && !work.PurposeChoiceOnly {
			return "Return exactly one complete compact semantic_plan JSON object with planStep=proof_batch, exact batch identity, and proposals only for the original supplied CWI. Its evidencePurpose=implementation_work and implementationWork define the sole current vocabulary: return one or more independently feasible, bounded unique-target proposals within policy.allowedOperations, policy.allowedTargetPolicies and the complete exact owner scope. Profile targetSelectorRef identifies the observed work scope only; target.ownerSelectorRefs uses only the same operation/variant ownerCatalog selector vocabulary. Existing targets must be explicit ownerScopeTargetRefs or exactTargetRefs, and prospective targets require the native selector plus all sealed constraints; preserve the full mandatory prospective all-of constraints when applicable. Echo declarationRef exactly, require requiredEvidenceKinds=[\"state_change\"] ; effect scope is derived internally from owner facts and must not be emitted, and exclude every excludedTargetRefs entry. Do not return a validationRecipeId, validation input review, terminal proof, coverageOrder, new CWI, operation execution, credit, completion, Runtime action, ticket, retry or receipt. Original Goal/DoD and terminalRequiredEvidenceKinds remain unchanged for later independent terminal proof; working effects grant zero terminal evidence or coverage credit. A genuine Definition gap uses only the existing exact-batch clarification schema branch."
		}
	}
	repairInstruction := ""
	if runtimeSemanticRequestHasValidationFailureRepair(context, manifest) {
		repairInstruction = " Specialized validation-failure repair rule: when evidencePurpose=validation_failure_repair and validationFailureRepair is present, return exactly one proposal with ownerModuleId=repairOwnerModuleId, operationKind selected from repairOperationKinds, target.policy=repairTargetPolicy, target.ownerSelectorRefs=[repairOperationTargetSelectorRef], target.prospectiveConstraintRefs=repairConstraintRefs, declarationRef=convergenceId, and requiredEvidenceKinds=[\"state_change\"]. Select a non-empty bounded target.prospectiveTargetRefs set within repairDeclarationTargetSelectorCandidateRefs when that set is present. Treat triggerDiagnosticClass and failureSignature as the exact repair purpose: every proposed target must contribute to resolving that typed failure, and an unrelated artifact is invalid. The proposal is non-authoritative: the File owner must evaluate the complete all-of target set, and only an SM-2 private event may seal it before dispatch. requireFreshValidation=true means the repair grants no terminal validation evidence or coverage credit."
	}
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return "Return exactly one complete compact semantic_plan JSON object with planStep=coverage_order, a complete coverageOrder, and proposals=[], and finish it in this response. Emit every schema-required field and only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or repeated Goal, DoD, manifest, authority, or input context. If and only if a complete all-of order cannot be stated because the sealed Definition implies alternatives, truly omits a prerequisite, or is semantically ambiguous, return the definition_clarification_required schema branch instead. For an undeclared prerequisite, include exact typed prerequisiteClaims as required by the schema; an existing exact C2 owner-fact declaration is not undeclared. The proposals field must be present and empty; do not return any proposal item or any operation, target, recipe, feasibility, action, credit, terminal, retry, governance, ticket, or receipt field."
	}
	if context.BatchID != "" {
		return "Return exactly one complete compact semantic_plan JSON object with planStep=proof_batch and proposals only for the supplied batch coverage IDs, and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. Echo the sealed batch identity exactly. A proofObligation with evidencePurpose=definition_proof must have one proof for every terminalRequiredEvidenceKinds value, and each proposal has exactly one requiredEvidenceKinds value. When a proposal uses target.policy=prospective_owner_target and its proofObligation carries declarationProspectiveTargetAllOfConstraintRefs, target.prospectiveConstraintRefs must equal that complete mandatory set; never choose a subset or alternative. Specialized current-prerequisite rule: when evidencePurpose=owner_fact_acquisition and ownerFactAcquisition is present, currentRequiredEvidenceKinds and that object define the exact current acquisition proof for this generation and replace the terminal-evidence proposal rule for that CWI. Return exactly one proposal with ownerModuleId=acquisitionOwnerModuleId, operationKind=acquisitionOperationKind, operationVariantId=acquisitionOperationVariantId when present, target.policy=acquisitionTargetPolicy, target.ownerSelectorRefs=[acquisitionTargetSelectorRef], target.prospectiveConstraintRefs=acquisitionConstraintRefs when required, a non-empty bounded target.prospectiveTargetRefs set selected within the declaration selector constraints and declarationTargetSelectorCandidateRefs when that bounded set is present, and requiredEvidenceKinds=[\"state_change\"]. When prospectiveTargetConstraint is present, every allOf target clause must be covered by the proposed set; do not invent an alternative, weaken a predicate, or derive a target from an owner message. The proposed prospectiveTargetRefs are not Definition authority: the required fact owners must evaluate that exact proposal and only an SM-2 CoverageEvidenceBinding may seal it as Runtime authority before dispatch. The final validation surface and validationRecipeId are expected to be unavailable until that acquisition is executed and its required owner is reobserved; do not return gapKind=undeclared_prerequisite solely because either is not currently selectable. The acquisition proposal proves only the prerequisite state change: terminalEvidenceCredit=false, so it does not grant terminal validation evidence or coverage credit, and you must not also propose the terminal validation proof in this generation. Match every operation kind, variant and target policy to the referenced ownerCatalog fact; never apply a prospective/create-only operation fact to a target already listed by the owner catalog. For terminal validation evidence use only mcp/tool_call or validation/managed_session and select a validationRecipeId published by the same ownerCatalog; an unpublished prerequisite-required identity is never terminal validation proof. Never use file/read, file/write, shell/command, git/write, or patch/apply as terminal validation evidence. If and only if the supplied batch cannot form a complete all-of proof plan because the sealed Definition implies alternatives, truly has no declared prerequisite, or is semantically ambiguous, return the definition_clarification_required schema branch with the exact batch identity instead. An undeclared-prerequisite branch must include exact typed prerequisiteClaims; any matching C2 owner-fact declaration makes that claim invalid, including when C3 shows a selectable ownerCatalog fact or ownerFactAcquisition. Do not return coverageOrder, add or omit batch coverage, claim coverage outside the batch, or invent a ticket/action." + repairInstruction
	}
	return "Return exactly one complete compact semantic_plan JSON object and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, unrequested proposals, and do not repeat the Goal, DoD, manifest, authority, or input context. For every manifest coverage item with evidencePurpose=definition_proof, return one bounded proof declaration for each terminalRequiredEvidenceKinds value. Each proposal has exactly one requiredEvidenceKinds value. For an item with evidencePurpose=owner_fact_acquisition, return only its exact current prerequisite acquisition proposal using currentRequiredEvidenceKinds=[\"state_change\"]; when prospectiveTargetConstraint is present, every allOf target clause must be covered by target.prospectiveTargetRefs. The temporarily unavailable final validation surface is not by itself an undeclared prerequisite, and this acquisition grants no terminal validation evidence or coverage credit. Omit optional arrays and fields when empty. Match operationKind, operationVariantId and target.policy to one ownerCatalog operation fact; an exact or existing target must use a kind/variant whose allowedTargetPolicies includes that policy, while a prospective policy is only for absent owner-authorized targets. A terminal validation evidence proof must use only operationKind=mcp/tool_call or operationKind=validation/managed_session and a validationRecipeId published by the same ownerCatalog. File/read, file/write, shell/command, git/write, and patch/apply cannot claim terminal validation evidence. Do not return operations, runtime actions, coverage credit, terminal state, retry policy, governance tickets, receipts, or owner results." + repairInstruction
}

func runtimeSemanticRequestHasValidationFailureRepair(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest) bool {
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return false
	}
	visible := make(map[string]struct{}, len(context.BatchCoverageWorkItemIDs))
	for _, coverageID := range context.BatchCoverageWorkItemIDs {
		visible[string(coverageID)] = struct{}{}
	}
	for _, obligation := range manifest.Normalized().ProofObligations {
		if len(visible) != 0 {
			if _, found := visible[string(obligation.CoverageWorkItemID)]; !found {
				continue
			}
		}
		if obligation.ValidationFailureRepair != nil {
			return true
		}
	}
	return false
}

func runtimeSemanticBatchSequence(context runtimecontract.RuntimeSemanticPlanRequestContext) *int {
	if context.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || context.BatchID == "" {
		return nil
	}
	value := context.BatchSequence
	return &value
}

func runtimeSemanticBaseOutputInstruction(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest) string {
	base := runtimeSemanticBaseOutputInstructionWithoutWorkContent(context, manifest)
	if context.WorkContext == nil {
		return base
	}
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return base + " C4 workContext contains exact current resource bytes and complete trusted prior rounds for this CWI. Compare actual contents against the original C1 Goal/DoD and C3 declaration. Return the schema-required implementationIntent with exact declarationRef and contentHash, a minimal remainingWork statement and reason for this purpose. Different changes to the same target must address a distinct unmet original requirement; unchanged content is not implementation progress. Do not repeatedly polish a subset while ignoring missing original requirements or required new resources. Request terminal_proof when ready for independent checking; this never asserts completion or validation success."
	}
	return base + " Use C4 workContext exact current contents, trusted prior rounds and original purpose intent to propose only the minimal missing work within the original declaration. File hashes and earlier modifications alone never prove semantic completeness."
}
