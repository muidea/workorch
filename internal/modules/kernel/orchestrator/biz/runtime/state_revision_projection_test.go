package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeStateRevisionProjectionKeepsCompleteGate(t *testing.T) {
	_, _, aggregate := newSemanticPlanningController(t)
	body, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var original runtimecontract.RuntimeStateRecord
	if err := json.Unmarshal(body, &original); err != nil {
		t.Fatal(err)
	}
	if err := original.Validate(); err != nil {
		t.Fatalf("缺少有效 opaque state 前像：%v", err)
	}
	for _, scenario := range []string{"valid", "definition-trim", "facts-trim", "mismatch", "missing-definition-before-mismatch", "invalid-facts-before-mismatch", "invalid-policy-before-mismatch", "invalid-seed-before-mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			value := original.Clone()
			var expected string
			switch scenario {
			case "definition-trim":
				value.Definition.Revision = " \t" + value.Definition.Revision + "\n"
			case "facts-trim":
				value.Facts.DefinitionRevision = " \t" + value.Facts.DefinitionRevision + "\n"
			case "mismatch":
				value.Definition.Revision = "different"
				expected = "runtime state definition revision mismatch"
			case "missing-definition-before-mismatch":
				value.Definition.Revision, value.Definition.GoalDefinition = "different", ""
				expected = "runtime definition context requires Goal and DoD definitions"
			case "invalid-facts-before-mismatch":
				value.Definition.Revision, value.Facts.EvidenceRevision = "different", ""
				expected = "runtime state evidence ledger: runtime facts evidence revision"
			case "invalid-policy-before-mismatch":
				value.Definition.Revision, value.Facts.ExecutionBudgetPolicy.PolicyHash = "different", "different"
				expected = "policy hash"
			case "invalid-seed-before-mismatch":
				value.Definition.Revision, value.SemanticPlanning.DefinitionSeed.Hash = "different", "different"
				expected = "seed"
			}
			before := value.Clone()
			for range 2 {
				oldMismatch := value.Definition.Normalized().Revision != value.Facts.Normalized().DefinitionRevision
				if mismatch := strings.TrimSpace(value.Definition.Revision) != strings.TrimSpace(value.Facts.DefinitionRevision); mismatch != oldMismatch {
					t.Fatal("revision 原完整投影比较漂移")
				}
				err := value.Validate()
				if expected == "" {
					if err != nil {
						t.Fatalf("有效 trim 语义漂移：%v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), expected) {
					t.Fatalf("门禁错误顺序漂移：got=%v want=%s", err, expected)
				}
			}
			if !reflect.DeepEqual(value, before) {
				t.Fatal("revision 校验改写原 opaque record")
			}
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var loaded runtimecontract.RuntimeStateRecord
			if err := json.Unmarshal(body, &loaded); err != nil {
				t.Fatal(err)
			}
			got, want := loaded.Validate(), value.Validate()
			if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
				t.Fatalf("重载错误漂移：got=%v want=%v", got, want)
			}
		})
	}
	original.Definition.Revision = "later-revision"
	if err := original.Validate(); err == nil || err.Error() != "runtime state definition revision mismatch" {
		t.Fatalf("后续漂移借用先前校验：%v", err)
	}
}
