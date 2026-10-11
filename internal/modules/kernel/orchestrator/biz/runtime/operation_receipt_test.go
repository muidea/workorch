package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeOperationFeedbackForTest(t *testing.T, aggregate RuntimeAggregate, operationID string, failure runtimecontract.FailureFact) *runtimecontract.RuntimeFeedback {
	t.Helper()
	if failure.Code == "" {
		failure.Code = string(failure.Class)
	}
	if failure.Signature == "" {
		failure.Signature = "test-operation-rejection"
	}
	if aggregate.ActiveDispatch == nil {
		t.Fatalf("feedback fixture requires active dispatch: aggregate=%+v", aggregate.ActiveDispatch)
	}
	operation, found := candidateOperationByID(aggregate.ActiveDispatch.Candidate, operationID)
	if !found {
		t.Fatalf("feedback fixture operation %q is not sealed", operationID)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found {
		t.Fatalf("feedback fixture operation kind %q is unknown", operation.Kind)
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: aggregate.ActiveDispatch.ID,
	}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected), descriptor.OwnerModuleID, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = operationID
	return &feedback
}

func TestRuntimeControllerOperationReceiptFenceAndTerminalUniqueness(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("receipt")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	sealedExecution, ok := operationExecutionRecord(sealed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("sealed execution is missing")
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, operationID, sealedExecution.ExecutionKey, 1, runtimecontract.RuntimeModuleFile, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unclaimed receipt query err = %v, want conflict", err)
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("claimed execution is missing")
	}
	claimedBudget, err := runtimecontract.BuildRuntimeExecutionBudgetView(claimed.Facts.WorkspaceID, claimed.Facts.RunID, claimed.StateRevision, claimed.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, claimedBudget, runtimecontract.RuntimeBudgetDimensionCapabilityOperation, 0, 1, 0)
	if _, found, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", claimed.ActiveDispatch.ID, operationID, execution.ExecutionKey, execution.Fence, runtimecontract.RuntimeModuleFile, nil); err != nil || found {
		t.Fatalf("claimed receipt query found=%v err=%v, want authorized found=false", found, err)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", "different-dispatch", operationID, execution.ExecutionKey, execution.Fence, runtimecontract.RuntimeModuleFile, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong dispatch receipt query err = %v, want conflict", err)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", claimed.ActiveDispatch.ID, operationID, execution.ExecutionKey+"-wrong", execution.Fence, runtimecontract.RuntimeModuleFile, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong execution key receipt query err = %v, want conflict", err)
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "written"},
	}
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence+1, runtimecontract.RuntimeModuleFile, receipt); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale fence err = %v, want conflict", err)
	}
	committed, first, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt)
	if err != nil {
		t.Fatal(err)
	}
	committedBudget, err := runtimecontract.BuildRuntimeExecutionBudgetView(committed.Facts.WorkspaceID, committed.Facts.RunID, committed.StateRevision, committed.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, committedBudget, runtimecontract.RuntimeBudgetDimensionCapabilityOperation, 1, 0, 0)
	queried, found, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", claimed.ActiveDispatch.ID, operationID, execution.ExecutionKey, execution.Fence, runtimecontract.RuntimeModuleFile, nil)
	if err != nil || !found || !runtimecontract.SameRuntimeOperationReceipt(first, queried) {
		t.Fatalf("committed receipt query found=%v receipt=%+v err=%v", found, queried, err)
	}
	saves := repository.saves
	operation, found := candidateOperationByID(claimed.ActiveDispatch.Candidate, operationID)
	if !found {
		t.Fatal("missing original operation")
	}
	source := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: claimed.Facts.WorkspaceID, RunID: claimed.Facts.RunID, DispatchID: claimed.ActiveDispatch.ID, OperationID: operationID,
		Transaction: claimed.ActiveDispatch.Transaction, TargetRefs: operation.TargetRefs, CoverageWorkItemIDs: operation.CoverageWorkItemIDs,
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationHash: execution.OperationHash,
			ExpectedEffectScope: execution.ExpectedEffectScope, IdempotencyClass: execution.IdempotencyClass,
			AuthorityHash: execution.AuthorityHash, AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID, AcquisitionBinding: execution.AcquisitionBinding, ReplayProof: execution.ReplayProof,
		},
	}
	if read, err := controller.queryOperationReceiptWithReadSource(context.Background(), source, runtimecontract.RuntimeModuleFile); err != nil || !runtimecontract.SameRuntimeOperationReceipt(read, first) || repository.saves != saves {
		t.Fatalf("original source read err=%v saves=%d", err, repository.saves)
	}
	if read, found, err := controller.QueryOperationReceipt(context.Background(), source.WorkspaceID, source.RunID, source.DispatchID, source.OperationID, source.Execution.ExecutionKey, source.Execution.Fence, runtimecontract.RuntimeModuleFile, nil, source); err != nil || !found || !runtimecontract.SameRuntimeOperationReceipt(read, first) || repository.saves != saves {
		t.Fatalf("source query facade err=%v saves=%d", err, repository.saves)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), source.WorkspaceID, source.RunID, source.DispatchID, source.OperationID, source.Execution.ExecutionKey, source.Execution.Fence, runtimecontract.RuntimeModuleFile, nil, source, source); err == nil {
		t.Fatal("multiple source identities accepted")
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if read, err := restarted.queryOperationReceiptWithReadSource(context.Background(), source, runtimecontract.RuntimeModuleFile); err != nil || !runtimecontract.SameRuntimeOperationReceipt(read, first) || repository.saves != saves {
		t.Fatalf("source reload err=%v saves=%d", err, repository.saves)
	}
	for _, scenario := range []string{"definition", "transaction", "operation", "target", "coverage", "authority", "fence", "source", "coherent_operation", "coherent_catalog"} {
		t.Run("read_source_"+scenario, func(t *testing.T) {
			value := runtimecontract.CloneRuntimeOperationExecutionContext(source)
			owner := runtimecontract.RuntimeModuleFile
			switch scenario {
			case "definition":
				value.Transaction.DefinitionRevision = "other"
			case "transaction":
				value.Transaction.EvidenceRevision = "other"
			case "operation":
				value.Execution.OperationHash = "other"
			case "target":
				value.TargetRefs = []string{"other.txt"}
			case "coverage":
				value.CoverageWorkItemIDs = []common.CoverageWorkItemID{"other"}
			case "authority":
				value.Execution.AuthorityHash = "other"
			case "fence":
				value.Execution.Fence++
			case "source":
				owner = runtimecontract.RuntimeModuleShell
			case "coherent_operation":
				changed := operation
				changed.Parameters = runtimeFileWriteParameters("different")
				value.Execution.OperationHash = runtimecontract.StableRuntimeOperationHash(changed)
				value.Execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
					OperationID: changed.ID, OperationHash: value.Execution.OperationHash, ExecutionKey: value.Execution.ExecutionKey,
					ExpectedEffectScope: value.Execution.ExpectedEffectScope, IdempotencyClass: value.Execution.IdempotencyClass,
					AdmissibilitySnapshotID: value.Execution.AdmissibilitySnapshotID, AcquisitionBinding: value.Execution.AcquisitionBinding, ReplayProof: value.Execution.ReplayProof,
				})
			case "coherent_catalog":
				value.Execution.AdmissibilitySnapshotID = "different"
				value.Execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
					OperationID: operation.ID, OperationHash: value.Execution.OperationHash, ExecutionKey: value.Execution.ExecutionKey,
					ExpectedEffectScope: value.Execution.ExpectedEffectScope, IdempotencyClass: value.Execution.IdempotencyClass,
					AdmissibilitySnapshotID: value.Execution.AdmissibilitySnapshotID, AcquisitionBinding: value.Execution.AcquisitionBinding, ReplayProof: value.Execution.ReplayProof,
				})
			}
			if _, err := controller.queryOperationReceiptWithReadSource(context.Background(), value, owner); err == nil || repository.saves != saves {
				t.Fatalf("source drift accepted err=%v saves=%d", err, repository.saves)
			}
		})
	}
	replayed, duplicate, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt)
	if err != nil || !runtimecontract.SameRuntimeOperationReceipt(first, duplicate) || replayed.StateRevision != committed.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate receipt was not idempotent: aggregate=%+v receipt=%+v saves=%d err=%v", replayed, duplicate, repository.saves, err)
	}
	conflict := receipt
	conflict.Evidence.Summary = "different terminal outcome"
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, conflict); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different receipt for same execution key err = %v, want conflict", err)
	}
	terminated, err := controller.requestTermination(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeTerminationSuperseded, "blocks/run", "replacement")
	if err != nil {
		t.Fatal(err)
	}
	saves = repository.saves
	replayed, duplicate, err = controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt)
	if err != nil || !runtimecontract.SameRuntimeOperationReceipt(first, duplicate) || replayed.StateRevision != terminated.StateRevision || repository.saves != saves {
		t.Fatalf("post-termination exact receipt replay was not idempotent: aggregate=%+v receipt=%+v saves=%d err=%v", replayed, duplicate, repository.saves, err)
	}
	if _, _, err := controller.candidateDispatch.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleShell, receipt); !errors.Is(err, common.ErrConflict) || repository.saves != saves {
		t.Fatalf("post-termination forged receipt source err=%v saves=%d, want conflict/%d", err, repository.saves, saves)
	}
}

