package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestValidateRuntimeOperationContractMCPInvalidParameterHint(t *testing.T) {
	langID := "go"
	err := ValidateRuntimeOperationContract(RuntimeOperation{
		ID: "op-1", Kind: StepKindMCPToolCall,
		Parameters: []OperationParameter{
			{Name: "serverId", String: strPtr("mcp-lsp-bridge")},
			{Name: "tool", String: strPtr("lsp_definition")},
			{Name: "lang_id", String: &langID},
		},
	})
	if err == nil {
		t.Fatal("expected invalid parameter error")
	}
	message := err.Error()
	if !strings.Contains(message, `does not accept parameter "lang_id"`) {
		t.Fatalf("missing base error: %s", message)
	}
	if !strings.Contains(message, "input.<name>") || !strings.Contains(message, `"input.lang_id"`) {
		t.Fatalf("missing MCP rewrite hint: %s", message)
	}
}

func TestValidateRuntimeOperationContractMCPDynamicInputAllowed(t *testing.T) {
	err := ValidateRuntimeOperationContract(RuntimeOperation{
		ID: "op-1", Kind: StepKindMCPToolCall,
		Parameters: []OperationParameter{
			{Name: "serverId", String: strPtr("mcp-lsp-bridge")},
			{Name: "tool", String: strPtr("lsp_definition")},
			{Name: "input.languageId", String: strPtr("go")},
		},
	})
	if err != nil {
		t.Fatalf("dynamic input parameter should be allowed: %v", err)
	}
}

func strPtr(value string) *string { return &value }
