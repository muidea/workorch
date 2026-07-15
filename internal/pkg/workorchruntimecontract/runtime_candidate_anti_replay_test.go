package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateCandidateAgainstRejectedShapesBlocksOnlyActiveShapes(t *testing.T) {
	path := "cmd/proxy/main.go"
	patchType := "text_replace"
	oldText := "package main"
	newText := "package proxy"
	op := RuntimeOperation{
		ID: "op-001", Kind: RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		TargetRefs:          []string{path},
		Parameters: []OperationParameter{
			{Name: "targetRef", String: &path},
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &newText},
		},
	}
	facts := RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-1",
		Focus: CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}},
		Evidence: EvidenceLedger{Rejected: []EvidenceFact{{
			ID: "rej-1", Kind: EvidenceKindStateChange, DispatchID: "dispatch-1", OperationID: "op-001",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, FailureSignature: "sig-1",
			Revision: "evidence-v2-00000000000000000001",
		}}},
		Failure: &FailureFact{Signature: "sig-1", Message: "conflict"},
	}.Normalized()
	dispatches := []RuntimeDispatchRecord{{
		ID: "dispatch-1", Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: op}}}},
	}}
	replay := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: op}}}}
	if err := ValidateCandidateAgainstRejectedShapes(facts, dispatches, replay); err == nil || !strings.Contains(err.Error(), "replays active rejected operation shape") {
		t.Fatalf("expected active replay rejection, got %v", err)
	}

	// After a later accepted evidence, the old rejected shape is no longer active.
	facts.Evidence.Accepted = []EvidenceFact{{
		ID: "acc-1", Kind: EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "evidence-v2-00000000000000000002",
	}}
	if err := ValidateCandidateAgainstRejectedShapes(facts, dispatches, replay); err != nil {
		t.Fatalf("replay after accepted progress should be allowed: %v", err)
	}
}

func TestStableRuntimeOperationShapeIgnoresSummaryMetadata(t *testing.T) {
	path := "main.go"
	cmd := "go"
	args := []string{"test", "./..."}
	summaryA := "first try"
	summaryB := "retry with different wording"
	left := RuntimeOperation{
		ID: "op-1", Kind: StepKindShellCommand, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args},
			{Name: "summary", String: &summaryA},
		},
	}
	right := RuntimeOperation{
		ID: "op-2", Kind: StepKindShellCommand, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args},
			{Name: "summary", String: &summaryB},
		},
	}
	if stableRuntimeOperationShape(left) != stableRuntimeOperationShape(right) {
		t.Fatalf("summary-only change must not create a new shape")
	}
	// args change must create a new shape
	args2 := []string{"test", "./pkg"}
	changed := RuntimeOperation{
		ID: "op-3", Kind: StepKindShellCommand, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "command", String: &cmd},
			{Name: "args", StringList: args2},
			{Name: "summary", String: &summaryA},
		},
	}
	if stableRuntimeOperationShape(left) == stableRuntimeOperationShape(changed) {
		t.Fatalf("args change should create a new shape")
	}
}

func TestStableRuntimeOperationShapeIncludesApprovalAndDependencies(t *testing.T) {
	query := "runtime contract"
	approved := "approval-approved"
	left := RuntimeOperation{
		ID: "web-1", Kind: StepKindWebSearch, DependsOn: []string{"prepare-1"},
		Parameters: []OperationParameter{{Name: "query", String: &query}, {Name: "approvalId", String: &approved}},
	}
	otherApproval := "approval-new"
	approvalChanged := left
	approvalChanged.ID = "web-2"
	approvalChanged.Parameters = []OperationParameter{{Name: "query", String: &query}, {Name: "approvalId", String: &otherApproval}}
	if stableRuntimeOperationShape(left) == stableRuntimeOperationShape(approvalChanged) {
		t.Fatal("approval change must create a new operation shape")
	}

	dependencyChanged := left
	dependencyChanged.ID = "web-3"
	dependencyChanged.DependsOn = []string{"prepare-2"}
	if stableRuntimeOperationShape(left) == stableRuntimeOperationShape(dependencyChanged) {
		t.Fatal("dependency graph change must create a new operation shape")
	}
}

