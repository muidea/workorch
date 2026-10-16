package workspace

import (
	"encoding/json"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGoalExecutionProofWireVersionAndCompleteText(t *testing.T) {
	_, mapping, _ := fullSizeMappingFixture(t)
	m := GoalExecutionProofMapping{ContractRef: runtimecontract.RuntimeGoalSourceExecutionProofV2, ConsumerItemRef: "goal", RuleRef: "work", SourceItemRef: "dod", ConsumerGoalText: " 完整原 Goal\r\n不能摘要。 "}
	mapping.ExecutionProofMappings = []GoalExecutionProofMapping{m}
	body, _ := json.Marshal(struct {
		Mapping DefinitionAuthoringMapping `json:"mapping"`
	}{mapping})
	decoded, err := DecodeDefinitionAuthoringMapping(body)
	if err != nil || decoded.ExecutionProofMappings[0] != m {
		t.Fatalf("strict v2 wire: %v", err)
	}
	for _, replacement := range []string{`"ConsumerGoalText":null`, `"ConsumerGoalText":false`, `"ConsumerGoalText":"","ConsumerGoalText":"duplicate"`, `"ConsumerGoalText":"","Unknown":"x"`} {
		original, _ := json.Marshal(m.ConsumerGoalText)
		changed := strings.Replace(string(body), `"ConsumerGoalText":`+string(original), replacement, 1)
		if _, err := DecodeDefinitionAuthoringMapping([]byte(changed)); err == nil {
			t.Fatal("invalid Goal wire accepted", replacement)
		}
	}
	legacy := strings.Replace(string(body), runtimecontract.RuntimeGoalSourceExecutionProofV2, runtimecontract.RuntimeCrossSourceExecutionProofV1, 1)
	if _, err := DecodeDefinitionAuthoringMapping([]byte(legacy)); err == nil {
		t.Fatal("legacy wire accepted Goal extension")
	}
	mapping.ExecutionProofMappings[0].ConsumerGoalText = ""
	body, _ = json.Marshal(struct {
		Mapping DefinitionAuthoringMapping `json:"mapping"`
	}{mapping})
	if _, err := DecodeDefinitionAuthoringMapping(body); err == nil {
		t.Fatal("v2 wire accepted missing full Goal")
	}
}
