package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeTestNativeOwnerFactAcquisitionProposal echoes only the acquisition
// authorization already sealed by SM-2.  The Shell owner's exact per-CWI
// preparation supplies the prospective target; the helper does not invent a
// validation surface, proof binding, or owner fact.
func runtimeTestNativeOwnerFactAcquisitionProposal(
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	coverageID common.CoverageWorkItemID,
) (runtimecontract.RuntimeSemanticProofProposal, bool, error) {
	manifest = manifest.Normalized()
	var obligation runtimecontract.RuntimeSemanticProofObligation
	found := false
	for _, candidate := range manifest.ProofObligations {
		if candidate.CoverageWorkItemID != coverageID || candidate.OwnerFactAcquisition == nil {
			continue
		}
		if found {
			return runtimecontract.RuntimeSemanticProofProposal{}, false, fmt.Errorf("native acquisition duplicated obligation for %q", coverageID)
		}
		obligation = candidate.Clone()
		found = true
	}
	if !found {
		return runtimecontract.RuntimeSemanticProofProposal{}, false, nil
	}
	acquisition := obligation.OwnerFactAcquisition.Clone()
	var catalog runtimecontract.RuntimeOperationAdmissibility
	for _, candidate := range manifest.OwnerFactDependencyCatalogs {
		if candidate.OwnerModuleID != acquisition.Requirement.OwnerModuleID {
			continue
		}
		if catalog.SnapshotID != "" {
			return runtimecontract.RuntimeSemanticProofProposal{}, false, fmt.Errorf("native acquisition duplicated dependency catalog for %q", coverageID)
		}
		catalog = candidate.Normalized()
	}
	if catalog.SnapshotID == "" {
		return runtimecontract.RuntimeSemanticProofProposal{}, false, fmt.Errorf("native acquisition has no %q dependency catalog for %q", acquisition.Requirement.OwnerModuleID, coverageID)
	}
	preparation, err := runtimecontract.ResolveRuntimeValidationProofPreparation(
		catalog,
		acquisition.Requirement,
		acquisition.ProspectiveSatisfactionContractRef,
	)
	if err != nil {
		return runtimecontract.RuntimeSemanticProofProposal{}, false, fmt.Errorf("resolve native acquisition preparation for %q: %w", coverageID, err)
	}
	targetRefs := make([]string, 0, len(preparation.ProspectiveTargetSatisfactionConstraint.AllOf)+1)
	if preparation.TargetRef != "" {
		targetRefs = append(targetRefs, preparation.TargetRef)
	}
	// Every exact missing prerequisite is owner-authored in the all-of
	// constraint. Include it in the same bounded File transaction. Non-exact
	// clauses (for example *_test.go) are satisfied by the native target above.
	for _, clause := range preparation.ProspectiveTargetSatisfactionConstraint.AllOf {
		for _, predicate := range clause.AllOf {
			if predicate.Kind == runtimecontract.RuntimeProspectiveTargetPredicatePathExact {
				targetRefs = runtimeTestNormalizeStrings(append(targetRefs, predicate.Value))
			}
		}
	}
	if len(targetRefs) == 0 || !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(
		targetRefs,
		preparation.ProspectiveTargetSatisfactionConstraint,
	) {
		return runtimecontract.RuntimeSemanticProofProposal{}, false, fmt.Errorf("native acquisition targets %v do not satisfy Shell constraint for %q", targetRefs, coverageID)
	}
	return runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: coverageID,
		OwnerModuleID:      acquisition.AcquisitionOwnerModuleID,
		OperationKind:      acquisition.AcquisitionOperationKind,
		OperationVariantID: acquisition.AcquisitionOperationVariantID,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:                    acquisition.AcquisitionTargetPolicy,
			OwnerSelectorRefs:         []string{acquisition.AcquisitionTargetSelectorRef},
			ProspectiveConstraintRefs: append([]string(nil), acquisition.AcquisitionConstraintRefs...),
			ProspectiveTargetRefs:     targetRefs,
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        runtimecontract.RuntimeSemanticCurrentDeclarationRef(obligation),
	}.Normalized(), true, nil
}

