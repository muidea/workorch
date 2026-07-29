package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
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
func loadOrCreateRuntimeCandidateRequestContent(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtimev2.RuntimeAggregate, logicalRequestID string) (*runtimecontract.ArtifactContinuation, error) {
	if repo == nil || logicalRequestID == "" {
		return nil, common.ErrInvalidArgument
	}
	id := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	value, err := repo.Get(ctx, id)
	if err == nil {
		if value.WorkspaceID != aggregate.Facts.WorkspaceID || value.RunID != aggregate.Facts.RunID || value.ArtifactID != logicalRequestID ||
			value.TargetRef != runtimeCandidateRequestContentTargetRef || value.OutputMode != runtimeCandidateRequestContentMode ||
			value.AuthorityTransaction != aggregate.Decision.AuthorityTransaction() || value.CandidateAttempt != 0 {
			return nil, fmt.Errorf("%w: candidate request content does not match sealed authority", common.ErrConflict)
		}
		return value, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}
	value = &runtimecontract.ArtifactContinuation{
		ID: id, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ArtifactID: logicalRequestID, TargetRef: runtimeCandidateRequestContentTargetRef,
		Operation: "candidate_request_content", OutputMode: runtimeCandidateRequestContentMode,
		AuthorityTransaction: aggregate.Decision.AuthorityTransaction(), State: common.ArtifactContinuationStateChunking,
		ExpectedSequence: 1, ReceivedChunkHashes: map[int]string{},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := repo.Create(ctx, value); err != nil {
		if !errors.Is(err, common.ErrConflict) {
			return nil, err
		}
		return repo.Get(ctx, id)
	}
	return value, nil
}

// persistRuntimeCandidateRequestContentChunk durably writes opaque raw bytes
// before C3 records their metadata in the request-session ledger. Replays are
// accepted only for byte-identical content and the same physical call.
func persistRuntimeCandidateRequestContentChunk(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtimev2.RuntimeAggregate, logicalRequestID string, sequence int, physicalCallID string, content []byte, reachedProviderLimit bool) (runtimecontract.RuntimeCandidateRequestFragment, error) {
	if err := runtimecontract.ValidateRuntimeCandidateRequestIdentity(logicalRequestID, sequence, physicalCallID); err != nil || len(content) == 0 {
		return runtimecontract.RuntimeCandidateRequestFragment{}, common.ErrInvalidArgument
	}
	continuation, err := loadOrCreateRuntimeCandidateRequestContent(ctx, repo, aggregate, logicalRequestID)
	if err != nil {
		return runtimecontract.RuntimeCandidateRequestFragment{}, err
	}
	contentString := string(content)
	hash := runtimeCandidateOutputHash(contentString)
	if existing, existingContent, getErr := repo.GetChunk(ctx, continuation.ID, sequence); getErr == nil {
		if existing.ContentHash != hash || existingContent != contentString || len(existing.LLMCallRefs) != 1 || existing.LLMCallRefs[0] != physicalCallID {
			return runtimecontract.RuntimeCandidateRequestFragment{}, fmt.Errorf("%w: candidate request content chunk conflicts at sequence %d", common.ErrConflict, sequence)
		}
		return runtimecontract.RuntimeCandidateRequestFragment{Sequence: sequence, ContentHash: hash, Bytes: int64(len(content)), ReachedProviderLimit: reachedProviderLimit, PhysicalCallID: physicalCallID}, nil
	} else if !errors.Is(getErr, common.ErrNotFound) {
		return runtimecontract.RuntimeCandidateRequestFragment{}, getErr
	}
	if err := repo.SaveChunk(ctx, &runtimecontract.ArtifactContinuationChunk{
		ContinuationID: continuation.ID, Sequence: sequence, ArtifactID: continuation.ArtifactID, TargetRef: continuation.TargetRef,
		ContentHash: hash, Bytes: int64(len(content)), LLMCallRefs: []string{physicalCallID}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, contentString); err != nil {
		return runtimecontract.RuntimeCandidateRequestFragment{}, err
	}
	return runtimecontract.RuntimeCandidateRequestFragment{Sequence: sequence, ContentHash: hash, Bytes: int64(len(content)), ReachedProviderLimit: reachedProviderLimit, PhysicalCallID: physicalCallID}, nil
}

// persistAndRecordRuntimeCandidateRequestFragment is the C3 bridge between
// opaque content storage and the aggregate-owned session ledger. Content must
// be durable before the controller CAS makes its hash/sequence visible. This
// helper intentionally does not submit a provider request, allocate a
// candidate attempt, alter a recovery budget, or create a dispatch.
func (m *Biz) persistAndRecordRuntimeCandidateRequestFragment(ctx context.Context, aggregate runtimev2.RuntimeAggregate, logicalRequestID string, sequence int, physicalCallID string, content []byte, reachedProviderLimit bool) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestFragment, error) {
	if m == nil || m.bizPtr == nil || m.bizPtr.EvidenceRepositories().ArtifactContinuations == nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestFragment{}, common.ErrInvalidArgument
	}
	fragment, err := persistRuntimeCandidateRequestContentChunk(
		ctx,
		m.bizPtr.EvidenceRepositories().ArtifactContinuations,
		aggregate,
		logicalRequestID,
		sequence,
		physicalCallID,
		content,
		reachedProviderLimit,
	)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestFragment{}, err
	}
	updatedEvent, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventCandidateRequestFragmentObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		LogicalRequestID: logicalRequestID, RequestFragment: &fragment,
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestFragment{}, err
	}
	return updatedEvent.Aggregate, fragment, nil
}

