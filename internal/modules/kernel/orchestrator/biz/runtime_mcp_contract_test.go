package orchestrator

import (
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateMCPToolCallStepInputRejectsCapabilitySchemaDrift(t *testing.T) {
	run := runWithMCPCapabilitySchema()
	step := &runModel.Step{
		ID:    "run-mcp-schema/graph-001/001-init",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-lsp-bridge",
			"tool":     "lsp_initialize",
			"input": map[string]any{
				"rootPath": "/tmp/project",
				"langId":   "typescript",
			},
		},
	}

	err := validateMCPToolCallStepInputBeforeRuntime(run, step, step.Input)
	if err == nil {
		t.Fatalf("validateMCPToolCallStepInputBeforeRuntime err = nil, want schema rejection")
	}
	text := err.Error()
	if !strings.Contains(text, "root_path") || !strings.Contains(text, "mcp-lsp-bridge/lsp_initialize") {
		t.Fatalf("err = %v, want missing snake_case schema field", err)
	}
}

func TestValidateMCPToolCallStepInputAllowsCapabilitySchemaMatch(t *testing.T) {
	run := runWithMCPCapabilitySchema()
	step := &runModel.Step{
		ID:    "run-mcp-schema/graph-001/001-init",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-lsp-bridge",
			"tool":     "lsp_initialize",
			"input": map[string]any{
				"root_path": "/tmp/project",
				"lang_id":   "typescript",
			},
		},
	}

	if err := validateMCPToolCallStepInputBeforeRuntime(run, step, step.Input); err != nil {
		t.Fatalf("validateMCPToolCallStepInputBeforeRuntime err = %v, want nil", err)
	}
}

func TestValidateMCPToolCallStepInputSkipsWhenCapabilitySchemaMissing(t *testing.T) {
	run := &runModel.Run{
		ID:          "run-mcp-schema",
		WorkspaceID: "ws-mcp-schema",
		ExecutionContext: map[string]any{
			"mcpCapabilities": []map[string]any{{
				"serverId": "mcp-lsp-bridge",
				"kind":     "tool",
				"name":     "lsp_initialize",
				"status":   "active",
			}},
		},
	}
	step := &runModel.Step{
		ID:    "run-mcp-schema/graph-001/001-init",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-lsp-bridge",
			"tool":     "lsp_initialize",
			"input":    map[string]any{"rootPath": "/tmp/project"},
		},
	}

	if err := validateMCPToolCallStepInputBeforeRuntime(run, step, step.Input); err != nil {
		t.Fatalf("validateMCPToolCallStepInputBeforeRuntime err = %v, want nil when schema is absent", err)
	}
}

func runWithMCPCapabilitySchema() *runModel.Run {
	return &runModel.Run{
		ID:          "run-mcp-schema",
		WorkspaceID: common.WorkspaceID("ws-mcp-schema"),
		ExecutionContext: map[string]any{
			"mcpCapabilities": []map[string]any{{
				"serverId": "mcp-lsp-bridge",
				"kind":     "tool",
				"name":     "lsp_initialize",
				"status":   "active",
				"schema": map[string]any{
					"type":     "object",
					"required": []string{"root_path", "lang_id"},
					"properties": map[string]any{
						"root_path": map[string]any{"type": "string"},
						"lang_id":   map[string]any{"type": "string"},
					},
					"additionalProperties": false,
				},
			}},
		},
	}
}
