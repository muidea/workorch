package server

import "fmt"

func validateWorkspaceApplicationPorts(ports WorkspacePorts) error {
	if ports.Validate == nil {
		return fmt.Errorf("missing Workspace.Validate application port")
	}
	if ports.DeleteWorkspace == nil {
		return fmt.Errorf("missing Workspace.DeleteWorkspace application port")
	}
	if ports.ReadWorkspace == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspace application port")
	}
	if ports.ReadWorkspaceSnapshot == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspaceSnapshot application port")
	}
	if ports.ReadWorkspaceDetailSnapshot == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspaceDetailSnapshot application port")
	}
	if ports.ReadProjectionSnapshot == nil {
		return fmt.Errorf("missing Workspace.ReadProjectionSnapshot application port")
	}
	if ports.ReadWorkspaceSoulBindings == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspaceSoulBindings application port")
	}
	if ports.UpdateWorkspaceSoulBindings == nil {
		return fmt.Errorf("missing Workspace.UpdateWorkspaceSoulBindings application port")
	}
	if ports.UpdateWorkspaceLLMSelection == nil {
		return fmt.Errorf("missing Workspace.UpdateWorkspaceLLMSelection application port")
	}
	if ports.ListWorkspaces == nil {
		return fmt.Errorf("missing Workspace.ListWorkspaces application port")
	}
	if ports.ReadStartupPreparation == nil {
		return fmt.Errorf("missing Workspace.ReadStartupPreparation application port")
	}
	if ports.CheckStartupPreparation == nil {
		return fmt.Errorf("missing Workspace.CheckStartupPreparation application port")
	}
	if ports.RecoverStartupPreparation == nil {
		return fmt.Errorf("missing Workspace.RecoverStartupPreparation application port")
	}
	return nil
}

func validateDefinitionApplicationPorts(ports DefinitionPorts) error {
	if ports.CreateDefinitionAuthoring == nil {
		return fmt.Errorf("missing Definition.CreateDefinitionAuthoring application port")
	}
	if ports.ReadDefinitionAuthoring == nil {
		return fmt.Errorf("missing Definition.ReadDefinitionAuthoring application port")
	}
	if ports.SaveDefinitionAuthoring == nil {
		return fmt.Errorf("missing Definition.SaveDefinitionAuthoring application port")
	}
	if ports.CompileDefinitionAuthoring == nil {
		return fmt.Errorf("missing Definition.CompileDefinitionAuthoring application port")
	}
	if ports.ConfirmDefinitionAuthoring == nil {
		return fmt.Errorf("missing Definition.ConfirmDefinitionAuthoring application port")
	}
	if ports.ReadDefinitionAuthoringDiagnostic == nil {
		return fmt.Errorf("missing Definition.ReadDefinitionAuthoringDiagnostic application port")
	}
	if ports.ReadDefinitionAuthoringSuccessAudit == nil {
		return fmt.Errorf("missing Definition.ReadDefinitionAuthoringSuccessAudit application port")
	}
	if ports.ReadDefinitionAuthoringPreparationDiagnostic == nil {
		return fmt.Errorf("missing Definition.ReadDefinitionAuthoringPreparationDiagnostic application port")
	}
	if ports.ReadGoal == nil {
		return fmt.Errorf("missing Definition.ReadGoal application port")
	}
	if ports.ListGoals == nil {
		return fmt.Errorf("missing Definition.ListGoals application port")
	}
	if ports.ReadDoD == nil {
		return fmt.Errorf("missing Definition.ReadDoD application port")
	}
	if ports.ListDoDs == nil {
		return fmt.Errorf("missing Definition.ListDoDs application port")
	}
	return nil
}

