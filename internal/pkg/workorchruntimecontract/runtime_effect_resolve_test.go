package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResolveExpectedEffectScopeShell(t *testing.T) {
	// Default shell is durable.
	op := RuntimeOperation{ID: "op-1", Kind: StepKindShellCommand, Parameters: []OperationParameter{
		{Name: "command", String: strPtr("go")},
	}}
	if got := ResolveExpectedEffectScope(op, nil); got != RuntimeEffectWorkspaceDurable {
		t.Fatalf("default shell effect = %q, want workspace_durable", got)
	}

	falseFlag := false
	op.Parameters = append(op.Parameters, OperationParameter{Name: "stateChanging", Boolean: &falseFlag})
	if got := ResolveExpectedEffectScope(op, nil); got != RuntimeEffectWorkspaceDurable {
		t.Fatalf("unpublished stateChanging=false shell effect = %q, want conservative durable default", got)
	}
	// The generic resolver remains data-driven, but the Shell owner no longer
	// publishes this legacy policy and the operation contract below rejects the
	// shape before a candidate can be sealed.
	readOnlyPolicy := RuntimeOperationPolicyFact{
		ID: "shell-read-only", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
		ParameterMatches: []RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &falseFlag}},
		EffectScope:      RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	if got := ResolveExpectedEffectScope(op, []RuntimeOperationPolicyFact{readOnlyPolicy}); got != RuntimeEffectNone {
		t.Fatalf("explicit legacy stateChanging=false policy effect = %q, want none", got)
	}
	if err := ValidateRuntimeOperationContract(op); err == nil || !strings.Contains(err.Error(), "stateChanging=true") {
		t.Fatalf("Shell contract accepted legacy stateChanging=false shape: %v", err)
	}

	trueFlag := true
	op.TargetRefs = []string{"main.go"}
	op.Parameters = []OperationParameter{
		{Name: "command", String: strPtr("go")},
		{Name: "stateChanging", Boolean: &trueFlag},
	}
	if got := ResolveExpectedEffectScope(op, nil); got != RuntimeEffectWorkspaceDurable {
		t.Fatalf("stateChanging=true shell effect = %q, want workspace_durable", got)
	}
}

func TestResolveExpectedEffectScopeUsesExactOwnerPolicy(t *testing.T) {
	op := RuntimeOperation{ID: "op-1", Kind: StepKindMCPToolCall, Parameters: []OperationParameter{
		{Name: "serverId", String: strPtr("lsp")},
		{Name: "tool", String: strPtr("lsp_initialize")},
	}}
	// Unmatched capability → unknown.
	if got := ResolveExpectedEffectScope(op, nil); got != RuntimeEffectUnknown {
		t.Fatalf("unmatched MCP effect = %q, want unknown", got)
	}

	policies := []RuntimeOperationPolicyFact{mcpOperationPolicyForTest(
		"cap-1", "lsp", "lsp_initialize", RuntimeEffectEphemeralSession, RuntimeOperationIdempotencyOwnerManaged,
	)}
	if got := ResolveExpectedEffectScope(op, policies); got != RuntimeEffectEphemeralSession {
		t.Fatalf("explicit EffectScope = %q, want ephemeral_session", got)
	}

	// A policy for another exact parameter tuple does not leak authority.
	op.Parameters = []OperationParameter{
		{Name: "serverId", String: strPtr("lsp")},
		{Name: "tool", String: strPtr("apply_edit")},
	}
	if got := ResolveExpectedEffectScope(op, policies); got != RuntimeEffectUnknown {
		t.Fatalf("unmatched owner policy effect = %q, want unknown", got)
	}
}

