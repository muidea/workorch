package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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

func TestFileModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(filebiz.Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestFileModuleStartRequiresEventHub(t *testing.T) {
	module := New()
	if err := module.Configure(filebiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure with empty dependencies err = %v, want invalid argument", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

func TestFileModuleSemanticAndOperationFeasibilityShareExistingTargetPolicy(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	workspace := t.TempDir()
	const targetRef = "existing.txt"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := filebiz.NewRuntimeAdapter()
	workspaceManifest, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"file-catalog", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileWrite}, nil,
		[]string{"workspace-relative-new-file-target"}, []string{"workspace-relative-path"}, workspaceManifest.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"},
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"workspace-relative-new-file-target"},
			ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRefs: []string{targetRef},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-1"),
	}
	semanticManifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "deliver", Order: 1}},
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-1", DeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-1"), RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	semanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(semanticManifest)
	semanticResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicProbeRuntimeSemanticProofFeasibility, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ProbeRuntimeSemanticProofFeasibilityCommand(runtimecontract.RuntimeSemanticProofFeasibilityRequest{
			WorkspaceID: "ws-1", RunID: "run-1", FeasibilityBatchID: "batch-1", WorkspaceDeliverableDirectory: workspace,
			Manifest: semanticManifest, Proposal: proposal, OwnerCatalog: catalog,
		}),
	))
	semanticData, semanticErr := semanticResult.Get()
	if semanticErr != nil {
		t.Fatal(semanticErr)
	}
	semantic := runtimecontract.RuntimeSemanticProofFeasibilityObservation(semanticData.(fileevents.ProbeRuntimeSemanticProofFeasibilityResult))

	authorization := mustRuntimeOperationAuthorizationIdentity(t, runtimecontract.StepKindFileWrite)
	authorization.Target = runtimecontract.RuntimeOperationTargetAuthorization{
		Policy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		OwnerSelectorRefs: []string{"workspace-relative-new-file-target"}, ProspectiveTargetRefs: []string{targetRef},
	}
	authorization = authorization.Normalized()
	content := "new\n"
	operationResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicProbeRuntimeOperationFeasibility, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ProbeRuntimeOperationFeasibilityCommand(runtimecontract.RuntimeOperationFeasibilityRequest{
			WorkspaceID: "ws-1", RunID: "run-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			WorkspaceDeliverableDirectory: workspace, Admissibility: catalog, CandidateManifestHash: "candidate-manifest-1", OperationAuthorization: authorization,
			Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{targetRef}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}}},
		}),
	))
	operationData, operationErr := operationResult.Get()
	if operationErr != nil {
		t.Fatal(operationErr)
	}
	operation := runtimecontract.RuntimeOperationFeasibilityObservation(operationData.(fileevents.ProbeRuntimeOperationFeasibilityResult))
	if semantic.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityInfeasible || semantic.Observation.OriginalDiagnostic == nil ||
		operation.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible || operation.Feedback == nil || operation.Feedback.OriginalDiagnostic == nil ||
		semantic.Observation.OriginalDiagnostic.OriginalCode != "operation_target_exists" || operation.Feedback.OriginalDiagnostic.OriginalCode != "operation_target_exists" ||
		semantic.Observation.OwnerPolicy.PolicyRevision == "" || semantic.Observation.OwnerPolicy != operation.OwnerPolicy ||
		semantic.Observation.OwnerPolicy.CatalogSnapshotID != catalog.SnapshotID ||
		operation.ConstraintImpact != runtimecontract.RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted ||
		operation.Feedback.OriginalDiagnostic.ObservedRevision != catalog.ObservedRevision {
		t.Fatalf("semantic/operation File policy drifted: semantic=%+v operation=%+v", semantic, operation)
	}
}

