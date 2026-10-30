package shell

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectAdapterBindsExactStagesReviewAndOriginalRequirements(t *testing.T) {
	recipe, requirement, oldChecks := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectConjunction
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok || len(checks) != len(oldChecks) {
		t.Fatal("project mapping incomplete")
	}
	pkg, err := shellIsolatedValidationPackage(checks)
	if err != nil {
		t.Fatal(err)
	}
	identities := []runtimecontract.RuntimeValidationExecutionStageIdentity{
		{ID: "project-build", Scope: "root-module:./..."},
		{ID: "project-test", Scope: "root-module:./...", RequireTests: true},
		{ID: "project-race", Scope: "root-module:./...", RequireTests: true},
		{ID: "reviewed-cwi-behavior", Scope: "reviewed-package:" + pkg, RequireTests: true, RequireBehavior: true},
	}
	hash := runtimecontract.StableRuntimeValidationExecutionStagesHash(identities)
	for index, check := range checks {
		if check.ExecutionStagesHash != hash || check.ObservationProtocolRef != shellProjectConjunctionProtocolRef || check.RequiredCheckRef != oldChecks[index].RequiredCheckRef {
			t.Fatal("project protocol changed criterion or omitted stage identity")
		}
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, "delivery/value.go", "package delivery\nfunc Compute(v int) int{return v+7}\n")
	writeNativeStructureSource(t, root, target, conjunctiveSourceInput(checks, requirement.SourceObligationChecks[1].CheckRef, `if delivery.Compute(4)!=11 {t.Fatal("B")}`))
	manifest := shellTestWorkspaceManifestBinding("project-input", []string{"go.mod", "delivery/value.go", target})
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 1 {
		t.Fatalf("project catalog missing: %+v", surfaces)
	}
	binding := surfaces[0].ProofRequirementBindings[0]
	if binding.InputReview == nil || binding.InputReview.ScopeContractRef != shellProjectReviewScopeContractRef || binding.InputReview.ValidateAgainstBinding(binding) != nil {
		t.Fatal("project review does not bind complete source package")
	}
	for _, scenario := range []string{"protocol", "command", "args", "timeout"} {
		changed := recipe
		switch scenario {
		case "protocol":
			changed.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
		case "command":
			changed.Command = "true"
		case "args":
			changed.Args = []string{"test", "./delivery"}
		case "timeout":
			changed.TimeoutMillis = 60001
		}
		if values := resolveRuntimeValidationRecipeDeclarations([]runtimecontract.RuntimeValidationRecipe{changed}, manifest.Manifest); len(values) != 0 {
			t.Fatalf("unsupported project declaration resolved: %s", scenario)
		}
	}
	legacy, ok := shellValidationCheckAttestations(runtimecontract.RuntimeValidationRecipe{ID: recipe.ID, AdapterRef: modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint, InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, Command: "go", Args: []string{"test", "-v", "./..."}}, requirement)
	if !ok || !reflect.DeepEqual(legacy, oldChecks) {
		t.Fatal("new adapter upgraded old mapping")
	}
}

func TestProjectAdapterSummaryDoesNotReplaceOriginalRawOutput(t *testing.T) {
	output := RuntimeCommandResult{FailureCode: "validation_project_stage_failed", Stage: "project-test", ExitCode: 1, Stdout: "原 stdout\x00\xff", Stderr: "原 stderr\r\n", ValidationObservation: &runtimecontract.RuntimeValidationObservation{ObservationProtocolRef: shellProjectConjunctionProtocolRef}}
	before := output
	message := enrichShellFailureMessage("provider text must remain private", output)
	if message != "复合验证阶段事实：code=validation_project_stage_failed; stage=project-test; exitCode=1" || before.Stdout != output.Stdout || before.Stderr != output.Stderr || before.ValidationObservation != output.ValidationObservation {
		t.Fatal("project summary copied/rewrote raw diagnostic or changed original result")
	}
}

