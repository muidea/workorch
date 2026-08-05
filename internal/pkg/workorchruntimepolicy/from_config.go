package workorchruntimepolicy

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func Current() (RuntimePolicy, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return RuntimePolicy{}, err
	}
	return FromConfig(cfg), nil
}

func FromConfig(cfg workorchconfig.Config) RuntimePolicy {
	return RuntimePolicy{
		MaxRuntimeRepairAttempts:                     cfg.Runtime.MaxRuntimeRepairAttempts,
		MaxProviderRetryAttempts:                     cfg.Runtime.MaxProviderRetryAttempts,
		MaxContractViolationRetriesPerStep:           cfg.Runtime.MaxContractViolationRetriesPerStep,
		MaxContractViolationRetriesPerRun:            cfg.Runtime.MaxContractViolationRetriesPerRun,
		MaxRepeatedContractViolationSignature:        cfg.Runtime.MaxRepeatedContractViolationSignature,
		MaxConsecutiveContractViolationNoShapeChange: cfg.Runtime.MaxConsecutiveContractViolationNoShapeChange,
		LLMOutput: LLMOutputRuntimePolicy(cfg.Runtime.LLMOutput),
	}
}
