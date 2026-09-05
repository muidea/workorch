package events

import (
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceExecutionIntentRejectsDuplicateBootstrapAuthorityBeforeNormalization(t *testing.T) {
	policy := runtimecontract.RuntimeEvidencePolicy{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileWrite}.Normalized()
	bootstrap := runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{policy, policy}}}
	cmd := WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RequestID: "request-1", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap,
	}
	if err := cmd.Validate(); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("restart intent accepted duplicate bootstrap authority: %v", err)
	}
	if got := cmd.Normalized(); len(got.Bootstrap.Activation.EvidencePolicies) != 2 {
		t.Fatalf("command normalization laundered duplicate bootstrap authority: %+v", got.Bootstrap.Activation.EvidencePolicies)
	}
}

func TestWorkspaceExecutionIntentResultUsesTypedAdmissionWithoutInventedRun(t *testing.T) {
	result := WorkspaceExecutionIntentResult{
		WorkspaceID: "ws-1", RequestID: "request-1", Kind: runtimecontract.RuntimeExecutionIntentResume,
		Outcome: runtimecontract.RuntimeExecutionIntentRestartRequired,
		Reason:  runtimecontract.RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Outcome = runtimecontract.RuntimeExecutionIntentReconciliationRequested
	if err := result.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("mismatched admission err = %v", err)
	}
}

func TestWorkspaceRuntimeTerminalDeliveryFactValidate(t *testing.T) {
	cmd := RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, SourceOwner: "kernel/orchestrator", CausationID: "terminal-delivery/td-1"},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	if err := (RuntimeTerminalDeliveryFact{}).Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty terminal err = %v", err)
	}
}
