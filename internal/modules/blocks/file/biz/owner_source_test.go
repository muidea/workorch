package file

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
		{fileevents.TopicProbeRuntimeOperationFeasibility, fileevents.ProbeRuntimeOperationFeasibilityCommand{}, owner.handleRuntimeOperationFeasibility},
		{fileevents.TopicProbeRuntimeSemanticProofFeasibility, fileevents.ProbeRuntimeSemanticProofFeasibilityCommand{}, owner.handleRuntimeSemanticProofFeasibility},
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
