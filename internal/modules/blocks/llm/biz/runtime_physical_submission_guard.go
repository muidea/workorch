package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	llmruntime "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtime"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The durable provider-submitted marker remains in the Orchestrator-owned
// Runtime aggregate. This bounded owner-local guard only closes the live
// process window in which duplicate/concurrent delivery of the same physical
// identity could otherwise invoke the provider twice.
const (
	defaultRuntimePhysicalSubmissionGuardLimit = 16 * 1024
	maxRuntimePhysicalSubmissionIdentityBytes  = 1024
)

type runtimePhysicalSubmissionIdentity struct {
	WorkspaceID      string
	RunID            string
	LogicalRequestID string
	Sequence         int
	PhysicalCallID   string
}

func runtimePhysicalSubmissionIdentityFromRequest(request llmmodels.RuntimeCandidateRequest) (runtimePhysicalSubmissionIdentity, error) {
	workspaceID, runID := runtimeRequestScope(request)
	identity := request.RequestIdentity.Normalized()
	ret := runtimePhysicalSubmissionIdentity{
		WorkspaceID:      strings.TrimSpace(workspaceID),
		RunID:            strings.TrimSpace(runID),
		LogicalRequestID: identity.LogicalRequestID,
		Sequence:         identity.Sequence,
		PhysicalCallID:   identity.PhysicalCallID,
	}
	if ret.WorkspaceID == "" || ret.RunID == "" ||
		len(ret.WorkspaceID)+len(ret.RunID)+len(ret.LogicalRequestID)+len(ret.PhysicalCallID) > maxRuntimePhysicalSubmissionIdentityBytes {
		return runtimePhysicalSubmissionIdentity{}, common.ErrInvalidArgument
	}
	if err := identity.Validate(); err != nil {
		return runtimePhysicalSubmissionIdentity{}, err
	}
	return ret, nil
}

type runtimePhysicalSubmissionGuard struct {
	mu      sync.Mutex
	limit   int
	entries map[string]runtimePhysicalSubmissionIdentity
}

func newRuntimePhysicalSubmissionGuard(limit int) *runtimePhysicalSubmissionGuard {
	if limit <= 0 {
		limit = defaultRuntimePhysicalSubmissionGuardLimit
	}
	return &runtimePhysicalSubmissionGuard{
		limit:   limit,
		entries: make(map[string]runtimePhysicalSubmissionIdentity),
	}
}

type runtimePhysicalSubmissionGuardError struct {
	reason string
}

func (e *runtimePhysicalSubmissionGuardError) Error() string {
	if e == nil || strings.TrimSpace(e.reason) == "" {
		return "runtime physical submission rejected"
	}
	return "runtime physical submission rejected: " + strings.TrimSpace(e.reason)
}

func (e *runtimePhysicalSubmissionGuardError) Unwrap() error { return common.ErrConflict }

func isRuntimePhysicalSubmissionGuardError(err error) bool {
	var target *runtimePhysicalSubmissionGuardError
	return errors.As(err, &target)
}

func (g *runtimePhysicalSubmissionGuard) reserve(identity runtimePhysicalSubmissionIdentity) error {
	if g == nil || identity.PhysicalCallID == "" || identity.LogicalRequestID == "" || identity.Sequence <= 0 || identity.WorkspaceID == "" || identity.RunID == "" {
		return &runtimePhysicalSubmissionGuardError{reason: "guard or request identity is unavailable"}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.entries == nil {
		return &runtimePhysicalSubmissionGuardError{reason: "guard is not active"}
	}
	if current, ok := g.entries[identity.PhysicalCallID]; ok {
		if current != identity {
			return &runtimePhysicalSubmissionGuardError{reason: "physical call identity was reused with different scope or lineage"}
		}
		return &runtimePhysicalSubmissionGuardError{reason: "physical call identity was already reserved or submitted"}
	}
	if len(g.entries) >= g.limit {
		return &runtimePhysicalSubmissionGuardError{reason: "bounded guard capacity is exhausted"}
	}
	g.entries[identity.PhysicalCallID] = identity
	return nil
}

// releaseIfProvenUnsubmitted removes only an exact, canonical adapter fact
// proving that no provider submission happened. Submitted, malformed, missing,
// or contradictory observations remain fenced until Teardown. That conservative
// retention is deliberate: this cache is a live-effect guard, not recovery
// authority and never authorizes a replay.
func (g *runtimePhysicalSubmissionGuard) releaseIfProvenUnsubmitted(identity runtimePhysicalSubmissionIdentity, generation llmmodels.RuntimeCandidateGenerationResult) {
	if g == nil || !runtimeGenerationProvesUnsubmitted(identity, generation) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if current, ok := g.entries[identity.PhysicalCallID]; ok && current == identity {
		delete(g.entries, identity.PhysicalCallID)
	}
}

func runtimeGenerationProvesUnsubmitted(identity runtimePhysicalSubmissionIdentity, generation llmmodels.RuntimeCandidateGenerationResult) bool {
	audit := generation.Audit
	if audit == nil || generation.Usage != nil || generation.ReachedProviderLimit ||
		strings.TrimSpace(generation.Status) != llmmodels.RuntimeCandidateStatusFailed || strings.TrimSpace(generation.FailureClass) == "" ||
		strings.TrimSpace(audit.Status) != llmmodels.RuntimeCandidateStatusFailed || strings.TrimSpace(audit.FailureClass) != strings.TrimSpace(generation.FailureClass) ||
		audit.ProviderSubmitted || audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming ||
		audit.ResponseBytes != 0 ||
		audit.ContextBudget == nil || audit.ContextBudget.ProviderSubmitted || audit.ContextBudget.ActualTokens != 0 {
		return false
	}
	return strings.TrimSpace(audit.WorkspaceID) == identity.WorkspaceID &&
		strings.TrimSpace(audit.RunID) == identity.RunID &&
		strings.TrimSpace(audit.LogicalRequestID) == identity.LogicalRequestID &&
		strings.TrimSpace(audit.PhysicalCallID) == identity.PhysicalCallID &&
		strings.TrimSpace(audit.RequestID) == identity.PhysicalCallID
}

func (g *runtimePhysicalSubmissionGuard) clear() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entries = make(map[string]runtimePhysicalSubmissionIdentity)
}

type runtimePhysicalSubmissionGuardedProvider struct {
	guard    *runtimePhysicalSubmissionGuard
	delegate llmruntime.CandidateProvider
}

func (p runtimePhysicalSubmissionGuardedProvider) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) (generation llmmodels.RuntimeCandidateGenerationResult, err error) {
	if p.delegate == nil || p.guard == nil {
		return llmmodels.RuntimeCandidateGenerationResult{}, &runtimePhysicalSubmissionGuardError{reason: "provider or guard is unavailable"}
	}
	identity, identityErr := runtimePhysicalSubmissionIdentityFromRequest(request)
	if identityErr != nil {
		return llmmodels.RuntimeCandidateGenerationResult{}, &runtimePhysicalSubmissionGuardError{reason: fmt.Sprintf("invalid request identity: %v", identityErr)}
	}
	if reserveErr := p.guard.reserve(identity); reserveErr != nil {
		return llmmodels.RuntimeCandidateGenerationResult{}, reserveErr
	}
	defer func() {
		p.guard.releaseIfProvenUnsubmitted(identity, generation)
	}()
	return p.delegate.GenerateCandidate(ctx, request)
}

func runtimeCommandHasExactOrchestratorSource(event interface{ Source() string }) bool {
	return event != nil && event.Source() == runtimecontract.RuntimeModuleOrchestrator
}