func TestProjectAdapterPreparationAndReviewCapacityRemainExact(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectConjunction
	checks, supported := shellValidationCheckAttestations(recipe, requirement)
	if !supported {
		t.Fatal("missing complete project mappings")
	}
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	manifest := shellTestWorkspaceManifestBinding("project-preparation", []string{"go.mod"})
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, nil, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementPreparations) != 1 || len(surfaces[0].ProofRequirementBindings) != 0 {
		t.Fatal("missing input advertised project proof")
	}
	preparation := surfaces[0].ProofRequirementPreparations[0]
	if preparation.TargetRef != target || preparation.ContentContractRef != shellProjectInputContentContractRef || !reflect.DeepEqual(preparation.CheckAttestations, checks) {
		t.Fatal("preparation lost original all-of contract")
	}
	for _, scenario := range []string{"files", "bytes"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			writeNativeStructureSource(t, directory, target, conjunctiveSourceInput(checks, requirement.SourceObligationChecks[1].CheckRef, `if delivery.Compute(4)!=11 {t.Fatal("B")}`))
			refs := []string{target}
			if scenario == "bytes" {
				ref := filepath.ToSlash(filepath.Join(filepath.Dir(target), "fixture.txt"))
				writeNativeStructureSource(t, directory, ref, strings.Repeat("x", runtimecontract.RuntimeValidationInputReviewMaxBytes))
				refs = append(refs, ref)
			} else {
				for index := range runtimecontract.RuntimeValidationInputReviewMaxFiles {
					ref := fmt.Sprintf("%s/fixture-%d.txt", filepath.ToSlash(filepath.Dir(target)), index)
					writeNativeStructureSource(t, directory, ref, "data")
					refs = append(refs, ref)
				}
			}
			binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
			if err != nil {
				t.Fatal(err)
			}
			if subject, code := shellVersionedInputReview(directory, refs, binding); subject != nil || code != "validation_input_review_capacity_exceeded" {
				t.Fatal("project review truncated or expanded original capacity")
			}
		})
	}
}

func projectAdapterOwnerCommand(t *testing.T, adapter *RuntimeAdapter, failed bool, adapters ...string) shellevents.ExecuteOperationCommand {
	t.Helper()
	diagnostic := len(adapters) != 0 && adapters[0] == modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
	request, execution := projectExecutionFixture(t, failed && !diagnostic, adapters...)
	recipe := runtimecontract.RuntimeValidationRecipe{ID: request.ValidationRecipeID, AdapterRef: modulecommon.ValidationRecipeAdapterProjectConjunction, InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60000}
	if diagnostic {
		recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
		if failed {
			writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, "delivery/value.go", "package delivery\nfunc Value() int{return 3}\n")
		}
	}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "execution-cwi")
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("missing exact mappings")
	}
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, target, "package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\nfunc "+checks[0].NativeCheckRef+"(t *testing.T){if delivery.Value()!=4{t.Fatal(\"wrong\")}}\n")
	refs := append([]string(nil), request.ValidationScopeTargetRefs...)
	if !slices.Contains(refs, target) {
		refs = append(refs, target)
	}
	manifest := shellTestWorkspaceManifestBinding("project-owner-fixture", refs)
	catalog, err := adapter.AcquireRuntimeOperationAdmissibility(request.WorkspaceDeliverableDirectory, "project-owner-catalog", []string{runtimecontract.RuntimeBlockKindManagedValidationSession})
	if err != nil {
		t.Fatal(err)
	}
	catalog.TargetRefs = refs
	catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{recipe}
	catalog.ValidationRecipeRefs = []string{recipe.ID}
	catalog.ValidationSurfaces = shellValidationSurfaces(catalog.ValidationRecipes, catalog.ValidationRecipes, catalog.ValidationRecipes, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, catalog.ObservedRevision, request.WorkspaceDeliverableDirectory)
	if len(catalog.ValidationSurfaces) != 1 || len(catalog.ValidationSurfaces[0].ProofRequirementBindings) != 1 {
		t.Fatal("actual owner surface missing")
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := adapter.runtimeAdmissibilityObservation(request.WorkspaceDeliverableDirectory, catalog.PrerequisiteID, catalog, refs)
	operation := runtimecontract.RuntimeOperation{ID: execution.OperationID, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, TargetRefs: []string{target}, BindingRefs: []string{"fixture/project-binding"}, Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipe.ID}}}
	sealed, err := shellevents.SealRuntimeOperation(operation, catalog, shellevents.RuntimeExecutionSealInput{CatalogObservation: observation, ValidationScopeTargetRefs: refs, ValidationProofBinding: catalog.ValidationSurfaces[0].ProofRequirementBindings[0], TargetRole: "implementation"})
	if err != nil {
		t.Fatal(err)
	}
	execution.Transaction = runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "fixture-project-authority", DefinitionRevision: requirement.DefinitionRevision, RequiredOutputHash: "fixture-required-output"}
	execution.EvidenceKind = runtimecontract.EvidenceKindValidation
	execution.TargetRefs = operation.TargetRefs
	execution.Execution.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	execution.Execution.AdmissibilitySnapshotID = catalog.SnapshotID
	execution.Execution.AcquisitionBinding = catalog.AcquisitionBinding
	execution.Execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{OperationID: operation.ID, OperationHash: execution.Execution.OperationHash, ExecutionKey: execution.Execution.ExecutionKey, ExpectedEffectScope: execution.Execution.ExpectedEffectScope, IdempotencyClass: execution.Execution.IdempotencyClass, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding})
	return shellevents.ExecuteOperationCommand{RuntimeOperationExecutionContext: execution, SealedOperation: sealed}
}

