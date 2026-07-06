package orchestrator

import (
	"testing"

	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
)

func TestRuntimeStepKindRegistrationMatrixMatchesCurrentRuntimeExecutors(t *testing.T) {
	items := runtimeStepKindRegistrations()
	seen := map[string]runtimeStepKindRegistration{}
	for _, item := range items {
		if item.StepKind == "" || item.RuntimeOwner == "" || item.ExecutorModule == "" || item.Responsibility == "" {
			t.Fatalf("registration item has empty field: %+v", item)
		}
		if previous, ok := seen[item.StepKind]; ok {
			t.Fatalf("duplicate registration for %q: %+v and %+v", item.StepKind, previous, item)
		}
		seen[item.StepKind] = item
	}

	wantOwners := map[string]string{
		llmcommon.StepKindWorkspaceExecution:   llmcommon.ModuleID,
		mcpcommon.StepKindToolCall:             mcpcommon.ModuleID,
		mcpcommon.StepKindResourceRead:         mcpcommon.ModuleID,
		mcpcommon.StepKindPromptGet:            mcpcommon.ModuleID,
		shellcommon.StepKindCommand:            shellcommon.ModuleID,
		filecommon.StepKindList:                filecommon.ModuleID,
		filecommon.StepKindRead:                filecommon.ModuleID,
		filecommon.StepKindEdit:                filecommon.ModuleID,
		llmcommon.RuntimeBlockKindPatchApply:   ModuleID,
		llmcommon.RuntimeBlockKindPatchPreview: ModuleID,
		llmcommon.RuntimeBlockKindPatchRevert:  ModuleID,
		gitcommon.StepKindRead:                 gitcommon.ModuleID,
		gitcommon.StepKindWrite:                gitcommon.ModuleID,
		webcommon.StepKindSearch:               webcommon.ModuleID,
		webcommon.StepKindFetch:                webcommon.ModuleID,
		sshcommon.StepKindCommand:              sshcommon.ModuleID,
	}
	if len(seen) != len(wantOwners) {
		t.Fatalf("registered kind count = %d, want %d: %+v", len(seen), len(wantOwners), seen)
	}
	for stepKind, owner := range wantOwners {
		item, ok := seen[stepKind]
		if !ok {
			t.Fatalf("missing registration for %q", stepKind)
		}
		if item.RuntimeOwner != owner {
			t.Fatalf("registration %q owner = %q, want %q", stepKind, item.RuntimeOwner, owner)
		}
	}
	for _, stepKind := range []string{
		llmcommon.RuntimeBlockKindPatchApply,
		llmcommon.RuntimeBlockKindPatchPreview,
		llmcommon.RuntimeBlockKindPatchRevert,
	} {
		if seen[stepKind].ExecutorModule != filecommon.ModuleID {
			t.Fatalf("registration %q executor = %q, want %q", stepKind, seen[stepKind].ExecutorModule, filecommon.ModuleID)
		}
	}
	if _, ok := seen["llm/assessment_only"]; ok {
		t.Fatalf("llm/assessment_only must remain a mode of %s, not a separately registered step kind", llmcommon.StepKindWorkspaceExecution)
	}
}
