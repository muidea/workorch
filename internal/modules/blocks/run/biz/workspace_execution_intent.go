package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceExecutionIntentRunSource = "blocks/workspace"

func (m *Biz) handleWorkspaceExecutionIntent(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeWorkspaceExecutionIntent(ev)
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	if err := response.Validate(); err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

// executeWorkspaceExecutionIntent converts a Workspace-owner selection fact
// to a Run-private SM-1 action. No ingress source supplies a target state;
// all activation, replacement, and continuation choices are made by Run
// against its own persisted records.
func (m *Biz) executeWorkspaceExecutionIntent(ev magicevent.Event) (runevents.WorkspaceExecutionIntentResult, error) {
	if m == nil {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.runs == nil || m.EventHub() == nil {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(runevents.WorkspaceExecutionIntentCommand)
	if !ok || ev.Source() != workspaceExecutionIntentRunSource {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	cmd = cmd.Normalized()
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	if m.stopping {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	switch cmd.Kind {
	case runtimecontract.RuntimeExecutionIntentResume:
		return m.resumeWorkspaceExecutionIntent(ctx, cmd)
	case runtimecontract.RuntimeExecutionIntentRestart:
		return m.restartWorkspaceExecutionIntent(ctx, cmd)
	default:
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
}

func runExecutionIntentResult(cmd runevents.WorkspaceExecutionIntentCommand, runID common.RunID, outcome runtimecontract.RuntimeExecutionIntentOutcome, reason runtimecontract.RuntimeExecutionIntentReason) runevents.WorkspaceExecutionIntentResult {
	return runevents.WorkspaceExecutionIntentResult{
		RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: cmd.WorkspaceID,
			RunID:       runID,
			RequestID:   cmd.RequestID,
			Kind:        cmd.Kind,
			Outcome:     outcome,
			Reason:      reason,
		},
	}
}

func (m *Biz) resumeWorkspaceExecutionIntent(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	current, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		if err == common.ErrNotFound {
			return runExecutionIntentResult(cmd, cmd.RunID, runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonSelectedRunAbsent), nil
		}
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	if runStateIsTerminal(current.State) {
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonSelectedRunTerminal), nil
	}
	// Resume is isolated from every later Workspace candidate or promotion. The
	// selected Run's immutable bootstrap is the only startup input authority.
	if current.RuntimeBootstrap == nil ||
		current.RuntimeBootstrap.Definition.Normalized().Revision != strings.TrimSpace(current.DefinitionRevision) ||
		current.RuntimeBootstrap.Definition.Normalized().StartupInputHash != strings.TrimSpace(current.StartupInputHash) {
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeBootstrapMissing), nil
	}
	switch current.State {
	case common.RunStateActive:
		m.publishContinuationRequested(ctx, current, "workspace-execution/resume/"+cmd.RequestID)
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted), nil
	case common.RunStatePending:
		transition, err := m.applyRunActivation(ctx, activateRunFact{
			WorkspaceID: cmd.WorkspaceID, RunID: current.ID,
			CausationID: "workspace-execution/resume/" + cmd.RequestID, ReconciliationRequestID: cmd.RequestID,
		})
		if err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, transition.Run.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated), nil
	case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending), nil
	default:
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonRunNotResumable), nil
	}
}

