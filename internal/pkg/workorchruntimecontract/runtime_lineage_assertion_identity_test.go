package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeLineageMinimumRoundsEntersSealedProfileIdentity(t *testing.T) {
	base := profileIdentityFixture()
	base[0].EvidenceRequirements[0].LineageAssertions = []RuntimeLineageAssertionRule{{
		Ref: "profile/completed-rounds", Kind: RuntimeLineageCompletedWorkRounds,
		ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2,
	}}
	if err := ValidateRuntimeProfileContracts(base); err != nil {
		t.Fatal(err)
	}
	before := CloneRuntimeProfileContracts(base)
	changed := CloneRuntimeProfileContracts(base)
	changed[0].EvidenceRequirements[0].LineageAssertions[0].MinCompletedRounds = 3
	if err := ValidateRuntimeProfileContracts(changed); err != nil {
		t.Fatal(err)
	}
	assertDifferent := func(left, right []RuntimeProfileContract) {
		t.Helper()
		if SameRuntimeProfileContracts(left, right) {
			t.Fatal("不同最少工作轮次被视为同一封存 Profile")
		}
		if StableRuntimeProfileContractsHash(left) == StableRuntimeProfileContractsHash(right) {
			t.Fatal("最少工作轮次未进入 admission hash")
		}
		prefix := func(profiles []RuntimeProfileContract) string {
			return StableRuntimeCandidatePrefixHashWithExecutionContract(RuntimeDefinitionContext{}, RuntimeExecutionContract{ProfileContracts: profiles})
		}
		if prefix(left) == prefix(right) {
			t.Fatal("最少工作轮次未进入 candidate prefix")
		}
	}
	assertDifferent(base, changed)
	for _, profiles := range [][]RuntimeProfileContract{base, changed} {
		encoded, err := json.Marshal(profiles)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded []RuntimeProfileContract
		if err := json.Unmarshal(encoded, &reloaded); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRuntimeProfileContracts(reloaded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(profiles, reloaded) || !SameRuntimeProfileContracts(profiles, reloaded) || StableRuntimeProfileContractsHash(profiles) != StableRuntimeProfileContractsHash(reloaded) {
			t.Fatal("JSON 重载改变原 lineage 声明或封存身份")
		}
		if reloaded[0].EvidenceRequirements[0].LineageAssertions[0].MinCompletedRounds == 2 {
			assertDifferent(reloaded, changed)
		} else {
			assertDifferent(base, reloaded)
		}
	}
	if !reflect.DeepEqual(base, before) {
		t.Fatal("身份比较、hash 或重载修改原 Profile")
	}
}
