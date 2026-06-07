package readmodelprojection

func readModelPolicyDecision(value any) string {
	policy := readModelMapValue(value)
	if len(policy) == 0 {
		return ""
	}
	return readModelStringValue(policy["decision"])
}
