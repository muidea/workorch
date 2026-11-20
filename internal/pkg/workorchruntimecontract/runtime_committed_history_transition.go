package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
)

// A formal preimage is supplied only by the owner transaction. Preserve its
// original results before consuming them; a proposal with a matching self hash
// is never a committed result. Storage packing may change, append order may not.
func ValidateRuntimeCommittedHistoryTransition(before, after RuntimeStateRecord) (err error) {
	return ValidateRuntimeCommittedHistoryTransitionWithDiff(before, after, ComputeRuntimeStateTransitionDiff(before, after))
}

func ValidateRuntimeCommittedHistoryTransitionWithDiff(before, after RuntimeStateRecord, diff RuntimeStateTransitionDiff) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %v", common.ErrConflict, err)
		}
	}()
	if !diff.PreviousAvailable || diff.Definition || diff.ResultFeedbackVersion || diff.FactsScope || diff.FactsDefinitionSeed || diff.FactsDefinitionFocuses {
		if !runtimeRawEqual(before.Definition, after.Definition) ||
			before.ResultFeedbackVersion != after.ResultFeedbackVersion ||
			before.Facts.WorkspaceID != after.Facts.WorkspaceID || before.Facts.RunID != after.Facts.RunID ||
			!runtimeRawEqual(before.Facts.DefinitionSeed, after.Facts.DefinitionSeed) ||
			!runtimeRawEqual(before.Facts.DefinitionFocuses, after.Facts.DefinitionFocuses) {
			return fmt.Errorf("runtime transition rewrites sealed definition or scope")
		}
	}
	if !diff.PreviousAvailable || diff.SemanticPlanning {
		if err := validateCommittedSemanticSources(before.SemanticPlanning, after.SemanticPlanning); err != nil {
			return err
		}
		historyUnchangedPrefix := runtimeRawEqual(before.SemanticPlanning.HistoryArchive, after.SemanticPlanning.HistoryArchive) &&
			len(after.SemanticPlanning.History) >= len(before.SemanticPlanning.History) &&
			(len(before.SemanticPlanning.History) == 0 || runtimeRawEqual(before.SemanticPlanning.History, after.SemanticPlanning.History[:len(before.SemanticPlanning.History)]))
		if !historyUnchangedPrefix {
			prior := make([]RuntimeSemanticPlanningGeneration, 0, before.SemanticPlanning.HistoryCount())
			for _, generation := range before.SemanticPlanning.AllHistory() {
				prior = append(prior, generation)
			}
			if after.SemanticPlanning.HistoryCount() < len(prior) {
				return fmt.Errorf("runtime transition removes committed semantic history")
			}
			for i, generation := range after.SemanticPlanning.AllHistory() {
				if i >= len(prior) {
					break
				}
				if !runtimeRawEqual(prior[i], generation) {
					return fmt.Errorf("runtime transition rewrites committed semantic generation %d", i)
				}
			}
		}
	}
	if !diff.PreviousAvailable || diff.ImplementationWork {
		if len(after.ImplementationWork.PurposeChoices) < len(before.ImplementationWork.PurposeChoices) {
			return fmt.Errorf("runtime transition removes committed work purpose")
		}
		for i, choice := range before.ImplementationWork.PurposeChoices {
			if !runtimeRawEqual(choice, after.ImplementationWork.PurposeChoices[i]) {
				return fmt.Errorf("runtime transition rewrites committed work purpose")
			}
		}
		if len(after.ImplementationWork.Rounds) < len(before.ImplementationWork.Rounds) {
			return fmt.Errorf("runtime transition removes work round")
		}
		for i, round := range before.ImplementationWork.Rounds {
			// Suspended, unknown and open rounds still have original transactions to
			// settle. Only successful collective completion is a closed work result.
			if round.CompletedReceiptSetHash != "" && !runtimeRawEqual(round, after.ImplementationWork.Rounds[i]) {
				return fmt.Errorf("runtime transition rewrites completed work round")
			}
		}
	}
	for _, entry := range []struct {
		changed bool
		before  []EvidenceFact
		after   []EvidenceFact
	}{
		{!diff.PreviousAvailable || diff.FactsEvidenceAccepted, before.Facts.Evidence.Accepted, after.Facts.Evidence.Accepted},
		{!diff.PreviousAvailable || diff.FactsEvidenceRejected, before.Facts.Evidence.Rejected, after.Facts.Evidence.Rejected},
	} {
		if !entry.changed {
			continue
		}
		if len(entry.after) >= len(entry.before) && (len(entry.before) == 0 || runtimeRawEqual(entry.before, entry.after[:len(entry.before)])) {
			continue
		}
		// Evidence has canonical identity order, not append position.
		for _, fact := range entry.before {
			matches := 0
			for _, next := range entry.after {
				if next.ID == fact.ID && runtimeRawEqual(fact, next) {
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("runtime transition rewrites original evidence")
			}
		}
	}
	// Accepted request/attempt sources are original authority, not replaceable
	// copies of the dispatch's current self hash. Pending requests still evolve.
	if !diff.PreviousAvailable || diff.CandidateAttempts || diff.CandidateRequestSessions {
		attemptsAndSessionsUnchangedPrefix := len(after.CandidateAttempts) >= len(before.CandidateAttempts) &&
			(len(before.CandidateAttempts) == 0 || runtimeRawEqual(before.CandidateAttempts, after.CandidateAttempts[:len(before.CandidateAttempts)])) &&
			len(after.CandidateRequestSessions) >= len(before.CandidateRequestSessions) &&
			(len(before.CandidateRequestSessions) == 0 || runtimeRawEqual(before.CandidateRequestSessions, after.CandidateRequestSessions[:len(before.CandidateRequestSessions)]))
		if !attemptsAndSessionsUnchangedPrefix {
			for _, attempt := range before.CandidateAttempts {
				if attempt.State != RuntimeCandidateAttemptStateAccepted {
					continue
				}
				matches := 0
				for _, next := range after.CandidateAttempts {
					if next.LogicalRequestID == attempt.LogicalRequestID && next.Attempt == attempt.Attempt && next.State == attempt.State && runtimeRawEqual(attempt, next) {
						matches++
					}
				}
				if matches != 1 {
					return fmt.Errorf("runtime transition rewrites accepted candidate source")
				}
				for _, session := range before.CandidateRequestSessions {
					if session.LogicalRequestID != attempt.LogicalRequestID {
						continue
					}
					matches = 0
					for _, next := range after.CandidateRequestSessions {
						if next.LogicalRequestID == session.LogicalRequestID && runtimeRawEqual(session, next) {
							matches++
						}
					}
					if matches != 1 {
						return fmt.Errorf("runtime transition rewrites accepted request source")
					}
				}
			}
		}
	}
	type receiptIdentityKey struct {
		executionKey string
		fence        uint64
	}
	if (!diff.PreviousAvailable || diff.OperationReceipts) && (len(after.OperationReceipts) < len(before.OperationReceipts) ||
		(len(before.OperationReceipts) > 0 && !runtimeRawEqual(before.OperationReceipts, after.OperationReceipts[:len(before.OperationReceipts)]))) {
		receipts := make(map[receiptIdentityKey]RuntimeOperationReceipt, len(after.OperationReceipts))
		for _, receipt := range after.OperationReceipts {
			receipts[receiptIdentityKey{executionKey: receipt.ExecutionKey, fence: receipt.Fence}] = receipt
		}
		for _, receipt := range before.OperationReceipts {
			if !runtimeRawEqual(receipt, receipts[receiptIdentityKey{executionKey: receipt.ExecutionKey, fence: receipt.Fence}]) {
				return fmt.Errorf("runtime transition rewrites committed owner receipt")
			}
		}
	}
	if (!diff.PreviousAvailable || diff.CompletedDispatches) && (len(after.CompletedDispatches) < len(before.CompletedDispatches) ||
		(len(before.CompletedDispatches) > 0 && !runtimeRawEqual(before.CompletedDispatches, after.CompletedDispatches[:len(before.CompletedDispatches)]))) {
		dispatches := make(map[string]RuntimeDispatchRecord, len(after.CompletedDispatches))
		for _, dispatch := range after.CompletedDispatches {
			dispatches[dispatch.ID] = dispatch
		}
		for _, dispatch := range before.CompletedDispatches {
			next, found := dispatches[dispatch.ID]
			if !found {
				return fmt.Errorf("runtime transition removes completed dispatch")
			}
			settled := dispatch.State == RuntimeDispatchStateCommitted
			for _, execution := range dispatch.OperationExecutions {
				settled = settled && execution.State == OperationExecutionEvidenceCommitted
			}
			if settled {
				if !runtimeRawEqual(dispatch, next) {
					return fmt.Errorf("runtime transition rewrites committed dispatch result")
				}
			} else if dispatch.DispatchAuthorityHash != next.DispatchAuthorityHash || StableRuntimeDispatchAuthorityHash(dispatch) != StableRuntimeDispatchAuthorityHash(next) {
				return fmt.Errorf("runtime transition rewrites invalidated dispatch authority")
			}
		}
	}
	return nil
}

