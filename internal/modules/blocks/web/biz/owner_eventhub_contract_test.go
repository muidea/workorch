package web

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWebOwnerAdmissibilityUsesTypedEventHubAndDoesNotAdvertiseCoverageProof(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
		return "provider-a", map[string]webmodels.ProviderConfig{
			"provider-a": {ID: "provider-a", SearchEndpoint: "https://provider.invalid/search"},
		}, nil
	}, func() uint64 { return 1 })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	command := webevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-web-eventhub", RunID: "run-web-eventhub", AuthoritySnapshotID: "authority-web-eventhub",
		PrerequisiteID: "admissibility-web-eventhub", OwnerModuleID: ModuleID,
		WorkspaceDeliverableDirectory: t.TempDir(),
		RequiredKinds:                 []string{runtimecontract.StepKindWebSearch, runtimecontract.StepKindWebFetch}, MaxItems: 8,
	}
	data, err := hub.Send(magicevent.NewEventWithContext(
		webevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	)).Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(webevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok {
		t.Fatalf("Web admissibility result type = %T", data)
	}
	acquisition := result.Acquisition
	if acquisition.WorkspaceID != command.WorkspaceID || acquisition.RunID != command.RunID || acquisition.AuthoritySnapshotID != command.AuthoritySnapshotID ||
		acquisition.Admissibility.OwnerModuleID != ModuleID || len(acquisition.Admissibility.OperationKinds) != 2 || len(acquisition.Admissibility.SelectorRefs) != 1 || acquisition.Admissibility.SelectorRefs[0] != "provider-a" {
		t.Fatalf("Web admissibility acquisition = %+v", acquisition)
	}
	if err := acquisition.Admissibility.Validate(); err != nil {
		t.Fatalf("Web admissibility: %v", err)
	}
	if err := acquisition.Observation.Validate(); err != nil {
		t.Fatalf("Web admissibility observation: %v", err)
	}
	if len(acquisition.Admissibility.ValidationRecipeRefs) != 0 || len(acquisition.Admissibility.ValidationSurfaces) != 0 {
		t.Fatalf("Web catalog advertised validation proof: %+v", acquisition.Admissibility)
	}
	for _, kind := range []string{runtimecontract.StepKindWebSearch, runtimecontract.StepKindWebFetch} {
		for _, evidenceKind := range []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation} {
			if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
				t.Fatalf("Web operation %q unexpectedly supports %q coverage proof", kind, evidenceKind)
			}
		}
	}
}
