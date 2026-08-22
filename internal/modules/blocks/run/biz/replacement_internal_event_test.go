package run

import (
	"errors"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReplacementInternalEventRequiresExactlyOneFact(t *testing.T) {
	requested := runmodel.ReplacementIntent{}
	_, err := normalizeReplacementInternalEvent(replacementInternalEvent{
		Kind: replacementInternalEventRequested, WorkspaceID: "workspace-1", RunID: "run-1",
		Requested: &requested, TerminationAttempt: &replacementTerminationAttemptFact{AttemptedAt: "2026-08-22T00:00:00Z"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("multi-payload replacement event err = %v, want invalid argument", err)
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
	intent.State = runmodel.ReplacementIntentPendingClarificationResolution
	predecessor.ReplacementIntent = &intent
	if !intent.ValidFor(predecessor) {
		t.Fatalf("test replacement intent is invalid: %+v", intent)
	}

	_, err := reduceReplacementInternalEvent(predecessor, replacementInternalEvent{
		Kind: replacementInternalEventTerminationFailed, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		TerminationFailed: &replacementTerminationFailedFact{RequestID: intent.RequestID, Error: "must not deliver before clarification"},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("pre-clarification termination failure err = %v, want conflict", err)
	}

	_, err = reduceReplacementInternalEvent(predecessor, replacementInternalEvent{
		Kind: replacementInternalEventClarificationResolutionObserved, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		ClarificationResolution: &replacementClarificationResolutionFact{
			RequestID: intent.RequestID, ResolutionID: "resolution-1", ResolutionBindingHash: "binding-1",
			ConflictKind: decisionevents.RuntimeDefinitionClarificationNoConflict,
		},
	})
	if err != nil {
		t.Fatalf("valid clarification resolution transition: %v", err)
	}
}
