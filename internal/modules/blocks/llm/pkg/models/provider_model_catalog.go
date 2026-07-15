package models

import (
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CapabilitySource values for resolved provider/model profiles.
const (
	CapabilitySourceAIProxyCatalog         = "ai_proxy_catalog"
	CapabilitySourceAIProxyExactOverride   = "ai_proxy_exact_override"
	CapabilitySourceCustomProviderExplicit = "custom_provider_explicit"
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

// ProviderModelDescriptor is one model discovered from ai-proxy catalog or declared
// by a custom provider configuration.
type ProviderModelDescriptor struct {
	Target              runtimecontract.ProviderModelTarget
	RouteOwner          string
	ContextWindowTokens int
	MaxOutputTokens     int
	Operations          []string
	CapabilityReady     bool
	UnavailableReason   string
	CapabilityRevision  string
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
