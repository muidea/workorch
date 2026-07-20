package ssh

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSSHAdmissibilityObservationReturnsTypedPolicyStale(t *testing.T) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "admissibility-ssh", OwnerModuleID: ModuleID, OperationKinds: []string{StepKindCommand}, ObservedRevision: "policy-v1"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := sshAdmissibilityObservation(inventory.PrerequisiteID, inventory)
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	stale := sshAdmissibilityStale(&observation, inventory, "policy-v2")
	if stale == nil || stale.Domain != runtimecontract.RuntimeResourceOwnerCatalog || stale.ExpectedRevision != "policy-v1" || stale.ObservedRevision != "policy-v2" {
		t.Fatalf("ssh policy stale = %+v", stale)
	}
}

func TestSSHOwnerProbeReturnsTypedStaleBeforeAnyNetworkOperationWhenPolicyChanges(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	adapter := NewRuntimeAdapterWithTimeout(5 * time.Second)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-ssh", OwnerModuleID: ModuleID, OperationKinds: []string{StepKindCommand}, ObservedRevision: adapter.admissibilityRevision(),
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := sshAdmissibilityObservation(inventory.PrerequisiteID, inventory)
	adapter.timeout = 7 * time.Second // owner-observed policy revision drift

	response := sendSSHFeasibilityProbe(t, ctx, hub, inventory, observation)
	if response.Feasible || response.Stale == nil || response.Failure != nil || response.Stale.ExpectedRevision != inventory.ObservedRevision || response.Stale.ObservedRevision != adapter.admissibilityRevision() {
		t.Fatalf("policy drift response = %+v, want typed stale before probe", response)
	}
}

func sendSSHFeasibilityProbe(t *testing.T, ctx context.Context, hub magicevent.Hub, inventory runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation) orchestratorevents.ProbeRuntimeOperationFeasibilityResult {
	t.Helper()
	host := "example.invalid"
	username := "operator"
	password := "secret"
	command := "uname -a"
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: "ws-ssh-freshness", RunID: "run-ssh-freshness",
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-ssh-freshness"},
			WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, Observation: &observation,
			Operation: runtimecontract.RuntimeOperation{ID: "ssh-command", Kind: StepKindCommand, Parameters: []runtimecontract.OperationParameter{
				{Name: "host", String: &host}, {Name: "username", String: &username}, {Name: "password", String: &password}, {Name: "command", String: &command},
			}},
		},
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok {
		t.Fatalf("probe response type = %T", data)
	}
	return response
}
