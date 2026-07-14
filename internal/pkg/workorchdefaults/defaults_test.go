package workorchdefaults

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
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
	if semanticNavigation["provider"] != "mcp" {
		t.Fatalf("software-dev semanticCodeNavigation = %+v, want mcp provider", semanticNavigation)
	}
	if semanticNavigation["optional"] != true {
		t.Fatalf("software-dev semanticCodeNavigation = %+v, want optional capability", semanticNavigation)
	}
	if semanticNavigation["evidenceStrength"] != "auxiliary" {
		t.Fatalf("software-dev semanticCodeNavigation = %+v, want auxiliary evidence", semanticNavigation)
	}
	notes, ok := semanticNavigation["notes"].([]any)
	if !ok || len(notes) == 0 {
		t.Fatalf("software-dev semanticCodeNavigation notes = %+v, want non-empty notes", semanticNavigation["notes"])
	}
	assertStringListContains(t, notes, "semantic navigation is optional and must not replace build/test/lint or direct DoD evidence")
	assertStringListContains(t, semanticNavigation["defaultMCPServerIds"].([]any), "mcp-lsp-bridge")
	defaultCapabilities := semanticNavigation["defaultCapabilityIds"].([]any)
	for _, want := range []string{
		"mcp-lsp-bridge:tool:lsp_initialize",
		"mcp-lsp-bridge:tool:lsp_sync",
		"mcp-lsp-bridge:tool:lsp_definition",
		"mcp-lsp-bridge:tool:lsp_hover",
		"mcp-lsp-bridge:tool:lsp_references",
		"mcp-lsp-bridge:tool:lsp_diagnostics",
		"mcp-lsp-bridge:tool:lsp_status",
		"mcp-lsp-bridge:tool:lsp_shutdown",
		"mcp-lsp-bridge:tool:lsp_repair",
	} {
		assertStringListContains(t, defaultCapabilities, want)
	}
	capabilityCatalog, err := loadEmbeddedMCPCapabilities()
	if err != nil {
		t.Fatalf("loadEmbeddedMCPCapabilities: %v", err)
	}
	capabilityIDs := map[string]struct{}{}
	for _, capability := range capabilityCatalog {
		capabilityIDs[capability.ID] = struct{}{}
	}
	for _, raw := range defaultCapabilities {
		id, _ := raw.(string)
		if _, ok := capabilityIDs[id]; !ok {
			t.Fatalf("software-dev semanticCodeNavigation capability %q missing from embedded MCP capability catalog", id)
		}
	}
	assertSoftwareDevAcceptanceSlicePolicy(t, item.ContextHints)
	assertSoftwareDevTemplateStagePolicies(t, item.ContextHints)
	assertSoftwareDevAcceptanceDimensions(t, item.ContextHints)
	assertSoftwareDevCapabilityUsagePolicies(t, item.ContextHints)
	assertSoftwareDevValidationPolicies(t, item.ContextHints)
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
	requirements, ok := policy["coverageRequirements"].([]any)
	if !ok || len(requirements) < 2 {
		t.Fatalf("software-dev evidence policy coverageRequirements = %+v, want coverage bindings", policy["coverageRequirements"])
	}
	requiredScopes := map[string]bool{"dod_item": false, "goal_coverage": false}
	for _, raw := range requirements {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("software-dev evidence policy coverage requirement = %+v, want map", raw)
		}
		scope, _ := item["scope"].(string)
		if _, ok := requiredScopes[scope]; !ok {
			continue
		}
		requiredScopes[scope] = true
		if id, _ := item["id"].(string); id != "*" {
			t.Fatalf("software-dev coverage requirement scope %q id = %q, want wildcard", scope, id)
		}
		kinds, ok := item["requiredEvidenceKinds"].([]any)
		if !ok {
			t.Fatalf("software-dev coverage requirement %q requiredEvidenceKinds = %+v, want list", scope, item["requiredEvidenceKinds"])
		}
		assertStringListContains(t, kinds, "validation")
		if direct, _ := item["requireDirectEvidence"].(bool); !direct {
			t.Fatalf("software-dev coverage requirement %q requireDirectEvidence = false, want true", scope)
		}
		if executable, _ := item["requireExecutableBehaviorEvidence"].(bool); !executable {
			t.Fatalf("software-dev coverage requirement %q requireExecutableBehaviorEvidence = false, want true", scope)
		}
	}
	for scope, seen := range requiredScopes {
		if !seen {
			t.Fatalf("software-dev evidence policy coverageRequirements missing scope %q in %+v", scope, requirements)
		}
	}
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

