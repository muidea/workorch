package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SemanticExpectedDocument projects the current persisted request intent. Both
// request rendering and SM-3 admission use this read-only projection; callers
// cannot select a different batch, correction attempt, or output budget.
func SemanticExpectedDocument(aggregate RuntimeAggregate) (runtimecontract.RuntimeExpectedDocumentContract, error) {
	ledger := aggregate.SemanticPlanning
	if ledger.Manifest == nil {
		return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: semantic expected document requires a manifest", common.ErrConflict)
	}
	execution := aggregate.Facts.Activation.ExecutionContract.Normalized()
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:      runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:       runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:           runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		ManifestHash:       ledger.Manifest.Hash, LogicalRequestID: ledger.LogicalRequestID,
		StablePrefixHash: ledger.StablePrefixHash, CorrectionAttempt: ledger.Attempt,
		MaxRecoveryAttempts: execution.CandidateDocumentRecovery.MaxAttempts,
	}
	if aggregate.Facts.Resources.ProviderCeiling != nil {
		expected.MaxOutputTokens = aggregate.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens
	}
	if expected.MaxRecoveryAttempts <= 0 {
		expected.MaxRecoveryAttempts = 1
	}
	if ledger.ActiveBatchID != "" {
		found := false
		for _, batch := range ledger.Batches {
			if batch.BatchID != ledger.ActiveBatchID {
				continue
			}
			if found {
				return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: ambiguous semantic active batch", common.ErrConflict)
			}
			found = true
			expected.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
			expected.BatchID, expected.BatchSequence = batch.BatchID, batch.Sequence
			expected.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
			expected.CoverageOrderHash = batch.CoverageOrderHash
		}
		if !found {
			return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: semantic expected document has no persisted active batch", common.ErrConflict)
		}
	}
	expected.SemanticContextEncoding = runtimecontract.RuntimeSemanticContextEncoding

	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID != expected.LogicalRequestID {
			continue
		}
		if session.DocumentKind != expected.DocumentKind ||
			!runtimecontract.SameAuthorityContract(session.Transaction, aggregate.Decision.AuthorityTransaction()) ||
			session.ManifestHash != expected.ManifestHash ||
			!sameRuntimeExpectedDocument(session.ExpectedDocument, &expected) {
			return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: persisted semantic request differs from current intent", common.ErrConflict)
		}
	}
	return expected, nil
}

// SemanticRequestIdentityHash identifies the request within its sealed intent;
// it does not create or advance the semantic planning ledger.
func SemanticRequestIdentityHash(ledger runtimecontract.RuntimeSemanticPlanningLedger) (string, error) {
	if ledger.CorrectionDirective != nil {
		return SemanticRequestIdentityHashForKey(ledger, ledger.CorrectionDirective.Hash), nil
	}
	if ledger.ActiveBatchID == "" {
		return SemanticRequestIdentityHashForKey(ledger, ""), nil
	}
	for _, batch := range ledger.Batches {
		if batch.BatchID == ledger.ActiveBatchID {
			return SemanticRequestIdentityHashForKey(ledger, fmt.Sprintf("semantic-proof-batch-%d", batch.Sequence)), nil
		}
	}
	return "", fmt.Errorf("%w: semantic request identity has no persisted active batch", common.ErrConflict)
}

// SemanticRequestIdentityHashForKey binds a provider request to the current
// semantic generation as well as its order/batch/correction slot. The first
// generation intentionally keeps the historical empty/order and readable
// batch keys. Once a generation has been retired, its immutable retirement
// revision prevents a later generation with the same authority transaction,
// manifest and prefix from reusing a terminal prerequisite-stale SM-3 session.
func SemanticRequestIdentityHashForKey(ledger runtimecontract.RuntimeSemanticPlanningLedger, requestKey string) string {
	requestKey = strings.TrimSpace(requestKey)
	if len(ledger.History) == 0 {
		return requestKey
	}
	retired := ledger.History[len(ledger.History)-1].Normalized()
	h := sha256.New()
	_, _ = h.Write([]byte("runtime-semantic-request-generation\x00"))
	_, _ = h.Write([]byte(fmt.Sprintf("%d", retired.RetiredAtStateRevision)))
	_, _ = h.Write([]byte{'\x00'})
	_, _ = h.Write([]byte(retired.Reason))
	_, _ = h.Write([]byte{'\x00'})
	if retired.Snapshot.Manifest != nil {
		_, _ = h.Write([]byte(retired.Snapshot.Manifest.Hash))
	}
	_, _ = h.Write([]byte{'\x00'})
	_, _ = h.Write([]byte(retired.Snapshot.LogicalRequestID))
	_, _ = h.Write([]byte{'\x00'})
	_, _ = h.Write([]byte(requestKey))
	return "semantic-request-identity-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func validateSemanticRequestIntent(aggregate RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) error {
	expected, err := SemanticExpectedDocument(aggregate)
	if err != nil {
		return err
	}
	identityHash, err := SemanticRequestIdentityHash(aggregate.SemanticPlanning)
	if err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("%w: current semantic expected document is invalid", common.ErrConflict)
	}
	if session.LogicalRequestID != expected.LogicalRequestID || session.ManifestHash != expected.ManifestHash ||
		session.StablePrefixHash != expected.StablePrefixHash || session.CorrectionHash != identityHash ||
		!sameRuntimeExpectedDocument(session.ExpectedDocument, &expected) {
		return fmt.Errorf("%w: semantic request session does not match current request intent", common.ErrConflict)
	}
	return nil
}
