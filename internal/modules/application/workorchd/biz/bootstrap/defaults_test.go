package bootstrap

import (
	"context"
	"strings"
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestEmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("loadEmbeddedTemplates: %v", err)
	}
	item := findEmbeddedTemplateByID(templates, "software-dev")
	if item == nil {
		t.Fatal("software-dev template missing")
	}
	if len(item.DoDItems) < 6 {
		t.Fatalf("software-dev DoD items = %d, want >= 6", len(item.DoDItems))
	}

	assertContainsAnyDoDItem(t, item.DoDItems, "Goal")
	assertContainsAnyDoDItem(t, item.DoDItems, "PRD")
	assertContainsAnyDoDItem(t, item.DoDItems, "unified_diff")
	assertContainsAnyDoDItem(t, item.DoDItems, "build/test")
	assertContainsAnyDoDItem(t, item.DoDItems, "最终交付入口")
	assertContainsAnyDoDItem(t, item.DoDItems, "真实交付入口")
	assertContainsAnyDoDItem(t, item.DoDItems, "用户可观察行为")
	assertContainsAnyDoDItem(t, item.DoDItems, "fake/mock/helper")
	assertContainsAnyDoDItem(t, item.DoDItems, "主路径")
	assertContainsAnyDoDItem(t, item.DoDItems, "lint/静态检查")
	assertContainsAnyDoDItem(t, item.DoDItems, "残余风险")
	assertContainsAnyDoDItem(t, item.DoDItems, "evidenceRefs")
	if item.ContextHints["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("software-dev contextHints = %+v, want must_produce_state_change", item.ContextHints)
	}
	versionControl, ok := item.ContextHints["versionControl"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev versionControl context hint = %+v, want map", item.ContextHints["versionControl"])
	}
	gitConfig, ok := versionControl["git"].(map[string]any)
	if !ok || gitConfig["initializeOnCreate"] != true {
		t.Fatalf("software-dev git context hint = %+v, want initializeOnCreate=true", versionControl["git"])
	}
	assertSoftwareDevAcceptanceDimensions(t, item.ContextHints)
	if len(item.EvidencePolicies) == 0 {
		t.Fatalf("software-dev evidencePolicies missing")
	}
	policy := item.EvidencePolicies[0]
	if _, ok := policy["workEvidenceMarkers"]; !ok {
		t.Fatalf("software-dev evidence policy missing workEvidenceMarkers: %+v", policy)
	}
	markers := policy["workEvidenceMarkers"].([]any)
	assertStringListContains(t, markers, "=== RUN")
	assertStringListContains(t, markers, "--- PASS:")
}