func TestFileModuleReportsUnifiedDiffTargetApplicabilityAsTypedCandidateConstraint(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	workspace := t.TempDir()
	targetRef := "main.txt"
	current := "one\nchanged\nthree\n"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceManifest, err := filebiz.NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"operation-admissibility:file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{targetRef}, nil, nil, workspaceManifest.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply operation contract is missing")
	}
	authorization := runtimecontract.RuntimeCandidateOperationManifest{
		Kind: contract.Kind, OwnerModuleID: runtimecontract.RuntimeModuleFile, Parameters: contract.Parameters, Variants: contract.Variants,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{targetRef},
		},
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
	}.Normalized()
	patchType := "unified_diff"
	diff := "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+updated\n three\n"
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ProbeRuntimeOperationFeasibilityCommand(runtimecontract.RuntimeOperationFeasibilityRequest{
			WorkspaceID: "ws-1", RunID: "run-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			WorkspaceDeliverableDirectory: workspace, Admissibility: inventory, CandidateManifestHash: "candidate-manifest-1", OperationAuthorization: authorization,
			Operation: runtimecontract.RuntimeOperation{ID: "patch-main", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{targetRef}, Parameters: []runtimecontract.OperationParameter{
				{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff},
			}},
			CurrentTargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: targetRef, Content: current, ContentHash: "target-v2", ContentState: runtimecontract.SnapshotContentStateComplete},
		}),
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	observed := runtimecontract.RuntimeOperationFeasibilityObservation(data.(fileevents.ProbeRuntimeOperationFeasibilityResult))
	if observed.Outcome != runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated || observed.CandidateConstraintViolation == nil ||
		observed.CandidateConstraintViolation.ConstraintRef != "patch/unified_diff/diff/observed_target" ||
		observed.CandidateConstraintViolation.TargetRef != targetRef || observed.CandidateConstraintViolation.ObservedRevision != inventory.ObservedRevision ||
		observed.Feedback != nil || observed.OwnerPolicy.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		t.Fatalf("File candidate applicability observation = %+v", observed)
	}
	content, err := os.ReadFile(filepath.Join(workspace, targetRef))
	if err != nil || string(content) != current {
		t.Fatalf("read-only applicability probe changed target: content=%q err=%v", string(content), err)
	}
}

func TestFileModuleExecutesConcreteFileEditEventAndWritesState(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}
	operation := runtimecontract.RuntimeOperation{
		ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.txt"},
		Parameters: []runtimecontract.OperationParameter{fileStringParameter("oldText", "old"), fileStringParameter("newText", "new")},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileEditOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileEditOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/dispatch-1/edit-1", "dispatch-1", transaction, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
			SealedOperation:                  mustSealFileOperation(t, operation),
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(fileevents.ExecuteOperationResult)
	if response.Receipt.Evidence.ContentHash == "" || len(response.Receipt.Evidence.EvidenceRefs) != 1 {
		t.Fatalf("file/edit response = %+v", response)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("file/edit content = %q err=%v", string(content), err)
	}
}

func TestFileModuleSensitiveFileEditRequiresGovernanceBeforePrivateEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	target := filepath.Join(workspace, ".env")
	if err := os.WriteFile(target, []byte("TOKEN=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	operation := runtimecontract.RuntimeOperation{
		ID: "edit-env", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{".env"},
		Parameters: []runtimecontract.OperationParameter{fileStringParameter("oldText", "old"), fileStringParameter("newText", "new")},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileEditOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileEditOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/dispatch-env/edit-env", "dispatch-env", runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-env"}, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
			SealedOperation:                  mustSealFileOperation(t, operation),
		},
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response := data.(fileevents.ExecuteOperationResult)
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired || response.Receipt.Governance == nil ||
		response.Receipt.Governance.SubjectRef != ".env" || response.Receipt.Governance.RequestedAction != runtimecontract.StepKindFileEdit {
		t.Fatalf("sensitive file/edit receipt = %+v", response.Receipt)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "TOKEN=old\n" {
		t.Fatalf("sensitive file/edit changed content=%q err=%v", content, err)
	}
}