func (m *Biz) restartWorkspaceExecutionIntent(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	requestedIntent := replacementIntentFromCommand(cmd)
	items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	inventory := runInventoryByWorkspaceAndID(items)
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ActivationRequestID) == cmd.RequestID {
			if !replacementRunMatchesIntent(item, requestedIntent) {
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: restart request identity was reused with different sealed input", common.ErrConflict)
			}
			switch item.State {
			case common.RunStatePending:
				if err := validatePendingWorkspaceActivation(item); err != nil {
					return runevents.WorkspaceExecutionIntentResult{}, err
				}
				predecessor, err := validatePendingReplacementSuccessor(item, inventory)
				if err != nil {
					return runevents.WorkspaceExecutionIntentResult{}, err
				}
				if predecessor != nil {
					// A linked successor is not an ordinary create-before-activate
					// cut. Its predecessor must first seal SuccessorRunID and the
					// superseded lineage through the replacement private reducer.
					// Exact ingress replay may wake that persisted transaction, but
					// must not activate the child directly.
					m.scheduleReplacementRecovery(predecessor.WorkspaceID, predecessor.ID)
					return runExecutionIntentResult(cmd, item.ID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart), nil
				}
				transition, err := m.applyRunActivation(ctx, activateRunFact{
					WorkspaceID: item.WorkspaceID, RunID: item.ID,
					CausationID: "workspace-execution/restart-replay/" + cmd.RequestID,
				})
				if err != nil {
					return runevents.WorkspaceExecutionIntentResult{}, err
				}
				return runExecutionIntentResult(cmd, transition.Run.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated), nil
			case common.RunStateActive:
				// Run-state notifications are an at-least-once hint, not durable
				// authority. Replaying the exact restart must therefore provide a
				// fresh owner fact after the activation CAS/notification cut.
				m.publishContinuationRequested(ctx, item, "workspace-execution/restart-replay/"+cmd.RequestID)
				return runExecutionIntentResult(cmd, item.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted), nil
			case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
				return runExecutionIntentResult(cmd, item.ID, runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending), nil
			default:
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: restart request already belongs to a terminal or non-reconcilable Run", common.ErrConflict)
			}
		}
	}
	// A successor can be durably created before its predecessor seals the
	// SuccessorRunID CAS. Resolve the exact ActivationRequestID across the
	// complete inventory first; repository ordering must not let the older
	// predecessor shadow that already-materialized child.
	for _, item := range items {
		if item != nil && item.ReplacementIntent != nil && item.ReplacementIntent.RequestID == cmd.RequestID {
			if !item.ReplacementIntent.ValidFor(item) {
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: persisted replacement intent is invalid", common.ErrConflict)
			}
			expectedIntent := replacementIntentForPredecessor(requestedIntent, item)
			if !replacementIntentMatches(item.ReplacementIntent, expectedIntent) {
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: restart request identity was reused with different sealed input", common.ErrConflict)
			}
			if item.ReplacementIntent.State == runmodel.ReplacementIntentClarificationResolutionRejected {
				return runevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
			}
			runID := item.ID
			if item.ReplacementIntent.SuccessorRunID != "" {
				runID = item.ReplacementIntent.SuccessorRunID
			}
			m.scheduleReplacementRecovery(item.WorkspaceID, item.ID)
			return runExecutionIntentResult(cmd, runID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart), nil
		}
	}
	for _, item := range items {
		if item != nil && item.State == common.RunStatePending && strings.TrimSpace(item.ActivationRequestID) != "" {
			return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: another restart activation is durably pending", common.ErrConflict)
		}
	}
	for _, item := range items {
		if item == nil || item.ReplacementIntent == nil {
			continue
		}
		if !item.ReplacementIntent.ValidFor(item) {
			return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: persisted replacement intent is invalid", common.ErrConflict)
		}
		if item.ReplacementIntent.State != runmodel.ReplacementIntentCompleted &&
			item.ReplacementIntent.State != runmodel.ReplacementIntentClarificationResolutionRejected {
			return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: another Workspace replacement transaction is still open", common.ErrConflict)
		}
	}
	var predecessor *runmodel.Run
	var terminalPredecessor *runmodel.Run
	for _, item := range items {
		if item == nil {
			continue
		}
		if runStateIsTerminal(item.State) {
			if item.WorkspaceRunGeneration == 0 {
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: terminal Run lacks owner generation", common.ErrConflict)
			}
			if terminalPredecessor == nil || item.WorkspaceRunGeneration > terminalPredecessor.WorkspaceRunGeneration {
				terminalPredecessor = item
			}
			continue
		}
		if predecessor != nil && predecessor.ID != item.ID {
			return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: more than one non-terminal Run belongs to Workspace", common.ErrConflict)
		}
		predecessor = item
	}
	if predecessor == nil {
		predecessor = terminalPredecessor
	}
	if predecessor == nil {
		created, err := m.createWorkspaceExecutionReplacement(ctx, requestedIntent, "")
		if err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		if err := m.activateWorkspaceExecutionReplacement(ctx, created, cmd.RequestID); err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, created.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonNewRunActivated), nil
	}
	requestedIntent = replacementIntentForPredecessor(requestedIntent, predecessor)
	if runtimeDefinitionClarificationInputNotRevised(predecessor, requestedIntent) {
		return runExecutionIntentResult(cmd, predecessor.ID, runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonDefinitionClarificationPending), nil
	}
	if err := m.persistReplacementIntent(ctx, predecessor, requestedIntent); err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	// The predecessor owns the durable request. A non-terminal predecessor must
	// first complete ordered SM-4 delivery; an already-terminal predecessor
	// starts at the same transaction's successor-materialization stage. No
	// successor is activated before the Run-private lineage CAS.
	m.scheduleReplacementRecovery(predecessor.WorkspaceID, predecessor.ID)
	reason := runtimecontract.RuntimeExecutionIntentReasonPredecessorSupersedeRequested
	if runStateIsTerminal(predecessor.State) {
		reason = runtimecontract.RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested
	}
	return runExecutionIntentResult(cmd, predecessor.ID, runtimecontract.RuntimeExecutionIntentReplacementPending, reason), nil
}

// replacementIntentForPredecessor derives the immutable clarification
// identity from Run-owned predecessor state. Exact replay therefore compares
// the same ticket/startup-input lineage that was sealed on the first write,
// while any changed successor bootstrap still fails the payload comparison.
// It deliberately leaves State empty: only the replacement owner-private
// reducer may select the initial lifecycle state.
func replacementIntentForPredecessor(intent runmodel.ReplacementIntent, predecessor *runmodel.Run) runmodel.ReplacementIntent {
	if predecessor == nil {
		return intent
	}
	ticketID := predecessor.BlockingTicketID
	if !isRuntimeDefinitionClarificationTicket(ticketID) && predecessor.ReplacementIntent != nil {
		ticketID = predecessor.ReplacementIntent.ClarificationTicketID
	}
	if !isRuntimeDefinitionClarificationTicket(ticketID) {
		return intent
	}
	intent.ClarificationTicketID = ticketID
	intent.PredecessorStartupInputHash = strings.TrimSpace(predecessor.StartupInputHash)
	intent.SuccessorStartupInputHash = strings.TrimSpace(intent.StartupInputHash)
	intent.ReplacementIntentID = "runtime-definition-replacement/" + intent.RequestID
	intent.ReplacementIntentHash = runmodel.StableReplacementIntentHash(intent)
	return intent
}

// runtimeDefinitionClarificationInputNotRevised is the Run-owner admission
// guard for a clarification replacement. Both lineage coordinates must move
// before the private replacement reducer is invoked; ReplacementIntent.ValidFor
// retains the same rule as a defensive persistence invariant.
func runtimeDefinitionClarificationInputNotRevised(predecessor *runmodel.Run, intent runmodel.ReplacementIntent) bool {
	if predecessor == nil ||
		predecessor.State != common.RunStateBlockedOnDecision ||
		!isRuntimeDefinitionClarificationTicket(predecessor.BlockingTicketID) ||
		intent.ClarificationTicketID != predecessor.BlockingTicketID {
		return false
	}
	return strings.TrimSpace(intent.DefinitionRevision) == strings.TrimSpace(predecessor.DefinitionRevision) ||
		strings.TrimSpace(intent.StartupInputHash) == strings.TrimSpace(predecessor.StartupInputHash)
}

