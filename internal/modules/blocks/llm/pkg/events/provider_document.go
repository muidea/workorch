package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RequestIdentity 是调用方已保留的关联身份，LLM owner 不解释其业务语义。
type CompleteProviderDocumentCommand struct {
	RequestIdentity               string
	Target                        runtimecontract.ProviderModelTarget
	Request                       ProviderRequest
	ExpectedConfigurationRevision string
}

func (c CompleteProviderDocumentCommand) Clone() CompleteProviderDocumentCommand {
	c.Request.TraceRefs = slices.Clone(c.Request.TraceRefs)
	c.Request.Config.Capabilities.NonChatOperations = slices.Clone(c.Request.Config.Capabilities.NonChatOperations)
	if c.Request.Config.InferenceProfile.Temperature != nil {
		temperature := *c.Request.Config.InferenceProfile.Temperature
		c.Request.Config.InferenceProfile.Temperature = &temperature
	}
	if c.Request.PromptContext != nil {
		p := *c.Request.PromptContext
		p.Sections = slices.Clone(p.Sections)
		for i := range p.Sections {
			p.Sections[i].SourceRefs = slices.Clone(p.Sections[i].SourceRefs)
		}
		p.Turns = slices.Clone(p.Turns)
		if p.Normalization != nil {
			n := *p.Normalization
			n.NormalizedSections = slices.Clone(n.NormalizedSections)
			n.ProtectedSections = slices.Clone(n.ProtectedSections)
			p.Normalization = &n
		}
		c.Request.PromptContext = &p
	}
	if c.Request.ContextBudget != nil {
		b := *c.Request.ContextBudget
		b.Rules = slices.Clone(b.Rules)
		b.SourceRefs = slices.Clone(b.SourceRefs)
		b.IncludedSections = slices.Clone(b.IncludedSections)
		b.NormalizedSections = slices.Clone(b.NormalizedSections)
		b.CompressedSections = slices.Clone(b.CompressedSections)
		b.OmittedSections = slices.Clone(b.OmittedSections)
		b.RedactedPaths = slices.Clone(b.RedactedPaths)
		b.ProtectedFields = slices.Clone(b.ProtectedFields)
		b.IntegrityWarnings = slices.Clone(b.IntegrityWarnings)
		if b.CachePlan != nil {
			p := *b.CachePlan
			p.SectionHashes = slices.Clone(p.SectionHashes)
			for i := range p.SectionHashes {
				p.SectionHashes[i].SourceRefs = slices.Clone(p.SectionHashes[i].SourceRefs)
			}
			p.CacheBreakers = slices.Clone(p.CacheBreakers)
			p.Canonicalization.SectionOrder = slices.Clone(p.Canonicalization.SectionOrder)
			p.AdapterHint.CacheablePrefix = slices.Clone(p.AdapterHint.CacheablePrefix)
			p.AdapterHint.VolatileSections = slices.Clone(p.AdapterHint.VolatileSections)
			p.AdapterHint.CacheBreakers = slices.Clone(p.AdapterHint.CacheBreakers)
			b.CachePlan = &p
		}
		c.Request.ContextBudget = &b
	}
	return c
}

func (c CompleteProviderDocumentCommand) Validate() error {
	if c.RequestIdentity == "" || len(c.RequestIdentity) > 256 || strings.TrimSpace(c.RequestIdentity) != c.RequestIdentity || strings.ContainsRune(c.RequestIdentity, 0) || c.Target.Validate() != nil || c.Target != c.Target.Normalized() || c.ExpectedConfigurationRevision == "" {
		return fmt.Errorf("document command identity invalid")
	}
	if _, err := c.Request.ToModel(); err != nil {
		return err
	}
	if c.Request.Model != c.Target.ModelID {
		return fmt.Errorf("document command model mismatch")
	}
	return nil
}

func (c CompleteProviderDocumentCommand) RequestHash() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return "provider-document-sha256:" + hex.EncodeToString(hash[:]), nil
}

type ProviderCompleteDocument struct {
	Content models.ProviderObservedContent
}

type ProviderDocumentFailure struct {
	Code  string
	Cause []byte `json:"-"`
}

type ProviderIncompleteDocument struct {
	Code  string
	Cause []byte `json:"-"`
}

