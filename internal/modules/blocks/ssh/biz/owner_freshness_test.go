package ssh

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSSHOwnerAdmissibilityPublishesOnlyOpaqueTargetsAndAtMostOncePolicy(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	command := sshevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-ssh", RunID: "run-ssh", AuthoritySnapshotID: "authority-ssh", PrerequisiteID: "admissibility-ssh",
		OwnerModuleID: ModuleID, WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{StepKindCommand}, MaxItems: 4,
	}
	result := hub.Send(magicevent.NewEventWithContext(sshevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(sshevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	inventory := response.Acquisition.Admissibility
	if len(inventory.TargetRefs) != 1 || inventory.TargetRefs[0] != "target-a" || len(inventory.OperationPolicies) != 1 ||
		inventory.OperationPolicies[0].EffectScope != runtimecontract.RuntimeEffectRemoteDurable ||
		inventory.OperationPolicies[0].IdempotencyClass != runtimecontract.RuntimeOperationIdempotencyAtMostOnce || len(inventory.OperationVariants) != 1 ||
		!containsSSHRef(inventory.ConstraintRefs, "ssh-binding/target-a/session-a/credential-a") {
		t.Fatalf("SSH admissibility = %+v", inventory)
	}
	probe := sendSSHFeasibilityProbe(t, ctx, hub, inventory, response.Acquisition.Observation)
	if probe.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || probe.Feedback != nil || probe.Stale != nil {
		t.Fatalf("configured opaque SSH binding probe = %+v, want feasible", probe)
	}
	wire, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, prohibited := range []string{"example.internal", "operator", "test-password"} {
		if strings.Contains(string(wire), prohibited) {
			t.Fatalf("SSH admissibility exposed private owner data %q: %s", prohibited, wire)
		}
	}
}

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !ok {
		t.Fatalf("runtime operation authorization identity has unknown kind %q", kind)
	}
	return runtimecontract.RuntimeCandidateOperationManifest{
		Kind: descriptor.Kind, OwnerModuleID: descriptor.OwnerModuleID,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:            runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerSelectorRefs: []string{"test/owner-feasibility"},
		},
	}.Normalized()
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
	adapter.revision = "ssh-policy-drifted" // simulated next-process owner policy

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
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-1", DeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-1"), RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: ModuleID, OperationKind: StepKindCommand,
		Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"remote-host"}},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectRemoteDurable}, DeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-1"),
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("semantic-ssh", ModuleID, []string{StepKindCommand}, []string{"remote-host"}, nil, []string{"remote-host-parameter", "owner-execution-policy"}, adapter.admissibilityRevision())
	if err != nil {
		t.Fatal(err)
	}
	manifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	adapter.revision = "ssh-policy-drifted"
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
	sessionRef := "session-a"
	credentialRef := "credential-a"
	command := "uname -a"
	result := hub.Send(magicevent.NewEventWithContext(
		sshevents.TopicProbeRuntimeOperationFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		sshevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: "ws-ssh-freshness", RunID: "run-ssh-freshness",
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-ssh-freshness"},
			WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, Observation: &observation,
			CandidateManifestHash: "manifest-ssh-freshness", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, StepKindCommand),
			Operation: runtimecontract.RuntimeOperation{ID: "ssh-command", Kind: StepKindCommand, TargetRefs: []string{"target-a"}, Parameters: []runtimecontract.OperationParameter{
				{Name: "sessionRef", String: &sessionRef}, {Name: "credentialRef", String: &credentialRef}, {Name: "command", String: &command},
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
