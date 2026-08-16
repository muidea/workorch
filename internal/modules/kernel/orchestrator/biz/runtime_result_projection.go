package orchestrator

// Runtime terminal projection is owned by Biz.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// deliverRuntimeTerminal is the single SM-4 terminal delivery lane. The
// persisted aggregate remains authority. Result, Workspace and Run receive
// typed terminal facts and only mutate their own state, in that order.
func (m *Biz) deliverRuntimeTerminal(ctx context.Context, aggregate runtime.RuntimeAggregate) error {
	if m == nil || (aggregate.Decision.Terminal == nil && aggregate.Termination == nil) {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return fmt.Errorf("persisted Runtime terminal aggregate is invalid: %w", err)
	}
	request := terminalDeliveryRequest(aggregate)
	if err := request.Validate(); err != nil {
		return err
	}
	if !deliveryAcknowledged(aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerResult) {
		if err := m.deliverRuntimeResult(ctx, request, runtimeResultMaterialFromAggregate(aggregate)); err != nil {
			return err
		}
	}
	if !m.persistedDeliveryAcknowledged(ctx, aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace) {
		if err := m.deliverRuntimeWorkspace(ctx, request); err != nil {
			return err
		}
	}
	if !m.persistedDeliveryAcknowledged(ctx, aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerRun) {
		return m.deliverRuntimeRun(ctx, request)
	}
	return nil
}

func deliveryAcknowledged(aggregate runtime.RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	for _, acknowledgement := range aggregate.DeliveryAcknowledgements {
		if acknowledgement.TerminalDeliveryID == deliveryID && acknowledgement.Owner == owner {
			return true
		}
	}
	return false
}

func (m *Biz) persistedDeliveryAcknowledged(ctx context.Context, aggregate runtime.RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	if m == nil || m.runtimeController == nil {
		return deliveryAcknowledged(aggregate, deliveryID, owner)
	}
	loaded, err := m.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	return err == nil && deliveryAcknowledged(loaded, deliveryID, owner)
}

func terminalDeliveryRequest(aggregate runtime.RuntimeAggregate) orchestratorevents.RuntimeTerminalDeliveryRequestData {
	outcome := terminalOutcomeForAggregate(aggregate)
	deliveryID := runtimecontract.StableTerminalDeliveryID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, outcome)
	if aggregate.Termination != nil {
		return orchestratorevents.RuntimeTerminalDeliveryRequestData{
			RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision,
				EvidenceRevision: aggregate.Facts.EvidenceRevision, DispatchID: aggregate.Decision.AuthoritySnapshot.ID,
				Outcome: outcome, TerminalDeliveryID: deliveryID,
			}, SourceOwner: ModuleID,
			CausationID: "terminal-delivery/" + deliveryID, StateRevision: aggregate.StateRevision,
		}
	}
	return orchestratorevents.RuntimeTerminalDeliveryRequestData{
		RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision,
			EvidenceRevision: aggregate.Facts.EvidenceRevision, DispatchID: aggregate.Decision.AuthoritySnapshot.ID,
			Outcome: outcome, TerminalDeliveryID: deliveryID,
		}, SourceOwner: ModuleID,
		CausationID:   "terminal-delivery/" + deliveryID,
		StateRevision: aggregate.StateRevision,
	}
}

func terminalOutcomeForAggregate(aggregate runtime.RuntimeAggregate) runtimecontract.TerminalOutcome {
	if aggregate.Termination != nil {
		kind := runtimecontract.TerminalOutcomeKind("")
		switch aggregate.Termination.Kind {
		case runtimecontract.RuntimeTerminationCancelled:
			kind = runtimecontract.TerminalOutcomeKindCancelled
		case runtimecontract.RuntimeTerminationSuperseded:
			kind = runtimecontract.TerminalOutcomeKindSuperseded
		}
		return runtimecontract.TerminalOutcome{Kind: kind, Code: "administrative_" + string(aggregate.Termination.Kind), Summary: aggregate.Termination.Reason}.Normalized()
	}
	if aggregate.Decision.Terminal == nil {
		return runtimecontract.TerminalOutcome{}
	}
	kind := runtimecontract.TerminalOutcomeKind("")
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateComplete:
		kind = runtimecontract.TerminalOutcomeKindCompleted
	case runtimecontract.ConvergenceStateFailed:
		kind = runtimecontract.TerminalOutcomeKindFailed
	}
	return runtimecontract.TerminalOutcome{Kind: kind, Code: aggregate.Decision.Terminal.Code, Summary: aggregate.Decision.Terminal.Summary}.Normalized()
}

