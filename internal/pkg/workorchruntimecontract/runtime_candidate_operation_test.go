package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateRequiresAcceptedOperationWhenDeclared(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{"git/write"}, AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)

	add := "add"
	pathspecs := []string{"main.go"}
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "git-add", Kind: "git/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Parameters: []OperationParameter{{Name: "operation", String: &add}, {Name: "pathspecs", StringList: pathspecs}},
	}}}}}
	if err := candidate.Validate(decision); err == nil || !strings.Contains(err.Error(), "requires one accepted operation variant") {
		t.Fatalf("candidate error = %v, want accepted operation variant requirement", err)
	}

	commit := "commit"
	message := "commit change"
	candidate.StateChange.Operations = append(candidate.StateChange.Operations, StateChangingOperation{Operation: RuntimeOperation{
		ID: "git-commit", Kind: "git/write", DependsOn: []string{"git-add"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []OperationParameter{{Name: "operation", String: &commit}, {Name: "message", String: &message}},
	}})
	if err := candidate.Validate(decision); err != nil {
		t.Fatalf("candidate with commit was rejected: %v", err)
	}
}

func TestRuntimeCandidateRejectsFileOperationAliasesAndNoOpBeforeDispatch(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileEdit}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)

	filePath := "main.go"
	oldText := "old"
	newText := "new"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "edit-1", Kind: StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		Parameters: []OperationParameter{{Name: "filePath", String: &filePath}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
	}}}}}
	if err := candidate.Validate(decision); err == nil || !strings.Contains(err.Error(), `does not accept parameter "filePath"`) {
		t.Fatalf("filePath candidate error = %v", err)
	}

	candidate.StateChange.Operations[0].Operation.Parameters = []OperationParameter{
		{Name: "path", String: &filePath}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &oldText},
	}
	if err := candidate.Validate(decision); err == nil || !strings.Contains(err.Error(), "distinct newText") {
		t.Fatalf("no-op candidate error = %v", err)
	}
}

func TestRuntimeCandidateAcceptsConcretePatchApplyContract(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{RuntimeBlockKindPatchApply}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)
	targetRef := "main.go"
	patchType := "text_replace"
	oldText := "old"
	newText := "new"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "patch-1", Kind: RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{targetRef},
		Parameters: []OperationParameter{
			{Name: "targetRef", String: &targetRef}, {Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		},
	}}}}}
	if err := candidate.Validate(decision); err != nil {
		t.Fatalf("concrete patch/apply candidate rejected: %v", err)
	}
}

func TestValidateRuntimeOperationsRejectsMutatingKindForValidation(t *testing.T) {
	decision := validDecisionForState(ConvergenceStateNeedValidation)
	decision.Action = RuntimeActionProduceValidation
	decision.RequiredOutput = RequiredOutputContract{
		Kind:                 RequiredOutputKindValidation,
		AcceptedKinds:        []string{RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview, StepKindShellCommand},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)

	path := "cmd/proxy/main.go"
	content := "package main\n"
	patchType := "file_write"
	mutating := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{
		Operation: RuntimeOperation{
			ID: "op-1", Kind: RuntimeBlockKindPatchApply,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:          []string{path},
			Parameters: []OperationParameter{
				{Name: "targetRef", String: &path},
				{Name: "patchType", String: &patchType},
				{Name: "content", String: &content},
			},
		},
	}}}}
	if err := mutating.Validate(decision); err == nil || !strings.Contains(err.Error(), "cannot satisfy produce_validation") {
		t.Fatalf("expected patch/apply rejection under validation, got %v", err)
	}

	previewType := "unified_diff"
	diff := "--- a\n+++ b\n"
	preview := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{
		Operation: RuntimeOperation{
			ID: "op-preview", Kind: RuntimeBlockKindPatchPreview,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:          []string{path},
			Parameters: []OperationParameter{
				{Name: "targetRef", String: &path},
				{Name: "patchType", String: &previewType},
				{Name: "diff", String: &diff},
			},
		},
	}}}}
	if err := preview.Validate(decision); err != nil {
		t.Fatalf("patch/preview should be allowed under validation: %v", err)
	}

	shCmd := "sh"
	args := []string{"-c", "echo x > out.txt"}
	stateChanging := true
	target := "out.txt"
	shellMut := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{
		Operation: RuntimeOperation{
			ID: "op-shell-mut", Kind: StepKindShellCommand,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:          []string{target},
			Parameters: []OperationParameter{
				{Name: "command", String: &shCmd},
				{Name: "args", StringList: args},
				{Name: "stateChanging", Boolean: &stateChanging},
				{Name: "targetRef", String: &target},
			},
		},
	}}}}
	if err := shellMut.Validate(decision); err == nil || !strings.Contains(err.Error(), "cannot satisfy produce_validation") {
		t.Fatalf("expected stateChanging shell rejection under validation, got %v", err)
	}

	// RVC-R6: explicit stateChanging=false narrows shell to non-durable and is
	// admissible under produce_validation. Candidate cannot self-declare durable
	// effect away without the parameter; default shell remains rejected above.
	stateChanging = false
	shellDeclaredReadOnly := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{
		Operation: RuntimeOperation{
			ID: "op-shell-declared-read-only", Kind: StepKindShellCommand,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Parameters: []OperationParameter{
				{Name: "command", String: &shCmd},
				{Name: "args", StringList: args},
				{Name: "stateChanging", Boolean: &stateChanging},
			},
		},
	}}}}
	if err := shellDeclaredReadOnly.Validate(decision); err != nil {
		t.Fatalf("stateChanging=false shell under validation must be accepted, got %v", err)
	}
}

