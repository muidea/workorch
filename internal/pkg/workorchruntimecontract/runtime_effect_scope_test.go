package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestRuntimeEffectScopeNormalizeAndClassify(t *testing.T) {
	if got := NormalizeRuntimeEffectScope(" workspace_durable "); got != RuntimeEffectWorkspaceDurable {
		t.Fatalf("normalize durable = %q", got)
	}
	if got := NormalizeRuntimeEffectScope("nope"); got != RuntimeEffectUnknown {
		t.Fatalf("normalize unknown = %q", got)
	}
	if !RuntimeEffectIsDurable(RuntimeEffectWorkspaceDurable) || !RuntimeEffectIsDurable(RuntimeEffectRemoteDurable) {
		t.Fatal("durable scopes must be durable")
	}
	if RuntimeEffectIsDurable(RuntimeEffectEphemeralSession) || RuntimeEffectIsDurable(RuntimeEffectNone) {
		t.Fatal("ephemeral/none must not be durable")
	}
	if !RuntimeEffectIsNonDurable(RuntimeEffectNone) || !RuntimeEffectIsNonDurable(RuntimeEffectEphemeralSession) {
		t.Fatal("none/ephemeral must be non-durable")
	}
}

func TestValidateEffectScopesCompatibleWithAction(t *testing.T) {
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedStateChange, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}); err != nil {
		t.Fatalf("state-change durable should pass: %v", err)
	}
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedStateChange, []RuntimeEffectScope{RuntimeEffectNone}); err == nil {
		t.Fatal("state-change with none must fail")
	}
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedStateChange, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectNone}); err == nil {
		t.Fatal("state-change policy must reject a mixed durable/non-durable scope set")
	}
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedValidation, []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession}); err != nil {
		t.Fatalf("validation non-durable should pass: %v", err)
	}
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedValidation, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}); err == nil {
		t.Fatal("validation with durable must fail")
	}
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedGovernance, []RuntimeEffectScope{RuntimeEffectGovernanceOnly}); err != nil {
		t.Fatalf("governance-only should pass: %v", err)
	}
	if err := ValidateEffectScopesCompatibleWithState(ConvergenceStateNeedAssessment, []RuntimeEffectScope{RuntimeEffectNone}); err == nil {
		t.Fatal("assessment cannot carry effect scopes")
	}
}

func TestRuntimeConvergenceEffectPolicyIsClosedAcrossCandidateAndReceipt(t *testing.T) {
	stateChange, ok := RuntimeConvergenceEffectPolicyFor(ConvergenceStateNeedStateChange)
	if !ok || stateChange.Validate() != nil {
		t.Fatalf("state-change policy = %+v, ok=%v", stateChange, ok)
	}
	if !stateChange.AllowsOperationEffect(RuntimeEffectWorkspaceDurable) ||
		stateChange.AllowsOperationEffect(RuntimeEffectNone) ||
		stateChange.AllowsAcceptedReceiptEffect(RuntimeEffectEphemeralSession) {
		t.Fatalf("state-change policy accepts an incompatible effect: %+v", stateChange)
	}
	validation, ok := RuntimeConvergenceEffectPolicyFor(ConvergenceStateNeedValidation)
	if !ok || validation.Validate() != nil || !validation.AllowsOperationEffect(RuntimeEffectNone) ||
		validation.AllowsAcceptedReceiptEffect(RuntimeEffectRemoteDurable) {
		t.Fatalf("validation policy = %+v, ok=%v", validation, ok)
	}
	assessment, ok := RuntimeConvergenceEffectPolicyFor(ConvergenceStateNeedAssessment)
	if !ok || assessment.ExecutableOperationsAllowed || assessment.AllowsOperationEffect(RuntimeEffectNone) {
		t.Fatalf("assessment policy permits executable operations: %+v", assessment)
	}
}

