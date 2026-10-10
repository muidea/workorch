package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	activationcore "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// TestBuiltinSoftwareDevAllManagedCWIsCloseThroughTypedNativeProofPipeline
// proves the default Profile's complete, actually compiled managed-validation
// set. Application seals an exact requirement per CWI; Shell publishes an
// owner-native preparation; File creates exactly that target; Shell reobserves
// and attests the native check; only then can the pure receipt matcher match it.
// This is owner-protocol calibration, not semantic binding admission. The
// independent input-review transaction is covered by the drive mainline tests.
// Neither Orchestrator nor the provider interprets Go, test names, or stdout.
func TestBuiltinSoftwareDevAllManagedCWIsCloseThroughTypedNativeProofPipeline(t *testing.T) {
	t.Run("ready_inputs", func(t *testing.T) { runDefaultNativeProofPipeline(t, false) })
	t.Run("create_only_conjunctive_input", func(t *testing.T) { runDefaultNativeProofPipeline(t, true) })
}

func runDefaultNativeProofPipeline(t *testing.T, supplementFirst bool) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerDefaultNativeProofReceiptOwner(t, hub)

	fileOwner, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fileOwner.Teardown(ctx) })
	shellOwner, err := shellbiz.New(hub, nil, shellbiz.Dependencies{RuntimeAdapter: shellbiz.NewRuntimeAdapter()})
	if err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shellOwner.Teardown(ctx) })

	bootstrap := builtinSoftwareDevNativeProofBootstrap(t)
	workspaceID := common.WorkspaceID("workspace-default-native-proof")
	runID := common.RunID("run-default-native-proof")
	facts, err := bootstrap.InitialFacts(workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := RuntimeAggregate{
		ResultFeedbackVersion: runtimecontract.RuntimeResultFeedbackVersionCurrent,
		Definition:            bootstrap.Definition,
		Facts:                 facts,
		Decision: runtimecontract.RuntimeDecision{AuthoritySnapshot: runtimecontract.AuthoritySnapshot{
			ID: "authority-default-native-proof",
		}},
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		bootstrap.Definition.Revision,
		bootstrap.DefinitionSeed.CoverageItems,
		bootstrap.DefinitionFocuses,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != len(builtinSoftwareDevDoDItems(t))+1 {
		t.Fatalf("default managed proof set = %d, want DoD+validation_report=%d", len(requirements), len(builtinSoftwareDevDoDItems(t))+1)
	}

	workspaceDirectory := t.TempDir()
	manifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:default-native-proof", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	shellPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:default-native-proof", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        runtimecontract.RuntimeModuleShell,
		RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	initialManifest := sendObservedManifest(t, ctx, hub, aggregate, manifestPrerequisite, workspaceDirectory)
	initial := acquireObservedShellValidationCatalog(
		t, ctx, hub, aggregate, shellPrerequisite, workspaceDirectory, initialManifest, nil, requirements,
	)
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability(
		requirements, []runtimecontract.RuntimeOperationAdmissibility{initial.Acquisition.Admissibility},
	); err != nil {
		t.Fatalf("initial default proof applicability: %v", err)
	}
	preparations := defaultNativeProofPreparations(t, initial.Acquisition.Admissibility, requirements)
	preparedTargets := make(map[string]string, len(requirements))
	for _, requirement := range requirements {
		preparation := preparations[requirement.Hash]
		prospectiveTargets := []string{"go.mod", preparation.TargetRef}
		if !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(
			prospectiveTargets,
			preparation.ProspectiveTargetSatisfactionConstraint,
		) {
			t.Fatalf("File target set %v does not satisfy Shell preparation %q: %+v", prospectiveTargets, requirement.Hash, preparation)
		}
	}

	executeFileWrite := func(operation runtimecontract.RuntimeOperation, dispatchID string) runtimecontract.RuntimeOperationReceipt {
		sealed, err := fileevents.SealRuntimeOperation(operation)
		if err != nil {
			t.Fatal(err)
		}
		response := sendDefaultNativeProofFileWrite(t, ctx, hub, fileevents.ExecuteFileWriteOperationCommand{
			RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
				Execution:   defaultNativeProofExecutionEnvelope(workspaceID, runID, dispatchID, operation, nil),
				WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID,
				Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision, RequiredOutputHash: "required-output-default-native-proof"},
				OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...),
				EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: workspaceDirectory,
			},
			SealedOperation: sealed,
		})
		if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !response.Receipt.Changed {
			t.Fatalf("File preparation %q receipt = %+v", operation.ID, response.Receipt)
		}
		return response.Receipt
	}

	// Start from an empty workspace. The module root is not a harness shortcut:
	// it is one of the exact targets in every Shell-authored all-of preparation,
	// and File is the only component that creates it.
	moduleOperation := defaultNativeProofModuleFileOperation()
	moduleReceipt := executeFileWrite(moduleOperation, "dispatch-default-proof-file-module")
	if len(moduleReceipt.TargetRefs) != 1 || moduleReceipt.TargetRefs[0] != "go.mod" {
		t.Fatalf("File module preparation did not attest Shell's exact module target: %+v", moduleReceipt)
	}
	// 夹具校准 owner 协议，不自证任意模板语义；实际交付入口独立于验证输入，
	// 必须在默认 adapter v5 的完整合取 observation 合同下真实执行。
	subject := "package fixture\nfunc DeliveredValue() int { return 4 }\n"
	executeFileWrite(runtimecontract.RuntimeOperation{ID: "fixture-subject", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"fixture.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &subject}}}, "dispatch-fixture-subject")

	// Each File operation is one exact owner-authored prospective target. A
	// provider may return the set as several operations in one candidate, but it
	// cannot merge targets or invent native-check identities.
	for index, requirement := range requirements {
		preparation := preparations[requirement.Hash]
		operation := defaultNativeProofFileOperation(requirement, preparation, index)
		if supplementFirst && index == 0 {
			weak := fmt.Sprintf("package workorch_validation\nimport (\"testing\"; \"strings\")\nfunc %s(t *testing.T) { if !strings.Contains(\"contract\", \"contract\") { t.Fatal(\"missing contract\") } }\n", preparation.CheckAttestations[0].NativeCheckRef)
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "content", String: &weak}}
		}
		dispatchID := fmt.Sprintf("dispatch-default-proof-file-%02d", index+1)
		receipt := executeFileWrite(operation, dispatchID)
		if len(receipt.TargetRefs) != 1 || receipt.TargetRefs[0] != preparation.TargetRef {
			t.Fatalf("File preparation %q did not attest the exact Shell target: receipt=%+v preparation=%+v", requirement.Hash, receipt, preparation)
		}
		preparedTargets[requirement.Hash] = receipt.TargetRefs[0]
	}

	observedManifest := sendObservedManifest(t, ctx, hub, aggregate, manifestPrerequisite, workspaceDirectory)
	reobserved := acquireObservedShellValidationCatalog(
		t, ctx, hub, aggregate, shellPrerequisite, workspaceDirectory, observedManifest, observedManifest.Manifest.TargetRefs, requirements,
	)
	catalog := reobserved.Acquisition.Admissibility.Normalized()
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability(
		requirements, []runtimecontract.RuntimeOperationAdmissibility{catalog},
	); err != nil {
		t.Fatalf("reobserved default proof applicability: %v", err)
	}

	focusByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeDefinitionFocus, len(requirements))
	for _, focus := range bootstrap.DefinitionFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 &&
			containsDefaultNativeProofString(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) &&
			containsDefaultNativeProofString(focus.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession) {
			coverageID := focus.CoverageWorkItemIDs[0]
			if _, duplicate := focusByCoverage[coverageID]; duplicate {
				t.Fatalf("default managed CWI %q has duplicate Definition focuses", coverageID)
			}
			focusByCoverage[coverageID] = focus
		}
	}
	var firstReceipt runtimecontract.RuntimeOperationReceipt
	var firstBinding runtimecontract.CoverageEvidenceBinding
	var firstOperation runtimecontract.RuntimeOperation
	for index, requirement := range requirements {
		surface, ownerBinding, found := runtimecontract.RuntimeValidationSurfaceForProofRequirement(catalog.ValidationSurfaces, requirement)
		if !found {
			t.Fatalf("reobserved catalog has no exact ready binding for %q: %+v", requirement.Hash, catalog.ValidationSurfaces)
		}
		if len(ownerBinding.CheckAttestations) != 1 || ownerBinding.CheckAttestations[0].ObservationProtocolRef != "shell/go-test-json/workspace-behavior/v6" || ownerBinding.CheckAttestations[0].SupplementalNativeCheckRef != "" || ownerBinding.InputReview == nil || ownerBinding.InputReview.ValidateAgainstBinding(ownerBinding) != nil || len(ownerBinding.InputReview.Files) != 2 {
			t.Fatalf("default adapter did not publish an exact isolated review subject: %+v", ownerBinding)
		}
		focus, found := focusByCoverage[requirement.CoverageWorkItemID]
		if !found {
			t.Fatalf("default proof requirement %q has no exact focus", requirement.Hash)
		}
		preparedTarget := preparedTargets[requirement.Hash]
		coverageBinding, semanticFocus := defaultNativeProofCoverageBinding(t, focus, requirement, ownerBinding, preparedTarget)
		recipeID := ownerBinding.ValidationRecipeID
		operation := runtimecontract.RuntimeOperation{
			ID:          fmt.Sprintf("validate-default-proof-%02d", index+1),
			Kind:        runtimecontract.RuntimeBlockKindManagedValidationSession,
			BindingRefs: []string{coverageBinding.ID},
			TargetRefs:  []string{preparedTarget},
			Parameters:  []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
		}
		executeBoundValidation := func(dispatchID string) shellevents.ExecuteOperationResult {
			sealed, err := shellevents.SealRuntimeOperation(operation, catalog, shellevents.RuntimeExecutionSealInput{
				CatalogObservation:        reobserved.Acquisition.Observation,
				TargetRole:                semanticFocus.TargetRole,
				ValidationScopeTargetRefs: append([]string(nil), catalog.TargetRefs...),
				EvidencePolicies:          bootstrap.Activation.EvidencePolicies,
				ValidationProofBinding:    ownerBinding,
			})
			if err != nil {
				t.Fatalf("seal default native proof %q on surface %q: %v", requirement.Hash, surface.ID, err)
			}
			return sendObservedShellExecution(t, ctx, hub, shellevents.ExecuteOperationCommand{
				RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
					Execution:   defaultNativeProofExecutionEnvelope(workspaceID, runID, dispatchID, operation, &catalog),
					WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID,
					Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision, RequiredOutputHash: "required-output-default-native-proof"},
					OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...), EvidenceKind: runtimecontract.EvidenceKindValidation,
					WorkspaceDeliverableDirectory: workspaceDirectory,
				},
				SealedOperation: sealed,
			})
		}
		response := executeBoundValidation(fmt.Sprintf("dispatch-default-proof-shell-%02d", index+1))
		if supplementFirst && index == 0 {
			if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || response.Receipt.Feedback == nil || response.Receipt.Feedback.OriginalDiagnostic == nil || response.Receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil {
				t.Fatalf("weak input was not rejected with typed diagnostic: %+v", response.Receipt)
			}
			diagnostic := response.Receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic
			addition := diagnostic.InputAddition
			if addition.ContentContractRef != "shell/go-test-input/conjunctive-entrypoint/v1" {
				t.Fatalf("default adapter did not publish the versioned entrypoint input contract: %+v", addition)
			}
			convergence := focus.ValidationFailureRepairConvergence
			if convergence == nil {
				t.Fatal("default CWI lacks repair policy")
			}
			route, found := convergence.RouteForTrigger(runtimecontract.RuntimeModuleShell, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode)
			if !found || !route.RequireInputAddition || len(route.RepairOperationKinds) != 1 || route.RepairOperationKinds[0] != runtimecontract.StepKindFileWrite || !addition.MatchesBinding(coverageBinding) {
				t.Fatalf("default Profile has no exact create-only input contract: diagnostic=%+v route=%+v", diagnostic, route)
			}
			originalBinding := ownerBinding.Clone()
			content := fmt.Sprintf("package workorch_validation\nimport (\"testing\"; subject \"example.com/workorch-default-native-proof\")\nfunc %s(t *testing.T) { if subject.DeliveredValue() != 4 { t.Fatal(\"delivery entry returned wrong value\") } }\n", addition.NativeCheckRef)
			created := executeFileWrite(runtimecontract.RuntimeOperation{ID: "supplement-input", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{addition.TargetRef}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}}}, "dispatch-create-supplement")
			if _, credit, _ := coverageEvidenceCreditForReceipt([]runtimecontract.CoverageEvidenceBinding{coverageBinding}, operation, runtimecontract.EvidenceKindValidation, &created); len(credit) != 0 {
				t.Fatal("input creation credited validation")
			}
			observedManifest = sendObservedManifest(t, ctx, hub, aggregate, manifestPrerequisite, workspaceDirectory)
			reobserved = acquireObservedShellValidationCatalog(t, ctx, hub, aggregate, shellPrerequisite, workspaceDirectory, observedManifest, observedManifest.Manifest.TargetRefs, requirements)
			catalog = reobserved.Acquisition.Admissibility.Normalized()
			_, ownerBinding, found = runtimecontract.RuntimeValidationSurfaceForProofRequirement(catalog.ValidationSurfaces, requirement)
			if !found || len(ownerBinding.CheckAttestations) != 1 || ownerBinding.CheckAttestations[0].NativeCheckRef != originalBinding.CheckAttestations[0].NativeCheckRef || ownerBinding.CheckAttestations[0].SupplementalNativeCheckRef != addition.NativeCheckRef || ownerBinding.CheckAttestations[0].ObservationProtocolRef != originalBinding.CheckAttestations[0].ObservationProtocolRef {
				t.Fatalf("fresh owner binding replaced original instead of adding all-of: %+v", ownerBinding)
			}
			if ownerBinding.InputReview == nil || ownerBinding.InputReview.Hash == originalBinding.InputReview.Hash || len(ownerBinding.InputReview.Files) != 3 {
				t.Fatal("supplement did not require a fresh complete input review")
			}
			coverageBinding, semanticFocus = defaultNativeProofCoverageBinding(t, focus, requirement, ownerBinding, preparedTarget)
			response = executeBoundValidation("dispatch-default-proof-fresh-validation")
		}

		if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || response.Receipt.Evidence.Validation == nil {
			t.Fatalf("Shell native proof %q receipt = %+v", requirement.Hash, response.Receipt)
		}
		if response.Receipt.Evidence.Validation.ProofClass != runtimecontract.RuntimeValidationProofBehavioral || response.Receipt.Evidence.Validation.ValidateFailureDetails() != nil {
			t.Fatalf("real owner behavioral observation lost typed modality: %+v", response.Receipt.Evidence.Validation)
		}
		bindingRefs, coverageIDs, diagnostic := coverageEvidenceCreditForReceipt(
			[]runtimecontract.CoverageEvidenceBinding{coverageBinding}, operation,
			runtimecontract.EvidenceKindValidation, &response.Receipt,
		)
		if diagnostic != "" || len(bindingRefs) != 1 || bindingRefs[0] != coverageBinding.ID ||
			len(coverageIDs) != 1 || coverageIDs[0] != requirement.CoverageWorkItemID {
			t.Fatalf("SM-2 exact native credit %q: bindings=%v coverage=%v diagnostic=%q", requirement.Hash, bindingRefs, coverageIDs, diagnostic)
		}
		if index == 0 {
			firstReceipt, firstBinding, firstOperation = response.Receipt, coverageBinding, operation
		}
	}

	// A generic test name or raw success cannot satisfy the immutable CWI
	// marker, even when recipe and protocol are copied from a real receipt.
	generic := firstReceipt.Normalized()
	observation := generic.Evidence.Validation.Clone()
	observation.ExecutedChecks = []string{"TestGenericWorkspacePass"}
	observation.PassedChecks = []string{"TestGenericWorkspacePass"}
	observation.FailedChecks = nil
	observation.SkippedChecks = nil
	generic.Evidence.Validation = &observation
	_, genericCredit, diagnostic := coverageEvidenceCreditForReceipt(
		[]runtimecontract.CoverageEvidenceBinding{firstBinding}, firstOperation,
		runtimecontract.EvidenceKindValidation, &generic,
	)
	if len(genericCredit) != 0 || !strings.Contains(diagnostic, "validation observation does not satisfy sealed proof") {
		t.Fatalf("generic Go test incorrectly closed exact CWI: credit=%v diagnostic=%q", genericCredit, diagnostic)
	}
}