func runtimeTestSemanticCurrentDeclarationRefForCoverage(
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	coverageID common.CoverageWorkItemID,
) (string, error) {
	declarationRef := ""
	for _, obligation := range manifest.Normalized().ProofObligations {
		if obligation.CoverageWorkItemID != coverageID {
			continue
		}
		if declarationRef != "" {
			return "", fmt.Errorf("semantic manifest duplicated obligation for %q", coverageID)
		}
		declarationRef = runtimecontract.RuntimeSemanticCurrentDeclarationRef(obligation)
	}
	if declarationRef == "" {
		return "", fmt.Errorf("semantic manifest has no declaration for %q", coverageID)
	}
	return declarationRef, nil
}

// runtimeTestNativeOwnerFactAcquisitionCandidate turns an SM-3 required File
// sequence into the concrete source declared by Shell's exact preparation.
// It is deliberately inapplicable to ordinary state-change transactions.
func runtimeTestNativeOwnerFactAcquisitionCandidate(
	command llmevents.RequestRuntimeCandidateCommand,
) (runtimecontract.RuntimeCandidate, bool, error) {
	if command.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		len(command.Decision.RequiredOutput.RequiredOperationSequence) == 0 {
		return runtimecontract.RuntimeCandidate{}, false, nil
	}
	coverageIDs := command.Decision.Focus.CoverageWorkItemIDs
	if len(coverageIDs) != 1 {
		return runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("native acquisition requires one current CWI, got %v", coverageIDs)
	}
	preparations := make(map[string]runtimecontract.RuntimeValidationProofPreparation)
	for _, catalog := range command.CandidateContext.Resources.OperationAdmissibilities {
		for _, surface := range catalog.Normalized().ValidationSurfaces {
			for _, preparation := range surface.ProofRequirementPreparations {
				preparation = preparation.Clone()
				if preparation.CoverageWorkItemID != coverageIDs[0] || preparation.TargetRef == "" {
					continue
				}
				if _, duplicate := preparations[preparation.TargetRef]; duplicate {
					return runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("native acquisition duplicated preparation target %q", preparation.TargetRef)
				}
				preparations[preparation.TargetRef] = preparation
			}
		}
	}
	requirements := command.Decision.RequiredOutput.RequiredOperationSequence
	operationIDs := make(map[string]string, len(requirements))
	for index, requirement := range requirements {
		if runtimecontract.CanonicalRuntimeBlockKind(requirement.Kind) != runtimecontract.StepKindFileWrite || requirement.TargetRef == "" {
			return runtimecontract.RuntimeCandidate{}, false, nil
		}
		operationIDs[requirement.RequirementID] = fmt.Sprintf("prepare-native-proof-%02d", index+1)
	}
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirements))
	for _, requirement := range requirements {
		content := ""
		switch requirement.TargetRef {
		case "go.mod":
			content = "module example.com/workorch-runtime-native-fixture\n\ngo 1.26\n"
		default:
			preparation, ok := preparations[requirement.TargetRef]
			if !ok {
				return runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("required native acquisition target %q has no Shell preparation", requirement.TargetRef)
			}
			content = runtimeTestNativeValidationSource(preparation)
		}
		dependsOn := make([]string, 0, len(requirement.DependsOnRequirementIDs))
		for _, dependencyRef := range requirement.DependsOnRequirementIDs {
			operationID, ok := operationIDs[dependencyRef]
			if !ok {
				return runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("native acquisition dependency %q is absent", dependencyRef)
			}
			dependsOn = append(dependsOn, operationID)
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: runtimecontract.RuntimeOperation{
			ID:                   operationIDs[requirement.RequirementID],
			Kind:                 runtimecontract.StepKindFileWrite,
			RequiredOperationRef: requirement.RequirementID,
			DependsOn:            dependsOn,
			CoverageWorkItemIDs:  append([]common.CoverageWorkItemID(nil), coverageIDs...),
			TargetRefs:           []string{requirement.TargetRef},
			Parameters:           []runtimecontract.OperationParameter{{Name: "content", String: &content}},
		}})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}, true, nil
}