func (m *Biz) deliverRuntimeResult(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData, material resultevents.ResultMaterial) error {
	if m == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	command := resultevents.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: request.Identity(), SourceOwner: ModuleID, CausationID: request.CausationID},
		Material:                    material,
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultFactTerminalDelivery, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicResultFactTerminalDelivery, "result", string(material.ID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	if response == nil || response.Error() != nil {
		if response != nil {
			return response.Error()
		}
		return common.ErrInvalidArgument
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	ownerResult, ok := data.(resultevents.ResultLifecycleCommandResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) deliverRuntimeWorkspace(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData) error {
	command := workspaceevents.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: request.Identity(), SourceOwner: ModuleID, CausationID: request.CausationID}}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceFactTerminalDelivery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceFactTerminalDelivery, "workspace", string(request.WorkspaceID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	if response == nil || response.Error() != nil {
		if response != nil {
			return response.Error()
		}
		return common.ErrInvalidArgument
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	ownerResult, ok := data.(workspaceevents.WorkspaceLifecycleCommandResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) deliverRuntimeRun(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData) error {
	command := runevents.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: request.Identity(), SourceOwner: ModuleID, CausationID: request.CausationID}}
	event := magicevent.NewEventWithContext(runevents.TopicRunFactTerminalDelivery, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunFactTerminalDelivery, "run", string(request.RunID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	ownerResult, err := runLifecycleCommandResponse(response, runevents.TopicRunFactTerminalDelivery)
	if err != nil {
		return err
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

// recordRuntimeDeliveryAcknowledgement is deliberately private to the terminal
// delivery lane.  Its input is the concrete synchronous Result/Workspace/Run
// owner response; no public EventHub command may manufacture an SM-4 ack.
func (m *Biz) recordRuntimeDeliveryAcknowledgement(ctx context.Context, acknowledgement *runtimecontract.RuntimeDeliveryAcknowledgement) error {
	if acknowledgement == nil {
		return common.ErrInvalidArgument
	}
	// Focused owner-boundary tests may construct a delivery Biz without the
	// Runtime repository; production Configure always installs it. The
	// resource-owner transition remains fully exercised in that narrow fixture.
	if m.runtimeController == nil {
		return nil
	}
	_, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventTerminalDeliveryAcknowledged, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: acknowledgement.WorkspaceID, RunID: acknowledgement.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{DeliveryAcknowledgement: acknowledgement},
	})
	return err
}

func runtimeResultMaterialFromAggregate(aggregate runtime.RuntimeAggregate) resultevents.ResultMaterial {
	aggregate.Facts = aggregate.Facts.Normalized()
	accepted := terminalOutcomeForAggregate(aggregate).Kind == runtimecontract.TerminalOutcomeKindCompleted
	terminalSummary := ""
	if aggregate.Termination != nil {
		terminalSummary = aggregate.Termination.Reason
	} else {
		terminalSummary = aggregate.Decision.Terminal.Summary
	}
	assessment := workorchresultcontract.DoDAssessment{Satisfied: accepted, Summary: terminalSummary}
	evidenceRefs := runtimeTerminalEvidenceRefs(aggregate.Facts)
	artifactRefs := runtimeTerminalArtifactRefs(aggregate.Facts)
	goalCoverage := workorchresultcontract.DoDCoverage{Satisfied: true}
	acceptanceDimensions := workorchresultcontract.DoDCoverage{Satisfied: true}
	deliverableCoverage := workorchresultcontract.DoDCoverage{Satisfied: true}
	for _, fact := range aggregate.Facts.Assessments {
		switch fact.Scope {
		case "goalCoverage":
			goalCoverage.Items = append(goalCoverage.Items, runtimeCoverageItem(fact))
			goalCoverage.Satisfied = goalCoverage.Satisfied && fact.Disposition == runtimecontract.AssessmentDispositionSatisfied
		case "acceptanceDimension":
			acceptanceDimensions.Items = append(acceptanceDimensions.Items, runtimeCoverageItem(fact))
			acceptanceDimensions.Satisfied = acceptanceDimensions.Satisfied && fact.Disposition == runtimecontract.AssessmentDispositionSatisfied
		case "deliverable":
			deliverableCoverage.Items = append(deliverableCoverage.Items, runtimeCoverageItem(fact))
			deliverableCoverage.Satisfied = deliverableCoverage.Satisfied && fact.Disposition == runtimecontract.AssessmentDispositionSatisfied
		default:
			assessment.Items = append(assessment.Items, workorchresultcontract.DoDAssessmentItem{
				Item: fact.RequirementText, State: runtimeAssessmentState(fact.Disposition == runtimecontract.AssessmentDispositionSatisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
				FailureReason: runtimeAssessmentFailureReason(fact),
			})
		}
	}
	if len(goalCoverage.Items) > 0 {
		assessment.GoalCoverage = &goalCoverage
	}
	if len(acceptanceDimensions.Items) > 0 {
		assessment.AcceptanceDimensions = &acceptanceDimensions
	}
	if len(deliverableCoverage.Items) > 0 {
		assessment.DeliverableCoverage = &deliverableCoverage
	}
	item := resultevents.ResultMaterial{
		ID: common.ResultID(string(aggregate.Facts.WorkspaceID) + "/" + string(aggregate.Facts.RunID) + "/result/dod-assessment"), WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		GoalID: aggregate.Definition.GoalID, DoDID: aggregate.Definition.DoDID,
		Title: "Runtime acceptance result", Summary: terminalSummary,
		SourceType: resultcommon.SourceTypeDoDAssessment, SourceRef: aggregate.Decision.AuthoritySnapshot.ID,
		EvidenceRefs: evidenceRefs, ArtifactRefs: artifactRefs, DoDAssessment: assessment,
		SummaryDetail: workorchresultcontract.ResultSummary{GoalOutcome: terminalSummary, EvidenceRefs: append([]string(nil), evidenceRefs...), ArtifactRefs: append([]common.ArtifactID(nil), artifactRefs...)},
	}
	item.RuntimeCloseout = runtimeResultCloseout(aggregate, accepted, evidenceRefs)
	return item
}

func runtimeResultCloseout(aggregate runtime.RuntimeAggregate, accepted bool, evidenceRefs []string) *runtimecontract.RuntimeResultCloseout {
	closeout := runtimecontract.RuntimeResultCloseout{
		SchemaVersion:      runtimecontract.RuntimeResultCloseoutSchemaVersionCurrent,
		DefinitionRevision: aggregate.Facts.DefinitionRevision,
		EvidenceRefs:       append([]string(nil), evidenceRefs...),
		ArtifactRefs:       runtimeTerminalArtifactRefs(aggregate.Facts),
		OperationOutcomes:  runtimeOperationCloseouts(aggregate),
		RecoveryPolicy:     aggregate.Facts.RecoveryPolicy.Normalized(),
		RecoveryLedger:     aggregate.Facts.Recovery,
	}
	accumulator := newRuntimeCloseoutAccumulator(accepted)
	// The persisted coverage ledger is the sole current-schema closure authority.
	// Assessment facts remain audit evidence and gap lineage, never a replacement
	// for item state.
	accumulator.collectCoverage(aggregate.Facts.Coverage)
	// Open coverage items without satisfied assessment remain unsatisfied on reject/complete residual.
	accumulator.collectOpenCoverage(aggregate.Facts.Focus, aggregate.Facts.RemainingFocuses)
	accumulator.collectAssessments(aggregate.Facts.Assessments, accepted)
	if !accepted {
		accumulator.collectActiveGap(aggregate.Facts.Repair.ActiveGap)
	}
	accumulator.apply(&closeout)
	if !accepted {
		accumulator.applyTerminalFailure(&closeout, aggregate)
	}
	value := closeout.Normalized()
	return &value
}

type runtimeCloseoutAccumulator struct {
	satisfied   map[common.CoverageWorkItemID]struct{}
	unsatisfied map[common.CoverageWorkItemID]struct{}
	missing     map[string]struct{}
	targets     map[string]struct{}
	gaps        []runtimecontract.RuntimeAssessmentGap
	disposition runtimecontract.AssessmentDisposition
}

func newRuntimeCloseoutAccumulator(accepted bool) *runtimeCloseoutAccumulator {
	disposition := runtimecontract.AssessmentDispositionSatisfied
	if !accepted {
		disposition = runtimecontract.AssessmentDispositionTerminalRejection
	}
	return &runtimeCloseoutAccumulator{
		satisfied: map[common.CoverageWorkItemID]struct{}{}, unsatisfied: map[common.CoverageWorkItemID]struct{}{},
		missing: map[string]struct{}{}, targets: map[string]struct{}{}, disposition: disposition,
	}
}

func (a *runtimeCloseoutAccumulator) collectCoverage(items []runtimecontract.RuntimeCoverageItemFact) {
	for _, item := range items {
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			a.satisfied[item.ID] = struct{}{}
		} else {
			a.unsatisfied[item.ID] = struct{}{}
		}
	}
}

func (a *runtimeCloseoutAccumulator) collectAssessments(facts []runtimecontract.RuntimeAssessmentFact, accepted bool) {
	for _, fact := range facts {
		current := a.coverageCurrentlyUnsatisfied(fact.CoverageWorkItemIDs)
		if !current && fact.Gap != nil {
			current = a.coverageCurrentlyUnsatisfied(fact.Gap.CoverageWorkItemIDs)
		}
		if !current {
			continue
		}
		a.collectAssessmentDisposition(fact.Disposition, accepted)
		if fact.Gap != nil && runtimeAssessmentDispositionCarriesGap(fact.Disposition) {
			if gap, current := a.currentGap(fact.Gap.Normalized()); current {
				a.collectAssessmentGap(gap)
			}
		}
	}
}

func (a *runtimeCloseoutAccumulator) collectAssessmentDisposition(disposition runtimecontract.AssessmentDisposition, accepted bool) {
	if accepted {
		return
	}
	switch disposition {
	case runtimecontract.AssessmentDispositionRepairableGap,
		runtimecontract.AssessmentDispositionGovernanceRequired,
		runtimecontract.AssessmentDispositionTerminalRejection:
		a.disposition = disposition
	}
}

func (a *runtimeCloseoutAccumulator) collectAssessmentGap(gap runtimecontract.RuntimeAssessmentGap) {
	a.gaps = append(a.gaps, gap)
	for _, kind := range gap.MissingEvidenceKinds {
		a.missing[kind] = struct{}{}
	}
	for _, ref := range gap.TargetRefs {
		a.targets[ref] = struct{}{}
	}
}

func (a *runtimeCloseoutAccumulator) collectActiveGap(gap *runtimecontract.RuntimeAssessmentGap) {
	if gap == nil || len(a.gaps) != 0 {
		return
	}
	if current, ok := a.currentGap(gap.Normalized()); ok {
		a.collectAssessmentGap(current)
	}
}

func (a *runtimeCloseoutAccumulator) currentGap(gap runtimecontract.RuntimeAssessmentGap) (runtimecontract.RuntimeAssessmentGap, bool) {
	ids := make([]common.CoverageWorkItemID, 0, len(gap.CoverageWorkItemIDs))
	for _, id := range gap.CoverageWorkItemIDs {
		if a.coverageItemCurrentlyUnsatisfied(id) {
			ids = append(ids, id)
		}
	}
	gap.CoverageWorkItemIDs = ids
	return gap.Normalized(), len(ids) != 0
}

func (a *runtimeCloseoutAccumulator) coverageCurrentlyUnsatisfied(ids []common.CoverageWorkItemID) bool {
	for _, id := range ids {
		if a.coverageItemCurrentlyUnsatisfied(id) {
			return true
		}
	}
	return false
}

func (a *runtimeCloseoutAccumulator) coverageItemCurrentlyUnsatisfied(id common.CoverageWorkItemID) bool {
	if _, satisfied := a.satisfied[id]; satisfied {
		return false
	}
	_, unsatisfied := a.unsatisfied[id]
	return unsatisfied
}

func runtimeAssessmentDispositionCarriesGap(disposition runtimecontract.AssessmentDisposition) bool {
	switch disposition.Normalized() {
	case runtimecontract.AssessmentDispositionRepairableGap,
		runtimecontract.AssessmentDispositionGovernanceRequired,
		runtimecontract.AssessmentDispositionTerminalRejection:
		return true
	default:
		return false
	}
}

func (a *runtimeCloseoutAccumulator) collectOpenCoverage(focus runtimecontract.CoverageFocus, remaining []runtimecontract.CoverageFocus) {
	a.collectOpenCoverageIDs(focus.CoverageWorkItemIDs)
	for _, item := range remaining {
		a.collectOpenCoverageIDs(item.CoverageWorkItemIDs)
	}
}

func (a *runtimeCloseoutAccumulator) collectOpenCoverageIDs(ids []common.CoverageWorkItemID) {
	for _, id := range ids {
		if _, ok := a.satisfied[id]; !ok {
			a.unsatisfied[id] = struct{}{}
		}
	}
}

func (a *runtimeCloseoutAccumulator) apply(closeout *runtimecontract.RuntimeResultCloseout) {
	for id := range a.satisfied {
		delete(a.unsatisfied, id)
	}
	closeout.SatisfiedCoverageWorkItemIDs = coverageIDsFromSet(a.satisfied)
	closeout.UnsatisfiedCoverageWorkItemIDs = coverageIDsFromSet(a.unsatisfied)
	closeout.Gaps = a.gaps
	closeout.MissingEvidenceKinds = stringsFromSet(a.missing)
	closeout.TargetRefs = stringsFromSet(a.targets)
	closeout.Disposition = a.disposition
}

func (a *runtimeCloseoutAccumulator) applyTerminalFailure(closeout *runtimecontract.RuntimeResultCloseout, aggregate runtime.RuntimeAggregate) {
	if aggregate.Facts.Failure != nil {
		a.applyFailureFact(closeout, aggregate.Facts.Failure)
	} else if aggregate.Facts.Repair.ActiveGap != nil {
		if gap, current := a.currentGap(aggregate.Facts.Repair.ActiveGap.Normalized()); current {
			a.applyGapFailure(closeout, gap)
		}
	} else if len(closeout.Gaps) != 0 {
		a.applyGapFailure(closeout, closeout.Gaps[0])
	}
	if closeout.FailureCode == "" && aggregate.Decision.Terminal != nil {
		closeout.FailureCode = strings.TrimSpace(aggregate.Decision.Terminal.Code)
	}
	if closeout.FailureSignature == "" && aggregate.Facts.Repair.LastCycleID != "" {
		closeout.FailureSignature = aggregate.Facts.Repair.LastCycleID
	}
}

func (a *runtimeCloseoutAccumulator) applyFailureFact(closeout *runtimecontract.RuntimeResultCloseout, failure *runtimecontract.FailureFact) {
	if failure.Class != "" {
		closeout.FailureClass = failure.Class
	}
	if strings.TrimSpace(failure.Code) != "" {
		closeout.FailureCode = strings.TrimSpace(failure.Code)
	}
	if strings.TrimSpace(failure.Signature) != "" {
		closeout.FailureSignature = strings.TrimSpace(failure.Signature)
	}
	for _, ref := range failure.TargetRefs {
		a.targets[ref] = struct{}{}
	}
	closeout.TargetRefs = stringsFromSet(a.targets)
}

func (a *runtimeCloseoutAccumulator) applyGapFailure(closeout *runtimecontract.RuntimeResultCloseout, gap runtimecontract.RuntimeAssessmentGap) {
	gap = gap.Normalized()
	if closeout.FailureClass == "" {
		closeout.FailureClass = gap.FailureClass
	}
	if closeout.FailureCode == "" {
		closeout.FailureCode = gap.FailureCode
	}
	if closeout.FailureSignature == "" {
		closeout.FailureSignature = gap.FailureSignature
	}
}

func runtimeOperationCloseouts(aggregate runtime.RuntimeAggregate) []runtimecontract.RuntimeOperationCloseout {
	dispatches := append([]runtime.SealedDispatch(nil), aggregate.CompletedDispatches...)
	if aggregate.ActiveDispatch != nil {
		dispatches = append(dispatches, *aggregate.ActiveDispatch)
	}
	ret := make([]runtimecontract.RuntimeOperationCloseout, 0)
	for _, dispatch := range dispatches {
		for _, execution := range dispatch.OperationExecutions {
			execution = execution.Normalized()
			outcome := runtimecontract.RuntimeOperationCloseout{
				DispatchID: dispatch.ID, OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey,
				Fence:               execution.Fence,
				ExpectedEffectScope: execution.ExpectedEffectScope, ExecutionState: execution.State,
				ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: execution.OwnerReceiptHash,
			}
			if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence); ok {
				outcome.ObservedEffectScope = receipt.ObservedEffectScope
				outcome.Outcome = receipt.Outcome
			}
			ret = append(ret, outcome.Normalized())
		}
	}
	return ret
}

func runtimeTerminalEvidenceRefs(facts runtimecontract.RuntimeFacts) []string {
	facts = facts.Normalized()
	values := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values[value] = struct{}{}
		}
	}
	for _, ledger := range [][]runtimecontract.EvidenceFact{facts.Evidence.Accepted, facts.Evidence.Rejected} {
		for _, evidence := range ledger {
			add(string(evidence.ID))
			add(evidence.SourceRef)
			for _, ref := range evidence.EvidenceRefs {
				add(ref)
			}
		}
	}
	for _, assessment := range facts.Assessments {
		for _, ref := range assessment.EvidenceRefs {
			add(ref)
		}
		for _, item := range assessment.Items {
			for _, ref := range item.EvidenceRefs {
				add(ref)
			}
		}
		if assessment.Gap != nil {
			for _, ref := range assessment.Gap.EvidenceRefs {
				add(ref)
			}
		}
	}
	for _, item := range facts.Coverage {
		for _, ref := range item.EvidenceRefs {
			add(ref)
		}
	}
	return stringsFromSet(values)
}