func TestRuntimeOperationNormalizedPreservesOrderedArgs(t *testing.T) {
	cmd := "go"
	// Flags that sort before "test" would produce `go -run ...` if StringList were sorted.
	args := []string{"test", "./internal/pkg/...", "-run", "TestFoo$", "-count=1", "-v"}
	operation := RuntimeOperation{
		ID: "op-go-test", Kind: StepKindShellCommand,
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args},
		},
	}.Normalized()

	gotArgs := operationStringListParameter(operation.Parameters, "args")
	wantArgs := []string{"test", "./internal/pkg/...", "-run", "TestFoo$", "-count=1", "-v"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args after Normalized() = %#v, want %#v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("args after Normalized() = %#v, want %#v (order must be preserved)", gotArgs, wantArgs)
		}
	}

	// Unordered set parameters should still dedupe+sort.
	operation = RuntimeOperation{
		ID: "op-shell-targets", Kind: StepKindShellCommand, TargetRefs: []string{"b", "a", "a"},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args},
			{Name: "targetRefs", StringList: []string{"b", "a", "a", "  "}},
		},
	}.Normalized()
	gotTargets := operationStringListParameter(operation.Parameters, "targetRefs")
	wantTargets := []string{"a", "b"}
	if len(gotTargets) != len(wantTargets) || gotTargets[0] != wantTargets[0] || gotTargets[1] != wantTargets[1] {
		t.Fatalf("targetRefs after Normalized() = %#v, want %#v", gotTargets, wantTargets)
	}
	// pathspecs are positional for git; preserve order too.
	pathspecs := []string{"z.go", "a.go"}
	operation = RuntimeOperation{
		ID: "op-git-add", Kind: StepKindGitWrite,
		Parameters: []OperationParameter{
			{Name: "operation", String: strPtr("add")},
			{Name: "pathspecs", StringList: pathspecs},
		},
	}.Normalized()
	gotPathspecs := operationStringListParameter(operation.Parameters, "pathspecs")
	if len(gotPathspecs) != 2 || gotPathspecs[0] != "z.go" || gotPathspecs[1] != "a.go" {
		t.Fatalf("pathspecs after Normalized() = %#v, want [z.go a.go]", gotPathspecs)
	}
}

func operationStringListParameter(parameters []OperationParameter, name string) []string {
	for _, parameter := range parameters {
		if parameter.Name == name {
			return append([]string(nil), parameter.StringList...)
		}
	}
	return nil
}

