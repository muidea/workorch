package orchestrator

// Runtime V2 terminal projection is owned by Biz.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// deliverRuntimeV2Terminal is the single SM-4 terminal delivery lane. The
// persisted aggregate remains authority. Result, Workspace and Run receive
// typed terminal facts and only mutate their own state, in that order.
func (m *Biz) deliverRuntimeV2Terminal(ctx context.Context, aggregate runtimev2.RuntimeAggregate) error {
	if m == nil || m.bizPtr == nil || (aggregate.Decision.Terminal == nil && aggregate.Termination == nil) {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return fmt.Errorf("persisted Runtime V2 terminal aggregate is invalid: %w", err)
	}
	request := terminalDeliveryRequest(aggregate)
	if err := request.Validate(); err != nil {
		return err
	}
	if !deliveryAcknowledged(aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerResult) {
		if err := m.deliverRuntimeV2Result(ctx, request, runtimeV2ResultMaterialFromAggregate(aggregate)); err != nil {
			return err
		}
	}
	if !m.persistedDeliveryAcknowledged(ctx, aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace) {
		if err := m.deliverRuntimeV2Workspace(ctx, request); err != nil {
			return err
		}
	}
	if !m.persistedDeliveryAcknowledged(ctx, aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerRun) {
		return m.deliverRuntimeV2Run(ctx, request)
	}
	return nil
}

func deliveryAcknowledged(aggregate runtimev2.RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	for _, acknowledgement := range aggregate.DeliveryAcknowledgements {
		if acknowledgement.TerminalDeliveryID == deliveryID && acknowledgement.Owner == owner {
			return true
		}
	}
	return false
}

func (m *Biz) persistedDeliveryAcknowledged(ctx context.Context, aggregate runtimev2.RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	if m == nil || m.bizPtr == nil || !m.bizPtr.RuntimeV2RepositoryConfigured() {
		return deliveryAcknowledged(aggregate, deliveryID, owner)
	}
	loaded, err := m.bizPtr.LoadRuntimeV2(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	return err == nil && deliveryAcknowledged(loaded, deliveryID, owner)
}

func terminalDeliveryRequest(aggregate runtimev2.RuntimeAggregate) orchestratorevents.RuntimeTerminalDeliveryRequestData {
	if aggregate.Termination != nil {
		kind := aggregate.Termination.Kind
		code := "administrative_" + string(kind)
		return orchestratorevents.RuntimeTerminalDeliveryRequestData{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, SourceOwner: ModuleID,
			CausationID:         "terminal-delivery/" + runtimecontract.StableTerminalDeliveryID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, string(runtimecontract.RuntimeActionFail), code),
			AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision, DispatchID: aggregate.Decision.AuthoritySnapshot.ID,
			Action: runtimecontract.RuntimeActionFail, TerminalCode: code, TerminalSummary: aggregate.Termination.Reason, AdministrativeKind: kind,
			TerminalDeliveryID: runtimecontract.StableTerminalDeliveryID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, string(runtimecontract.RuntimeActionFail), code), StateRevision: aggregate.StateRevision}
	}
	terminal := aggregate.Decision.Terminal
	return orchestratorevents.RuntimeTerminalDeliveryRequestData{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, SourceOwner: ModuleID,
		CausationID:         "terminal-delivery/" + runtimecontract.StableTerminalDeliveryID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, string(aggregate.Decision.Action), terminal.Code),
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision,
		EvidenceRevision: aggregate.Facts.EvidenceRevision, DispatchID: aggregate.Decision.AuthoritySnapshot.ID,
		Action: aggregate.Decision.Action, TerminalCode: terminal.Code, TerminalSummary: terminal.Summary,
		TerminalDeliveryID: runtimecontract.StableTerminalDeliveryID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, string(aggregate.Decision.Action), terminal.Code), StateRevision: aggregate.StateRevision,
	}
}