func TestEmbeddedSoftwareDevSoulsContainPlanningExecutionValidationArtifactGuidance(t *testing.T) {
	souls, err := loadEmbeddedSouls()
	if err != nil {
		t.Fatalf("loadEmbeddedSouls: %v", err)
	}

	planning := findEmbeddedSoulByID(souls, "soul-software-dev-planning")
	if planning == nil {
		t.Fatal("soul-software-dev-planning missing")
	}
	assertSoulContains(t, planning.SoulMD, "技术栈")
	assertSoulContains(t, planning.SoulMD, "PRD")
	assertSoulContains(t, planning.SoulMD, "受影响文件")
	assertSoulContains(t, planning.SoulMD, "DoD 条目")
	assertSoulContains(t, planning.SoulMD, "DoD-to-test")
	assertSoulContains(t, planning.SoulMD, "主路径")
	assertSoulContains(t, planning.SoulMD, "新增 Goal")
	assertSoulContains(t, planning.SoulMD, "gap analysis")
	assertSoulContains(t, planning.SoulMD, "workspace 文件清单为空不等于缺少源码")
	assertSoulContains(t, planning.SoulMD, "新项目初始化场景")
	assertSoulContains(t, planning.SoulMD, "包/module/import")
	assertSoulContains(t, planning.SoulMD, "functional_behavior")
	assertSoulContains(t, planning.SoulMD, "external_contract")
	assertSoulContains(t, planning.SoulMD, "runtime_entrypoint")
	assertSoulContains(t, planning.SoulMD, "persistence_or_side_effect")
	assertSoulContains(t, planning.SoulMD, "target/interface contract matrix")
	assertSoulContains(t, planning.SoulMD, "输入契约")
	assertSoulContains(t, planning.SoulMD, "输出契约")
	assertSoulContains(t, planning.SoulMD, "file/read")
	assertSoulContains(t, planning.SoulMD, "rejectedResultContinuation")
	assertSoulContains(t, planning.SoulMD, "missingGoalCoverage")
	assertSoulContains(t, planning.SoulMD, "processEvidenceTimeline")
	assertSoulContains(t, planning.SoulMD, "currentProgressDigest")
	assertSoulContains(t, planning.SoulMD, "已有输出")
	assertSoulContains(t, planning.SoulMD, "当前缺口")
	assertSoulNotContains(t, planning.SoulMD, "provider/protocol matrix")
	assertSoulNotContains(t, planning.SoulMD, "多供应商")
	assertSoulNotContains(t, planning.SoulMD, "多模型")
	assertSoulNotContains(t, planning.SoulMD, "Token")

	execution := findEmbeddedSoulByID(souls, "soul-software-dev-execution")
	if execution == nil {
		t.Fatal("soul-software-dev-execution missing")
	}
	assertStringSliceContains(t, execution.AllowedToolKinds, "git/read")
	assertStringSliceContains(t, execution.AllowedToolKinds, "git/write")
	assertSoulContains(t, execution.SoulMD, "git/write init")
	assertSoulContains(t, execution.SoulMD, "最小代码或配置变更")
	assertSoulContains(t, execution.SoulMD, "workspace 为空且 Goal/DoD 未要求既有源码")
	assertSoulContains(t, execution.SoulMD, "每轮修改后先执行最小相关验证")
	assertSoulContains(t, execution.SoulMD, "失败命令")
	assertSoulContains(t, execution.SoulMD, "DoD 不满足")
	assertSoulContains(t, execution.SoulMD, "rootCause")
	assertSoulContains(t, execution.SoulMD, "错误路径")
	assertSoulContains(t, execution.SoulMD, "新增 Goal 处理")
	assertSoulContains(t, execution.SoulMD, "重复运行已有测试")
	assertSoulContains(t, execution.SoulMD, "不能替代实现")
	assertSoulContains(t, execution.SoulMD, "补丁质量要求")
	assertSoulContains(t, execution.SoulMD, "完整、可直接应用")
	assertSoulContains(t, execution.SoulMD, "占位函数体")
	assertSoulContains(t, execution.SoulMD, "当前文件快照")
	assertSoulContains(t, execution.SoulMD, "包/module/import")
	assertSoulContains(t, execution.SoulMD, "Goal / DoD、现有代码和验收证据")
	assertSoulContains(t, execution.SoulMD, "交付入口")
	assertSoulContains(t, execution.SoulMD, "多外部系统、多接口契约")
	assertSoulContains(t, execution.SoulMD, "file/read")
	assertSoulContains(t, execution.SoulMD, "git/read")
	assertSoulContains(t, execution.SoulMD, "git/write")
	assertSoulContains(t, execution.SoulMD, "编译与接口形状修复")
	assertSoulContains(t, execution.SoulMD, "undefined")
	assertSoulContains(t, execution.SoulMD, "unknown field")
	assertSoulContains(t, execution.SoulMD, "当前项目已经存在的类型")
	assertSoulContains(t, execution.SoulMD, "failureTargetRepairContext")
	assertSoulContains(t, execution.SoulMD, "repairActionPackage.currentFailure")
	assertSoulContains(t, execution.SoulMD, "failedPatchDigest")
	assertSoulContains(t, execution.SoulMD, "patch_current_snapshot_only")
	assertSoulContains(t, execution.SoulMD, "exact_replacement_against_current_snapshot")
	assertSoulContains(t, execution.SoulMD, "currentTargetAlreadySatisfiesFailedPatch")
	assertSoulContains(t, execution.SoulMD, "真实交付入口")
	assertSoulContains(t, execution.SoulMD, "测试文件内临时实现")
	assertSoulContains(t, execution.SoulMD, "processEvidenceTimeline")
	assertSoulContains(t, execution.SoulMD, "currentProgressDigest")
	assertSoulContains(t, execution.SoulMD, "沿用当前项目已有架构")
	assertSoulContains(t, execution.SoulMD, "保护当前 workspace 已有有效内容")
	assertSoulContains(t, execution.SoulMD, "不能长期只调整测试")
	assertSoulNotContains(t, execution.SoulMD, "多 provider/多 LLM 网关")
	assertSoulNotContains(t, execution.SoulMD, "providers[]")
	assertSoulNotContains(t, execution.SoulMD, "model_prefixes")
	assertSoulNotContains(t, execution.SoulMD, "真实外部 provider")
	assertSoulNotContains(t, execution.SoulMD, "provider 行为")

	validation := findEmbeddedSoulByID(souls, "soul-software-dev-validation")
	if validation == nil {
		t.Fatal("soul-software-dev-validation missing")
	}
	assertSoulContains(t, validation.SoulMD, "satisfied/failed/skipped")
	assertSoulContains(t, validation.SoulMD, "逐项")
	assertSoulContains(t, validation.SoulMD, "effective DoD")
	assertSoulContains(t, validation.SoulMD, "不得只选择")
	assertSoulContains(t, validation.SoulMD, "evidenceRefs")
	assertSoulContains(t, validation.SoulMD, "DoD-to-test")
	assertSoulContains(t, validation.SoulMD, "关键分支")
	assertSoulContains(t, validation.SoulMD, "Goal 叶子需求")
	assertSoulContains(t, validation.SoulMD, "验收维度")
	assertSoulContains(t, validation.SoulMD, "目标入口可用")
	assertSoulContains(t, validation.SoulMD, "入口路径")
	assertSoulContains(t, validation.SoulMD, "acceptanceDimensions")
	assertSoulContains(t, validation.SoulMD, "deliverableCoverage")
	assertSoulContains(t, validation.SoulMD, "claim-to-evidence consistency")
	assertSoulContains(t, validation.SoulMD, "file/read")
	assertSoulContains(t, validation.SoulMD, "persistence_or_side_effect")
	assertSoulContains(t, validation.SoulMD, "直接 evidenceRefs")
	assertSoulContains(t, validation.SoulMD, "runtime_entrypoint")
	assertSoulContains(t, validation.SoulMD, "external_contract")
	assertSoulContains(t, validation.SoulMD, "不能只引用 build/race/coverage_map")
	assertSoulContains(t, validation.SoulMD, "用户可观察行为")
	assertSoulContains(t, validation.SoulMD, "最终交付入口")
	assertSoulContains(t, validation.SoulMD, "fake/mock/helper")
	assertSoulContains(t, validation.SoulMD, "真实交付入口")
	assertSoulContains(t, validation.SoulMD, "生产实现")
	assertSoulContains(t, validation.SoulMD, "唯一被测对象")
	assertSoulContains(t, validation.SoulMD, "rejectedResultContinuation")
	assertSoulContains(t, validation.SoulMD, "missingDeliverableCoverage")
	assertSoulContains(t, validation.SoulMD, "processEvidenceTimeline")
	assertSoulContains(t, validation.SoulMD, "currentProgressDigest")
	assertSoulContains(t, validation.SoulMD, "不要让旧失败覆盖后续成功证据")
	assertSoulNotContains(t, validation.SoulMD, "多供应商")
	assertSoulNotContains(t, validation.SoulMD, "多模型")
	assertSoulNotContains(t, validation.SoulMD, "Token")

	artifact := findEmbeddedSoulByID(souls, "soul-software-dev-artifact")
	if artifact == nil {
		t.Fatal("soul-software-dev-artifact missing")
	}
	assertSoulContains(t, artifact.SoulMD, "DoD 条目证据引用")
	assertSoulContains(t, artifact.SoulMD, "变更文件")
	assertSoulContains(t, artifact.SoulMD, "行为变化")
	assertSoulContains(t, artifact.SoulMD, "coverage_map")
	assertSoulContains(t, artifact.SoulMD, "Goal 覆盖项")
	assertSoulContains(t, artifact.SoulMD, "构建产物")
	assertSoulContains(t, artifact.SoulMD, "不要只写")

	decision := findEmbeddedSoulByID(souls, "soul-software-dev-decision")
	if decision == nil {
		t.Fatal("soul-software-dev-decision missing")
	}
	assertSoulContains(t, decision.SoulMD, "不要因为 workspace 为空就直接请求源码")
}

