package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func TestFileOwnerConfirmsWholeBoundedFailureUnionAndReloadedFence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	for _, n := range []int{8, 9, 16, 17} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			workspace := t.TempDir()
			refs := make([]string, n)
			for i := range refs {
				refs[i] = fmt.Sprintf("part-%02d.go", i)
				if err := os.WriteFile(filepath.Join(workspace, refs[i]), []byte("package fixture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			manifest, err := owner.runtimeAdapter.AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			observation := runtimecontract.RuntimeValidationFailureObservation{WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate", FailureSignature: "failure", TargetRole: "implementation", DiagnosticClass: runtimecontract.ValidationDiagnosticTestFailed, CandidateTargetRefs: refs, ValidationScopeTargetRefs: refs, DiagnosticHash: strings.Repeat("a", 64)}
			command := fileevents.ConfirmRuntimeRepairSurfaceCommand{RequestID: "request", IssuedAtStateRevision: 7, ExpectedSubjectRevision: manifest.Revision, WorkspaceID: "ws", RunID: "run", WorkspaceDeliverableDirectory: workspace, Observation: observation}
			query := func(c fileevents.ConfirmRuntimeRepairSurfaceCommand) (fileevents.ConfirmRuntimeRepairSurfaceResult, error) {
				result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, c))
				data, e := result.Get()
				if e != nil {
					return fileevents.ConfirmRuntimeRepairSurfaceResult{}, e
				}
				v, ok := data.(fileevents.ConfirmRuntimeRepairSurfaceResult)
				if !ok {
					t.Fatalf("unexpected response %T", data)
				}
				return v, nil
			}
			first, err := query(command)
			if n > runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs {
				if err == nil {
					t.Fatal("overflow accepted")
				}
				return
			}
			if err != nil || first.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceConfirmed || !reflect.DeepEqual(first.Confirmation.ConfirmedTargetRefs, refs) {
				t.Fatal("lost complete union", err)
			}
			bytes, _ := json.Marshal(command)
			var reloaded fileevents.ConfirmRuntimeRepairSurfaceCommand
			if err := json.Unmarshal(bytes, &reloaded); err != nil {
				t.Fatal(err)
			}
			second, err := query(reloaded)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatal("read-only duplicate/reload changed fact", err)
			}
			if err := os.WriteFile(filepath.Join(workspace, refs[0]), []byte("package fixture\n//new\n"), 0600); err != nil {
				t.Fatal(err)
			}
			stale, err := query(reloaded)
			if err != nil || stale.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceStale || len(stale.Confirmation.ConfirmedTargetRefs) != 0 {
				t.Fatal("stale scope granted authority", err)
			}
			forged := reloaded
			forged.WorkspaceID = "another-workspace"
			if _, err := query(forged); err == nil {
				t.Fatal("cross workspace accepted")
			}
		})
	}
}