func (m *Biz) deliverRuntimeV2Result(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData, material resultevents.ResultMaterial) error {
	if m == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	command := resultevents.ApplyRuntimeTerminalDeliveryCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, SourceOwner: ModuleID, CausationID: request.CausationID, TerminalDeliveryID: request.TerminalDeliveryID, AuthoritySnapshotID: request.AuthoritySnapshotID, DefinitionRevision: request.DefinitionRevision, EvidenceRevision: request.EvidenceRevision, DispatchID: request.DispatchID, Action: request.Action, TerminalCode: request.TerminalCode, TerminalSummary: request.TerminalSummary, Material: material}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandApplyTerminalDelivery, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, resultevents.TopicResultCommandApplyTerminalDelivery, "result", string(material.ID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	if response == nil || response.Error() != nil {
		if response != nil {
			return response.Error()
		}
		return common.ErrInvalidArgument
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	ownerResult, ok := data.(resultevents.ResultLifecycleCommandResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) deliverRuntimeV2Workspace(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData) error {
	command := workspaceevents.ApplyRuntimeTerminalDeliveryCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, SourceOwner: ModuleID, CausationID: request.CausationID, TerminalDeliveryID: request.TerminalDeliveryID, AuthoritySnapshotID: request.AuthoritySnapshotID, DefinitionRevision: request.DefinitionRevision, EvidenceRevision: request.EvidenceRevision, DispatchID: request.DispatchID, Action: request.Action, TerminalCode: request.TerminalCode, TerminalSummary: request.TerminalSummary}
	command.AdministrativeKind = request.AdministrativeKind
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, "workspace", string(request.WorkspaceID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	if response == nil || response.Error() != nil {
		if response != nil {
			return response.Error()
		}
		return common.ErrInvalidArgument
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	ownerResult, ok := data.(workspaceevents.WorkspaceLifecycleCommandResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) deliverRuntimeV2Run(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData) error {
	command := runevents.ApplyRuntimeTerminalDeliveryCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, SourceOwner: ModuleID, CausationID: request.CausationID, TerminalDeliveryID: request.TerminalDeliveryID, AuthoritySnapshotID: request.AuthoritySnapshotID, DefinitionRevision: request.DefinitionRevision, EvidenceRevision: request.EvidenceRevision, DispatchID: request.DispatchID, Action: request.Action, TerminalCode: request.TerminalCode, TerminalSummary: request.TerminalSummary}
	command.AdministrativeKind = request.AdministrativeKind
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandApplyTerminalDelivery, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.TopicRunCommandApplyTerminalDelivery, "run", string(request.RunID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	ownerResult, err := runLifecycleCommandResponse(response, runevents.TopicRunCommandApplyTerminalDelivery)
	if err != nil {
		return err
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) recordRuntimeDeliveryAcknowledgement(ctx context.Context, acknowledgement *runtimecontract.RuntimeDeliveryAcknowledgement) error {
	if acknowledgement == nil {
		return common.ErrInvalidArgument
	}
	// Focused owner-boundary tests may construct a delivery Biz without the
	// Runtime V2 repository; production Configure always installs it. The
	// resource-owner transition remains fully exercised in that narrow fixture.
	if m.bizPtr == nil || !m.bizPtr.RuntimeV2RepositoryConfigured() {
		return nil
	}
	_, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventTerminalDeliveryAcknowledged, WorkspaceID: acknowledgement.WorkspaceID, RunID: acknowledgement.RunID, DeliveryAcknowledgement: acknowledgement,
	})
	return err
}

func runtimeV2ResultMaterialFromAggregate(aggregate runtimev2.RuntimeAggregate) resultevents.ResultMaterial {
	aggregate.Facts = aggregate.Facts.Normalized()
	accepted := aggregate.Termination == nil && aggregate.Decision.Action == runtimecontract.RuntimeActionComplete
	terminalSummary := ""
	if aggregate.Termination != nil {
		terminalSummary = aggregate.Termination.Reason
	} else {
		terminalSummary = aggregate.Decision.Terminal.Summary
	}
	assessment := resultmodel.DoDAssessment{Satisfied: accepted, Summary: terminalSummary}
	evidenceRefs := runtimeV2TerminalEvidenceRefs(aggregate.Facts)
	artifactRefs := runtimeV2TerminalArtifactRefs(aggregate.Facts)
	goalCoverage := resultmodel.DoDCoverage{Satisfied: true}
	acceptanceDimensions := resultmodel.DoDCoverage{Satisfied: true}
	deliverableCoverage := resultmodel.DoDCoverage{Satisfied: true}
	for _, fact := range aggregate.Facts.Assessments {
		switch fact.Scope {
		case "goalCoverage":
			goalCoverage.Items = append(goalCoverage.Items, runtimeV2CoverageItem(fact))
			goalCoverage.Satisfied = goalCoverage.Satisfied && fact.Disposition == runtimecontract.AssessmentDispositionSatisfied
		case "acceptanceDimension":
			acceptanceDimensions.Items = append(acceptanceDimensions.Items, runtimeV2CoverageItem(fact))
			acceptanceDimensions.Satisfied = acceptanceDimensions.Satisfied && fact.Disposition == runtimecontract.AssessmentDispositionSatisfied
		case "deliverable":
			deliverableCoverage.Items = append(deliverableCoverage.Items, runtimeV2CoverageItem(fact))
			deliverableCoverage.Satisfied = deliverableCoverage.Satisfied && fact.Disposition == runtimecontract.AssessmentDispositionSatisfied
		default:
			assessment.Items = append(assessment.Items, resultmodel.DoDAssessmentItem{
				Item: fact.RequirementText, State: runtimeV2AssessmentState(fact.Disposition == runtimecontract.AssessmentDispositionSatisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
				FailureReason: runtimeV2AssessmentFailureReason(fact),
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
		Title: "Runtime V2 acceptance result", Summary: terminalSummary,
		SourceType: resultcommon.SourceTypeDoDAssessment, SourceRef: aggregate.Decision.AuthoritySnapshot.ID,
		EvidenceRefs: evidenceRefs, ArtifactRefs: artifactRefs, DoDAssessment: assessment,
		SummaryDetail: resultmodel.ResultSummary{GoalOutcome: terminalSummary, EvidenceRefs: append([]string(nil), evidenceRefs...), ArtifactRefs: append([]common.ArtifactID(nil), artifactRefs...)},
	}
	item.RuntimeCloseout = runtimeV2ResultCloseout(aggregate, accepted, evidenceRefs)
	return item
}

func runtimeV2ResultCloseout(aggregate runtimev2.RuntimeAggregate, accepted bool, evidenceRefs []string) *runtimecontract.RuntimeResultCloseout {
	closeout := runtimecontract.RuntimeResultCloseout{
		SchemaVersion:      runtimecontract.RuntimeResultCloseoutSchemaVersionCurrent,
		DefinitionRevision: aggregate.Facts.DefinitionRevision,
		EvidenceRefs:       append([]string(nil), evidenceRefs...),
		ArtifactRefs:       runtimeV2TerminalArtifactRefs(aggregate.Facts),
		OperationOutcomes:  runtimeV2OperationCloseouts(aggregate),
		RecoveryPolicy:     aggregate.Facts.RecoveryPolicy.Normalized(),
		RecoveryLedger:     aggregate.Facts.Recovery,
	}
	satisfied := map[common.CoverageWorkItemID]struct{}{}
	unsatisfied := map[common.CoverageWorkItemID]struct{}{}
	missing := map[string]struct{}{}
	targets := map[string]struct{}{}
	var gaps []runtimecontract.RuntimeAssessmentGap
	disposition := runtimecontract.AssessmentDispositionSatisfied
	if !accepted {
		disposition = runtimecontract.AssessmentDispositionTerminalRejection
	}
	// The persisted coverage ledger is the sole current-schema closure authority.
	// Assessment facts remain audit evidence and gap lineage, never a replacement
	// for item state.
	for _, item := range aggregate.Facts.Coverage {
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			satisfied[item.ID] = struct{}{}
		} else {
			unsatisfied[item.ID] = struct{}{}
		}
	}
	for _, fact := range aggregate.Facts.Assessments {
		switch fact.Disposition {
		case runtimecontract.AssessmentDispositionRepairableGap:
			if !accepted {
				disposition = runtimecontract.AssessmentDispositionRepairableGap
			}
		case runtimecontract.AssessmentDispositionGovernanceRequired:
			if !accepted {
				disposition = runtimecontract.AssessmentDispositionGovernanceRequired
			}
		case runtimecontract.AssessmentDispositionTerminalRejection:
			if !accepted {
				disposition = runtimecontract.AssessmentDispositionTerminalRejection
			}
		}
		if fact.Gap != nil {
			gap := fact.Gap.Normalized()
			gaps = append(gaps, gap)
			for _, kind := range gap.MissingEvidenceKinds {
				missing[kind] = struct{}{}
			}
			for _, ref := range gap.TargetRefs {
				targets[ref] = struct{}{}
			}
			if closeout.FailureClass == "" {
				closeout.FailureClass = gap.FailureClass
				closeout.FailureCode = gap.FailureCode
				closeout.FailureSignature = gap.FailureSignature
			}
		}
	}
	// Open coverage items without satisfied assessment remain unsatisfied on reject/complete residual.
	for _, id := range aggregate.Facts.Focus.CoverageWorkItemIDs {
		if _, ok := satisfied[id]; !ok {
			unsatisfied[id] = struct{}{}
		}
	}
	for _, focus := range aggregate.Facts.RemainingFocuses {
		for _, id := range focus.CoverageWorkItemIDs {
			if _, ok := satisfied[id]; !ok {
				unsatisfied[id] = struct{}{}
			}
		}
	}
	// satisfied wins over unsatisfied when both appear
	for id := range satisfied {
		delete(unsatisfied, id)
	}
	closeout.SatisfiedCoverageWorkItemIDs = coverageIDsFromSet(satisfied)
	closeout.UnsatisfiedCoverageWorkItemIDs = coverageIDsFromSet(unsatisfied)
	closeout.Gaps = gaps
	closeout.MissingEvidenceKinds = stringsFromSet(missing)
	closeout.TargetRefs = stringsFromSet(targets)
	closeout.Disposition = disposition
	if !accepted {
		if aggregate.Facts.Failure != nil {
			if closeout.FailureClass == "" {
				closeout.FailureClass = aggregate.Facts.Failure.Class
			}
			if closeout.FailureCode == "" {
				closeout.FailureCode = strings.TrimSpace(aggregate.Facts.Failure.Code)
			}
			if closeout.FailureSignature == "" {
				closeout.FailureSignature = strings.TrimSpace(aggregate.Facts.Failure.Signature)
			}
			for _, ref := range aggregate.Facts.Failure.TargetRefs {
				targets[ref] = struct{}{}
			}
			closeout.TargetRefs = stringsFromSet(targets)
		}
		if closeout.FailureCode == "" && aggregate.Decision.Terminal != nil {
			closeout.FailureCode = strings.TrimSpace(aggregate.Decision.Terminal.Code)
		}
		if closeout.FailureSignature == "" && aggregate.Facts.Repair.LastCycleID != "" {
			closeout.FailureSignature = aggregate.Facts.Repair.LastCycleID
		}
		if aggregate.Facts.Repair.ActiveGap != nil && len(closeout.Gaps) == 0 {
			closeout.Gaps = []runtimecontract.RuntimeAssessmentGap{aggregate.Facts.Repair.ActiveGap.Normalized()}
		}
	}
	value := closeout.Normalized()
	return &value
}

func runtimeV2OperationCloseouts(aggregate runtimev2.RuntimeAggregate) []runtimecontract.RuntimeOperationCloseout {
	dispatches := append([]runtimev2.SealedDispatch(nil), aggregate.CompletedDispatches...)
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

func runtimeV2TerminalEvidenceRefs(facts runtimecontract.RuntimeFacts) []string {
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

func runtimeV2TerminalArtifactRefs(facts runtimecontract.RuntimeFacts) []common.ArtifactID {
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

func runtimeV2CoverageItem(fact runtimecontract.RuntimeAssessmentFact) resultmodel.DoDCoverageItem {
	return resultmodel.DoDCoverageItem{
		ID: fact.RequirementID, Item: fact.RequirementText, Requirement: fact.RequirementText,
		State: runtimeV2AssessmentState(fact.Disposition == runtimecontract.AssessmentDispositionSatisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
	}
}

func runtimeV2AssessmentState(satisfied bool) string {
	if satisfied {
		return "satisfied"
	}
	return "failed"
}

func runtimeV2AssessmentFailureReason(fact runtimecontract.RuntimeAssessmentFact) string {
	if fact.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		return ""
	}
	return fact.Summary
}