func validateRunApplicationPorts(ports RunPorts) error {
	if ports.ReadRun == nil {
		return fmt.Errorf("missing Run.ReadRun application port")
	}
	if ports.ReadRunSnapshot == nil {
		return fmt.Errorf("missing Run.ReadRunSnapshot application port")
	}
	if ports.ListRunSteps == nil {
		return fmt.Errorf("missing Run.ListRunSteps application port")
	}
	if ports.ListRunToolCalls == nil {
		return fmt.Errorf("missing Run.ListRunToolCalls application port")
	}
	if ports.ListRunRuntimeCheckpoints == nil {
		return fmt.Errorf("missing Run.ListRunRuntimeCheckpoints application port")
	}
	if ports.ListRunResults == nil {
		return fmt.Errorf("missing Run.ListRunResults application port")
	}
	if ports.ListRunPatches == nil {
		return fmt.Errorf("missing Run.ListRunPatches application port")
	}
	if ports.ListRunValidationResults == nil {
		return fmt.Errorf("missing Run.ListRunValidationResults application port")
	}
	if ports.ListRunArtifacts == nil {
		return fmt.Errorf("missing Run.ListRunArtifacts application port")
	}
	if ports.ReadRunStatus == nil {
		return fmt.Errorf("missing Run.ReadRunStatus application port")
	}
	if ports.ReadRunObservability == nil {
		return fmt.Errorf("missing Run.ReadRunObservability application port")
	}
	if ports.ReadArtifactContent == nil {
		return fmt.Errorf("missing Run.ReadArtifactContent application port")
	}
	if ports.ReadRuntimeRawContent == nil {
		return fmt.Errorf("missing Run.ReadRuntimeRawContent application port")
	}
	if ports.ReadRunTaskProgress == nil {
		return fmt.Errorf("missing Run.ReadRunTaskProgress application port")
	}
	if ports.ReadRunContextBundle == nil {
		return fmt.Errorf("missing Run.ReadRunContextBundle application port")
	}
	if ports.ReadRunTaskDecompositions == nil {
		return fmt.Errorf("missing Run.ReadRunTaskDecompositions application port")
	}
	if ports.ListEventRecords == nil {
		return fmt.Errorf("missing Run.ListEventRecords application port")
	}
	if ports.ListRuns == nil {
		return fmt.Errorf("missing Run.ListRuns application port")
	}
	return nil
}

func validateGovernanceApplicationPorts(ports GovernancePorts) error {
	if ports.ListDecisions == nil {
		return fmt.Errorf("missing Governance.ListDecisions application port")
	}
	if ports.ListDecisionPage == nil {
		return fmt.Errorf("missing Governance.ListDecisionPage application port")
	}
	if ports.ReadDecision == nil {
		return fmt.Errorf("missing Governance.ReadDecision application port")
	}
	if ports.ResolveDecision == nil {
		return fmt.Errorf("missing Governance.ResolveDecision application port")
	}
	if ports.BatchResolveDecisions == nil {
		return fmt.Errorf("missing Governance.BatchResolveDecisions application port")
	}
	if ports.ListApprovals == nil {
		return fmt.Errorf("missing Governance.ListApprovals application port")
	}
	if ports.ListApprovalPage == nil {
		return fmt.Errorf("missing Governance.ListApprovalPage application port")
	}
	if ports.ReadApproval == nil {
		return fmt.Errorf("missing Governance.ReadApproval application port")
	}
	if ports.ApproveApproval == nil {
		return fmt.Errorf("missing Governance.ApproveApproval application port")
	}
	if ports.RejectApproval == nil {
		return fmt.Errorf("missing Governance.RejectApproval application port")
	}
	if ports.BatchResolveApprovals == nil {
		return fmt.Errorf("missing Governance.BatchResolveApprovals application port")
	}
	return nil
}

