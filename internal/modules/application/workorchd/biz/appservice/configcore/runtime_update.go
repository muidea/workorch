package configcore

import (
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
)

func UpdateRuntimeConfiguration(cmd UpdateRuntimeConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	if cmd.MaxRuntimeRepairAttempts != nil {
		if *cmd.MaxRuntimeRepairAttempts <= 0 {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: maxRuntimeRepairAttempts must be greater than zero", common.ErrInvalidArgument)
		}
		cfg.Runtime.MaxRuntimeRepairAttempts = *cmd.MaxRuntimeRepairAttempts
	}
	if cmd.MaxProviderRetryAttempts != nil {
		if *cmd.MaxProviderRetryAttempts <= 0 {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: maxProviderRetryAttempts must be greater than zero", common.ErrInvalidArgument)
		}
		cfg.Runtime.MaxProviderRetryAttempts = *cmd.MaxProviderRetryAttempts
	}
	if cmd.MaxContractViolationRetriesPerStep != nil {
		if *cmd.MaxContractViolationRetriesPerStep <= 0 {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: maxContractViolationRetriesPerStep must be greater than zero", common.ErrInvalidArgument)
		}
		cfg.Runtime.MaxContractViolationRetriesPerStep = *cmd.MaxContractViolationRetriesPerStep
	}
	if cmd.MaxContractViolationRetriesPerRun != nil {
		if *cmd.MaxContractViolationRetriesPerRun <= 0 {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: maxContractViolationRetriesPerRun must be greater than zero", common.ErrInvalidArgument)
		}
		cfg.Runtime.MaxContractViolationRetriesPerRun = *cmd.MaxContractViolationRetriesPerRun
	}
	if cmd.MaxRepeatedContractViolationSignature != nil {
		if *cmd.MaxRepeatedContractViolationSignature <= 0 {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: maxRepeatedContractViolationSignature must be greater than zero", common.ErrInvalidArgument)
		}
		cfg.Runtime.MaxRepeatedContractViolationSignature = *cmd.MaxRepeatedContractViolationSignature
	}
	if cmd.MaxConsecutiveContractViolationNoShapeChange != nil {
		if *cmd.MaxConsecutiveContractViolationNoShapeChange <= 0 {
			return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: maxConsecutiveContractViolationNoShapeChange must be greater than zero", common.ErrInvalidArgument)
		}
		cfg.Runtime.MaxConsecutiveContractViolationNoShapeChange = *cmd.MaxConsecutiveContractViolationNoShapeChange
	}
	if cmd.ShellCommandTimeout != nil {
		timeout := strings.TrimSpace(*cmd.ShellCommandTimeout)
		if timeout != "" {
			parsed, err := time.ParseDuration(timeout)
			if err != nil || parsed <= 0 {
				return workorchconfigurationcontract.ServiceConfiguration{}, fmt.Errorf("%w: shellCommandTimeout must be a positive duration", common.ErrInvalidArgument)
			}
		}
		cfg.Runtime.ShellCommandTimeout = timeout
	}
	if cmd.LLMOutput != nil {
		cfg.Runtime.LLMOutput = applyRuntimeLLMOutputUpdate(cfg.Runtime.LLMOutput, *cmd.LLMOutput)
		normalized, err := workorchconfig.NormalizeLLMOutputRuntimeConfig(cfg.Runtime.LLMOutput)
		if err != nil {
			return workorchconfigurationcontract.ServiceConfiguration{}, err
		}
		cfg.Runtime.LLMOutput = normalized
	}
	if err := workorchconfig.Save(cfg); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}

func applyRuntimeLLMOutputUpdate(current workorchconfig.LLMOutputRuntimeConfig, update UpdateRuntimeLLMOutputConfiguration) workorchconfig.LLMOutputRuntimeConfig {
	applyInt := func(target *int, value *int) {
		if value != nil {
			*target = *value
		}
	}
	applyInt(&current.MaxPatchContentBytes, update.MaxPatchContentBytes)
	applyInt(&current.MaxGraphBytes, update.MaxGraphBytes)
	applyInt(&current.MaxGraphFileWrites, update.MaxGraphFileWrites)
	applyInt(&current.SchemaInvalidLargePreviewThreshold, update.SchemaInvalidLargePreviewThreshold)
	applyInt(&current.MaxArtifactChunkBytes, update.MaxArtifactChunkBytes)
	applyInt(&current.PreviousChunkTailBytes, update.PreviousChunkTailBytes)
	applyInt(&current.MaxChunkRetryAttempts, update.MaxChunkRetryAttempts)
	applyInt(&current.MaxArtifactPlanAttempts, update.MaxArtifactPlanAttempts)
	applyInt(&current.MaxArtifactReplanAttempts, update.MaxArtifactReplanAttempts)
	return current
}
