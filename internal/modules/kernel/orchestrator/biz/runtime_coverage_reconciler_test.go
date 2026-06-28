package orchestrator

import (
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func makeReconcileRun(goalText string, dodItems []string) *runmodel.Run {
	goal := map[string]any{}
	if goalText != "" {
		goal = map[string]any{
			"id":   "goal-recon",
			"text": goalText,
		}
	}
	dod := map[string]any{}
	if len(dodItems) > 0 {
		dod = map[string]any{
			"id":    "dod-recon",
			"items": dodItems,
		}
	}
	ctx := map[string]any{}
	if len(goal) > 0 {
		ctx["goal"] = goal
	}
	if len(dod) > 0 {
		ctx["dod"] = dod
	}
	return &runmodel.Run{
		ID:               "run-recon-1",
		WorkspaceID:      "ws-recon",
		ExecutionContext: ctx,
	}
}

func reconcileStepShell(stepID string, command string) *runmodel.Step {
	return &runmodel.Step{
		ID:    common.StepID(stepID),
		RunID: "run-recon-1",
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"exitCode": 0,
			"command":  command,
		},
	}
}

func reconcileValidValidation(id string, stepID common.StepID) *runmodel.ValidationResult {
	return &runmodel.ValidationResult{
		ID:              common.ValidationResultID(id),
		WorkspaceID:     "ws-recon",
		RunID:           "run-recon-1",
		StepID:          stepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		Summary:         "ok",
		EvidenceRef:     string(stepID) + "/tool-call/runtime-execute",
		EvidenceQuality: common.EvidenceQualityValid,
	}
}

// findOriginalCwiByLeafSubstring locates the cwi map in run.ExecutionContext[coverageWorkItemsKey]
// (not a clone) whose targetText contains leafToken. Modifications to the returned map
// persist across the next ensure/rebuild call.
func findOriginalCwiByLeafSubstring(run *runmodel.Run, leafToken string) map[string]any {
	raw, ok := run.ExecutionContext[coverageWorkItemsKey]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []map[string]any:
		for _, m := range typed {
			if strings.Contains(stringFromMap(m, "targetText"), leafToken) {
				return m
			}
		}
	case []any:
		for _, item := range typed {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if strings.Contains(stringFromMap(m, "targetText"), leafToken) {
				return m
			}
		}
	}
	return nil
}

func TestReconcile_BindsValidShellToOpenCwiWithValidationKind(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)
	if run.ExecutionContext[coverageWorkItemsKey] == nil {
		t.Fatalf("coverageWorkItems not initialized after ensure")
	}

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-001", step.ID)
	validation.TargetRef = "go-build-verify"
	validation.Summary = "go-build-verify validation observed"

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if !changed || bound != 1 {
		t.Fatalf("reconcile = (changed=%v, bound=%d), want (true, 1)", changed, bound)
	}
	matched := findOriginalCwiByLeafSubstring(run, "go-build-verify")
	if matched == nil {
		t.Fatalf("cwi with targetText containing go-build-verify not found")
	}
	if status := strings.TrimSpace(stringFromMap(matched, "status")); status != "evidence_ready" {
		t.Fatalf("cwi status = %q, want evidence_ready (closeSatisfied=false; final satisfaction comes from LLM dodAssessment)", status)
	}
	accepted := mapStringSliceValue(matched["acceptedEvidenceByKind"])
	if len(accepted["validation"]) != 1 {
		t.Fatalf("accepted.validation = %v, want 1 ref", accepted["validation"])
	}
}

func TestReconcile_SkipsFailedShell(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	step.Output["exitCode"] = 1
	validation := reconcileValidValidation("val-002", step.ID)
	validation.EvidenceQuality = common.EvidenceQualityInvalid

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (changed=%v, bound=%d), want (false, 0)", changed, bound)
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-003", step.ID)
	validation.TargetRef = "go-build-verify"
	validation.Summary = "go-build-verify validation observed"

	if changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step}); !changed || bound != 1 {
		t.Fatalf("first reconcile = (%v,%d), want (true,1)", changed, bound)
	}
	firstRefs := mapStringSliceValue(findOriginalCwiByLeafSubstring(run, "go-build-verify")["acceptedEvidenceByKind"])["validation"]
	if len(firstRefs) != 1 {
		t.Fatalf("after first bind: accepted.validation = %v, want 1", firstRefs)
	}

	if changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step}); changed || bound != 0 {
		t.Fatalf("second reconcile = (%v,%d), want (false,0)", changed, bound)
	}
	secondRefs := mapStringSliceValue(findOriginalCwiByLeafSubstring(run, "go-build-verify")["acceptedEvidenceByKind"])["validation"]
	if len(secondRefs) != 1 {
		t.Fatalf("after second bind: accepted.validation = %v, want still 1", secondRefs)
	}
}

func TestReconcile_DoesNotBindDecisionCwi(t *testing.T) {
	run := makeReconcileRun("", []string{"需要审批后变更决策工单审批通过"})
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-004", step.ID)

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — must not bind decision cwi", changed, bound)
	}
}