func TestResolveOperationIdempotencyClassUsesDescriptorAndExactOwnerPolicy(t *testing.T) {
	if got := ResolveOperationIdempotencyClass(RuntimeOperation{ID: "read-1", Kind: StepKindFileRead}, nil); got != RuntimeOperationIdempotencySafeToReplay {
		t.Fatalf("file/read idempotency = %q", got)
	}
	if got := ResolveOperationIdempotencyClass(RuntimeOperation{ID: "write-1", Kind: StepKindFileWrite}, nil); got != RuntimeOperationIdempotencyAtMostOnce {
		t.Fatalf("file/write idempotency = %q", got)
	}
	op := RuntimeOperation{ID: "tool-1", Kind: StepKindMCPToolCall, Parameters: []OperationParameter{
		{Name: "serverId", String: strPtr("srv")}, {Name: "tool", String: strPtr("inspect")},
	}}
	if got := ResolveOperationIdempotencyClass(op, nil); got != RuntimeOperationIdempotencyUnknown {
		t.Fatalf("unmatched MCP idempotency = %q", got)
	}
	policies := []RuntimeOperationPolicyFact{mcpOperationPolicyForTest(
		"cap-1", "srv", "inspect", RuntimeEffectNone, RuntimeOperationIdempotencyOwnerManaged,
	)}
	if got := ResolveOperationIdempotencyClass(op, policies); got != RuntimeOperationIdempotencyOwnerManaged {
		t.Fatalf("exact MCP idempotency = %q", got)
	}
}

func TestRuntimeMCPCapabilitiesForActionUsesExplicitEffectMetadata(t *testing.T) {
	capabilities := []RuntimeMCPCapabilityContext{
		{ID: "missing", ServerID: "srv", Kind: "tool", Name: "missing", Enabled: true},
		{ID: "read", ServerID: "srv", Kind: "tool", Name: "inspect", Enabled: true,
			EffectScope: string(RuntimeEffectNone), IdempotencyClass: string(RuntimeOperationIdempotencySafeToReplay)},
		{ID: "session", ServerID: "srv", Kind: "tool", Name: "connect", Enabled: true,
			EffectScope: string(RuntimeEffectEphemeralSession), IdempotencyClass: string(RuntimeOperationIdempotencySafeToReplay)},
		{ID: "write", ServerID: "srv", Kind: "tool", Name: "apply", Enabled: true,
			EffectScope: string(RuntimeEffectWorkspaceDurable), IdempotencyClass: string(RuntimeOperationIdempotencyAtMostOnce)},
	}
	policies := []RuntimeOperationPolicyFact{
		mcpOperationPolicyForTest("read", "srv", "inspect", RuntimeEffectNone, RuntimeOperationIdempotencySafeToReplay),
		mcpOperationPolicyForTest("session", "srv", "connect", RuntimeEffectEphemeralSession, RuntimeOperationIdempotencySafeToReplay),
		mcpOperationPolicyForTest("write", "srv", "apply", RuntimeEffectWorkspaceDurable, RuntimeOperationIdempotencyAtMostOnce),
	}
	validation := RuntimeMCPCapabilitiesForState(capabilities, policies, ConvergenceStateNeedValidation)
	if len(validation) != 2 || validation[0].ID != "read" || validation[1].ID != "session" {
		t.Fatalf("validation MCP capability projection = %+v, want explicit non-durable capabilities", validation)
	}
	stateChange := RuntimeMCPCapabilitiesForState(capabilities, policies, ConvergenceStateNeedStateChange)
	if len(stateChange) != 1 || stateChange[0].ID != "write" {
		t.Fatalf("state-change MCP capability projection = %+v, want explicit durable capability", stateChange)
	}
}

func TestSealRejectsStateChangingFalseShellUnderStateChange(t *testing.T) {
	falseFlag := false
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		Focus: CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}},
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:         []string{StepKindShellCommand},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		},
		AuthoritySnapshot: AuthoritySnapshot{
			ID: "snap-1", WorkspaceID: "ws-1", RunID: "run-1", State: ConvergenceStateNeedStateChange,
			FocusID:            "focus-1",
			RequiredOutputHash: "x", DefinitionRevision: "def-1", EvidenceRevision: InitialRuntimeEvidenceRevision,
		},
	}
	// Seal path uses Validate; build a candidate with stateChanging=false only.
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{
		Operation: RuntimeOperation{
			ID: "op-1", Kind: StepKindShellCommand,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Parameters: []OperationParameter{
				{Name: "command", String: strPtr("go")},
				{Name: "args", StringList: []string{"test", "./..."}},
				{Name: "stateChanging", Boolean: &falseFlag},
			},
		},
	}}}}
	// Authority snapshot hash must match; use SealRuntimeDecision helper path instead via Validate on operations.
	ops := []RuntimeOperation{candidate.StateChange.Operations[0].Operation}
	policy := RuntimeOperationPolicyFact{
		ID: "shell-read-only", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
		ParameterMatches: []RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &falseFlag}},
		EffectScope:      RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, ops, decision.RequiredOutput, []RuntimeOperationPolicyFact{policy})
	if err == nil || !strings.Contains(err.Error(), "durable effect") {
		t.Fatalf("stateChanging=false under state-change must fail seal, got %v", err)
	}
}

