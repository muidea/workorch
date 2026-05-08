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
