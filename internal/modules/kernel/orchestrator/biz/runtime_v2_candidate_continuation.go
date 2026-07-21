package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeCandidateOutputContinuationMode = "runtime_candidate_output"

// continueRuntimeV2CandidateOutput implements the bounded SM-3 subprotocol
// used only after a provider proves that a structured candidate reached its
// sealed single-response limit. It persists every raw fragment before asking
// for a suffix, so restart never relies on process memory or repeats a known
// chunk. It does not choose a new runtime action.
func (m *Biz) continueRuntimeV2CandidateOutput(ctx context.Context, aggregate runtimev2.RuntimeAggregate, stablePrefixHash string, attempt int, initial runtimecontract.RuntimeCandidateOutputFragment) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	policy := aggregate.Facts.Activation.ExecutionContract.Normalized().CandidateOutputContinuation
	if err := policy.Validate(); err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if policy.MaxChunks == 0 {
		return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_disabled", "provider candidate output reached its sealed limit and this Run has no continuation budget")
	}
	repo := m.bizPtr.EvidenceRepositories().ArtifactContinuations
	if repo == nil {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("runtime candidate output continuation repository is not configured")
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	id := runtimecontract.StableRuntimeCandidateOutputContinuationID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction, attempt)
	continuation, err := loadOrCreateRuntimeCandidateOutputContinuation(ctx, repo, id, aggregate, attempt, stablePrefixHash, policy)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if len(initial.Content) > 0 {
		if err := recordRuntimeCandidateOutputChunk(ctx, repo, continuation, 1, initial.Content, ""); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
	}
	continuation, assembled, err := reconcileRuntimeCandidateOutputContinuation(ctx, repo, continuation)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if continuation.FinalReceived {
		_, stored, storedErr := repo.GetAssembled(ctx, continuation.ID)
		if storedErr != nil {
			return runtimev2.RuntimeAggregate{}, storedErr
		}
		candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayload(aggregate.Decision.Action, []byte(stored))
		if decodeErr != nil {
			return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_assembly_invalid", decodeErr.Error())
		}
		proposal, proposalErr := m.bizPtr.AdmitRuntimeV2CandidateProposal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt, candidate)
		if proposalErr != nil {
			return runtimev2.RuntimeAggregate{}, proposalErr
		}
		return proposal.Aggregate, nil
	}
	for continuation.ExpectedSequence-1 <= policy.MaxChunks {
		sequence := continuation.ExpectedSequence
		response, requestErr := RequestRuntimeCandidate(ctx, m.EventHub(), events.RequestRuntimeCandidateCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Decision: aggregate.Decision, Transaction: transaction,
			CandidateAttempt: attempt, StablePrefixHash: stablePrefixHash, DefinitionContext: aggregate.Definition,
			CandidateContext:   runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, runtimev2.ActiveRejectedOperationShapes(aggregate)),
			OutputContinuation: &runtimecontract.RuntimeCandidateOutputContinuationRequest{ID: id, Sequence: sequence, PreviousChunkTail: continuation.PreviousChunkTail},
		})
		if requestErr != nil {
			return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_request_failed", requestErr.Error())
		}
		if response.Transaction != transaction {
			return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_transaction_mismatch", "candidate output continuation changed the sealed authority transaction")
		}
		if response.Stale != nil {
			return m.bizPtr.InvalidateRuntimeV2CandidateAttemptForPrerequisiteStale(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt, *response.Stale)
		}
		if response.Failure != nil {
			return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_failed", response.Failure.Reason)
		}
		if response.OutputFragment == nil {
			return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_protocol_invalid", "continuation response did not contain a raw output fragment")
		}
		if err := recordRuntimeCandidateOutputChunk(ctx, repo, continuation, sequence, response.OutputFragment.Content, continuation.PreviousChunkTail); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		continuation, assembled, err = reconcileRuntimeCandidateOutputContinuation(ctx, repo, continuation)
		if err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayload(aggregate.Decision.Action, []byte(assembled))
		if decodeErr != nil && response.OutputFragment.ReachedProviderLimit {
			continue
		}
		if decodeErr != nil {
			return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_assembly_invalid", decodeErr.Error())
		}
		metadata := &runtimecontract.ArtifactContinuationAssembledMetadata{ContinuationID: id, TargetRef: "runtime_candidate", Bytes: int64(len(assembled)), RollingHash: continuation.RollingHash, SourceChunkRefs: runtimeCandidateOutputChunkRefs(continuation.ReceivedSequences), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if err := repo.SaveAssembled(ctx, metadata, assembled); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		continuation.State = common.ArtifactContinuationStateApplied
		continuation.FinalReceived = true
		continuation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := repo.Update(ctx, continuation); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		proposal, proposalErr := m.bizPtr.AdmitRuntimeV2CandidateProposal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt, candidate)
		if proposalErr != nil {
			return runtimev2.RuntimeAggregate{}, proposalErr
		}
		return proposal.Aggregate, nil
	}
	return m.failRuntimeV2CandidateOutputContinuation(ctx, aggregate, attempt, "candidate_output_continuation_exhausted", "candidate output continuation exhausted its activation-sealed chunk budget")
}

