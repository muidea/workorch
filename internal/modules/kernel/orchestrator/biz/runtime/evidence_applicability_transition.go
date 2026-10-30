package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	orchestratorcommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (event RuntimeInternalEvent) validateEvidenceApplicabilityEnvelope() error {
	if event.Authority == nil {
		return nil
	}
	payload := *event.Authority
	observation := payload.EvidenceApplicability
	payload.EvidenceApplicability = nil
	if event.Kind == RuntimeInternalEventEvidenceApplicabilityObserved {
		if observation == nil || !reflect.DeepEqual(payload, RuntimeInternalAuthorityPayload{}) {
			return fmt.Errorf("%w: evidence applicability accepts exactly one owner observation", common.ErrInvalidArgument)
		}
		return observation.ValidateRaw()
	}
	if observation != nil {
		return fmt.Errorf("%w: unrelated private event cannot carry applicability observation", common.ErrInvalidArgument)
	}
	return nil
}

func validateEvidenceApplicabilityObservationTransition(previous, next RuntimeAggregate) error {
	before, after := previous.Facts.EvidenceApplicabilityObservations, next.Facts.EvidenceApplicabilityObservations
	if len(after) < len(before) || len(after) > len(before)+1 {
		return fmt.Errorf("%w: applicability observations are append-only single-CAS facts", common.ErrConflict)
	}
	for index, value := range before {
		if !reflect.DeepEqual(value, after[index]) {
			return fmt.Errorf("%w: original applicability observation cannot be rewritten", common.ErrConflict)
		}
	}
	if len(after) == len(before)+1 {
		added := after[len(before)]
		if previous.Termination != nil || previous.TerminalMaterialSeal != nil || isTerminalConvergenceState(previous.Decision.State) || previous.ActiveDispatch != nil ||
			added.Request.IssuedAtStateRevision != previous.StateRevision || added.RecordedAtStateRevision != next.StateRevision || added.Request.EvidenceRevision != previous.Facts.EvidenceRevision {
			return fmt.Errorf("%w: applicability observation crossed its original authority window", common.ErrConflict)
		}
		if runtimeApplicabilityHasUnsettledProvider(previous) {
			return fmt.Errorf("%w: applicability cannot retire unsettled provider responsibility", common.ErrConflict)
		}
		if added.Request.PurposeRef == runtimecontract.RuntimeHistoricalEffectProofV1 {
			expected := previous.CloneRaw()
			expected.StateRevision = next.StateRevision
			expected.Facts.EvidenceApplicabilityObservations = runtimecontract.CloneRuntimeEvidenceApplicabilityObservations(after)
			if !reflect.DeepEqual(expected, next) {
				return fmt.Errorf("%w: historical audit can only append its original observation", common.ErrConflict)
			}
		}
		if runtimecontract.StableRuntimeEvidenceApplicabilitySemanticHash(before) != runtimecontract.StableRuntimeEvidenceApplicabilitySemanticHash(after) {
			expected := previous.CloneRaw()
			transaction := previous.Decision.AuthorityTransaction()
			retireUnsealedCandidateForApplicability(&expected, transaction, added.Request)
			if len(next.CandidateAttempts) != len(expected.CandidateAttempts) || len(next.CandidateRequestSessions) != len(expected.CandidateRequestSessions) {
				return fmt.Errorf("%w: applicability retirement cannot add or discard candidate history", common.ErrConflict)
			}
			for index, attempt := range previous.CandidateAttempts {
				if attempt.Transaction == transaction && !reflect.DeepEqual(next.CandidateAttempts[index], expected.CandidateAttempts[index]) {
					return fmt.Errorf("%w: applicability retirement has incomplete candidate write set", common.ErrConflict)
				}
			}
			for index, session := range previous.CandidateRequestSessions {
				if session.Transaction == transaction && !reflect.DeepEqual(next.CandidateRequestSessions[index], expected.CandidateRequestSessions[index]) {
					return fmt.Errorf("%w: applicability retirement has incomplete request write set", common.ErrConflict)
				}
			}
		}
	}
	return nil
}

// CompileRuntimeEvidenceApplicabilityRequest reverse-proves the exact immutable
// fact and owner read source. It does not select an operation or grant credit.
func CompileRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate, evidenceID common.EvidenceRecordID) (runtimecontract.RuntimeEvidenceApplicabilityRequest, error) {
	if err := aggregate.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, err
	}
	return compileRuntimeEvidenceApplicabilityRequest(aggregate, evidenceID)
}