// 实际公开 Shell seal/handler/runner；receipt authority 为 typed fixture，完整 Runtime 装配另行验收。
func TestProjectAdapterPublicOwnerPreservesFailureAndReplaysOriginalReceipt(t *testing.T) {
	for _, scenario := range []string{"complete", "existing-project-failure", "v7-complete", "v7-exact-failure"} {
		t.Run(scenario, func(t *testing.T) {
			failed := scenario == "existing-project-failure" || scenario == "v7-exact-failure"
			diagnostic := strings.HasPrefix(scenario, "v7-")
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner := newResourceLedgerOwner(hub)
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := owner.Teardown(ctx); err != nil {
					t.Error(err)
				}
			})
			var adapters []string
			if diagnostic {
				adapters = []string{modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic}
			}
			command := projectAdapterOwnerCommand(t, owner.runtimeAdapter, failed, adapters...)
			var saved *runtimecontract.RuntimeOperationReceipt
			commits := 0
			receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
			if err := receipts.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
				query := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
				if event.Source() != ModuleID || query.ExecutionKey != command.Execution.ExecutionKey || query.Fence != command.Execution.Fence {
					t.Error("wrong original receipt query")
				}
				result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: saved != nil, Receipt: saved}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			if err := receipts.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
				commit := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
				value := commit.Receipt.Clone()
				saved = &value
				commits++
				result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: value, StateRevision: 2}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				receipts.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
				receipts.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
			})
			send := func() shellevents.ExecuteOperationResult {
				t.Helper()
				response := hub.Send(magicevent.NewEventWithContext(shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
				if response == nil {
					t.Fatal("missing owner response")
				}
				data, err := response.Get()
				if err != nil {
					t.Fatal(err)
				}
				return data.(shellevents.ExecuteOperationResult)
			}
			first := send()
			if first.Receipt.Validate() != nil || first.Receipt.Evidence.Validation == nil || first.Receipt.Evidence.Validation.ExecutionConjunction == nil {
				t.Fatal("public receipt lost project facts")
			}
			conjunction := first.Receipt.Evidence.Validation.ExecutionConjunction
			if failed {
				stages := 2
				if diagnostic {
					stages = 3
				}
				if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || conjunction.Complete || len(conjunction.Stages) != stages || !conjunction.Stages[1].KnownFailure {
					t.Fatal("old project failure disappeared")
				}
				if diagnostic && (first.Receipt.Feedback == nil || first.Receipt.Feedback.OriginalDiagnostic == nil || first.Receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil || first.Receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic.OwnerCode != modulecommon.ValidationDiagnosticCodeTestFailed) {
					t.Fatalf("public v7 lost exact typed diagnostic: %+v", first.Receipt.Feedback)
				}
			} else if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !conjunction.Complete || len(conjunction.Stages) != 4 {
				t.Fatal("public project success incomplete")
			}
			for _, stage := range conjunction.Stages {
				if stage.Stdout.Storage == nil || stage.Stdout.Storage.Reference == "" || stage.Stderr.Storage == nil || stage.Stderr.Storage.Reference == "" {
					t.Fatal("public receipt lost raw references")
				}
			}
			duplicate := send()
			if commits != 1 || !runtimecontract.SameRuntimeOperationReceipt(first.Receipt, duplicate.Receipt) {
				t.Fatal("duplicate reexecuted or changed original receipt")
			}
		})
	}
}