func replacementIntentMatches(current *runmodel.ReplacementIntent, requested runmodel.ReplacementIntent) bool {
	if current == nil || current.Bootstrap == nil || requested.Bootstrap == nil {
		return false
	}
	return current.RequestID == requested.RequestID &&
		current.WorkspaceID == requested.WorkspaceID &&
		current.GoalID == requested.GoalID &&
		current.DoDID == requested.DoDID &&
		strings.TrimSpace(current.DefinitionRevision) == strings.TrimSpace(requested.DefinitionRevision) &&
		strings.TrimSpace(current.StartupInputHash) == strings.TrimSpace(requested.StartupInputHash) &&
		current.ReconciliationScope == requested.ReconciliationScope &&
		current.Kind == requested.Kind &&
		strings.TrimSpace(current.Reason) == strings.TrimSpace(requested.Reason) &&
		current.ClarificationTicketID == requested.ClarificationTicketID &&
		strings.TrimSpace(current.ReplacementIntentID) == strings.TrimSpace(requested.ReplacementIntentID) &&
		strings.TrimSpace(current.ReplacementIntentHash) == strings.TrimSpace(requested.ReplacementIntentHash) &&
		strings.TrimSpace(current.PredecessorStartupInputHash) == strings.TrimSpace(requested.PredecessorStartupInputHash) &&
		strings.TrimSpace(current.SuccessorStartupInputHash) == strings.TrimSpace(requested.SuccessorStartupInputHash) &&
		runtimecontract.StableRuntimeBootstrapHash(*current.Bootstrap) == runtimecontract.StableRuntimeBootstrapHash(*requested.Bootstrap)
}

func replacementRunMatchesIntent(current *runmodel.Run, requested runmodel.ReplacementIntent) bool {
	if current == nil || current.RuntimeBootstrap == nil || requested.Bootstrap == nil {
		return false
	}
	return strings.TrimSpace(current.ActivationRequestID) == requested.RequestID &&
		current.WorkspaceID == requested.WorkspaceID &&
		current.GoalID == requested.GoalID &&
		current.DoDID == requested.DoDID &&
		strings.TrimSpace(current.DefinitionRevision) == strings.TrimSpace(requested.DefinitionRevision) &&
		strings.TrimSpace(current.StartupInputHash) == strings.TrimSpace(requested.StartupInputHash) &&
		current.ReconciliationScope == requested.ReconciliationScope &&
		runtimecontract.StableRuntimeBootstrapHash(*current.RuntimeBootstrap) == runtimecontract.StableRuntimeBootstrapHash(*requested.Bootstrap)
}

func replacementIntentFromCommand(cmd runevents.WorkspaceExecutionIntentCommand) runmodel.ReplacementIntent {
	bootstrap := cmd.Bootstrap.Clone()
	return runmodel.ReplacementIntent{
		RequestID: cmd.RequestID, WorkspaceID: cmd.WorkspaceID, GoalID: cmd.GoalID, DoDID: cmd.DoDID,
		DefinitionRevision: cmd.DefinitionRevision, StartupInputHash: cmd.StartupInputHash, Bootstrap: &bootstrap,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind:                runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + cmd.RequestID,
	}
}