func TestFileModuleRejectsDriftedSealedOperationBeforeEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	target := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	operation := runtimecontract.RuntimeOperation{
		ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.txt"},
		Parameters: []runtimecontract.OperationParameter{fileStringParameter("oldText", "old"), fileStringParameter("newText", "new")},
	}
	sealed := mustSealFileOperation(t, operation)
	drifted := sealed
	different := "forged"
	drifted.Operation.Parameters[1].String = &different
	command := fileevents.ExecuteFileEditOperationCommand{
		RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/dispatch-1/edit-1", "dispatch-1", runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"}, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
		SealedOperation:                  drifted,
	}
	if _, err := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileEditOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command)).Get(); err == nil {
		t.Fatal("drifted sealed payload was accepted")
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "old\n" {
		t.Fatalf("drifted payload changed content=%q err=%v", content, err)
	}

	sealed = mustSealFileOperation(t, operation)
	command.SealedOperation = sealed
	command.TargetRefs = []string{"other.txt"}
	if _, err := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileEditOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command)).Get(); err == nil {
		t.Fatal("sealed target/context mismatch was accepted")
	}
	command.TargetRefs = []string{"main.txt"}
	if _, err := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileEditOperation, "untrusted/source", ModuleID, magicevent.NewHeader(), ctx, command)).Get(); err == nil {
		t.Fatal("untrusted File operation source was accepted")
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "old\n" {
		t.Fatalf("rejected operation changed content=%q err=%v", content, err)
	}
}

func TestFileModuleExecutesConcreteFileWriteEventAndWritesState(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}
	operation := runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"README.md"}, Parameters: []runtimecontract.OperationParameter{fileStringParameter("content", "durable\n")}}
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileWriteOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileWriteOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/dispatch-1/write-1", "dispatch-1", transaction, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
			SealedOperation:                  mustSealFileOperation(t, operation),
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(fileevents.ExecuteOperationResult)
	if response.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("file/write response = %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "durable\n" {
		t.Fatalf("file/write content = %q err=%v", string(content), err)
	}
}

func TestFileModuleFileWriteRejectsExistingTargetWithoutChangingContent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	target := filepath.Join(workspace, "main.go")
	original := []byte("package main\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-existing", RequiredOutputHash: "required-output-existing"}
	operation := runtimecontract.RuntimeOperation{ID: "write-main", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"main.go"}, Parameters: []runtimecontract.OperationParameter{fileStringParameter("content", "# markdown\n")}}
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileWriteOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileWriteOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/dispatch-existing/write-main", "dispatch-existing", transaction, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
			SealedOperation:                  mustSealFileOperation(t, operation),
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(fileevents.ExecuteOperationResult)
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || response.Receipt.Feedback == nil || response.Receipt.Feedback.OriginalDiagnostic == nil || response.Receipt.Feedback.OriginalDiagnostic.OriginalCode != "patch_target_exists" {
		t.Fatalf("file/write existing-target receipt = %+v", response.Receipt)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != string(original) {
		t.Fatalf("existing target changed to %q err=%v", string(content), err)
	}
}

func TestFileOwnerStateChangingReceiptReplaysWithoutSecondWrite(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		if receipt, found := receipts[command.Receipt.ExecutionKey]; found {
			result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt}, nil)
			return
		}
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	command := fileWriteReceiptCommand(t, workspace, "first\n")
	first := sendFileWriteReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !first.Receipt.Changed || first.Receipt.AfterHash == "" {
		t.Fatalf("first receipt = %+v", first.Receipt)
	}
	target := filepath.Join(workspace, "README.md")
	if content, err := os.ReadFile(target); err != nil || string(content) != "first\n" {
		t.Fatalf("first file write content=%q err=%v", content, err)
	}
	if err := os.WriteFile(target, []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	replayed := sendFileWriteReceiptCommand(t, ctx, hub, command)
	if replayed.Receipt.ExecutionKey != first.Receipt.ExecutionKey || replayed.Receipt.AfterHash != first.Receipt.AfterHash {
		t.Fatalf("replayed receipt=%+v, want persisted=%+v", replayed.Receipt, first.Receipt)
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "external\n" {
		t.Fatalf("duplicate receipt replay wrote target content=%q err=%v", content, err)
	}
}

func fileWriteReceiptCommand(t *testing.T, workspace, content string) fileevents.ExecuteFileWriteOperationCommand {
	t.Helper()
	operation := runtimecontract.RuntimeOperation{ID: "write-readme", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"README.md"}, Parameters: []runtimecontract.OperationParameter{fileStringParameter("content", content)}}
	execution := fileTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-file/run-file/dispatch-file/write-readme", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable})
	return fileevents.ExecuteFileWriteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   execution,
			WorkspaceID: "ws-file", RunID: "run-file", DispatchID: "dispatch-file",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-file", RequiredOutputHash: "output-file"},
			OperationID: "write-readme", TargetRefs: []string{"README.md"}, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			WorkspaceDeliverableDirectory: workspace,
		},
		SealedOperation: mustSealFileOperation(t, operation),
	}
}

func sendFileWriteReceiptCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command fileevents.ExecuteFileWriteOperationCommand) fileevents.ExecuteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileWriteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("response type=%T", data)
	}
	return response
}

func TestFileModuleExecutesConcreteInspectionAndPreviewEvents(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", DefinitionRevision: "definition-1", RequiredOutputHash: "required-output-1"}
	listOperation := runtimecontract.RuntimeOperation{ID: "list-1", Kind: runtimecontract.StepKindFileList, Parameters: []runtimecontract.OperationParameter{fileStringParameter("path", "."), fileIntegerParameter("maxEntries", 10)}}

	listResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileListOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileListOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(listOperation, "exec/ws-1/run-1/dispatch-1/list-1", "dispatch-1", transaction, runtimecontract.RuntimeEffectNone, runtimecontract.EvidenceKindValidation, workspace),
			SealedOperation:                  mustSealFileOperation(t, listOperation),
		},
	))
	listData, magicErr := listResult.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	listResponse := listData.(fileevents.ExecuteOperationResult)
	if listResponse.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("file/list response = %+v", listResponse)
	}

	readOperation := runtimecontract.RuntimeOperation{ID: "read-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{"main.txt"}, Parameters: []runtimecontract.OperationParameter{fileIntegerParameter("maxBytes", 4096)}}
	readResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileReadOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecuteFileReadOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(readOperation, "exec/ws-1/run-1/dispatch-1/read-1", "dispatch-1", transaction, runtimecontract.RuntimeEffectNone, runtimecontract.EvidenceKindValidation, workspace),
			SealedOperation:                  mustSealFileOperation(t, readOperation),
		},
	))
	readData, magicErr := readResult.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	readResponse := readData.(fileevents.ExecuteOperationResult)
	if readResponse.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || readResponse.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("file/read response = %+v", readResponse)
	}

	previewOperation := runtimecontract.RuntimeOperation{ID: "preview-1", Kind: runtimecontract.RuntimeBlockKindPatchPreview, TargetRefs: []string{"preview.txt"}, Parameters: []runtimecontract.OperationParameter{fileStringParameter("patchType", "file_write"), fileStringParameter("content", "preview\n")}}
	previewResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecutePatchPreviewOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecutePatchPreviewOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(previewOperation, "exec/ws-1/run-1/dispatch-1/preview-1", "dispatch-1", transaction, runtimecontract.RuntimeEffectNone, runtimecontract.EvidenceKindStateChange, workspace),
			SealedOperation:                  mustSealFileOperation(t, previewOperation),
		},
	))
	previewData, magicErr := previewResult.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	previewResponse := previewData.(fileevents.ExecuteOperationResult)
	if previewResponse.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("patch/preview response = %+v", previewResponse)
	}
	if _, err := os.Stat(filepath.Join(workspace, "preview.txt")); !os.IsNotExist(err) {
		t.Fatalf("patch/preview wrote target or stat failed: %v", err)
	}
}

