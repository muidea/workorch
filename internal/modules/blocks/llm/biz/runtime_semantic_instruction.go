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
	return "Return exactly one complete compact JSON object and finish it in this response. Emit only schema fields; never add prose, a preface or suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. You propose a bounded semantic proof plan for a sealed WorkOrch definition. The proposal is not authority and has no side effects. Use only the owner modules, operation kinds, variants, target policies, evidence kinds, declarations, and selectable identities supplied in C3. A RequiredOwnerFacts declaration in C2 is a postcondition, not a current owner fact: only a matching selectable identity in the same owner's C3 catalog establishes current availability, and you must not invent or claim that fact. Operation kind, variant and target policy must belong to the same ownerCatalog fact; prospective/create-only authority cannot be used for an existing owner target. Each proposal is one independently feasible proof and has exactly one evidence kind. Group proofs by coverageWorkItemId: return one proof for every required evidence kind of each coverage item, with no extra CWI or duplicate evidence kind. Validation evidence is receipt-backed and may use only mcp/tool_call or validation/managed_session with a validationRecipeId published by that same ownerCatalog; each published recipe is already owner-attested, input-bound and non-durable. File/read, file/write, shell/command, git/write, and patch/apply are never validation proof operations. If the sealed Definition cannot be represented as a complete all-of proof plan because it implies alternatives, omits a prerequisite, or remains semantically ambiguous, return only the schema's definition_clarification_required factual document with the exact current planStep, an allowed gapKind, manifest-authorized offendingItemRefs, and a short safe diagnosticMessage. That factual document must not propose a relation, action, ticket, target state, or replacement. Never emit operation execution, runtime action, coverage state or credit, terminal/retry/governance decisions, receipts, or owner observations."
}

func runtimeSemanticOutputInstruction(context runtimecontract.RuntimeSemanticPlanRequestContext) string {
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return "Return exactly one complete compact semantic_plan JSON object with planStep=coverage_order, a complete coverageOrder, and proposals=[], and finish it in this response. Emit every schema-required field and only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or repeated Goal, DoD, manifest, authority, or input context. If and only if a complete all-of order cannot be stated because the sealed Definition implies alternatives, omits a prerequisite, or is semantically ambiguous, return the definition_clarification_required schema branch instead. The proposals field must be present and empty; do not return any proposal item or any operation, target, recipe, feasibility, action, credit, terminal, retry, governance, ticket, or receipt field."
	}
	if context.BatchID != "" {
		return "Return exactly one complete compact semantic_plan JSON object with planStep=proof_batch and proposals only for the supplied batch coverage IDs, and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. Echo the sealed batch identity exactly. A proofObligation with evidencePurpose=definition_proof must have one proof for every terminalRequiredEvidenceKinds value, and each proposal has exactly one requiredEvidenceKinds value. Specialized current-prerequisite rule: when evidencePurpose=owner_fact_acquisition and ownerFactAcquisition is present, currentRequiredEvidenceKinds and that object define the exact current acquisition proof for this generation and replace the terminal-evidence proposal rule for that CWI. Return exactly one proposal with ownerModuleId=acquisitionOwnerModuleId, operationKind=acquisitionOperationKind, operationVariantId=acquisitionOperationVariantId when present, target.policy=acquisitionTargetPolicy, target.ownerSelectorRefs=[acquisitionTargetSelectorRef], target.prospectiveConstraintRefs=acquisitionConstraintRefs when required, one concrete non-empty target.prospectiveTargetRef selected within the declaration selector constraints and declarationTargetSelectorCandidateRefs when that bounded set is present, and requiredEvidenceKinds=[\"state_change\"]. When prospectiveTargetConstraint is present, the target must satisfy every owner-authored predicate in prospectiveTargetConstraint.allOf conjunctively; do not invent an alternative, weaken a predicate, or derive a target from an owner message. The proposed prospectiveTargetRef is not Definition authority: the required fact owners must evaluate that exact proposal and only an SM-2 CoverageEvidenceBinding may seal it as Runtime authority before dispatch. The final validation surface and validationRecipeId are expected to be unavailable until that acquisition is executed and its required owner is reobserved; do not return gapKind=undeclared_prerequisite solely because either is not currently selectable. The acquisition proposal proves only the prerequisite state change: terminalEvidenceCredit=false, so it does not grant terminal validation evidence or coverage credit, and you must not also propose the terminal validation proof in this generation. Match every operation kind, variant and target policy to the referenced ownerCatalog fact; never apply a prospective/create-only operation fact to a target already listed by the owner catalog. For terminal validation evidence use only mcp/tool_call or validation/managed_session and select a validationRecipeId published by the same ownerCatalog; an unpublished prerequisite-required identity is never terminal validation proof. Never use file/read, file/write, shell/command, git/write, or patch/apply as terminal validation evidence. If and only if the supplied batch cannot form a complete all-of proof plan because the sealed Definition implies alternatives, truly has no declared prerequisite acquisition, or is semantically ambiguous, return the definition_clarification_required schema branch with the exact batch identity instead. Do not return coverageOrder, add or omit batch coverage, claim coverage outside the batch, or invent a ticket/action."
	}
	return "Return exactly one complete compact semantic_plan JSON object and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or unrequested proposals, and do not repeat the Goal, DoD, manifest, authority, or input context. For every manifest coverage item with evidencePurpose=definition_proof, return one bounded proof declaration for each terminalRequiredEvidenceKinds value. Each proposal has exactly one requiredEvidenceKinds value. For an item with evidencePurpose=owner_fact_acquisition, return only its exact current prerequisite acquisition proposal using currentRequiredEvidenceKinds=[\"state_change\"]; when prospectiveTargetConstraint is present, its allOf predicates are conjunctive target requirements and every predicate must be satisfied. The temporarily unavailable final validation surface is not by itself an undeclared prerequisite, and this acquisition grants no terminal validation evidence or coverage credit. Omit optional arrays and fields when empty. Match operationKind, operationVariantId and target.policy to one ownerCatalog operation fact; an exact or existing target must use a kind/variant whose allowedTargetPolicies includes that policy, while a prospective policy is only for an absent owner-authorized target. A terminal validation evidence proof must use only operationKind=mcp/tool_call or operationKind=validation/managed_session and a validationRecipeId published by the same ownerCatalog. File/read, file/write, shell/command, git/write, and patch/apply cannot claim terminal validation evidence. Do not return operations, runtime actions, coverage credit, terminal state, retry policy, governance tickets, receipts, or owner results."
}

func runtimeSemanticBatchSequence(context runtimecontract.RuntimeSemanticPlanRequestContext) *int {
	if context.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || context.BatchID == "" {
		return nil
	}
	value := context.BatchSequence
	return &value
}
