package shell

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellOwnerFeasibilityAndSemanticProofHappyPathsUseTypedEventHub(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := t.TempDir()
	targetRef := "target.txt"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("unchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	acquire := shellevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-shell-eventhub", RunID: "run-shell-eventhub", AuthoritySnapshotID: "authority-shell-eventhub",
		PrerequisiteID: "admissibility-shell-eventhub", OwnerModuleID: ModuleID,
		WorkspaceDeliverableDirectory: workspace, RequiredKinds: []string{runtimecontract.StepKindShellCommand},
		RequiredTargetRefs: []string{targetRef}, MaxItems: 8,
	}
	data, err := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, acquire,
	)).Get()
	if err != nil {
		t.Fatal(err)
	}
	acquisition, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok {
		t.Fatalf("admissibility result type = %T", data)
	}
	catalog := acquisition.Acquisition.Admissibility
	observation := acquisition.Acquisition.Observation

	commandName := "sh"
	stateChanging := true
	operation := runtimecontract.RuntimeOperation{
		ID: "shell-eventhub-probe", Kind: runtimecontract.StepKindShellCommand, TargetRefs: []string{targetRef},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "command", String: &commandName},
			{Name: "args", StringList: []string{"-c", "true"}},
			{Name: "stateChanging", Boolean: &stateChanging},
		},
	}
	authorization := runtimecontract.RuntimeCandidateOperationManifest{
		Kind: runtimecontract.StepKindShellCommand, OwnerModuleID: ModuleID,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{targetRef},
		},
		AdmissibilitySnapshotRefs: []string{catalog.SnapshotID},
	}
	feasibilityData, err := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, shellevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: acquire.WorkspaceID, RunID: acquire.RunID,
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: acquire.AuthoritySnapshotID},
			WorkspaceDeliverableDirectory: workspace, Operation: operation, Admissibility: catalog,
			CandidateManifestHash: "manifest-shell-eventhub", OperationAuthorization: authorization, Observation: &observation,
		},
	)).Get()
	if err != nil {
		t.Fatal(err)
	}
	feasibility, ok := feasibilityData.(shellevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok || feasibility.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || feasibility.Feedback != nil || feasibility.Stale != nil {
		t.Fatalf("Shell operation feasibility = %#v", feasibilityData)
	}

	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-shell-eventhub", OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindShellCommand,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{targetRef},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "dod/cwi-shell-eventhub",
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   acquire.WorkspaceID, RunID: acquire.RunID,
		DefinitionRevision: "definition-shell-eventhub", DefinitionSeedHash: "seed-shell-eventhub",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: "cwi-shell-eventhub", SliceID: "slice-shell-eventhub", SourceScope: "dodItem", SourceID: "dod-shell-eventhub", TargetText: "update target", Order: 1,
		}},
		AllowedOwnerModuleIDs: []string{ModuleID}, AllowedOperationKinds: []string{runtimecontract.StepKindShellCommand},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-shell-eventhub", DeclarationRef: proposal.DeclarationRef,
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	semanticData, err := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicProbeRuntimeSemanticProofFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, shellevents.ProbeRuntimeSemanticProofFeasibilityCommand(runtimecontract.RuntimeSemanticProofFeasibilityRequest{
			WorkspaceID: acquire.WorkspaceID, RunID: acquire.RunID, FeasibilityBatchID: "batch-shell-eventhub",
			WorkspaceDeliverableDirectory: workspace, Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog,
		}),
	)).Get()
	if err != nil {
		t.Fatal(err)
	}
	semantic, ok := semanticData.(shellevents.ProbeRuntimeSemanticProofFeasibilityResult)
	if !ok || semantic.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible || semantic.Observation.OriginalDiagnostic != nil || semantic.Observation.ObservedRevision != catalog.ObservedRevision {
		t.Fatalf("Shell semantic feasibility = %#v", semanticData)
	}
}