func builtinSoftwareDevNativeProofBootstrap(t *testing.T) runtimecontract.RuntimeBootstrap {
	t.Helper()
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.ID != "software-dev" {
			continue
		}
		policies, err := workorcheventpayload.FromMapSlice(template.EvidencePolicies)
		if err != nil {
			t.Fatal(err)
		}
		activation, err := activationports.RuntimeActivationContext(
			runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "test-provider", ModelID: "test-model"}},
			"zh-CN", nil, nil, appmodelcore.SoulRouting{}, nil, nil, nil, nil, nil,
			policies, template.ContextHints, runtimecontract.RuntimeContinuationContext{},
		)
		if err != nil {
			t.Fatal(err)
		}
		bootstrap, err := activationcore.RuntimeBootstrap(
			&workspacemodel.Goal{ID: "goal-default-native-proof", WorkspaceID: "workspace-default-native-proof", Text: template.GoalText},
			&workspacemodel.DoD{ID: "dod-default-native-proof", WorkspaceID: "workspace-default-native-proof", Items: append([]string(nil), template.DoDItems...)},
			"definition-default-native-proof", "startup-default-native-proof", nil, activation,
		)
		if err != nil {
			t.Fatal(err)
		}
		return *bootstrap
	}
	t.Fatal("software-dev built-in template is missing")
	return runtimecontract.RuntimeBootstrap{}
}

