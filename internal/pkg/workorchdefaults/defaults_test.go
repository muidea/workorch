package workorchdefaults

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestEmbeddedSoftwareDevTemplatePresentationIsFrameworkNeutral(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("loadEmbeddedTemplates: %v", err)
	}
	item := findEmbeddedTemplateByID(templates, "software-dev")
	if item == nil {
		t.Fatal("software-dev template missing")
	}
	if item.Name != "软件开发" {
		t.Fatalf("software-dev template name = %q, want framework-neutral presentation", item.Name)
	}
	presentation := strings.ToLower(item.Name + "\n" + item.Description)
	for _, framework := range []string{"go 原生", "go-native", "golang", "node.js", "python", "rust"} {
		if strings.Contains(presentation, framework) {
			t.Fatalf("software-dev template presentation leaks validation adapter %q: name=%q description=%q", framework, item.Name, item.Description)
		}
	}
}

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
	assertContainsAnyDoDItem(t, item.DoDItems, "capability-owner receipt")
	assertContainsAnyDoDItem(t, item.DoDItems, "state_change")
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
	assertStringListContains(t, runtimeRepair["prerequisiteManifestTargetPatterns"].([]any), "go.sum")
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
	if policy["recipeAdapterRef"] != "shell/project-native-validation/v5" {
		t.Fatalf("software-dev evidence policy recipeAdapterRef = %v, want neutral Shell adapter", policy["recipeAdapterRef"])
	}
	if policy["recipeInputReviewProtocolRef"] != "validation/input-consistency/v2" {
		t.Fatal("software-dev template must explicitly seal the obligation-review protocol")
	}
	for _, forbidden := range []string{"commandName", "recipeArgs", "requiredArgs", "argsContain", "workEvidenceMarkers"} {
		if _, exists := policy[forbidden]; exists {
			t.Fatalf("software-dev evidence policy embeds adapter-specific %s: %+v", forbidden, policy)
		}
	}
	if declared, ok := policy["recipeDeclared"].(bool); !ok || !declared {
		t.Fatalf("software-dev evidence policy recipeDeclared = %v, want true", policy["recipeDeclared"])
	}
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

