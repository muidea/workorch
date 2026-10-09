package workorchruntimecontract

import "testing"

func TestRuntimeLineageAssertionClosedUnionCannotGrantStandaloneCredit(t *testing.T) {
	base := RuntimeLineageAssertionRule{
		Ref: "profile/rounds", Kind: RuntimeLineageCompletedWorkRounds,
		ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2,
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeLineageAssertionRule){
		"unknown kind": func(r *RuntimeLineageAssertionRule) { r.Kind = "arbitrary_predicate" },
		"wrong consumption point": func(r *RuntimeLineageAssertionRule) {
			r.ConsumptionPoint = RuntimeProofPhasePostTerminalDelivery
		},
		"zero rounds": func(r *RuntimeLineageAssertionRule) { r.MinCompletedRounds = 0 },
		"unbounded rounds": func(r *RuntimeLineageAssertionRule) {
			r.MinCompletedRounds = MaxRuntimeImplementationWorkRounds + 1
		},
		"mixed union":     func(r *RuntimeLineageAssertionRule) { r.OperationKind = StepKindFileWrite },
		"trim laundering": func(r *RuntimeLineageAssertionRule) { r.Ref += " " },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := base
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("非法 lineage union 规则被接受")
			}
		})
	}
	if err := ValidateRuntimeLineageAssertionRules([]RuntimeLineageAssertionRule{base, base}); err == nil {
		t.Fatal("重复 lineage 规则被接受")
	}
	_, binding := lineageSourceFixture(t, base)
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	binding.Proofs = nil
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err == nil {
		t.Fatal("lineage guard 代替普通覆盖 proof")
	}
}