func builtinSoftwareDevDoDItems(t *testing.T) []string {
	t.Helper()
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.ID == "software-dev" {
			return append([]string(nil), template.DoDItems...)
		}
	}
	t.Fatal("software-dev built-in template is missing")
	return nil
}

func defaultNativeProofPreparations(
	t *testing.T,
	catalog runtimecontract.RuntimeOperationAdmissibility,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
) map[string]runtimecontract.RuntimeValidationProofPreparation {
	t.Helper()
	ret := make(map[string]runtimecontract.RuntimeValidationProofPreparation, len(requirements))
	for _, surface := range catalog.Normalized().ValidationSurfaces {
		for _, preparation := range surface.ProofRequirementPreparations {
			if _, duplicate := ret[preparation.RequirementHash]; duplicate {
				t.Fatalf("default proof requirement %q has duplicate preparations", preparation.RequirementHash)
			}
			ret[preparation.RequirementHash] = preparation.Clone()
		}
	}
	seenTargets := make(map[string]string, len(ret))
	for _, requirement := range requirements {
		preparation, found := ret[requirement.Hash]
		if !found || preparation.TargetRef == "" || len(preparation.CheckAttestations) == 0 {
			t.Fatalf("default proof requirement %q has no exact owner preparation: %+v", requirement.Hash, catalog.ValidationSurfaces)
		}
		if previous, duplicate := seenTargets[preparation.TargetRef]; duplicate {
			t.Fatalf("default CWI preparations %q and %q share target %q", previous, requirement.Hash, preparation.TargetRef)
		}
		seenTargets[preparation.TargetRef] = requirement.Hash
	}
	return ret
}