func TestValidationPreparationIsNotDeclaredAsAProfileCWI(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		contracts, _ := template.ContextHints["profileContracts"].(map[string]any)
		for profileID, raw := range contracts {
			profile, _ := raw.(map[string]any)
			encoded, err := json.Marshal(profile)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "validation_preparation") {
				t.Fatalf("template/profile %s/%s declares validation_preparation as an independent CWI", template.ID, profileID)
			}
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

func TestEmbeddedSoftwareDevSoulsUseCurrentRuntimeDocuments(t *testing.T) {
	souls, err := loadEmbeddedSouls()
	if err != nil {
		t.Fatalf("loadEmbeddedSouls: %v", err)
	}

	planning := findEmbeddedSoulByID(souls, "soul-software-dev-planning")
	execution := findEmbeddedSoulByID(souls, "soul-software-dev-execution")
	validation := findEmbeddedSoulByID(souls, "soul-software-dev-validation")
	artifact := findEmbeddedSoulByID(souls, "soul-software-dev-artifact")
	decision := findEmbeddedSoulByID(souls, "soul-software-dev-decision")
	for id, soul := range map[string]*embeddedSoul{
		"planning": planning, "execution": execution, "validation": validation,
		"artifact": artifact, "decision": decision,
	} {
		if soul == nil {
			t.Fatalf("software-dev %s soul missing", id)
		}
	}

	assertSoulContains(t, planning.SoulMD, "semantic_plan")
	assertSoulContains(t, planning.SoulMD, "DefinitionFocuses")
	assertSoulContains(t, planning.SoulMD, "每个 required CWI")
	assertSoulContains(t, planning.SoulMD, "sealed File owner catalog")
	assertSoulContains(t, planning.SoulMD, "具体 variant、参数形态和语法只由 File owner contract 声明")
	assertSoulNotContains(t, planning.SoulMD, "unified_diff")
	assertSoulNotContains(t, planning.SoulMD, "text_replace")
	assertSoulContains(t, planning.SoulMD, "file/write")
	assertSoulContains(t, planning.SoulMD, "bounded implementation slice")
	assertSoulContains(t, planning.SoulMD, "workspace 文件清单为空不等于缺少源码")
	assertSoulContains(t, planning.SoulMD, "package/module/import")
	assertSoulContains(t, planning.SoulMD, "真实交付入口")
	assertSoulContains(t, planning.SoulMD, "实现语言")
	assertSoulContains(t, planning.SoulMD, "不能因其他实现语言的 template 适配尚未提供而删减或延期")

	assertStringSliceContains(t, execution.AllowedToolKinds, "git/read")
	assertStringSliceContains(t, execution.AllowedToolKinds, "git/write")
	assertStringSliceContains(t, execution.AllowedToolKinds, "file/write")
	assertStringSliceContains(t, execution.AllowedToolKinds, "patch/apply")
	for _, kind := range execution.AllowedToolKinds {
		if kind == "file/edit" {
			t.Fatal("software-dev execution Soul still permits file/edit outside the declared patch/apply profile")
		}
	}
	assertSoulContains(t, execution.SoulMD, "operations")
	assertSoulContains(t, execution.SoulMD, "sealed manifest")
	assertSoulContains(t, execution.SoulMD, "inputAddition.contentContractRef")
	assertSoulContains(t, execution.SoulMD, "contentRequirements")
	assertSoulContains(t, execution.SoulMD, "不得把后端语言支持移入其他实现语言 template 的待办")
	assertSoulContains(t, execution.SoulMD, "旧协议不支持的路径不能仅凭 Soul 指令获得授权")
	assertSoulContains(t, execution.SoulMD, "reportedAssertion")
	assertSoulContains(t, execution.SoulMD, "comparisonHash")
	assertSoulContains(t, execution.SoulMD, "不从 hash 猜测正文")
	assertSoulContains(t, execution.SoulMD, "不为增加诊断而改写已有受保护的验证输入")
	assertSoulContains(t, execution.SoulMD, "bindingRefs")
	assertSoulContains(t, execution.SoulMD, "patch/apply")
	assertSoulContains(t, execution.SoulMD, "sealed File owner contract")
	assertSoulNotContains(t, execution.SoulMD, "unified_diff")
	assertSoulNotContains(t, execution.SoulMD, "text_replace")
	assertSoulContains(t, execution.SoulMD, "create-only `file/write`")
	assertSoulContains(t, execution.SoulMD, "owner snapshot")
	assertSoulContains(t, execution.SoulMD, "真实 root cause")
	assertSoulContains(t, execution.SoulMD, "保护 workspace 中已有有效内容")

	assertSoulContains(t, validation.SoulMD, "sealed binding")
	assertSoulContains(t, validation.SoulMD, "capability-owner receipt")
	assertSoulContains(t, validation.SoulMD, "每个 required CWI")
	assertSoulContains(t, validation.SoulMD, "exact recipe/check")
	assertSoulContains(t, validation.SoulMD, "version-control snapshot")
	assertSoulContains(t, validation.SoulMD, "claim-to-evidence")
	assertSoulContains(t, validation.SoulMD, "使用 Go 实现多语言服务不减少任何已封存语言或协议的验收项")
	assertSoulContains(t, validation.SoulMD, "skip 只能报告未验证")
	assertSoulContains(t, validation.SoulMD, "仍须核对真实协议响应、错误路径与逐项语义断言")

	assertSoulContains(t, artifact.SoulMD, "DoD 条目证据引用")
	assertSoulContains(t, artifact.SoulMD, "version_control_snapshot")
	assertSoulContains(t, artifact.SoulMD, "capability-owner receipt")
	assertSoulNotContains(t, artifact.SoulMD, "unified_diff")
	assertSoulNotContains(t, artifact.SoulMD, "text_replace")
	assertSoulContains(t, decision.SoulMD, "不要因为 workspace 为空就直接请求源码")

	for _, soul := range souls {
		for _, forbidden := range []string{
			"executionGraph",
			"nextStep",
			"nextAction",
			"executionContext",
			"repairActionPackage",
			"patchRequests",
			"shellCommands",
			"mcpToolCalls",
			"memoryWrites",
			"graph follow-up",
			"repair graph",
			"validation graph",
		} {
			assertSoulNotContains(t, soul.SoulMD, forbidden)
		}
	}
}
func TestBuiltinDefaultsArePureDeclarations(t *testing.T) {
	souls, err := BuiltinSoulDefaults()
	if err != nil || len(souls) == 0 {
		t.Fatalf("BuiltinSoulDefaults = %d, err=%v", len(souls), err)
	}
	var planning *workorchmanagementcontract.Soul
	for idx := range souls {
		if souls[idx].ID == "soul-software-dev-planning" {
			planning = &souls[idx]
			break
		}
	}
	if planning == nil {
		t.Fatal("planning soul declaration missing")
	}
	assertSoulContains(t, planning.SoulMD, "workspace 文件清单为空不等于缺少源码")
	templates, err := BuiltinWorkspaceTemplateDefaults()
	if err != nil || len(templates) == 0 {
		t.Fatalf("BuiltinWorkspaceTemplateDefaults = %d, err=%v", len(templates), err)
	}
	var template *workorchmanagementcontract.WorkspaceTemplate
	for idx := range templates {
		if templates[idx].ID == "software-dev" {
			template = &templates[idx]
			break
		}
	}
	if template == nil || template.SoulBindings["planning"] != "soul-software-dev-planning" || template.DefaultSoulID != "soul-software-dev-execution" {
		t.Fatalf("software-dev template declaration = %+v", template)
	}
	contextHints := template.ContextHints
	if contextHints["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("software-dev contextHints = %+v, want must_produce_state_change", template.ContextHints)
	}
	assertSoftwareDevWorkspaceDeliverableSelectors(t, contextHints)
	assertSoftwareDevAcceptanceSlicePolicy(t, contextHints)
	assertSoftwareDevAcceptanceDimensions(t, contextHints)
	assertSoftwareDevCapabilityUsagePolicies(t, contextHints)
	assertSoftwareDevValidationPolicies(t, contextHints)
	assertContainsAnyDoDItem(t, template.DoDItems, "evidenceRefs")
	servers, err := BuiltinMCPServerDefaults()
	if err != nil || len(servers) == 0 {
		t.Fatalf("BuiltinMCPServerDefaults = %d, err=%v", len(servers), err)
	}
	capabilities, err := BuiltinMCPCapabilityDefaults()
	if err != nil || len(capabilities) == 0 {
		t.Fatalf("BuiltinMCPCapabilityDefaults = %d, err=%v", len(capabilities), err)
	}
	foundLSPStatus := false
	for _, capability := range capabilities {
		if capability.ServerID == "mcp-lsp-bridge" && (capability.EffectScope == "" || capability.IdempotencyClass == "" || capability.ValidationProofApplicability == "") {
			t.Fatalf("embedded MCP capability %q missing runtime metadata", capability.Name)
		}
		if capability.ServerID == "mcp-lsp-bridge" && capability.Name == "lsp_status" {
			foundLSPStatus = true
			if capability.EffectScope != "none" || capability.IdempotencyClass != "safe_to_replay" {
				t.Fatalf("lsp_status runtime metadata = %q/%q, want none/safe_to_replay", capability.EffectScope, capability.IdempotencyClass)
			}
			if capability.ValidationProofApplicability != "auxiliary" {
				t.Fatalf("lsp_status proof applicability = %q, want auxiliary", capability.ValidationProofApplicability)
			}
		}
		if capability.ServerID == "mcp-lsp-bridge" && capability.Name == "lsp_definition" && capability.ValidationProofApplicability != "direct" {
			t.Fatalf("lsp_definition proof applicability = %q, want direct", capability.ValidationProofApplicability)
		}
	}
	if !foundLSPStatus {
		t.Fatal("embedded MCP defaults missing mcp-lsp-bridge lsp_status")
	}
}

// assertSoftwareDevWorkspaceDeliverableSelectors guards the startup-default
// path used by a new workspace. Generic direct-evidence selectors intentionally
// do not invent a document fallback; profile-owned bootstrap recipes declare
// bounded artifacts where a template can resolve one. File only verifies those
// declarations before a provider can propose a concrete operation.
func assertSoftwareDevWorkspaceDeliverableSelectors(t *testing.T, hints map[string]any) {
	t.Helper()
	contracts, ok := hints["profileContracts"].(map[string]any)
	if !ok {
		t.Fatalf("profileContracts = %T, want object", hints["profileContracts"])
	}
	profile, ok := contracts["software-dev"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev profile = %T, want object", contracts["software-dev"])
	}
	requirements, ok := profile["evidenceRequirements"].([]any)
	if !ok {
		t.Fatalf("software-dev evidenceRequirements = %T, want list", profile["evidenceRequirements"])
	}
	for _, id := range []string{"source_change", "*"} {
		found := 0
		for _, raw := range requirements {
			item, ok := raw.(map[string]any)
			requiresGrounding, grounded := item["requireTargetGrounding"].(bool)
			if !ok || item["id"] != id || !grounded || !requiresGrounding {
				continue
			}
			if item["targetSelectorRef"] != "file/workspace_deliverable" {
				t.Fatalf("%s targetSelectorRef = %q, want file/workspace_deliverable", id, item["targetSelectorRef"])
			}
			if id == "source_change" && (item["targetSelectorOwnerRef"] != "software-dev" || item["targetSelectorSubjectRef"] != "workspace_deliverable_create_scope") {
				t.Fatalf("%s empty-workspace scope binding = %q/%q, want software-dev/workspace_deliverable_create_scope", id, item["targetSelectorOwnerRef"], item["targetSelectorSubjectRef"])
			}
			if id == "*" && item["scope"] == "dodItem" && (item["targetSelectorOwnerRef"] != "software-dev" || item["targetSelectorSubjectRef"] != "dodItem") {
				t.Fatalf("%s DoD scope binding = %q/%q, want software-dev/dodItem", id, item["targetSelectorOwnerRef"], item["targetSelectorSubjectRef"])
			}
			if _, present := item["prospectiveTargetRef"]; present {
				t.Fatalf("%s/%s retains deprecated prospectiveTargetRef = %q", id, item["scope"], item["prospectiveTargetRef"])
			}
			constraints, ok := item["prospectiveTargetAllOfConstraintRefs"].([]any)
			if !ok || len(constraints) != 2 {
				t.Fatalf("%s/%s prospective target all-of declaration = %+v, want two mandatory constraints", id, item["scope"], item["prospectiveTargetAllOfConstraintRefs"])
			}
			assertStringListContains(t, constraints, "workspace-relative-path")
			assertStringListContains(t, constraints, "new-target-requires-owner-selector")
			found++
		}
		if found == 0 {
			t.Fatalf("missing grounded %s evidence requirement", id)
		}
	}
	if _, present := profile["bootstrapRecipes"]; present {
		t.Fatalf("software-dev must not embed workspace-specific bootstrap recipes: %+v", profile["bootstrapRecipes"])
	}
}

func findEmbeddedTemplateByID(items []workorchmanagementcontract.WorkspaceTemplate, id string) *workorchmanagementcontract.WorkspaceTemplate {
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
	if !ok || len(deliverables) != 3 {
		t.Fatalf("software-dev deliverableExpectations = %+v, want declared expectations", softwareDev["deliverableExpectations"])
	}
	requiredDeliverables := map[string]bool{
		"source_change":            false,
		"validation_report":        false,
		"version_control_snapshot": false,
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
	dependencies, ok := softwareDev["coverageDependencies"].([]any)
	if !ok || len(dependencies) == 0 {
		t.Fatalf("software-dev coverageDependencies = %+v, want declared source-to-validation dependency", softwareDev["coverageDependencies"])
	}
	wantDependencies := map[string]bool{
		"source-change-precedes-and-refreshes-dod-validation":     false,
		"dod-validation-precedes-and-refreshes-validation-report": false,
		"source-change-precedes-and-refreshes-final-snapshot":     false,
		"dod-validation-precedes-and-refreshes-final-snapshot":    false,
		"validation-report-precedes-final-snapshot":               false,
	}
	for _, raw := range dependencies {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("coverage dependency = %+v, want map", raw)
		}
		id, _ := item["id"].(string)
		if _, wanted := wantDependencies[id]; !wanted {
			continue
		}
		if item["requireSourceSettled"] != true {
			t.Fatalf("coverage dependency %q does not declare admission semantics: %+v", id, item)
		}
		if id != "validation-report-precedes-final-snapshot" {
			triggers, ok := item["triggerEvidenceKinds"].([]any)
			if !ok || len(triggers) != 1 || triggers[0] != "state_change" {
				t.Fatalf("coverage dependency %q triggers = %+v", id, item["triggerEvidenceKinds"])
			}
		}
		wantDependencies[id] = true
	}
	for id, found := range wantDependencies {
		if !found {
			t.Fatalf("software-dev coverageDependencies missing %s in %+v", id, dependencies)
		}
	}
	if _, exists := softwareDev["validationEnvironmentPolicy"]; exists {
		t.Fatal("software-dev retains an unconsumed Profile policy")
	}
	validationPolicies, ok := hints["validationPolicies"].([]any)
	if !ok || len(validationPolicies) != 1 {
		t.Fatal("software-dev requires one effective validation policy")
	}
	validationPolicy, ok := validationPolicies[0].(map[string]any)
	if !ok || validationPolicy["validationScratchRequired"] != true || validationPolicy["validationSafetyClass"] != "bounded" {
		t.Fatalf("effective validation policy lost scratch/bounded requirements: %+v", validationPolicy)
	}
	guidance, _ := json.Marshal(validationPolicy["notes"])
	for _, required := range []string{"isolated scratch", "bounded foreground", "do not use detached background", "must clean up"} {
		if !strings.Contains(string(guidance), required) {
			t.Fatalf("effective validation policy lost %q guidance", required)
		}
	}
	if strings.Contains(strings.ToLower(string(guidance)), "go scratch") || strings.Contains(string(guidance), "GOFLAGS") {
		t.Fatalf("software-dev validation guidance embeds Go adapter policy: %s", guidance)
	}
	requirements, ok := softwareDev["evidenceRequirements"].([]any)
	if !ok || len(requirements) < 4 {
		t.Fatalf("software-dev evidenceRequirements = %+v, want declared requirements", softwareDev["evidenceRequirements"])
	}
	if _, exists := softwareDev["stateChangingRepairGuidance"]; exists {
		t.Fatal("software-dev profile retains removed untyped state-changing guidance")
	}
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
	validationReportEvidenceFound := false
	validationReadinessPrerequisiteFound := false
	versionControlEvidenceFound := false
	directValidationTargetContracts := map[string]bool{
		"dodItem":      false,
		"goalCoverage": false,
	}
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
			if role, _ := item["targetRole"].(string); role != "workspace_delivery_change" {
				t.Fatalf("source_change targetRole = %q, want workspace_delivery_change", role)
			}
			if required, _ := item["requireTargetGrounding"].(bool); !required {
				t.Fatalf("source_change requireTargetGrounding = false, want true")
			}
			discovery, ok := item["targetDiscoverySources"].([]any)
			if !ok {
				t.Fatalf("source_change targetDiscoverySources = %+v, want list", item["targetDiscoverySources"])
			}
			assertStringListContains(t, discovery, "workspace_manifest")
			assertStringListContains(t, discovery, "project_files")
			if selector, _ := item["targetSelectorRef"].(string); selector != "file/workspace_deliverable" {
				t.Fatalf("source_change targetSelectorRef = %q, want file/workspace_deliverable", selector)
			}
			if _, present := item["prospectiveTargetRef"]; present {
				t.Fatalf("source_change retains deprecated prospectiveTargetRef = %q", item["prospectiveTargetRef"])
			}
			if direct, _ := item["requireDirectEvidence"].(bool); !direct {
				t.Fatalf("source_change requireDirectEvidence = false, want true")
			}
			if coverageTargets, _ := item["requireCoverageTargets"].(bool); !coverageTargets {
				t.Fatalf("source_change requireCoverageTargets = false, want true")
			}
			requiredKinds, ok := item["requiredEvidenceKinds"].([]any)
			if !ok || len(requiredKinds) != 1 || requiredKinds[0] != "state_change" {
				t.Fatalf("source_change requiredEvidenceKinds = %+v, want state_change", item["requiredEvidenceKinds"])
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
			assertStringListContains(t, accepted, "file/write")
			if variants, present := item["acceptedOperationVariants"]; present {
				t.Fatalf("source_change must leave concrete File variants to the sealed owner catalog: %+v", variants)
			}
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
			requiredKinds, ok := item["requiredEvidenceKinds"].([]any)
			if !ok || len(requiredKinds) != 1 || requiredKinds[0] != "state_change" {
				t.Fatalf("version_control_snapshot requiredEvidenceKinds = %+v, want state_change", item["requiredEvidenceKinds"])
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
			variants, ok := item["acceptedOperationVariants"].([]any)
			if !ok {
				t.Fatalf("version_control_snapshot acceptedOperationVariants = %+v, want list", item["acceptedOperationVariants"])
			}
			var variant map[string]any
			if len(variants) == 1 {
				variant, ok = variants[0].(map[string]any)
			}
			if !ok || variant["kind"] != "git/write" || variant["variantId"] != "commit" {
				t.Fatalf("version_control_snapshot acceptedOperationVariants = %+v, want git/write commit", variants)
			}
			capabilityStates, ok := item["capabilityStateRequirements"].([]any)
			if !ok || len(capabilityStates) != 1 {
				t.Fatalf("version_control_snapshot capabilityStateRequirements = %+v, want one canonical Git state requirement", item["capabilityStateRequirements"])
			}
			capabilityState, ok := capabilityStates[0].(map[string]any)
			if !ok || capabilityState["ownerModuleId"] != "blocks/git" || capabilityState["capabilityStateKind"] != "repository_state" || capabilityState["subjectRef"] != "workspace_deliverable" {
				t.Fatalf("version_control_snapshot capabilityStateRequirements = %+v, want blocks/git repository_state workspace_deliverable", capabilityStates)
			}
			assertStringListContains(t, rejected, "shell/command")
			assertStringListContains(t, rejected, "file/read")
			versionControlEvidenceFound = true
			continue
		}
		if id == "validation_report" {
			requiredKinds, ok := item["requiredEvidenceKinds"].([]any)
			if !ok || len(requiredKinds) != 1 || requiredKinds[0] != "validation" {
				t.Fatalf("validation_report requiredEvidenceKinds = %+v, want validation", item["requiredEvidenceKinds"])
			}
			accepted, ok := item["acceptedStepKinds"].([]any)
			if !ok {
				t.Fatalf("validation_report acceptedStepKinds = %+v, want list", item["acceptedStepKinds"])
			}
			assertStringListContains(t, accepted, "validation/managed_session")
			assertStringListNotContains(t, accepted, "shell/command")
			validationReportEvidenceFound = true
			continue
		}
		if id == "*" {
			scope, _ := item["scope"].(string)
			if _, expected := directValidationTargetContracts[scope]; expected {
				if role, _ := item["targetRole"].(string); role != "deliverable_implementation" {
					t.Fatalf("%s wildcard targetRole = %q, want deliverable_implementation", scope, role)
				}
				if required, _ := item["requireTargetGrounding"].(bool); !required {
					t.Fatalf("%s wildcard requireTargetGrounding = false, want true", scope)
				}
				discovery, ok := item["targetDiscoverySources"].([]any)
				if !ok {
					t.Fatalf("%s wildcard targetDiscoverySources = %+v, want list", scope, item["targetDiscoverySources"])
				}
				assertStringListContains(t, discovery, "workspace_manifest")
				assertStringListContains(t, discovery, "project_files")
				if selector, _ := item["targetSelectorRef"].(string); selector != "file/workspace_deliverable" {
					t.Fatalf("%s wildcard targetSelectorRef = %q, want file/workspace_deliverable", scope, selector)
				}
				if scope == "dodItem" && (item["targetSelectorOwnerRef"] != "software-dev" || item["targetSelectorSubjectRef"] != "dodItem") {
					t.Fatalf("dodItem declaration selector = %q/%q/%q, want file/workspace_deliverable/software-dev/dodItem", item["targetSelectorRef"], item["targetSelectorOwnerRef"], item["targetSelectorSubjectRef"])
				}
				if _, present := item["prospectiveTargetRef"]; present {
					t.Fatalf("%s wildcard retains deprecated prospectiveTargetRef = %q", scope, item["prospectiveTargetRef"])
				}
				accepted, ok := item["acceptedStepKinds"].([]any)
				if !ok {
					t.Fatalf("%s wildcard acceptedStepKinds = %+v, want list", scope, item["acceptedStepKinds"])
				}
				assertStringListContains(t, accepted, "file/write")
				assertStringListContains(t, accepted, "patch/apply")
				if variants, present := item["acceptedOperationVariants"]; present {
					t.Fatalf("%s wildcard must remain validation-only and cannot declare state-change variants: %+v", scope, variants)
				}
				if scope == "dodItem" {
					repair, ok := item["validationFailureRepairConvergence"].(map[string]any)
					if !ok {
						t.Fatalf("dodItem validationFailureRepairConvergence = %+v, want object", item["validationFailureRepairConvergence"])
					}
					if repair["requireFreshValidation"] != true || repair["maxAttempts"] != float64(2) {
						t.Fatalf("dodItem validation failure repair convergence = %+v", repair)
					}
					routes, ok := repair["routes"].([]any)
					if !ok || len(routes) != 5 {
						t.Fatalf("dodItem validation failure repair routes = %+v", repair["routes"])
					}
					routesByID := make(map[string]map[string]any, len(routes))
					for _, rawRoute := range routes {
						route, ok := rawRoute.(map[string]any)
						if !ok {
							t.Fatalf("dodItem validation failure repair route = %+v", rawRoute)
						}
						id, _ := route["id"].(string)
						routesByID[id] = route
					}
					addition := routesByID["missing-behavior-to-new-validation-input"]
					if addition["requireInputAddition"] != true || addition["repairTargetPolicy"] != "prospective_owner_target" || addition["repairTargetRole"] != "validation_input_supplement" || addition["triggerOwnerDiagnosticCode"] != "validation_workspace_behavior_unobserved" || addition["repairOperationTargetSelectorRef"] != "workspace-relative-new-file-target" {
						t.Fatalf("input addition is not independently create-only: %+v", addition)
					}
					kinds, _ := addition["repairOperationKinds"].([]any)
					if len(kinds) != 1 || kinds[0] != "file/write" {
						t.Fatal("input addition may overwrite existing tests")
					}
					inputRepair := routesByID["input-symbol-case-to-attested-patch"]
					repairKinds, _ := inputRepair["repairOperationKinds"].([]any)
					if inputRepair["requireInputRepair"] != true || inputRepair["requireInputAddition"] == true ||
						inputRepair["triggerEvidenceTargetType"] != "validation_input" || inputRepair["triggerOwnerDiagnosticCode"] != "validation_input_symbol_case_mismatch" ||
						inputRepair["repairTargetPolicy"] != "existing_owner_scope" || inputRepair["repairTargetRole"] != "validation_input_maintenance" ||
						len(repairKinds) != 1 || repairKinds[0] != "patch/apply" {
						t.Fatalf("input repair lost its independent exact-content gate: %+v", inputRepair)
					}
					expectedRoutes := map[string]struct {
						diagnosticClass string
						targetType      string
						ownerCode       string
					}{
						"module-manifest-parse-to-file-edit": {diagnosticClass: "validation_build_failed", targetType: "failure_surface", ownerCode: "module_manifest_parse_failed"},
						"source-compile-to-file-edit":        {diagnosticClass: "validation_build_failed", targetType: "failure_surface", ownerCode: "source_compile_failed"},
						"test-failure-to-file-edit":          {diagnosticClass: "validation_test_failed", targetType: "behavior_evidence", ownerCode: "test_failed"},
					}
					for routeID, expected := range expectedRoutes {
						route := routesByID[routeID]
						if route["triggerOwnerModuleId"] != "blocks/shell" || route["triggerOperationKind"] != "validation/managed_session" ||
							route["triggerDiagnosticClass"] != expected.diagnosticClass || route["triggerEvidenceTargetType"] != expected.targetType ||
							route["triggerOwnerDiagnosticCode"] != expected.ownerCode || route["repairEvidenceKind"] != "state_change" ||
							route["repairOwnerModuleId"] != "blocks/file" || route["repairTargetPolicy"] != "existing_owner_scope" ||
							route["repairTargetRole"] != "deliverable_implementation" || route["repairDeclarationTargetSelectorRef"] != "file/workspace_deliverable" ||
							route["repairDeclarationTargetSelectorOwnerRef"] != "software-dev" || route["repairDeclarationTargetSelectorSubjectRef"] != "dodItem_repair" ||
							route["repairOperationTargetSelectorRef"] != "workspace-relative-existing-file-target" {
							t.Fatalf("dodItem File repair route %q = %+v", routeID, route)
						}
						operationKinds := route["repairOperationKinds"].([]any)
						if len(operationKinds) != 1 {
							t.Fatalf("dodItem File repair route %q operations = %+v", routeID, operationKinds)
						}
						assertStringListContains(t, operationKinds, "file/edit")
					}
					ownerFacts, ok := item["requiredOwnerFacts"].([]any)
					if !ok || len(ownerFacts) != 1 {
						t.Fatalf("dodItem validation readiness owner facts = %+v, want one Shell postcondition", item["requiredOwnerFacts"])
					}
					ownerFact, ok := ownerFacts[0].(map[string]any)
					if !ok || ownerFact["ownerModuleId"] != "blocks/shell" || ownerFact["factKind"] != "selectable_validation_surface" || ownerFact["operationKind"] != "validation/managed_session" {
						t.Fatalf("dodItem validation readiness owner facts = %+v", ownerFacts)
					}
					convergences, ok := item["ownerFactConvergences"].([]any)
					if !ok || len(convergences) != 1 {
						t.Fatalf("dodItem validation readiness convergence = %+v, want one bounded acquisition", item["ownerFactConvergences"])
					}
					convergence, ok := convergences[0].(map[string]any)
					if !ok || convergence["missingTrigger"] != "owner_fact_missing" || convergence["acquisitionOwnerModuleId"] != "blocks/file" || convergence["acquisitionOperationKind"] != "file/write" || convergence["acquisitionTargetPolicy"] != "prospective_owner_target" || convergence["acquisitionTargetSelectorRef"] != "workspace-relative-new-file-target" || convergence["prospectiveSatisfactionContractRef"] != "shell/selectable-validation-surface/prospective-workspace-manifest/v1" || convergence["targetSelectorRef"] != item["targetSelectorRef"] || convergence["targetSelectorOwnerRef"] != item["targetSelectorOwnerRef"] || convergence["targetSelectorSubjectRef"] != item["targetSelectorSubjectRef"] || convergence["reobserveOwnerModuleId"] != "blocks/shell" || convergence["maxAcquisitionAttempts"] != float64(1) {
						t.Fatalf("dodItem validation readiness convergence = %+v, want File acquisition then Shell re-observation", convergences)
					}
					validationReadinessPrerequisiteFound = true
				}
				directValidationTargetContracts[scope] = true
			}
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
	if !validationReportEvidenceFound {
		t.Fatalf("software-dev evidenceRequirements missing validation_report in %+v", requirements)
	}
	if !validationReadinessPrerequisiteFound {
		t.Fatalf("software-dev dodItem proof contract is missing validation-readiness convergence in %+v", requirements)
	}
	for scope, found := range directValidationTargetContracts {
		if !found {
			t.Fatalf("software-dev evidenceRequirements missing grounded wildcard for %s in %+v", scope, requirements)
		}
	}
}

func assertSoftwareDevAcceptanceSlicePolicy(t *testing.T, hints map[string]any) {
	t.Helper()
	policy, ok := hints["acceptanceSlicePolicy"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev acceptanceSlicePolicy = %+v, want map", hints["acceptanceSlicePolicy"])
	}
	if policy["schemaVersion"] != "workorch.acceptance_slice_policy" {
		t.Fatalf("acceptanceSlicePolicy schemaVersion = %q", policy["schemaVersion"])
	}
	if got := testIntFromAny(policy["maxWorkItems"]); got != 4 {
		t.Fatalf("acceptanceSlicePolicy maxWorkItems = %d, want 4", got)
	}
	if got := testIntFromAny(policy["maxFocuses"]); got != 64 {
		t.Fatalf("acceptanceSlicePolicy maxFocuses = %d, want 64", got)
	}
	if got := testIntFromAny(policy["maxCandidateContextItems"]); got != 4 {
		t.Fatalf("acceptanceSlicePolicy maxCandidateContextItems = %d, want 4", got)
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
	wantMaxWorkItems := map[string]int{
		"slice-primary-state-change":     2,
		"slice-direct-validation":        4,
		"slice-version-control-snapshot": 2,
		"slice-supplemental-evidence":    4,
		"slice-remaining-acceptance":     4,
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
		if expected, ok := wantMaxWorkItems[id]; ok && testIntFromAny(group["maxWorkItems"]) != expected {
			t.Fatalf("acceptanceSlicePolicy group %q maxWorkItems = %d, want %d", id, testIntFromAny(group["maxWorkItems"]), expected)
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
	if !(priorities["slice-version-control-snapshot"] < priorities["slice-direct-validation"] &&
		priorities["slice-direct-validation"] < priorities["slice-supplemental-evidence"]) {
		t.Fatalf("acceptanceSlicePolicy priorities = %+v, want version snapshot before direct validation and generic supplemental", priorities)
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
		assertStringListContains(t, item["allowedExecutionModes"].([]any), "runtime_operations")
		assertStringListContains(t, item["allowedExecutionModes"].([]any), "managed_harness")
		assertStringListContains(t, item["allowedValidationModes"].([]any), "controlled_substitute")
		if got := testIntFromAny(item["maxValidationOnlyRetriesPerFocus"]); got != 2 {
			t.Fatalf("validation policy maxValidationOnlyRetriesPerFocus = %v, want 2", item["maxValidationOnlyRetriesPerFocus"])
		}
		for _, removed := range []string{"validationFailureImpliesStateChange", "implementationGapDiagnosticClasses", "preferMinimalPatchBeforeRepeatedValidation", "ifNoTargetRoute"} {
			if _, exists := item[removed]; exists {
				t.Fatalf("validation policy retains removed action-routing field %q: %+v", removed, item)
			}
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
	assertTemplateStageOutput(t, byID["planning"], "gap_baseline", "assessment")
	assertTemplateStageOutput(t, byID["execution"], "state_change", "state_change")
	assertTemplateStageOutput(t, byID["repair"], "state_change", "state_change")
	assertTemplateStageOutput(t, byID["validation"], "validation", "validation")
	assertTemplateStageOutput(t, byID["artifact"], "artifact", "artifact")
	assertTemplateStagePolicyRef(t, byID["repair"], "capabilityUsagePolicies", "software-dev-code-semantics-guidance")
	assertTemplateStagePolicyRef(t, byID["repair"], "evidencePolicies", "software-dev-validation-evidence")
	assertTemplateStagePolicyRef(t, byID["validation"], "evidencePolicies", "software-dev-validation-evidence")
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
	if policy["schemaVersion"] != "workorch.template_stage_policy" {
		t.Fatalf("%s templateStagePolicies schemaVersion = %v, want workorch.template_stage_policy", templateID, policy["schemaVersion"])
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
		if _, exists := stage["entry"]; exists {
			t.Fatalf("%s templateStagePolicies stage %q retains removed entry authority: %+v", templateID, id, stage["entry"])
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