func TestSealRejectsReadOnlyMCPUnderStateChange(t *testing.T) {
	required := RequiredOutputContract{
		AcceptedKinds:        []string{StepKindMCPToolCall},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	ops := []RuntimeOperation{{
		ID: "op-1", Kind: StepKindMCPToolCall,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []OperationParameter{
			{Name: "serverId", String: strPtr("lsp")},
			{Name: "tool", String: strPtr("lsp_initialize")},
		},
	}}
	policies := []RuntimeOperationPolicyFact{mcpOperationPolicyForTest(
		"cap-1", "lsp", "lsp_initialize", RuntimeEffectEphemeralSession, RuntimeOperationIdempotencySafeToReplay,
	)}
	err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, ops, required, policies)
	if err == nil || !strings.Contains(err.Error(), "durable effect") {
		t.Fatalf("session/read-only MCP under state-change must fail, got %v", err)
	}
}

func TestSealRejectsMixedDurableAndNonDurableStateChangeCandidate(t *testing.T) {
	falseFlag := false
	required := RequiredOutputContract{
		AcceptedKinds:        []string{StepKindFileWrite, StepKindShellCommand},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	operations := []RuntimeOperation{
		{
			ID: "write", Kind: StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs: []string{"main.go"}, Parameters: []OperationParameter{
				{Name: "content", String: strPtr("package main\n")},
			},
		},
		{
			ID: "validate", Kind: StepKindShellCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Parameters: []OperationParameter{
				{Name: "command", String: strPtr("go")},
				{Name: "args", StringList: []string{"test", "./..."}},
				{Name: "stateChanging", Boolean: &falseFlag},
			},
		},
	}
	policy := RuntimeOperationPolicyFact{
		ID: "shell-read-only", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
		ParameterMatches: []RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &falseFlag}},
		EffectScope:      RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	if err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, operations, required, []RuntimeOperationPolicyFact{policy}); err == nil || !strings.Contains(err.Error(), "validate") {
		t.Fatalf("mixed state-change candidate must be rejected before seal, got %v", err)
	}
}

func TestSealRejectsMutatingUnderValidation(t *testing.T) {
	required := RequiredOutputContract{
		AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	ops := []RuntimeOperation{{
		ID: "op-1", Kind: RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		TargetRefs:          []string{"main.go"},
		Parameters: []OperationParameter{
			{Name: "patchType", String: strPtr("file_write")},
			{Name: "content", String: strPtr("package main\n")},
		},
	}}
	err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedValidation, ops, required, nil)
	if err == nil {
		t.Fatal("mutating patch under validation must fail")
	}
}

func TestEffectCompatibilityCannotOverrideShellOperationContract(t *testing.T) {
	falseFlag := false
	required := RequiredOutputContract{
		AcceptedKinds:        []string{StepKindShellCommand},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	ops := []RuntimeOperation{{
		ID: "op-1", Kind: StepKindShellCommand,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []OperationParameter{
			{Name: "command", String: strPtr("go")},
			{Name: "args", StringList: []string{"test", "./..."}},
			{Name: "stateChanging", Boolean: &falseFlag},
		},
	}}
	policy := RuntimeOperationPolicyFact{
		ID: "shell-read-only", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
		ParameterMatches: []RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &falseFlag}},
		EffectScope:      RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	if err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedValidation, ops, required, []RuntimeOperationPolicyFact{policy}); err != nil {
		t.Fatalf("generic effect compatibility unexpectedly rejected explicit policy: %v", err)
	}
	if err := ValidateRuntimeOperationContract(ops[0]); err == nil || !strings.Contains(err.Error(), "stateChanging=true") {
		t.Fatalf("Shell operation contract accepted legacy non-durable shape: %v", err)
	}
}

