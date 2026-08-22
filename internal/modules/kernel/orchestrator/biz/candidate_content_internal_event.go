package orchestrator

import (
	"context"
	"errors"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type candidateContentInternalEventKind string

const (
	candidateContentInternalEventCreated candidateContentInternalEventKind = "created"
	candidateContentInternalEventApplied candidateContentInternalEventKind = "applied"
	candidateContentInternalEventFailed  candidateContentInternalEventKind = "failed"
)

type candidateContentCreatedFact struct {
	CreatedAt string
	UpdatedAt string
}

type candidateContentAppliedFact struct {
	BytesWritten int64
	RollingHash  string
	UpdatedAt    string
}

type candidateContentFailedFact struct {
	UpdatedAt string
}

// candidateContentInternalEvent controls only the Orchestrator-owned opaque
// content carrier. The Runtime request session remains authoritative in SM-3;
// this local event cannot choose request recovery, candidate admission, or a
// Runtime transition.
type candidateContentInternalEvent struct {
	Kind                 candidateContentInternalEventKind
	ContinuationID       common.ArtifactContinuationID
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	LogicalRequestID     string
	AuthorityTransaction runtimecontract.AuthorityTransaction

	Created *candidateContentCreatedFact
	Applied *candidateContentAppliedFact
	Failed  *candidateContentFailedFact
}

type candidateContentTransition struct {
	Next *runtimecontract.ArtifactContinuation
	Noop bool
}

func normalizeCandidateContentInternalEvent(event candidateContentInternalEvent) (candidateContentInternalEvent, error) {
	event.LogicalRequestID = strings.TrimSpace(event.LogicalRequestID)
	if event.ContinuationID == "" || event.WorkspaceID == "" || event.RunID == "" || event.LogicalRequestID == "" || event.ContinuationID != runtimecontract.StableRuntimeCandidateRequestContentID(event.LogicalRequestID) || event.AuthorityTransaction.AuthoritySnapshotID == "" {
		return candidateContentInternalEvent{}, common.ErrInvalidArgument
	}
	payloadCount := 0
	for _, present := range []bool{event.Created != nil, event.Applied != nil, event.Failed != nil} {
		if present {
			payloadCount++
		}
	}
	if payloadCount != 1 {
		return candidateContentInternalEvent{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case candidateContentInternalEventCreated:
		if event.Created == nil || strings.TrimSpace(event.Created.CreatedAt) == "" || strings.TrimSpace(event.Created.UpdatedAt) == "" {
			return candidateContentInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.Created
		fact.CreatedAt = strings.TrimSpace(fact.CreatedAt)
		fact.UpdatedAt = strings.TrimSpace(fact.UpdatedAt)
		event.Created = &fact
	case candidateContentInternalEventApplied:
		if event.Applied == nil || event.Applied.BytesWritten <= 0 || strings.TrimSpace(event.Applied.RollingHash) == "" || strings.TrimSpace(event.Applied.UpdatedAt) == "" {
			return candidateContentInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.Applied
		fact.RollingHash = strings.TrimSpace(fact.RollingHash)
		fact.UpdatedAt = strings.TrimSpace(fact.UpdatedAt)
		event.Applied = &fact
	case candidateContentInternalEventFailed:
		if event.Failed == nil || strings.TrimSpace(event.Failed.UpdatedAt) == "" {
			return candidateContentInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.Failed
		fact.UpdatedAt = strings.TrimSpace(fact.UpdatedAt)
		event.Failed = &fact
	default:
		return candidateContentInternalEvent{}, common.ErrInvalidArgument
	}
	return event, nil
}

func reduceCandidateContentInternalEvent(current *runtimecontract.ArtifactContinuation, event candidateContentInternalEvent) (candidateContentTransition, error) {
	if current != nil && !candidateContentMatchesEvent(current, event) {
		return candidateContentTransition{}, common.ErrConflict
	}
	switch event.Kind {
	case candidateContentInternalEventCreated:
		if current != nil {
			return candidateContentTransition{Next: cloneCandidateContent(current), Noop: true}, nil
		}
		return candidateContentTransition{Next: &runtimecontract.ArtifactContinuation{
			ID: event.ContinuationID, WorkspaceID: event.WorkspaceID, RunID: event.RunID,
			ArtifactID: event.LogicalRequestID, TargetRef: runtimeCandidateRequestContentTargetRef,
			Operation: "candidate_request_content", OutputMode: runtimeCandidateRequestContentMode,
			AuthorityTransaction: event.AuthorityTransaction, State: common.ArtifactContinuationStateChunking,
			ExpectedSequence: 1, ReceivedChunkHashes: map[int]string{},
			CreatedAt: event.Created.CreatedAt, UpdatedAt: event.Created.UpdatedAt,
		}}, nil
	case candidateContentInternalEventApplied:
		if current == nil {
			return candidateContentTransition{}, common.ErrNotFound
		}
		switch current.State {
		case common.ArtifactContinuationStateApplied:
			if current.FinalReceived && current.BytesWritten == event.Applied.BytesWritten && current.RollingHash == event.Applied.RollingHash {
				return candidateContentTransition{Next: cloneCandidateContent(current), Noop: true}, nil
			}
			return candidateContentTransition{}, common.ErrConflict
		case common.ArtifactContinuationStateFailed:
			return candidateContentTransition{}, common.ErrConflict
		case common.ArtifactContinuationStateChunking:
		default:
			return candidateContentTransition{}, common.ErrConflict
		}
		next := cloneCandidateContent(current)
		next.State = common.ArtifactContinuationStateApplied
		next.FinalReceived = true
		next.BytesWritten = event.Applied.BytesWritten
		next.RollingHash = event.Applied.RollingHash
		next.UpdatedAt = event.Applied.UpdatedAt
		return candidateContentTransition{Next: next}, nil
	case candidateContentInternalEventFailed:
		if current == nil {
			return candidateContentTransition{}, common.ErrNotFound
		}
		switch current.State {
		case common.ArtifactContinuationStateFailed:
			return candidateContentTransition{Next: cloneCandidateContent(current), Noop: true}, nil
		case common.ArtifactContinuationStateApplied:
			return candidateContentTransition{}, common.ErrConflict
		case common.ArtifactContinuationStateChunking:
		default:
			return candidateContentTransition{}, common.ErrConflict
		}
		next := cloneCandidateContent(current)
		next.State = common.ArtifactContinuationStateFailed
		next.UpdatedAt = event.Failed.UpdatedAt
		return candidateContentTransition{Next: next}, nil
	default:
		return candidateContentTransition{}, common.ErrInvalidArgument
	}
}

func candidateContentMatchesEvent(current *runtimecontract.ArtifactContinuation, event candidateContentInternalEvent) bool {
	return current != nil && current.ID == event.ContinuationID && current.WorkspaceID == event.WorkspaceID && current.RunID == event.RunID && current.ArtifactID == event.LogicalRequestID && current.TargetRef == runtimeCandidateRequestContentTargetRef && current.OutputMode == runtimeCandidateRequestContentMode && current.AuthorityTransaction == event.AuthorityTransaction && current.CandidateAttempt == 0
}

func cloneCandidateContent(value *runtimecontract.ArtifactContinuation) *runtimecontract.ArtifactContinuation {
	if value == nil {
		return nil
	}
	next := *value
	next.CoverageTargets = append([]string(nil), value.CoverageTargets...)
	next.ReceivedSequences = append([]int(nil), value.ReceivedSequences...)
	next.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	if value.ReceivedChunkHashes != nil {
		next.ReceivedChunkHashes = make(map[int]string, len(value.ReceivedChunkHashes))
		for sequence, hash := range value.ReceivedChunkHashes {
			next.ReceivedChunkHashes[sequence] = hash
		}
	}
	if value.RetryCounters != nil {
		next.RetryCounters = make(map[string]int, len(value.RetryCounters))
		for key, count := range value.RetryCounters {
			next.RetryCounters[key] = count
		}
	}
	return &next
}

func submitCandidateContentInternalEvent(ctx context.Context, repo repository.ArtifactContinuationRepository, event candidateContentInternalEvent) (*runtimecontract.ArtifactContinuation, bool, error) {
	if repo == nil {
		return nil, false, common.ErrInvalidArgument
	}
	event, err := normalizeCandidateContentInternalEvent(event)
	if err != nil {
		return nil, false, err
	}
	current, err := repo.Get(ctx, event.ContinuationID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return nil, false, err
	}
	if errors.Is(err, common.ErrNotFound) {
		current = nil
	}
	transition, err := reduceCandidateContentInternalEvent(current, event)
	if err != nil {
		return nil, false, err
	}
	if transition.Noop {
		return cloneCandidateContent(transition.Next), false, nil
	}
	if transition.Next == nil {
		return nil, false, common.ErrInvalidArgument
	}
	if current == nil {
		if err := repo.Create(ctx, transition.Next); err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return nil, false, err
			}
			current, err = repo.Get(ctx, event.ContinuationID)
			if err != nil || !candidateContentMatchesEvent(current, event) {
				if err != nil {
					return nil, false, err
				}
				return nil, false, common.ErrConflict
			}
			return cloneCandidateContent(current), false, nil
		}
		return cloneCandidateContent(transition.Next), true, nil
	}
	if err := repo.Update(ctx, transition.Next); err != nil {
		return nil, false, err
	}
	return cloneCandidateContent(transition.Next), true, nil
}