func TestRequiredOutputRequiresExplicitEffectScopes(t *testing.T) {
	base := RequiredOutputContract{
		AcceptedKinds:         []string{StepKindFileEdit},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
	}
	without := StableRequiredOutputHash(base)
	if err := ValidateRequiredOutputForState(ConvergenceStateNeedStateChange, base); err == nil || !strings.Contains(err.Error(), "requires explicit required and accepted effect scopes") {
		t.Fatalf("missing effect scopes must fail fast, got %v", err)
	}
	withScopes := base
	withScopes.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}
	withScopes.AcceptedEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}
	if got := StableRequiredOutputHash(withScopes); got == without {
		t.Fatal("explicit effect scopes must participate in stable hash")
	}
	if err := ValidateRequiredOutputForState(ConvergenceStateNeedStateChange, withScopes); err != nil {
		t.Fatalf("explicit state-change effect contract rejected: %v", err)
	}
	// Invalid durable scope on validation fails Validate.
	bad := RequiredOutputContract{
		AcceptedKinds:        []string{StepKindFileRead},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
	}
	if err := ValidateRequiredOutputForState(ConvergenceStateNeedValidation, bad); err == nil || !strings.Contains(err.Error(), "accepted effect scopes") {
		t.Fatalf("validation with durable accepted scope must fail, got %v", err)
	}
}

func TestRuntimeOperationDescriptorEffectScopeInventory(t *testing.T) {
	descriptors := RuntimeOperationDescriptors()
	if len(descriptors) == 0 {
		t.Fatal("empty descriptor inventory")
	}
	for _, descriptor := range descriptors {
		if descriptor.DefaultEffectScope == "" {
			t.Fatalf("descriptor %q missing DefaultEffectScope", descriptor.Kind)
		}
		if !ValidRuntimeEffectScope(descriptor.DefaultEffectScope) {
			t.Fatalf("descriptor %q has invalid DefaultEffectScope %q", descriptor.Kind, descriptor.DefaultEffectScope)
		}
		if descriptor.IdempotencyClass == "" {
			t.Fatalf("descriptor %q missing IdempotencyClass", descriptor.Kind)
		}
	}
	// shell/command remains side-effecting at kind level via unknown-or-durable scope.
	shell, ok := RuntimeOperationDescriptorForKind(StepKindShellCommand)
	if !ok || !RuntimeEffectIsDurable(shell.DefaultEffectScope) {
		t.Fatalf("shell default effect = %+v", shell)
	}
	// managed validation is non-durable but owns an ephemeral session.
	managed, ok := RuntimeOperationDescriptorForKind(RuntimeBlockKindManagedValidationSession)
	if !ok || managed.DefaultEffectScope != RuntimeEffectEphemeralSession || managed.IdempotencyClass != RuntimeOperationIdempotencyAtMostOnce {
		t.Fatalf("managed validation effect = %+v", managed)
	}
	// MCP tool_call defaults to unknown until capability risk narrows it.
	mcp, ok := RuntimeOperationDescriptorForKind(StepKindMCPToolCall)
	if !ok || mcp.DefaultEffectScope != RuntimeEffectUnknown {
		t.Fatalf("mcp tool_call default effect = %+v", mcp)
	}
	// Side-effect helper still treats MCP unknown as mutating (deny validation entry).
	if !OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: StepKindMCPToolCall}) {
		t.Fatal("unmatched MCP tool_call must remain side-effecting")
	}
	if OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: RuntimeBlockKindManagedValidationSession}) {
		t.Fatal("managed validation must remain non-mutating")
	}
}

