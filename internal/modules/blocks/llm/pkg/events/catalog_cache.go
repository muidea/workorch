package events

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const TopicObserveProviderCatalogCache = "/llm/startup-preparation/catalog-cache/query"
const CatalogCacheObservationMaxBytes = 1024 * 1024
const CatalogCacheObservationModelLimit = 4096

// This query only observes an already populated owner cache. It neither joins
// an inflight refresh nor asks the owner to fetch or invoke a model. A fresh
// observation may also compute an exact capacity preview with the pure resolver.
type ObserveProviderCatalogCacheCommand struct {
	WorkspaceID common.WorkspaceID
	// Optional exact target intent; nil means this owner's frozen default.
	Target *runtimecontract.ProviderModelTarget
}

func (c ObserveProviderCatalogCacheCommand) Validate() error {
	if !cacheObservationText(string(c.WorkspaceID), 256) {
		return common.ErrInvalidArgument
	}
	if c.Target != nil && (c.Target.Validate() != nil || *c.Target != c.Target.Normalized() || c.Target.ProviderID != runtimecontract.BuiltinAIProxyProviderID || !cacheObservationText(c.Target.ModelID, 256)) {
		return common.ErrInvalidArgument
	}
	return nil
}

// OwnerEpoch is random per frozen configuration lifecycle, not derived from
// credentials. It is invalid across owner/process restart. A fresh catalog
// proves only a complete cached capability declaration at ObservedAt, not
// connectivity, authorization, provider ceiling, Workspace readiness or M3.
// Optional Budget is a separate historical capacity preview, not such a proof.
type ObserveProviderCatalogCacheResult struct {
	WorkspaceID common.WorkspaceID
	OwnerEpoch  string
	ObservedAt  time.Time
	Catalog     llmmodel.ProviderModelCatalog
	// Optional only for historical records. New owner observations include the
	// frozen non-secret selection declarations, never endpoint/key-derived hashes.
	Configuration *PreparationProviderConfiguration `json:",omitempty"`
	Budget        *PreparationProviderBudget        `json:",omitempty"`
}

func (r ObserveProviderCatalogCacheResult) Clone() ObserveProviderCatalogCacheResult {
	if r.Budget != nil {
		v := *r.Budget
		r.Budget = &v
	}
	if r.Configuration != nil {
		v := r.Configuration.Clone()
		r.Configuration = &v
	}
	r.Catalog.Models = slices.Clone(r.Catalog.Models)
	for index := range r.Catalog.Models {
		model := &r.Catalog.Models[index]
		model.SupportedEndpoints = slices.Clone(model.SupportedEndpoints)
		model.Operations = slices.Clone(model.Operations)
		// Do not normalize while detaching an observation.
		model.Reasoning.Efforts = slices.Clone(model.Reasoning.Efforts)
	}
	return r
}

func (r ObserveProviderCatalogCacheResult) ValidateFor(c ObserveProviderCatalogCacheCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || !cacheObservationEpoch(r.OwnerEpoch) || r.ObservedAt.IsZero() {
		return common.ErrConflict
	}
	if ValidateProviderCatalogCacheSnapshot(r.Catalog, r.ObservedAt) != nil {
		return common.ErrConflict
	}
	if r.Configuration != nil && r.Configuration.Validate() != nil {
		return common.ErrConflict
	}
	if r.Budget != nil {
		if r.Budget.Validate() != nil || r.Configuration == nil || r.Catalog.State != llmmodel.ProviderCatalogStateFresh {
			return common.ErrConflict
		}
		if c.Target != nil && r.Budget.Target != *c.Target || c.Target == nil && r.Budget.Target != r.Configuration.DefaultChat {
			return common.ErrConflict
		}
	}
	encoded, err := json.Marshal(r)
	if err != nil || len(encoded) > CatalogCacheObservationMaxBytes {
		return common.ErrConflict
	}
	return nil
}