func runtimeTestNativeValidationSource(preparation runtimecontract.RuntimeValidationProofPreparation) string {
	preparation = preparation.Clone()
	packageName := "workorch_validation"
	if directory := filepath.Dir(preparation.TargetRef); directory == "." {
		packageName = "main"
	}
	var source strings.Builder
	fmt.Fprintf(&source, "package %s\n\nimport \"testing\"\n\n", packageName)
	for _, attestation := range preparation.CheckAttestations {
		fmt.Fprintf(&source, "func %s(t *testing.T) {\n", attestation.NativeCheckRef)
		source.WriteString("\tif 2+2 != 4 { t.Fatal(\"owner-native validation assertion failed\") }\n")
		source.WriteString("}\n\n")
	}
	return source.String()
}

// runtimeTestPrepareNativeValidationThroughOwners is setup for tests whose
// subject is not owner-fact acquisition. It asks the real Shell owner for its
// exact per-CWI preparations, executes those writes through the real File
// owner, and then requires a fresh Shell observation to expose direct native
// proof bindings. It does not synthesize catalogs, bindings, or receipts.
func runtimeTestPrepareNativeValidationThroughOwners(
	t *testing.T,
	bootstrap runtimecontract.RuntimeBootstrap,
	workspaceDirectory string,
) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		bootstrap.Definition.Revision,
		bootstrap.CoverageItems,
		bootstrap.DefinitionFocuses,
	)
	if err != nil {
		t.Fatalf("compile native validation preparation requirements: %v", err)
	}
	if len(requirements) == 0 {
		t.Fatal("native validation preparation has no exact CWI requirement")
	}

	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runtimeTestRegisterNativePreparationReceiptOwner(t, hub)
	workspaceID := common.WorkspaceID("workspace-native-preparation")
	runID := common.RunID("run-native-preparation")
	authoritySnapshotID := "authority-native-preparation"
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: workspaceID, DeliverableDirectory: workspaceDirectory, InternalDirectory: workspaceDirectory,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatalf("configure File native-preparation owner: %v", err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatalf("start File native-preparation owner: %v", err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatalf("configure Shell native-preparation owner: %v", err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatalf("start Shell native-preparation owner: %v", err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })

	manifest := runtimeTestAcquireNativePreparationManifest(
		t, ctx, hub, workspaceID, runID, authoritySnapshotID, workspaceDirectory,
	)
	catalog := runtimeTestAcquireNativePreparationShellCatalog(
		t, ctx, hub, workspaceID, runID, authoritySnapshotID, workspaceDirectory,
		manifest, bootstrap.Activation.EvidencePolicies, requirements,
	)
	preparations := runtimeTestNativeProofPreparations(t, catalog, requirements)

	executionSequence := 0
	executeFileWrite := func(operation runtimecontract.RuntimeOperation) {
		executionSequence++
		sealed, sealErr := fileevents.SealRuntimeOperation(operation)
		if sealErr != nil {
			t.Fatalf("seal File native preparation %q: %v", operation.ID, sealErr)
		}
		dispatchID := fmt.Sprintf("dispatch-native-preparation-%02d", executionSequence)
		record := runtimecontract.RuntimeOperationExecutionRecord{
			OperationID: operation.ID, OperationHash: runtimecontract.StableRuntimeOperationHash(operation),
			ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, dispatchID, operation.ID),
			Fence:        1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}
		record.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(record)
		execution := runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: record.ExecutionKey, Fence: record.Fence, OperationHash: record.OperationHash,
			ExpectedEffectScope: record.ExpectedEffectScope, IdempotencyClass: record.IdempotencyClass,
			AuthorityHash: record.AuthorityHash,
		}
		result := hub.Send(magicevent.NewEventWithContext(
			fileevents.TopicExecuteFileWriteOperation,
			runtimecontract.RuntimeModuleOrchestrator,
			runtimecontract.RuntimeModuleFile,
			magicevent.NewHeader(), ctx,
			fileevents.ExecuteFileWriteOperationCommand{
				RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
					Execution:   execution,
					WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID,
					Transaction: runtimecontract.AuthorityTransaction{
						AuthoritySnapshotID: authoritySnapshotID,
						RequiredOutputHash:  "required-output-native-preparation",
					},
					OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...),
					EvidenceKind:                  runtimecontract.EvidenceKindStateChange,
					WorkspaceDeliverableDirectory: workspaceDirectory,
				},
				SealedOperation: sealed,
			},
		))
		if result == nil {
			t.Fatalf("File owner returned no result for native preparation %q", operation.ID)
		}
		data, getErr := result.Get()
		if getErr != nil {
			t.Fatalf("File owner native preparation %q: %v", operation.ID, getErr)
		}
		response, ok := data.(fileevents.ExecuteOperationResult)
		if !ok || response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !response.Receipt.Changed {
			t.Fatalf("File owner native preparation %q response = %#v", operation.ID, data)
		}
	}

	if !runtimeTestManifestContainsTarget(manifest.Manifest, "go.mod") {
		content := "module example.com/workorch-runtime-native-fixture\n\ngo 1.26\n"
		executeFileWrite(runtimecontract.RuntimeOperation{
			ID: "prepare-native-go-module", Kind: runtimecontract.StepKindFileWrite,
			RequiredOperationRef: "prepare-validation-proof/go-module-root",
			TargetRefs:           []string{"go.mod"},
			Parameters:           []runtimecontract.OperationParameter{{Name: "content", String: &content}},
		})
	}
	for index, requirement := range requirements {
		preparation := preparations[requirement.Hash]
		var source strings.Builder
		source.WriteString("package workorch_validation\n\nimport \"testing\"\n\n")
		for _, attestation := range preparation.CheckAttestations {
			fmt.Fprintf(&source, "func %s(t *testing.T) {\n", attestation.NativeCheckRef)
			source.WriteString("\tif 2+2 != 4 { t.Fatal(\"owner-native validation assertion failed\") }\n")
			source.WriteString("}\n\n")
		}
		content := source.String()
		executeFileWrite(runtimecontract.RuntimeOperation{
			ID: fmt.Sprintf("prepare-native-cwi-%02d", index+1), Kind: runtimecontract.StepKindFileWrite,
			RequiredOperationRef: "prepare-validation-proof/" + requirement.Hash,
			TargetRefs:           []string{preparation.TargetRef},
			Parameters:           []runtimecontract.OperationParameter{{Name: "content", String: &content}},
		})
	}

	reobservedManifest := runtimeTestAcquireNativePreparationManifest(
		t, ctx, hub, workspaceID, runID, authoritySnapshotID, workspaceDirectory,
	)
	reobservedCatalog := runtimeTestAcquireNativePreparationShellCatalog(
		t, ctx, hub, workspaceID, runID, authoritySnapshotID, workspaceDirectory,
		reobservedManifest, bootstrap.Activation.EvidencePolicies, requirements,
	)
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability(
		requirements,
		[]runtimecontract.RuntimeOperationAdmissibility{reobservedCatalog},
	); err != nil {
		t.Fatalf("Shell owner did not reobserve exact native proof applicability: %v", err)
	}
}

