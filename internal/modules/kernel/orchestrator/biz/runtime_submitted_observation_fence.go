package orchestrator

import (
	"context"
	"fmt"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This is an ordering adapter over the existing private provider-terminal
// transaction, not a new settlement authority. Complete documents retain their
// original admission lane. A stale historical identity cannot be rebound to a
// current decision merely to make progress.
func (m *Biz) settleRuntimeSubmittedProviderBeforeObservation(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, bool, error) {
	var submitted *runtimecontract.RuntimeCandidateRequestSession
	for index := range aggregate.CandidateRequestSessions {
		session := &aggregate.CandidateRequestSessions[index]
		if !session.ProviderSubmitted || session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
			continue
		}
		if submitted != nil || session.Transaction != aggregate.Decision.AuthorityTransaction() {
			return runtime.RuntimeAggregate{}, false, fmt.Errorf("%w: recovered provider responsibility lacks unique current authority", common.ErrConflict)
		}
		submitted = session
	}
	if submitted == nil || aggregate.ActiveDispatch != nil {
		return aggregate, false, nil
	}
	settlement, err := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), submitted.LogicalRequestID, submitted.LastPhysicalCallID, nil, nil)
	if err != nil {
		return runtime.RuntimeAggregate{}, false, err
	}
	code, signature := "provider_submission_unknown", "provider-submission-unknown"
	if submitted.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		code, signature = "semantic_provider_submission_unknown", "semantic-provider-submission-unknown"
	}
	updated, err := m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, submitted.LogicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
		DocumentKind: submitted.DocumentKind, OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
		TerminalCode: code, TerminalSignature: signature, Message: "provider request was submitted without a durable outcome",
	}, settlement)
	return updated, err == nil, err
}
