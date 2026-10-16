package workspace

import (
	"encoding/json"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProfileEvidencePurposeWireVersionFieldsAndFullText(t *testing.T) {
	m := GoalEvidencePurposeMapping{ContractRef: runtimecontract.RuntimeProfileDeliverableEvidencePurposeV2,
		HistoricalItemRef: "profile/p/deliverable/d", AcceptanceItemRef: "original-dod",
		HistoricalProfileID: "p", HistoricalDeliverableID: "d", HistoricalProfileHash: "snapshot", HistoricalText: "  全文\n不能摘要。  "}
	body, _ := json.Marshal(m)
	shape := *evidencePurposeMappingsShape.item
	if err := shape.validate(body, "$.mapping"); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HistoricalProfileID", "HistoricalDeliverableID", "HistoricalProfileHash", "HistoricalText"} {
		original := fields[key]
		for _, value := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("false")} {
			if value == nil {
				delete(fields, key)
			} else {
				fields[key] = value
			}
			raw, _ := json.Marshal(fields)
			if shape.validate(raw, "$.mapping") == nil {
				t.Fatal("required Profile field absent/null/wrong type", key)
			}
		}
		fields[key] = original
	}
	fields["ContractRef"] = json.RawMessage(`"runtime/cross-cwi-evidence-purpose/v1"`)
	for _, key := range []string{"HistoricalProfileID", "HistoricalDeliverableID", "HistoricalProfileHash", "HistoricalText"} {
		fields[key] = json.RawMessage(`""`)
	}
	legacy, _ := json.Marshal(fields)
	if shape.validate(legacy, "$.mapping") == nil {
		t.Fatal("v1 accepted even empty Profile extension")
	}
	var reload GoalEvidencePurposeMapping
	if json.Unmarshal(body, &reload) != nil || reload != m {
		t.Fatal("raw text/identity changed on reload")
	}
}
