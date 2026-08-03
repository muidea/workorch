package ssh

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	authorization, err := runtimecontract.RuntimeOperationAuthorizationIdentity(kind)
	if err != nil {
		t.Fatalf("runtime operation authorization identity for %q: %v", kind, err)
	}
	return authorization
}

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
	if response.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || response.Stale == nil || response.Feedback != nil || response.Stale.ExpectedRevision != inventory.ObservedRevision || response.Stale.ObservedRevision != adapter.admissibilityRevision() {
		t.Fatalf("policy drift response = %+v, want typed stale before probe", response)
	}
}

func TestSSHSemanticProofProbeReturnsTypedStaleWithoutNetworkOperation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	adapter := NewRuntimeAdapterWithTimeout(5 * time.Second)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-semantic-ssh", RunID: "run-semantic-ssh", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "remote state", Order: 1}},
		AllowedOwnerModuleIDs:     []string{ModuleID}, AllowedOperationKinds: []string{StepKindCommand},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: ModuleID, OperationKind: StepKindCommand,
		Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"remote-host"}},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectRemoteDurable}, DeclarationRef: "semantic/dod-1",
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("semantic-ssh", ModuleID, []string{StepKindCommand}, []string{"remote-host"}, nil, []string{"remote-host-parameter", "owner-execution-policy"}, adapter.admissibilityRevision())
	if err != nil {
		t.Fatal(err)
	}
	manifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	adapter.timeout = 7 * time.Second
	result := hub.Send(magicevent.NewEventWithContext(
		sshevents.TopicProbeRuntimeSemanticProofFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		sshevents.ProbeRuntimeSemanticProofFeasibilityCommand{
			WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID, FeasibilityBatchID: "batch-1", WorkspaceDeliverableDirectory: t.TempDir(),
			Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog,
		},
	))
	data, getErr := result.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	response, ok := data.(sshevents.ProbeRuntimeSemanticProofFeasibilityResult)
	if !ok || response.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityStale || response.Observation.OriginalDiagnostic == nil ||
		response.Observation.OriginalDiagnostic.ExpectedRevision != catalog.ObservedRevision || response.Observation.OriginalDiagnostic.ObservedRevision != adapter.admissibilityRevision() {
		t.Fatalf("semantic stale response = %+v", data)
	}
}

func sendSSHFeasibilityProbe(t *testing.T, ctx context.Context, hub magicevent.Hub, inventory runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation) sshevents.ProbeRuntimeOperationFeasibilityResult {
	t.Helper()
	host := "example.invalid"
	username := "operator"
	password := "secret"
	command := "uname -a"
	result := hub.Send(magicevent.NewEventWithContext(
		sshevents.TopicProbeRuntimeOperationFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		sshevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: "ws-ssh-freshness", RunID: "run-ssh-freshness",
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-ssh-freshness"},
			WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, Observation: &observation,
			CandidateManifestHash: "manifest-ssh-freshness", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, StepKindCommand),
			Operation: runtimecontract.RuntimeOperation{ID: "ssh-command", Kind: StepKindCommand, Parameters: []runtimecontract.OperationParameter{
				{Name: "host", String: &host}, {Name: "username", String: &username}, {Name: "password", String: &password}, {Name: "command", String: &command},
			}},
		},
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(sshevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok {
		t.Fatalf("probe response type = %T", data)
	}
	return response
}
