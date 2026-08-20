package llm

import (
	"fmt"
	"math"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	providerResponseEnvelopeOverheadBytes = 64 * 1024
	providerResponseWireExpansion         = 6
	providerStreamEventsPerOutputToken    = 4
	providerStreamEventEnvelope           = 16
)

type providerResponseLimits struct {
	MaxContentBytes int
	MaxWireBytes    int
	MaxDataEvents   int
	MaxLineBytes    int
}

func providerCompletionResponseLimits(config llmmodels.ProviderConfig, request llmmodels.ProviderRequest) (providerResponseLimits, error) {
	outputTokens := int(maxOutputTokensForRequest(config, request))
	tokensPerByte := config.ContextProfile.EstimatedTokensPerByte
	if outputTokens <= 0 || tokensPerByte <= 0 || math.IsNaN(tokensPerByte) || math.IsInf(tokensPerByte, 0) {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity is incomplete", common.ErrInvalidArgument)
	}
	contentFloat := math.Ceil(float64(outputTokens) / tokensPerByte)
	if contentFloat <= 0 || contentFloat > float64(math.MaxInt) {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows byte limits", common.ErrInvalidArgument)
	}
	contentBytes := int(contentFloat)
	wireBytes, ok := safeProviderLimitMultiply(contentBytes, providerResponseWireExpansion)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows wire limits", common.ErrInvalidArgument)
	}
	wireBytes, ok = safeProviderLimitAdd(wireBytes, providerResponseEnvelopeOverheadBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows wire limits", common.ErrInvalidArgument)
	}
	dataEvents, ok := safeProviderLimitMultiply(outputTokens, providerStreamEventsPerOutputToken)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows event limits", common.ErrInvalidArgument)
	}
	dataEvents, ok = safeProviderLimitAdd(dataEvents, providerStreamEventEnvelope)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows event limits", common.ErrInvalidArgument)
	}
	return providerResponseLimits{
		MaxContentBytes: contentBytes,
		MaxWireBytes:    wireBytes,
		MaxDataEvents:   dataEvents,
		MaxLineBytes:    wireBytes,
	}, nil
}

func providerEmbeddingResponseLimit(config llmmodels.ProviderConfig, input string) (int, error) {
	maxRequestBytes := config.ContextProfile.MaxRequestBytes
	if maxRequestBytes <= 0 {
		return 0, fmt.Errorf("%w: embedding model byte capacity is incomplete", common.ErrInvalidArgument)
	}
	if len(input) > maxRequestBytes {
		return 0, fmt.Errorf("%w: embedding input exceeds selected model byte capacity", common.ErrInvalidArgument)
	}
	limit, ok := safeProviderLimitMultiply(maxRequestBytes, providerStreamEventsPerOutputToken)
	if !ok {
		return 0, fmt.Errorf("%w: embedding response capacity overflows wire limits", common.ErrInvalidArgument)
	}
	limit, ok = safeProviderLimitAdd(limit, providerResponseEnvelopeOverheadBytes)
	if !ok {
		return 0, fmt.Errorf("%w: embedding response capacity overflows wire limits", common.ErrInvalidArgument)
	}
	return limit, nil
}

func safeProviderLimitMultiply(value, multiplier int) (int, bool) {
	if value < 0 || multiplier < 0 || (value > 0 && multiplier > math.MaxInt/value) {
		return 0, false
	}
	return value * multiplier, true
}

func safeProviderLimitAdd(left, right int) (int, bool) {
	if left < 0 || right < 0 || left > math.MaxInt-right {
		return 0, false
	}
	return left + right, true
}
