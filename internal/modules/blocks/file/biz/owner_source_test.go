package file

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type fileCountingDataEvent struct {
	magicevent.Event
	reads atomic.Int32
}

func (e *fileCountingDataEvent) Data() any {
	e.reads.Add(1)
	return struct{}{}
}

func TestFileRuntimeOwnerCommandsRejectUntrustedSources(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	tests := []struct {
		topic   string
		payload any
		handler func(magicevent.Event, magicevent.Result)
	}{
		{fileevents.TopicAcquireRuntimeCapabilities, fileevents.AcquireRuntimeCapabilitiesCommand{}, owner.handleRuntimeCapabilities},
		{fileevents.TopicAcquireRuntimeOperationAdmissibility, fileevents.AcquireRuntimeOperationAdmissibilityCommand{}, owner.handleRuntimeOperationAdmissibility},
		{fileevents.TopicValidateRuntimeOperationCandidateConformance, fileevents.ValidateRuntimeOperationCandidateConformanceCommand{}, owner.handleRuntimeOperationCandidateConformance},
		{fileevents.TopicProbeRuntimeOperationFeasibility, fileevents.ProbeRuntimeOperationFeasibilityCommand{}, owner.handleRuntimeOperationFeasibility},
		{fileevents.TopicProbeRuntimeSemanticProofFeasibility, fileevents.ProbeRuntimeSemanticProofFeasibilityCommand{}, owner.handleRuntimeSemanticProofFeasibility},
		{fileevents.TopicQueryRuntimeFileReadDependencies, fileevents.QueryRuntimeFileReadDependenciesCommand{}, owner.handleRuntimeFileReadDependenciesQuery},
		{fileevents.TopicQueryRuntimeCWIReconciliation, fileevents.QueryRuntimeCWIReconciliationCommand{}, owner.handleRuntimeCWIReconciliationQuery},
		{fileevents.TopicAcquireTargetSnapshot, fileevents.AcquireTargetSnapshotCommand{}, owner.handleAcquireTargetSnapshot},
		{fileevents.TopicAcquireRuntimeTargetSelection, fileevents.AcquireRuntimeTargetSelectionCommand{}, owner.handleAcquireRuntimeTargetSelection},
		{fileevents.TopicAcquireWorkspaceManifest, fileevents.AcquireWorkspaceManifestCommand{}, owner.handleAcquireWorkspaceManifest},
		{fileevents.TopicConfirmRuntimeRepairSurface, fileevents.ConfirmRuntimeRepairSurfaceCommand{}, owner.handleConfirmRuntimeRepairSurface},
	}
	for _, testCase := range tests {
		t.Run(testCase.topic, func(t *testing.T) {
			for _, event := range []magicevent.Event{
				nil,
				magicevent.NewEventWithContext(testCase.topic, "application/workorchd", ModuleID, magicevent.NewHeader(), ctx, testCase.payload),
				magicevent.NewEventWithContext(testCase.topic, " kernel/orchestrator ", ModuleID, magicevent.NewHeader(), ctx, testCase.payload),
			} {
				result := magicevent.NewResult(testCase.topic, "untrusted", ModuleID)
				testCase.handler(event, result)
				_, err := result.Get()
				if err == nil || (!strings.Contains(err.Error(), "exact Orchestrator source") && !strings.Contains(err.Error(), "exact Orchestrator or Git owner source")) {
					t.Fatalf("source error = %v", err)
				}
			}
		})
	}
}

func TestFileWorkspaceManifestAllowsGitOwnerSource(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	command := fileevents.AcquireWorkspaceManifestCommand{
		WorkspaceID: "workspace-git-manifest", RunID: "run-git-manifest", AuthoritySnapshotID: "authority-git-manifest",
		PrerequisiteID: "prerequisite-git-manifest", WorkspaceDeliverableDirectory: t.TempDir(),
	}
	result := magicevent.NewResult(fileevents.TopicAcquireWorkspaceManifest, runtimecontract.RuntimeModuleGit, ModuleID)
	owner.handleAcquireWorkspaceManifest(magicevent.NewEventWithContext(
		fileevents.TopicAcquireWorkspaceManifest, runtimecontract.RuntimeModuleGit, ModuleID, magicevent.NewHeader(), ctx, command,
	), result)
	data, err := result.Get()
	if err != nil {
		t.Fatalf("Git workspace manifest query: %v", err)
	}
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID || response.Manifest.Revision == "" {
		t.Fatalf("Git workspace manifest response = %#v", data)
	}
}

