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
	for _, want := range []string{
		"runId?: string;",
		"intent?: 'resume';",
	} {
		if !strings.Contains(typesData, want) {
			t.Fatalf("WorkspaceExecutionActivationRequest missing %q", want)
		}
	}

	apiData := string(readWebConsoleFile(t, "src/api.ts"))
	if !strings.Contains(apiData, "/snapshot?view=detail") {
		t.Fatal("readWorkspaceSnapshotProjection must request explicit detail view")
	}
	for _, want := range []string{
		"const commandIntent = 'resume';",
		"intent: commandIntent,",
		"runCommandRequestId(commandIntent, workspaceId, activationRequest.runId || 'workspace')",
	} {
		if !strings.Contains(apiData, want) {
			t.Fatalf("resumeWorkspaceExecution command surface missing %q", want)
		}
	}
}

func TestWebLLMConfigurationUsesProviderScopedModelTargets(t *testing.T) {
	pageData := string(readWebConsoleFile(t, "src/pages/ConfigurationPage.tsx"))
	for _, forbidden := range []string{
		`name={['defaultChat', 'providerID']} hidden`,
		`name={['defaultEmbedding', 'providerID']} hidden`,
		`value: item.modelId`,
	} {
		if strings.Contains(pageData, forbidden) {
			t.Fatalf("ConfigurationPage retains provider-blind model selection %q", forbidden)
		}
	}
	for _, want := range []string{
		`name="defaultChatTargetKey"`,
		`name="defaultEmbeddingTargetKey"`,
		`parseProviderModelTargetKey(values.defaultChatTargetKey)`,
		`item.ready && item.value === values.defaultChatTargetKey`,
	} {
		if !strings.Contains(pageData, want) {
			t.Fatalf("ConfigurationPage missing provider-scoped model target contract %q", want)
		}
	}

	helperData := string(readWebConsoleFile(t, "src/helpers.tsx"))
	for _, want := range []string{
		"export function providerModelTargetKey",
		"export function parseProviderModelTargetKey",
		"encodeURIComponent(normalizedProviderId)",
		"decodeURIComponent(normalized.slice(separator + 1))",
		"（已配置但未 ready）",
		"source: 'configured'",
		"disabled: true",
	} {
		if !strings.Contains(helperData, want) {
			t.Fatalf("LLM target helper missing round-trip/readiness contract %q", want)
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