func TestEnsureBuiltinDefaultsUpsertsEmbeddedSoulsAndTemplates(t *testing.T) {
	ctx := context.Background()
	soulRepo := memory.NewSoulRepository()
	templateRepo := memory.NewWorkspaceTemplateRepository()

	if err := soulRepo.Create(ctx, &soulmodel.Soul{
		ID:     common.SoulID("soul-software-dev-planning"),
		Name:   "stale planning soul",
		SoulMD: "# stale\n",
	}); err != nil {
		t.Fatalf("seed stale soul: %v", err)
	}
	if err := templateRepo.Create(ctx, &repository.WorkspaceTemplate{
		ID:       "software-dev",
		Name:     "stale template",
		DoDItems: []string{"stale dod"},
		SoulBindings: map[string]string{
			"planning": "stale-soul",
		},
	}); err != nil {
		t.Fatalf("seed stale template: %v", err)
	}

	if err := EnsureBuiltinDefaults(ctx, soulRepo, templateRepo); err != nil {
		t.Fatalf("EnsureBuiltinDefaults: %v", err)
	}

	planning, err := soulRepo.Get(ctx, common.SoulID("soul-software-dev-planning"))
	if err != nil {
		t.Fatalf("get planning soul: %v", err)
	}
	assertSoulContains(t, planning.SoulMD, "workspace 文件清单为空不等于缺少源码")
	if planning.Name == "stale planning soul" || strings.Contains(planning.SoulMD, "# stale") {
		t.Fatalf("planning soul was not refreshed: %+v", planning)
	}

	template, err := templateRepo.Get(ctx, "software-dev")
	if err != nil {
		t.Fatalf("get software-dev template: %v", err)
	}
	if template.Name == "stale template" || template.SoulBindings["planning"] != "soul-software-dev-planning" {
		t.Fatalf("software-dev template was not refreshed: %+v", template)
	}
	if template.ContextHints["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("software-dev contextHints = %+v, want must_produce_state_change", template.ContextHints)
	}
	assertSoftwareDevAcceptanceDimensions(t, template.ContextHints)
	assertContainsAnyDoDItem(t, template.DoDItems, "evidenceRefs")
	if len(template.EvidencePolicies) == 0 {
		t.Fatalf("software-dev evidence policies were not refreshed: %+v", template)
	}
}

