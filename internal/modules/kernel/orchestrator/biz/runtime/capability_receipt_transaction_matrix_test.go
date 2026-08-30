package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type capabilityReceiptTransactionFixture struct {
	name              string
	kind              string
	ownerModuleID     string
	targetRefs        []string
	parameters        []runtimecontract.OperationParameter
	constraintRefs    []string
	operationVariants []runtimecontract.RuntimeOperationAdmissibilityVariant
	admissionState    runtimecontract.ConvergenceState
	effectScope       runtimecontract.RuntimeEffectScope
	idempotencyClass  runtimecontract.RuntimeOperationIdempotencyClass
}

func TestNonFileCapabilityOwnerReceiptTransactionsUseProductionShapedAdmissionForEveryOutcome(t *testing.T) {
	t.Parallel()
	for _, fixture := range capabilityReceiptTransactionFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			for _, outcome := range []runtimecontract.RuntimeOperationOutcome{
				runtimecontract.RuntimeOperationOutcomeAccepted,
				runtimecontract.RuntimeOperationOutcomeNoOp,
				runtimecontract.RuntimeOperationOutcomeRejected,
			} {
				outcome := outcome
				t.Run(string(outcome), func(t *testing.T) {
					t.Parallel()
					testCapabilityOwnerReceiptTransaction(t, fixture, outcome)
				})
			}
		})
	}
}

func capabilityReceiptTransactionFixtures() []capabilityReceiptTransactionFixture {
	gitOperation := "init"
	shellCommand, shellStateChanging := "printf workorch", true
	sshSession, sshCredential, sshCommand := "session-a", "credential-a", "printf workorch"
	mcpServer, mcpTool := "server-a", "inspect"
	webQuery := "workorch owner receipt contract"
	return []capabilityReceiptTransactionFixture{
		{
			name: "git", kind: runtimecontract.StepKindGitWrite, ownerModuleID: runtimecontract.RuntimeModuleGit,
			targetRefs: []string{"target.txt"}, parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &gitOperation}},
			operationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyExactTarget,
					runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				},
			}},
		},
		{
			name: "shell", kind: runtimecontract.StepKindShellCommand, ownerModuleID: runtimecontract.RuntimeModuleShell,
			targetRefs: []string{"target.txt"}, parameters: []runtimecontract.OperationParameter{
				{Name: "command", String: &shellCommand}, {Name: "stateChanging", Boolean: &shellStateChanging},
			},
		},
		{
			name: "ssh", kind: runtimecontract.StepKindSSHCommand, ownerModuleID: runtimecontract.RuntimeModuleSSH,
			targetRefs: []string{"target-a"}, parameters: []runtimecontract.OperationParameter{
				{Name: "sessionRef", String: &sshSession}, {Name: "credentialRef", String: &sshCredential}, {Name: "command", String: &sshCommand},
			},
			constraintRefs: []string{"ssh-binding/target-a/session-a/credential-a", "owner-execution-policy"},
		},
		{
			name: "web", kind: runtimecontract.StepKindWebSearch, ownerModuleID: runtimecontract.RuntimeModuleWeb,
			targetRefs:     []string{"target.txt"},
			parameters:     []runtimecontract.OperationParameter{{Name: "query", String: &webQuery}},
			admissionState: runtimecontract.ConvergenceStateNeedValidation,
			effectScope:    runtimecontract.RuntimeEffectNone, idempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		},
		{
			name: "mcp", kind: runtimecontract.StepKindMCPToolCall, ownerModuleID: runtimecontract.RuntimeModuleMCP,
			targetRefs:     []string{"target.txt"},
			parameters:     []runtimecontract.OperationParameter{{Name: "serverId", String: &mcpServer}, {Name: "tool", String: &mcpTool}},
			admissionState: runtimecontract.ConvergenceStateNeedValidation,
			effectScope:    runtimecontract.RuntimeEffectNone, idempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
			constraintRefs: []string{"mcp-server/server-a", "mcp-tool/server-a/inspect", "owner-execution-policy"},
		},
	}
}