// No validation credential escapes this call chain. Public entry points run
// the complete aggregate gate once before these unchanged exact source joins.
func compileRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate, evidenceID common.EvidenceRecordID) (runtimecontract.RuntimeEvidenceApplicabilityRequest, error) {
	var fact runtimecontract.EvidenceFact
	matches := 0
	for _, value := range aggregate.Facts.Evidence.Accepted {
		if value.ID == evidenceID {
			fact, matches = value, matches+1
		}
	}
	if matches != 1 || fact.ApplicabilityDependencies == nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, fmt.Errorf("%w: applicability requires unique versioned accepted source", common.ErrConflict)
	}
	var receiptHash string
	for _, receipt := range aggregate.OperationReceipts {
		if fact.SourceRef == "runtime-receipt/"+receipt.ExecutionKey && receipt.OperationID == fact.OperationID {
			if receiptHash != "" {
				return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, common.ErrConflict
			}
			receiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		}
	}
	source, err := compileRuntimeOperationReadSource(aggregate, fact.DispatchID, fact.OperationID, receiptHash)
	if err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, err
	}
	if !reflect.DeepEqual(source.ApplicabilitySource, fact.ApplicabilityDependencies) || !reflect.DeepEqual(&source.OriginalDependencies, fact.OwnerRevisionSet) {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, fmt.Errorf("%w: applicability source payload mismatch", common.ErrConflict)
	}
	request := (runtimecontract.RuntimeEvidenceApplicabilityRequest{
		ProtocolRef:           runtimecontract.RuntimeEvidenceApplicabilityObservationV1,
		IssuedAtStateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		SourceEvidenceID: fact.ID, SourceEvidenceHash: runtimecontract.StableRuntimeEvidenceFactHash(fact), SourceReceiptHash: receiptHash,
		DispatchID: fact.DispatchID, OperationID: fact.OperationID,
		ReconciliationRequestHash: aggregate.Reconciliation.RequestHash,
		PurposeRef:                fact.ApplicabilityDependencies.PurposeRef,
	}).WithID()
	return request, request.ValidateRaw()
}

// NextRuntimeEvidenceApplicabilityRequest projects at most one read-only
// obligation for evidence attributable to a current binding. SM-4 performs the
// owner query; the private writer independently repeats source/fence admission.
// A new evidence revision requires fresh observations before further credit.
func NextRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, bool, error) {
	if err := aggregate.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, err
	}
	return nextRuntimeEvidenceApplicabilityRequest(aggregate)
}

// Only complete cold/owner-local gates may enter this pure continuation.
func nextRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, bool, error) {
	if aggregate.ActiveDispatch != nil || aggregate.Termination != nil || aggregate.TerminalMaterialSeal != nil || isTerminalConvergenceState(aggregate.Decision.State) {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, nil
	}
	if runtimeApplicabilityHasUnsettledProvider(aggregate) {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, nil
	}
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if fact.ApplicabilityDependencies == nil || runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) || runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, aggregate.Facts.EvidenceApplicabilityObservations, aggregate.Facts.EvidenceRevision) {
			continue
		}
		if !runtimeApplicabilityEvidenceAttributable(aggregate.Facts, fact, aggregate.CoverageProofLineage) {
			continue
		}
		observed := false
		for _, observation := range aggregate.Facts.EvidenceApplicabilityObservations {
			if observation.Request.SourceEvidenceID == fact.ID && observation.Request.SourceEvidenceHash == runtimecontract.StableRuntimeEvidenceFactHash(fact) && observation.Request.EvidenceRevision == aggregate.Facts.EvidenceRevision && observation.Request.ReconciliationRequestHash == aggregate.Reconciliation.RequestHash {
				observed = true
			}
		}
		if observed {
			continue
		}
		request, err := compileRuntimeEvidenceApplicabilityRequest(aggregate, fact.ID)
		return request, err == nil, err
	}
	return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, nil
}

// CompileRuntimeEvidenceApplicabilityRead keeps request and owner source in one
// complete read compilation. These detached values are ordinary query data;
// the owner and private observation writer still independently check provenance.
// It retains nothing and grants no credit or execution authority.
func CompileRuntimeEvidenceApplicabilityRead(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, RuntimeOperationReadSource, bool, error) {
	if err := aggregate.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, RuntimeOperationReadSource{}, false, err
	}
	return compileRuntimeEvidenceApplicabilityRead(aggregate)
}

