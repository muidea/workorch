package workorch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWebConsoleMutatingEndpointsStayWithinProductSurfaceContract(t *testing.T) {
	data := readWebConsoleFile(t, "src/api.ts")
	lines := strings.Split(string(data), "\n")
	allowed := []string{
		`${apiBase}/configuration/llm`,
		`${apiBase}/configuration/server`,
		`${apiBase}/configuration/web-search`,
		`${apiBase}/workspaces`, // create workspace only.
		`${apiBase}/workspace-setup-draft-sessions`,
		`/workspace-setup-draft-sessions/${encodeURIComponent(sessionId)}/messages`,
		`/workspace-setup-draft-sessions/${encodeURIComponent(sessionId)}/regenerate`,
		`/workspaces/${encodeURIComponent(workspaceId)}${suffix}`, // delete workspace only.
		`/workspaces/${encodeURIComponent(workspaceId)}/soul-bindings`,
		`/workspaces/${encodeURIComponent(workspaceId)}/goals`,
		`/workspaces/${encodeURIComponent(workspaceId)}/goals/${encodeURIComponent(goalId)}`,
		`/workspaces/${encodeURIComponent(workspaceId)}/dods`,
		`/workspaces/${encodeURIComponent(workspaceId)}/dods/${encodeURIComponent(dodId)}`,
		`/workspaces/${encodeURIComponent(workspaceId)}/goal-dod-quality-check`,
		`/workspaces/${encodeURIComponent(workspaceId)}/execution-activations`,
		`${apiBase}/souls`,
		`${apiBase}/souls/${encodeURIComponent(soulId)}`,
		`${apiBase}/workspace-templates`,
		`${apiBase}/workspace-templates/${encodeURIComponent(templateId)}`,
		`${apiBase}/mcp/servers`,
		`${apiBase}/mcp/servers/${encodeURIComponent(serverId)}`,
		`${apiBase}/mcp/capabilities`,
		`${apiBase}/mcp/capabilities/${encodeURIComponent(capabilityId)}`,
		`${apiBase}/decisions/${encodeURIComponent(ticketId)}/resolve`,
		`${apiBase}/decisions/resolve-batch`,
		`${apiBase}/approvals/${encodeURIComponent(approvalId)}/approve`,
		`${apiBase}/approvals/${encodeURIComponent(approvalId)}/reject`,
		`${apiBase}/approvals/resolve-batch`,
	}

	mutatingCalls := 0
	for i, line := range lines {
		method := mutatingMethod(line)
		if method == "" {
			continue
		}
		mutatingCalls++
		start := i - 3
		if start < 0 {
			start = 0
		}
		end := i + 2
		if end > len(lines) {
			end = len(lines)
		}
		snippet := strings.Join(lines[start:end], "\n")
		if !containsAny(snippet, allowed) {
			t.Fatalf("web console mutating endpoint at api.ts:%d is outside the product surface write contract:\n%s", i+1, snippet)
		}
	}
	if mutatingCalls < 20 {
		t.Fatalf("mutatingCalls = %d, want a non-vacuous web API contract scan", mutatingCalls)
	}
}