func TestManagedValidationReceiptRejectsForeignOrMismatchedValidationFailureObservation(t *testing.T) {
	t.Run("non-managed owners", func(t *testing.T) {
		for _, kind := range []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindShellCommand} {
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !found {
				t.Fatalf("descriptor %q is missing", kind)
			}
			observation := runtimecontract.RuntimeValidationFailureObservation{
				WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1", OperationID: "operation-1",
				FailureSignature: "failure-1", TargetRole: "implementation", DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
				CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("a", 64),
			}
			diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: descriptor.OwnerModuleID,
				OriginalClass: string(runtimecontract.FailureClassBuildFailure), OriginalCode: "failed", OriginalMessage: "operation failed", Signature: observation.FailureSignature,
				OperationID: observation.OperationID, ValidationFailureObservation: &observation,
			}
			if err := validateOperationReceiptDiagnosticOwner(RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-1", RunID: "run-1"}}, SealedDispatch{}, runtimecontract.RuntimeOperation{ID: observation.OperationID, Kind: kind}, diagnostic); err == nil ||
				!strings.Contains(err.Error(), "does not admit validation failure candidates") {
				t.Fatalf("kind %q foreign observation error = %v", kind, err)
			}
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeValidationFailureObservation, *runtimecontract.ValidationFailureDiagnostic)
	}{
		{name: "workspace", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.WorkspaceID = "ws-other"
		}},
		{name: "run", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.RunID = "run-other"
		}},
		{name: "authority", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.AuthoritySnapshotID = "authority-other"
		}},
		{name: "dispatch", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.DispatchID = "dispatch-other"
		}},
		{name: "operation", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.OperationID = "operation-other"
		}},
		{name: "role", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.TargetRole = "validation-entry"
		}},
		{name: "class", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.DiagnosticClass = runtimecontract.ValidationDiagnosticTestFailed
		}},
		{name: "diagnostic operation", mutate: func(_ *runtimecontract.RuntimeValidationFailureObservation, diagnostic *runtimecontract.ValidationFailureDiagnostic) {
			diagnostic.Operation = runtimecontract.StepKindFileWrite
		}},
		{name: "candidate", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, _ *runtimecontract.ValidationFailureDiagnostic) {
			value.CandidateTargetRefs = []string{"support.go"}
			value.ValidationScopeTargetRefs = []string{"main.go", "support.go"}
		}},
		{name: "scope", mutate: func(value *runtimecontract.RuntimeValidationFailureObservation, diagnostic *runtimecontract.ValidationFailureDiagnostic) {
			value.ValidationScopeTargetRefs = []string{"main.go", "support.go"}
			diagnostic.TargetRefs = append([]string(nil), value.CandidateTargetRefs...)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, controller, claimed, execution, receipt := claimedManagedValidationReceiptForTest(t)
			beforeReceiptCount := len(repository.aggregate.OperationReceipts)
			test.mutate(receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation, receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic)
			if _, _, err := controller.commitOperationReceipt(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID, claimed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleShell, receipt); err == nil {
				t.Fatal("mismatched observation was accepted at receipt commit")
			}
			if len(repository.aggregate.OperationReceipts) != beforeReceiptCount {
				t.Fatalf("mismatched receipt was partially persisted: %+v", repository.aggregate.OperationReceipts)
			}

			invalid := repository.aggregate
			receipt.Fence = execution.Fence
			receipt = receipt.Normalized()
			for index := range invalid.ActiveDispatch.OperationExecutions {
				if invalid.ActiveDispatch.OperationExecutions[index].OperationID != receipt.OperationID {
					continue
				}
				invalid.ActiveDispatch.OperationExecutions[index].State = runtimecontract.OperationExecutionOwnerCommitted
				invalid.ActiveDispatch.OperationExecutions[index].OwnerReceiptRef = "runtime-receipt/" + receipt.ExecutionKey
				invalid.ActiveDispatch.OperationExecutions[index].OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
			}
			invalid.OperationReceipts = append(invalid.OperationReceipts, receipt)
			if err := invalid.Validate(); err == nil {
				t.Fatal("mismatched observation was accepted at fresh load")
			}
		})
	}
}