func TestRuntimeOperationExecutionRecordAndReceiptRoundTrip(t *testing.T) {
	key := StableRuntimeOperationExecutionKey("ws-1", "run-1", "dispatch-1", "op-1")
	if key != "exec/ws-1/run-1/dispatch-1/op-1" {
		t.Fatalf("execution key = %q", key)
	}
	content := "sealed content"
	operation := RuntimeOperation{ID: "op-1", Kind: StepKindFileWrite, TargetRefs: []string{"target.txt"}, Parameters: []OperationParameter{{Name: "content", String: &content}}}
	sealed := RuntimeOperationExecutionRecord{
		OperationID: "op-1", OperationHash: StableRuntimeOperationHash(operation), ExecutionKey: key,
		ExpectedEffectScope:     RuntimeEffectWorkspaceDurable,
		IdempotencyClass:        RuntimeOperationIdempotencyAtMostOnce,
		AdmissibilitySnapshotID: "admissibility-1",
		AcquisitionBinding:      "capability-state/ws-1/run-1/owner/0",
		State:                   OperationExecutionSealed,
	}
	sealed.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(sealed)
	if err := sealed.Validate(); err != nil {
		t.Fatalf("sealed validate: %v", err)
	}
	for name, scope := range map[string]RuntimeEffectScope{
		"missing": "",
		"invalid": "future_effect",
		"unknown": RuntimeEffectUnknown,
	} {
		t.Run("sealed effect scope "+name, func(t *testing.T) {
			invalid := sealed
			invalid.ExpectedEffectScope = scope
			invalid.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("execution record accepted unresolved effect scope %q", scope)
			}
		})
	}
	missingClassJSON, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	var missingClassObject map[string]any
	if err := json.Unmarshal(missingClassJSON, &missingClassObject); err != nil {
		t.Fatal(err)
	}
	delete(missingClassObject, "IdempotencyClass")
	missingClassJSON, err = json.Marshal(missingClassObject)
	if err != nil {
		t.Fatal(err)
	}
	var missingClass RuntimeOperationExecutionRecord
	if err := json.Unmarshal(missingClassJSON, &missingClass); err != nil {
		t.Fatal(err)
	}
	missingClass.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(missingClass)
	if err := missingClass.Validate(); err == nil {
		t.Fatal("execution record accepted JSON with missing idempotency class")
	}
	invalidClass := sealed
	invalidClass.IdempotencyClass = "invalid"
	invalidClass.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(invalidClass)
	if err := invalidClass.Validate(); err == nil {
		t.Fatal("execution record accepted an invalid idempotency class")
	}
	unknownClass := sealed
	unknownClass.IdempotencyClass = RuntimeOperationIdempotencyUnknown
	unknownClass.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(unknownClass)
	if err := unknownClass.Validate(); err == nil {
		t.Fatal("execution record accepted an unresolved idempotency class")
	}
	missingAuthorityHash := sealed
	missingAuthorityHash.AuthorityHash = ""
	if err := missingAuthorityHash.Validate(); err == nil {
		t.Fatal("execution record accepted a missing authority hash")
	}
	driftedAuthority := sealed
	driftedAuthority.IdempotencyClass = RuntimeOperationIdempotencySafeToReplay
	if err := driftedAuthority.Validate(); err == nil {
		t.Fatal("execution record accepted idempotency drift outside its authority hash")
	}
	unbound := sealed
	unbound.AdmissibilitySnapshotID = ""
	if err := unbound.Validate(); err == nil {
		t.Fatal("execution record accepted acquisition binding without owner catalog snapshot")
	}
	proofed := sealed
	proofed.ReplayProof = &RuntimeOperationReplayProof{
		OwnerModuleID: RuntimeModuleFile, OperationID: sealed.OperationID, TargetRefs: []string{"target.txt"},
		BeforeHash: "before", AfterHash: "after", AdmissibilitySnapshotID: sealed.AdmissibilitySnapshotID,
		AcquisitionBinding: sealed.AcquisitionBinding,
	}
	proofed.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(proofed)
	if err := proofed.Validate(); err != nil {
		t.Fatalf("proofed execution validate: %v", err)
	}
	proofClone := proofed.Clone()
	proofClone.ReplayProof.TargetRefs[0] = "other.txt"
	if proofed.ReplayProof.TargetRefs[0] != "target.txt" {
		t.Fatal("operation execution clone aliases replay proof targets")
	}
	proofed.ReplayProof.AdmissibilitySnapshotID = "other-admissibility"
	if err := proofed.Validate(); err == nil {
		t.Fatal("execution record accepted replay proof with substituted owner authority")
	}
	// claimed without fence fails
	claimed := sealed
	claimed.State = OperationExecutionClaimed
	if err := claimed.Validate(); err == nil {
		t.Fatal("claimed without fence must fail")
	}
	claimed.Fence = 1
	claimed.ClaimedAt = "unknown-claim-marker"
	if err := claimed.Validate(); err == nil {
		t.Fatal("claimed execution accepted an unknown claim marker")
	}
	claimed.ClaimedAt = RuntimeOperationClaimMarkerInitial
	if err := claimed.Validate(); err != nil {
		t.Fatalf("claimed validate: %v", err)
	}
	// owner committed needs receipt
	committed := claimed
	committed.State = OperationExecutionOwnerCommitted
	if err := committed.Validate(); err == nil {
		t.Fatal("owner_committed without receipt must fail")
	}
	committed.OwnerReceiptRef = "receipt-1"
	committed.OwnerReceiptHash = "hash-1"
	if err := committed.Validate(); err != nil {
		t.Fatalf("owner_committed validate: %v", err)
	}

	// duplicate execution keys fail
	if err := ValidateRuntimeOperationExecutionRecords([]RuntimeOperationExecutionRecord{sealed, sealed}); err == nil {
		t.Fatal("duplicate execution keys must fail")
	}

	accepted := RuntimeOperationReceipt{
		Fence:        1,
		ExecutionKey: key, OperationID: "op-1",
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectWorkspaceDurable,
		Changed: true, CompletedAt: "2026-07-17T00:00:01Z",
		Evidence: RuntimeOperationEvidence{Summary: "edited file", ContentHash: "abc"},
	}
	if err := accepted.Validate(); err != nil {
		t.Fatalf("accepted receipt: %v", err)
	}
	acceptedWithFailure := accepted
	acceptedWithFailure.Feedback = testRejectedOperationFeedback(t, "op-1", FailureFact{Class: FailureClassExecutionRejected, Code: "must_not_coexist", Signature: "must-not-coexist", Message: "accepted receipt cannot carry a failure"})
	if err := acceptedWithFailure.Validate(); err == nil {
		t.Fatal("accepted receipt with a failure creates a second operation result authority")
	}
	kind, err := EvidenceKindFromReceipt(ConvergenceStateNeedStateChange, RuntimeEffectWorkspaceDurable, accepted)
	if err != nil || kind != EvidenceKindStateChange {
		t.Fatalf("evidence from accepted durable = %q err=%v", kind, err)
	}

	// no-op cannot close durable state-change
	noop := RuntimeOperationReceipt{
		Fence:        1,
		ExecutionKey: key, OperationID: "op-1",
		Outcome: RuntimeOperationOutcomeNoOp, ObservedEffectScope: RuntimeEffectNone,
	}
	if err := noop.Validate(); err != nil {
		t.Fatalf("noop validate: %v", err)
	}
	if _, err := EvidenceKindFromReceipt(ConvergenceStateNeedStateChange, RuntimeEffectWorkspaceDurable, noop); err == nil {
		t.Fatal("noop must not close durable state-change")
	}
	// no-op can be validation observation
	if kind, err := EvidenceKindFromReceipt(ConvergenceStateNeedValidation, RuntimeEffectNone, noop); err != nil || kind != EvidenceKindValidation {
		t.Fatalf("noop validation observation = %q err=%v", kind, err)
	}
	encoded, err := json.Marshal(noop)
	if err != nil {
		t.Fatalf("marshal no-op receipt: %v", err)
	}
	if strings.Contains(string(encoded), "noOp") {
		t.Fatalf("receipt must use outcome as the only no-op authority: %s", encoded)
	}

	// effect mismatch rejected
	mismatch := accepted
	mismatch.ObservedEffectScope = RuntimeEffectNone
	if _, err := EvidenceKindFromReceipt(ConvergenceStateNeedStateChange, RuntimeEffectWorkspaceDurable, mismatch); err == nil {
		t.Fatal("non-durable observed cannot satisfy durable expected")
	}

	// governance required
	gov := RuntimeOperationReceipt{
		Fence:        1,
		ExecutionKey: key, OperationID: "op-1",
		Outcome: RuntimeOperationOutcomeGovernanceRequired, ObservedEffectScope: RuntimeEffectGovernanceOnly,
		Governance: &RuntimeGovernanceRequirement{
			Kind: GovernanceApproval, SubjectRef: "web/fetch", Reason: "external network requires approval",
			RequestedAction: "web.fetch",
			AuthorizationContext: workorcheventpayload.New(
				workorcheventpayload.StringField("subjectRef", "web/fetch"),
			),
		},
	}
	if err := gov.Validate(); err != nil {
		t.Fatalf("governance receipt: %v", err)
	}
	if _, err := EvidenceKindFromReceipt(ConvergenceStateNeedValidation, RuntimeEffectNone, gov); err == nil {
		t.Fatal("governance receipt cannot commit ordinary evidence")
	}

	// rejected needs failure
	rejected := RuntimeOperationReceipt{
		Fence:        1,
		ExecutionKey: key, OperationID: "op-1",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectNone,
		Feedback: testRejectedOperationFeedback(t, "op-1", FailureFact{Class: FailureClassExecutionRejected, Code: "exit_1", Signature: "exit-1", Message: "command failed"}),
	}
	if err := rejected.Validate(); err != nil {
		t.Fatalf("rejected receipt: %v", err)
	}
	if kind, err := EvidenceKindFromReceipt(ConvergenceStateNeedValidation, RuntimeEffectNone, rejected); err != nil || kind != EvidenceKindValidation {
		t.Fatalf("rejected validation evidence = %q err=%v", kind, err)
	}
}

