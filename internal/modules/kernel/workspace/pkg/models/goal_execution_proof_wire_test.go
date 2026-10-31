package workspace

import (
	"encoding/json"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGoalExecutionProofWireVersionAndCompleteText(t *testing.T) {
	_, mapping, _ := fullSizeMappingFixture(t)
	m := GoalExecutionProofMapping{ContractRef: runtimecontract.RuntimeGoalSourceExecutionProof, ConsumerItemRef: "goal", RuleRef: "work", SourceItemRef: "dod", ConsumerGoalText: " 完整原 Goal\r\n不能摘要。 "}
	mapping.ExecutionProofMappings = []GoalExecutionProofMapping{m}
	body := mappingAuditFixture(t, mapping).Document.Content
	decoded, err := decodeCurrentMappingFixture(body)
	if err != nil || decoded.ExecutionProofMappings[0] != m {
		t.Fatalf("strict v2 wire: %v", err)
	}
	for _, replacement := range []string{`"ConsumerGoalText":null`, `"ConsumerGoalText":false`, `"ConsumerGoalText":"","ConsumerGoalText":"duplicate"`, `"ConsumerGoalText":"","Unknown":"x"`} {
		original, _ := json.Marshal(m.ConsumerGoalText)
		changed := strings.Replace(string(body), `"ConsumerGoalText":`+string(original), replacement, 1)
		if _, err := decodeCurrentMappingFixture([]byte(changed)); err == nil {
			t.Fatal("invalid Goal wire accepted", replacement)
		}
	}
	legacy := strings.Replace(string(body), runtimecontract.RuntimeGoalSourceExecutionProof, runtimecontract.RuntimeDoDSourceExecutionProof, 1)
	if _, err := decodeCurrentMappingFixture([]byte(legacy)); err == nil {
		t.Fatal("legacy wire accepted Goal extension")
	}
	mapping.ExecutionProofMappings[0].ConsumerGoalText = ""
	body = mappingAuditFixture(t, mapping).Document.Content
	if _, err := decodeCurrentMappingFixture(body); err == nil {
		t.Fatal("v2 wire accepted missing full Goal")
	}
}