func (s *runtimeSemanticValidationSession) committedGeneration(raw RuntimeSemanticPlanningGeneration) bool {
	if s == nil || s.committedPreimage == nil {
		return false
	}
	if s.committedGenerations == nil {
		s.committedGenerations = make(map[runtimeCommittedGenerationIdentity][]RuntimeSemanticPlanningGeneration)
		for _, prior := range s.committedPreimage.SemanticPlanning.AllHistory() {
			key := runtimeCommittedGenerationIdentity{prior.RetiredAtStateRevision, prior.Reason}
			s.committedGenerations[key] = append(s.committedGenerations[key], prior)
		}
	}
	key := runtimeCommittedGenerationIdentity{raw.RetiredAtStateRevision, raw.Reason}
	for _, prior := range s.committedGenerations[key] {
		if runtimeRawEqual(prior, raw) {
			return true
		}
	}
	return false
}

func (s *runtimeSemanticValidationSession) completedWorkRound(round RuntimeImplementationWorkRound) bool {
	return round.CompletedReceiptSetHash != "" && s.committedWorkRound(round)
}

// An unchanged open round keeps its admitted structural/semantic inputs too.
// Callers must still join its changing execution, receipts and current resources.
func (s *runtimeSemanticValidationSession) committedWorkRound(round RuntimeImplementationWorkRound) bool {
	if s == nil || s.committedPreimage == nil {
		return false
	}
	if s.committedWorkRounds == nil {
		s.committedWorkRounds = make(map[string][]RuntimeImplementationWorkRound)
		for _, prior := range s.committedPreimage.ImplementationWork.Rounds {
			key := prior.ScopeRequest.RequestID
			s.committedWorkRounds[key] = append(s.committedWorkRounds[key], prior)
		}
	}
	for _, prior := range s.committedWorkRounds[round.ScopeRequest.RequestID] {
		if runtimeRawEqual(prior, round) {
			return true
		}
	}
	return false
}

// An index of original values lives only inside one synchronous compiler call.
// Membership never establishes reuse: every hit still compares the complete
// raw original, including all history and nested inputs. No validity survives.
type runtimeCommittedGenerationIdentity struct {
	Revision uint64
	Reason   RuntimeSemanticPlanningRetirementReason
}
