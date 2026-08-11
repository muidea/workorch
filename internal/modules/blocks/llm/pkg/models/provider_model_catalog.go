package models

import (
	"strings"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CapabilitySource values for resolved provider/model profiles.
const (
	CapabilitySourceAIProxyCatalog       = "ai_proxy_catalog"
	CapabilitySourceAIProxyExactOverride = "ai_proxy_exact_override"
)

// ProviderCatalogState describes the cache/readiness state of a model catalog.
type ProviderCatalogState string

const (
	ProviderCatalogStateEmpty       ProviderCatalogState = "empty"
	ProviderCatalogStateFresh       ProviderCatalogState = "fresh"
	ProviderCatalogStateStale       ProviderCatalogState = "stale"
	ProviderCatalogStateRefreshing  ProviderCatalogState = "refreshing"
	ProviderCatalogStateUnavailable ProviderCatalogState = "unavailable"
)

// ProviderModelDescriptor is one model discovered from the ai-proxy catalog.
type ProviderModelDescriptor struct {
	Target              runtimecontract.ProviderModelTarget
	RouteOwner          string
	ContextWindowTokens int
	MaxOutputTokens     int
	// SupportedEndpoints is the exact client-facing endpoint capability
	// reported by ai-proxy. Operations are derived locally from these paths.
	SupportedEndpoints []string
	Operations         []string
	Reasoning          ProviderReasoningCapability
	CapabilityReady    bool
	UnavailableReason  string
	CapabilityRevision string
}

// ProviderReasoningCapability is the LLM owner's normalized view of the
// optional ai-proxy capabilities.reasoning declaration. It is independent of
// base model readiness so missing reasoning metadata does not disable ordinary
// Responses requests.
type ProviderReasoningCapability struct {
	Supported     bool
	DefaultEffort string
	Efforts       []string
	Valid         bool
	InvalidReason string
}

func (c ProviderReasoningCapability) Normalized() ProviderReasoningCapability {
	c.DefaultEffort = strings.TrimSpace(c.DefaultEffort)
	c.InvalidReason = ""
	if !c.Supported {
		c.DefaultEffort = ""
		c.Efforts = nil
		c.Valid = true
		return c
	}
	seen := map[string]struct{}{}
	efforts := make([]string, 0, len(c.Efforts))
	for _, effort := range c.Efforts {
		effort = strings.TrimSpace(effort)
		if effort == "" {
			continue
		}
		if _, ok := seen[effort]; ok {
			continue
		}
		seen[effort] = struct{}{}
		efforts = append(efforts, effort)
	}
	c.Efforts = efforts
	c.Valid = true
	if len(c.Efforts) == 0 {
		c.Valid = false
		c.InvalidReason = "reasoning_efforts_missing"
		return c
	}
	if c.DefaultEffort != "" {
		if _, ok := seen[c.DefaultEffort]; !ok {
			c.Valid = false
			c.InvalidReason = "reasoning_default_effort_unsupported"
		}
	}
	return c
}

func (c ProviderReasoningCapability) Allows(effort string) bool {
	c = c.Normalized()
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return true
	}
	if !c.Supported || !c.Valid {
		return false
	}
	for _, allowed := range c.Efforts {
		if effort == allowed {
			return true
		}
	}
	return false
}

func (c ProviderReasoningCapability) Clone() ProviderReasoningCapability {
	c = c.Normalized()
	c.Efforts = append([]string(nil), c.Efforts...)
	return c
}

// ProviderModelCatalog is a bounded, normalized snapshot of models for one provider endpoint.
type ProviderModelCatalog struct {
	ProviderID   string
	Models       []ProviderModelDescriptor
	Revision     string
	FetchedAt    time.Time
	ExpiresAt    time.Time
	Stale        bool
	Source       string
	State        ProviderCatalogState
	ErrorCode    string
	ErrorMessage string
}

// ResolvedProviderModelProfile is the complete capability/budget profile for one target.
type ResolvedProviderModelProfile struct {
	Target                   runtimecontract.ProviderModelTarget
	Operation                string
	APISurface               string
	ContextWindowTokens      int
	ProviderMaxOutputTokens  int
	EffectiveMaxOutputTokens int
	ReservedOutputTokens     int
	MaxInputTokens           int
	EstimatedTokensPerByte   float64
	MaxRequestBytes          int
	CapabilitySource         string
	CapabilityRevision       string
	ConfigurationRevision    string
}

// ProviderModelFailureCode classifies target/catalog/capability failures.
type ProviderModelFailureCode string

const (
	ProviderModelFailureConfigurationInvalid ProviderModelFailureCode = "configuration_invalid"
	ProviderModelFailureConfigurationDrift   ProviderModelFailureCode = "provider_configuration_drift"
	ProviderModelFailureCatalogUnavailable   ProviderModelFailureCode = "catalog_unavailable"
	ProviderModelFailureCatalogInvalid       ProviderModelFailureCode = "catalog_invalid"
	ProviderModelFailureSelectionRequired    ProviderModelFailureCode = "selection_required"
	ProviderModelFailureProviderNotFound     ProviderModelFailureCode = "provider_not_found"
	ProviderModelFailureModelNotFound        ProviderModelFailureCode = "model_not_found"
	ProviderModelFailureCapabilityIncomplete ProviderModelFailureCode = "capability_incomplete"
	ProviderModelFailureOperationUnsupported ProviderModelFailureCode = "operation_unsupported"
	ProviderModelFailureCeilingInsufficient  ProviderModelFailureCode = "ceiling_insufficient"
	ProviderModelFailureRequestOverBudget    ProviderModelFailureCode = "request_over_budget"
	ProviderModelFailureProviderUnavailable  ProviderModelFailureCode = "provider_unavailable"
)

// ProviderModelFailure is a typed failure for catalog/target resolution paths.
type ProviderModelFailure struct {
	Code    ProviderModelFailureCode
	Message string
	Target  runtimecontract.ProviderModelTarget
}

func (f ProviderModelFailure) Error() string {
	if f.Message == "" {
		return string(f.Code)
	}
	if f.Code == "" {
		return f.Message
	}
	return string(f.Code) + ": " + f.Message
}