func TestEmbeddedTemplateStagePolicySchemaWhenDeclared(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("loadEmbeddedTemplates: %v", err)
	}
	var declared int
	for _, item := range templates {
		policy, ok := item.ContextHints["templateStagePolicies"].(map[string]any)
		if !ok {
			continue
		}
		declared++
		assertTemplateStagePolicySchema(t, item.ID, policy)
	}
	if declared == 0 {
		t.Fatal("no embedded template declares templateStagePolicies")
	}
}

func TestEmbeddedNonSoftwareTemplatesDoNotDeclareSoftwareDevStrategy(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("loadEmbeddedTemplates: %v", err)
	}
	softwareKeys := []string{
		"stateChangePolicy",
		"runtimeRepair",
		"versionControl",
		"semanticCodeNavigation",
		"managedWorkspaceConfigs",
		"acceptanceSlicePolicy",
		"capabilityUsagePolicies",
	}
	var checked int
	for _, item := range templates {
		if item.ID == "software-dev" {
			continue
		}
		checked++
		for _, key := range softwareKeys {
			if _, ok := item.ContextHints[key]; ok {
				t.Fatalf("%s contextHints unexpectedly declared software-dev strategy key %q: %+v", item.ID, key, item.ContextHints[key])
			}
		}
		if policies := item.EvidencePolicies; len(policies) != 0 {
			t.Fatalf("%s evidencePolicies = %+v, want no software-dev evidence policy", item.ID, policies)
		}
		if contracts, ok := item.ContextHints["profileContracts"].(map[string]any); ok {
			if _, ok := contracts["software-dev"]; ok {
				t.Fatalf("%s profileContracts unexpectedly include software-dev: %+v", item.ID, contracts)
			}
		}
		policy, ok := item.ContextHints["templateStagePolicies"].(map[string]any)
		if !ok {
			continue
		}
		stages, _ := policy["stages"].([]any)
		for _, raw := range stages {
			stage, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("%s templateStagePolicies stage = %+v, want map", item.ID, raw)
			}
			policyRefs, ok := stage["policyRefs"].(map[string]any)
			if !ok {
				continue
			}
			for _, refType := range []string{"evidencePolicies", "capabilityUsagePolicies"} {
				refs, _ := policyRefs[refType].([]any)
				for _, rawRef := range refs {
					ref, _ := rawRef.(string)
					if strings.Contains(ref, "software-dev") {
						t.Fatalf("%s stage %q policyRefs.%s includes software-dev ref %q", item.ID, testStringFromAny(stage["id"]), refType, ref)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-software embedded templates checked")
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
	assertSoulContains(t, planning.SoulMD, "bounded implementation slice")
	assertSoulContains(t, planning.SoulMD, "单文件职责")
	assertSoulContains(t, planning.SoulMD, "catch-all helper")
	assertSoulContains(t, planning.SoulMD, "mega file")
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
	assertSoulContains(t, execution.SoulMD, "bounded implementation slice")
	assertSoulContains(t, execution.SoulMD, "large generated file")
	assertSoulContains(t, execution.SoulMD, "monolithic implementation")
	assertSoulContains(t, execution.SoulMD, "catch-all utility")
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
	assertSoulContains(t, validation.SoulMD, "large generated file")
	assertSoulContains(t, validation.SoulMD, "bounded implementation slice")
	assertSoulContains(t, validation.SoulMD, "目标文件、验证命令")
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
	contextHints := template.ContextHints.Map()
	if contextHints["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("software-dev contextHints = %+v, want must_produce_state_change", template.ContextHints)
	}
	assertSoftwareDevAcceptanceSlicePolicy(t, contextHints)
	assertSoftwareDevAcceptanceDimensions(t, contextHints)
	assertSoftwareDevCapabilityUsagePolicies(t, contextHints)
	assertSoftwareDevValidationPolicies(t, contextHints)
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
	assertStringSliceContains(t, server.AllowedTools, "lsp_diagnostics")
	assertStringSliceContains(t, server.AllowedTools, "lsp_status")
	assertStringSliceContains(t, server.AllowedTools, "lsp_shutdown")
	assertStringSliceContains(t, server.AllowedTools, "lsp_repair")
	capability, err := mcpCapabilityRepo.Get(ctx, common.MCPCapabilityID("mcp-lsp-bridge:tool:lsp_diagnostics"))
	if err != nil {
		t.Fatalf("get lsp diagnostics capability: %v", err)
	}
	if capability.Category != "code_semantics" || capability.RiskLevel != "read_only" {
		t.Fatalf("lsp diagnostics capability = %+v, want code semantics read-only", capability)
	}
	if len(capability.Schema.Fields) == 0 {
		t.Fatalf("lsp diagnostics capability schema is empty")
	}
	initializeCapability, err := mcpCapabilityRepo.Get(ctx, common.MCPCapabilityID("mcp-lsp-bridge:tool:lsp_initialize"))
	if err != nil {
		t.Fatalf("get lsp initialize capability: %v", err)
	}
	required, ok := initializeCapability.Schema.StringSlice("required")
	if !ok {
		t.Fatalf("lsp initialize capability required schema = %+v, want string list", initializeCapability.Schema)
	}
	assertStringSliceContains(t, required, "lang_id")
	statusCapability, err := mcpCapabilityRepo.Get(ctx, common.MCPCapabilityID("mcp-lsp-bridge:tool:lsp_status"))
	if err != nil {
		t.Fatalf("get lsp status capability: %v", err)
	}
	if statusCapability.Category != "code_semantics" || statusCapability.RiskLevel != "read_only" {
		t.Fatalf("lsp status capability = %+v, want code semantics read-only", statusCapability)
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

func testIntFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
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
	validationPolicy, ok := softwareDev["validationEnvironmentPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev validationEnvironmentPolicy = %+v, want map", softwareDev["validationEnvironmentPolicy"])
	}
	if validationPolicy["scratchIsolationRequired"] != true || validationPolicy["boundedForegroundOnly"] != true {
		t.Fatalf("validationEnvironmentPolicy = %+v, want scratch isolation and bounded foreground", validationPolicy)
	}
	languageProfiles, ok := validationPolicy["languageProfiles"].(map[string]any)
	if !ok {
		t.Fatalf("validationEnvironmentPolicy.languageProfiles = %+v, want map", validationPolicy["languageProfiles"])
	}
	goProfile, ok := languageProfiles["go"].(map[string]any)
	if !ok {
		t.Fatalf("validationEnvironmentPolicy.languageProfiles.go = %+v, want map", languageProfiles["go"])
	}
	env, ok := goProfile["scratchBuildEnv"].(map[string]any)
	if !ok || env["GOFLAGS"] != "-buildvcs=false" {
		t.Fatalf("go scratchBuildEnv = %+v, want GOFLAGS=-buildvcs=false", goProfile["scratchBuildEnv"])
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
		"slice-primary-state-change":     false,
		"slice-direct-validation":        false,
		"slice-version-control-snapshot": false,
		"slice-supplemental-evidence":    false,
		"slice-remaining-acceptance":     false,
	}
	priorities := map[string]int{}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("acceptanceSlicePolicy group = %+v, want map", raw)
		}
		id := testStringFromAny(group["id"])
		if _, ok := want[id]; ok {
			want[id] = true
		}
		priorities[id] = testIntFromAny(group["priority"])
		match, ok := group["match"].(map[string]any)
		if !ok {
			t.Fatalf("acceptanceSlicePolicy group %q match = %+v, want map", id, group["match"])
		}
		switch id {
		case "slice-primary-state-change":
			assertStringListContains(t, match["evidenceRoles"].([]any), "primary_state_change")
		case "slice-direct-validation":
			assertStringListContains(t, match["missingEvidenceKinds"].([]any), "validation")
		case "slice-version-control-snapshot":
			assertStringListContains(t, match["sourceRequirementIds"].([]any), "version_control_snapshot")
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
	if !(priorities["slice-direct-validation"] < priorities["slice-version-control-snapshot"] &&
		priorities["slice-version-control-snapshot"] < priorities["slice-supplemental-evidence"]) {
		t.Fatalf("acceptanceSlicePolicy priorities = %+v, want version snapshot after validation and before generic supplemental", priorities)
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
			assertStringListContains(t, preferred, "lsp_status")
			assertStringListContains(t, preferred, "lsp_shutdown")
			assertStringListContains(t, preferred, "lsp_repair")
		}
	}
	if !found {
		t.Fatalf("capabilityUsagePolicies = %+v, want software-dev-code-semantics-guidance", rawPolicies)
	}
}

func assertSoftwareDevValidationPolicies(t *testing.T, hints map[string]any) {
	t.Helper()
	rawPolicies, ok := hints["validationPolicies"].([]any)
	if !ok || len(rawPolicies) == 0 {
		t.Fatalf("contextHints.validationPolicies = %+v, want non-empty policies", hints["validationPolicies"])
	}
	var found bool
	for _, raw := range rawPolicies {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("validation policy item = %+v, want map", raw)
		}
		if item["id"] != "software-dev-validation-discovery-first" {
			continue
		}
		found = true
		if item["discoveryRequired"] != true || item["validationScratchRequired"] != true {
			t.Fatalf("validation policy = %+v, want discovery and scratch requirements", item)
		}
		assertStringListContains(t, item["appliesToEvidenceKinds"].([]any), "validation")
		assertStringListContains(t, item["allowedExecutionModes"].([]any), "llm_graph")
		assertStringListContains(t, item["allowedExecutionModes"].([]any), "managed_harness")
		assertStringListContains(t, item["allowedValidationModes"].([]any), "controlled_substitute")
		if item["validationFailureImpliesStateChange"] != true || item["preferMinimalPatchBeforeRepeatedValidation"] != true {
			t.Fatalf("validation policy = %+v, want state-change failure policy and minimal patch preference", item)
		}
		if got := testIntFromAny(item["maxValidationOnlyRetriesPerFocus"]); got != 2 {
			t.Fatalf("validation policy maxValidationOnlyRetriesPerFocus = %v, want 2", item["maxValidationOnlyRetriesPerFocus"])
		}
		assertStringListContains(t, item["implementationGapDiagnosticClasses"].([]any), "implementation_gap_detected")
		if item["ifNoTargetRoute"] != "failed_assessment_with_missing_evidence_or_deferred" {
			t.Fatalf("validation policy ifNoTargetRoute = %v", item["ifNoTargetRoute"])
		}
	}
	if !found {
		t.Fatalf("validationPolicies = %+v, want software-dev-validation-discovery-first", rawPolicies)
	}
}

func assertSoftwareDevTemplateStagePolicies(t *testing.T, hints map[string]any) {
	t.Helper()
	rawPolicy, ok := hints["templateStagePolicies"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev templateStagePolicies = %+v, want map", hints["templateStagePolicies"])
	}
	assertTemplateStagePolicySchema(t, "software-dev", rawPolicy)
	stages, ok := rawPolicy["stages"].([]any)
	if !ok {
		t.Fatalf("software-dev templateStagePolicies.stages = %+v, want list", rawPolicy["stages"])
	}
	byID := map[string]map[string]any{}
	for _, raw := range stages {
		stage, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("software-dev stage = %+v, want map", raw)
		}
		byID[testStringFromAny(stage["id"])] = stage
	}
	for _, id := range []string{"planning", "execution", "repair", "validation", "artifact"} {
		if byID[id] == nil {
			t.Fatalf("software-dev templateStagePolicies missing stage %q in %+v", id, stages)
		}
	}
	assertTemplateStageNextAction(t, byID["planning"], "assess_gap")
	assertTemplateStageNextAction(t, byID["execution"], "state_change")
	assertTemplateStageNextAction(t, byID["repair"], "repair")
	assertTemplateStageNextAction(t, byID["validation"], "validate")
	assertTemplateStageNextAction(t, byID["artifact"], "produce_artifact")
	assertTemplateStageOutput(t, byID["planning"], "gap_baseline", "assessment")
	assertTemplateStageOutput(t, byID["execution"], "state_change", "state_change")
	assertTemplateStageOutput(t, byID["repair"], "state_change", "state_change")
	assertTemplateStageOutput(t, byID["validation"], "validation", "validation")
	assertTemplateStageOutput(t, byID["artifact"], "artifact", "artifact")
	assertTemplateStagePolicyRef(t, byID["repair"], "capabilityUsagePolicies", "software-dev-code-semantics-guidance")
	assertTemplateStagePolicyRef(t, byID["repair"], "evidencePolicies", "software-dev-validation-evidence")
	assertTemplateStagePolicyRef(t, byID["validation"], "evidencePolicies", "software-dev-validation-evidence")
}