func TestValidateCandidateAgainstRejectedShapesDoesNotClearOnUnboundEvidenceBeforeLastFocus(t *testing.T) {
	path := "main.go"
	patchType := "file_write"
	content := "package main\n"
	op := RuntimeOperation{
		ID: "op-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{path},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters:          []OperationParameter{{Name: "targetRef", String: &path}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}},
	}
	facts := RuntimeFacts{
		Activation:  RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-2",
		Focus:            CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}},
		RemainingFocuses: []CoverageFocus{{ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}}},
		Evidence: EvidenceLedger{
			Rejected: []EvidenceFact{{ID: "rej-1", Kind: EvidenceKindStateChange, DispatchID: "dispatch-1", OperationID: "op-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000001"}},
			Accepted: []EvidenceFact{{ID: "acc-unbound", Kind: EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002"}},
		},
	}.Normalized()
	dispatches := []RuntimeDispatchRecord{{ID: "dispatch-1", Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: op}}}}}}
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: op}}}}
	if err := ValidateCandidateAgainstRejectedShapes(facts, dispatches, candidate); err == nil || !strings.Contains(err.Error(), "replays active rejected operation shape") {
		t.Fatalf("unbound evidence before the last focus must not clear the rejected shape, got %v", err)
	}
}

func TestStableRuntimeOperationShapeIncludesOldText(t *testing.T) {
	path := "cmd/proxy/main.go"
	patchType := "text_replace"
	oldA := "a"
	oldB := "b"
	newText := "c"
	left := RuntimeOperation{
		ID: "op-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "path", String: &path}, {Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldA}, {Name: "newText", String: &newText},
		},
	}
	right := RuntimeOperation{
		ID: "op-2", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{path},
		Parameters: []OperationParameter{
			{Name: "targetRef", String: &path}, {Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldB}, {Name: "newText", String: &newText},
		},
	}
	if stableRuntimeOperationShape(left) == stableRuntimeOperationShape(right) {
		t.Fatalf("different oldText should produce different shapes")
	}
}

func TestValidateCandidateAgainstMissingValidationGap(t *testing.T) {
	path := "cmd/proxy/main.go"
	patchType := "text_replace"
	oldText := "package main"
	newText := "package proxy"
	facts := RuntimeFacts{
		Activation:  RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-3",
		Focus: CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation},
		},
		Evidence: EvidenceLedger{
			Accepted: []EvidenceFact{{
				ID: "acc", Kind: EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				Revision: "evidence-v2-00000000000000000001",
			}},
			Rejected: []EvidenceFact{{
				ID: "rej", Kind: EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				Revision: "evidence-v2-00000000000000000002", FailureCode: "conflict",
			}},
		},
		Failure: &FailureFact{Code: "conflict", Message: "conflict"},
	}.Normalized()
	decision := RuntimeDecision{Action: RuntimeActionProduceStateChange, Focus: facts.Focus}
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{
		Operation: RuntimeOperation{
			ID: "op-1", Kind: RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{path},
			Parameters: []OperationParameter{
				{Name: "targetRef", String: &path}, {Name: "patchType", String: &patchType},
				{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
			},
		},
	}}}}
	if err := ValidateCandidateAgainstMissingValidationGap(facts, decision, candidate); err == nil || !strings.Contains(err.Error(), "only validation evidence is missing") {
		t.Fatalf("expected validation-gap rejection, got %v", err)
	}
}

func TestStableRuntimeOperationShapeIncludesWebFilters(t *testing.T) {
	query := "workorch runtime"
	providerA := "provider-a"
	providerB := "provider-b"
	left := RuntimeOperation{
		ID: "web-1", Kind: StepKindWebSearch,
		Parameters: []OperationParameter{
			{Name: "query", String: &query},
			{Name: "providerId", String: &providerA},
		},
	}
	right := RuntimeOperation{
		ID: "web-2", Kind: StepKindWebSearch,
		Parameters: []OperationParameter{
			{Name: "query", String: &query},
			{Name: "providerId", String: &providerB},
		},
	}
	if stableRuntimeOperationShape(left) == stableRuntimeOperationShape(right) {
		t.Fatalf("providerId change must create a new web search shape")
	}
}

func TestStableRuntimeOperationShapeIncludesFileListRecursive(t *testing.T) {
	pathValue := "internal"
	recTrue := true
	recFalse := false
	left := RuntimeOperation{
		ID: "list-1", Kind: StepKindFileList,
		Parameters: []OperationParameter{
			{Name: "path", String: &pathValue},
			{Name: "recursive", Boolean: &recTrue},
		},
	}
	right := RuntimeOperation{
		ID: "list-2", Kind: StepKindFileList,
		Parameters: []OperationParameter{
			{Name: "path", String: &pathValue},
			{Name: "recursive", Boolean: &recFalse},
		},
	}
	if stableRuntimeOperationShape(left) == stableRuntimeOperationShape(right) {
		t.Fatalf("recursive change must create a new file/list shape")
	}
}