func TestFileOwnerStartIsIdempotentAndTeardownClearsLifecycle(t *testing.T) {
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
	if !owner.started {
		t.Fatal("owner did not enter started lifecycle")
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	owner.Teardown(ctx)
	if owner.started || owner.runtimeAdapter != nil {
		t.Fatalf("owner teardown retained lifecycle resources: started=%t adapter=%v", owner.started, owner.runtimeAdapter)
	}
}

func TestFileOwnerWrongDTOAndLifecycleMatrixCoversAllTopics(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	topics := []struct {
		topic       string
		destination string
	}{
		{fileevents.TopicAcquireRuntimeCapabilities, ModuleID},
		{fileevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID},
		{fileevents.TopicValidateRuntimeOperationCandidateConformance, ModuleID},
		{fileevents.TopicProbeRuntimeOperationFeasibility, ModuleID},
		{fileevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID},
		{fileevents.TopicQueryRuntimeCWIReconciliation, ModuleID},
		{fileevents.TopicQueryRuntimeFileReadDependencies, ModuleID},
		{fileevents.TopicAcquireTargetSnapshot, fileevents.PrerequisiteCommandDestination},
		{fileevents.TopicAcquireAuthoringResources, ModuleID},
		{fileevents.TopicVerifyAuthoringResource, ModuleID},
		{fileevents.TopicObservePreparationDirectories, ModuleID},
		{fileevents.TopicObservePreparationResources, ModuleID},
		{fileevents.TopicObservePreparationManagedConfigs, ModuleID},
		{fileevents.TopicReobservePreparationFiles, ModuleID},
		{fileevents.TopicQueryPreparationEpoch, ModuleID},
		{fileevents.TopicAcquireRuntimeTargetSelection, fileevents.PrerequisiteCommandDestination},
		{fileevents.TopicAcquireWorkspaceManifest, fileevents.PrerequisiteCommandDestination},
		{fileevents.TopicConfirmRuntimeRepairSurface, fileevents.PrerequisiteCommandDestination},
		{fileevents.TopicExecuteFileListOperation, ModuleID},
		{fileevents.TopicExecuteFileReadOperation, ModuleID},
		{fileevents.TopicExecuteFileEditOperation, ModuleID},
		{fileevents.TopicExecuteFileWriteOperation, ModuleID},
		{fileevents.TopicExecutePatchApplyOperation, ModuleID},
		{fileevents.TopicExecutePatchPreviewOperation, ModuleID},
	}
	if len(topics) != len(fileevents.Contracts()) {
		t.Fatalf("DTO/lifecycle matrix topic count=%d contracts=%d", len(topics), len(fileevents.Contracts()))
	}
	for _, item := range topics {
		source := runtimecontract.RuntimeModuleOrchestrator
		if item.topic == fileevents.TopicAcquireAuthoringResources {
			source = authoringApplicationSource
		}
		if item.topic == fileevents.TopicVerifyAuthoringResource || item.topic == fileevents.TopicObservePreparationDirectories || item.topic == fileevents.TopicObservePreparationResources || item.topic == fileevents.TopicObservePreparationManagedConfigs || item.topic == fileevents.TopicReobservePreparationFiles || item.topic == fileevents.TopicQueryPreparationEpoch {
			source = "blocks/workspace"
		}
		event := &fileCountingDataEvent{Event: magicevent.NewEventWithContext(
			item.topic, source, item.destination, magicevent.NewHeader(), ctx, nil,
		)}
		result := hub.Send(event)
		if result == nil || result.Error() == nil {
			t.Fatalf("started topic %s accepted wrong concrete command: result=%#v", item.topic, result)
		}
		if got := event.reads.Load(); got != 1 {
			t.Fatalf("started topic %s payload reads = %d, want exactly 1 after duplicate Start", item.topic, got)
		}
	}
	owner.Teardown(ctx)
	for _, item := range topics {
		source := runtimecontract.RuntimeModuleOrchestrator
		if item.topic == fileevents.TopicAcquireAuthoringResources {
			source = authoringApplicationSource
		}
		if item.topic == fileevents.TopicVerifyAuthoringResource || item.topic == fileevents.TopicObservePreparationDirectories || item.topic == fileevents.TopicObservePreparationResources || item.topic == fileevents.TopicObservePreparationManagedConfigs || item.topic == fileevents.TopicReobservePreparationFiles || item.topic == fileevents.TopicQueryPreparationEpoch {
			source = "blocks/workspace"
		}
		event := &fileCountingDataEvent{Event: magicevent.NewEventWithContext(
			item.topic, source, item.destination, magicevent.NewHeader(), ctx, nil,
		)}
		hub.Send(event)
		if got := event.reads.Load(); got != 0 {
			t.Fatalf("post-Stop topic %s payload reads = %d, want 0", item.topic, got)
		}
	}
}
