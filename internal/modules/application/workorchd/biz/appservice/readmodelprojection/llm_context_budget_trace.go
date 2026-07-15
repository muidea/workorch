package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmContextBudgetTraceFromStep(step StepView, budget workorcheventpayload.Payload, usage workorcheventpayload.Payload) readmodelcore.LLMContextBudgetTrace {
	return readmodelcore.LLMContextBudgetTrace{
		StepID:                step.ID,
		ProviderID:            llmContextBudgetProviderID(budget, usage),
		Model:                 llmContextBudgetModel(budget, usage),
		CapabilityRevision:    llmContextBudgetCapabilityRevision(budget),
		ConfigurationRevision: llmContextBudgetConfigurationRevision(budget),
		MaxBytes:              llmContextBudgetMaxBytes(budget),
		ActualBytes:           llmContextBudgetActualBytes(budget),
		MaxTokens:             llmContextBudgetMaxTokens(budget),
		EstimatedInputTokens:  llmContextBudgetEstimatedInputTokens(budget),
		EstimatedOutputTokens: llmContextBudgetEstimatedOutputTokens(budget),
		ActualTokens:          llmContextBudgetActualTokens(budget),
		ShrinkLevel:           llmContextBudgetShrinkLevel(budget),
		Truncated:             llmContextBudgetTruncated(budget),
		Rules:                 llmContextBudgetRules(budget),
		SourceRefs:            llmContextBudgetSourceRefs(budget),
		IncludedSections:      llmContextBudgetIncludedSections(budget),
		NormalizedSections:    llmContextBudgetNormalizedSections(budget),
		CompressedSections:    llmContextBudgetCompressedSections(budget),
		OmittedSections:       llmContextBudgetOmittedSections(budget),
		GoalDoDCompressed:     llmContextBudgetGoalDoDCompressed(budget),
		OverBudget:            llmContextBudgetOverBudget(budget),
		P0Overflow:            llmContextBudgetP0Overflow(budget),
		ProfileMode:           llmContextBudgetProfileMode(budget),
		CapabilitySource:      llmContextBudgetCapabilitySource(budget),
		ContextUtilization:    llmContextBudgetContextUtilization(budget),
		MaxRequestBytes:       llmContextBudgetMaxRequestBytes(budget),
		StageBudgetRatio:      llmContextBudgetStageBudgetRatio(budget),
		StageBudgetSource:     llmContextBudgetStageBudgetSource(budget),
		TracePolicy:           llmContextBudgetTracePolicy(budget),
		PromptCaptured:        llmContextBudgetPromptCaptured(budget),
		Estimator:             llmContextBudgetEstimator(budget),
		Profile:               llmContextBudgetProfile(budget),
		CachePlan:             llmContextBudgetCachePlan(budget),
	}
}
