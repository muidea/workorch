package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeExpectedDocumentSchemaCurrent = "workorch.runtime_expected_document.v1"

// RuntimeExpectedDocumentContract is the sealed identity of one complete
// authority document expected from every physical provider call. Recovery may
// change only the physical call identity; it cannot change this value.
type RuntimeExpectedDocumentContract struct {
	SchemaVersion            string
	Hash                     string
	DocumentKind             RuntimeProviderDocumentKind
	PlanStep                 RuntimeSemanticPlanStep
	ManifestHash             string
	DefinitionSeedHash       string
	LogicalRequestID         string
	CorrectionAttempt        int
	StablePrefixHash         string
	BatchID                  string
	BatchSequence            int
	BatchCoverageWorkItemIDs []common.CoverageWorkItemID
	CoverageOrderHash        string
	MaxOutputTokens          int
	MaxRecoveryAttempts      int
}

func (c RuntimeExpectedDocumentContract) Normalized() RuntimeExpectedDocumentContract {
	c.SchemaVersion = strings.TrimSpace(c.SchemaVersion)
	c.Hash = strings.TrimSpace(c.Hash)
	c.DocumentKind = RuntimeProviderDocumentKind(strings.TrimSpace(string(c.DocumentKind)))
	c.PlanStep = RuntimeSemanticPlanStep(strings.TrimSpace(string(c.PlanStep)))
	c.ManifestHash = strings.TrimSpace(c.ManifestHash)
	c.DefinitionSeedHash = strings.TrimSpace(c.DefinitionSeedHash)
	c.LogicalRequestID = strings.TrimSpace(c.LogicalRequestID)
	c.StablePrefixHash = strings.TrimSpace(c.StablePrefixHash)
	c.BatchID = strings.TrimSpace(c.BatchID)
	c.CoverageOrderHash = strings.TrimSpace(c.CoverageOrderHash)
	ids := make([]common.CoverageWorkItemID, 0, len(c.BatchCoverageWorkItemIDs))
	for _, id := range c.BatchCoverageWorkItemIDs {
		id = common.CoverageWorkItemID(strings.TrimSpace(string(id)))
		if id != "" {
			ids = append(ids, id)
		}
	}
	c.BatchCoverageWorkItemIDs = nilIfEmpty(ids)
	return c
}

func (c RuntimeExpectedDocumentContract) Clone() RuntimeExpectedDocumentContract {
	c = c.Normalized()
	c.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), c.BatchCoverageWorkItemIDs...)
	return c
}

func (c RuntimeExpectedDocumentContract) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != RuntimeExpectedDocumentSchemaCurrent || c.Hash == "" || c.ManifestHash == "" || c.LogicalRequestID == "" || c.StablePrefixHash == "" || c.CorrectionAttempt < 0 || c.CorrectionAttempt > 1 || c.MaxOutputTokens <= 0 || c.MaxOutputTokens > 131072 || c.MaxRecoveryAttempts <= 0 || c.MaxRecoveryAttempts > RuntimeRecoveryBudgetLimit {
		return common.ErrInvalidArgument
	}
	switch c.DocumentKind {
	case RuntimeProviderDocumentOperations:
		if c.PlanStep != "" || c.DefinitionSeedHash != "" || c.BatchID != "" || c.BatchSequence != 0 || len(c.BatchCoverageWorkItemIDs) != 0 || c.CoverageOrderHash != "" {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentSemanticPlan:
		if c.DefinitionSeedHash == "" {
			return common.ErrInvalidArgument
		}
		switch c.PlanStep {
		case RuntimeSemanticPlanStepCoverageOrder:
			if c.BatchID != "" || c.BatchSequence != 0 || len(c.BatchCoverageWorkItemIDs) != 0 || c.CoverageOrderHash != "" {
				return common.ErrInvalidArgument
			}
		case RuntimeSemanticPlanStepProofBatch:
			if c.BatchID == "" || c.BatchSequence <= 0 || len(c.BatchCoverageWorkItemIDs) == 0 || c.CoverageOrderHash == "" {
				return common.ErrInvalidArgument
			}
			seen := make(map[common.CoverageWorkItemID]struct{}, len(c.BatchCoverageWorkItemIDs))
			for _, id := range c.BatchCoverageWorkItemIDs {
				if _, duplicate := seen[id]; duplicate {
					return common.ErrInvalidArgument
				}
				seen[id] = struct{}{}
			}
		default:
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	if c.Hash != StableRuntimeExpectedDocumentHash(c) {
		return fmt.Errorf("%w: expected document hash mismatch", common.ErrInvalidArgument)
	}
	return nil
}

func StableRuntimeExpectedDocumentHash(contract RuntimeExpectedDocumentContract) string {
	contract = contract.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeExpectedDocumentSchemaCurrent)
	writeHashString(h, string(contract.DocumentKind))
	writeHashString(h, string(contract.PlanStep))
	writeHashString(h, contract.ManifestHash)
	writeHashString(h, contract.DefinitionSeedHash)
	writeHashString(h, contract.LogicalRequestID)
	writeHashString(h, fmt.Sprintf("%d", contract.CorrectionAttempt))
	writeHashString(h, contract.StablePrefixHash)
	writeHashString(h, contract.BatchID)
	writeHashString(h, fmt.Sprintf("%d", contract.BatchSequence))
	for _, id := range contract.BatchCoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	writeHashString(h, contract.CoverageOrderHash)
	writeHashString(h, fmt.Sprintf("%d", contract.MaxOutputTokens))
	writeHashString(h, fmt.Sprintf("%d", contract.MaxRecoveryAttempts))
	return "runtime-expected-document-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeProviderDocumentDisposition string

const (
	RuntimeProviderDocumentComplete   RuntimeProviderDocumentDisposition = "complete_document"
	RuntimeProviderDocumentIncomplete RuntimeProviderDocumentDisposition = "incomplete_document"
	RuntimeProviderDocumentEmpty      RuntimeProviderDocumentDisposition = "empty_document"
	RuntimeProviderDocumentInvalid    RuntimeProviderDocumentDisposition = "invalid_provider_output"
)

// RuntimeProviderDocumentOutcome is a bounded LLM-owner observation. It does
// not admit a candidate, choose recovery, or select a Runtime transition.
type RuntimeProviderDocumentOutcome struct {
	Disposition          RuntimeProviderDocumentDisposition
	ExpectedDocumentHash string
	ResponseHash         string
	ResponseBytes        int64
	Code                 string
	Signature            string
}

func (o RuntimeProviderDocumentOutcome) Normalized() RuntimeProviderDocumentOutcome {
	o.Disposition = RuntimeProviderDocumentDisposition(strings.TrimSpace(string(o.Disposition)))
	o.ExpectedDocumentHash = strings.TrimSpace(o.ExpectedDocumentHash)
	o.ResponseHash = strings.TrimSpace(o.ResponseHash)
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	return o
}

func (o RuntimeProviderDocumentOutcome) Validate() error {
	o = o.Normalized()
	if o.ExpectedDocumentHash == "" || o.ResponseBytes < 0 {
		return common.ErrInvalidArgument
	}
	switch o.Disposition {
	case RuntimeProviderDocumentComplete:
		if o.ResponseHash == "" || o.ResponseBytes == 0 || o.Code != "" {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentIncomplete, RuntimeProviderDocumentInvalid:
		if o.ResponseHash == "" || o.ResponseBytes == 0 || o.Code == "" {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentEmpty:
		if o.ResponseHash != "" || o.ResponseBytes != 0 || o.Code == "" {
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	return nil
}
