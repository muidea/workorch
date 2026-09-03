package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeExpectedDocumentSchemaCurrent = "workorch.runtime_expected_document"

// RuntimeExpectedDocumentContract is the sealed identity of one complete
// authority document expected from every physical provider call. Recovery may
// change only the physical call identity; it cannot change this value.
type RuntimeExpectedDocumentContract struct {
	SchemaVersion string
	Hash          string
	DocumentKind  RuntimeProviderDocumentKind
	PlanStep      RuntimeSemanticPlanStep
	ManifestHash  string
	// WireContractHash binds an operations document to the exact
	// manifest-derived provider proposal surface. Semantic-plan documents use
	// their independent semantic schema and must leave this field empty.
	WireContractHash         string
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
	c.WireContractHash = strings.TrimSpace(c.WireContractHash)
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
		if c.WireContractHash == "" || c.PlanStep != "" || c.DefinitionSeedHash != "" || c.BatchID != "" || c.BatchSequence != 0 || len(c.BatchCoverageWorkItemIDs) != 0 || c.CoverageOrderHash != "" {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentSemanticPlan:
		if c.WireContractHash != "" || c.DefinitionSeedHash == "" {
			return common.ErrInvalidArgument
		}
		switch c.PlanStep {
		case RuntimeSemanticPlanStepCoverageOrder:
			if c.BatchID != "" || c.BatchSequence != 0 || len(c.BatchCoverageWorkItemIDs) != 0 || c.CoverageOrderHash != "" {
				return common.ErrInvalidArgument
			}
		case RuntimeSemanticPlanStepProofBatch:
			if c.BatchID == "" || c.BatchSequence < 0 || len(c.BatchCoverageWorkItemIDs) == 0 || c.CoverageOrderHash == "" {
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
	writeHashString(h, contract.WireContractHash)
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

	// Runtime provider-document identities are a closed owner-neutral
	// vocabulary.  The LLM owner reports only the observed framing fact; these
	// values do not grant retry or terminal authority to the producer.
	RuntimeProviderDocumentCodeOutputLimit         = "max_output_tokens"
	RuntimeProviderDocumentSignatureOutputLimit    = "provider-output-limit"
	RuntimeProviderDocumentCodeIncompleteJSON      = "incomplete_json_document"
	RuntimeProviderDocumentSignatureIncompleteJSON = "provider-document-incomplete"
	RuntimeProviderDocumentCodeFinalOutputMissing  = "provider_final_output_missing"
	RuntimeProviderDocumentSignatureOutputMissing  = "provider-final-output-missing"
	RuntimeProviderDocumentCodeFinalOutputInvalid  = "provider_final_output_invalid"
	RuntimeProviderDocumentSignatureOutputInvalid  = "provider-final-output-invalid"
)

// RuntimeProviderDocumentDispositionNeedsRecovery reports whether the LLM
// owner observed no candidate object at the physical-response boundary. These
// framing outcomes stay in the request-session recovery ledger and cannot be
// converted into a candidate or semantic correction attempt.
func RuntimeProviderDocumentDispositionNeedsRecovery(disposition RuntimeProviderDocumentDisposition) bool {
	switch RuntimeProviderDocumentDisposition(strings.TrimSpace(string(disposition))) {
	case RuntimeProviderDocumentIncomplete, RuntimeProviderDocumentEmpty, RuntimeProviderDocumentInvalid:
		return true
	default:
		return false
	}
}

// RuntimeProviderDocumentFraming is an owner-neutral, pure DTO describing the
// bounded framing of a physical provider response. Only the LLM owner may
// produce its disposition from raw provider bytes; shared placement does not
// authorize downstream reclassification. Framing becomes an LLM fact only
// inside the concrete LLM EventHub outcome and must not be treated as
// Orchestrator durable authority by itself.
type RuntimeProviderDocumentFraming struct {
	Disposition          RuntimeProviderDocumentDisposition
	ExpectedDocumentHash string
	ResponseHash         string
	ResponseBytes        int64
	Code                 string
	Signature            string
}

func (o RuntimeProviderDocumentFraming) Normalized() RuntimeProviderDocumentFraming {
	o.Disposition = RuntimeProviderDocumentDisposition(strings.TrimSpace(string(o.Disposition)))
	o.ExpectedDocumentHash = strings.TrimSpace(o.ExpectedDocumentHash)
	o.ResponseHash = strings.TrimSpace(o.ResponseHash)
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	return o
}

func (o RuntimeProviderDocumentFraming) Validate() error {
	o = o.Normalized()
	if o.ExpectedDocumentHash == "" || o.ResponseBytes < 0 {
		return common.ErrInvalidArgument
	}
	// ResponseHash and ResponseBytes describe the same bounded provider bytes
	// and therefore must always be present or absent together. An explicit
	// max-output stop may legitimately have no final-document bytes when the
	// provider spent its entire output budget on reasoning.
	hasResponse := o.ResponseBytes > 0
	if hasResponse != (o.ResponseHash != "") {
		return common.ErrInvalidArgument
	}
	switch o.Disposition {
	case RuntimeProviderDocumentComplete:
		if !hasResponse || o.Code != "" || o.Signature != "" {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentIncomplete:
		outputLimit := o.Code == RuntimeProviderDocumentCodeOutputLimit && o.Signature == RuntimeProviderDocumentSignatureOutputLimit
		truncatedJSON := o.Code == RuntimeProviderDocumentCodeIncompleteJSON && o.Signature == RuntimeProviderDocumentSignatureIncompleteJSON
		if !outputLimit && !truncatedJSON {
			return common.ErrInvalidArgument
		}
		// A provider may spend its output budget entirely on hidden reasoning,
		// but a structural EOF observation necessarily has response bytes.
		if !hasResponse && !outputLimit {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentInvalid:
		if !hasResponse || o.Code != RuntimeProviderDocumentCodeFinalOutputInvalid || o.Signature != RuntimeProviderDocumentSignatureOutputInvalid {
			return common.ErrInvalidArgument
		}
	case RuntimeProviderDocumentEmpty:
		if hasResponse || o.Code != RuntimeProviderDocumentCodeFinalOutputMissing || o.Signature != RuntimeProviderDocumentSignatureOutputMissing {
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	return nil
}

// ValidateRuntimeProviderDocumentFramingContent validates the owner-neutral
// relationship between bounded response framing and opaque content. It does
// not create an LLM fact or select any Runtime transition.
func ValidateRuntimeProviderDocumentFramingContent(framing RuntimeProviderDocumentFraming, content []byte) error {
	framing = framing.Normalized()
	if err := framing.Validate(); err != nil {
		return fmt.Errorf("invalid provider document framing: %w", err)
	}
	if framing.Disposition == RuntimeProviderDocumentComplete {
		if len(content) == 0 || int64(len(content)) != framing.ResponseBytes || StableRuntimeProviderResponseHash(content) != framing.ResponseHash {
			return fmt.Errorf("complete provider document requires matching content")
		}
		return nil
	}
	if content != nil {
		return fmt.Errorf("non-complete provider document must not carry content")
	}
	return nil
}

func StableRuntimeProviderResponseHash(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	sum := sha256.Sum256(content)
	return "provider-response-" + hex.EncodeToString(sum[:])[:20]
}
