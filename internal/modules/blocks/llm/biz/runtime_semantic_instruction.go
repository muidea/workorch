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
	return "Return exactly one complete compact JSON object and finish it in this response. Emit only schema fields; never add prose, a preface or suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. You propose a bounded semantic proof plan for a sealed WorkOrch definition. The proposal is not authority and has no side effects. Use only the owner modules, operation kinds, variants, target policies, evidence kinds, effect scopes, declarations, and identities supplied in C3. Operation kind, variant and target policy must belong to the same ownerCatalog fact; prospective/create-only authority cannot be used for an existing owner target. Each proposal is one independently feasible proof and has exactly one evidence kind. Group proofs by coverageWorkItemId: return one proof for every required evidence kind of each coverage item, with no extra CWI or duplicate evidence kind. Validation evidence is receipt-backed and may use only mcp/tool_call or validation/managed_session with effect none or ephemeral_session; file/read, file/write, shell/command, git/write, and patch/apply are never validation proof operations. Never emit operation execution, runtime action, coverage state or credit, terminal/retry/governance decisions, receipts, or owner observations."
}

func runtimeSemanticOutputInstruction(context runtimecontract.RuntimeSemanticPlanRequestContext) string {
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return "Return exactly one complete compact semantic_plan JSON object with planStep=coverage_order and a complete coverageOrder, and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or repeated Goal, DoD, manifest, authority, or input context. Do not return proposals or any operation, target, recipe, feasibility, action, credit, terminal, retry, governance, or receipt field."
	}
	if context.BatchID != "" {
		return "Return exactly one complete compact semantic_plan JSON object with planStep=proof_batch and proposals only for the supplied batch coverage IDs, and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. Every batch CWI must have one proof for each required evidence kind; each proposal has exactly one requiredEvidenceKinds value, so a CWI requiring state_change and validation has two proposals. Echo the sealed batch identity exactly. Match operationKind, operationVariantId and target.policy to one ownerCatalog operation fact; never apply a prospective/create-only operation fact to a target already listed by the owner catalog. For validation evidence use only mcp/tool_call or validation/managed_session with a non-durable effect; never use file/read, file/write, shell/command, git/write, or patch/apply for validation. Do not return coverageOrder, add or omit batch coverage, or claim coverage outside the batch."
	}
	return "Return exactly one complete compact semantic_plan JSON object and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or unrequested proposals, and do not repeat the Goal, DoD, manifest, authority, or input context. For every manifest coverage item, return one bounded proof declaration for each required evidence kind. Each proposal has exactly one requiredEvidenceKinds value; a CWI requiring state_change and validation therefore has two proposals with the same coverageWorkItemId. Omit optional arrays and fields when empty. Match operationKind, operationVariantId and target.policy to one ownerCatalog operation fact; an exact or existing target must use a kind/variant whose allowedTargetPolicies includes that policy, while a prospective policy is only for an absent owner-authorized target. A validation evidence proof must use only operationKind=mcp/tool_call or operationKind=validation/managed_session and a non-durable effect (none or ephemeral_session); file/read, file/write, shell/command, git/write, and patch/apply cannot claim validation evidence. Do not return operations, runtime actions, coverage credit, terminal state, retry policy, governance tickets, receipts, or owner results."
}

func runtimeSemanticBatchSequence(context runtimecontract.RuntimeSemanticPlanRequestContext) *int {
	if context.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || context.BatchID == "" {
		return nil
	}
	value := context.BatchSequence
	return &value
}