func assertTemplateStageNextAction(t *testing.T, stage map[string]any, want string) {
	t.Helper()
	entry, ok := stage["entry"].(map[string]any)
	if !ok {
		t.Fatalf("stage entry = %+v, want map", stage["entry"])
	}
	if got := testStringFromAny(entry["nextAction"]); got != want {
		t.Fatalf("stage %q nextAction = %q, want %q", testStringFromAny(stage["id"]), got, want)
	}
}

func assertTemplateStageOutput(t *testing.T, stage map[string]any, wantKind string, wantEvidenceKind string) {
	t.Helper()
	outputs, ok := stage["requiredOutputs"].([]any)
	if !ok {
		t.Fatalf("stage %q requiredOutputs = %+v, want list", testStringFromAny(stage["id"]), stage["requiredOutputs"])
	}
	for _, raw := range outputs {
		output, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("stage output = %+v, want map", raw)
		}
		if testStringFromAny(output["kind"]) != wantKind {
			continue
		}
		kinds, ok := output["evidenceKinds"].([]any)
		if !ok {
			t.Fatalf("stage output evidenceKinds = %+v, want list", output["evidenceKinds"])
		}
		assertStringListContains(t, kinds, wantEvidenceKind)
		return
	}
	t.Fatalf("stage %q missing required output %q in %+v", testStringFromAny(stage["id"]), wantKind, outputs)
}