func findEmbeddedTemplateByID(items []embeddedTemplate, id string) *embeddedTemplate {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func findEmbeddedSoulByID(items []embeddedSoul, id string) *embeddedSoul {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func assertContainsAnyDoDItem(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if strings.Contains(item, want) {
			return
		}
	}
	t.Fatalf("DoD items %q missing keyword %q", items, want)
}

func assertSoulContains(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("soul markdown missing %q", want)
	}
}

func assertSoulNotContains(t *testing.T, text string, forbidden string) {
	t.Helper()
	if strings.Contains(text, forbidden) {
		t.Fatalf("soul markdown contains forbidden business-specific text %q", forbidden)
	}
}

func assertSoftwareDevAcceptanceDimensions(t *testing.T, hints map[string]any) {
	t.Helper()
	contracts, ok := hints["profileContracts"].(map[string]any)
	if !ok {
		t.Fatalf("contextHints.profileContracts = %+v, want map", hints["profileContracts"])
	}
	softwareDev, ok := contracts["software-dev"].(map[string]any)
	if !ok {
		t.Fatalf("profileContracts[software-dev] = %+v, want map", contracts["software-dev"])
	}
	items, ok := softwareDev["acceptanceDimensions"].([]any)
	if !ok || len(items) < 9 {
		t.Fatalf("software-dev acceptanceDimensions = %+v, want declared dimensions", softwareDev["acceptanceDimensions"])
	}
	want := map[string]bool{
		"functional_behavior":        false,
		"external_contract":          false,
		"runtime_entrypoint":         false,
		"configuration":              false,
		"persistence_or_side_effect": false,
		"error_behavior":             false,
		"streaming_or_long_running":  false,
		"concurrency":                false,
		"deliverable":                false,
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("acceptance dimension item = %+v, want map", raw)
		}
		id, _ := item["id"].(string)
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Fatalf("software-dev acceptanceDimensions missing %q in %+v", id, items)
		}
	}
	deliverables, ok := softwareDev["deliverableExpectations"].([]any)
	if !ok || len(deliverables) < 4 {
		t.Fatalf("software-dev deliverableExpectations = %+v, want declared expectations", softwareDev["deliverableExpectations"])
	}
	requiredDeliverables := map[string]bool{
		"source_change":     false,
		"validation_report": false,
		"coverage_map":      false,
		"user_deliverable":  false,
	}
	for _, raw := range deliverables {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("deliverable expectation item = %+v, want map", raw)
		}
		id, _ := item["id"].(string)
		if _, ok := requiredDeliverables[id]; !ok {
			continue
		}
		if required, _ := item["required"].(bool); !required {
			t.Fatalf("software-dev deliverableExpectation %q required = false, want true", id)
		}
		requiredDeliverables[id] = true
	}
	for id, seen := range requiredDeliverables {
		if !seen {
			t.Fatalf("software-dev deliverableExpectations missing required %q in %+v", id, deliverables)
		}
	}
	requirements, ok := softwareDev["evidenceRequirements"].([]any)
	if !ok || len(requirements) < 4 {
		t.Fatalf("software-dev evidenceRequirements = %+v, want declared requirements", softwareDev["evidenceRequirements"])
	}
	requiredEvidence := map[string]bool{
		"runtime_entrypoint":         false,
		"external_contract":          false,
		"persistence_or_side_effect": false,
		"streaming_or_long_running":  false,
		"error_behavior":             false,
		"concurrency":                false,
	}
	for _, raw := range requirements {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("evidence requirement item = %+v, want map", raw)
		}
		id, _ := item["id"].(string)
		if _, ok := requiredEvidence[id]; !ok {
			continue
		}
		if scope, _ := item["scope"].(string); scope != "acceptanceDimension" {
			t.Fatalf("evidence requirement %q scope = %q, want acceptanceDimension", id, scope)
		}
		if direct, _ := item["requireDirectEvidence"].(bool); !direct {
			t.Fatalf("evidence requirement %q requireDirectEvidence = false, want true", id)
		}
		accepted, ok := item["acceptedStepKinds"].([]any)
		if !ok {
			t.Fatalf("evidence requirement %q acceptedStepKinds = %+v, want list", id, item["acceptedStepKinds"])
		}
		rejected, ok := item["rejectedStepKinds"].([]any)
		if !ok {
			t.Fatalf("evidence requirement %q rejectedStepKinds = %+v, want list", id, item["rejectedStepKinds"])
		}
		assertStringListContains(t, accepted, "shell/command")
		assertStringListContains(t, accepted, "patch/apply")
		assertStringListContains(t, accepted, "mcp/tool_call")
		assertStringListContains(t, rejected, "file/read")
		assertStringListContains(t, rejected, "file/list")
		assertStringListContains(t, rejected, "mcp/resource_read")
		assertStringListContains(t, rejected, "mcp/prompt_get")
		requiredEvidence[id] = true
	}
	for id, seen := range requiredEvidence {
		if !seen {
			t.Fatalf("software-dev evidenceRequirements missing %q in %+v", id, requirements)
		}
	}
}

func assertStringListContains(t *testing.T, values []any, want string) {
	t.Helper()
	for _, raw := range values {
		if value, _ := raw.(string); value == want {
			return
		}
	}
	t.Fatalf("list %+v missing %q", values, want)
}

func assertStringSliceContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("list %+v missing %q", values, want)
}