func defaultNativeProofFileOperation(
	requirement runtimecontract.RuntimeValidationProofRequirement,
	preparation runtimecontract.RuntimeValidationProofPreparation,
	index int,
) runtimecontract.RuntimeOperation {
	var source strings.Builder
	source.WriteString("package workorch_validation\n\nimport (\"testing\"; subject \"example.com/workorch-default-native-proof\")\n\n")
	for _, attestation := range preparation.CheckAttestations {
		fmt.Fprintf(&source, "func %s(t *testing.T) {\n", attestation.NativeCheckRef)
		source.WriteString("\tif subject.DeliveredValue() != 4 { t.Fatal(\"owner-native validation assertion failed\") }\n")
		source.WriteString("}\n\n")
	}
	content := source.String()
	return runtimecontract.RuntimeOperation{
		ID:                   fmt.Sprintf("prepare-default-proof-%02d", index+1),
		Kind:                 runtimecontract.StepKindFileWrite,
		RequiredOperationRef: "prepare-validation-proof/" + requirement.Hash,
		TargetRefs:           []string{preparation.TargetRef},
		Parameters:           []runtimecontract.OperationParameter{{Name: "content", String: &content}},
	}
}

func defaultNativeProofModuleFileOperation() runtimecontract.RuntimeOperation {
	content := "module example.com/workorch-default-native-proof\n\ngo 1.26\n"
	return runtimecontract.RuntimeOperation{
		ID:                   "prepare-default-proof-module",
		Kind:                 runtimecontract.StepKindFileWrite,
		RequiredOperationRef: "prepare-validation-proof/go-module-root",
		TargetRefs:           []string{"go.mod"},
		Parameters:           []runtimecontract.OperationParameter{{Name: "content", String: &content}},
	}
}