func assertTemplateStagePolicyRef(t *testing.T, stage map[string]any, refType string, want string) {
	t.Helper()
	policyRefs, ok := stage["policyRefs"].(map[string]any)
	if !ok {
		t.Fatalf("stage %q policyRefs = %+v, want map", testStringFromAny(stage["id"]), stage["policyRefs"])
	}
	values, ok := policyRefs[refType].([]any)
	if !ok {
		t.Fatalf("stage %q policyRefs.%s = %+v, want list", testStringFromAny(stage["id"]), refType, policyRefs[refType])
	}
	assertStringListContains(t, values, want)
}

func assertTemplateStagePolicySchema(t *testing.T, templateID string, policy map[string]any) {
	t.Helper()
	if policy["schemaVersion"] != "workorch.template_stage_policy.v1" {
		t.Fatalf("%s templateStagePolicies schemaVersion = %v, want workorch.template_stage_policy.v1", templateID, policy["schemaVersion"])
	}
	stages, ok := policy["stages"].([]any)
	if !ok || len(stages) == 0 {
		t.Fatalf("%s templateStagePolicies.stages = %+v, want non-empty list", templateID, policy["stages"])
	}
	seen := map[string]bool{}
	for _, raw := range stages {
		stage, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s templateStagePolicies stage = %+v, want map", templateID, raw)
		}
		id := strings.TrimSpace(testStringFromAny(stage["id"]))
		if id == "" {
			t.Fatalf("%s templateStagePolicies stage has empty id: %+v", templateID, stage)
		}
		if seen[id] {
			t.Fatalf("%s templateStagePolicies duplicate stage id %q", templateID, id)
		}
		seen[id] = true
		if binding := strings.TrimSpace(testStringFromAny(stage["soulBinding"])); binding != "" {
			if !isKnownSoulBindingStage(binding) {
				t.Fatalf("%s templateStagePolicies stage %q soulBinding = %q, want binding key", templateID, id, binding)
			}
		}
		entry, ok := stage["entry"].(map[string]any)
		if !ok {
			t.Fatalf("%s templateStagePolicies stage %q entry = %+v, want map", templateID, id, stage["entry"])
		}
		if strings.TrimSpace(testStringFromAny(entry["nextAction"])) == "" {
			t.Fatalf("%s templateStagePolicies stage %q entry.nextAction is empty", templateID, id)
		}
		outputs, ok := stage["requiredOutputs"].([]any)
		if !ok || len(outputs) == 0 {
			t.Fatalf("%s templateStagePolicies stage %q requiredOutputs = %+v, want non-empty list", templateID, id, stage["requiredOutputs"])
		}
		for _, rawOutput := range outputs {
			output, ok := rawOutput.(map[string]any)
			if !ok {
				t.Fatalf("%s templateStagePolicies stage %q output = %+v, want map", templateID, id, rawOutput)
			}
			if strings.TrimSpace(testStringFromAny(output["kind"])) == "" {
				t.Fatalf("%s templateStagePolicies stage %q output has empty kind", templateID, id)
			}
			evidenceKinds, ok := output["evidenceKinds"].([]any)
			if !ok || len(evidenceKinds) == 0 {
				t.Fatalf("%s templateStagePolicies stage %q output evidenceKinds = %+v, want non-empty list", templateID, id, output["evidenceKinds"])
			}
		}
		completion, ok := stage["completion"].(map[string]any)
		if !ok || strings.TrimSpace(testStringFromAny(completion["policy"])) == "" {
			t.Fatalf("%s templateStagePolicies stage %q completion = %+v, want policy", templateID, id, stage["completion"])
		}
	}
}

func isKnownSoulBindingStage(value string) bool {
	switch value {
	case "planning", "execution", "decision", "approval", "validation", "artifact":
		return true
	default:
		return false
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