func TestSealAcceptsDurableShellUnderStateChange(t *testing.T) {
	trueFlag := true
	required := RequiredOutputContract{
		AcceptedKinds:        []string{StepKindShellCommand},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	ops := []RuntimeOperation{{
		ID: "op-1", Kind: StepKindShellCommand,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		TargetRefs:          []string{"main.go"},
		Parameters: []OperationParameter{
			{Name: "command", String: strPtr("go")},
			{Name: "args", StringList: []string{"generate", "./..."}},
			{Name: "stateChanging", Boolean: &trueFlag},
		},
	}}
	if err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, ops, required, nil); err != nil {
		t.Fatalf("stateChanging=true shell under state-change must pass: %v", err)
	}
}

func TestMCPRegistryKindNormalizationDoesNotBackfillOperationKind(t *testing.T) {
	caps := normalizeRuntimeMCPCapabilities([]RuntimeMCPCapabilityContext{{
		ID: "cap-1", ServerID: "srv", Kind: "tool", Name: "list", Enabled: true, RiskLevel: "read_only",
	}})
	if len(caps) != 1 || caps[0].OperationKind != "" {
		t.Fatalf("normalization backfilled missing OperationKind: %+v", caps)
	}
}

func TestSealMCPCapabilityEffectRequiresExplicitOwnerMetadata(t *testing.T) {
	cap := SealMCPCapabilityEffect(RuntimeMCPCapabilityContext{
		ID: "cap-1", ServerID: "lsp", Kind: "tool", Name: "lsp_initialize",
		RiskLevel: "read_only", Enabled: true,
	})
	if cap.OperationKind != StepKindMCPToolCall {
		t.Fatalf("OperationKind = %q, want mcp/tool_call", cap.OperationKind)
	}
	if cap.EffectScope != string(RuntimeEffectUnknown) {
		t.Fatalf("missing EffectScope = %q, want unknown", cap.EffectScope)
	}
	if cap.IdempotencyClass != string(RuntimeOperationIdempotencyUnknown) {
		t.Fatalf("missing IdempotencyClass = %q, want unknown", cap.IdempotencyClass)
	}
	// Explicit capability metadata is preserved for provider projection; the
	// owner publishes separate generic operation policy authority.
	cap.EffectScope = string(RuntimeEffectEphemeralSession)
	cap.IdempotencyClass = string(RuntimeOperationIdempotencySafeToReplay)
	op := RuntimeOperation{ID: "op-1", Kind: StepKindMCPToolCall, Parameters: []OperationParameter{
		{Name: "serverId", String: strPtr("lsp")},
		{Name: "tool", String: strPtr("lsp_initialize")},
	}}
	policy := mcpOperationPolicyForTest("cap-1", "lsp", "lsp_initialize", RuntimeEffectEphemeralSession, RuntimeOperationIdempotencySafeToReplay)
	if scope := ResolveExpectedEffectScope(op, []RuntimeOperationPolicyFact{policy}); scope != RuntimeEffectEphemeralSession {
		t.Fatalf("explicit MCP EffectScope = %q, want ephemeral_session", scope)
	}
}

func mcpOperationPolicyForTest(id, serverID, tool string, effect RuntimeEffectScope, idempotency RuntimeOperationIdempotencyClass) RuntimeOperationPolicyFact {
	return RuntimeOperationPolicyFact{
		ID: id, OwnerModuleID: RuntimeModuleMCP, OperationKind: StepKindMCPToolCall, SubjectRef: id,
		ParameterMatches: []RuntimeOperationParameterMatch{
			{Name: "serverId", String: strPtr(serverID)},
			{Name: "tool", String: strPtr(tool)},
		},
		EffectScope: effect, IdempotencyClass: idempotency,
	}
}
