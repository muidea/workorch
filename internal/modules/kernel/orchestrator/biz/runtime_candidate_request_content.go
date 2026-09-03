package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeCandidateRequestContentMode      = "runtime_candidate_request_content"
	runtimeCandidateRequestContentTargetRef = "runtime_candidate_request"
)

// loadOrCreateRuntimeCandidateRequestContent creates only the opaque content
// carrier for a persisted logical request. Artifact metadata records sealed
// provenance for corruption detection, but session state, sequence ownership,
// budgets and candidate allocation remain in the Orchestrator aggregate.
func loadOrCreateRuntimeCandidateRequestContent(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtime.RuntimeAggregate, logicalRequestID string) (*runtimecontract.ArtifactContinuation, error) {
	if repo == nil || logicalRequestID == "" {
		return nil, common.ErrInvalidArgument
	}
	id := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	value, _, err := submitCandidateContentInternalEvent(ctx, repo, candidateContentInternalEvent{
		Kind: candidateContentInternalEventCreated, ContinuationID: id,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, LogicalRequestID: logicalRequestID,
		AuthorityTransaction: aggregate.Decision.AuthorityTransaction(), Created: &candidateContentCreatedFact{CreatedAt: now, UpdatedAt: now},
	})
	return value, err
}

// persistRuntimeCandidateRequestDocument stores a complete raw document before
// the request-session controller marks it complete. It is intentionally not a
// candidate record: decoder/admission consume it only in C5 after C4 has
// committed the session transition.
func persistRuntimeCandidateRequestDocument(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtime.RuntimeAggregate, logicalRequestID string, sequence int, physicalCallID string, content []byte) (common.ArtifactContinuationID, string, error) {
	if err := (runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID}).Validate(); err != nil || len(content) == 0 {
		return "", "", common.ErrInvalidArgument
	}
	continuation, err := loadOrCreateRuntimeCandidateRequestContent(ctx, repo, aggregate, logicalRequestID)
	if err != nil {
		return "", "", err
	}
	contentString := string(content)
	hash := runtimeCandidateOutputHash(contentString)
	if existing, existingContent, getErr := repo.GetAssembled(ctx, continuation.ID); getErr == nil {
		if existing.RollingHash != hash || existing.Bytes != int64(len(content)) || existingContent != contentString {
			return "", "", fmt.Errorf("%w: candidate request complete document conflicts", common.ErrConflict)
		}
		if _, _, err := submitCandidateContentInternalEvent(ctx, repo, candidateContentInternalEvent{
			Kind: candidateContentInternalEventApplied, ContinuationID: continuation.ID,
			WorkspaceID: continuation.WorkspaceID, RunID: continuation.RunID, LogicalRequestID: logicalRequestID,
			AuthorityTransaction: continuation.AuthorityTransaction,
			Applied:              &candidateContentAppliedFact{BytesWritten: int64(len(content)), RollingHash: hash, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		}); err != nil {
			return "", "", err
		}
		return continuation.ID, hash, nil
	} else if !errors.Is(getErr, common.ErrNotFound) {
		return "", "", getErr
	}
	metadata := &runtimecontract.ArtifactContinuationAssembledMetadata{
		ContinuationID: continuation.ID, TargetRef: continuation.TargetRef, Bytes: int64(len(content)), RollingHash: hash,
		SourceChunkRefs: nil, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := repo.SaveAssembled(ctx, metadata, contentString); err != nil {
		return "", "", err
	}
	if _, _, err := submitCandidateContentInternalEvent(ctx, repo, candidateContentInternalEvent{
		Kind: candidateContentInternalEventApplied, ContinuationID: continuation.ID,
		WorkspaceID: continuation.WorkspaceID, RunID: continuation.RunID, LogicalRequestID: logicalRequestID,
		AuthorityTransaction: continuation.AuthorityTransaction,
		Applied:              &candidateContentAppliedFact{BytesWritten: int64(len(content)), RollingHash: hash, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}); err != nil {
		return "", "", err
	}
	return continuation.ID, hash, nil
}

// persistAndCompleteRuntimeCandidateRequestDocument is the equivalent C3
// bridge for an already complete raw document. The controller transition is
// deliberately separate from candidate admission: after this function
// returns, CandidateAttempts must still be empty and C5 is solely responsible
// for decoding/admitting the stored document.
func (m *Biz) persistAndCompleteRuntimeCandidateRequestDocument(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID string, sequence int, physicalCallID string, content []byte, budgetSettlement ...*runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, common.ArtifactContinuationID, string, error) {
	if m == nil || m.runtimeController == nil || m.evidence.ArtifactContinuations == nil {
		return runtime.RuntimeAggregate{}, "", "", common.ErrInvalidArgument
	}
	var id common.ArtifactContinuationID
	var hash string
	err := m.withOrchestratorEvidenceMutation(ctx, aggregate.Facts.WorkspaceID, func(guarded context.Context) error {
		var persistErr error
		id, hash, persistErr = persistRuntimeCandidateRequestDocument(
			guarded,
			m.evidence.ArtifactContinuations,
			aggregate,
			logicalRequestID,
			sequence,
			physicalCallID,
			content,
		)
		return persistErr
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, "", "", err
	}
	updatedEvent, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateDocumentObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID, ProviderBudgetSettlement: firstRuntimeProviderBudgetSettlement(budgetSettlement)},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, "", "", err
	}
	return updatedEvent.Aggregate, id, hash, nil
}

// markRuntimeCandidateRequestContentFailed closes the opaque content carrier
// after its aggregate-owned request session has reached a terminal outcome.
// The carrier remains audit-only: it never chooses a retry or changes the
// session/candidate state machine.
func (m *Biz) markRuntimeCandidateRequestContentFailed(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID string) error {
	if m == nil || m.runtimeController == nil || m.evidence.ArtifactContinuations == nil || strings.TrimSpace(logicalRequestID) == "" {
		return common.ErrInvalidArgument
	}
	repository := m.evidence.ArtifactContinuations
	return m.withOrchestratorEvidenceMutation(ctx, aggregate.Facts.WorkspaceID, func(guarded context.Context) error {
		return m.markRuntimeCandidateRequestContentFailedGuarded(guarded, repository, aggregate, logicalRequestID)
	})
}

func (m *Biz) markRuntimeCandidateRequestContentFailedGuarded(ctx context.Context, repository repository.ArtifactContinuationRepository, aggregate runtime.RuntimeAggregate, logicalRequestID string) error {
	id := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	content, err := repository.Get(ctx, id)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	var sessionTransaction runtimecontract.AuthorityTransaction
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == logicalRequestID {
			sessionTransaction = session.Transaction
			break
		}
	}
	if sessionTransaction.AuthoritySnapshotID == "" {
		return fmt.Errorf("%w: candidate request session is missing", common.ErrNotFound)
	}
	if content.WorkspaceID != aggregate.Facts.WorkspaceID || content.RunID != aggregate.Facts.RunID ||
		content.ArtifactID != logicalRequestID || content.TargetRef != runtimeCandidateRequestContentTargetRef ||
		content.OutputMode != runtimeCandidateRequestContentMode || content.AuthorityTransaction != sessionTransaction {
		return fmt.Errorf("%w: candidate request content does not match sealed authority", common.ErrConflict)
	}
	_, _, err = submitCandidateContentInternalEvent(ctx, repository, candidateContentInternalEvent{
		Kind: candidateContentInternalEventFailed, ContinuationID: content.ID,
		WorkspaceID: content.WorkspaceID, RunID: content.RunID, LogicalRequestID: logicalRequestID,
		AuthorityTransaction: content.AuthorityTransaction, Failed: &candidateContentFailedFact{UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	return err
}

func loadRuntimeCandidateRequestDocument(ctx context.Context, repo repository.ArtifactContinuationRepository, logicalRequestID string) ([]byte, error) {
	return loadRuntimeCandidateRequestDocumentForAuthority(ctx, repo, logicalRequestID, "", "", runtimecontract.AuthorityTransaction{})
}

func loadRuntimeCandidateRequestDocumentForAuthority(ctx context.Context, repo repository.ArtifactContinuationRepository, logicalRequestID string, workspaceID common.WorkspaceID, runID common.RunID, transaction runtimecontract.AuthorityTransaction) ([]byte, error) {
	if repo == nil || logicalRequestID == "" {
		return nil, common.ErrInvalidArgument
	}
	id := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	carrier, err := repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	metadata, content, err := repo.GetAssembled(ctx, id)
	if err != nil {
		return nil, err
	}
	if carrier.ArtifactID != logicalRequestID || carrier.TargetRef != runtimeCandidateRequestContentTargetRef ||
		carrier.OutputMode != runtimeCandidateRequestContentMode || carrier.State != common.ArtifactContinuationStateApplied ||
		!carrier.FinalReceived || carrier.BytesWritten <= 0 || carrier.RollingHash == "" ||
		metadata.ContinuationID != carrier.ID || metadata.TargetRef != carrier.TargetRef ||
		metadata.RollingHash != carrier.RollingHash || metadata.Bytes != carrier.BytesWritten ||
		metadata.RollingHash != runtimeCandidateOutputHash(content) || metadata.Bytes != int64(len(content)) {
		return nil, fmt.Errorf("%w: candidate request complete document hash is invalid", common.ErrConflict)
	}
	if workspaceID != "" && (carrier.WorkspaceID != workspaceID || carrier.RunID != runID || carrier.AuthorityTransaction != transaction) {
		return nil, fmt.Errorf("%w: candidate request complete document authority is invalid", common.ErrConflict)
	}
	return []byte(content), nil
}

// loadCompletedRuntimeCandidateRequestDocument is the only C3/C5 read path
// for a raw request document. A chunk being durable is insufficient: the
// aggregate must first have committed document_complete for the exact logical
// session. This keeps an uncommitted or abandoned provider response out of the
// decoder/admission path.
func loadCompletedRuntimeCandidateRequestDocument(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtime.RuntimeAggregate, logicalRequestID string) ([]byte, error) {
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID != logicalRequestID {
			continue
		}
		if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete {
			return nil, fmt.Errorf("%w: runtime candidate request document is not complete", common.ErrConflict)
		}
		return loadRuntimeCandidateRequestDocumentForAuthority(ctx, repo, logicalRequestID, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.Transaction)
	}
	return nil, fmt.Errorf("%w: runtime candidate request session not found", common.ErrNotFound)
}
