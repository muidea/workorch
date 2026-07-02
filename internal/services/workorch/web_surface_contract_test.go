package workorch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWebConsoleMutatingEndpointsStayWithinProductSurfaceContract(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../.."))
	apiPath := filepath.Join(repoRoot, "web/workorch-console/src/api.ts")
	data, err := os.ReadFile(apiPath)
	if err != nil {
		t.Fatalf("read web console api: %v", err)
	}
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
