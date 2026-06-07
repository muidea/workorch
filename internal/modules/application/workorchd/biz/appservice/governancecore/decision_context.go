package governancecore

const (
	decisionContextKeyScope                 = "scope"
	decisionContextScopeWorkorchRuntime     = "workorch_runtime"
	decisionContextScopeTargetWorkspace     = "target_workspace"
	decisionContextScopeExternalDependency  = "external_dependency"
	decisionContextKeyDependency            = "dependency"
	decisionContextDependencyLLMProvider    = "llm_provider"
	decisionContextKeyRecoveryPolicy        = "recoveryPolicy"
	decisionContextRecoveryPolicyConfigOnly = "system_config_required"
	decisionContextRecoveryPolicyRepairPlan = "runtime_repair_exhausted_requires_strategy"
)

const (
	DecisionContextKeyScope                 = decisionContextKeyScope
	DecisionContextScopeWorkorchRuntime     = decisionContextScopeWorkorchRuntime
	DecisionContextScopeTargetWorkspace     = decisionContextScopeTargetWorkspace
	DecisionContextScopeExternalDependency  = decisionContextScopeExternalDependency
	DecisionContextKeyDependency            = decisionContextKeyDependency
	DecisionContextDependencyLLMProvider    = decisionContextDependencyLLMProvider
	DecisionContextKeyRecoveryPolicy        = decisionContextKeyRecoveryPolicy
	DecisionContextRecoveryPolicyConfigOnly = decisionContextRecoveryPolicyConfigOnly
	DecisionContextRecoveryPolicyRepairPlan = decisionContextRecoveryPolicyRepairPlan
)
