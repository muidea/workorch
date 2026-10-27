package events

import (
	"math"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparationProviderBudgetVersion = "workorch-preparation-provider-budget-observation/v1"

// An owner-computed capacity preview from the frozen configuration and the
// exact observed cache. It is NOT a Runtime ProviderCeiling, request allocation,
// authorization or readiness credential. Private configuration revisions never
// cross this boundary; the containing observation supplies its opaque epoch.
type PreparationProviderBudget struct {
	Version                  string
	Target                   runtimecontract.ProviderModelTarget
	ContextWindowTokens      int
	ProviderMaxOutputTokens  int
	EffectiveMaxOutputTokens int
	MaxInputTokens           int
	MaxRequestBytes          int
	EstimatedTokensPerByte   float64
	CapabilityRevision       string
	FailureCode              string
}

func (b PreparationProviderBudget) Validate() error {
	if b.Version != PreparationProviderBudgetVersion || b.Target.Validate() != nil || b.Target != b.Target.Normalized() || b.Target.ProviderID != runtimecontract.BuiltinAIProxyProviderID || !cacheObservationText(b.Target.ModelID, 256) {
		return common.ErrConflict
	}
	if b.FailureCode != "" {
		if b.FailureCode != "capacity_unavailable" || b.ContextWindowTokens != 0 || b.ProviderMaxOutputTokens != 0 || b.EffectiveMaxOutputTokens != 0 || b.MaxInputTokens != 0 || b.MaxRequestBytes != 0 || b.EstimatedTokensPerByte != 0 || b.CapabilityRevision != "" {
			return common.ErrConflict
		}
		return nil
	}
	if b.ContextWindowTokens <= 0 || b.ProviderMaxOutputTokens <= 0 || b.EffectiveMaxOutputTokens <= 0 || b.EffectiveMaxOutputTokens >= b.ContextWindowTokens || b.EffectiveMaxOutputTokens > b.ProviderMaxOutputTokens || b.MaxInputTokens <= 0 || b.MaxInputTokens > b.ContextWindowTokens-b.EffectiveMaxOutputTokens || b.MaxRequestBytes <= 0 || b.EstimatedTokensPerByte <= 0 || math.IsNaN(b.EstimatedTokensPerByte) || math.IsInf(b.EstimatedTokensPerByte, 0) || !cacheObservationText(b.CapabilityRevision, 128) {
		return common.ErrConflict
	}
	return nil
}
