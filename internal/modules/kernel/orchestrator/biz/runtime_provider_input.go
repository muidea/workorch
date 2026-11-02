package orchestrator

import (
	"context"
	"errors"
	"fmt"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func providerInputContentKey(logical string) string { return "provider-input/" + logical }

type runtimeProviderInputError struct {
	diagnostic *rc.RuntimeOriginalDiagnostic
}

func (e *runtimeProviderInputError) Error() string {
	return "provider_request_input_invalid: " + e.diagnostic.OriginalMessage
}

// Persist exact LLM-rendered input before session CAS and physical submission.
// Recovery reads this owner-scoped opaque carrier; it never reconstructs a
// request from current files, current budgets or newly deployed built-ins.
func (m *Biz) prepareRuntimeProviderInput(ctx context.Context, a runtime.RuntimeAggregate, logical string, expected rc.RuntimeExpectedDocumentContract, command llmevents.PreflightRuntimeCandidateCommand) ([]byte, *rc.RuntimeProviderInputRef, error) {
	repo := m.evidence.ArtifactContinuations
	if repo == nil {
		return nil, nil, fmt.Errorf("provider_request_input_store_unavailable")
	}
	id := rc.StableRuntimeCandidateRequestContentID(providerInputContentKey(logical))
	metadata, content, loadErr := repo.GetAssembled(ctx, id)
	if loadErr == nil {
		carrier, err := repo.Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if !candidateContentMatchesEvent(carrier, candidateContentInternalEvent{ContinuationID: id, WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, LogicalRequestID: providerInputContentKey(logical), AuthorityTransaction: a.Decision.AuthorityTransaction()}) || metadata.ContinuationID != id || metadata.TargetRef != carrier.TargetRef || metadata.Bytes != int64(len(content)) || metadata.RollingHash != runtimeCandidateOutputHash(content) {
			return nil, nil, fmt.Errorf("provider_request_input_corrupt")
		}
		raw := []byte(content)
		ref := &rc.RuntimeProviderInputRef{Hash: rc.RuntimeProviderInputHash(raw), Bytes: len(raw)}
		if session, found := runtimeCandidateRequestSession(a, logical); found {
			if err := session.InputRef.ValidateContent(raw); err != nil {
				return nil, nil, fmt.Errorf("provider_request_input_corrupt: %w", err)
			}
		}
		// A crash may follow SaveAssembled but precede the carrier transition.
		// Complete only that original transaction; never re-render its bytes.
		if err := m.persistRuntimeProviderInput(ctx, a, logical, raw); err != nil {
			return nil, nil, err
		}
		return raw, ref, nil
	}
	if !errors.Is(loadErr, common.ErrNotFound) {
		return nil, nil, loadErr
	}
	if _, found := runtimeCandidateRequestSession(a, logical); found {
		return nil, nil, fmt.Errorf("provider_request_input_unavailable: original frozen payload missing")
	}
	if command.CandidateAttempt == 1 {
		var base *rc.RuntimeCandidateRequestSession
		for _, candidate := range a.CandidateRequestSessions {
			if candidate.DocumentKind == command.DocumentKind && candidate.ManifestHash == expected.ManifestHash && candidate.StablePrefixHash == expected.StablePrefixHash && candidate.Transaction == a.Decision.AuthorityTransaction() && candidate.ExpectedDocument != nil && candidate.ExpectedDocument.CorrectionAttempt == 0 && candidate.ExpectedDocument.PlanStep == expected.PlanStep && candidate.ExpectedDocument.BatchID == expected.BatchID {
				if base != nil {
					return nil, nil, fmt.Errorf("provider_request_correction_base_ambiguous")
				}
				copy := candidate.Clone()
				base = &copy
			}
		}
		if base == nil {
			return nil, nil, fmt.Errorf("provider_request_correction_base_unavailable")
		}
		baseRaw, baseRef, baseErr := m.prepareRuntimeProviderInput(ctx, a, base.LogicalRequestID, *base.ExpectedDocument, llmevents.PreflightRuntimeCandidateCommand{})
		if baseErr != nil {
			return nil, nil, baseErr
		}
		command.BaseFrozenInput = baseRaw
		command.BaseFrozenInputRef = baseRef
	}
	command.FreezeInput = true
	command.LogicalRequestID = logical
	command.ExpectedDocument = &expected
	preflight, err := PreflightRuntimeCandidate(ctx, m.EventHub(), command)
	if err != nil {
		return nil, nil, fmt.Errorf("provider_request_input_preflight: %w", err)
	}
	if preflight.Feedback.StableCategory == rc.RuntimeFeedbackCategoryContextIntegrity {
		if preflight.Feedback.OriginalDiagnostic == nil {
			return nil, nil, fmt.Errorf("provider_request_input_invalid: missing original diagnostic")
		}
		return nil, nil, &runtimeProviderInputError{diagnostic: preflight.Feedback.OriginalDiagnostic.CloneOriginal()}
	}
	if err := preflight.FrozenInputRef.ValidateContent(preflight.FrozenInput); err != nil {
		return nil, nil, fmt.Errorf("provider_request_input_unavailable: %w", err)
	}
	if err = m.persistRuntimeProviderInput(ctx, a, logical, preflight.FrozenInput); err != nil {
		return nil, nil, fmt.Errorf("provider_request_input_persist: %w", err)
	}
	return preflight.FrozenInput, preflight.FrozenInputRef.Clone(), nil
}

func (m *Biz) persistRuntimeProviderInput(ctx context.Context, a runtime.RuntimeAggregate, logical string, raw []byte) error {
	return m.withOrchestratorEvidenceMutation(ctx, a.Facts.WorkspaceID, func(guarded context.Context) error {
		_, _, err := persistRuntimeCandidateOpaqueContent(guarded, m.evidence.ArtifactContinuations, a, providerInputContentKey(logical), raw)
		return err
	})
}