func testCapabilityOwnerReceiptTransaction(t *testing.T, fixture capabilityReceiptTransactionFixture, outcome runtimecontract.RuntimeOperationOutcome) {
	t.Helper()
	ctx := context.Background()
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(fixture.kind)
	if !found || descriptor.OwnerModuleID != fixture.ownerModuleID || !descriptor.RequiresOwnerReceipt {
		t.Fatalf("fixture descriptor = %+v found=%t", descriptor, found)
	}
	facts, definition := capabilityReceiptTransactionActivation(t, fixture, descriptor)
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, definition, nil)
	operation := runtimecontract.RuntimeOperation{
		ID: "operation-" + fixture.name, Kind: fixture.kind,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		TargetRefs:          append([]string(nil), fixture.targetRefs...),
		Parameters:          cloneCapabilityReceiptParameters(fixture.parameters),
	}
	if fixture.name == "web" {
		// Web reads are deliberately non-crediting diagnostics: only managed
		// validation and an explicitly declared MCP tool can attest a CWI
		// validation proof. Exercise Web through a production-shaped auxiliary
		// validation decision without laundering the operation into that proof.
		decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateNeedValidation,
			Focus: aggregate.Facts.Focus,
			RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:         []string{fixture.kind},
				AcceptedEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
				RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
				CoverageWorkItemIDs:   append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
				TargetRefs:            append([]string(nil), fixture.targetRefs...),
			},
		}, aggregate.Decision.AuthoritySnapshot.Attempt)
		if err != nil {
			t.Fatalf("seal auxiliary Web decision: %v", err)
		}
		aggregate.Decision = decision
		repository.aggregate = &aggregate
		operation.CoverageWorkItemIDs = nil
	}
	candidate := capabilityReceiptCandidate(t, fixture, descriptor, operation)
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatalf("production-shaped candidate admission: %v", err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	if fixture.name == "web" {
		operationID = capabilityReceiptOperationIDByKind(t, sealed.ActiveDispatch.Candidate, fixture.kind)
	}
	claimed, err := controller.claimOperationExecution(ctx, facts.WorkspaceID, facts.RunID, operationID)
	if err != nil {
		t.Fatalf("claim operation execution: %v", err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed || execution.Fence == 0 {
		t.Fatalf("claimed execution = %+v found=%t", execution, found)
	}
	if _, found, err := controller.QueryOperationReceipt(
		ctx, facts.WorkspaceID, facts.RunID, claimed.ActiveDispatch.ID, operationID,
		execution.ExecutionKey, execution.Fence, fixture.ownerModuleID, nil,
	); err != nil || found {
		t.Fatalf("pre-commit receipt query found=%t err=%v", found, err)
	}
	receipt := capabilityReceiptForOutcome(t, claimed, operationID, execution, descriptor, outcome)
	committed, persisted, err := controller.commitOperationReceipt(
		ctx, facts.WorkspaceID, facts.RunID, claimed.ActiveDispatch.ID,
		execution.Fence, fixture.ownerModuleID, receipt,
	)
	if err != nil {
		t.Fatalf("commit %s receipt: %v", outcome, err)
	}
	if committed.StateRevision != claimed.StateRevision+1 || persisted.Outcome != outcome || persisted.Fence != execution.Fence {
		t.Fatalf("committed receipt transaction = aggregate revision %d, receipt %+v", committed.StateRevision, persisted)
	}
	queried, found, err := controller.QueryOperationReceipt(
		ctx, facts.WorkspaceID, facts.RunID, claimed.ActiveDispatch.ID, operationID,
		execution.ExecutionKey, execution.Fence, fixture.ownerModuleID, nil,
	)
	if err != nil || !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, queried) {
		t.Fatalf("post-commit receipt query found=%t receipt=%+v err=%v", found, queried, err)
	}
}

func capabilityReceiptCandidate(
	t *testing.T,
	fixture capabilityReceiptTransactionFixture,
	descriptor runtimecontract.RuntimeOperationDescriptor,
	operation runtimecontract.RuntimeOperation,
) runtimecontract.RuntimeCandidate {
	t.Helper()
	candidate := runtimecontract.RuntimeCandidate{}
	switch capabilityReceiptAdmissionState(fixture, descriptor) {
	case runtimecontract.ConvergenceStateNeedStateChange:
		candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}
	case runtimecontract.ConvergenceStateNeedValidation:
		candidate.Validation = &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: operation}}}
	default:
		t.Fatalf("fixture kind %q has unsupported admission state %q", fixture.kind, descriptor.AllowedStates[0])
	}
	return candidate
}

