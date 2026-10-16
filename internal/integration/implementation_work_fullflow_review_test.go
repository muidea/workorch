package integration_test

import (
	"encoding/json"
	"slices"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestImplementationWorkFullflowReviewUsesExactIndependentSourceCriteria(t *testing.T) {
	var validation implementationWorkFullflowValidation
	// Use the actual provider-safe wire names, not the Runtime struct encoding.
	err := json.Unmarshal([]byte(`{"requiredCheckRefs":["workorch-obligation-check-A","workorch-obligation-check-B","profile"],"sourceObligationChecks":[{"checkRef":"workorch-obligation-check-A","criterion":"A CLI 输出 4","declaration":"执行 CLI 并检查 4"},{"checkRef":"workorch-obligation-check-B","criterion":"B Git owner receipt 独立确认","declaration":"查询实际 Git owner receipt"}]}`), &validation)
	if err != nil {
		t.Fatal(err)
	}
	files := []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "checks/delivery_test.go", Content: implementationWorkFullflowInput("checks/delivery_test.go", []string{"TestDelivery"}, "共同 CWI")}}
	refs := []string{files[0].TargetRef}
	for _, test := range []struct {
		check, criterion, conclusion string
	}{
		{"workorch-obligation-check-A", "A CLI 输出 4", runtimecontract.RuntimeValidationInputConsistent},
		{"workorch-obligation-check-B", "B Git owner receipt 独立确认", runtimecontract.RuntimeValidationInputInconclusive},
		{"profile", "共同 CWI", runtimecontract.RuntimeValidationInputConsistent},
	} {
		t.Run(test.check, func(t *testing.T) {
			item, err := implementationWorkFullflowReviewCheck(&validation, test.check, "共同 CWI", runtimecontract.RuntimeValidationInputReviewProtocolV2, files, "isolated package", refs)
			if err != nil || item.Conclusion != test.conclusion || len(item.Obligations) != 1 || item.Obligations[0].CriterionText != test.criterion {
				t.Fatalf("independent source review drifted: %+v err=%v", item, err)
			}
		})
	}
	// Even a behavioral criterion cannot prove a declaration requiring owner facts.
	validation.SourceChecks[0].Declaration = "查询真实 Git owner receipt"
	item, err := implementationWorkFullflowReviewCheck(&validation, validation.Checks[0], "共同 CWI", runtimecontract.RuntimeValidationInputReviewProtocolV2, files, "isolated package", refs)
	if err != nil || item.Conclusion != runtimecontract.RuntimeValidationInputInconclusive {
		t.Fatal("source declaration authority was ignored")
	}
	validation.SourceChecks = validation.SourceChecks[:1]
	if _, err := implementationWorkFullflowReviewCheck(&validation, validation.Checks[1], "共同 CWI", runtimecontract.RuntimeValidationInputReviewProtocolV2, files, "isolated package", refs); err == nil {
		t.Fatal("missing source criterion fell back to shared CWI")
	}
	validation.SourceChecks = append(validation.SourceChecks, validation.SourceChecks[0])
	if _, err := implementationWorkFullflowReviewCheck(&validation, validation.Checks[0], "共同 CWI", runtimecontract.RuntimeValidationInputReviewProtocolV2, files, "isolated package", refs); err == nil {
		t.Fatal("duplicate source identity accepted")
	}
}

func TestImplementationWorkFullflowReviewCompositionPreservesOrdinaryAndTypedConsumers(t *testing.T) {
	const criterion = "最终 CLI 输出4；同一原CWI两轮工作，真实失败后修复，全部交付确认"
	rules := []runtimecontract.RuntimeLineageAssertionRule{
		{Ref: "rounds", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 2},
		{Ref: "repair", Kind: runtimecontract.RuntimeLineageRepairThenFreshValidation, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution},
		{Ref: "delivery", Kind: runtimecontract.RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery},
	}
	assertions, err := runtimecontract.CompileRuntimeLineageAssertions(rules, "original-declaration", runtimecontract.RuntimeCoverageItem{ID: "original-cwi", SourceScope: "dodItem", SourceID: "original-dod", TargetText: criterion})
	if err != nil {
		t.Fatal(err)
	}
	validation := implementationWorkFullflowValidation{Checks: []string{"check"}}
	files := []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "checks/delivery_test.go", Content: implementationWorkFullflowInput("checks/delivery_test.go", []string{"TestDelivery"}, criterion)}}
	refs := []string{files[0].TargetRef}
	scope := "Shell 的根 module build/test/race 另外实际执行"
	review := func(input []runtimecontract.RuntimeValidationInputReviewFile, consumers []runtimecontract.RuntimeLineageAssertion) runtimecontract.RuntimeValidationInputCheckReview {
		t.Helper()
		item, err := implementationWorkFullflowReviewCheck(&validation, "check", criterion, runtimecontract.RuntimeValidationInputReviewProtocolV3, input, scope, refs, consumers)
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	item := review(files, assertions)
	if item.Conclusion != runtimecontract.RuntimeValidationInputConsistent || len(item.Obligations) != 1 || item.Obligations[0].CriterionText != criterion || !slices.Equal(item.Obligations[0].ProofConsumerRefs, []string{"delivery", "repair", "rounds", runtimecontract.RuntimeValidationInputConsumer}) {
		t.Fatalf("composition changed criterion or omitted consumer: %+v", item)
	}
	for index := range assertions {
		missing := slices.Clone(assertions)
		missing = slices.Delete(missing, index, index+1)
		if review(files, missing).Conclusion != runtimecontract.RuntimeValidationInputInconclusive {
			t.Fatal("missing Runtime/terminal consumer bypassed by input")
		}
	}
	if review([]runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: refs[0], Content: "// PASS"}}, assertions).Conclusion != runtimecontract.RuntimeValidationInputConflict {
		t.Fatal("typed lineage replaced ordinary engineering proof")
	}
	drifted := slices.Clone(assertions)
	drifted[1].CoverageWorkItemID = "other-cwi"
	if review(files, drifted).Conclusion != runtimecontract.RuntimeValidationInputConflict {
		t.Fatal("cross-CWI assertion substituted without original subject")
	}
	drifted = slices.Clone(assertions)
	drifted[0].Rule.MinCompletedRounds = 1
	drifted[0].RuleHash = runtimecontract.StableRuntimeLineageAssertionRuleHash(drifted[0].Rule)
	if review(files, drifted).Conclusion != runtimecontract.RuntimeValidationInputConflict {
		t.Fatal("rehashed single-round rule weakened two-round criterion")
	}
	if _, err := implementationWorkFullflowReviewCheck(&validation, "check", criterion, runtimecontract.RuntimeValidationInputReviewProtocolV2, files, scope, refs, assertions); err == nil {
		t.Fatal("legacy review silently carried composite declarations")
	}
}
