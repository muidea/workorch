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
	if got := ResolveExpectedEffectScope(op, nil); got != RuntimeEffectNone {
		t.Fatalf("stateChanging=false shell effect = %q, want none", got)
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

func TestResolveExpectedEffectScopeMCP(t *testing.T) {
	op := RuntimeOperation{ID: "op-1", Kind: StepKindMCPToolCall, Parameters: []OperationParameter{
		{Name: "serverId", String: strPtr("lsp")},
		{Name: "tool", String: strPtr("lsp_initialize")},
	}}
	// Unmatched capability → unknown.
	if got := ResolveExpectedEffectScope(op, nil); got != RuntimeEffectUnknown {
		t.Fatalf("unmatched MCP effect = %q, want unknown", got)
	}

	// Risk metadata is advisory only; missing explicit owner metadata stays unknown.
	caps := []RuntimeMCPCapabilityContext{{
		ID: "cap-1", ServerID: "lsp", Kind: "tool", Name: "lsp_initialize",
		RiskLevel: "read_only", Enabled: true,
	}}
	if got := ResolveExpectedEffectScope(op, caps); got != RuntimeEffectUnknown {
		t.Fatalf("MCP effect without explicit metadata = %q, want unknown", got)
	}

	// Explicit sealed EffectScope wins.
	caps[0].EffectScope = string(RuntimeEffectEphemeralSession)
	caps[0].RiskLevel = "write"
	if got := ResolveExpectedEffectScope(op, caps); got != RuntimeEffectEphemeralSession {
		t.Fatalf("explicit EffectScope = %q, want ephemeral_session", got)
	}

	// Mutating risk without an explicit effect is likewise unknown.
	caps[0].EffectScope = ""
	caps[0].Name = "apply_edit"
	caps[0].RiskLevel = "write"
	op.Parameters = []OperationParameter{
		{Name: "serverId", String: strPtr("lsp")},
		{Name: "tool", String: strPtr("apply_edit")},
	}
	if got := ResolveExpectedEffectScope(op, caps); got != RuntimeEffectUnknown {
		t.Fatalf("MCP write risk without explicit effect = %q, want unknown", got)
	}
}

func TestResolveOperationIdempotencyClassUsesDescriptorAndExactMCPCapability(t *testing.T) {
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
	capabilities := []RuntimeMCPCapabilityContext{{
		ID: "cap-1", ServerID: "srv", Kind: "tool", Name: "inspect", Enabled: true,
		EffectScope: string(RuntimeEffectNone), IdempotencyClass: string(RuntimeOperationIdempotencyOwnerManaged),
	}}
	if got := ResolveOperationIdempotencyClass(op, capabilities); got != RuntimeOperationIdempotencyOwnerManaged {
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
	validation := RuntimeMCPCapabilitiesForState(capabilities, ConvergenceStateNeedValidation)
	if len(validation) != 2 || validation[0].ID != "read" || validation[1].ID != "session" {
		t.Fatalf("validation MCP capability projection = %+v, want explicit non-durable capabilities", validation)
	}
	stateChange := RuntimeMCPCapabilitiesForState(capabilities, ConvergenceStateNeedStateChange)
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
			RuntimeContractVersion: RuntimeContractVersionV2,
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
	err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, ops, decision.RequiredOutput, nil)
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
	caps := []RuntimeMCPCapabilityContext{{
		ID: "cap-1", ServerID: "lsp", Kind: "tool", Name: "lsp_initialize",
		RiskLevel: "read_only", Enabled: true, EffectScope: string(RuntimeEffectEphemeralSession),
	}}
	err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, ops, required, caps)
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
	if err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedStateChange, operations, required, nil); err == nil || !strings.Contains(err.Error(), "validate") {
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

func TestSealAcceptsNonDurableShellUnderValidation(t *testing.T) {
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
	if err := ValidateCandidateEffectCompatibility(ConvergenceStateNeedValidation, ops, required, nil); err != nil {
		t.Fatalf("stateChanging=false shell under validation must pass: %v", err)
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

func TestMCPRegistryKindNormalizesToOperationKind(t *testing.T) {
	caps := normalizeRuntimeMCPCapabilities([]RuntimeMCPCapabilityContext{{
		ID: "cap-1", ServerID: "srv", Kind: "tool", Name: "list", Enabled: true, RiskLevel: "read_only",
	}})
	if len(caps) != 1 || caps[0].OperationKind != StepKindMCPToolCall {
		t.Fatalf("registry tool must normalize OperationKind to mcp/tool_call, got %+v", caps)
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
	// Explicit capability metadata is preserved and used as Runtime effect authority.
	cap.EffectScope = string(RuntimeEffectEphemeralSession)
	cap.IdempotencyClass = string(RuntimeOperationIdempotencySafeToReplay)
	op := RuntimeOperation{ID: "op-1", Kind: StepKindMCPToolCall, Parameters: []OperationParameter{
		{Name: "serverId", String: strPtr("lsp")},
		{Name: "tool", String: strPtr("lsp_initialize")},
	}}
	if scope := ResolveExpectedEffectScope(op, []RuntimeMCPCapabilityContext{cap}); scope != RuntimeEffectEphemeralSession {
		t.Fatalf("explicit MCP EffectScope = %q, want ephemeral_session", scope)
	}
}
