package events

import (
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparationProviderConfigurationVersion = "workorch-preparation-provider-configuration/v1"

// v2 checks the frozen local configuration used by activation. Catalog
// availability and a separate remote authorization receipt are not prerequisites.
const PreparationProviderConfigurationSnapshotVersion = "workorch-preparation-provider-configuration/v2"

// Complete non-secret selection declarations of one frozen owner lifecycle.
// OwnerEpoch on the containing observation binds these to its private active
// configuration, including credentials. No credential-derived revision crosses
// this boundary. These declarations do not prove remote authorization.
type PreparationProviderConfiguration struct {
	Version     string
	DefaultChat runtimecontract.ProviderModelTarget
	Overrides   []PreparationProviderModelOverride
}

type PreparationProviderModelOverride struct {
	ModelID             string
	ContextWindowTokens int
	MaxOutputTokens     int
	Operations          []string
}

func (c PreparationProviderConfiguration) Clone() PreparationProviderConfiguration {
	c.Overrides = slices.Clone(c.Overrides)
	for i := range c.Overrides {
		c.Overrides[i].Operations = slices.Clone(c.Overrides[i].Operations)
	}
	return c
}

func (c PreparationProviderConfiguration) Validate() error {
	if (c.Version != PreparationProviderConfigurationVersion && c.Version != PreparationProviderConfigurationSnapshotVersion) || c.DefaultChat.Validate() != nil || c.DefaultChat != c.DefaultChat.Normalized() || c.DefaultChat.ProviderID != runtimecontract.BuiltinAIProxyProviderID || !cacheObservationText(c.DefaultChat.ModelID, 256) || len(c.Overrides) > CatalogCacheObservationModelLimit {
		return common.ErrConflict
	}
	previous := ""
	for _, o := range c.Overrides {
		if !cacheObservationText(o.ModelID, 256) || o.ModelID <= previous || o.ContextWindowTokens <= 0 || o.MaxOutputTokens <= 0 || o.MaxOutputTokens >= o.ContextWindowTokens || len(o.Operations) == 0 || len(o.Operations) > 64 {
			return common.ErrConflict
		}
		previous = o.ModelID
		seen := map[string]bool{}
		for _, operation := range o.Operations {
			if (operation != "responses" && operation != "embeddings") || seen[operation] {
				return common.ErrConflict
			}
			seen[operation] = true
		}
	}
	return nil
}
