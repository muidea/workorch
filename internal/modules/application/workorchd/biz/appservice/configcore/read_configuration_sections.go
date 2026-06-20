package configcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func serverConfiguration(cfg workorchconfig.Config) ServerConfiguration {
	return ServerConfiguration{
		Addr:                      cfg.Server.Addr,
		ReadHeaderTimeout:         cfg.Server.ReadHeaderTimeout,
		ReadTimeout:               cfg.Server.ReadTimeout,
		WriteTimeout:              cfg.Server.WriteTimeout,
		IdleTimeout:               cfg.Server.IdleTimeout,
		ShutdownTimeout:           cfg.Server.ShutdownTimeout,
		MaxConcurrentRuns:         cfg.Server.MaxConcurrentRuns,
		StaleDispatchedStepMinAge: strings.TrimSpace(cfg.Server.StaleDispatchedStepMinAge),
		DefaultDialogLanguage:     strings.TrimSpace(cfg.Server.DefaultDialogLanguage),
	}
}

func repositoryConfiguration(cfg workorchconfig.Config) RepositoryConfiguration {
	return RepositoryConfiguration{
		Backend: cfg.Repository.Backend,
		DataDir: cfg.Repository.DataDir,
	}
}

func runtimeConfiguration(cfg workorchconfig.Config) RuntimeConfiguration {
	policy := workorchruntimepolicy.NormalizeRuntimePolicy(workorchruntimepolicy.RuntimePolicy{
		MaxRuntimeRepairAttempts:                     cfg.Runtime.MaxRuntimeRepairAttempts,
		MaxProviderRetryAttempts:                     cfg.Runtime.MaxProviderRetryAttempts,
		MaxContractViolationRetriesPerStep:           cfg.Runtime.MaxContractViolationRetriesPerStep,
		MaxContractViolationRetriesPerRun:            cfg.Runtime.MaxContractViolationRetriesPerRun,
		MaxRepeatedContractViolationSignature:        cfg.Runtime.MaxRepeatedContractViolationSignature,
		MaxConsecutiveContractViolationNoShapeChange: cfg.Runtime.MaxConsecutiveContractViolationNoShapeChange,
	})
	return RuntimeConfiguration{
		DefaultSoulID:                                strings.TrimSpace(cfg.Runtime.DefaultSoulID),
		MaxRuntimeRepairAttempts:                     policy.MaxRuntimeRepairAttempts,
		MaxProviderRetryAttempts:                     policy.MaxProviderRetryAttempts,
		MaxContractViolationRetriesPerStep:           policy.MaxContractViolationRetriesPerStep,
		MaxContractViolationRetriesPerRun:            policy.MaxContractViolationRetriesPerRun,
		MaxRepeatedContractViolationSignature:        policy.MaxRepeatedContractViolationSignature,
		MaxConsecutiveContractViolationNoShapeChange: policy.MaxConsecutiveContractViolationNoShapeChange,
		ShellCommandTimeout:                          runtimeShellCommandTimeout(cfg.Runtime.ShellCommandTimeout),
		Git:                                          runtimeGitConfiguration(cfg.Runtime.Git),
		LLMOutput:                                    runtimeLLMOutputConfiguration(cfg.Runtime.LLMOutput),
	}
}

func defaultEmbeddingProviderID(cfg workorchconfig.Config) string {
	return strings.TrimSpace(cfg.LLM.DefaultEmbeddingProvider)
}

func governanceConfiguration(cfg workorchconfig.Config) GovernanceConfiguration {
	return GovernanceConfiguration{
		AutoTicket: GovernanceAutoTicketConfiguration{
			Enabled:       isEnabled(cfg.Governance.AutoTicket.Enabled),
			MinConfidence: cfg.Governance.AutoTicket.MinConfidence,
			MaxPerRun:     cfg.Governance.AutoTicket.MaxPerRun,
		},
		AutoApproval: GovernanceAutoApprovalConfiguration{
			Enabled:             isEnabled(cfg.Governance.AutoApproval.Enabled),
			MinConfidence:       cfg.Governance.AutoApproval.MinConfidence,
			MaxPerRun:           cfg.Governance.AutoApproval.MaxPerRun,
			AllowReadOnly:       isEnabled(cfg.Governance.AutoApproval.AllowReadOnly),
			AllowWorkspaceWrite: isEnabled(cfg.Governance.AutoApproval.AllowWorkspaceWrite),
			RequireHumanFor:     append([]string(nil), cfg.Governance.AutoApproval.RequireHumanFor...),
		},
		Budget: GovernanceBudgetConfiguration{
			MaxConsecutiveFailures: cfg.Governance.Budget.MaxConsecutiveFailures,
			MaxRepeatedSamePrompt:  cfg.Governance.Budget.MaxRepeatedSamePrompt,
		},
	}
}