func capabilityReceiptTransactionActivation(
	t *testing.T,
	fixture capabilityReceiptTransactionFixture,
	descriptor runtimecontract.RuntimeOperationDescriptor,
) (runtimecontract.RuntimeFacts, runtimecontract.RuntimeDefinitionContext) {
	t.Helper()
	facts := controllerFacts()
	evidenceKind := runtimecontract.EvidenceKindValidation
	if capabilityReceiptAdmissionState(fixture, descriptor) == runtimecontract.ConvergenceStateNeedStateChange {
		evidenceKind = runtimecontract.EvidenceKindStateChange
		facts.Capabilities.StateChangeKinds = []string{fixture.kind}
		facts.Capabilities.ValidationKinds = nil
	} else {
		facts.Capabilities.StateChangeKinds = nil
		facts.Capabilities.ValidationKinds = []string{fixture.kind}
	}
	facts.Focus.RequiredEvidenceKinds = []string{evidenceKind}
	facts.Focus.TargetRefs = append([]string(nil), fixture.targetRefs...)
	facts.DefinitionFocuses = nil
	prerequisiteID := "operation-admissibility-" + fixture.name
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: fixture.ownerModuleID, RequiredCapabilities: []string{fixture.kind},
		Reason: "receipt transaction owner catalog",
	}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		prerequisiteID, fixture.ownerModuleID, []string{fixture.kind}, fixture.targetRefs, nil,
		fixture.constraintRefs, "owner-catalog-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "operation-policy/" + fixture.name, OwnerModuleID: fixture.ownerModuleID,
		OperationKind: fixture.kind, SubjectRef: "owner-subject-" + fixture.name,
		EffectScope: capabilityReceiptEffectScope(fixture, descriptor), IdempotencyClass: capabilityReceiptIdempotencyClass(fixture, descriptor),
	}}
	catalog.OperationVariants = append([]runtimecontract.RuntimeOperationAdmissibilityVariant(nil), fixture.operationVariants...)
	if capabilityReceiptAdmissionState(fixture, descriptor) == runtimecontract.ConvergenceStateNeedValidation {
		applicability := runtimecontract.RuntimeValidationProofDirect
		if fixture.name == "web" {
			applicability = runtimecontract.RuntimeValidationProofAuxiliary
		}
		surface := runtimecontract.RuntimeValidationSurface{
			ID: "validation-surface/" + fixture.name, OwnerModuleID: fixture.ownerModuleID,
			OperationKind: fixture.kind, Readiness: runtimecontract.RuntimeValidationSurfaceReady,
			InputSchemaHash: "validation-input-schema/" + fixture.name + "/v1",
			InputBindingRef: "validation-input-binding/" + fixture.name + "/v1",
			CheckRefs:       []string{"check/" + fixture.name}, ProofApplicability: applicability,
			EffectScope: capabilityReceiptEffectScope(fixture, descriptor), IdempotencyClass: capabilityReceiptIdempotencyClass(fixture, descriptor),
		}
		catalog.ValidationSurfaces = []runtimecontract.RuntimeValidationSurface{surface}
		if applicability == runtimecontract.RuntimeValidationProofDirect {
			catalog.ValidationRecipeRefs = []string{surface.ID}
		}
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("owner catalog: %v", err)
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: fixture.ownerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: fixture.ownerModuleID,
			SubjectRef: "owner-subject-" + fixture.name, Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatalf("owner catalog observation: %v", err)
	}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	definition := controllerDefinition()
	definition.CapabilityContract = []string{fixture.kind}
	if fixture.name == "web" {
		facts = runtimeFactsWithManagedValidationRecipe(facts)
		facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession, fixture.kind}
		definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindManagedValidationSession, fixture.kind}
	}
	return facts, definition
}

func capabilityReceiptOperationIDByKind(t *testing.T, candidate runtimecontract.RuntimeCandidate, kind string) string {
	t.Helper()
	for _, operation := range candidateRuntimeOperations(candidate) {
		if operation.Kind == kind {
			return operation.ID
		}
	}
	t.Fatalf("candidate has no operation kind %q", kind)
	return ""
}