func TestDirectTargetObservationReceiptIngressIsDescriptorAuthorizedAndEvidenceBound(t *testing.T) {
	for _, kind := range []string{
		runtimecontract.StepKindFileEdit,
		runtimecontract.StepKindGitRead,
		runtimecontract.StepKindShellCommand,
		runtimecontract.StepKindSSHCommand,
		runtimecontract.StepKindWebSearch,
	} {
		t.Run(kind, func(t *testing.T) {
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !found {
				t.Fatalf("descriptor %q is missing", kind)
			}
			observation := runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: descriptor.OwnerModuleID, SubjectRef: "owner-subject", TargetRole: "implementation",
				ObservedRevision: "owner-revision-1", TargetRefs: []string{"implementation.go"},
			}
			diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: descriptor.OwnerModuleID,
				OriginalClass: string(runtimecontract.FailureClassExecutionFailure), OriginalCode: "failed", OriginalMessage: "failed",
				Signature: "failure-1", OperationID: "operation-1", TargetRefs: observation.TargetRefs, TargetObservation: &observation,
			}
			err := validateOperationReceiptDiagnosticOwner(
				RuntimeAggregate{}, SealedDispatch{}, runtimecontract.RuntimeOperation{ID: "operation-1", Kind: kind}, diagnostic,
			)
			if err == nil || !strings.Contains(err.Error(), "does not admit direct target observations") {
				t.Fatalf("owner %q direct observation error = %v", descriptor.OwnerModuleID, err)
			}
		})
	}

	evidenceHash := strings.Repeat("a", 64)
	evidence := runtimecontract.RuntimeOperationEvidence{
		ContentHash:    evidenceHash,
		ContentAddress: &runtimecontract.RuntimeEvidenceContentAddress{Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: evidenceHash, Ref: "workorch://mcp/evidence/" + evidenceHash},
		EvidenceRefs:   []string{"workorch://mcp/capability/server-a/revision-1", "workorch://mcp/evidence/" + evidenceHash},
	}
	observation := runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
		ObservedRevision: "registry-1", TargetRefs: []string{"implementation.go"},
		CatalogDependency: &runtimecontract.RuntimeFreshnessDependency{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "registry-1"},
		DiagnosticClass:   runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence,
		EvidenceRefs:      append([]string(nil), evidence.EvidenceRefs...), ContentHash: evidence.ContentHash,
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OriginalClass: string(runtimecontract.FailureClassExecutionFailure), OriginalCode: "failed", OriginalMessage: "failed",
		Signature: "failure-1", OperationID: "operation-1", TargetRefs: observation.TargetRefs, TargetObservation: &observation,
	}
	if err := validateOperationReceiptDiagnosticOwner(RuntimeAggregate{}, SealedDispatch{}, runtimecontract.RuntimeOperation{ID: "operation-1", Kind: runtimecontract.StepKindMCPToolCall}, diagnostic); err != nil {
		t.Fatalf("valid MCP owner observation rejected: %v", err)
	}
	descriptor, _ := runtimecontract.RuntimeOperationDescriptorForKind(runtimecontract.StepKindMCPToolCall)
	if err := validateReceiptEvidenceTargetObservation(descriptor, evidence, observation); err != nil {
		t.Fatalf("valid MCP response evidence binding rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeOperationEvidence, *runtimecontract.RuntimeFailureTargetObservation)
	}{
		{name: "diagnostic class", mutate: func(_ *runtimecontract.RuntimeOperationEvidence, value *runtimecontract.RuntimeFailureTargetObservation) {
			value.DiagnosticClass = "untyped_failure"
		}},
		{name: "content hash", mutate: func(_ *runtimecontract.RuntimeOperationEvidence, value *runtimecontract.RuntimeFailureTargetObservation) {
			value.ContentHash = strings.Repeat("b", 64)
		}},
		{name: "evidence refs", mutate: func(_ *runtimecontract.RuntimeOperationEvidence, value *runtimecontract.RuntimeFailureTargetObservation) {
			value.EvidenceRefs = append(value.EvidenceRefs, "workorch://mcp/evidence/extra")
		}},
		{name: "receipt evidence", mutate: func(value *runtimecontract.RuntimeOperationEvidence, _ *runtimecontract.RuntimeFailureTargetObservation) {
			value.ContentHash = strings.Repeat("b", 64)
		}},
		{name: "missing persisted evidence", mutate: func(value *runtimecontract.RuntimeOperationEvidence, _ *runtimecontract.RuntimeFailureTargetObservation) {
			value.ContentHash = ""
			value.EvidenceRefs = nil
		}},
		{name: "coordinated non content addressed refs", mutate: func(value *runtimecontract.RuntimeOperationEvidence, observation *runtimecontract.RuntimeFailureTargetObservation) {
			refs := []string{"workorch://mcp/capability/server-a/revision-1", "workorch://mcp/opaque/response"}
			value.EvidenceRefs = append([]string(nil), refs...)
			observation.EvidenceRefs = append([]string(nil), refs...)
		}},
	} {
		t.Run("MCP/"+test.name, func(t *testing.T) {
			driftedEvidence := evidence
			driftedEvidence.EvidenceRefs = append([]string(nil), evidence.EvidenceRefs...)
			driftedObservation := observation.Clone()
			test.mutate(&driftedEvidence, &driftedObservation)
			if err := validateReceiptEvidenceTargetObservation(descriptor, driftedEvidence, driftedObservation); err == nil {
				t.Fatalf("drifted MCP response evidence binding validated: evidence=%+v observation=%+v", driftedEvidence, driftedObservation)
			}
		})
	}
}

