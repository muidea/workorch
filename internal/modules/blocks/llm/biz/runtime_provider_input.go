package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeFrozenProviderInput struct {
	Expected         rc.RuntimeExpectedDocumentContract
	Messages         json.RawMessage
	CacheKey         string
	LogicalRequestID string
	ExpectedHash     string
	DocumentKind     rc.RuntimeProviderDocumentKind
	ManifestHash     string
	StablePrefixHash string
	Ceiling          rc.ProviderCeiling
	Request          models.ProviderRequest
}

func freezeRuntimeProviderInput(r models.RuntimeCandidateRequest, p models.ProviderRequest) ([]byte, *rc.RuntimeProviderInputRef, error) {
	if r.ExpectedDocument == nil || r.RequestIdentity.LogicalRequestID == "" || r.ExpectedDocument.LogicalRequestID != r.RequestIdentity.LogicalRequestID || runtimeRequestProviderCeiling(r) == nil {
		return nil, nil, fmt.Errorf("provider input freeze requires exact request and expected document")
	}
	if err := r.ExpectedDocument.Validate(); err != nil {
		return nil, nil, err
	}
	p.Model = runtimeRequestProviderCeiling(r).Selection.Target.ModelID
	p.MaxOutputTokens = r.ExpectedDocument.MaxOutputTokens
	// Config is resolved only at the adapter boundary. This restricted snapshot
	// includes no endpoint, credential, response, physical identity or timestamp.
	var baseMessages []responsesInputMessage
	baseCacheKey := ""
	if len(r.BaseFrozenInput) > 0 || r.BaseFrozenInputRef != nil {
		if err := r.BaseFrozenInputRef.ValidateContent(r.BaseFrozenInput); err != nil {
			return nil, nil, err
		}
		base, err := decodeRuntimeFrozenProviderInput(r.BaseFrozenInput)
		if err != nil {
			return nil, nil, err
		}
		baseContract := r.ExpectedDocument.Clone()
		baseContract.CorrectionAttempt = 0
		baseContract.LogicalRequestID = base.Expected.LogicalRequestID
		baseContract.Hash = rc.StableRuntimeExpectedDocumentHash(baseContract)
		if r.ExpectedDocument.CorrectionAttempt != 1 || !reflect.DeepEqual(base.Expected, baseContract) || base.DocumentKind != r.DocumentKind || base.ManifestHash != r.ExpectedDocument.ManifestHash || base.StablePrefixHash != r.StablePrefixHash || !reflect.DeepEqual(base.Ceiling, runtimeRequestProviderCeiling(r).Normalized()) || base.Request.WorkspaceID != p.WorkspaceID || base.Request.RunID != p.RunID {
			return nil, nil, fmt.Errorf("correction base frozen input differs from exact request contract")
		}
		if p.PromptContext == nil || base.Request.PromptContext == nil {
			return nil, nil, fmt.Errorf("correction base input has no complete sections")
		}
		var sections []models.PromptSection
		for _, section := range base.Request.PromptContext.Sections {
			if group := promptSectionGroup(section.ID); group == "C0" || group == "C1" || group == "C2" || group == "C3" {
				sections = append(sections, section)
			}
		}
		for _, section := range p.PromptContext.Sections {
			if group := promptSectionGroup(section.ID); group == "C4" || group == "C5" {
				sections = append(sections, section)
			}
		}
		p.PromptContext.Sections = sections
		p.Schema = base.Request.Schema
		if err := json.Unmarshal(base.Messages, &baseMessages); err != nil {
			return nil, nil, err
		}
		baseCacheKey = base.CacheKey
	}
	if r.DocumentKind == rc.RuntimeProviderDocumentOperations {
		// A correction joins old C3 with new C4. Resolve references against
		// that actual composition, never against a discarded fresh projection.
		if err := validateRuntimeCandidateWorkIntentIntegrity(r.CandidateManifest, p.PromptContext); err != nil {
			return nil, nil, err
		}
		if err := validateRuntimeCandidateContextIntegrity(r.DefinitionContext, r.CandidateContext, r.CandidateManifest, r.CandidateAttempt, r.CandidateCorrectionObservation, p.ContextBudget, p.PromptContext); err != nil {
			return nil, nil, err
		}
	}
	rendered := sectionMessages(p, false)
	if baseMessages != nil {
		if len(baseMessages) != len(p.PromptContext.Sections) {
			return nil, nil, fmt.Errorf("correction frozen message inventory differs")
		}
		for i, section := range p.PromptContext.Sections {
			if group := promptSectionGroup(section.ID); group == "C0" || group == "C1" || group == "C2" || group == "C3" {
				rendered[i] = baseMessages[i]
			}
		}
	}
	messages, _ := json.Marshal(rendered)
	cacheKey := responsesPromptCacheKey(p)
	if baseCacheKey != "" {
		cacheKey = baseCacheKey
	}
	p.Config = models.RedactedProviderConfig{}
	// Audit the exact selected physical blocks, including a correction's
	// original prefix, rather than the current renderer's hypothetical layout.
	p.FrozenSectionMessages = append(json.RawMessage(nil), messages...)
	p.FrozenCacheKey = cacheKey
	attachProviderCachePlan(&p)
	// The Runtime commitment and the physical rendered-prefix hash have
	// distinct audit fields, just as in the original preflight renderer.
	p.ContextBudget.CachePlan.StablePrefixHash = r.StablePrefixHash
	populateRuntimeCandidateContextBudget(p.ContextBudget, p.PromptContext)
	encoded, encodeErr := json.Marshal(p)
	if encodeErr != nil {
		return nil, nil, encodeErr
	}
	if len(encoded) > runtimeRequestProviderCeiling(r).MaxBytes {
		return nil, nil, fmt.Errorf("frozen provider input exceeds sealed ceiling")
	}
	f := runtimeFrozenProviderInput{Expected: r.ExpectedDocument.Clone(), Messages: messages, CacheKey: cacheKey, LogicalRequestID: r.RequestIdentity.LogicalRequestID, ExpectedHash: r.ExpectedDocument.Hash, DocumentKind: r.DocumentKind, ManifestHash: r.ExpectedDocument.ManifestHash, StablePrefixHash: r.StablePrefixHash, Ceiling: runtimeRequestProviderCeiling(r).Normalized(), Request: p}
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, nil, err
	}
	ref := &rc.RuntimeProviderInputRef{Hash: rc.RuntimeProviderInputHash(raw), Bytes: len(raw)}
	return raw, ref, nil
}
func runtimeProviderRequestForSubmission(r models.RuntimeCandidateRequest) (models.ProviderRequest, error) {
	if r.FrozenInputRef == nil && len(r.FrozenInput) == 0 {
		return runtimeProviderRequest(r)
	}
	if err := r.FrozenInputRef.ValidateContent(r.FrozenInput); err != nil {
		return models.ProviderRequest{}, err
	}
	f, err := decodeRuntimeFrozenProviderInput(r.FrozenInput)
	if err != nil {
		return models.ProviderRequest{}, err
	}
	ws, run := runtimeRequestScope(r)
	ceiling := runtimeRequestProviderCeiling(r)
	if r.ExpectedDocument == nil || ceiling == nil || !reflect.DeepEqual(f.Expected, r.ExpectedDocument.Clone()) || f.LogicalRequestID != r.RequestIdentity.LogicalRequestID || f.ExpectedHash != r.ExpectedDocument.Hash || f.DocumentKind != r.DocumentKind || f.ManifestHash != r.ExpectedDocument.ManifestHash || f.StablePrefixHash != r.StablePrefixHash || f.Request.WorkspaceID != ws || f.Request.RunID != run {
		return models.ProviderRequest{}, fmt.Errorf("frozen provider input has different request/manifest/scope")
	}
	a, _ := json.Marshal(f.Ceiling)
	b, _ := json.Marshal(ceiling.Normalized())
	if !bytes.Equal(a, b) || !reflect.DeepEqual(f.Request.Config, models.RedactedProviderConfig{}) {
		return models.ProviderRequest{}, fmt.Errorf("frozen provider input differs from sealed ceiling")
	}
	if err := f.Request.ValidatePromptLayout(); err != nil {
		return models.ProviderRequest{}, err
	}
	if err := f.Request.ValidateSchemaFormat(); err != nil {
		return models.ProviderRequest{}, err
	}
	var messages []responsesInputMessage
	if err := json.Unmarshal(f.Messages, &messages); err != nil || f.Request.PromptContext == nil || len(messages) != len(f.Request.PromptContext.Sections) {
		return models.ProviderRequest{}, fmt.Errorf("frozen provider wire input missing or incomplete")
	}
	for i, s := range f.Request.PromptContext.Sections {
		role := s.Role
		if role == "system" {
			role = "developer"
		}
		text := ""
		for _, block := range messages[i].Content {
			if block.Type != "input_text" {
				return models.ProviderRequest{}, fmt.Errorf("frozen wire block type differs")
			}
			text += block.Text
		}
		if messages[i].Role != role || text != "### "+s.ID+"\n"+s.Content {
			return models.ProviderRequest{}, fmt.Errorf("frozen provider wire text/role differs from original input")
		}
	}
	f.Request.FrozenSectionMessages = append(json.RawMessage(nil), f.Messages...)
	f.Request.FrozenCacheKey = f.CacheKey
	return f.Request, nil
}

func decodeRuntimeFrozenProviderInput(raw []byte) (runtimeFrozenProviderInput, error) {
	var f runtimeFrozenProviderInput
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&f); err != nil {
		return f, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return f, fmt.Errorf("trailing frozen provider input")
	}
	if err := f.Expected.Validate(); err != nil {
		return f, err
	}
	if f.ExpectedHash != f.Expected.Hash || f.LogicalRequestID != f.Expected.LogicalRequestID || f.DocumentKind != f.Expected.DocumentKind || f.ManifestHash != f.Expected.ManifestHash || f.StablePrefixHash != f.Expected.StablePrefixHash {
		return f, fmt.Errorf("frozen input envelope differs from expected document")
	}
	return f, nil
}
