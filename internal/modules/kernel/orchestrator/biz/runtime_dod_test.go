package orchestrator

import (
	"errors"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateDoDAssessmentCoversRunDoD_LeafOnlyCoverageAndDiagnostics(t *testing.T) {
	run := &runModel.Run{
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{
					"### 验收清单",
					"- [ ] 配置管理：",
					"  - [ ] 配置文件必须被代码真实加载并有测试断言",
					"  - [ ] 错误路径必须可观测并包含断言",
				},
			},
		},
	}
	assessment := map[string]any{
		"items": []map[string]any{
			{
				"item":         "配置文件必须被代码真实加载并有测试断言",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-1/block/verify-config"},
			},
		},
	}

	err := validateDoDAssessmentCoversRunDoD(run, assessment)
	if err == nil {
		t.Fatalf("validateDoDAssessmentCoversRunDoD = nil, want missing DoD error")
	}
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("errors.Is(err, common.ErrInvalidArgument) = false, err=%v", err)
	}
	var diag runtimeFailureDiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("errors.As(err, runtimeFailureDiagnosticError) = false, err=%v", err)
	}
	diagnostics := diag.RuntimeFailureDiagnostics()
	if diagnostics["kind"] != failureClassDoDCoverageMissing {
		t.Fatalf("runtimeDiagnostics.kind = %v, want %s", diagnostics["kind"], failureClassDoDCoverageMissing)
	}
	missing := mapSliceFromValue(diagnostics["missingDoDItems"])
	if len(missing) != 1 {
		t.Fatalf("missingDoDItems = %#v, want one leaf missing item", diagnostics["missingDoDItems"])
	}
	if missing[0]["raw"] != "- [ ] 错误路径必须可观测并包含断言" || missing[0]["kind"] != "leaf" {
		t.Fatalf("missingDoDItems[0] = %#v, want leaf raw item", missing[0])
	}
}

func TestDoDRequiredEvidenceHintStaysProfileNeutral(t *testing.T) {
	cases := []string{
		"配置文件必须被代码真实加载并有测试断言",
		"并发请求必须安全",
		"stream verification must pass",
	}
	want := "Provide concrete evidence refs for this DoD item according to the active runtime stage and profile contract."
	for _, tc := range cases {
		if got := doDRequiredEvidenceHint(tc); got != want {
			t.Fatalf("doDRequiredEvidenceHint(%q) = %q, want %q", tc, got, want)
		}
	}
}

func TestValidateDoDAssessmentCoversRunGoal_RequiresIndependentChangedGoalLeaf(t *testing.T) {
	run := &runModel.Run{
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "- 保持现有交付物的可用能力\n- 新增需求必须形成独立验收覆盖项",
			},
			"definitionChangeImpact": map[string]any{
				"changed": true,
			},
		},
	}
	assessment := map[string]any{
		"goalCoverage": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "保持现有交付物的可用能力，并新增需求必须形成独立验收覆盖项。",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-001/old-config-test"},
				},
			},
		},
	}

	err := validateDoDAssessmentCoversRunGoal(run, assessment)
	if err == nil {
		t.Fatalf("validateDoDAssessmentCoversRunGoal = nil, want missing changed Goal leaf")
	}
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("errors.Is(err, common.ErrInvalidArgument) = false, err=%v", err)
	}
	var diag runtimeFailureDiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("errors.As(err, runtimeFailureDiagnosticError) = false, err=%v", err)
	}
	missing := mapSliceFromValue(diag.RuntimeFailureDiagnostics()["missingGoalCoverage"])
	found := false
	for _, item := range missing {
		if item["raw"] == "新增需求必须形成独立验收覆盖项" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missingGoalCoverage = %#v, want changed independent Goal leaf", missing)
	}
}
