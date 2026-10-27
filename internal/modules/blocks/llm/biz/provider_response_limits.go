package llm

import (
	"fmt"
	"math"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	// workorch.embedding-transport/v1 limits resources, not model token capacity.
	// The wire allowance covers JSON escaping of the entire input plus envelope.
	embeddingTransportPolicyVersion = "workorch.embedding-transport/v1"
	embeddingMaxInputBytes          = 64 * 1024
	embeddingMaxRequestBytes        = 512 * 1024
	embeddingMaxResponseBytes       = 4 * 1024 * 1024

	providerResponseEnvelopeOverheadBytes = 64 * 1024
	providerResponseWireExpansion         = 6
	providerStreamEventsPerOutputToken    = 4
	providerStreamEventCountOverhead      = 16
	providerStreamEventEnvelopeBytes      = 512
	providerStreamLineEnvelopeBytes       = 16 * 1024
)

type providerResponseLimits struct {
	MaxContentBytes       int
	MaxNonStreamWireBytes int
	MaxStreamWireBytes    int
	MaxDataEvents         int
	MaxLineBytes          int
}

func providerCompletionResponseLimits(config llmmodels.ProviderConfig, request llmmodels.ProviderRequest) (providerResponseLimits, error) {
	outputTokenLimit := maxOutputTokensForRequest(config, request)
	tokensPerByte := config.ContextProfile.EstimatedTokensPerByte
	if outputTokenLimit <= 0 || outputTokenLimit > int64(math.MaxInt) || tokensPerByte <= 0 || math.IsNaN(tokensPerByte) || math.IsInf(tokensPerByte, 0) {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity is incomplete", common.ErrInvalidArgument)
	}
	outputTokens := int(outputTokenLimit)
	contentFloat := math.Ceil(float64(outputTokens) / tokensPerByte)
	// float64(math.MaxInt) rounds upward on 64-bit platforms. Reject the
	// boundary value as well so the subsequent float-to-int conversion cannot
	// wrap before the integer guards take over.
	if contentFloat <= 0 || contentFloat >= float64(math.MaxInt) {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows byte limits", common.ErrInvalidArgument)
	}
	contentBytes := int(contentFloat)
	contentWireBytes, ok := safeProviderLimitMultiply(contentBytes, providerResponseWireExpansion)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows wire limits", common.ErrInvalidArgument)
	}
	nonStreamWireBytes, ok := safeProviderLimitAdd(contentWireBytes, providerResponseEnvelopeOverheadBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows wire limits", common.ErrInvalidArgument)
	}
	dataEvents, ok := safeProviderLimitMultiply(outputTokens, providerStreamEventsPerOutputToken)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows event limits", common.ErrInvalidArgument)
	}
	dataEvents, ok = safeProviderLimitAdd(dataEvents, providerStreamEventCountOverhead)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows event limits", common.ErrInvalidArgument)
	}
	eventEnvelopeBytes, ok := safeProviderLimitMultiply(dataEvents, providerStreamEventEnvelopeBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows stream envelope limits", common.ErrInvalidArgument)
	}
	maxLineBytes, ok := safeProviderLimitAdd(contentWireBytes, providerStreamLineEnvelopeBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows stream line limits", common.ErrInvalidArgument)
	}
	// A streaming response may carry the output once as deltas and once in a
	// terminal response object. Account for the aggregate escaped content, a
	// bounded envelope for every data event, one maximum-size terminal line,
	// and bounded non-data SSE framing independently from the non-stream JSON
	// response ceiling.
	streamWireBytes, ok := safeProviderLimitAdd(providerResponseEnvelopeOverheadBytes, contentWireBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows stream wire limits", common.ErrInvalidArgument)
	}
	streamWireBytes, ok = safeProviderLimitAdd(streamWireBytes, eventEnvelopeBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows stream wire limits", common.ErrInvalidArgument)
	}
	streamWireBytes, ok = safeProviderLimitAdd(streamWireBytes, maxLineBytes)
	if !ok {
		return providerResponseLimits{}, fmt.Errorf("%w: provider response capacity overflows stream wire limits", common.ErrInvalidArgument)
	}
	return providerResponseLimits{
		MaxContentBytes:       contentBytes,
		MaxNonStreamWireBytes: nonStreamWireBytes,
		MaxStreamWireBytes:    streamWireBytes,
		MaxDataEvents:         dataEvents,
		MaxLineBytes:          maxLineBytes,
	}, nil
}

func providerEmbeddingResponseLimit(config llmmodels.ProviderConfig, input string) (int, error) {
	if config.ContextProfile.MaxRequestBytes < 0 || config.ContextProfile.ContextWindowTokens < 0 {
		return 0, fmt.Errorf("%w: embedding capacity is invalid", common.ErrInvalidArgument)
	}
	maxInputBytes := embeddingMaxInputBytes
	if config.ContextProfile.MaxRequestBytes > 0 {
		maxInputBytes = min(maxInputBytes, config.ContextProfile.MaxRequestBytes)
	}
	if len(input) > maxInputBytes {
		return 0, fmt.Errorf("%w: embedding input exceeds local byte budget", common.ErrInvalidArgument)
	}
	return embeddingMaxResponseBytes, nil
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