func (m *Biz) openRuntimeV2CandidateOutputContinuation(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, prefix string) (runtimecontract.RuntimeCandidateOutputFragment, bool, error) {
	if m == nil || m.bizPtr == nil {
		return runtimecontract.RuntimeCandidateOutputFragment{}, false, common.ErrInvalidArgument
	}
	repo := m.bizPtr.EvidenceRepositories().ArtifactContinuations
	if repo == nil {
		return runtimecontract.RuntimeCandidateOutputFragment{}, false, nil
	}
	id := runtimecontract.StableRuntimeCandidateOutputContinuationID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction(), attempt)
	continuation, err := repo.Get(ctx, id)
	if errors.Is(err, common.ErrNotFound) {
		return runtimecontract.RuntimeCandidateOutputFragment{}, false, nil
	}
	if err != nil {
		return runtimecontract.RuntimeCandidateOutputFragment{}, false, err
	}
	policy := aggregate.Facts.Activation.ExecutionContract.Normalized().CandidateOutputContinuation
	if continuation.OutputMode != runtimeCandidateOutputContinuationMode || continuation.AuthorityTransaction != aggregate.Decision.AuthorityTransaction() || continuation.CandidateAttempt != attempt || continuation.StablePrefixHash != prefix || continuation.MaxChunks != policy.MaxChunks || continuation.TailBytes != policy.TailBytes {
		return runtimecontract.RuntimeCandidateOutputFragment{}, false, fmt.Errorf("%w: persisted candidate output continuation does not match current authority", common.ErrConflict)
	}
	return runtimecontract.RuntimeCandidateOutputFragment{}, true, nil
}

func (m *Biz) failRuntimeV2CandidateOutputContinuation(ctx context.Context, aggregate runtimev2.RuntimeAggregate, attempt int, code, message string) (runtimev2.RuntimeAggregate, error) {
	// This is a distinct, bounded continuation budget, not a candidate attempt
	// 0/1 retry. Terminalizing directly prevents a repeated truncated document
	// from being mislabeled as an actionable alternate candidate shape.
	if repo := m.bizPtr.EvidenceRepositories().ArtifactContinuations; repo != nil {
		id := runtimecontract.StableRuntimeCandidateOutputContinuationID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction(), attempt)
		if continuation, err := repo.Get(ctx, id); err == nil {
			continuation.State = common.ArtifactContinuationStateFailed
			continuation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if updateErr := repo.Update(ctx, continuation); updateErr != nil {
				return runtimev2.RuntimeAggregate{}, updateErr
			}
		} else if !errors.Is(err, common.ErrNotFound) {
			return runtimev2.RuntimeAggregate{}, err
		}
	}
	return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateOutputExhausted, Code: code, Signature: strings.ReplaceAll(code, "_", "-"), Message: strings.TrimSpace(message)})
}

