package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// Applicability is sampled after receipt admission. When that exact owner
// observation consumes a confirmed repair's fresh-validation handoff, SM-2
// must retire the directive even if unrelated prerequisites are still pending.
// Historical consumption never restores current proof or CWI credit.
func retireConsumedConfirmedRepairDirective(aggregate RuntimeAggregate) (runtimecontract.RuntimeFacts, error) {
	facts := aggregate.Facts
	if facts.RepairDirective == nil || facts.RepairDirective.Closure == nil {
		return facts, nil
	}
	directive := *facts.RepairDirective
	if directive.ID != "confirmed-repair/"+facts.Focus.ID+"/"+directive.Closure.ID+"/fresh-validation" {
		return facts, nil
	}
	transactions, err := completedConfirmedRepairTransactions(aggregate)
	if err != nil {
		return facts, err
	}
	for _, completed := range transactions {
		transaction := completed.transaction
		if !sameCoverageIDs(transaction.closure.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
			!confirmedRepairSourceUsesCurrentBinding(facts, transaction.sourceOp.BindingRefs, transaction.closure.CoverageWorkItemIDs) {
			continue
		}
		expected := transaction.freshValidationDirective(facts, completed.finalEvidenceRevision)
		if runtimecontract.SameRuntimeRepairDirective(&directive, &expected) && confirmedRepairFreshValidationConsumed(aggregate, expected) {
			facts.RepairDirective = nil
			return facts, nil
		}
	}
	return facts, nil
}