// 三种事实严格互斥；交付成功不代表 provider 或文档成功。
// 原始材料只在受限同步调用及诊断存储边界传递，不进入日志/普通 JSON。
type CompleteProviderDocumentResult struct {
	RequestIdentity              string
	RequestHash                  string
	Target                       runtimecontract.ProviderModelTarget
	ConfigurationRevision        string
	Document                     *ProviderCompleteDocument
	Failure                      *ProviderDocumentFailure
	Incomplete                   *ProviderIncompleteDocument
	ObservedDocument             models.ProviderObservedContent
	Wire                         models.ProviderObservedContent
	TransportCause               *models.ProviderObservedContent
	Usage                        *llmaudit.ProviderUsage
	ProviderSubmitted            bool
	PhysicalResponseMode         llmaudit.PhysicalResponseMode
	AuditPublicationFailureClass string
}

func (r CompleteProviderDocumentResult) ValidateFor(c CompleteProviderDocumentCommand) error {
	hash, err := c.RequestHash()
	if err != nil || r.RequestIdentity != c.RequestIdentity || r.RequestHash != hash || r.Target != c.Target || r.ConfigurationRevision != c.ExpectedConfigurationRevision {
		return fmt.Errorf("document result request identity mismatch")
	}
	if r.PhysicalResponseMode.Validate() != nil || r.PhysicalResponseMode != r.PhysicalResponseMode.Normalized() || validateGenericProviderUsage(r.Usage) != nil || r.Wire.Validate() != nil || r.ObservedDocument.Validate() != nil {
		return fmt.Errorf("document result observation invalid")
	}
	if r.TransportCause != nil && (r.TransportCause.Validate() != nil || !r.TransportCause.Observed || !r.TransportCause.Complete || r.Failure == nil) {
		return fmt.Errorf("document result transport cause invalid")
	}
	if r.AuditPublicationFailureClass != "" && r.AuditPublicationFailureClass != "audit_publication_failed" {
		return fmt.Errorf("document result audit publication invalid")
	}
	arms := 0
	if r.Document != nil {
		arms++
		if !r.ProviderSubmitted || !r.Document.Content.Observed || !r.Document.Content.Complete || r.Document.Content.Validate() != nil || !r.ObservedDocument.Complete || !reflect.DeepEqual(r.Document.Content, r.ObservedDocument) {
			return fmt.Errorf("document result complete content invalid")
		}
	}
	if r.Failure != nil {
		arms++
		if !r.ProviderSubmitted || !providerDocumentCodeCanonical(r.Failure.Code) || len(r.Failure.Cause) == 0 {
			return fmt.Errorf("document result failure invalid")
		}
	}
	if r.Incomplete != nil {
		arms++
		if !r.ProviderSubmitted || (r.Incomplete.Code != "output_limit" && r.Incomplete.Code != "stream_ended_before_completion") || len(r.Incomplete.Cause) == 0 || r.ObservedDocument.Complete {
			return fmt.Errorf("document result incomplete content invalid")
		}
	}
	if arms != 1 {
		return fmt.Errorf("document result requires exactly one outcome")
	}
	return nil
}

func providerDocumentCodeCanonical(code string) bool {
	if code == "" || len(code) > 128 {
		return false
	}
	for _, char := range code {
		if char != '_' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func (r CompleteProviderDocumentResult) Clone() CompleteProviderDocumentResult {
	r.Wire = r.Wire.Clone()
	if r.TransportCause != nil {
		cause := r.TransportCause.Clone()
		r.TransportCause = &cause
	}
	r.ObservedDocument = r.ObservedDocument.Clone()
	if r.Document != nil {
		r.Document = &ProviderCompleteDocument{Content: r.Document.Content.Clone()}
	}
	if r.Failure != nil {
		r.Failure = &ProviderDocumentFailure{Code: r.Failure.Code, Cause: append([]byte{}, r.Failure.Cause...)}
	}
	if r.Incomplete != nil {
		r.Incomplete = &ProviderIncompleteDocument{Code: r.Incomplete.Code, Cause: append([]byte{}, r.Incomplete.Cause...)}
	}
	if r.Usage != nil {
		usage := *r.Usage
		r.Usage = &usage
	}
	return r
}