func TestProjectAdapterReconciliationCannotBorrowPackageOnlySuccess(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := newResourceLedgerOwner(hub)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})
	origin := projectAdapterOwnerCommand(t, owner.runtimeAdapter, false)
	authorization := origin.SealedOperation.Authorization
	command := shellReconciliationQueryCommand(t, origin.WorkspaceDeliverableDirectory, authorization.ValidationRecipes[0])
	command.WorkspaceInternalDirectory = origin.WorkspaceInternalDirectory
	command.ValidationScopeTargetRefs = authorization.ValidationScopeTargetRefs
	command.Binding.TargetRefs = origin.TargetRefs
	command.Binding.RequiredCheckRefs = authorization.ValidationProofBinding.RequiredCheckRefs
	command.Binding.ValidationCheckAttestations = authorization.ValidationProofBinding.CheckAttestations
	if command.ValidateForOwner(ModuleID) != nil {
		t.Fatal("invalid project reconciliation fixture")
	}
	for _, scenario := range []string{"complete", "old-failure", "missing-scope", "missing-internal"} {
		t.Run(scenario, func(t *testing.T) {
			changed := command
			switch scenario {
			case "old-failure":
				writeNativeStructureSource(t, origin.WorkspaceDeliverableDirectory, "workorch_validation/v3/old_test.go", "package old\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Fatal(\"原工程失败\")}\n")
				changed.ValidationScopeTargetRefs = append(append([]string(nil), changed.ValidationScopeTargetRefs...), "workorch_validation/v3/old_test.go")
			case "missing-scope":
				changed.ValidationScopeTargetRefs = nil
			case "missing-internal":
				changed.WorkspaceInternalDirectory = ""
			}
			response := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeCWIReconciliation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, changed))
			if response == nil {
				t.Fatal("missing reconciliation response")
			}
			data, err := response.Get()
			if err != nil {
				t.Fatal(err)
			}
			value := data.(shellevents.QueryRuntimeCWIReconciliationResult).Observation
			if scenario == "complete" && value.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
				t.Fatalf("project reconciliation incomplete: %+v", value)
			}
			if scenario != "complete" && value.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
				t.Fatal("incomplete conjunction supplied current proof")
			}
		})
	}
}

func TestProjectReconciliationRawScopeBindsFullIdentityAndRejectsUnsafeRequests(t *testing.T) {
	command := shellReconciliationQueryCommand(t, t.TempDir(), isolatedInputRecipe())
	base := shellProjectReconciliationRawScope(command, "recipe")
	probe := base
	probe.StageID, probe.Stream = "project-test", "stdout"
	if probe.Validate() != nil || base.ExecutionKey == runtimecontract.StableRuntimeOperationExecutionKey(command.WorkspaceID, command.RunID, command.RequestID, "recipe") {
		t.Fatal("reconciliation borrowed original execution scope")
	}
	for _, field := range []string{"workspace", "run", "request", "generation", "definition", "binding-set", "binding", "baseline-revision", "baseline-ref", "recipe"} {
		changed, recipe := command, "recipe"
		switch field {
		case "workspace":
			changed.WorkspaceID = "other"
		case "run":
			changed.RunID = "other"
		case "request":
			changed.RequestID = "other"
		case "generation":
			changed.Generation++
		case "definition":
			changed.DefinitionRevision = "other"
		case "binding-set":
			changed.BindingSetHash = "other"
		case "binding":
			changed.Binding.BindingHash = "other"
		case "baseline-revision":
			changed.WorkspaceBaselineRevision = "other"
		case "baseline-ref":
			changed.WorkspaceBaselineRef = "other"
		case "recipe":
			recipe = "other"
		}
		if scope := shellProjectReconciliationRawScope(changed, recipe); scope.ExecutionKey == base.ExecutionKey {
			t.Fatalf("raw scope omitted %s", field)
		}
	}
	request, _ := projectExecutionFixture(t, false)
	scope := shellProjectRawScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID, ExecutionKey: base.ExecutionKey}
	for _, field := range []string{"state-changing", "scratch", "unmanaged", "no-mapping", "timeout"} {
		changed := request
		switch field {
		case "state-changing":
			changed.StateChanging = true
		case "scratch":
			changed.ValidationScratchDirectory = t.TempDir()
		case "unmanaged":
			changed.ManagedValidation = runtimecontract.RuntimeManagedValidation{}
		case "no-mapping":
			changed.ValidationCheckAttestations = nil
		case "timeout":
			changed.TimeoutMillis = 60001
		}
		produced, err := NewRuntimeAdapter().executeShellProjectScope(context.Background(), changed, scope)
		if err == nil || produced.Conjunction != nil || len(produced.Result.ValidationResources) != 0 || len(produced.Raw) != 0 {
			t.Fatalf("unsafe reconciliation request prepared or executed: %s", field)
		}
	}
}

