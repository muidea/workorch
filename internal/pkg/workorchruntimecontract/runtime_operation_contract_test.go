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

func TestValidateRuntimeOperationContractFileWriteRequiresTargetAndContent(t *testing.T) {
	target := "README.md"
	content := ""
	valid := RuntimeOperation{
		ID: "write-1", Kind: StepKindFileWrite, TargetRefs: []string{target},
		Parameters: []OperationParameter{{Name: "content", String: &content}},
	}
	if err := ValidateRuntimeOperationContract(valid); err != nil {
		t.Fatalf("valid file/write: %v", err)
	}
	missingContent := valid
	missingContent.Parameters = nil
	if err := ValidateRuntimeOperationContract(missingContent); err == nil {
		t.Fatal("file/write without content must fail")
	}
	missingTarget := valid
	missingTarget.TargetRefs = nil
	if err := ValidateRuntimeOperationContract(missingTarget); err == nil {
		t.Fatal("file/write without target must fail")
	}
}

func TestRuntimeOperationRequiresCurrentTargetSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		operation RuntimeOperation
		want      bool
	}{
		{name: "file edit", operation: RuntimeOperation{Kind: StepKindFileEdit}, want: true},
		{name: "text replace", operation: patchOperationWithType("text_replace"), want: true},
		{name: "unified diff", operation: patchOperationWithType("unified_diff"), want: true},
		{name: "file write", operation: patchOperationWithType("file_write"), want: false},
		{name: "validation", operation: RuntimeOperation{Kind: RuntimeBlockKindManagedValidationSession}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RuntimeOperationRequiresCurrentTargetSnapshot(tt.operation); got != tt.want {
				t.Fatalf("RuntimeOperationRequiresCurrentTargetSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func patchOperationWithType(patchType string) RuntimeOperation {
	return RuntimeOperation{Kind: RuntimeBlockKindPatchApply, Parameters: []OperationParameter{{Name: "patchType", String: &patchType}}}
}

func strPtr(value string) *string { return &value }
