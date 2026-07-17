package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// deliverRuntimeV2Terminal is the single SM-4 terminal delivery lane. The
// persisted aggregate remains authority; Result and Run/Workspace are
// idempotent derived deliveries and may be retried in this order.
func (m *Module) deliverRuntimeV2Terminal(ctx context.Context, aggregate runtimev2.RuntimeAggregate) error {
	if m == nil || m.bizPtr == nil || aggregate.Decision.Terminal == nil ||
		(aggregate.Decision.Action != runtimecontract.RuntimeActionComplete && aggregate.Decision.Action != runtimecontract.RuntimeActionFail) {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return fmt.Errorf("persisted Runtime V2 terminal aggregate is invalid: %w", err)
	}
	if err := m.projectRuntimeV2Result(ctx, aggregate); err != nil {
		return err
	}
	return m.bizPtr.ProjectRuntimeV2Terminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.Action, aggregate.Decision.Terminal)
}

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
	aggregate.Facts = aggregate.Facts.Normalized()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	accepted := aggregate.Decision.Action == runtimecontract.RuntimeActionComplete
	state := common.ResultStateRejected
	if accepted {
		state = common.ResultStateAccepted
	}
	assessment := resultmodel.DoDAssessment{Satisfied: accepted, Summary: aggregate.Decision.Terminal.Summary}
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
	item := &resultmodel.Result{
		ID: common.ResultID(string(aggregate.Facts.WorkspaceID) + "/" + string(aggregate.Facts.RunID) + "/result/dod-assessment"), WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		GoalID: aggregate.Definition.GoalID, DoDID: aggregate.Definition.DoDID, State: state,
		Title: "Runtime V2 acceptance result", Summary: aggregate.Decision.Terminal.Summary,
		SourceType: resultcommon.SourceTypeDoDAssessment, SourceRef: aggregate.Decision.AuthoritySnapshot.ID,
		EvidenceRefs: evidenceRefs, ArtifactRefs: artifactRefs, DoDAssessment: assessment,
		SummaryDetail: resultmodel.ResultSummary{GoalOutcome: aggregate.Decision.Terminal.Summary, EvidenceRefs: append([]string(nil), evidenceRefs...), ArtifactRefs: append([]common.ArtifactID(nil), artifactRefs...)},
		CreatedAt:     now, UpdatedAt: now,
	}
	if accepted {
		item.AcceptedAt = now
		item.AcceptedBy = "kernel/orchestrator"
	} else {
		item.RejectedAt = now
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
