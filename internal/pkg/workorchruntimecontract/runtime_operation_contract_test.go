package workorchruntimecontract

import (
	"fmt"
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

func TestValidateRuntimeOperationContractMCPDynamicInputRequiresScalarValue(t *testing.T) {
	err := ValidateRuntimeOperationContract(RuntimeOperation{
		ID: "op-1", Kind: StepKindMCPToolCall,
		Parameters: []OperationParameter{
			{Name: "serverId", String: strPtr("mcp-lsp-bridge")},
			{Name: "tool", String: strPtr("lsp_definition")},
			{Name: "input.languageIds", StringList: []string{"go", "rust"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "requires exactly one scalar string, integer, or boolean value") {
		t.Fatalf("dynamic list parameter error = %v", err)
	}

	value := "go"
	count := int64(1)
	err = ValidateRuntimeOperationContract(RuntimeOperation{
		ID: "op-1", Kind: StepKindMCPToolCall,
		Parameters: []OperationParameter{
			{Name: "serverId", String: strPtr("mcp-lsp-bridge")},
			{Name: "tool", String: strPtr("lsp_definition")},
			{Name: "input.languageId", String: &value, Integer: &count},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "requires exactly one scalar string, integer, or boolean value") {
		t.Fatalf("dynamic multi-value parameter error = %v", err)
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

func TestValidateRuntimeOperationContractSSHUsesOnlyOpaqueOwnerRefs(t *testing.T) {
	sessionRef, credentialRef, command := "session-a", "credential-a", "uname -a"
	valid := RuntimeOperation{
		ID: "ssh-1", Kind: StepKindSSHCommand, TargetRefs: []string{"target-a"},
		Parameters: []OperationParameter{{Name: "sessionRef", String: &sessionRef}, {Name: "credentialRef", String: &credentialRef}, {Name: "command", String: &command}},
	}
	if err := ValidateRuntimeOperationContract(valid); err != nil {
		t.Fatalf("valid SSH owner-ref operation: %v", err)
	}
	host := "example.internal"
	withEndpoint := valid
	withEndpoint.Parameters = append(withEndpoint.Parameters, OperationParameter{Name: "host", String: &host})
	if err := ValidateRuntimeOperationContract(withEndpoint); err == nil || !strings.Contains(err.Error(), `does not accept parameter "host"`) {
		t.Fatalf("SSH endpoint parameter error = %v", err)
	}
	endpointRef := "https://example.internal"
	withEndpointRef := valid
	withEndpointRef.TargetRefs = []string{endpointRef}
	if err := ValidateRuntimeOperationContract(withEndpointRef); err == nil || !strings.Contains(err.Error(), "canonical owner refs") {
		t.Fatalf("SSH endpoint-shaped target ref error = %v", err)
	}
	oversized := strings.Repeat("x", 16*1024+1)
	withOversizedCommand := valid
	withOversizedCommand.Parameters = []OperationParameter{{Name: "sessionRef", String: &sessionRef}, {Name: "credentialRef", String: &credentialRef}, {Name: "command", String: &oversized}}
	if err := ValidateRuntimeOperationContract(withOversizedCommand); err == nil || !strings.Contains(err.Error(), "bounded command") {
		t.Fatalf("SSH oversized command error = %v", err)
	}
}

func TestValidateRuntimeOperationContractWebIsBoundedAndGrantOnly(t *testing.T) {
	query := "bounded query"
	maxResults := int64(20)
	valid := RuntimeOperation{
		ID: "web-search-1", Kind: StepKindWebSearch, TargetRefs: []string{"web-target"},
		Parameters: []OperationParameter{{Name: "query", String: &query}, {Name: "maxResults", Integer: &maxResults}},
	}
	if err := ValidateRuntimeOperationContract(valid); err != nil {
		t.Fatalf("valid Web search: %v", err)
	}
	approvalID := "legacy-approval"
	withLegacyApproval := valid
	withLegacyApproval.Parameters = append(withLegacyApproval.Parameters, OperationParameter{Name: "approvalId", String: &approvalID})
	if err := ValidateRuntimeOperationContract(withLegacyApproval); err == nil || !strings.Contains(err.Error(), `does not accept parameter "approvalId"`) {
		t.Fatalf("legacy Web approval parameter error = %v", err)
	}
	oversizedQuery := strings.Repeat("q", 4097)
	withOversizedQuery := valid
	withOversizedQuery.Parameters = append([]OperationParameter(nil), valid.Parameters...)
	withOversizedQuery.Parameters[0].String = &oversizedQuery
	if err := ValidateRuntimeOperationContract(withOversizedQuery); err == nil || !strings.Contains(err.Error(), "at most 4096 bytes") {
		t.Fatalf("oversized Web query error = %v", err)
	}
	tooManyResults := int64(21)
	withTooManyResults := valid
	withTooManyResults.Parameters = append([]OperationParameter(nil), valid.Parameters...)
	withTooManyResults.Parameters[1].Integer = &tooManyResults
	if err := ValidateRuntimeOperationContract(withTooManyResults); err == nil || !strings.Contains(err.Error(), "between 1 and 20") {
		t.Fatalf("unbounded Web result count error = %v", err)
	}
	tooManyDomains := make([]string, 33)
	for index := range tooManyDomains {
		tooManyDomains[index] = fmt.Sprintf("domain-%d.example.com", index)
	}
	withTooManyDomains := valid
	withTooManyDomains.Parameters = append(withTooManyDomains.Parameters, OperationParameter{Name: "domains", StringList: tooManyDomains})
	if err := ValidateRuntimeOperationContract(withTooManyDomains); err == nil || !strings.Contains(err.Error(), "exceeds 32 entries") {
		t.Fatalf("unbounded Web domain count error = %v", err)
	}
	webDescriptor, found := RuntimeOperationDescriptorForKind(StepKindWebSearch)
	if !found || webDescriptor.IdempotencyClass != RuntimeOperationIdempotencyAtMostOnce || webDescriptor.DefaultEffectScope != RuntimeEffectNone {
		t.Fatalf("Web execution policy = %+v", webDescriptor)
	}
}

func TestValidateRuntimeOperationContractRejectsCandidateClaimedShellFailureTargets(t *testing.T) {
	command := "go"
	err := ValidateRuntimeOperationContract(RuntimeOperation{
		ID: "validate-1", Kind: StepKindShellCommand,
		Parameters: []OperationParameter{
			{Name: "command", String: &command},
			{Name: "relatedFailureTargets", StringList: []string{"implementation.go"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `does not accept parameter "relatedFailureTargets"`) {
		t.Fatalf("candidate-claimed shell failure targets error = %v", err)
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

func TestRuntimeOperationVariantContractRejectsInvalidGitWriteShapes(t *testing.T) {
	pathspecs := []string{"."}
	message := "checkpoint"
	operation := RuntimeOperation{
		ID: "commit-1", Kind: StepKindGitWrite,
		Parameters: []OperationParameter{
			{Name: "operation", String: strPtr("commit")},
			{Name: "message", String: &message},
			{Name: "pathspecs", StringList: pathspecs},
		},
	}
	err := ValidateRuntimeOperationContract(operation)
	if err == nil || !strings.Contains(err.Error(), `variant "commit" forbids parameter "pathspecs"`) {
		t.Fatalf("commit with pathspecs error = %v", err)
	}
	if !strings.Contains(err.Error(), RuntimeOperationVariantRuleID(StepKindGitWrite, "commit", "forbidden", "pathspecs")) {
		t.Fatalf("commit with pathspecs rule id missing: %v", err)
	}

	operation.Parameters = []OperationParameter{{Name: "operation", String: strPtr("add")}}
	err = ValidateRuntimeOperationContract(operation)
	if err == nil || !strings.Contains(err.Error(), `variant "add" requires parameter "pathspecs"`) {
		t.Fatalf("add without pathspecs error = %v", err)
	}
}

func TestRuntimeOperationVariantContractMatchesPatchVariants(t *testing.T) {
	target := "README.md"
	content := ""
	valid := RuntimeOperation{
		ID: "patch-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []OperationParameter{
			{Name: "patchType", String: strPtr("file_write")},
			{Name: "content", String: &content},
		},
	}
	contract, ok := RuntimeOperationContractForKind(valid.Kind)
	if !ok {
		t.Fatal("patch contract missing")
	}
	variant, err := MatchRuntimeOperationVariant(valid, contract)
	if err != nil || variant.ID != "file_write" {
		t.Fatalf("patch variant = %+v, %v", variant, err)
	}
	if err := ValidateRuntimeOperationContract(valid); err != nil {
		t.Fatalf("valid patch variant: %v", err)
	}

	invalid := valid
	invalid.Parameters = []OperationParameter{
		{Name: "patchType", String: strPtr("text_replace")},
		{Name: "oldText", String: strPtr("same")},
		{Name: "newText", String: strPtr("same")},
	}
	err = ValidateRuntimeOperationContract(invalid)
	if err == nil || !strings.Contains(err.Error(), `variant "text_replace" requires distinct parameters`) {
		t.Fatalf("invalid patch variant error = %v", err)
	}
}

func TestRuntimeOperationVariantContractRejectsOwnerIgnoredParameters(t *testing.T) {
	target := "README.md"
	value := "unexpected"
	tests := []struct {
		name      string
		operation RuntimeOperation
		forbidden string
	}{
		{
			name: "patch file write diff", operation: RuntimeOperation{Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{target}, Parameters: []OperationParameter{
				{Name: "patchType", String: strPtr("file_write")}, {Name: "content", String: strPtr("ok")}, {Name: "diff", String: &value},
			}}, forbidden: "diff",
		},
		{
			name: "patch text replace content", operation: RuntimeOperation{Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{target}, Parameters: []OperationParameter{
				{Name: "patchType", String: strPtr("text_replace")}, {Name: "oldText", String: strPtr("before")}, {Name: "newText", String: strPtr("after")}, {Name: "content", String: &value},
			}}, forbidden: "content",
		},
		{
			name: "git read status ref", operation: RuntimeOperation{Kind: StepKindGitRead, Parameters: []OperationParameter{
				{Name: "operation", String: strPtr("status")}, {Name: "ref", String: &value},
			}}, forbidden: "ref",
		},
		{
			name: "git read branch pathspecs", operation: RuntimeOperation{Kind: StepKindGitRead, Parameters: []OperationParameter{
				{Name: "operation", String: strPtr("branch")}, {Name: "pathspecs", StringList: []string{"."}},
			}}, forbidden: "pathspecs",
		},
		{
			name: "git write init expected head", operation: RuntimeOperation{Kind: StepKindGitWrite, Parameters: []OperationParameter{
				{Name: "operation", String: strPtr("init")}, {Name: "expectedHead", String: &value},
			}}, forbidden: "expectedHead",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRuntimeOperationContract(test.operation)
			if err == nil || !strings.Contains(err.Error(), `forbids parameter "`+test.forbidden+`"`) {
				t.Fatalf("ValidateRuntimeOperationContract() error = %v", err)
			}
		})
	}
}

func TestValidateRequiredOperationBindings(t *testing.T) {
	message := "checkpoint"
	pathspecs := []string{"."}
	operations := []RuntimeOperation{
		{ID: "init", Kind: StepKindGitWrite, RequiredOperationRef: "repo-init", Parameters: []OperationParameter{{Name: "operation", String: strPtr("init")}}},
		{ID: "add", Kind: StepKindGitWrite, RequiredOperationRef: "stage", DependsOn: []string{"init"}, Parameters: []OperationParameter{{Name: "operation", String: strPtr("add")}, {Name: "pathspecs", StringList: pathspecs}}},
		{ID: "commit", Kind: StepKindGitWrite, RequiredOperationRef: "commit", DependsOn: []string{"add"}, Parameters: []OperationParameter{{Name: "operation", String: strPtr("commit")}, {Name: "message", String: &message}}},
	}
	requirements := []RuntimeRequiredOperationVariant{
		{RequirementID: "repo-init", Kind: StepKindGitWrite, VariantID: "init"},
		{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
		{RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
	}
	if err := ValidateRequiredOperationBindings(operations, requirements); err != nil {
		t.Fatalf("valid sequence: %v", err)
	}
	operations[2].DependsOn = nil
	if err := ValidateRequiredOperationBindings(operations, requirements); err == nil || !strings.Contains(err.Error(), `must depend on "stage"`) {
		t.Fatalf("missing dependency error = %v", err)
	}
}

func patchOperationWithType(patchType string) RuntimeOperation {
	return RuntimeOperation{Kind: RuntimeBlockKindPatchApply, Parameters: []OperationParameter{{Name: "patchType", String: &patchType}}}
}

func strPtr(value string) *string { return &value }
