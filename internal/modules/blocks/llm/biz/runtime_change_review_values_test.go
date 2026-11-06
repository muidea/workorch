package llm

import (
	"encoding/json"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"strings"
	"testing"
)

func TestChangeReviewValueReferencesPreserveEveryOriginalParameter(t *testing.T) {
	current, baseline, intermediate := strings.Repeat("当前原文\r\n", 100), strings.Repeat("前像\r\n", 100), strings.Repeat("中间版本\r\n", 100)
	original := &rc.RuntimeChangeReviewContext{Current: []rc.RuntimeValidationInputReviewFile{{TargetRef: "a", Content: current, ContentHash: rc.RuntimeValidationInputContentHash(current)}, {TargetRef: "b", Content: current, ContentHash: rc.RuntimeValidationInputContentHash(current)}}, Changes: []rc.RuntimeChangeReviewOperation{
		{TargetRef: "a", Baseline: &rc.RuntimeValidationInputReviewFile{TargetRef: "a", Content: baseline, ContentHash: rc.RuntimeValidationInputContentHash(baseline)}, Operation: rc.RuntimeOperation{Parameters: []rc.OperationParameter{{Name: "opaque-left", String: &baseline}, {Name: "opaque-right", String: &intermediate}}}},
		{TargetRef: "a", Operation: rc.RuntimeOperation{Parameters: []rc.OperationParameter{{Name: "old", String: &intermediate}, {Name: "new", String: &current}}}},
	}}
	c := original.Clone()
	refs := compactChangeReviewValues(c)
	if len(refs) != 3 || refs[0].SourceKind != "baseline" || refs[1].SourceKind != "parameter" || refs[2].SourceKind != "current" {
		t.Fatal("did not share exact typed bodies", refs)
	}
	before, _ := json.Marshal(original)
	compressed, _ := json.Marshal(struct {
		Context any
		Refs    any
	}{c, refs})
	if len(compressed) >= len(before) {
		t.Fatal("reference overhead defeated large-value reuse")
	}
	if err := expandChangeReviewValues(c, refs); err != nil || !reflect.DeepEqual(c, original) {
		t.Fatal("lossy roundtrip", err)
	}
	for _, name := range []string{"duplicate", "unordered", "foreign-target", "cycle", "missing-source", "mixed-value", "changed-source", "unknown-kind"} {
		t.Run(name, func(t *testing.T) {
			v := original.Clone()
			r := compactChangeReviewValues(v)
			switch name {
			case "duplicate":
				r = append(r, r[len(r)-1])
			case "unordered":
				r[0], r[1] = r[1], r[0]
			case "foreign-target":
				r[2].SourceIndex = 1
			case "cycle":
				r[1].SourceIndex = r[1].Change
			case "missing-source":
				v.Changes[0].Baseline = nil
			case "mixed-value":
				v.Changes[r[0].Change].Operation.Parameters[r[0].Parameter].String = &current
			case "changed-source":
				v.Changes[0].Baseline.Content += "altered"
			case "unknown-kind":
				r[0].SourceKind = "external"
			}
			if expandChangeReviewValues(v, r) == nil {
				t.Fatal("invalid local reference accepted")
			}
		})
	}
	after, _ := json.Marshal(original)
	if string(after) != string(before) {
		t.Fatal("original corpus mutated")
	}
}
