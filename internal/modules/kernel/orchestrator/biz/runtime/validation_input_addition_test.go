package runtime

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationInputAdditionOwnerFactIsBoundedAndRestartStable(t *testing.T) {
	for _, scenario := range []string{"exact", "missing_contract", "wrong_requirement", "wrong_original", "second_supplement"} {
		t.Run(scenario, func(t *testing.T) {
			convergence := runtimeValidationFailureRepairConvergenceForTest(1)
			route := runtimeValidationFailureRepairRouteForTest("missing-input", runtimecontract.ValidationDiagnosticOutputInconclusive, runtimecontract.ValidationEvidenceTargetValidationOutput)
			route.TriggerOwnerDiagnosticCode = "owner_input_missing"
			route.RequireInputAddition = true
			convergence.Routes = []runtimecontract.RuntimeValidationFailureRepairRoute{route}
			aggregate := runtimeValidationFailureRepairAggregateForTest(t, convergence, func(initial *RuntimeAggregate) {
				initial.Facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
				initial.Facts.DefinitionFocuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
				initial.Facts.DefinitionFocuses = runtimecontract.NormalizeRuntimeDefinitionFocuses(initial.Facts.DefinitionFocuses)
			})
			repository := &durableRuntimeStateRepository{aggregate: aggregate}
			coordinator, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			sealed := sealCandidateForTest(t, repository, validationCandidate())
			binding := sealed.ActiveDispatch.CoverageBindings[0]
			var check runtimecontract.RuntimeValidationCheckAttestationBinding
			for _, proof := range binding.Proofs {
				if len(proof.ValidationCheckAttestations) > 0 {
					check = proof.ValidationCheckAttestations[0]
				}
			}
			if check.RequiredCheckRef == "" {
				t.Fatalf("fixture lacks sealed native requirement: %+v", binding)
			}
			addition := runtimecontract.RuntimeValidationInputAddition{RequiredCheckRef: check.RequiredCheckRef, OriginalNativeCheckRef: check.NativeCheckRef, NativeCheckRef: "owner/native/supplement", TargetRef: "checks/supplement.test"}
			evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-input-addition", runtimecontract.ValidationDiagnosticOutputInconclusive)
			diagnostic := evidence.Failure.ValidationDiagnostic
			diagnostic.RecommendedEvidenceTargetType = runtimecontract.ValidationEvidenceTargetValidationOutput
			diagnostic.OwnerCode = route.TriggerOwnerDiagnosticCode
			diagnostic.InputAddition = addition
			switch scenario {
			case "missing_contract":
				diagnostic.InputAddition = runtimecontract.RuntimeValidationInputAddition{}
			case "wrong_requirement":
				diagnostic.InputAddition.RequiredCheckRef = "another-requirement"
			case "wrong_original":
				diagnostic.InputAddition.OriginalNativeCheckRef = "another-original"
			case "second_supplement":
				diagnostic.OwnerCode = "owner_supplement_still_insufficient"
			}
			attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
			committed, err := coordinator.commitEvidence(context.Background(), evidence)
			if err != nil {
				t.Fatal(err)
			}
			input, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(committed.Aggregate)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "exact" {
				if !required || input.InputAddition != addition || committed.Aggregate.Facts.Coverage[0].CurrentBindingRef != nil || committed.Aggregate.Facts.Repair.Exhausted {
					t.Fatalf("exact owner input did not reopen one bounded generation: input=%+v facts=%+v", input, committed.Aggregate.Facts)
				}
			} else if required || committed.Aggregate.Decision.Terminal == nil || committed.Aggregate.Decision.Terminal.Code != "validation_failure_repair_route_unavailable" || len(committed.Aggregate.Facts.Coverage[0].ReopenLineage) != 0 {
				t.Fatalf("unusable input did not terminate in receipt CAS: %+v", committed.Aggregate)
			}
			reloaded, err := repository.Load(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			reloadedInput, reloadedRequired, err := CurrentRuntimeValidationFailureRepairGenerationInput(reloaded)
			if err != nil || reloadedInput != input || reloadedRequired != required {
				t.Fatalf("restart changed owner input: %+v %v %v", reloadedInput, reloadedRequired, err)
			}
			restarted, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := restarted.commitEvidence(context.Background(), evidence)
			if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != committed.Aggregate.StateRevision || duplicate.Aggregate.Facts.Repair.RepairAttempts != 1 {
				t.Fatalf("duplicate consumed another repair: %+v %v", duplicate, err)
			}
			if scenario != "exact" {
				return
			}

			manifest, proposal, feasibility := runtimeValidationFailureRepairProofForTest(t, reloaded, []string{addition.TargetRef})
			repair := manifest.ProofObligations[0].ValidationFailureRepair
			if repair == nil || !repair.RequireInputAddition || repair.InputAddition != addition {
				t.Fatalf("input contract not sealed: %+v", repair)
			}
			schema, err := runtimecontract.RuntimeSemanticPlanJSONSchemaForRequest(manifest, runtimecontract.RuntimeSemanticPlanStepProofBatch, "repair-batch", 0, []common.CoverageWorkItemID{input.CoverageWorkItemID})
			if err != nil || !strings.Contains(schema, addition.TargetRef) {
				t.Fatalf("input missing from strict schema: %s %v", schema, err)
			}
			for _, targets := range [][]string{{"checks/original.test"}, {addition.TargetRef, "unrelated.test"}} {
				wrong := proposal.Normalized()
				wrong.Target.ProspectiveTargetRefs = targets
				document := runtimecontract.RuntimeSemanticPlanDocument{
					SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
					PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: "repair-batch", BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{input.CoverageWorkItemID}, Proposals: []runtimecontract.RuntimeSemanticProofProposal{wrong},
				}
				if err := document.ValidateAgainstManifest(manifest); err == nil {
					t.Fatalf("input contract widened to %+v", targets)
				}
			}
			semantic := runtimeSealValidationFailureRepairSemanticProofForTest(t, repository, manifest, proposal, feasibility)
			if !reflect.DeepEqual(semantic.Definition, sealed.Definition) || !reflect.DeepEqual(semantic.Facts.DefinitionFocuses, sealed.Facts.DefinitionFocuses) {
				t.Fatal("supplement changed immutable Definition/Profile")
			}
			binding, found := coverageEvidenceBindingForCoverage(semantic.Facts.Focus, input.CoverageWorkItemID)
			if !found || len(binding.ProspectiveTargetRefs) != 1 || binding.ProspectiveTargetRefs[0] != addition.TargetRef {
				t.Fatalf("repair target binding = %+v", binding)
			}
			sequence := semantic.Decision.RequiredOutput.RequiredOperationSequence
			if len(sequence) != 1 {
				t.Fatalf("supplement requires exactly one create: %+v", sequence)
			}
			operation := runtimecontract.RuntimeOperation{ID: "create-input", Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{binding.ID}, TargetRefs: []string{addition.TargetRef}, RequiredOperationRef: sequence[0].RequirementID, Parameters: runtimeFileWriteParameters("supplemental input")}
			catalog := runtimeValidationFailureRepairCatalogForOwnerTest(t, manifest, runtimecontract.RuntimeModuleFile)
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}
			dispatch := sealCandidateWithFeasibilitiesForTest(t, repository, candidate, []runtimecontract.RuntimeCandidateFeasibilityRecord{{OperationID: operation.ID, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding, Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}}})
			stateEvidence := ExecutionEvidence{EventID: "input-created", WorkspaceID: dispatch.Facts.WorkspaceID, RunID: dispatch.Facts.RunID, DispatchID: dispatch.ActiveDispatch.ID, AuthoritySnapshotID: dispatch.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: runtimeTestActiveOperationID(t, dispatch, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityValid}}
			stale := stateEvidence
			stale.EventID, stale.AuthoritySnapshotID = "input-created-stale", "old-authority"
			staleResult, err := restarted.commitEvidence(context.Background(), stale)
			if err != nil || staleResult.Disposition != CommitDispositionStale || !reflect.DeepEqual(staleResult.Aggregate.Facts.Coverage, dispatch.Facts.Coverage) || staleResult.Aggregate.Facts.Repair.RepairAttempts != dispatch.Facts.Repair.RepairAttempts {
				t.Fatalf("stale input effect changed convergence: disposition=%s err=%v", staleResult.Disposition, err)
			}
			attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &stateEvidence)
			settled, err := restarted.commitEvidence(context.Background(), stateEvidence)
			if err != nil {
				t.Fatal(err)
			}
			item := settled.Aggregate.Facts.Coverage[0]
			if !settled.DispatchCompleted || item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.CurrentBindingRef != nil || runtimeValidationFailureRepairReopenReason(item) != runtimeCWIReopenReasonValidationFailureRepairSettled {
				t.Fatalf("create receipt gave terminal credit or skipped fresh validation: %+v", settled.Aggregate)
			}
			if _, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(settled.Aggregate); err != nil || required {
				t.Fatalf("create started another input addition: %v %v", required, err)
			}
			duplicate, err = restarted.commitEvidence(context.Background(), stateEvidence)
			if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != settled.Aggregate.StateRevision {
				t.Fatalf("replayed create changed convergence: %+v %v", duplicate, err)
			}
		})
	}
}