func defaultNativeProofCoverageBinding(
	t *testing.T,
	definitionFocus runtimecontract.RuntimeDefinitionFocus,
	requirement runtimecontract.RuntimeValidationProofRequirement,
	ownerBinding runtimecontract.RuntimeValidationProofRequirementBinding,
	preparedTarget string,
) (runtimecontract.CoverageEvidenceBinding, runtimecontract.CoverageFocus) {
	t.Helper()
	// The concrete target is not copied from the Definition declaration. It is
	// the exact Shell-authored preparation target that File just attested, which
	// is the generation-specific prospective target SM-2 is allowed to seal.
	focus := definitionFocus.CoverageFocusDeclaration()
	focus.ProspectiveTargetRefs = []string{preparedTarget}
	focus = focus.Normalized()
	binding := runtimecontract.CoverageEvidenceBinding{
		ID:                 "binding/default-native/" + requirement.Hash,
		CoverageWorkItemID: requirement.CoverageWorkItemID,
		FocusID:            focus.ID,
		DefinitionRevision: requirement.DefinitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind:                   runtimecontract.EvidenceKindValidation,
			RequiredEffectScopes:           []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
			AllowedOperationKinds:          []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			ValidationRecipeID:             ownerBinding.ValidationRecipeID,
			ValidationProofRequirementHash: requirement.Hash,
			RequiredCheckRefs:              append([]string(nil), requirement.RequiredCheckRefs...),
			ValidationCheckAttestations:    runtimecontract.CloneRuntimeValidationCheckAttestationBindings(ownerBinding.CheckAttestations),
		}},
		TargetRefs:                           append([]string(nil), focus.TargetRefs...),
		TargetSelectorRef:                    focus.TargetSelectorRef,
		TargetSelectorOwnerRef:               focus.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef:             focus.TargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:          append([]string(nil), focus.TargetSelectorCandidateRefs...),
		ProspectiveTargetAllOfConstraintRefs: append([]string(nil), focus.ProspectiveTargetAllOfConstraintRefs...),
		ProspectiveTargetRefs:                []string{preparedTarget},
		DeclarationRef:                       requirement.DeclarationRef,
	}
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		t.Fatalf("default exact coverage binding %q: %v; focus=%+v", requirement.Hash, err, focus)
	}
	focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
	if err := runtimecontract.ValidateCoverageEvidenceBindingsForFocus(focus, requirement.DefinitionRevision); err != nil {
		t.Fatalf("default SM-2 focus binding %q: %v", requirement.Hash, err)
	}
	return binding, focus
}

