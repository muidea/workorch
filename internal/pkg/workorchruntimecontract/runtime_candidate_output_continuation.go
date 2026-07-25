package workorchruntimecontract

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCandidateOutputContinuationPolicy is part of the activation-sealed
// execution contract. MaxChunks counts provider calls after the initial
// truncated candidate response; zero disables continuation for that Run.
type RuntimeCandidateOutputContinuationPolicy struct {
	MaxChunks int
	TailBytes int
}

func (p RuntimeCandidateOutputContinuationPolicy) Normalized() RuntimeCandidateOutputContinuationPolicy {
	if p.MaxChunks < 0 {
		p.MaxChunks = 0
	}
	if p.TailBytes < 0 {
		p.TailBytes = 0
	}
	return p
}

func (p RuntimeCandidateOutputContinuationPolicy) Validate() error {
	p = p.Normalized()
	if p.MaxChunks > 64 {
		return fmt.Errorf("runtime candidate output continuation max chunks must be in 0..64")
	}
	if p.TailBytes > 8192 {
		return fmt.Errorf("runtime candidate output continuation tail bytes must be in 0..8192")
	}
	if p.MaxChunks > 0 && p.TailBytes <= 0 {
		return fmt.Errorf("runtime candidate output continuation tail bytes are required when enabled")
	}
	return nil
}

// RuntimeCandidateOutputContinuationRequest is the typed, Orchestrator-sealed
// request context for one suffix call. The LLM owner may only return text; it
// never selects the sequence, budget, or continuation identity.
type RuntimeCandidateOutputContinuationRequest struct {
	ID                common.ArtifactContinuationID
	Sequence          int
	PreviousChunkTail string
}

func (r RuntimeCandidateOutputContinuationRequest) Normalized() RuntimeCandidateOutputContinuationRequest {
	return r
}

func (r RuntimeCandidateOutputContinuationRequest) Validate() error {
	r = r.Normalized()
	if r.ID == "" || r.Sequence < 2 || r.PreviousChunkTail == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

// RuntimeCandidateOutputFragment is returned by the LLM owner only when the
// payload is an incomplete candidate document (initial response) or a raw
// suffix (continuation response). Content is persisted by Orchestrator before
// another suffix is requested.
type RuntimeCandidateOutputFragment struct {
	Content              []byte
	ReachedProviderLimit bool
}

func (f RuntimeCandidateOutputFragment) Validate() error {
	if len(f.Content) == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}
