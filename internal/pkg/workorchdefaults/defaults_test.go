package workorchdefaults

import (
	"context"
	"encoding/json"
	"os"
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
	assertContainsAnyDoDItem(t, item.DoDItems, "git/write")
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
	runtimeRepair, ok := item.ContextHints["runtimeRepair"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev runtimeRepair context hint = %+v, want map", item.ContextHints["runtimeRepair"])
	}
	assertStringListContains(t, runtimeRepair["prerequisiteManifestTargetPatterns"].([]any), "go.mod")
	assertStringListContains(t, runtimeRepair["prerequisiteManifestTargetPatterns"].([]any), "package.json")
	versionControl, ok := item.ContextHints["versionControl"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev versionControl context hint = %+v, want map", item.ContextHints["versionControl"])
	}
	gitConfig, ok := versionControl["git"].(map[string]any)
	if !ok || gitConfig["initializeOnCreate"] != true {
		t.Fatalf("software-dev git context hint = %+v, want initializeOnCreate=true", versionControl["git"])
	}
	if gitConfig["snapshotPolicy"] != "commit_meaningful_changes" || gitConfig["snapshotEvidence"] != "local_commit_for_workspace_output_changes" {
		t.Fatalf("software-dev git snapshot hint = %+v, want commit_meaningful_changes policy", gitConfig)
	}
	semanticNavigation, ok := item.ContextHints["semanticCodeNavigation"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev semanticCodeNavigation hint = %+v, want map", item.ContextHints["semanticCodeNavigation"])
	}
	if semanticNavigation["evidenceStrength"] != "auxiliary" {
		t.Fatalf("software-dev semanticCodeNavigation = %+v, want auxiliary evidence", semanticNavigation)
	}
	assertStringListContains(t, semanticNavigation["defaultMCPServerIds"].([]any), "mcp-lsp-bridge")
	assertStringListContains(t, semanticNavigation["defaultCapabilityIds"].([]any), "mcp-lsp-bridge:tool:lsp_diagnostics")
	assertSoftwareDevAcceptanceSlicePolicy(t, item.ContextHints)
	assertSoftwareDevAcceptanceDimensions(t, item.ContextHints)
	assertSoftwareDevCapabilityUsagePolicies(t, item.ContextHints)
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

func TestEmbeddedTemplatesDeclareDefaultSoulIDs(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("loadEmbeddedTemplates: %v", err)
	}
	for _, item := range templates {
		if strings.TrimSpace(item.DefaultSoulID) == "" {
			t.Fatalf("%s defaultSoulId is empty", item.ID)
		}
		executionSoul := item.SoulBindings["execution"]
		if item.DefaultSoulID != executionSoul {
			t.Fatalf("%s defaultSoulId = %q, want execution binding %q", item.ID, item.DefaultSoulID, executionSoul)
		}
	}
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
	assertSoulContains(t, planning.SoulMD, "completion follow-up")
	assertSoulContains(t, planning.SoulMD, "可执行 repair graph")
	assertSoulContains(t, planning.SoulMD, "coverageTargets")
	assertSoulContains(t, planning.SoulMD, "repairActionPackage.targetSnapshots")
	assertSoulContains(t, planning.SoulMD, "placeholder 测试")
	assertSoulContains(t, planning.SoulMD, "git/write add")
	assertSoulContains(t, planning.SoulMD, "version_control_snapshot 引用了 shell/command")
	assertSoulContains(t, planning.SoulMD, "zeroWork 评估")
	assertSoulContains(t, planning.SoulMD, "runtimeCapabilities.blockKinds 未声明 git/write")
	assertSoulContains(t, planning.SoulMD, "runtime capability contract 缺失核心 git/write 能力")
	assertSoulNotContains(t, planning.SoulMD, "runtime.git.writeEnabled")
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
	assertSoulContains(t, execution.SoulMD, "版本控制快照")
	assertSoulContains(t, execution.SoulMD, "git/write commit")
	assertSoulContains(t, execution.SoulMD, "编译与接口形状修复")
	assertSoulContains(t, execution.SoulMD, "undefined")
	assertSoulContains(t, execution.SoulMD, "unknown field")
	assertSoulContains(t, execution.SoulMD, "当前项目已经存在的类型")
	assertSoulContains(t, execution.SoulMD, "failureTargetRepairContext")
	assertSoulContains(t, execution.SoulMD, "repairActionPackage.currentFailure")
	assertSoulContains(t, execution.SoulMD, "failedPatchDigest")
	assertSoulContains(t, execution.SoulMD, "patch_current_snapshot_only")
	assertSoulContains(t, execution.SoulMD, "exact_replacement_against_current_snapshot")
	assertSoulContains(t, execution.SoulMD, "baseHash mismatch")
	assertSoulContains(t, execution.SoulMD, "invalid_unified_diff")
	assertSoulContains(t, execution.SoulMD, "file_write cannot replace existing target")
	assertSoulContains(t, execution.SoulMD, "currentTargetAlreadySatisfiesFailedPatch")
	assertSoulContains(t, execution.SoulMD, "同一个 executionGraph")
	assertSoulContains(t, execution.SoulMD, "strategyReassessment.required=true")
	assertSoulContains(t, execution.SoulMD, "failedSurface、rootCauseTarget、repairTargetDecision")
	assertSoulContains(t, execution.SoulMD, "oldText/newText 必须是实质变化")
	assertSoulContains(t, execution.SoulMD, "LSP 使用约束")
	assertSoulContains(t, execution.SoulMD, "首次使用 LSP 前必须先执行 lsp_initialize")
	assertSoulContains(t, execution.SoulMD, "lsp_sync")
	assertSoulContains(t, execution.SoulMD, "lsp_diagnostics")
	assertSoulContains(t, execution.SoulMD, "不能替代对真实生产入口")
	assertSoulContains(t, execution.SoulMD, "真实交付入口")
	assertSoulContains(t, execution.SoulMD, "测试文件内临时实现")
	assertSoulContains(t, execution.SoulMD, "不得绕过生产实现")
	assertSoulContains(t, execution.SoulMD, "processEvidenceTimeline")
	assertSoulContains(t, execution.SoulMD, "currentProgressDigest")
	assertSoulContains(t, execution.SoulMD, "沿用当前项目已有架构")
	assertSoulContains(t, execution.SoulMD, "保护当前 workspace 已有有效内容")
	assertSoulContains(t, execution.SoulMD, "不能长期只调整测试")
	assertSoulContains(t, execution.SoulMD, "missing type")
	assertSoulContains(t, execution.SoulMD, "首要编译阻塞")
	assertSoulContains(t, execution.SoulMD, "bounded repair graph")
	assertSoulContains(t, execution.SoulMD, "failed dodAssessment")
	assertSoulContains(t, execution.SoulMD, "mustDeclareCoverageTargets")
	assertSoulContains(t, execution.SoulMD, "repairActionPackage.coverageTargetSet")
	assertSoulContains(t, execution.SoulMD, "补充证据")
	assertSoulContains(t, execution.SoulMD, "生产实现、入口或配置目标")
	assertSoulContains(t, execution.SoulMD, "placeholder 测试")
	assertSoulContains(t, execution.SoulMD, "version_control_snapshot 证据类型不符")
	assertSoulContains(t, execution.SoulMD, "必须直接生成 git/write add 和 git/write commit")
	assertSoulContains(t, execution.SoulMD, "非 git/write commit 证据")
	assertSoulContains(t, execution.SoulMD, "context.scope=runtime_capability_contract")
	assertSoulContains(t, execution.SoulMD, "context.requiresExternalEvidence=false")
	assertSoulContains(t, execution.SoulMD, "runtime capability contract 缺陷")
	assertSoulNotContains(t, execution.SoulMD, "缺失能力作为配置阻塞")
	assertSoulNotContains(t, execution.SoulMD, "runtime.git.writeEnabled")
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
	assertSoulContains(t, validation.SoulMD, "placeholder 测试")
	assertSoulContains(t, validation.SoulMD, "生产实现、入口或用户可观察行为")
	assertSoulContains(t, validation.SoulMD, "version_control_snapshot")
	assertSoulContains(t, validation.SoulMD, "提交 hash")
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
	assertSoulContains(t, validation.SoulMD, "bounded repair graph")
	assertSoulContains(t, validation.SoulMD, "目标文件、目标测试和验证命令")
	assertSoulContains(t, validation.SoulMD, "deliverableCoverage.source_change")
	assertSoulContains(t, validation.SoulMD, "git/write commit")
	assertSoulContains(t, validation.SoulMD, "coverageTargets 缺失")
	assertSoulContains(t, validation.SoulMD, "version_control_snapshot 因引用 shell/command")
	assertSoulContains(t, validation.SoulMD, "git/write add 和 git/write commit")
	assertSoulContains(t, validation.SoulMD, "runtimeCapabilities.blockKinds 未声明 git/write")
	assertSoulContains(t, validation.SoulMD, "runtime capability contract 缺失核心 git/write 能力")
	assertSoulNotContains(t, validation.SoulMD, "runtime.git.writeEnabled")
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
	assertSoulContains(t, artifact.SoulMD, "version_control_snapshot")
	assertSoulContains(t, artifact.SoulMD, "commit hash")
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
	mcpServerRepo := memory.NewMCPServerRepository()
	mcpCapabilityRepo := memory.NewMCPCapabilityRepository()

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

	if err := EnsureBuiltinDefaults(ctx, soulRepo, templateRepo, mcpServerRepo, mcpCapabilityRepo); err != nil {
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
	if template.DefaultSoulID != "soul-software-dev-execution" {
		t.Fatalf("software-dev defaultSoulId = %q, want soul-software-dev-execution", template.DefaultSoulID)
	}
	if template.ContextHints["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("software-dev contextHints = %+v, want must_produce_state_change", template.ContextHints)
	}
	assertSoftwareDevAcceptanceSlicePolicy(t, template.ContextHints)
	assertSoftwareDevAcceptanceDimensions(t, template.ContextHints)
	assertSoftwareDevCapabilityUsagePolicies(t, template.ContextHints)
	assertContainsAnyDoDItem(t, template.DoDItems, "evidenceRefs")
	if len(template.EvidencePolicies) == 0 {
		t.Fatalf("software-dev evidence policies were not refreshed: %+v", template)
	}
	server, err := mcpServerRepo.Get(ctx, common.MCPServerID("mcp-lsp-bridge"))
	if err != nil {
		t.Fatalf("get mcp-lsp-bridge server: %v", err)
	}
	if server.Transport != "stdio" || server.Command != "lsp-bridge" || server.SessionPolicy != "long_lived" {
		t.Fatalf("mcp-lsp-bridge server = %+v, want stdio command long_lived", server)
	}
	if server.Env["LSP_BRIDGE_CONFIG"] != "{workspaceManagedConfig:lsp-bridge/mcp-config.json}" {
		t.Fatalf("mcp-lsp-bridge env = %+v, want managed config placeholder", server.Env)
	}
	capability, err := mcpCapabilityRepo.Get(ctx, common.MCPCapabilityID("mcp-lsp-bridge:tool:lsp_diagnostics"))
	if err != nil {
		t.Fatalf("get lsp diagnostics capability: %v", err)
	}
	if capability.Category != "code_semantics" || capability.RiskLevel != "read_only" {
		t.Fatalf("lsp diagnostics capability = %+v, want code semantics read-only", capability)
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

func TestEmbeddedSoulMarkdownRefsAreTemplateScoped(t *testing.T) {
	data, err := os.ReadFile("defaults/souls.json")
	if err != nil {
		t.Fatalf("read souls.json: %v", err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal souls.json: %v", err)
	}
	for _, item := range raw {
		id := strings.TrimSpace(testStringFromAny(item["id"]))
		if id == "" {
			t.Fatalf("soul item missing id: %+v", item)
		}
		if _, ok := item["soulMD"]; ok {
			t.Fatalf("%s still has inline soulMD", id)
		}
		ref := strings.TrimSpace(testStringFromAny(item["soulMDRef"]))
		if err := validateEmbeddedSoulMDRef(ref); err != nil {
			t.Fatalf("%s soulMDRef %q invalid: %v", id, ref, err)
		}
		wantRef := expectedSoulMDRefFromID(id)
		if ref != wantRef {
			t.Fatalf("%s soulMDRef = %q, want %q", id, ref, wantRef)
		}
	}

	souls, err := loadEmbeddedSouls()
	if err != nil {
		t.Fatalf("loadEmbeddedSouls: %v", err)
	}
	for _, soul := range souls {
		if strings.TrimSpace(soul.SoulMD) == "" {
			t.Fatalf("%s loaded empty SoulMD", soul.ID)
		}
	}
}

func TestValidateEmbeddedSoulMDRefRejectsInvalidRefs(t *testing.T) {
	for _, ref := range []string{
		"",
		"/souls/software-dev/execution.md",
		"../souls/software-dev/execution.md",
		"soul-software-dev-execution.md",
		"souls/software-dev-execution.md",
		"souls/software-dev/execution.txt",
		"souls/software-dev/../execution.md",
		"souls/software-dev//execution.md",
	} {
		if err := validateEmbeddedSoulMDRef(ref); err == nil {
			t.Fatalf("validateEmbeddedSoulMDRef(%q) = nil, want error", ref)
		}
	}
	if err := validateEmbeddedSoulMDRef("souls/software-dev/execution.md"); err != nil {
		t.Fatalf("validateEmbeddedSoulMDRef(valid): %v", err)
	}
}

func expectedSoulMDRefFromID(id string) string {
	const prefix = "soul-"
	trimmed := strings.TrimPrefix(id, prefix)
	for _, stage := range []string{"planning", "execution", "decision", "approval", "validation", "artifact"} {
		suffix := "-" + stage
		if strings.HasSuffix(trimmed, suffix) {
			template := strings.TrimSuffix(trimmed, suffix)
			return "souls/" + template + "/" + stage + ".md"
		}
	}
	return ""
}

func testStringFromAny(value any) string {
	typed, _ := value.(string)
	return typed
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
	if !ok || len(items) < 10 {
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
		"quantitative_behavior":      false,
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
		"source_change":            false,
		"validation_report":        false,
		"coverage_map":             false,
		"version_control_snapshot": false,
		"user_deliverable":         false,
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
		role, _ := item["evidenceRole"].(string)
		if id == "source_change" && role != "primary_state_change" {
			t.Fatalf("software-dev deliverableExpectation %q evidenceRole = %q, want primary_state_change", id, role)
		}
		if id != "source_change" && role != "supplemental_evidence" {
			t.Fatalf("software-dev deliverableExpectation %q evidenceRole = %q, want supplemental_evidence", id, role)
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
	guidance, ok := softwareDev["stateChangingRepairGuidance"].([]any)
	if !ok || len(guidance) < 4 {
		t.Fatalf("software-dev stateChangingRepairGuidance = %+v, want declared guidance", softwareDev["stateChangingRepairGuidance"])
	}
	assertStringListContains(t, guidance, "When repairActionPackage.requiredOutput accepts patch/apply, return at least one patch/apply node that performs a concrete source, configuration, test, documentation, or workspace state change directly tied to the current failure.")
	assertStringListContains(t, guidance, "When repairActionPackage.requiredOutput.mustDeclareCoverageTargets is true, every patch/apply or equivalent state-changing evidence node must include coverageTargets copied from the active coverageTargetSet item it intends to close.")
	assertStringListContains(t, guidance, "When targetSnapshots, failureTargetRepairContext, or repairTargetSet already identify existing implementation or entrypoint files for an unresolved Goal/DoD/source_change gap, patch those targets first; do not spend the primary state-changing turn on placeholder tests, go.sum, coverage_map, report-only files, or other supplemental evidence resources unless the current diagnostics explicitly target them.")
	assertStringListContains(t, guidance, "When repairActionPackage.shapeContractEvidenceRequired is present, the next graph must first acquire bounded evidence for the actual API, dependency, schema, constructor, method, field, or capability contract shape via file/read, shell/command, mcp/tool_call, or git/read; do not guess SDK or dependency shapes from diagnostics alone.")
	assertStringListContains(t, guidance, "If version_control_snapshot was rejected because it cited shell/command, file/read, file/list, LSP, or other non git/write commit evidence, the next repair must converge on git/read status/diff followed by git/write add and git/write commit, or return a zeroWork/skipped assessment when no commit-worthy change exists.")
	requiredEvidence := map[string]bool{
		"runtime_entrypoint":         false,
		"external_contract":          false,
		"persistence_or_side_effect": false,
		"streaming_or_long_running":  false,
		"error_behavior":             false,
		"concurrency":                false,
		"quantitative_behavior":      false,
	}
	sourceChangeEvidenceFound := false
	versionControlEvidenceFound := false
	for _, raw := range requirements {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("evidence requirement item = %+v, want map", raw)
		}
		id, _ := item["id"].(string)
		if id == "source_change" {
			if scope, _ := item["scope"].(string); scope != "deliverable" {
				t.Fatalf("source_change evidence scope = %q, want deliverable", scope)
			}
			if role, _ := item["evidenceRole"].(string); role != "primary_state_change" {
				t.Fatalf("source_change evidenceRole = %q, want primary_state_change", role)
			}
			if direct, _ := item["requireDirectEvidence"].(bool); !direct {
				t.Fatalf("source_change requireDirectEvidence = false, want true")
			}
			if coverageTargets, _ := item["requireCoverageTargets"].(bool); !coverageTargets {
				t.Fatalf("source_change requireCoverageTargets = false, want true")
			}
			accepted, ok := item["acceptedStepKinds"].([]any)
			if !ok {
				t.Fatalf("source_change acceptedStepKinds = %+v, want list", item["acceptedStepKinds"])
			}
			rejected, ok := item["rejectedStepKinds"].([]any)
			if !ok {
				t.Fatalf("source_change rejectedStepKinds = %+v, want list", item["rejectedStepKinds"])
			}
			assertStringListContains(t, accepted, "patch/apply")
			assertStringListContains(t, rejected, "file/read")
			assertStringListContains(t, rejected, "file/list")
			assertStringListContains(t, rejected, "mcp/resource_read")
			assertStringListContains(t, rejected, "mcp/prompt_get")
			sourceChangeEvidenceFound = true
			continue
		}
		if id == "version_control_snapshot" {
			if scope, _ := item["scope"].(string); scope != "deliverable" {
				t.Fatalf("version_control_snapshot evidence scope = %q, want deliverable", scope)
			}
			if role, _ := item["evidenceRole"].(string); role != "supplemental_evidence" {
				t.Fatalf("version_control_snapshot evidenceRole = %q, want supplemental_evidence", role)
			}
			accepted, ok := item["acceptedStepKinds"].([]any)
			if !ok {
				t.Fatalf("version_control_snapshot acceptedStepKinds = %+v, want list", item["acceptedStepKinds"])
			}
			rejected, ok := item["rejectedStepKinds"].([]any)
			if !ok {
				t.Fatalf("version_control_snapshot rejectedStepKinds = %+v, want list", item["rejectedStepKinds"])
			}
			assertStringListContains(t, accepted, "git/write")
			operations, ok := item["acceptedOperations"].([]any)
			if !ok {
				t.Fatalf("version_control_snapshot acceptedOperations = %+v, want list", item["acceptedOperations"])
			}
			assertStringListContains(t, operations, "commit")
			assertStringListContains(t, rejected, "shell/command")
			assertStringListContains(t, rejected, "file/read")
			versionControlEvidenceFound = true
			continue
		}
		if _, ok := requiredEvidence[id]; !ok {
			continue
		}
		if scope, _ := item["scope"].(string); scope != "acceptanceDimension" {
			t.Fatalf("evidence requirement %q scope = %q, want acceptanceDimension", id, scope)
		}
		if direct, _ := item["requireDirectEvidence"].(bool); !direct {
			t.Fatalf("evidence requirement %q requireDirectEvidence = false, want true", id)
		}
		if executable, _ := item["requireExecutableBehaviorEvidence"].(bool); !executable {
			t.Fatalf("evidence requirement %q requireExecutableBehaviorEvidence = false, want true", id)
		}
		if id == "quantitative_behavior" {
			if claim, _ := item["requireClaimEvidence"].(bool); !claim {
				t.Fatalf("evidence requirement %q requireClaimEvidence = false, want true", id)
			}
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
		assertStringListNotContains(t, accepted, "patch/apply")
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
	if !versionControlEvidenceFound {
		t.Fatalf("software-dev evidenceRequirements missing version_control_snapshot in %+v", requirements)
	}
	if !sourceChangeEvidenceFound {
		t.Fatalf("software-dev evidenceRequirements missing source_change in %+v", requirements)
	}
}

func assertSoftwareDevAcceptanceSlicePolicy(t *testing.T, hints map[string]any) {
	t.Helper()
	policy, ok := hints["acceptanceSlicePolicy"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev acceptanceSlicePolicy = %+v, want map", hints["acceptanceSlicePolicy"])
	}
	if policy["schemaVersion"] != "workorch.acceptance_slice_policy.v1" {
		t.Fatalf("acceptanceSlicePolicy schemaVersion = %q", policy["schemaVersion"])
	}
	groups, ok := policy["groups"].([]any)
	if !ok || len(groups) < 4 {
		t.Fatalf("acceptanceSlicePolicy groups = %+v, want declared groups", policy["groups"])
	}
	want := map[string]bool{
		"slice-primary-state-change":  false,
		"slice-direct-validation":     false,
		"slice-supplemental-evidence": false,
		"slice-remaining-acceptance":  false,
	}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("acceptanceSlicePolicy group = %+v, want map", raw)
		}
		id := testStringFromAny(group["id"])
		if _, ok := want[id]; ok {
			want[id] = true
		}
		match, ok := group["match"].(map[string]any)
		if !ok {
			t.Fatalf("acceptanceSlicePolicy group %q match = %+v, want map", id, group["match"])
		}
		switch id {
		case "slice-primary-state-change":
			assertStringListContains(t, match["evidenceRoles"].([]any), "primary_state_change")
		case "slice-direct-validation":
			assertStringListContains(t, match["missingEvidenceKinds"].([]any), "validation")
		case "slice-supplemental-evidence":
			assertStringListContains(t, match["evidenceRoles"].([]any), "supplemental_evidence")
		case "slice-remaining-acceptance":
			if match["all"] != true {
				t.Fatalf("remaining acceptance match = %+v, want all=true", match)
			}
		}
	}
	for id, seen := range want {
		if !seen {
			t.Fatalf("acceptanceSlicePolicy missing group %q in %+v", id, groups)
		}
	}
}

func assertSoftwareDevCapabilityUsagePolicies(t *testing.T, hints map[string]any) {
	t.Helper()
	rawPolicies, ok := hints["capabilityUsagePolicies"].([]any)
	if !ok || len(rawPolicies) == 0 {
		t.Fatalf("contextHints.capabilityUsagePolicies = %+v, want non-empty policies", hints["capabilityUsagePolicies"])
	}
	var found bool
	for _, raw := range rawPolicies {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("capability usage policy item = %+v, want map", raw)
		}
		if item["id"] == "software-dev-code-semantics-guidance" {
			found = true
			if item["appliesToCategory"] != "code_semantics" {
				t.Fatalf("capability usage policy = %+v, want code_semantics category", item)
			}
			if item["usageMode"] != "prefer_when_triggered" {
				t.Fatalf("capability usage policy = %+v, want prefer_when_triggered usageMode", item)
			}
			preferred, ok := item["preferredTools"].([]any)
			if !ok {
				t.Fatalf("capability usage policy preferredTools = %+v, want list", item["preferredTools"])
			}
			assertStringListContains(t, preferred, "lsp_sync")
			assertStringListContains(t, preferred, "lsp_diagnostics")
		}
	}
	if !found {
		t.Fatalf("capabilityUsagePolicies = %+v, want software-dev-code-semantics-guidance", rawPolicies)
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

func assertStringListNotContains(t *testing.T, values []any, forbidden string) {
	t.Helper()
	for _, raw := range values {
		if value, _ := raw.(string); value == forbidden {
			t.Fatalf("list %+v unexpectedly contains %q", values, forbidden)
		}
	}
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
