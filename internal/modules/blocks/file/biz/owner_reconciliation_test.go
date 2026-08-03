package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileOwnerReconciliationReportsMissingTargetWithoutInferringGoalSemantics(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := t.TempDir()
	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    common.CoverageWorkItemID("cwi-file"),
		BindingID:             "binding-file-missing",
		BindingHash:           "binding-hash-file-missing",
		OwnerModuleIDs:        []string{modulecommon.ModuleID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		TargetRefs:            []string{"declared.go"},
	})
	result := sendFileReconciliationQuery(t, ctx, hub, command)
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied {
		t.Fatalf("missing target status = %q, want unsatisfied", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "target_missing" {
		t.Fatalf("missing target diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
}

func TestFileOwnerReconciliationRejectsUnmappedArtifactReference(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    common.CoverageWorkItemID("cwi-file-artifact"),
		BindingID:             "binding-file-artifact",
		BindingHash:           "binding-hash-file-artifact",
		OwnerModuleIDs:        []string{modulecommon.ModuleID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileRead},
		ArtifactRefs:          []string{"workorch://workspace/main.go"},
	})
	result := sendFileReconciliationQuery(t, ctx, hub, command)
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
		t.Fatalf("unmapped artifact status = %q, want unverifiable", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "artifact_scope_unverifiable" {
		t.Fatalf("unmapped artifact diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
}

func fileReconciliationQueryCommand(workspace string, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileevents.QueryRuntimeCWIReconciliationCommand {
	return fileevents.QueryRuntimeCWIReconciliationCommand{
		WorkspaceID:                   "ws-file-reconciliation",
		RunID:                         "run-file-reconciliation",
		RequestID:                     "request-file-reconciliation",
		Generation:                    1,
		DefinitionRevision:            "definition-file-reconciliation",
		BindingSetHash:                "binding-set-file-reconciliation",
		OwnerModuleID:                 modulecommon.ModuleID,
		Binding:                       binding,
		WorkspaceDeliverableDirectory: workspace,
		WorkspaceBaselineRevision:     "tree-file-reconciliation",
		WorkspaceBaselineRef:          "workspace-tree/tree-file-reconciliation",
	}
}

func sendFileReconciliationQuery(t *testing.T, ctx context.Context, hub magicevent.Hub, command fileevents.QueryRuntimeCWIReconciliationCommand) fileevents.QueryRuntimeCWIReconciliationResult {
	t.Helper()
	response := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(fileevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	return result
}