func TestFileModulePatchFailureKeepsStableSignatureAcrossDispatches(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("current\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}
	execute := func(dispatchID string) fileevents.ExecuteOperationResult {
		operation := runtimecontract.RuntimeOperation{ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.txt"}, Parameters: []runtimecontract.OperationParameter{
			fileStringParameter("patchType", "text_replace"), fileStringParameter("oldText", "missing"), fileStringParameter("newText", "next"),
		}}
		result := hub.Send(magicevent.NewEventWithContext(
			fileevents.TopicExecutePatchApplyOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
			fileevents.ExecutePatchApplyOperationCommand{
				RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/"+dispatchID+"/patch-1", dispatchID, transaction, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
				SealedOperation:                  mustSealFileOperation(t, operation),
			},
		))
		data, magicErr := result.Get()
		if magicErr != nil {
			t.Fatal(magicErr)
		}
		return data.(fileevents.ExecuteOperationResult)
	}
	first := execute("dispatch-1")
	second := execute("dispatch-2")
	firstDiagnostic := first.Receipt.Feedback.OriginalDiagnostic
	secondDiagnostic := second.Receipt.Feedback.OriginalDiagnostic
	if firstDiagnostic == nil || secondDiagnostic == nil ||
		firstDiagnostic.OriginalCode != "patch_text_replace_conflict" || firstDiagnostic.Signature == "" || firstDiagnostic.Signature != secondDiagnostic.Signature || firstDiagnostic.OriginalMessage == "" {
		t.Fatalf("stable failures = first:%+v second:%+v", firstDiagnostic, secondDiagnostic)
	}
}

func TestFileModuleExecutesConcretePatchApplyEvent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerFileModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	operation := runtimecontract.RuntimeOperation{ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"README.md"}, Parameters: []runtimecontract.OperationParameter{fileStringParameter("patchType", "file_write"), fileStringParameter("content", "hello\n")}}
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecutePatchApplyOperation, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx,
		fileevents.ExecutePatchApplyOperationCommand{
			RuntimeOperationExecutionContext: fileExecutionContext(operation, "exec/ws-1/run-1/dispatch-1/patch-1", "dispatch-1", runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "required-output-1"}, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.EvidenceKindStateChange, workspace),
			SealedOperation:                  mustSealFileOperation(t, operation),
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(fileevents.ExecuteOperationResult)
	if response.Receipt.Evidence.ContentHash == "" {
		t.Fatalf("patch/apply response = %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "hello\n" {
		t.Fatalf("patch/apply content = %q err=%v", string(content), err)
	}
}

func fileExecutionContext(operation runtimecontract.RuntimeOperation, executionKey, dispatchID string, transaction runtimecontract.AuthorityTransaction, effect runtimecontract.RuntimeEffectScope, evidenceKind, workspace string) runtimecontract.RuntimeOperationExecutionContext {
	execution := fileTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: executionKey, Fence: 1, ExpectedEffectScope: effect})
	return runtimecontract.RuntimeOperationExecutionContext{
		Execution:   execution,
		WorkspaceID: "ws-1", RunID: "run-1", DispatchID: dispatchID, Transaction: transaction,
		OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...), EvidenceKind: evidenceKind, WorkspaceDeliverableDirectory: workspace,
	}
}

func fileTestExecutionEnvelope(operation runtimecontract.RuntimeOperation, envelope runtimecontract.RuntimeOperationExecutionEnvelope) runtimecontract.RuntimeOperationExecutionEnvelope {
	envelope.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	envelope.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, nil)
	envelope.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: envelope.OperationHash, ExecutionKey: envelope.ExecutionKey, ExpectedEffectScope: envelope.ExpectedEffectScope,
		IdempotencyClass: envelope.IdempotencyClass, AdmissibilitySnapshotID: envelope.AdmissibilitySnapshotID,
		AcquisitionBinding: envelope.AcquisitionBinding, ReplayProof: envelope.ReplayProof,
	})
	return envelope
}

func mustSealFileOperation(t *testing.T, operation runtimecontract.RuntimeOperation) fileevents.SealedRuntimeOperation {
	t.Helper()
	sealed, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatalf("seal File operation: %v", err)
	}
	return sealed
}

func fileStringParameter(name, value string) runtimecontract.OperationParameter {
	return runtimecontract.OperationParameter{Name: name, String: &value}
}

func fileIntegerParameter(name string, value int64) runtimecontract.OperationParameter {
	return runtimecontract.OperationParameter{Name: name, Integer: &value}
}

func registerFileModuleReceiptOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}