func TestOperationReceiptFeedbackScopeRejectsDriftAtCommitAndFreshLoad(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFeedback)
	}{
		{name: "workspace", mutate: func(value *runtimecontract.RuntimeFeedback) { value.WorkspaceID = "ws-other" }},
		{name: "run", mutate: func(value *runtimecontract.RuntimeFeedback) { value.RunID = "run-other" }},
		{name: "authority", mutate: func(value *runtimecontract.RuntimeFeedback) { value.AuthoritySnapshotID = "authority-other" }},
		{name: "dispatch", mutate: func(value *runtimecontract.RuntimeFeedback) { value.DispatchID = "dispatch-other" }},
		{name: "diagnostic operation", mutate: func(value *runtimecontract.RuntimeFeedback) { value.OriginalDiagnostic.OperationID = "operation-other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, controller, claimed, execution, receipt := claimedManagedValidationReceiptForTest(t)
			beforeReceiptCount := len(repository.aggregate.OperationReceipts)
			test.mutate(receipt.Feedback)
			if _, _, err := controller.commitOperationReceipt(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID, claimed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleShell, receipt); err == nil {
				t.Fatal("drifted feedback scope was accepted at receipt commit")
			}
			if len(repository.aggregate.OperationReceipts) != beforeReceiptCount {
				t.Fatalf("drifted receipt was partially persisted: %+v", repository.aggregate.OperationReceipts)
			}

			invalid := repository.aggregate
			receipt.Fence = execution.Fence
			receipt = receipt.Normalized()
			for index := range invalid.ActiveDispatch.OperationExecutions {
				if invalid.ActiveDispatch.OperationExecutions[index].OperationID != receipt.OperationID {
					continue
				}
				invalid.ActiveDispatch.OperationExecutions[index].State = runtimecontract.OperationExecutionOwnerCommitted
				invalid.ActiveDispatch.OperationExecutions[index].OwnerReceiptRef = "runtime-receipt/" + receipt.ExecutionKey
				invalid.ActiveDispatch.OperationExecutions[index].OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
			}
			invalid.OperationReceipts = append(invalid.OperationReceipts, receipt)
			if err := invalid.Validate(); err == nil {
				t.Fatal("drifted feedback scope was accepted at fresh load")
			}
		})
	}
}