func (m *Biz) persistReplacementIntent(ctx context.Context, predecessor *runmodel.Run, intent runmodel.ReplacementIntent) error {
	if predecessor == nil || predecessor.ID == "" || predecessor.WorkspaceID == "" || intent.State != "" {
		return common.ErrInvalidArgument
	}
	if intent.Bootstrap != nil {
		if err := runtimecontract.ValidateRuntimeBootstrapRaw(*intent.Bootstrap); err != nil {
			return err
		}
	}
	clone := runmodel.CloneReplacementIntent(intent)
	_, err := m.submitReplacementInternalEvent(ctx, m.runs, replacementInternalEvent{
		Kind: replacementInternalEventRequested, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID, Requested: &clone,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	return err
}

func (m *Biz) createWorkspaceExecutionReplacement(ctx context.Context, intent runmodel.ReplacementIntent, predecessorID common.RunID) (*runmodel.Run, error) {
	if intent.Bootstrap == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeBootstrapRaw(*intent.Bootstrap); err != nil {
		return nil, err
	}
	bootstrap := intent.Bootstrap.Clone()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	items, err := m.runs.ListByWorkspaceID(ctx, intent.WorkspaceID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ActivationRequestID) == intent.RequestID {
			if replacementRunMatchesIntent(item, intent) {
				return item, nil
			}
			return nil, common.ErrConflict
		}
	}
	var predecessorResultID common.ResultID
	if predecessorID != "" {
		var predecessor *runmodel.Run
		for _, item := range items {
			if item != nil && item.ID == predecessorID {
				predecessor = item
				break
			}
		}
		if predecessor == nil || !runStateIsTerminal(predecessor.State) || predecessor.TerminalDelivery == nil || predecessor.TerminalResultID == "" {
			return nil, fmt.Errorf("%w: replacement predecessor lacks terminal Result lineage", common.ErrConflict)
		}
		predecessorResultID = predecessor.TerminalResultID
	}
	item, err := submitRunCreationInternalEvent(ctx, m.runs, runCreationInternalEvent{
		Kind: runCreationInternalEventRequested,
		Requested: &runCreationRequestedFact{
			WorkspaceID: intent.WorkspaceID, GoalID: intent.GoalID, DoDID: intent.DoDID,
			PredecessorRunID: predecessorID, PredecessorResultID: predecessorResultID,
			DefinitionRevision: intent.DefinitionRevision, StartupInputHash: intent.StartupInputHash,
			InitialStepKind: llmcommon.StepKindWorkspaceExecution, RuntimeBootstrap: &bootstrap,
			ActivationRequestID: intent.RequestID, ReconciliationScope: intent.ReconciliationScope,
		},
	})
	if err != nil {
		return nil, err
	}
	if err := m.publishRunEvent(ctx, runevents.TopicRunCreated, item); err != nil {
		return nil, err
	}
	// Workspace execution replacement persists the successor and its lineage
	// inside the Run owner. Hand off the sealed activation-memory fact only
	// after the Run record is durable.
	m.publishRuntimeActivationMemoryRecall(ctx, runtimeActivationMemoryRecall{
		RunID: item.ID, WorkspaceID: item.WorkspaceID, RuntimeBootstrap: item.RuntimeBootstrap,
	}, item.CreatedAt)
	return item, nil
}

func (m *Biz) activateWorkspaceExecutionReplacement(ctx context.Context, run *runmodel.Run, requestID string) error {
	if run == nil {
		return common.ErrInvalidArgument
	}
	_, err := m.applyRunActivation(ctx, activateRunFact{WorkspaceID: run.WorkspaceID, RunID: run.ID, CausationID: "workspace-execution/restart/" + requestID})
	return err
}

// validatePendingWorkspaceActivation proves that a pending Run is exactly the
// durable first half of Workspace restart activation. Recovery never guesses
// from RunID or timestamps and never rebuilds sealed startup input.
func validatePendingWorkspaceActivation(item *runmodel.Run) error {
	if item == nil || item.State != common.RunStatePending || item.ID == "" || item.WorkspaceID == "" ||
		item.WorkspaceRunGeneration == 0 || strings.TrimSpace(item.ActivationRequestID) == "" ||
		item.RuntimeBootstrap == nil || item.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull ||
		strings.TrimSpace(item.InitialStepKind) != llmcommon.StepKindWorkspaceExecution ||
		item.Lineage.LineageState != runmodel.RunLineageStateActive || item.Lineage.SuccessorRunID != "" ||
		strings.TrimSpace(item.StartedAt) != "" || strings.TrimSpace(item.EndedAt) != "" || item.TerminalDelivery != nil ||
		item.BlockingTicketID != "" || item.BlockingApprovalID != "" || strings.TrimSpace(item.FailureReason) != "" {
		return fmt.Errorf("%w: pending restart activation record is incomplete or inconsistent", common.ErrConflict)
	}
	if item.ID != runmodel.WorkspaceRunID(item.WorkspaceRunGeneration) {
		return fmt.Errorf("%w: pending restart activation generation is not canonical", common.ErrConflict)
	}
	if err := item.RuntimeBootstrap.Validate(); err != nil {
		return fmt.Errorf("%w: pending restart bootstrap is invalid: %v", common.ErrConflict, err)
	}
	definition := item.RuntimeBootstrap.Definition.Normalized()
	if definition.GoalID != item.GoalID || definition.DoDID != item.DoDID ||
		definition.Revision != strings.TrimSpace(item.DefinitionRevision) ||
		definition.StartupInputHash != strings.TrimSpace(item.StartupInputHash) {
		return fmt.Errorf("%w: pending restart bootstrap does not match Run identity", common.ErrConflict)
	}
	return nil
}

// validatePendingReplacementSuccessor distinguishes the second half of a
// replacement transaction from a root restart activation. A successor can be
// durable before the predecessor's lineage CAS; neither startup nor an exact
// Workspace ingress replay may activate it across that cut.
type workspaceRunInventoryKey struct {
	workspaceID common.WorkspaceID
	runID       common.RunID
}

func validatePendingReplacementSuccessor(item *runmodel.Run, inventory map[workspaceRunInventoryKey]*runmodel.Run) (*runmodel.Run, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	predecessorID := item.Lineage.PredecessorRunID
	predecessorResultID := item.Lineage.PredecessorResultID
	if predecessorID == "" && predecessorResultID == "" {
		return nil, nil
	}
	if predecessorID == "" || predecessorResultID == "" || item.Lineage.LineageState != runmodel.RunLineageStateActive || item.Lineage.SuccessorRunID != "" {
		return nil, fmt.Errorf("%w: pending replacement successor lineage is incomplete", common.ErrConflict)
	}
	predecessor := inventory[workspaceRunInventoryKey{workspaceID: item.WorkspaceID, runID: predecessorID}]
	if predecessor == nil || !runStateIsTerminal(predecessor.State) ||
		predecessor.TerminalResultID != predecessorResultID || predecessor.ReplacementIntent == nil ||
		!predecessor.ReplacementIntent.ValidFor(predecessor) ||
		!replacementRunMatchesIntent(item, *predecessor.ReplacementIntent) {
		return nil, fmt.Errorf("%w: pending replacement successor is not bound to its predecessor transaction", common.ErrConflict)
	}
	if predecessor.WorkspaceRunGeneration == ^uint64(0) ||
		item.WorkspaceRunGeneration != predecessor.WorkspaceRunGeneration+1 ||
		item.ID != runmodel.WorkspaceRunID(item.WorkspaceRunGeneration) {
		return nil, fmt.Errorf("%w: pending replacement successor generation is not canonical", common.ErrConflict)
	}
	intent := predecessor.ReplacementIntent
	if intent.Recovery.Stage != runmodel.ReplacementRecoverySuccessorMaterialization || !replacementRecoveryAdmissibleAtStartup(intent.Recovery.State) {
		return nil, fmt.Errorf("%w: pending replacement successor has no recoverable owner lease", common.ErrConflict)
	}
	switch intent.State {
	case runmodel.ReplacementIntentTerminationAcknowledged:
		if intent.SuccessorRunID != "" || predecessor.Lineage.SuccessorRunID != "" {
			return nil, fmt.Errorf("%w: unbound replacement successor has conflicting predecessor lineage", common.ErrConflict)
		}
	case runmodel.ReplacementIntentSuccessorCreated:
		if intent.SuccessorRunID != item.ID || predecessor.Lineage.SuccessorRunID != item.ID || predecessor.Lineage.LineageState != runmodel.RunLineageStateSuperseded {
			return nil, fmt.Errorf("%w: replacement successor lineage binding drifted", common.ErrConflict)
		}
	default:
		return nil, fmt.Errorf("%w: pending replacement successor is inconsistent with predecessor stage", common.ErrConflict)
	}
	return predecessor, nil
}

// replacementRecoveryAdmissibleAtStartup separates structural durability from
// process-local scheduling. Exhausted is a valid terminal owner lease: startup
// must retain its linked pending child for diagnosis/exact replay, but
// scheduleReplacementRecovery deliberately mounts no new attempt for it.
func replacementRecoveryAdmissibleAtStartup(state runmodel.ReplacementRecoveryState) bool {
	return state == runmodel.ReplacementRecoveryScheduled ||
		state == runmodel.ReplacementRecoveryRunning ||
		state == runmodel.ReplacementRecoveryExhausted
}

func runInventoryByWorkspaceAndID(items []*runmodel.Run) map[workspaceRunInventoryKey]*runmodel.Run {
	ret := make(map[workspaceRunInventoryKey]*runmodel.Run, len(items))
	for _, item := range items {
		if item != nil && item.WorkspaceID != "" && item.ID != "" {
			ret[workspaceRunInventoryKey{workspaceID: item.WorkspaceID, runID: item.ID}] = item
		}
	}
	return ret
}

// recoverPendingWorkspaceActivations closes the Run-create -> activation CAS
// crash cut before downstream startup inventory is visible. A failed owner
// reducer or repository write aborts startup; it is never swallowed.
func (m *Biz) recoverPendingWorkspaceActivations() error {
	if m == nil || m.runs == nil {
		return common.ErrInvalidArgument
	}
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	items, err := m.runs.List(context.Background())
	if err != nil {
		return err
	}
	inventory := runInventoryByWorkspaceAndID(items)
	// Validate the complete owner inventory before the first CAS. Startup must
	// not partially activate one Workspace and only then discover corrupt
	// activation lineage in another.
	currentByWorkspace := make(map[common.WorkspaceID]common.RunID)
	plan := make([]*runmodel.Run, 0)
	for _, item := range items {
		if item == nil {
			continue
		}
		if !runStateIsTerminal(item.State) {
			if currentRunID := currentByWorkspace[item.WorkspaceID]; currentRunID != "" && currentRunID != item.ID {
				return fmt.Errorf("%w: Workspace %s has more than one non-terminal Run", common.ErrConflict, item.WorkspaceID)
			}
			currentByWorkspace[item.WorkspaceID] = item.ID
		}
		if item.State != common.RunStatePending {
			continue
		}
		if strings.TrimSpace(item.ActivationRequestID) == "" {
			return fmt.Errorf("%w: pending Workspace activation lacks request identity", common.ErrConflict)
		}
		if err := validatePendingWorkspaceActivation(item); err != nil {
			return err
		}
		predecessor, err := validatePendingReplacementSuccessor(item, inventory)
		if err != nil {
			return err
		}
		if predecessor != nil {
			// Start remounts the predecessor's persisted replacement lease.
			// Leave the child pending until that private reducer seals lineage.
			continue
		}
		plan = append(plan, item)
	}
	for _, item := range plan {
		ctx, err := common.BindRuntimeWorkspaceContext(context.Background(), item.WorkspaceID)
		if err != nil {
			return err
		}
		if _, err := m.applyRunActivation(ctx, activateRunFact{
			WorkspaceID: item.WorkspaceID, RunID: item.ID,
			CausationID: "workspace-execution/startup-recovery/" + strings.TrimSpace(item.ActivationRequestID),
		}); err != nil {
			return fmt.Errorf("recover pending Workspace activation %s/%s: %w", item.WorkspaceID, item.ID, err)
		}
	}
	return nil
}

const (
	replacementRecoveryInitialBackoff = 100 * time.Millisecond
	replacementRecoveryMaxBackoff     = 5 * time.Second
)

func replacementRecoveryTaskKey(workspaceID common.WorkspaceID, runID common.RunID) string {
	return string(workspaceID) + "\x00" + string(runID)
}

func replacementRecoveryRetryDueAt(failedAttempts uint64) string {
	backoff := replacementRecoveryInitialBackoff
	for step := uint64(1); step < failedAttempts && backoff < replacementRecoveryMaxBackoff; step++ {
		backoff *= 2
		if backoff > replacementRecoveryMaxBackoff {
			backoff = replacementRecoveryMaxBackoff
		}
	}
	return time.Now().UTC().Add(backoff).Format(time.RFC3339Nano)
}

// scheduleReplacementRecovery mounts one process-local wakeup for the exact
// persisted Run-owned generation. The map is not authority: duplicate tasks
// still have to claim the same lease through the private reducer.
func (m *Biz) scheduleReplacementRecovery(workspaceID common.WorkspaceID, predecessorRunID common.RunID) {
	if m == nil || workspaceID == "" || predecessorRunID == "" {
		return
	}
	lifecycleCtx, lifecycleEpoch, runs, ok := m.beginReplacementRecoveryLifecycle()
	if !ok {
		return
	}
	defer m.replacementRecoveryWG.Done()
	background := m.BackgroundRoutine()
	if background == nil {
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	item, err := runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
	if err != nil || item == nil || item.ReplacementIntent == nil || !item.ReplacementIntent.ValidFor(item) {
		return
	}
	lease := item.ReplacementIntent.Recovery
	if lease.State != runmodel.ReplacementRecoveryScheduled {
		return
	}
	key := replacementRecoveryTaskKey(workspaceID, predecessorRunID)
	m.replacementRecoveryMu.Lock()
	if m.replacementRecoveryEpoch != lifecycleEpoch || m.replacementRecoveryTasks == nil || lifecycleCtx.Err() != nil {
		m.replacementRecoveryMu.Unlock()
		return
	}
	if generation, mounted := m.replacementRecoveryTasks[key]; mounted && generation >= lease.Generation {
		m.replacementRecoveryMu.Unlock()
		return
	}
	m.replacementRecoveryTasks[key] = lease.Generation
	m.replacementRecoveryMu.Unlock()

	err = background.AsyncFunction(func() {
		clearOnReturn := true
		defer func() {
			if clearOnReturn {
				m.clearReplacementRecoveryWakeup(lifecycleEpoch, key, lease.Generation)
			}
		}()
		if due, parseErr := time.Parse(time.RFC3339Nano, lease.DueAt); parseErr == nil {
			if wait := time.Until(due); wait > 0 {
				timer := time.NewTimer(wait)
				defer timer.Stop()
				select {
				case <-lifecycleCtx.Done():
					return
				case <-timer.C:
				}
			}
		}
		if !m.beginReplacementRecoveryGeneration(lifecycleCtx, lifecycleEpoch, key, lease.Generation) {
			return
		}
		_ = m.driveReplacementRecoveryGeneration(workspaceID, predecessorRunID, lease.Generation)
		m.replacementRecoveryWG.Done()
		m.clearReplacementRecoveryWakeup(lifecycleEpoch, key, lease.Generation)
		clearOnReturn = false
		m.scheduleReplacementRecovery(workspaceID, predecessorRunID)
	})
	if err != nil {
		m.clearReplacementRecoveryWakeup(lifecycleEpoch, key, lease.Generation)
	}
}

// beginReplacementRecoveryLifecycle joins the owner-local Stop drain before
// reading Run dependencies. The returned context/epoch are wakeup identity,
// never recovery authority; stage/generation/due time remain persisted in Run.
func (m *Biz) beginReplacementRecoveryLifecycle() (context.Context, uint64, repository.RunRepository, bool) {
	if m == nil {
		return nil, 0, nil, false
	}
	m.replacementRecoveryMu.Lock()
	defer m.replacementRecoveryMu.Unlock()
	if m.replacementRecoveryCtx == nil || m.replacementRecoveryTasks == nil || m.replacementRecoveryCtx.Err() != nil || m.runs == nil {
		return nil, 0, nil, false
	}
	m.replacementRecoveryWG.Add(1)
	return m.replacementRecoveryCtx, m.replacementRecoveryEpoch, m.runs, true
}

func (m *Biz) beginReplacementRecoveryGeneration(lifecycleCtx context.Context, lifecycleEpoch uint64, key string, generation uint64) bool {
	if m == nil || lifecycleCtx == nil || generation == 0 {
		return false
	}
	m.replacementRecoveryMu.Lock()
	defer m.replacementRecoveryMu.Unlock()
	if lifecycleCtx.Err() != nil || m.replacementRecoveryCtx == nil || m.replacementRecoveryEpoch != lifecycleEpoch || m.replacementRecoveryTasks == nil || m.replacementRecoveryTasks[key] != generation {
		return false
	}
	m.replacementRecoveryWG.Add(1)
	return true
}

func (m *Biz) clearReplacementRecoveryWakeup(lifecycleEpoch uint64, key string, generation uint64) {
	if m == nil {
		return
	}
	m.replacementRecoveryMu.Lock()
	defer m.replacementRecoveryMu.Unlock()
	if m.replacementRecoveryEpoch != lifecycleEpoch || m.replacementRecoveryTasks == nil {
		return
	}
	if current, ok := m.replacementRecoveryTasks[key]; ok && current == generation {
		delete(m.replacementRecoveryTasks, key)
	}
}

func (m *Biz) driveReplacementRecoveryGeneration(workspaceID common.WorkspaceID, predecessorRunID common.RunID, generation uint64) error {
	intent, drive, err := m.claimReplacementRecovery(workspaceID, predecessorRunID, generation)
	if err != nil || !drive {
		return err
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	switch intent.Recovery.Stage {
	case runmodel.ReplacementRecoveryClarificationResolution:
		predecessor, loadErr := m.runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
		if loadErr == nil {
			loadErr = m.bindRuntimeClarificationReplacement(ctx, predecessor, generation)
		}
		err = loadErr
	case runmodel.ReplacementRecoveryAdministrativeTermination:
		err = m.deliverClaimedReplacementTermination(ctx, predecessorRunID, intent, generation)
	case runmodel.ReplacementRecoverySuccessorMaterialization:
		predecessor, loadErr := m.runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
		if loadErr == nil {
			_, loadErr = m.materializeReplacementAfterTerminalDelivery(ctx, predecessor, intent.TerminalDeliveryID, generation)
		}
		err = loadErr
	default:
		err = common.ErrConflict
	}
	if err != nil {
		_ = m.recordReplacementRecoveryFailure(workspaceID, predecessorRunID, intent, generation, err)
	}
	return err
}

// deliverReplacementTermination is retained as an owner-local focused-test
// entry. Production scheduling always uses the persisted recovery generation.
func (m *Biz) deliverReplacementTermination(workspaceID common.WorkspaceID, predecessorRunID common.RunID) error {
	runs, err := m.activeRunRepository()
	if err != nil {
		return err
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	item, err := runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
	if err != nil || item.ReplacementIntent == nil {
		return err
	}
	return m.driveReplacementRecoveryGeneration(workspaceID, predecessorRunID, item.ReplacementIntent.Recovery.Generation)
}

func (m *Biz) deliverClaimedReplacementTermination(ctx context.Context, predecessorRunID common.RunID, intent runmodel.ReplacementIntent, generation uint64) error {
	workspaceID := intent.WorkspaceID
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{
		WorkspaceID: workspaceID, RunID: predecessorRunID, Kind: intent.Kind,
		RequestedBy: ModuleID, Reason: intent.Reason,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(orchestratorevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "run", SubjectID: string(predecessorRunID), WorkspaceID: string(workspaceID), RunID: string(predecessorRunID)}))
	response := m.SendEvent(event)
	if response == nil {
		return fmt.Errorf("%w: Runtime termination returned no result", common.ErrConflict)
	}
	if responseErr := response.Error(); responseErr != nil {
		return responseErr
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	ack, ok := data.(orchestratorevents.RequestRuntimeTerminationResult)
	if !ok || !ack.WorkspaceReleased || strings.TrimSpace(ack.TerminalDeliveryID) == "" || ack.StateRevision == 0 {
		return fmt.Errorf("%w: invalid Runtime termination acknowledgement", common.ErrConflict)
	}
	return m.acknowledgeReplacementTermination(workspaceID, predecessorRunID, intent.RequestID, generation, ack)
}

func (m *Biz) claimReplacementRecovery(workspaceID common.WorkspaceID, predecessorRunID common.RunID, generation uint64) (runmodel.ReplacementIntent, bool, error) {
	runs, err := m.activeRunRepository()
	if err != nil {
		return runmodel.ReplacementIntent{}, false, err
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	current, err := runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
	if err != nil {
		return runmodel.ReplacementIntent{}, false, err
	}
	if current == nil || current.ReplacementIntent == nil || !current.ReplacementIntent.ValidFor(current) {
		return runmodel.ReplacementIntent{}, false, common.ErrConflict
	}
	requestID := current.ReplacementIntent.RequestID
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	transition, err := m.submitReplacementInternalEvent(ctx, runs, replacementInternalEvent{
		Kind: replacementInternalEventRecoveryClaimed, WorkspaceID: workspaceID, RunID: predecessorRunID,
		OccurredAt:      observedAt,
		RecoveryClaimed: &replacementRecoveryClaimedFact{RequestID: requestID, Generation: generation, ClaimedAt: observedAt},
	})
	if err != nil {
		return runmodel.ReplacementIntent{}, false, err
	}
	if transition.Intent == nil {
		return runmodel.ReplacementIntent{}, false, nil
	}
	return runmodel.CloneReplacementIntent(*transition.Intent), transition.Drive, nil
}

func (m *Biz) acknowledgeReplacementTermination(workspaceID common.WorkspaceID, predecessorRunID common.RunID, requestID string, generation uint64, ack orchestratorevents.RequestRuntimeTerminationResult) error {
	runs, err := m.activeRunRepository()
	if err != nil {
		return err
	}
	acknowledgedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = m.submitReplacementInternalEvent(context.Background(), runs, replacementInternalEvent{
		Kind: replacementInternalEventTerminationAcknowledged, WorkspaceID: workspaceID, RunID: predecessorRunID,
		OccurredAt: acknowledgedAt,
		TerminationAcknowledged: &replacementTerminationAcknowledgedFact{
			RequestID: requestID, Generation: generation, Acknowledgement: ack, NextDueAt: acknowledgedAt,
		},
	})
	return err
}

func (m *Biz) recordReplacementRecoveryFailure(workspaceID common.WorkspaceID, predecessorRunID common.RunID, intent runmodel.ReplacementIntent, generation uint64, cause error) error {
	if cause == nil {
		return nil
	}
	runs, err := m.activeRunRepository()
	if err != nil {
		return err
	}
	failedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = m.submitReplacementInternalEvent(context.Background(), runs, replacementInternalEvent{
		Kind: replacementInternalEventRecoveryFailed, WorkspaceID: workspaceID, RunID: predecessorRunID,
		OccurredAt: failedAt,
		RecoveryFailed: &replacementRecoveryFailedFact{
			RequestID: intent.RequestID, Generation: generation, Stage: intent.Recovery.Stage,
			FailedAt: failedAt, RetryDueAt: replacementRecoveryRetryDueAt(intent.Recovery.Attempt + 1), Error: cause.Error(),
		},
	})
	return err
}

func (m *Biz) activeRunRepository() (repository.RunRepository, error) {
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
		return nil, common.ErrConflict
	}
	return m.runs, nil
}

func (m *Biz) materializeReplacementAfterTerminalDelivery(ctx context.Context, predecessor *runmodel.Run, terminalDeliveryID string, generations ...uint64) (*runmodel.Run, error) {
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	if predecessor == nil || strings.TrimSpace(terminalDeliveryID) == "" {
		return nil, common.ErrInvalidArgument
	}
	authoritative, err := m.runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		return nil, err
	}
	predecessor = authoritative
	if predecessor.ReplacementIntent == nil || !runStateIsTerminal(predecessor.State) {
		return nil, common.ErrInvalidArgument
	}
	intent := *predecessor.ReplacementIntent
	if !intent.ValidFor(predecessor) {
		return nil, common.ErrConflict
	}
	if intent.State == runmodel.ReplacementIntentCompleted && intent.TerminalDeliveryID == terminalDeliveryID {
		return m.runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, intent.SuccessorRunID)
	}
	generation := intent.Recovery.Generation
	if len(generations) > 0 {
		generation = generations[0]
	}
	if generation == 0 || intent.Recovery.Stage != runmodel.ReplacementRecoverySuccessorMaterialization || intent.Recovery.State != runmodel.ReplacementRecoveryRunning || intent.Recovery.Generation != generation {
		return nil, common.ErrConflict
	}
	created, err := m.createWorkspaceExecutionReplacement(ctx, intent, predecessor.ID)
	if err != nil {
		return nil, fmt.Errorf("create replacement successor: %w", err)
	}
	if _, err := m.submitReplacementInternalEvent(ctx, m.runs, replacementInternalEvent{
		Kind: replacementInternalEventSuccessorMaterialized, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SuccessorMaterialized: &replacementSuccessorMaterializedFact{
			RequestID: intent.RequestID, Generation: generation, SuccessorRunID: created.ID, TerminalDeliveryID: terminalDeliveryID,
		},
	}); err != nil {
		return nil, fmt.Errorf("record replacement successor materialization: %w", err)
	}
	if err := m.activateWorkspaceExecutionReplacement(ctx, created, intent.RequestID); err != nil {
		return nil, fmt.Errorf("activate replacement successor: %w", err)
	}
	if _, err := m.submitReplacementInternalEvent(ctx, m.runs, replacementInternalEvent{
		Kind: replacementInternalEventSuccessorActivated, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SuccessorActivated: &replacementSuccessorActivatedFact{
			RequestID: intent.RequestID, Generation: generation, SuccessorRunID: created.ID,
			TerminalDeliveryID: terminalDeliveryID, AcknowledgedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		return nil, fmt.Errorf("record replacement successor activation: %w", err)
	}
	return created, nil
}

func (m *Biz) recoverPendingWorkspaceExecutionSuccessors() {
	if m == nil {
		return
	}
	m.commandMu.RLock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
		m.commandMu.RUnlock()
		return
	}
	runs := m.runs
	m.commandMu.RUnlock()
	items, err := runs.List(context.Background())
	if err != nil {
		return
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		// The Run record is the durable source for activation selection. Re-drive
		// the create-once Memory fact on startup so a crash between Run commit and
		// the cross-owner handoff cannot permanently lose the recall trace. Exact
		// replay is zero-write at the Memory owner and never becomes SM authority.
		m.publishRuntimeActivationMemoryRecall(context.Background(), runtimeActivationMemoryRecall{
			RunID: item.ID, WorkspaceID: item.WorkspaceID, RuntimeBootstrap: item.RuntimeBootstrap,
		}, item.CreatedAt)
		if item.ReplacementIntent == nil || !item.ReplacementIntent.ValidFor(item) {
			continue
		}
		lease := item.ReplacementIntent.Recovery
		if lease.State == runmodel.ReplacementRecoveryRunning {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			_, _ = m.submitReplacementInternalEvent(context.Background(), runs, replacementInternalEvent{
				Kind: replacementInternalEventRecoveryRecovered, WorkspaceID: item.WorkspaceID, RunID: item.ID,
				OccurredAt: now,
				RecoveryRecovered: &replacementRecoveryRecoveredFact{
					RequestID: item.ReplacementIntent.RequestID, Generation: lease.Generation,
					RecoveredAt: now, RetryDueAt: now,
				},
			})
		}
		m.scheduleReplacementRecovery(item.WorkspaceID, item.ID)
	}
}

func isRuntimeDefinitionClarificationTicket(ticketID common.TicketID) bool {
	return strings.HasPrefix(strings.TrimSpace(string(ticketID)), "runtime-definition-clarification:")
}

func (m *Biz) bindRuntimeClarificationReplacement(ctx context.Context, predecessor *runmodel.Run, generations ...uint64) error {
	if predecessor == nil || predecessor.ReplacementIntent == nil {
		return common.ErrInvalidArgument
	}
	intent := *predecessor.ReplacementIntent
	if len(generations) == 0 {
		return m.driveReplacementRecoveryGeneration(predecessor.WorkspaceID, predecessor.ID, intent.Recovery.Generation)
	}
	generation := generations[0]
	if intent.State != runmodel.ReplacementIntentPendingClarificationResolution {
		if intent.State == runmodel.ReplacementIntentClarificationResolutionBound || intent.State == runmodel.ReplacementIntentCompleted {
			return nil
		}
		return common.ErrConflict
	}
	if intent.Recovery.State != runmodel.ReplacementRecoveryRunning || intent.Recovery.Stage != runmodel.ReplacementRecoveryClarificationResolution || intent.Recovery.Generation != generation {
		return common.ErrConflict
	}
	cmd := decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand{
		TicketID: intent.ClarificationTicketID, WorkspaceID: intent.WorkspaceID, PredecessorRunID: predecessor.ID,
		ReplacementIntentID: intent.ReplacementIntentID, PredecessorDefinitionRevision: predecessor.DefinitionRevision,
		PredecessorStartupInputHash: intent.PredecessorStartupInputHash, SuccessorDefinitionRevision: intent.DefinitionRevision,
		SuccessorStartupInputHash: intent.SuccessorStartupInputHash, ReplacementIntentHash: intent.ReplacementIntentHash,
	}
	if err := cmd.Validate(); err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "ticket", SubjectID: string(cmd.TicketID), WorkspaceID: string(cmd.WorkspaceID), RunID: string(cmd.PredecessorRunID)}))
	response := m.SendEvent(event)
	if response == nil {
		return common.ErrConflict
	}
	if responseErr := response.Error(); responseErr != nil {
		if responseErr.Cause != nil {
			return responseErr.Cause
		}
		return responseErr
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	result, ok := data.(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult)
	if !ok || result.ValidateFor(cmd) != nil {
		return common.ErrConflict
	}
	if result.ConflictKind == decisionevents.RuntimeDefinitionClarificationNoConflict {
		observedAt := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := m.submitReplacementInternalEvent(ctx, m.runs, replacementInternalEvent{
			Kind: replacementInternalEventClarificationResolutionObserved, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
			OccurredAt: observedAt,
			ClarificationResolution: &replacementClarificationResolutionFact{
				RequestID: intent.RequestID, Generation: generation, ResolutionID: result.ResolutionID, ResolutionBindingHash: result.ResolutionBindingHash,
				ConflictKind: result.ConflictKind, NextDueAt: observedAt,
			},
		})
		return err
	}
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := m.submitReplacementInternalEvent(ctx, m.runs, replacementInternalEvent{
		Kind: replacementInternalEventClarificationResolutionObserved, WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		OccurredAt: observedAt,
		ClarificationResolution: &replacementClarificationResolutionFact{
			RequestID: intent.RequestID, Generation: generation, ResolutionBindingHash: result.ResolutionBindingHash, ConflictKind: result.ConflictKind,
			CurrentReplacementIntentID: result.CurrentReplacementIntentID, CurrentReplacementIntentHash: result.CurrentReplacementIntentHash,
			NextDueAt: observedAt,
		},
	}); err != nil {
		return err
	}
	return nil
}