func TestWebWorkspaceExecutionCommandSurfaceMatchesRunIntentContract(t *testing.T) {
	typesData := string(readWebConsoleFile(t, "src/types.ts"))
	activationType := webSourceSection(t, typesData, "export type WorkspaceExecutionActivationRequest = {", "\n};")
	for _, want := range []string{
		"intent: WorkspaceExecutionIntent;",
		"llmTarget?: ProviderModelTarget;",
		"reasoningEffort?: string;",
		"soulBindings?: Record<string, string>;",
		"priority?: number;",
	} {
		if !strings.Contains(activationType, want) {
			t.Fatalf("WorkspaceExecutionActivationRequest missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"runId", "goalId", "doDId", "skillIds", "mcpServerIds", "capabilityIds", "memoryIds", "definitionRevision", "decisionId",
	} {
		if strings.Contains(activationType, forbidden) {
			t.Fatalf("WorkspaceExecutionActivationRequest retains non-owner activation field %q", forbidden)
		}
	}
	if !strings.Contains(typesData, "runtimeActivation?: RuntimeActivationStatus;") {
		t.Fatal("Run projection must retain read-only RuntimeActivationStatus")
	}

	apiData := string(readWebConsoleFile(t, "src/api.ts"))
	if !strings.Contains(apiData, "/snapshot?view=detail") {
		t.Fatal("readWorkspaceSnapshotProjection must request explicit detail view")
	}
	for _, want := range []string{
		"submitWorkspaceExecution: async (workspaceId: string, requestBody: WorkspaceExecutionIntentRequest = {}) => {",
		"const commandIntent = requestBody.intent ?? 'resume';",
		"intent: commandIntent,",
		"runCommandRequestId(commandIntent, workspaceId, 'workspace')",
		"const commandRequest: WorkspaceExecutionIntentRequest = {",
	} {
		if !strings.Contains(apiData, want) {
			t.Fatalf("resumeWorkspaceExecution command surface missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"body: JSON.stringify(activationRequest)",
		"resumeWorkspaceExecution:",
		"goalIdOrRequest",
	} {
		if strings.Contains(apiData, forbidden) {
			t.Fatalf("execution intent API retains legacy activation surface %q", forbidden)
		}
	}

	detailData := string(readWebConsoleFile(t, "src/pages/WorkspaceDetailPage.tsx"))
	for _, want := range []string{
		"workspaceExecutionPrimaryIntent(snapshot)",
		"primaryExecutionIntent === 'resume'",
		"Resume 只继续当前 Run 已封存的定义、模型和执行上下文",
		"commandIntent === 'restart'",
		"workorchApi.submitWorkspaceExecution(targetWorkspaceId, { intent: commandIntent, priority: request.priority })",
	} {
		if !strings.Contains(detailData, want) {
			t.Fatalf("WorkspaceDetailPage missing execution intent contract %q", want)
		}
	}
	activationModal := webSourceSection(t, detailData, "title={activationIntent === 'restart'", "<FloatButton.BackTop")
	for _, forbidden := range []string{
		`name="goalId"`, `name="doDId"`, `name="skillIds"`, `name="mcpServerIds"`, `name="capabilityIds"`, `name="memoryIds"`,
	} {
		if strings.Contains(activationModal, forbidden) {
			t.Fatalf("Workspace execution modal retains ignored form field %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		`title="创建工作空间"`, "workspaceSetupDraftSession", "submitWorkspaceSetup",
	} {
		if strings.Contains(detailData, forbidden) {
			t.Fatalf("WorkspaceDetailPage retains duplicate workspace setup implementation %q", forbidden)
		}
	}

	boardData := string(readWebConsoleFile(t, "src/pages/WorkspaceBoardPage.tsx"))
	if strings.Contains(boardData, "updateWorkspaceLLMSelection") || strings.Contains(boardData, "activationReadyChatTarget") {
		t.Fatal("Workspace board Resume/Restart must not mutate or derive Workspace selection")
	}
	if strings.Contains(boardData, ">恢复运行</Button>") || strings.Contains(boardData, "canStartRun") {
		t.Fatal("Workspace board must use one neutral submit action until detail snapshot selects Resume or Restart")
	}
	helperData := string(readWebConsoleFile(t, "src/helpers.tsx"))
	for _, want := range []string{
		"export function workspaceExecutionPrimaryIntent",
		"if (!activeRunID)",
		"return 'restart';",
		"return 'resume';",
	} {
		if !strings.Contains(helperData, want) {
			t.Fatalf("workspace execution intent helper missing %q", want)
		}
	}
}

func TestWebWorkspaceSetupRequiresTemplateAndDirectory(t *testing.T) {
	data := string(readWebConsoleFile(t, "src/pages/components/WorkspaceSetupModal.tsx"))
	for _, want := range []string{
		`name="templateId" label="工作空间模板（Workspace Template）" rules={[{ required: true`,
		`name="deliverableDirectory" label="工作目录" rules={[{ required: true`,
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("WorkspaceSetupModal missing required setup field %q", want)
		}
	}
	if strings.Contains(data, "可选。选择后自动带入") || strings.Contains(data, "留空时自动使用") {
		t.Fatal("WorkspaceSetupModal still describes required template/directory as optional")
	}
}

func TestWebLLMConfigurationHidesAIProxyImplementationDetails(t *testing.T) {
	pageData := string(readWebConsoleFile(t, "src/pages/ConfigurationPage.tsx"))
	for _, forbidden := range []string{
		`defaultChatTargetKey`,
		`defaultEmbeddingTargetKey`,
		`parseProviderModelTargetKey`,
		`baseURL`,
		`baseUrl`,
		`customProviders`,
		`ai-proxy`,
		`Route Owner`,
		`Endpoints`,
		`Operations`,
		`模型目录`,
		`refreshLLMModelCatalog()`,
		`clearApiKey`,
		`显式清空 API 密钥`,
	} {
		if strings.Contains(pageData, forbidden) {
			t.Fatalf("ConfigurationPage exposes an LLM implementation detail %q", forbidden)
		}
	}
	for _, want := range []string{
		`label: 'LLM'`,
		`name="apiKey"`,
		`name="defaultChatModelId"`,
		`name="defaultEmbeddingModelId"`,
		`name={['budget', 'contextUtilizationRatio']}`,
		`name={['budget', 'estimatedTokensPerByte']}`,
		`保存 LLM 配置`,
		`重启 WorkOrch 后生效`,
	} {
		if !strings.Contains(pageData, want) {
			t.Fatalf("ConfigurationPage missing LLM configuration contract %q", want)
		}
	}

	helperData := string(readWebConsoleFile(t, "src/helpers.tsx"))
	for _, want := range []string{
		"export function llmModelOptions",
		"（已配置但未 ready）",
		"source: 'configured'",
		"disabled: true",
	} {
		if !strings.Contains(helperData, want) {
			t.Fatalf("LLM target helper missing round-trip/readiness contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"label: `${modelId}${item.routeOwner ? ` (${item.routeOwner})` : ''}`",
	} {
		if strings.Contains(helperData, forbidden) {
			t.Fatalf("LLM target helper exposes a catalog implementation detail %q", forbidden)
		}
	}
	apiData := string(readWebConsoleFile(t, "src/api.ts"))
	if strings.Contains(apiData, "/configuration/llm/models") {
		t.Fatal("Web API client retains the standalone LLM catalog surface")
	}

	workspaceData := string(readWebConsoleFile(t, "src/pages/WorkspaceDetailPage.tsx"))
	for _, forbidden := range []string{
		"刷新 ai-proxy 模型目录",
		"${defaultChatProviderId}/${defaultChatModelId}",
		"${activationReadyTarget?.providerId}/${activationReadyTarget?.modelId}",
	} {
		if strings.Contains(workspaceData, forbidden) {
			t.Fatalf("WorkspaceDetailPage exposes LLM provider details %q", forbidden)
		}
	}
}

func readWebConsoleFile(t *testing.T, relPath string) []byte {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../.."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "web/workorch-console", filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read web console file %s: %v", relPath, err)
	}
	return data
}

func mutatingMethod(line string) string {
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if strings.Contains(line, "method: '"+method+"'") || strings.Contains(line, `method: "`+method+`"`) {
			return method
		}
	}
	return ""
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func webSourceSection(t *testing.T, source string, startMarker string, endMarker string) string {
	t.Helper()
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("web source missing start marker %q", startMarker)
	}
	end := strings.Index(source[start+len(startMarker):], endMarker)
	if end < 0 {
		t.Fatalf("web source after %q missing end marker %q", startMarker, endMarker)
	}
	return source[start : start+len(startMarker)+end]
}