func capabilityReceiptForOutcome(
	t *testing.T,
	aggregate RuntimeAggregate,
	operationID string,
	execution runtimecontract.RuntimeOperationExecutionRecord,
	descriptor runtimecontract.RuntimeOperationDescriptor,
	outcome runtimecontract.RuntimeOperationOutcome,
) runtimecontract.RuntimeOperationReceipt {
	t.Helper()
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: operationID, Outcome: outcome,
		TargetRefs: append([]string(nil), candidateOperationTargetRefs(t, aggregate.ActiveDispatch.Candidate, operationID)...),
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: fmt.Sprintf("%s owner receipt", outcome)},
	}
	switch outcome {
	case runtimecontract.RuntimeOperationOutcomeAccepted:
		receipt.ObservedEffectScope = execution.ExpectedEffectScope
		receipt.Changed = execution.ExpectedEffectScope != runtimecontract.RuntimeEffectNone
	case runtimecontract.RuntimeOperationOutcomeNoOp:
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectNone
	case runtimecontract.RuntimeOperationOutcomeRejected:
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		failureTargets := append([]string(nil), receipt.TargetRefs...)
		if descriptor.FailureTargetProtocol == runtimecontract.RuntimeFailureTargetProtocolReceiptEvidence {
			// MCP may discover repair targets only through a content-addressed
			// target observation. This matrix exercises a target-free rejection;
			// the positive discovered-target protocol has its own integration test.
			failureTargets = nil
		}
		receipt.Feedback = runtimeOperationFeedbackForTest(t, aggregate, operationID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_rejected",
			Signature: "owner-rejected-" + operationID, Message: "capability owner rejected the sealed operation",
			TargetRefs: failureTargets,
		})
	default:
		t.Fatalf("unsupported operation outcome %q", outcome)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("receipt %s: %v", outcome, err)
	}
	return receipt
}

func capabilityReceiptAdmissionState(fixture capabilityReceiptTransactionFixture, descriptor runtimecontract.RuntimeOperationDescriptor) runtimecontract.ConvergenceState {
	if fixture.admissionState != "" {
		return fixture.admissionState
	}
	return descriptor.AllowedStates[0]
}

func capabilityReceiptEffectScope(fixture capabilityReceiptTransactionFixture, descriptor runtimecontract.RuntimeOperationDescriptor) runtimecontract.RuntimeEffectScope {
	if fixture.effectScope != "" {
		return fixture.effectScope
	}
	return descriptor.DefaultEffectScope
}

func capabilityReceiptIdempotencyClass(fixture capabilityReceiptTransactionFixture, descriptor runtimecontract.RuntimeOperationDescriptor) runtimecontract.RuntimeOperationIdempotencyClass {
	if fixture.idempotencyClass != "" {
		return fixture.idempotencyClass
	}
	if descriptor.IdempotencyClass == runtimecontract.RuntimeOperationIdempotencyUnknown {
		return runtimecontract.RuntimeOperationIdempotencyAtMostOnce
	}
	return descriptor.IdempotencyClass
}

func candidateOperationTargetRefs(t *testing.T, candidate runtimecontract.RuntimeCandidate, operationID string) []string {
	t.Helper()
	operation, found := candidateOperationByID(candidate, operationID)
	if !found {
		t.Fatalf("candidate operation %q is missing", operationID)
	}
	return operation.TargetRefs
}

func cloneCapabilityReceiptParameters(values []runtimecontract.OperationParameter) []runtimecontract.OperationParameter {
	ret := make([]runtimecontract.OperationParameter, len(values))
	for index := range values {
		ret[index] = values[index]
		if values[index].String != nil {
			value := *values[index].String
			ret[index].String = &value
		}
		if values[index].Integer != nil {
			value := *values[index].Integer
			ret[index].Integer = &value
		}
		if values[index].Boolean != nil {
			value := *values[index].Boolean
			ret[index].Boolean = &value
		}
		ret[index].StringList = append([]string(nil), values[index].StringList...)
	}
	return ret
}