func loadOrCreateRuntimeCandidateOutputContinuation(ctx context.Context, repo repository.ArtifactContinuationRepository, id common.ArtifactContinuationID, aggregate runtimev2.RuntimeAggregate, attempt int, prefix string, policy runtimecontract.RuntimeCandidateOutputContinuationPolicy) (*runtimecontract.ArtifactContinuation, error) {
	value, err := repo.Get(ctx, id)
	if err == nil {
		if value.WorkspaceID != aggregate.Facts.WorkspaceID || value.RunID != aggregate.Facts.RunID || value.OutputMode != runtimeCandidateOutputContinuationMode || value.AuthorityTransaction != aggregate.Decision.AuthorityTransaction() || value.CandidateAttempt != attempt || value.StablePrefixHash != prefix || value.MaxChunks != policy.MaxChunks || value.TailBytes != policy.TailBytes {
			return nil, fmt.Errorf("%w: persisted candidate output continuation does not match sealed authority", common.ErrConflict)
		}
		return value, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}
	value = &runtimecontract.ArtifactContinuation{ID: id, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, ArtifactID: "runtime-candidate", TargetRef: "runtime_candidate", Operation: "candidate_generation", OutputMode: runtimeCandidateOutputContinuationMode, State: common.ArtifactContinuationStateChunking, ExpectedSequence: 1, ReceivedChunkHashes: map[int]string{}, AuthorityTransaction: aggregate.Decision.AuthorityTransaction(), CandidateAttempt: attempt, StablePrefixHash: prefix, MaxChunks: policy.MaxChunks, TailBytes: policy.TailBytes, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := repo.Create(ctx, value); err != nil {
		if !errors.Is(err, common.ErrConflict) {
			return nil, err
		}
		return repo.Get(ctx, id)
	}
	return value, nil
}

func recordRuntimeCandidateOutputChunk(ctx context.Context, repo repository.ArtifactContinuationRepository, continuation *runtimecontract.ArtifactContinuation, sequence int, content []byte, previousTail string) error {
	if continuation == nil || sequence <= 0 || len(content) == 0 {
		return common.ErrInvalidArgument
	}
	contentString := string(content)
	hash := runtimeCandidateOutputHash(contentString)
	if existing, existingContent, err := repo.GetChunk(ctx, continuation.ID, sequence); err == nil {
		if existing.ContentHash != hash || existingContent != contentString || existing.PreviousChunkTail != previousTail {
			return fmt.Errorf("%w: candidate output continuation duplicate chunk conflicts at sequence %d", common.ErrConflict, sequence)
		}
		return nil
	} else if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return repo.SaveChunk(ctx, &runtimecontract.ArtifactContinuationChunk{ContinuationID: continuation.ID, Sequence: sequence, ArtifactID: continuation.ArtifactID, TargetRef: continuation.TargetRef, ContentHash: hash, Bytes: int64(len(content)), PreviousChunkTail: previousTail, CurrentChunkTail: runtimeCandidateOutputTail(contentString, continuation.TailBytes), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, contentString)
}

func reconcileRuntimeCandidateOutputContinuation(ctx context.Context, repo repository.ArtifactContinuationRepository, continuation *runtimecontract.ArtifactContinuation) (*runtimecontract.ArtifactContinuation, string, error) {
	chunks, err := repo.ListChunks(ctx, continuation.ID)
	if err != nil {
		return nil, "", err
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].Sequence < chunks[j].Sequence })
	var builder strings.Builder
	hashes := map[int]string{}
	sequences := make([]int, 0, len(chunks))
	previousTail := ""
	for expected, idx := 1, 0; idx < len(chunks); expected, idx = expected+1, idx+1 {
		chunk := chunks[idx]
		if chunk.Sequence != expected || chunk.PreviousChunkTail != previousTail {
			return nil, "", fmt.Errorf("%w: candidate output continuation chunk sequence or tail is invalid", common.ErrConflict)
		}
		_, content, getErr := repo.GetChunk(ctx, continuation.ID, expected)
		if getErr != nil {
			return nil, "", getErr
		}
		if chunk.ContentHash != runtimeCandidateOutputHash(content) {
			return nil, "", fmt.Errorf("%w: candidate output continuation chunk hash is invalid", common.ErrConflict)
		}
		builder.WriteString(content)
		hashes[expected] = chunk.ContentHash
		sequences = append(sequences, expected)
		previousTail = chunk.CurrentChunkTail
	}
	assembled := builder.String()
	continuation.ExpectedSequence = len(chunks) + 1
	continuation.ReceivedSequences = sequences
	continuation.ReceivedChunkHashes = hashes
	continuation.BytesWritten = int64(len(assembled))
	continuation.RollingHash = runtimeCandidateOutputHash(assembled)
	continuation.LastChunkHash = hashes[len(chunks)]
	continuation.PreviousChunkTail = previousTail
	continuation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := repo.Update(ctx, continuation); err != nil {
		return nil, "", err
	}
	return continuation, assembled, nil
}

func runtimeCandidateOutputHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func runtimeCandidateOutputTail(value string, max int) string {
	if max <= 0 || len(value) == 0 {
		return ""
	}
	if len(value) > max {
		return value[len(value)-max:]
	}
	return value
}
func runtimeCandidateOutputChunkRefs(sequences []int) []string {
	ret := make([]string, 0, len(sequences))
	for _, sequence := range sequences {
		ret = append(ret, fmt.Sprintf("chunk/%d", sequence))
	}
	return ret
}
