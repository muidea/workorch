package orchestrator

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeShellExecutorSealsExactPersistedValidationAttestation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })

	request := runtimeShellValidationExecutionRequest(t)
	var captured []shellevents.ExecuteOperationCommand
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleShell, hub)
	observer.Subscribe(shellevents.TopicExecuteOperation, func(event magicevent.Event, result magicevent.Result) {
		captured = append(captured, event.Data().(shellevents.ExecuteOperationCommand))
		result.Set(shellevents.ExecuteOperationResult{}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(shellevents.TopicExecuteOperation) })

	executor := eventRuntimeOperationExecutor{hub: hub}
	if _, err := executor.executeShell(ctx, request, "/tmp/runtime-validation-proof", "/tmp/runtime-validation-proof-internal"); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 {
		t.Fatalf("Shell execute commands = %d, want 1", len(captured))
	}
	_, authorization, err := captured[0].SealedOperation.ValidateForExecution(captured[0].RuntimeOperationExecutionContext)
	if err != nil || authorization.ValidationProofBinding.RequirementHash != request.ValidationProofBinding.RequirementHash ||
		!runtimecontract.SameRuntimeValidationCheckAttestationBindings(authorization.ValidationProofBinding.CheckAttestations, request.ValidationProofBinding.CheckAttestations) {
		t.Fatalf("sealed Shell authorization lost exact attestation: authorization=%+v err=%v", authorization, err)
	}
	firstPayloadHash := captured[0].SealedOperation.PayloadHash
	if _, err := executor.executeShell(ctx, request, "/tmp/runtime-validation-proof", "/tmp/runtime-validation-proof-internal"); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 || captured[1].SealedOperation.PayloadHash != firstPayloadHash {
		t.Fatalf("restart/duplicate reseal changed authorization: first=%q commands=%+v", firstPayloadHash, captured)
	}

	missing := request
	missing.ValidationProofBinding = runtimecontract.RuntimeValidationProofRequirementBinding{}
	if _, err := executor.executeShell(ctx, missing, "/tmp/runtime-validation-proof", "/tmp/runtime-validation-proof-internal"); err == nil || !strings.Contains(err.Error(), "selected validation proof binding") {
		t.Fatalf("missing persisted proof binding err=%v", err)
	}
	drifted := request
	drifted.ValidationProofBinding = request.ValidationProofBinding.Clone()
	drifted.ValidationProofBinding.CheckAttestations[0].NativeCheckRef = "candidate/native/drift"
	if _, err := executor.executeShell(ctx, drifted, "/tmp/runtime-validation-proof", "/tmp/runtime-validation-proof-internal"); err == nil || !strings.Contains(err.Error(), "selected validation proof binding") {
		t.Fatalf("candidate/attestation drift err=%v", err)
	}
}

func runtimeShellValidationExecutionRequest(t *testing.T) runtime.RuntimeOperationExecutionRequest {
	t.Helper()
	item := runtimecontract.RuntimeCoverageItem{ID: "cwi-shell-execution", SliceID: "slice-shell-execution", SourceScope: "dodItem", SourceID: "dod-shell-execution", TargetText: "typed owner check passes"}
	focus := runtimecontract.RuntimeDefinitionFocus{
		ID: "focus-shell-execution", SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition-shell-execution", []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeDefinitionFocus{focus})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile validation requirement: requirements=%+v err=%v", requirements, err)
	}
	const recipeID = "recipe/shell-execution"
	attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(requirements[0].RequiredCheckRefs))
	for index, checkRef := range requirements[0].RequiredCheckRefs {
		attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: checkRef, ObservationProtocolRef: "owner/opaque-observation/v1",
			NativeCheckRef: "owner/native/check/" + string(rune('a'+index)),
		})
	}
	binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirements[0], recipeID, attestations...)
	if err != nil {
		t.Fatal(err)
	}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: recipeID, Command: "owner-validation", Args: []string{"opaque"}, TimeoutMillis: 1000}
	policy := runtimecontract.RuntimeOperationPolicyFact{
		ID: "shell-managed-validation-policy", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, SubjectRef: recipeID,
		ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{{Name: "recipeId", String: runtimeTestStringPointer(recipeID)}},
		EffectScope:      runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-validation-admissibility", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"validation_target.go"},
		ValidationRecipeRefs: []string{recipeID}, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: recipeID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "schema/shell-execution", InputBindingRef: "input/shell-execution",
			CheckRefs: append([]string(nil), requirements[0].RequiredCheckRefs...), ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{binding},
			EffectScope:              runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{policy}, ObservedRevision: "shell-validation-owner-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			SubjectRef: "shell_runtime_policy", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-shell-execution", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{"binding/shell-execution"}, TargetRefs: []string{"validation_target.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: runtimeTestStringPointer(recipeID)}},
	}
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: runtimecontract.StableRuntimeOperationHash(operation),
		ExecutionKey: "exec/ws-shell/run-shell/dispatch-shell/validate-shell-execution", Fence: 1,
		ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
	return runtime.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-shell", RunID: "run-shell", DispatchID: "dispatch-shell", Operation: operation, Execution: execution,
		EvidenceKind: runtimecontract.EvidenceKindValidation, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		ValidationScopeTargetRefs: []string{"validation_target.go"}, ValidationProofBinding: binding,
		OwnerCatalog: &catalog, OwnerCatalogObservation: &observation,
	}
}

