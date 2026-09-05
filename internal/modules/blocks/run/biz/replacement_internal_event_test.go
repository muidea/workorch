package run

import (
	"errors"
	"strings"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReplacementInternalEventRequiresExactlyOneFact(t *testing.T) {
	requested := runmodel.ReplacementIntent{}
	_, err := normalizeReplacementInternalEvent(replacementInternalEvent{
		Kind: replacementInternalEventRequested, WorkspaceID: "workspace-1", RunID: "run-1", OccurredAt: "2026-08-22T00:00:00Z",
		Requested: &requested, RecoveryClaimed: &replacementRecoveryClaimedFact{RequestID: "request-1", Generation: 1, ClaimedAt: "2026-08-22T00:00:00Z"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("multi-payload replacement event err = %v, want invalid argument", err)
	}
}

func TestReplacementRequestedRejectsRawBootstrapBeforeClone(t *testing.T) {
	policy := runtimecontract.RuntimeEvidencePolicy{
		ID: "policy-1", AppliesTo: runtimecontract.StepKindFileWrite,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "dod-1", Scope: "dodItem", TargetRefs: []string{" target.go "},
		}},
	}
	bootstrap := runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{policy}}}
	requested := runmodel.ReplacementIntent{WorkspaceID: "workspace-1", Bootstrap: &bootstrap}
	_, err := normalizeReplacementInternalEvent(replacementInternalEvent{
		Kind: replacementInternalEventRequested, WorkspaceID: "workspace-1", RunID: "run-1", OccurredAt: "2026-08-22T00:00:00Z", Requested: &requested,
	})
	if err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("replacement event accepted noncanonical bootstrap before clone: %v", err)
	}
	if got := requested.Bootstrap.Activation.EvidencePolicies[0].CoverageRequirements[0].TargetRefs[0]; got != " target.go " {
		t.Fatalf("rejected replacement bootstrap was normalized in place: %q", got)
	}
}

func TestReplacementInternalReducerRejectsTerminationFailureBeforeClarificationResolution(t *testing.T) {
	bootstrap := restartTestBootstrap(t, "workspace-1")
	predecessor := &runmodel.Run{
		ID: "run-1", WorkspaceID: "workspace-1", DefinitionRevision: "definition-old", StartupInputHash: "startup-old",
		State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-1",
	}
	intent := replacementIntentFromCommand(runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: predecessor.WorkspaceID, RequestID: "restart-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	})
	intent = replacementIntentForPredecessor(intent, predecessor)
	requested, err := reduceReplacementRequested(predecessor, intent, "2026-08-22T00:00:00Z")
	if err != nil || requested.Next == nil {
		t.Fatalf("persist replacement request: transition=%+v err=%v", requested, err)
	}
	predecessor = requested.Next
	intent = *predecessor.ReplacementIntent
	claimed, err := reduceReplacementRecoveryClaimed(predecessor, replacementRecoveryClaimedFact{
		RequestID: intent.RequestID, Generation: intent.Recovery.Generation, ClaimedAt: "2026-08-22T00:00:00Z",
	})
	if err != nil || claimed.Next == nil || !claimed.Drive {
		t.Fatalf("claim clarification recovery: transition=%+v err=%v", claimed, err)
	}
	predecessor = claimed.Next
	intent = *predecessor.ReplacementIntent

	_, err = reduceReplacementInternalEvent(predecessor, replacementInternalEvent{
		Kind: replacementInternalEventRecoveryFailed, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		RecoveryFailed: &replacementRecoveryFailedFact{
			RequestID: intent.RequestID, Generation: intent.Recovery.Generation, Stage: runmodel.ReplacementRecoveryAdministrativeTermination,
			FailedAt: "2026-08-22T00:00:01Z", RetryDueAt: "2026-08-22T00:00:02Z", Error: "must not deliver before clarification",
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("pre-clarification termination failure err = %v, want conflict", err)
	}

	_, err = reduceReplacementInternalEvent(predecessor, replacementInternalEvent{
		Kind: replacementInternalEventClarificationResolutionObserved, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		ClarificationResolution: &replacementClarificationResolutionFact{
			RequestID: intent.RequestID, Generation: intent.Recovery.Generation, ResolutionID: "resolution-1", ResolutionBindingHash: "binding-1",
			ConflictKind: decisionevents.RuntimeDefinitionClarificationNoConflict, NextDueAt: "2026-08-22T00:00:02Z",
		},
	})
	if err != nil {
		t.Fatalf("valid clarification resolution transition: %v", err)
	}
}