// CompileEvidenceApplicabilityRead is an Orchestrator-local read compiler. It
// executes the complete aggregate gate on every supplied raw image, sharing
// only the same bounded exact-raw historical pure products as Load/Commit.
// This does not read an owner or cache a plan/current invariant success. The
// caller still queries the actual resource owner; the observation event reloads
// and independently admits its current source, revision and CAS fences.
func (c *RuntimeController) CompileEvidenceApplicabilityRead(aggregate RuntimeAggregate) (request runtimecontract.RuntimeEvidenceApplicabilityRequest, source RuntimeOperationReadSource, required bool, err error) {
	if c == nil {
		return request, source, false, common.ErrInvalidArgument
	}
	err = c.historicalCompiler.Compile(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
		if err := validate(ownerStateSourceImage(nil, aggregate)); err != nil {
			return err
		}
		var compileErr error
		request, source, required, compileErr = compileRuntimeEvidenceApplicabilityRead(aggregate)
		return compileErr
	})
	return
}

func compileRuntimeEvidenceApplicabilityRead(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, RuntimeOperationReadSource, bool, error) {
	request, required, err := nextRuntimeEvidenceApplicabilityRequest(aggregate)
	if err != nil || !required {
		return request, RuntimeOperationReadSource{}, false, err
	}
	source, err := compileRuntimeOperationReadSource(aggregate, request.DispatchID, request.OperationID, request.SourceReceiptHash)
	if err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, RuntimeOperationReadSource{}, false, err
	}
	return request, source, true, nil
}

func runtimeApplicabilityEvidenceAttributable(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	focuses := append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	focuses = append(focuses, facts.CompletedFocuses...)
	for _, focus := range focuses {
		probe := facts
		probe.Focus = focus
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if runtimeEvidenceAppliesToCoverageWorkItemWithLineage(probe, fact, coverageID, lineage) {
				return true
			}
		}
	}
	return false
}

// A current-focus proof cannot close the Run while another current binding's
// versioned source still awaits this evidence revision. This is a facts-only
// reducer fence; aggregate admission and the query planner additionally bind
// the exact Resume/reconciliation generation.
func runtimeApplicabilityRevisionPending(facts runtimecontract.RuntimeFacts, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	for _, fact := range facts.Evidence.Accepted {
		if fact.ApplicabilityDependencies != nil && !runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) && runtimeApplicabilityEvidenceAttributable(facts, fact, lineage) &&
			!runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) &&
			!runtimecontract.RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) {
			return true
		}
	}
	return false
}

// Aggregate-backed reductions also have the exact Resume/reconciliation fence.
// Keep this separate from the facts-only helper: transaction metadata must not
// be inferred from an old observation or added to immutable evidence facts.
func runtimeApplicabilityGenerationPending(facts runtimecontract.RuntimeFacts, lineage []runtimecontract.RuntimeCoverageProofLineage, reconciliationRequestHash string) bool {
	for _, fact := range facts.Evidence.Accepted {
		if fact.ApplicabilityDependencies != nil && !runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) && runtimeApplicabilityEvidenceAttributable(facts, fact, lineage) &&
			!runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) &&
			!runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision, reconciliationRequestHash) {
			return true
		}
	}
	return false
}

// Project the first original source that still needs a read. The dependency
// ledger is the observation ledger, not a second acquisition/credit ledger.
// Generic prerequisite acquisition cannot submit this kind; SM-4 must use the
// concrete original-owner query and EvidenceApplicabilityObserved transaction.
func runtimeApplicabilityPrerequisiteDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, lineage *RuntimeAggregate, proofLineage []runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, bool, error) {
	for _, fact := range facts.Evidence.Accepted {
		if fact.ApplicabilityDependencies == nil || runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) || !runtimeApplicabilityEvidenceAttributable(facts, fact, proofLineage) {
			continue
		}
		fresh := runtimecontract.RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision)
		generation := ""
		if lineage != nil {
			generation = lineage.Reconciliation.RequestHash
			fresh = runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision, generation)
		}
		if fresh || runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) {
			continue
		}
		id := runtimeApplicabilityPrerequisiteID(fact, facts.EvidenceRevision, generation)
		base.State = runtimecontract.ConvergenceStateNeedPrerequisite
		base.Prerequisites = []runtimecontract.RuntimePrerequisite{{ID: id, Kind: runtimecontract.RuntimeEvidenceApplicabilityPrerequisiteV1, Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: orchestratorcommon.ModuleID, SubjectRef: string(fact.ID), Reason: "observe original owner dependencies before current proof consumption"}}
		base.RequiredOutput = runtimecontract.RequiredOutputContract{PrerequisiteIDs: []string{id}}
		return base, true, nil
	}
	return runtimecontract.RuntimeDecision{}, false, nil
}

