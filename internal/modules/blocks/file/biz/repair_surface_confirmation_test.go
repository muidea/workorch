package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileOwnerConfirmsOnlyCurrentRegularRepairSurface(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "manager.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initialManifest, acquireErr := owner.runtimeAdapter.AcquireWorkspaceManifest(workspace)
	if acquireErr != nil {
		t.Fatal(acquireErr)
	}
	observation := runtimecontract.RuntimeValidationFailureObservation{WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate", FailureSignature: "build-failure", TargetRole: "implementation", DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed, CandidateTargetRefs: []string{"internal/manager.go"}, ValidationScopeTargetRefs: []string{"internal/manager.go"}, DiagnosticHash: strings.Repeat("a", 64)}
	command := fileevents.ConfirmRuntimeRepairSurfaceCommand{RequestID: "repair-request-1", IssuedAtStateRevision: 7, ExpectedSubjectRevision: initialManifest.Revision, WorkspaceID: "ws", RunID: "run", WorkspaceDeliverableDirectory: workspace, Observation: observation}
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || response.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceConfirmed || response.Confirmation.OwnerModuleID != ModuleID ||
		response.Confirmation.RequestID != command.RequestID || response.Confirmation.IssuedAtStateRevision != command.IssuedAtStateRevision ||
		response.Confirmation.ExpectedSubjectRevision != command.ExpectedSubjectRevision || response.Confirmation.ObservedRevision != command.ExpectedSubjectRevision {
		t.Fatalf("confirmation = %#v", data)
	}
	missingFence := command
	missingFence.RequestID = ""
	result = hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, missingFence))
	if _, err := result.Get(); err == nil {
		t.Fatal("File owner accepted repair-surface query without request fence")
	}
	command.Observation.CandidateTargetRefs = []string{"missing.go"}
	command.Observation.ValidationScopeTargetRefs = []string{"missing.go"}
	command.Observation.DiagnosticHash = strings.Repeat("b", 64)
	result = hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	data, err = result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok = data.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || response.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceRejected || len(response.Confirmation.ConfirmedTargetRefs) != 0 {
		t.Fatalf("missing target confirmation = %#v", data)
	}
	if err := os.WriteFile(filepath.Join(workspace, "new.go"), []byte("package workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result = hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	data, err = result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok = data.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || response.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceStale ||
		response.Confirmation.ExpectedSubjectRevision != initialManifest.Revision ||
		response.Confirmation.ObservedRevision == initialManifest.Revision || len(response.Confirmation.ConfirmedTargetRefs) != 0 {
		t.Fatalf("workspace revision drift confirmation = %#v", data)
	}
}