func runtimeTerminalArtifactRefs(facts runtimecontract.RuntimeFacts) []common.ArtifactID {
	values := map[common.ArtifactID]struct{}{}
	for _, assessment := range facts.Normalized().Assessments {
		for _, ref := range assessment.ArtifactRefs {
			id := common.ArtifactID(strings.TrimSpace(ref))
			if id != "" {
				values[id] = struct{}{}
			}
		}
		for _, item := range assessment.Items {
			for _, ref := range item.ArtifactRefs {
				id := common.ArtifactID(strings.TrimSpace(string(ref)))
				if id != "" {
					values[id] = struct{}{}
				}
			}
		}
	}
	ret := make([]common.ArtifactID, 0, len(values))
	for id := range values {
		ret = append(ret, id)
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i]) < string(ret[j]) })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func coverageIDsFromSet(values map[common.CoverageWorkItemID]struct{}) []common.CoverageWorkItemID {
	if len(values) == 0 {
		return nil
	}
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for id := range values {
		ret = append(ret, id)
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i]) < string(ret[j]) })
	return ret
}

func stringsFromSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for value := range values {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func runtimeCoverageItem(fact runtimecontract.RuntimeAssessmentFact) workorchresultcontract.DoDCoverageItem {
	return workorchresultcontract.DoDCoverageItem{
		ID: fact.RequirementID, Item: fact.RequirementText, Requirement: fact.RequirementText,
		State: runtimeAssessmentState(fact.Disposition == runtimecontract.AssessmentDispositionSatisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
	}
}

func runtimeAssessmentState(satisfied bool) string {
	if satisfied {
		return "satisfied"
	}
	return "failed"
}

func runtimeAssessmentFailureReason(fact runtimecontract.RuntimeAssessmentFact) string {
	if fact.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		return ""
	}
	return fact.Summary
}