func runtimeApplicabilityPrerequisiteID(fact runtimecontract.EvidenceFact, revision, generation string) string {
	body, _ := json.Marshal(struct {
		ProtocolRef               string
		EvidenceID                common.EvidenceRecordID
		EvidenceHash              string
		EvidenceRevision          string
		ReconciliationRequestHash string
	}{string(runtimecontract.RuntimeEvidenceApplicabilityPrerequisiteV1), fact.ID, runtimecontract.StableRuntimeEvidenceFactHash(fact), revision, generation})
	digest := sha256.Sum256(body)
	return "evidence-read-debt-" + hex.EncodeToString(digest[:])
}

func validateRuntimeApplicabilityPrerequisiteProjection(aggregate RuntimeAggregate) error {
	for _, prerequisite := range aggregate.Decision.Prerequisites {
		if prerequisite.Kind != runtimecontract.RuntimeEvidenceApplicabilityPrerequisiteV1 {
			continue
		}
		if len(aggregate.Decision.Prerequisites) != 1 || runtimecontract.StableRequiredOutputHash(aggregate.Decision.RequiredOutput) != runtimecontract.StableRequiredOutputHash(runtimecontract.RequiredOutputContract{PrerequisiteIDs: []string{prerequisite.ID}}) {
			return fmt.Errorf("%w: applicability debt cannot carry operation authority", common.ErrConflict)
		}
		for _, fact := range aggregate.Facts.Evidence.Accepted {
			if string(fact.ID) != prerequisite.SubjectRef || fact.ApplicabilityDependencies == nil {
				continue
			}
			generations := []string{"", aggregate.Reconciliation.RequestHash}
			for _, history := range aggregate.Reconciliation.History {
				generations = append(generations, history.RequestHash)
			}
			for _, generation := range generations {
				id := runtimeApplicabilityPrerequisiteID(fact, aggregate.Decision.AuthoritySnapshot.EvidenceRevision, generation)
				expected := runtimecontract.RuntimePrerequisite{ID: id, Kind: runtimecontract.RuntimeEvidenceApplicabilityPrerequisiteV1, Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: orchestratorcommon.ModuleID, SubjectRef: string(fact.ID), Reason: "observe original owner dependencies before current proof consumption"}
				if reflect.DeepEqual(prerequisite, expected) {
					return nil
				}
			}
		}
		return fmt.Errorf("%w: applicability debt has no exact original source/fence", common.ErrConflict)
	}
	return nil
}

