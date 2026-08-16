package workorchruntimepolicy

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestLLMOutputRuntimePolicySharesConfigShapeWithoutSharingSemantics(t *testing.T) {
	cfg := workorchconfig.LLMOutputRuntimeConfig{
		MaxDocumentRecoveryAttempts: 107,
	}
	policy := LLMOutputRuntimePolicy(cfg)
	if got := workorchconfig.LLMOutputRuntimeConfig(policy); got != cfg {
		t.Fatalf("policy conversion = %+v, want %+v", got, cfg)
	}

	defaults := workorchconfig.DefaultLLMOutputRuntimeConfig()
	if got := workorchconfig.LLMOutputRuntimeConfig(NormalizeLLMOutputRuntimePolicy(LLMOutputRuntimePolicy{})); got != defaults {
		t.Fatalf("zero policy normalization = %+v, want %+v", got, defaults)
	}
	negative := LLMOutputRuntimePolicy{
		MaxDocumentRecoveryAttempts: -1,
	}
	if got := workorchconfig.LLMOutputRuntimeConfig(NormalizeLLMOutputRuntimePolicy(negative)); got != defaults {
		t.Fatalf("negative policy normalization = %+v, want %+v", got, defaults)
	}
	if got := NormalizeLLMOutputRuntimePolicy(policy); got != policy {
		t.Fatalf("positive policy normalization = %+v, want %+v", got, policy)
	}
	if got, want := policy.Values(), workorchconfig.LLMOutputRuntimeConfigValues(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("policy values = %+v, want %+v", got, want)
	}
}

func TestFromConfigConvertsCanonicalLLMOutputLimitsAsOneValue(t *testing.T) {
	cfg := workorchconfig.Config{Runtime: workorchconfig.RuntimeConfig{LLMOutput: workorchconfig.DefaultLLMOutputRuntimeConfig()}}
	policy := FromConfig(cfg)
	if got := workorchconfig.LLMOutputRuntimeConfig(policy.LLMOutput); got != cfg.Runtime.LLMOutput {
		t.Fatalf("FromConfig LLM output = %+v, want %+v", got, cfg.Runtime.LLMOutput)
	}
}