func runtimeTestStringPointer(value string) *string { return &value }

func TestRuntimeOperationExecutionEventBindsExactExecutionLane(t *testing.T) {
	base := runtime.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-lane", RunID: "run-lane",
		Execution: runtimecontract.RuntimeOperationExecutionRecord{ExecutionKey: "exec/ws-lane/run-lane/dispatch-1/operation-1", Fence: 1},
	}
	tests := []struct {
		name        string
		contracts   []eventcontract.Contract
		topic       string
		destination string
		laneOwner   string
	}{
		{name: "file_list", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileListOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "file_read", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileReadOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "file_edit", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileEditOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "file_write", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileWriteOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "patch_apply", contracts: fileevents.Contracts(), topic: fileevents.TopicExecutePatchApplyOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "patch_preview", contracts: fileevents.Contracts(), topic: fileevents.TopicExecutePatchPreviewOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "git", contracts: gitevents.Contracts(), topic: gitevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleGit, laneOwner: "git"},
		{name: "shell", contracts: shellevents.Contracts(), topic: shellevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleShell, laneOwner: "shell"},
		{name: "ssh", contracts: sshevents.Contracts(), topic: sshevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleSSH, laneOwner: "ssh"},
		{name: "web", contracts: webevents.Contracts(), topic: webevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleWeb, laneOwner: "web"},
		{name: "mcp", contracts: mcpevents.Contracts(), topic: mcpevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleMCP, laneOwner: "mcp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contractFound := false
			for _, contract := range test.contracts {
				if contract.Topic == test.topic {
					contractFound = true
					if contract.LanePolicy != eventcontract.LanePolicyExecute {
						t.Fatalf("owner contract lane policy = %q, want %q", contract.LanePolicy, eventcontract.LanePolicyExecute)
					}
					if contract.Metadata.EffectClass != eventcontract.EffectClassExternalSideEffect {
						t.Fatalf("owner execute effect class = %q, want %q", contract.Metadata.EffectClass, eventcontract.EffectClassExternalSideEffect)
					}
				}
			}
			if !contractFound {
				t.Fatalf("execute topic %q is absent from receiver contracts", test.topic)
			}

			event := runtimeOperationExecutionEvent(context.Background(), test.contracts, test.topic, test.destination, base, struct{}{})
			want := "exec/block/" + test.laneOwner + "/workspace/ws-lane/step/" + base.Execution.ExecutionKey
			if event.LaneKey() != want {
				t.Fatalf("operation execution lane = %q, want %q", event.LaneKey(), want)
			}

			retry := base
			retry.Execution.Fence = 2
			retryEvent := runtimeOperationExecutionEvent(context.Background(), test.contracts, test.topic, test.destination, retry, struct{}{})
			if retryEvent.LaneKey() != event.LaneKey() {
				t.Fatalf("same execution key with a new fence changed lane: first=%q retry=%q", event.LaneKey(), retryEvent.LaneKey())
			}

			other := base
			other.Execution.ExecutionKey = "exec/ws-lane/run-lane/dispatch-1/operation-2"
			otherEvent := runtimeOperationExecutionEvent(context.Background(), test.contracts, test.topic, test.destination, other, struct{}{})
			if otherEvent.LaneKey() == event.LaneKey() {
				t.Fatal("distinct execution keys shared one capability execution lane")
			}
		})
	}
}