// The owner-private transaction retains the observation and invalidates only
// the exact source CWI's existing credit. It cannot manufacture another receipt,
// erase effect/unknown/budget responsibility or reopen a delivered terminal.
func (c *authorityComponent) recordEvidenceApplicability(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, observation runtimecontract.RuntimeEvidenceApplicabilityObservation) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	for _, existing := range aggregate.Facts.EvidenceApplicabilityObservations {
		if existing.Request.RequestID == observation.Request.RequestID {
			if !reflect.DeepEqual(existing, observation) {
				return RuntimeAggregate{}, fmt.Errorf("%w: duplicate applicability result changed body", common.ErrConflict)
			}
			return aggregate, nil
		}
	}
	if aggregate.Termination != nil || aggregate.TerminalMaterialSeal != nil || isTerminalConvergenceState(aggregate.Decision.State) || aggregate.ActiveDispatch != nil ||
		runtimeApplicabilityHasUnsettledProvider(aggregate) ||
		observation.Request.IssuedAtStateRevision != aggregate.StateRevision || observation.Request.EvidenceRevision != aggregate.Facts.EvidenceRevision {
		return RuntimeAggregate{}, fmt.Errorf("%w: applicability observation has stale authority window", common.ErrConflict)
	}
	expected, err := CompileRuntimeEvidenceApplicabilityRequest(aggregate, observation.Request.SourceEvidenceID)
	if err != nil || !reflect.DeepEqual(expected, observation.Request) {
		return RuntimeAggregate{}, fmt.Errorf("%w: applicability request does not match original source: %v", common.ErrConflict, err)
	}
	if len(aggregate.Facts.EvidenceApplicabilityObservations) >= runtimecontract.RuntimeEvidenceApplicabilityObservationLimit {
		return RuntimeAggregate{}, fmt.Errorf("%w: applicability observation ledger exhausted", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeEvidenceApplicabilityObservationSource(observation, aggregate.Facts, aggregate.OperationReceipts); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	previousObservationHash := runtimecontract.StableRuntimeEvidenceApplicabilitySemanticHash(aggregate.Facts.EvidenceApplicabilityObservations)
	previousTransaction := aggregate.Decision.AuthorityTransaction()
	aggregate.Facts.EvidenceApplicabilityObservations = append(aggregate.Facts.EvidenceApplicabilityObservations, observation.CloneRaw())
	if previousObservationHash != runtimecontract.StableRuntimeEvidenceApplicabilitySemanticHash(aggregate.Facts.EvidenceApplicabilityObservations) {
		retireUnsealedCandidateForApplicability(&aggregate, previousTransaction, observation.Request)
	}
	var fact runtimecontract.EvidenceFact
	for _, value := range aggregate.Facts.Evidence.Accepted {
		if value.ID == observation.Request.SourceEvidenceID {
			fact = value
		}
	}
	if runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) {
		// Audit-only facts must not re-run focus advancement or decision sealing:
		// even an otherwise equivalent reduction increments the action attempt,
		// which would strand an ordinary request on its original transaction.
		aggregate.StateRevision++
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return RuntimeAggregate{}, err
		}
		return aggregate, nil
	}
	if runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, aggregate.Facts.EvidenceApplicabilityObservations, aggregate.Facts.EvidenceRevision) {
		var triggers []runtimeCWIReopenTrigger
		for _, coverageID := range fact.CoverageWorkItemIDs {
			triggers = append(triggers, runtimeCWIReopenTrigger{CoverageWorkItemID: coverageID, TriggerRef: observation.Request.RequestID, Reason: "owner dependency observation no longer matches original evidence applicability"})
		}
		if _, err := applyRuntimeCWIReopen(&aggregate, triggers); err != nil {
			return RuntimeAggregate{}, err
		}
		if _, err := supersedeActiveReconciliationForCurrentContract(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
		if err := retireSemanticPlanningForCurrentReopenedCWIs(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
		if _, err := suspendRuntimeSuccessorForStalePredecessor(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
	}
	// Assessment advances the evidence revision, so its settled CWI remains
	// fenced until fresh reads are complete. The matching observation must use
	// the ordinary SM-2 advancement path before reducing a focus-level complete;
	// otherwise a remaining CWI could be mistaken for whole-Run completion.
	previousFocusID := aggregate.Facts.Focus.ID
	if len(aggregate.Facts.Prerequisites.Pending) == 0 {
		advanced, advanceErr := advanceRuntimeCWIFocus(aggregate.Facts, aggregate.CoverageProofLineage)
		if advanceErr != nil {
			return RuntimeAggregate{}, advanceErr
		}
		aggregate.Facts = advanced
		if aggregate.Facts.Focus.ID != previousFocusID {
			if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
				return RuntimeAggregate{}, err
			}
		}
	}
	facts, decision, err := reduceRuntimeAggregateWithDirective(aggregate, aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// Submission responsibility must first take the original document/unknown
// settlement lane. A read-only observation cannot clear reserved allocations.
func runtimeApplicabilityHasUnsettledProvider(aggregate RuntimeAggregate) bool {
	for _, session := range aggregate.CandidateRequestSessions {
		if session.ProviderSubmitted && session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			return true
		}
	}
	return false
}

func retireUnsealedCandidateForApplicability(aggregate *RuntimeAggregate, transaction runtimecontract.AuthorityTransaction, request runtimecontract.RuntimeEvidenceApplicabilityRequest) {
	boundSessions := make(map[string]bool)
	for index := range aggregate.CandidateAttempts {
		attempt := &aggregate.CandidateAttempts[index]
		boundSessions[attempt.LogicalRequestID] = true
		if attempt.Transaction != transaction || attempt.State != runtimecontract.RuntimeCandidateAttemptStatePending {
			continue
		}
		attempt.State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
		attempt.Proposal, attempt.Conformances, attempt.Feasibilities = nil, nil, nil
		originalRequest := request
		attempt.ApplicabilityInvalidationRequest = &originalRequest
	}
	for index := range aggregate.CandidateRequestSessions {
		session := &aggregate.CandidateRequestSessions[index]
		if session.Transaction != transaction || session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal || boundSessions[session.LogicalRequestID] {
			continue
		}
		// Completed sessions already bound to attempts stay byte-for-byte original.
		// Unadmitted documents and unsent requests cannot enter a new attempt.
		session.State = runtimecontract.RuntimeCandidateRequestSessionTerminal
		session.ProviderSubmitted = false
		session.LastDocumentFraming = nil
		session.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale, Code: "evidence_applicability_changed", Signature: request.RequestID}
	}
}