func TestOperationHasWorkspaceSideEffectsDefaultsSSHAndMCP(t *testing.T) {
	host := "example.com"
	user := "runner"
	command := "uname -a"
	sshOp := RuntimeOperation{
		ID: "ssh-1", Kind: StepKindSSHCommand,
		Parameters: []OperationParameter{
			{Name: "host", String: &host},
			{Name: "username", String: &user},
			{Name: "command", String: &command},
		},
	}
	if !OperationHasWorkspaceSideEffects(sshOp) {
		t.Fatalf("ssh/command must be treated as side-effecting without sealed metadata")
	}

	serverID := "mcp-server"
	tool := "repo.write"
	mcpOp := RuntimeOperation{
		ID: "mcp-1", Kind: StepKindMCPToolCall,
		Parameters: []OperationParameter{
			{Name: "serverId", String: &serverID},
			{Name: "tool", String: &tool},
		},
	}
	if !OperationHasWorkspaceSideEffects(mcpOp) {
		t.Fatalf("unknown mcp tool must default to side-effecting")
	}

	readOnlyCaps := []RuntimeMCPCapabilityContext{{
		ID: "cap-1", ServerID: common.MCPServerID(serverID), Kind: StepKindMCPToolCall, Name: tool,
		DisplayName: tool, Enabled: true, RiskLevel: "read_only", EffectScope: string(RuntimeEffectNone),
	}}
	if OperationHasWorkspaceSideEffectsWithCapabilities(mcpOp, readOnlyCaps) {
		t.Fatalf("sealed read_only MCP capability should be non-mutating")
	}

	mutatingCaps := []RuntimeMCPCapabilityContext{{
		ID: "cap-2", ServerID: common.MCPServerID(serverID), Kind: StepKindMCPToolCall, Name: tool,
		DisplayName: tool, Enabled: true, RiskLevel: "write", EffectScope: string(RuntimeEffectWorkspaceDurable),
	}}
	if !OperationHasWorkspaceSideEffectsWithCapabilities(mcpOp, mutatingCaps) {
		t.Fatalf("write MCP capability must remain side-effecting")
	}

	decision := validDecisionForState(ConvergenceStateNeedValidation)
	decision.Action = RuntimeActionProduceValidation
	decision.RequiredOutput = RequiredOutputContract{
		Kind: RequiredOutputKindValidation, AcceptedKinds: []string{StepKindSSHCommand, StepKindMCPToolCall},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}.Normalized()
	decision.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(decision.RequiredOutput)
	decision.AuthoritySnapshot.ID = stableAuthoritySnapshotID(decision.AuthoritySnapshot)

	sshCandidate := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{
		Operation: RuntimeOperation{
			ID: "ssh-val", Kind: StepKindSSHCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Parameters: []OperationParameter{
				{Name: "host", String: &host}, {Name: "username", String: &user}, {Name: "command", String: &command},
			},
		},
	}}}}
	if err := sshCandidate.Validate(decision); err == nil || !strings.Contains(err.Error(), "cannot satisfy produce_validation") {
		t.Fatalf("ssh validation candidate must be rejected, got %v", err)
	}

	mcpCandidate := RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{
		Operation: RuntimeOperation{
			ID: "mcp-val", Kind: StepKindMCPToolCall, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Parameters: []OperationParameter{
				{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool},
			},
		},
	}}}}
	if err := mcpCandidate.Validate(decision); err == nil || !strings.Contains(err.Error(), "cannot satisfy produce_validation") {
		t.Fatalf("unknown mcp validation candidate must be rejected, got %v", err)
	}
	activation := RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, MCPCapabilities: readOnlyCaps}
	if err := mcpCandidate.ValidateWithActivation(decision, activation); err != nil {
		t.Fatalf("read_only mcp validation candidate should be allowed: %v", err)
	}
}

func TestEvidenceAppliesToCurrentFocusRequiresLastFocusForEmptyCoverage(t *testing.T) {
	facts := RuntimeFacts{
		Activation:  RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-1",
		Focus: CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{EvidenceKindValidation},
		},
		RemainingFocuses: []CoverageFocus{{
			ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"},
			RequiredEvidenceKinds: []string{EvidenceKindValidation},
		}},
		Evidence: EvidenceLedger{Accepted: []EvidenceFact{{
			ID: "acc-empty", Kind: EvidenceKindValidation,
		}}},
	}.Normalized()
	if EvidenceAppliesToCurrentFocus(facts, facts.Evidence.Accepted[0]) {
		t.Fatalf("empty-coverage evidence must not apply before the last focus")
	}
	missing := MissingRequiredEvidenceKinds(facts)
	if len(missing) != 1 || missing[0] != EvidenceKindValidation {
		t.Fatalf("missing evidence = %#v, want [validation]", missing)
	}

	facts.RemainingFocuses = nil
	facts = facts.Normalized()
	if !EvidenceAppliesToCurrentFocus(facts, facts.Evidence.Accepted[0]) {
		t.Fatalf("empty-coverage evidence must apply on the last focus")
	}
	missing = MissingRequiredEvidenceKinds(facts)
	if len(missing) != 0 {
		t.Fatalf("missing evidence on last focus = %#v, want empty", missing)
	}
}