// persistRuntimeCandidateRequestDocument stores a complete raw document before
// the request-session controller marks it complete. It is intentionally not a
// candidate record: decoder/admission consume it only in C5 after C4 has
// committed the session transition.
func persistRuntimeCandidateRequestDocument(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtimev2.RuntimeAggregate, logicalRequestID string, sequence int, physicalCallID string, content []byte) (common.ArtifactContinuationID, string, error) {
	if err := runtimecontract.ValidateRuntimeCandidateRequestIdentity(logicalRequestID, sequence, physicalCallID); err != nil || len(content) == 0 {
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
	continuation.State = common.ArtifactContinuationStateApplied
	continuation.FinalReceived = true
	continuation.BytesWritten = int64(len(content))
	continuation.RollingHash = hash
	continuation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := repo.Update(ctx, continuation); err != nil {
		return "", "", err
	}
	return continuation.ID, hash, nil
}

// persistAndCompleteRuntimeCandidateRequestDocument is the equivalent C3
// bridge for an already complete raw document. The controller transition is
// deliberately separate from candidate admission: after this function
// returns, CandidateAttempts must still be empty and C5 is solely responsible
// for decoding/admitting the stored document.
func (m *Biz) persistAndCompleteRuntimeCandidateRequestDocument(ctx context.Context, aggregate runtimev2.RuntimeAggregate, logicalRequestID string, sequence int, physicalCallID string, content []byte) (runtimev2.RuntimeAggregate, common.ArtifactContinuationID, string, error) {
	if m == nil || m.bizPtr == nil || m.bizPtr.EvidenceRepositories().ArtifactContinuations == nil {
		return runtimev2.RuntimeAggregate{}, "", "", common.ErrInvalidArgument
	}
	id, hash, err := persistRuntimeCandidateRequestDocument(
		ctx,
		m.bizPtr.EvidenceRepositories().ArtifactContinuations,
		aggregate,
		logicalRequestID,
		sequence,
		physicalCallID,
		content,
	)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, "", "", err
	}
	updatedEvent, err := m.submitRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind: runtimev2.RuntimeInternalEventCandidateDocumentObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID,
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, "", "", err
	}
	return updatedEvent.Aggregate, id, hash, nil
}

// markRuntimeCandidateRequestContentFailed closes the opaque content carrier
// after its aggregate-owned request session has reached a terminal outcome.
// The carrier remains audit-only: it never chooses a retry or changes the
// session/candidate state machine.
func (m *Biz) markRuntimeCandidateRequestContentFailed(ctx context.Context, aggregate runtimev2.RuntimeAggregate, logicalRequestID string) error {
	if m == nil || m.bizPtr == nil || m.bizPtr.EvidenceRepositories().ArtifactContinuations == nil || strings.TrimSpace(logicalRequestID) == "" {
		return common.ErrInvalidArgument
	}
	repository := m.bizPtr.EvidenceRepositories().ArtifactContinuations
	id := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	content, err := repository.Get(ctx, id)
	if err != nil {
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
	switch content.State {
	case common.ArtifactContinuationStateFailed:
		return nil
	case common.ArtifactContinuationStateApplied:
		return fmt.Errorf("%w: complete candidate request content cannot become failed", common.ErrConflict)
	}
	content.State = common.ArtifactContinuationStateFailed
	content.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return repository.Update(ctx, content)
}

func loadRuntimeCandidateRequestDocument(ctx context.Context, repo repository.ArtifactContinuationRepository, logicalRequestID string) ([]byte, error) {
	if repo == nil || logicalRequestID == "" {
		return nil, common.ErrInvalidArgument
	}
	id := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	metadata, content, err := repo.GetAssembled(ctx, id)
	if err != nil {
		return nil, err
	}
	if metadata.RollingHash != runtimeCandidateOutputHash(content) || metadata.Bytes != int64(len(content)) {
		return nil, fmt.Errorf("%w: candidate request complete document hash is invalid", common.ErrConflict)
	}
	return []byte(content), nil
}

// loadCompletedRuntimeCandidateRequestDocument is the only C3/C5 read path
// for a raw request document. A chunk being durable is insufficient: the
// aggregate must first have committed document_complete for the exact logical
// session. This keeps a CAS-precrash fragment or an abandoned suffix out of
// the decoder/admission path.
func loadCompletedRuntimeCandidateRequestDocument(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtimev2.RuntimeAggregate, logicalRequestID string) ([]byte, error) {
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID != logicalRequestID {
			continue
		}
		if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete {
			return nil, fmt.Errorf("%w: runtime candidate request document is not complete", common.ErrConflict)
		}
		return loadRuntimeCandidateRequestDocument(ctx, repo, logicalRequestID)
	}
	return nil, fmt.Errorf("%w: runtime candidate request session not found", common.ErrNotFound)
}

