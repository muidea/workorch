package orchestrator

import (
	"context"
	"errors"
	"time"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Module) projectRuntimeV2Result(ctx context.Context, aggregate runtimev2.RuntimeAggregate) error {
	if m == nil || m.rawEventHub == nil || aggregate.Decision.Terminal == nil {
		return common.ErrInvalidArgument
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	repository := resultEventResultRepository{hub: m.rawEventHub}
	existing, err := repository.Get(ctx, item.ID)
	if err == nil {
		if existing.State == item.State && existing.WorkspaceID == item.WorkspaceID && existing.RunID == item.RunID {
			return nil
		}
		return common.ErrConflict
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return repository.Create(ctx, item)
}

func runtimeV2ResultFromAggregate(aggregate runtimev2.RuntimeAggregate) *resultmodel.Result {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	accepted := aggregate.Decision.Action == runtimecontract.RuntimeActionComplete
	state := common.ResultStateRejected
	if accepted {
		state = common.ResultStateAccepted
	}
	assessment := resultmodel.DoDAssessment{Satisfied: accepted, Summary: aggregate.Decision.Terminal.Summary}
	evidenceRefs := []string{}
	goalCoverage := resultmodel.DoDCoverage{Satisfied: true}
	acceptanceDimensions := resultmodel.DoDCoverage{Satisfied: true}
	deliverableCoverage := resultmodel.DoDCoverage{Satisfied: true}
	for _, fact := range aggregate.Facts.Assessments {
		evidenceRefs = append(evidenceRefs, fact.EvidenceRefs...)
		switch fact.Scope {
		case "goalCoverage":
			goalCoverage.Items = append(goalCoverage.Items, runtimeV2CoverageItem(fact))
			goalCoverage.Satisfied = goalCoverage.Satisfied && fact.Satisfied
		case "acceptanceDimension":
			acceptanceDimensions.Items = append(acceptanceDimensions.Items, runtimeV2CoverageItem(fact))
			acceptanceDimensions.Satisfied = acceptanceDimensions.Satisfied && fact.Satisfied
		case "deliverable":
			deliverableCoverage.Items = append(deliverableCoverage.Items, runtimeV2CoverageItem(fact))
			deliverableCoverage.Satisfied = deliverableCoverage.Satisfied && fact.Satisfied
		default:
			assessment.Items = append(assessment.Items, resultmodel.DoDAssessmentItem{
				Item: fact.RequirementText, State: runtimeV2AssessmentState(fact.Satisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
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
	item := &resultmodel.Result{
		ID: common.ResultID(string(aggregate.Facts.WorkspaceID) + "/" + string(aggregate.Facts.RunID) + "/result/dod-assessment"), WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		GoalID: aggregate.Definition.GoalID, DoDID: aggregate.Definition.DoDID, State: state,
		Title: "Runtime V2 acceptance result", Summary: aggregate.Decision.Terminal.Summary,
		SourceType: resultcommon.SourceTypeDoDAssessment, SourceRef: aggregate.Decision.AuthoritySnapshot.ID,
		EvidenceRefs: evidenceRefs, DoDAssessment: assessment,
		SummaryDetail: resultmodel.ResultSummary{GoalOutcome: aggregate.Decision.Terminal.Summary, EvidenceRefs: append([]string(nil), evidenceRefs...)},
		CreatedAt:     now, UpdatedAt: now,
	}
	if accepted {
		item.AcceptedAt = now
		item.AcceptedBy = "kernel/orchestrator"
	} else {
		item.RejectedAt = now
	}
	return item
}

func runtimeV2CoverageItem(fact runtimecontract.RuntimeAssessmentFact) resultmodel.DoDCoverageItem {
	return resultmodel.DoDCoverageItem{
		ID: fact.RequirementID, Item: fact.RequirementText, Requirement: fact.RequirementText,
		State: runtimeV2AssessmentState(fact.Satisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
	}
}

func runtimeV2AssessmentState(satisfied bool) string {
	if satisfied {
		return "satisfied"
	}
	return "failed"
}

func runtimeV2AssessmentFailureReason(fact runtimecontract.RuntimeAssessmentFact) string {
	if fact.Satisfied {
		return ""
	}
	return fact.Summary
}