func TestRuntimeRepairDirectiveRoundTrip(t *testing.T) {
	directive := RuntimeRepairDirective{
		ID: "repair-1", FocusID: "focus-1", Trigger: RuntimeRepairTriggerRejectedValidation,
		RequiredState:         ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		RepairAttempt:         1,
		CurrentFailure: &FailureFact{
			Class: FailureClassExecutionRejected, Code: "test_failed", Message: "validation failed",
		},
	}
	if err := directive.Validate(); err != nil {
		t.Fatalf("directive validate: %v", err)
	}
	clone := directive.Clone()
	if !SameRuntimeRepairDirective(&directive, &clone) {
		t.Fatal("clone must equal original")
	}
	// action/effect mismatch fails
	bad := directive
	bad.RequiredState = ConvergenceStateNeedValidation
	if err := bad.Validate(); err == nil {
		t.Fatal("validation action with durable required effect must fail")
	}
	// invalid action fails
	bad = directive
	bad.RequiredState = ConvergenceStateComplete
	if err := bad.Validate(); err == nil {
		t.Fatal("complete is not a repairable action")
	}
}

func TestRuntimeTerminationRecordAdministrativeHandoffSchema(t *testing.T) {
	term := RuntimeTerminationRecord{
		Kind: RuntimeTerminationCancelled, RequestedBy: "blocks/run", Reason: "operator cancel", Revision: 3,
		RequestedAt: "2026-07-17T01:00:00Z",
	}
	if err := term.Validate(); err != nil {
		t.Fatalf("termination validate: %v", err)
	}
	if clone := term.Clone(); clone != term {
		t.Fatalf("administrative handoff clone = %+v, want %+v", clone, term)
	}
	if err := (RuntimeTerminationRecord{Kind: RuntimeTerminationCancelled, Reason: "x"}).Validate(); err == nil {
		t.Fatal("termination without revision must fail")
	}
}