func TestProjectAdapterExecutionProducesPersistableExactObservation(t *testing.T) {
	request, execution := projectExecutionFixture(t, false)
	declaration := runtimecontract.RuntimeValidationRecipe{ID: request.ValidationRecipeID, AdapterRef: modulecommon.ValidationRecipeAdapterProjectConjunction, InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60000}
	requirement := shellValidationProofRequirementForTest(t, declaration.ID, "execution-cwi")
	checks, ok := shellValidationCheckAttestations(declaration, requirement)
	if !ok {
		t.Fatal("missing project checks")
	}
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, target, "package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\nfunc "+checks[0].NativeCheckRef+"(t *testing.T){if delivery.Value()!=4{t.Fatal(\"wrong\")}}\n")
	request.TargetRefs, request.ValidationCheckAttestations = []string{target}, checks
	if !slices.Contains(request.ValidationScopeTargetRefs, target) {
		request.ValidationScopeTargetRefs = append(request.ValidationScopeTargetRefs, target)
	}
	request.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{declaration}
	for _, scenario := range []string{"missing-recipe", "legacy-recipe", "missing-hash", "wrong-hash", "mixed-protocol", "missing-checks"} {
		changed := request
		changed.ValidationRecipes = runtimecontract.CloneRuntimeValidationRecipes(request.ValidationRecipes)
		changed.ValidationCheckAttestations = runtimecontract.CloneRuntimeValidationCheckAttestationBindings(checks)
		switch scenario {
		case "missing-recipe":
			changed.ValidationRecipes = nil
		case "legacy-recipe":
			changed.ValidationRecipes[0].AdapterRef = modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint
		case "missing-hash":
			changed.ValidationCheckAttestations[0].ExecutionStagesHash = ""
		case "wrong-hash":
			changed.ValidationCheckAttestations[0].ExecutionStagesHash = requirement.Hash
		case "mixed-protocol":
			changed.ValidationCheckAttestations[0].ObservationProtocolRef = shellGoConjunctiveEntrypointObservationProtocolRef
		case "missing-checks":
			changed.ValidationCheckAttestations = nil
		}
		if selected, err := shellProjectRequestSelected(changed); selected || err == nil {
			t.Fatalf("invalid project mapping selected: %s", scenario)
		}
	}
	if _, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request); err == nil {
		t.Fatal("direct adapter downgraded project execution without original scope")
	}
	if err := resolveManagedValidationRecipe(&request); err != nil {
		t.Fatal(err)
	}
	if selected, err := shellProjectRequestSelected(request); err != nil || !selected {
		t.Fatalf("selection: %v", err)
	}
	produced, err := NewRuntimeAdapter().executeShellProjectConjunction(context.Background(), request, execution)
	if err != nil {
		t.Fatal(err)
	}
	observation := produced.Result.Evidence.Validation
	if observation == nil || observation.ValidateFailureDetails() != nil || observation.ExecutionConjunction == nil || !observation.ExecutionConjunction.Complete || len(observation.ExecutionConjunction.Stages) != 4 {
		t.Fatalf("project observation incomplete: %+v", observation)
	}
	for _, check := range checks {
		if !runtimecontract.RuntimeValidationAttestationPassed(*observation, check) {
			t.Fatal("exact project attestation did not pass")
		}
	}
	changed := observation.Clone()
	changed.ExecutionConjunction.Stages = changed.ExecutionConjunction.Stages[1:]
	if runtimecontract.RuntimeValidationAttestationPassed(changed, checks[0]) {
		t.Fatal("missing stage supplied project proof")
	}
}