func TestRuntimeOperationExecutionResultUsesOnlyPersistedReceiptGovernance(t *testing.T) {
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "approval_required",
		Message: "ordinary operation text must not create governance",
	}
	withoutGovernance := runtimeOperationExecutionResult(
		runtime.RuntimeOperationExecutionRequest{DispatchID: "dispatch-1", Operation: runtimecontract.RuntimeOperation{ID: "operation-1"}, EvidenceKind: runtimecontract.EvidenceKindStateChange},
		runtimecontract.RuntimeOperationReceipt{
			Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
			Feedback: &runtimecontract.RuntimeFeedback{
				Domain: runtimecontract.RuntimeFeedbackDomainOperation, StableCategory: runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected),
				OriginalDiagnostic: runtimecontract.RuntimeOriginalDiagnosticFromFailureFact(runtimecontract.RuntimeModuleFile, *failure),
			},
		},
	)
	if withoutGovernance.Governance != nil {
		t.Fatalf("failure text unexpectedly inferred governance: %+v", withoutGovernance)
	}
	if withoutGovernance.Failure == nil || withoutGovernance.Failure.Code != "approval_required" {
		t.Fatalf("ordinary failure was not retained for repair: %+v", withoutGovernance)
	}
	if withoutGovernance.EventID != "runtime-operation|dispatch-1|operation-1" || withoutGovernance.SourceRef != "workorch://runtime-operations/dispatch-1/operation-1" || withoutGovernance.OperationID != "operation-1" || withoutGovernance.EvidenceKind != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("sealed operation identity was not projected by Orchestrator: %+v", withoutGovernance)
	}

	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: ".env", Reason: "sensitive workspace file",
		RequestedAction: runtimecontract.RuntimeBlockKindPatchApply, RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", ".env")),
	}
	withGovernance := runtimeOperationExecutionResult(
		runtime.RuntimeOperationExecutionRequest{DispatchID: "dispatch-2", Operation: runtimecontract.RuntimeOperation{ID: "operation-2"}, EvidenceKind: runtimecontract.EvidenceKindStateChange},
		runtimecontract.RuntimeOperationReceipt{Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, Governance: &requirement},
	)
	if withGovernance.Governance == nil || !runtimecontract.SameRuntimeGovernanceRequirement(*withGovernance.Governance, requirement) {
		t.Fatalf("persisted typed governance was not preserved: %+v", withGovernance)
	}
}

// SM-3 concrete producers must not reconstruct owner authorization from the
// operation or process configuration. The persisted SM-2 catalog and its
// observation are both mandatory before an EventHub execute command exists.
func TestCapabilityConcreteProducersRequirePersistedOwnerCatalog(t *testing.T) {
	executor := eventRuntimeOperationExecutor{}
	ctx := context.Background()
	for name, execute := range map[string]func(runtime.RuntimeOperationExecutionRequest) error{
		"shell": func(request runtime.RuntimeOperationExecutionRequest) error {
			_, err := executor.executeShell(ctx, request, "", "")
			return err
		},
		"web": func(request runtime.RuntimeOperationExecutionRequest) error {
			_, err := executor.executeWeb(ctx, request, "", "")
			return err
		},
		"ssh": func(request runtime.RuntimeOperationExecutionRequest) error {
			_, err := executor.executeSSH(ctx, request, "", "")
			return err
		},
	} {
		t.Run(name+"/missing_catalog", func(t *testing.T) {
			if err := execute(runtime.RuntimeOperationExecutionRequest{}); err == nil || !strings.Contains(err.Error(), "persisted owner catalog") {
				t.Fatalf("missing owner catalog err = %v", err)
			}
		})
		t.Run(name+"/missing_observation", func(t *testing.T) {
			catalog := runtimecontract.RuntimeOperationAdmissibility{}
			if err := execute(runtime.RuntimeOperationExecutionRequest{OwnerCatalog: &catalog}); err == nil || !strings.Contains(err.Error(), "persisted owner catalog") {
				t.Fatalf("missing owner catalog observation err = %v", err)
			}
		})
	}
	t.Run("mcp/missing_catalog", func(t *testing.T) {
		if _, err := executor.executeMCP(ctx, runtime.RuntimeOperationExecutionRequest{}, "", ""); err == nil || !strings.Contains(err.Error(), "persisted owner catalog") {
			t.Fatalf("missing MCP owner catalog err = %v", err)
		}
	})
}

func TestConcreteProducerCarriesSealedOwnerCatalogExecutionIdentity(t *testing.T) {
	record := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "web-1", ExecutionKey: "exec/ws/run/dispatch/web-1", Fence: 3,
		ExpectedEffectScope:     runtimecontract.RuntimeEffectNone,
		IdempotencyClass:        runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		AdmissibilitySnapshotID: "operation-admissibility-web-1",
		AcquisitionBinding:      "capability-state/ws/run/web/0",
	}
	record.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(record)
	envelope := runtimeOperationExecutionEnvelope(record, nil)
	if envelope.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || envelope.AcquisitionBinding != record.AcquisitionBinding ||
		envelope.IdempotencyClass != record.IdempotencyClass || envelope.AuthorityHash != record.AuthorityHash {
		t.Fatalf("concrete producer lost sealed owner catalog identity: record=%+v envelope=%+v", record, envelope)
	}
}