func TestReconcile_RejectsStaticOnlyShellForExecutableBehavior(t *testing.T) {
	run := makeReconcileRun("- concurrency-safe 多客户端 并发 请求", nil)
	run.ExecutionContext["executionContract"] = map[string]any{
		"profileContracts": map[string]any{
			"software-dev": map[string]any{
				"evidenceRequirements": []map[string]any{{
					"id":                                "concurrency",
					"scope":                             "dod_item",
					"acceptedStepKinds":                 []string{"shell/command", "mcp/tool_call"},
					"requireExecutableBehaviorEvidence": true,
				}},
			},
		},
	}
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	step.Output["evidenceTags"] = []string{"static-only"}
	validation := reconcileValidValidation("val-005", step.ID)

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — static-only shell must be rejected for executable behavior requirement", changed, bound)
	}
}

func TestReconcile_DoesNotBindAcrossUnrelatedGraph(t *testing.T) {
	run := makeReconcileRun("- openai-protocol-compat 通过 api_base_url 接入", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-099/042-rotate-secrets", "vault")
	validation := reconcileValidValidation("val-006", step.ID)

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — unrelated graph leaf must not bind", changed, bound)
	}
}

func TestReconcile_DoesNotDemoteSatisfiedCwi(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	priorCwi := findOriginalCwiByLeafSubstring(run, "go-build-verify")
	if priorCwi == nil {
		t.Fatalf("cwi with targetText containing go-build-verify not found")
	}
	priorCwi["status"] = "satisfied"
	priorCwi["acceptedEvidenceByKind"] = map[string]any{"validation": []string{"prior-evidence-ref"}}
	priorCwi["acceptedEvidenceRefs"] = []string{"prior-evidence-ref"}
	priorCwi["missingEvidenceKinds"] = []string{}
	priorCwi["currentGap"] = ""

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-007", step.ID)

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — already-satisfied cwi must not be touched", changed, bound)
	}
	matched := findOriginalCwiByLeafSubstring(run, "go-build-verify")
	if status := strings.TrimSpace(stringFromMap(matched, "status")); status != "satisfied" {
		t.Fatalf("cwi status = %q, want satisfied", status)
	}
	refs := mapStringSliceValue(matched["acceptedEvidenceByKind"])["validation"]
	if len(refs) != 1 || refs[0] != "prior-evidence-ref" {
		t.Fatalf("accepted.validation = %v, want [prior-evidence-ref]", refs)
	}
}

func TestReconcile_RunsOnEmptyRun_NoOp(t *testing.T) {
	run := &runmodel.Run{ID: "run-empty", ExecutionContext: map[string]any{}}
	c := newCoverageSubsystem()
	changed, bound := c.reconcileValidationEvidence(run, nil, nil)
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) on empty run", changed, bound)
	}
}

func TestReconcile_DoesNotBindCwiAlreadyWithValidation(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	priorCwi := findOriginalCwiByLeafSubstring(run, "go-build-verify")
	if priorCwi == nil {
		t.Fatalf("cwi with targetText containing go-build-verify not found")
	}
	priorCwi["acceptedEvidenceByKind"] = map[string]any{"validation": []string{"existing-ref"}}
	priorCwi["acceptedEvidenceRefs"] = []string{"existing-ref"}
	priorCwi["missingEvidenceKinds"] = []string{}

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-008", step.ID)

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — cwi already has validation ref", changed, bound)
	}
}

func TestReconcile_GraphLeafFuzzyMatchBinds(t *testing.T) {
	run := makeReconcileRun("- 基础 转发 通过 go-build-verify 完成 预期 回复", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-009", step.ID)
	validation.TargetRef = "go-build-verify"

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if !changed || bound != 1 {
		t.Fatalf("reconcile = (%v,%d), want (true,1) — go-build-verify graph leaf should fuzzy-match cwi targetText", changed, bound)
	}
}

func TestReconcile_StepKindNotBindable(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	step := &runmodel.Step{
		ID:     "run-recon-1/graph-007/001-read-main-go",
		RunID:  "run-recon-1",
		Kind:   "file/read",
		State:  common.StepStateCompleted,
		Output: map[string]any{"path": "main.go"},
	}
	validation := reconcileValidValidation("val-010", step.ID)

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — file/read must not bind validation", changed, bound)
	}
}

func TestReconcile_RejectsNonRuntimeStepValidationKind(t *testing.T) {
	run := makeReconcileRun("- go-build-verify 通过 编译 验证", nil)
	c := newCoverageSubsystem()
	c.ensure(run)

	step := reconcileStepShell("run-recon-1/graph-007/002-go-build-verify", "go")
	validation := reconcileValidValidation("val-011", step.ID)
	validation.ValidationKind = "go_test"

	changed, bound := c.reconcileValidationEvidence(run, []*runmodel.ValidationResult{validation}, []*runmodel.Step{step})
	if changed || bound != 0 {
		t.Fatalf("reconcile = (%v,%d), want (false,0) — non runtime_step validation must not auto-bind", changed, bound)
	}
}