func validateRegistryApplicationPorts(ports RegistryPorts) error {
	if ports.ListBuiltinDefaults == nil {
		return fmt.Errorf("missing Registry.ListBuiltinDefaults application port")
	}
	if ports.UpgradeBuiltinDefault == nil {
		return fmt.Errorf("missing Registry.UpgradeBuiltinDefault application port")
	}
	if ports.CreateSoul == nil {
		return fmt.Errorf("missing Registry.CreateSoul application port")
	}
	if ports.UpdateSoul == nil {
		return fmt.Errorf("missing Registry.UpdateSoul application port")
	}
	if ports.DeleteSoul == nil {
		return fmt.Errorf("missing Registry.DeleteSoul application port")
	}
	if ports.ReadSoul == nil {
		return fmt.Errorf("missing Registry.ReadSoul application port")
	}
	if ports.ListSouls == nil {
		return fmt.Errorf("missing Registry.ListSouls application port")
	}
	if ports.CreateWorkspaceTemplate == nil {
		return fmt.Errorf("missing Registry.CreateWorkspaceTemplate application port")
	}
	if ports.UpdateWorkspaceTemplate == nil {
		return fmt.Errorf("missing Registry.UpdateWorkspaceTemplate application port")
	}
	if ports.DeleteWorkspaceTemplate == nil {
		return fmt.Errorf("missing Registry.DeleteWorkspaceTemplate application port")
	}
	if ports.ListWorkspaceTemplates == nil {
		return fmt.Errorf("missing Registry.ListWorkspaceTemplates application port")
	}
	if ports.CreateSkill == nil {
		return fmt.Errorf("missing Registry.CreateSkill application port")
	}
	if ports.ReadSkill == nil {
		return fmt.Errorf("missing Registry.ReadSkill application port")
	}
	if ports.ListSkills == nil {
		return fmt.Errorf("missing Registry.ListSkills application port")
	}
	if ports.CreateMCPServer == nil {
		return fmt.Errorf("missing Registry.CreateMCPServer application port")
	}
	if ports.UpdateMCPServer == nil {
		return fmt.Errorf("missing Registry.UpdateMCPServer application port")
	}
	if ports.ReadMCPServer == nil {
		return fmt.Errorf("missing Registry.ReadMCPServer application port")
	}
	if ports.ListMCPServers == nil {
		return fmt.Errorf("missing Registry.ListMCPServers application port")
	}
	if ports.ListMCPServerTools == nil {
		return fmt.Errorf("missing Registry.ListMCPServerTools application port")
	}
	if ports.ListMCPServerResources == nil {
		return fmt.Errorf("missing Registry.ListMCPServerResources application port")
	}
	if ports.ListMCPServerPrompts == nil {
		return fmt.Errorf("missing Registry.ListMCPServerPrompts application port")
	}
	if ports.CreateMCPCapability == nil {
		return fmt.Errorf("missing Registry.CreateMCPCapability application port")
	}
	if ports.UpdateMCPCapability == nil {
		return fmt.Errorf("missing Registry.UpdateMCPCapability application port")
	}
	if ports.ReadMCPCapability == nil {
		return fmt.Errorf("missing Registry.ReadMCPCapability application port")
	}
	if ports.ListMCPCapabilities == nil {
		return fmt.Errorf("missing Registry.ListMCPCapabilities application port")
	}
	return nil
}

func validateMemoryApplicationPorts(ports MemoryPorts) error {
	if ports.CreateMemoryItem == nil {
		return fmt.Errorf("missing Memory.CreateMemoryItem application port")
	}
	if ports.ReadMemoryItem == nil {
		return fmt.Errorf("missing Memory.ReadMemoryItem application port")
	}
	if ports.UpdateMemoryItem == nil {
		return fmt.Errorf("missing Memory.UpdateMemoryItem application port")
	}
	if ports.DeleteMemoryItem == nil {
		return fmt.Errorf("missing Memory.DeleteMemoryItem application port")
	}
	if ports.ListMemoryByWorkspace == nil {
		return fmt.Errorf("missing Memory.ListMemoryByWorkspace application port")
	}
	if ports.SearchMemory == nil {
		return fmt.Errorf("missing Memory.SearchMemory application port")
	}
	if ports.ListMemoryRecallTraces == nil {
		return fmt.Errorf("missing Memory.ListMemoryRecallTraces application port")
	}
	return nil
}

func validateConfigurationApplicationPorts(ports ConfigurationPorts) error {
	if ports.ReadConfiguration == nil {
		return fmt.Errorf("missing Configuration.ReadConfiguration application port")
	}
	if ports.UpdateServerConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateServerConfiguration application port")
	}
	if ports.UpdateRuntimeConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateRuntimeConfiguration application port")
	}
	if ports.UpdateLLMConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateLLMConfiguration application port")
	}
	if ports.GetLLMModelCatalog == nil {
		return fmt.Errorf("missing Configuration.GetLLMModelCatalog application port")
	}
	if ports.RefreshLLMModelCatalog == nil {
		return fmt.Errorf("missing Configuration.RefreshLLMModelCatalog application port")
	}
	if ports.ReadConfigurationWithCatalog == nil {
		return fmt.Errorf("missing Configuration.ReadConfigurationWithCatalog application port")
	}
	if ports.UpdateWebSearchConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateWebSearchConfiguration application port")
	}
	return nil
}
