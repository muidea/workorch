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
		{TargetRef: "a", Baseline: &rc.RuntimeValidationInputReviewFile{TargetRef: "a", Content: intermediate, ContentHash: rc.RuntimeValidationInputContentHash(intermediate)}, Operation: rc.RuntimeOperation{Parameters: []rc.OperationParameter{{Name: "old", String: &intermediate}, {Name: "new", String: &current}}}},
	}}
	c := original.Clone()
	baselineRefs, refs := compactChangeReviewBaselinesAndValues(c, nil)
	if len(baselineRefs) != 1 || baselineRefs[0].Change != 1 || baselineRefs[0].SourceKind != "parameter" || baselineRefs[0].SourceIndex != 0 || baselineRefs[0].SourceParameter != 1 || c.Changes[1].Baseline.Content != "" {
		t.Fatal("did not compact duplicate baseline against earlier parameter", baselineRefs)
	}
	if len(refs) != 3 || refs[0].SourceKind != "baseline" || refs[1].SourceKind != "parameter" || refs[2].SourceKind != "current" {
		t.Fatal("did not share exact typed bodies", refs)
	}
	before, _ := json.Marshal(original)
	compressed, _ := json.Marshal(struct {
		Context      any
		BaselineRefs any
		Refs         any
	}{c, baselineRefs, refs})
	if len(compressed) >= len(before) {
		t.Fatal("reference overhead defeated large-value reuse")
	}
	if err := expandChangeReviewBaselinesAndValues(c, baselineRefs, refs, nil); err != nil || !reflect.DeepEqual(c, original) {
		t.Fatal("lossy roundtrip", err)
	}
	for _, name := range []string{"duplicate", "unordered", "foreign-target", "cycle", "baseline-cycle", "baseline-foreign-source", "baseline-lost-content", "missing-source", "mixed-value", "changed-source", "unknown-kind"} {
		t.Run(name, func(t *testing.T) {
			v := original.Clone()
			b, r := compactChangeReviewBaselinesAndValues(v, nil)
			switch name {
			case "duplicate":
				r = append(r, r[len(r)-1])
			case "unordered":
				r[0], r[1] = r[1], r[0]
			case "foreign-target":
				r[2].SourceIndex = 1
			case "cycle":
				r[1].SourceIndex = r[1].Change
			case "baseline-cycle":
				b[0].SourceIndex = b[0].Change
			case "baseline-foreign-source":
				b[0].SourceKind = "path"
				b[0].Source = "C3.workContext.content.snapshots[0].Content"
				b[0].SourceIndex = 0
				b[0].SourceParameter = 0
			case "baseline-lost-content":
				b = nil
			case "missing-source":
				v.Changes[0].Baseline = nil
			case "mixed-value":
				v.Changes[r[0].Change].Operation.Parameters[r[0].Parameter].String = &current
			case "changed-source":
				v.Changes[0].Baseline.Content += "altered"
			case "unknown-kind":
				r[0].SourceKind = "external"
			}
			err := expandChangeReviewBaselinesAndValues(v, b, r, nil)
			if name == "baseline-lost-content" {
				if reflect.DeepEqual(v, original) {
					t.Fatal("baseline stayed inline without ref")
				}
				return
			}
			if err == nil {
				t.Fatal("invalid local reference accepted")
			}
		})
	}
	after, _ := json.Marshal(original)
	if string(after) != string(before) {
		t.Fatal("original corpus mutated")
	}
}

func TestChangeReviewBaselineAndParameterPathSources(t *testing.T) {
	corpus := strings.Repeat("历史基线与提交参数原文\r\n", 100)
	hash := rc.RuntimeValidationInputContentHash(corpus)
	original := &rc.RuntimeChangeReviewContext{
		Changes: []rc.RuntimeChangeReviewOperation{
			{
				TargetRef: "pkg/a.go",
				Baseline:  &rc.RuntimeValidationInputReviewFile{TargetRef: "pkg/a.go", Content: corpus, ContentHash: hash},
				Operation: rc.RuntimeOperation{Parameters: []rc.OperationParameter{{Name: "oldText", String: &corpus}}},
			},
		},
	}
	sources := []runtimeChangeReviewBodySource{{
		ref:  "C3.manifest.validationGenerationContexts[0].context.Files[0].content",
		file: rc.RuntimeValidationInputReviewFile{TargetRef: "pkg/a.go", ContentHash: hash, Content: corpus},
	}}
	c := original.Clone()
	bRefs, vRefs := compactChangeReviewBaselinesAndValues(c, sources)
	if len(bRefs) != 1 || bRefs[0].SourceKind != "path" || len(vRefs) != 1 || vRefs[0].SourceKind != "path" {
		t.Fatalf("expected path compaction for baseline and parameter, got b=%+v v=%+v", bRefs, vRefs)
	}
	if c.Changes[0].Baseline.Content != "" || c.Changes[0].Operation.Parameters[0].String != nil {
		t.Fatal("baseline or parameter not compacted")
	}
	if err := expandChangeReviewBaselinesAndValues(c, bRefs, vRefs, sources); err != nil || !reflect.DeepEqual(c, original) {
		t.Fatalf("path expansion failed: %v", err)
	}
}