// ValidateProviderCatalogCacheSnapshot bounds the complete cached declaration
// before an owner detaches or hashes it. It does not establish cache origin,
// lifecycle identity, current configuration or Workspace readiness.
func ValidateProviderCatalogCacheSnapshot(catalog llmmodel.ProviderModelCatalog, observedAt time.Time) error {
	if observedAt.IsZero() {
		return common.ErrConflict
	}
	if catalog.ProviderID != runtimecontract.BuiltinAIProxyProviderID || catalog.Source != "ai_proxy" || catalog.ErrorMessage != "" || len(catalog.Models) > CatalogCacheObservationModelLimit {
		return common.ErrConflict
	}
	switch catalog.State {
	case llmmodel.ProviderCatalogStateUnavailable:
		if catalog.Stale || catalog.ErrorCode != "cache_miss" || len(catalog.Models) != 0 || catalog.Revision != "" || !catalog.FetchedAt.IsZero() || !catalog.ExpiresAt.IsZero() {
			return common.ErrConflict
		}
	case llmmodel.ProviderCatalogStateFresh, llmmodel.ProviderCatalogStateStale:
		if !cacheObservationText(catalog.Revision, 128) || catalog.FetchedAt.IsZero() || catalog.FetchedAt.After(observedAt) || !catalog.ExpiresAt.After(catalog.FetchedAt) {
			return common.ErrConflict
		}
		fresh := observedAt.Before(catalog.ExpiresAt)
		if fresh != (catalog.State == llmmodel.ProviderCatalogStateFresh) || catalog.Stale == fresh || fresh && catalog.ErrorCode != "" || !fresh && catalog.ErrorCode != "cache_expired" {
			return common.ErrConflict
		}
	default:
		return common.ErrConflict
	}
	seen := make(map[string]bool, len(catalog.Models))
	for _, model := range catalog.Models {
		if model.Target.ProviderID != catalog.ProviderID || !cacheObservationText(model.Target.ModelID, 256) || model.Target != model.Target.Normalized() || seen[model.Target.ModelID] ||
			!cacheObservationText(model.RouteOwner, 128) || !cacheObservationText(model.CapabilityRevision, 128) || model.ContextWindowTokens < 0 || model.MaxOutputTokens < 0 ||
			!cacheObservationOptionalText(model.UnavailableReason, 128) || !cacheObservationOptionalText(model.Reasoning.DefaultEffort, 128) || !cacheObservationOptionalText(model.Reasoning.InvalidReason, 128) {
			return common.ErrConflict
		}
		seen[model.Target.ModelID] = true
		for _, values := range [][]string{model.SupportedEndpoints, model.Operations, model.Reasoning.Efforts} {
			if len(values) > 64 {
				return common.ErrConflict
			}
			for _, value := range values {
				if !cacheObservationText(value, 256) {
					return common.ErrConflict
				}
			}
		}
	}
	encoded, err := json.Marshal(catalog)
	if err != nil || len(encoded) > CatalogCacheObservationMaxBytes {
		return common.ErrConflict
	}
	return nil
}

func cacheObservationEpoch(value string) bool {
	const prefix = "llm-preparation-epoch:"
	if !strings.HasPrefix(value, prefix) || len(value) > 128 {
		return false
	}
	suffix := strings.TrimPrefix(value, prefix)
	// crypto/rand.Text returns at least 128 bits in unpadded base32. Do not
	// accept an empty namespace as a lifecycle identity, or require a fixed
	// length that would prevent a future Go release from increasing entropy.
	if len(suffix) < 26 {
		return false
	}
	for _, char := range suffix {
		if !(char >= 'A' && char <= 'Z' || char >= '2' && char <= '7') {
			return false
		}
	}
	return true
}

func cacheObservationOptionalText(value string, limit int) bool {
	return value == "" || cacheObservationText(value, limit)
}

func cacheObservationText(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}