func containsDefaultNativeProofString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func defaultNativeProofExecutionEnvelope(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	dispatchID string,
	operation runtimecontract.RuntimeOperation,
	catalog *runtimecontract.RuntimeOperationAdmissibility,
) runtimecontract.RuntimeOperationExecutionEnvelope {
	policies := []runtimecontract.RuntimeOperationPolicyFact(nil)
	record := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID:   operation.ID,
		OperationHash: runtimecontract.StableRuntimeOperationHash(operation),
		ExecutionKey:  runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, dispatchID, operation.ID),
		Fence:         1,
	}
	if catalog != nil {
		policies = catalog.OperationPolicies
		record.AdmissibilitySnapshotID = catalog.SnapshotID
		record.AcquisitionBinding = catalog.AcquisitionBinding
	}
	record.ExpectedEffectScope = runtimecontract.ResolveExpectedEffectScope(operation, policies)
	record.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, policies)
	record.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(record)
	return runtimeOperationExecutionEnvelopeForTest(record, nil)
}

func sendDefaultNativeProofFileWrite(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	command fileevents.ExecuteFileWriteOperationCommand,
) fileevents.ExecuteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecuteFileWriteOperation,
		runtimecontract.RuntimeModuleOrchestrator,
		runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("File owner returned no preparation result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("File preparation result type = %T", data)
	}
	return response
}

func registerDefaultNativeProofReceiptOwner(t *testing.T, hub magicevent.Hub) {
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