func claimedManagedValidationReceiptForTest(t *testing.T) (*durableRuntimeStateRepository, *RuntimeController, RuntimeAggregate, runtimecontract.RuntimeOperationExecutionRecord, runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	repository := &durableRuntimeStateRepository{aggregate: validationRepairSurfaceAggregateForTest(t, true)}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	claimed, err := controller.claimOperationExecution(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !found {
		t.Fatal("claimed managed-validation execution is missing")
	}
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: claimed.Facts.WorkspaceID, RunID: claimed.Facts.RunID,
		AuthoritySnapshotID: claimed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID:          claimed.ActiveDispatch.ID, OperationID: operationID,
		FailureSignature: "managed-validation-failure", TargetRole: claimed.ActiveDispatch.FocusTargetRole,
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("b", 64),
	}
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Code: "managed_validation_failed",
		Signature: observation.FailureSignature, Message: "managed validation failed",
		ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
			Class: observation.DiagnosticClass, Operation: runtimecontract.StepKindShellCommand,
			TargetRefs: append([]string(nil), observation.CandidateTargetRefs...),
		},
		ValidationFailureObservation: &observation,
	}
	return repository, controller, claimed, execution, runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: operationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
		Feedback: runtimeOperationFeedbackForTest(t, claimed, operationID, failure),
	}
}