func runtimeTestAcquireNativePreparationManifest(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	authoritySnapshotID string,
	workspaceDirectory string,
) fileevents.AcquireWorkspaceManifestResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireWorkspaceManifest,
		runtimecontract.RuntimeModuleOrchestrator,
		orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx,
		fileevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: workspaceID, RunID: runID, AuthoritySnapshotID: authoritySnapshotID,
			PrerequisiteID: "manifest:native-preparation", WorkspaceDeliverableDirectory: workspaceDirectory,
		},
	))
	if result == nil {
		t.Fatal("File owner returned no native-preparation manifest")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("acquire File native-preparation manifest: %v", err)
	}
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || response.Manifest.Revision == "" || response.Observation.ObservationID == "" {
		t.Fatalf("File owner native-preparation manifest = %#v", data)
	}
	return response
}

func runtimeTestAcquireNativePreparationShellCatalog(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	authoritySnapshotID string,
	workspaceDirectory string,
	manifest fileevents.AcquireWorkspaceManifestResult,
	policies []runtimecontract.RuntimeEvidencePolicy,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeOperationAdmissibility,
		runtimecontract.RuntimeModuleOrchestrator,
		runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx,
		shellevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: workspaceID, RunID: runID, AuthoritySnapshotID: authoritySnapshotID,
			PrerequisiteID: "operation-admissibility:native-preparation", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			WorkspaceDeliverableDirectory: workspaceDirectory,
			RequiredKinds:                 []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			ValidationRecipes:             runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(policies),
			ValidationProofRequirements:   runtimecontract.NormalizeRuntimeValidationProofRequirements(requirements),
			WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{
				Manifest: manifest.Manifest, Observation: manifest.Observation,
			},
			MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
		},
	))
	if result == nil {
		t.Fatal("Shell owner returned no native-preparation catalog")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("acquire Shell native-preparation catalog: %v", err)
	}
	response, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Admissibility.SnapshotID == "" || response.Acquisition.Observation.ObservationID == "" {
		t.Fatalf("Shell owner native-preparation catalog = %#v", data)
	}
	return response.Acquisition.Admissibility.Normalized()
}