// assembleRuntimeCandidateRequestFragments reconstructs bytes exclusively from
// fragment metadata already committed in the request session. It is used only
// to decide whether a suffix made the document structurally complete; it does
// not advance session state, allocate an attempt, or accept a candidate.
func assembleRuntimeCandidateRequestFragments(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtimev2.RuntimeAggregate, logicalRequestID string) ([]byte, error) {
	if repo == nil || strings.TrimSpace(logicalRequestID) == "" {
		return nil, common.ErrInvalidArgument
	}
	var session *runtimecontract.RuntimeCandidateRequestSession
	for idx := range aggregate.CandidateRequestSessions {
		if aggregate.CandidateRequestSessions[idx].LogicalRequestID == logicalRequestID {
			value := aggregate.CandidateRequestSessions[idx].Clone()
			session = &value
			break
		}
	}
	if session == nil || len(session.Fragments) == 0 {
		return nil, fmt.Errorf("%w: runtime candidate request has no committed fragments", common.ErrNotFound)
	}
	contentID := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	var builder strings.Builder
	for _, fragment := range session.Fragments {
		chunk, content, err := repo.GetChunk(ctx, contentID, fragment.Sequence)
		if err != nil {
			return nil, err
		}
		if chunk.ContentHash != fragment.ContentHash || chunk.Bytes != fragment.Bytes || runtimeCandidateOutputHash(content) != fragment.ContentHash || int64(len(content)) != fragment.Bytes || len(chunk.LLMCallRefs) != 1 || chunk.LLMCallRefs[0] != fragment.PhysicalCallID {
			return nil, fmt.Errorf("%w: runtime candidate request chunk does not match committed session metadata", common.ErrConflict)
		}
		builder.WriteString(content)
	}
	return []byte(builder.String()), nil
}

// prepareRuntimeCandidateRequestContinuation derives the next physical call
// only from a committed continuation-pending session and its opaque chunks.
// It neither sends the request nor changes aggregate state. C4 will call it
// after persisting MarkSubmitted; keeping it here makes sequence/tail recovery
// deterministic without turning ArtifactContinuation into a second state
// machine.
func prepareRuntimeCandidateRequestContinuation(ctx context.Context, repo repository.ArtifactContinuationRepository, aggregate runtimev2.RuntimeAggregate, logicalRequestID string, tailBytes int) (runtimecontract.RuntimeCandidateOutputContinuationRequest, string, error) {
	if repo == nil || strings.TrimSpace(logicalRequestID) == "" || tailBytes <= 0 {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", common.ErrInvalidArgument
	}
	var session *runtimecontract.RuntimeCandidateRequestSession
	for idx := range aggregate.CandidateRequestSessions {
		if aggregate.CandidateRequestSessions[idx].LogicalRequestID == logicalRequestID {
			value := aggregate.CandidateRequestSessions[idx].Clone()
			session = &value
			break
		}
	}
	if session == nil {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", fmt.Errorf("%w: runtime candidate request session not found", common.ErrNotFound)
	}
	if err := session.Validate(); err != nil {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", err
	}
	if (session.State != runtimecontract.RuntimeCandidateRequestSessionContinuationPending &&
		session.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending) || len(session.Fragments) == 0 {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", fmt.Errorf("%w: runtime candidate request does not have a committed continuation", common.ErrConflict)
	}
	contentID := runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
	lastSequence := session.NextSequence - 1
	var lastContent string
	for _, fragment := range session.Fragments {
		chunk, content, err := repo.GetChunk(ctx, contentID, fragment.Sequence)
		if err != nil {
			return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", err
		}
		if chunk.ContentHash != fragment.ContentHash || chunk.Bytes != fragment.Bytes || runtimeCandidateOutputHash(content) != fragment.ContentHash || len(content) != int(fragment.Bytes) || len(chunk.LLMCallRefs) != 1 || chunk.LLMCallRefs[0] != fragment.PhysicalCallID {
			return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", fmt.Errorf("%w: runtime candidate request chunk does not match committed session metadata", common.ErrConflict)
		}
		if fragment.Sequence == lastSequence {
			lastContent = content
		}
	}
	if lastContent == "" {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", fmt.Errorf("%w: runtime candidate request last continuation chunk is absent", common.ErrConflict)
	}
	request := runtimecontract.RuntimeCandidateOutputContinuationRequest{
		ID:                contentID,
		Sequence:          session.NextSequence,
		PreviousChunkTail: runtimeCandidateOutputTail(lastContent, tailBytes),
	}
	if err := request.Validate(); err != nil {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", err
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, session.NextSequence)
	if err := runtimecontract.ValidateRuntimeCandidateRequestIdentity(logicalRequestID, session.NextSequence, physicalCallID); err != nil {
		return runtimecontract.RuntimeCandidateOutputContinuationRequest{}, "", err
	}
	return request, physicalCallID, nil
}