func runtimeTestNativeProofPreparations(
	t *testing.T,
	catalog runtimecontract.RuntimeOperationAdmissibility,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
) map[string]runtimecontract.RuntimeValidationProofPreparation {
	t.Helper()
	preparations := make(map[string]runtimecontract.RuntimeValidationProofPreparation, len(requirements))
	for _, surface := range catalog.Normalized().ValidationSurfaces {
		for _, preparation := range surface.ProofRequirementPreparations {
			if _, duplicate := preparations[preparation.RequirementHash]; duplicate {
				t.Fatalf("Shell owner duplicated native preparation %q", preparation.RequirementHash)
			}
			preparations[preparation.RequirementHash] = preparation.Clone()
		}
	}
	for _, requirement := range requirements {
		preparation, found := preparations[requirement.Hash]
		if !found || preparation.TargetRef == "" || len(preparation.CheckAttestations) == 0 {
			t.Fatalf("Shell owner has no exact native preparation for %q: %+v", requirement.Hash, catalog.ValidationSurfaces)
		}
	}
	return preparations
}

func runtimeTestManifestContainsTarget(manifest runtimecontract.WorkspaceManifest, target string) bool {
	for _, candidate := range manifest.TargetRefs {
		if candidate == target {
			return true
		}
	}
	return false
}

func runtimeTestRegisterNativePreparationReceiptOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	receipts := make(map[string]runtimecontract.RuntimeOperationReceipt)
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			value := receipt.Normalized()
			response.Receipt = &value
		}
		result.Set(response, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		if existing, found := receipts[command.Receipt.ExecutionKey]; found {
			result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: existing, StateRevision: uint64(len(receipts))}, nil)
			return
		}
		receipt := command.Receipt.Normalized()
		receipts[receipt.ExecutionKey] = receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: uint64(len(receipts))}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}
