package server

import "fmt"

// The callback types declared in http_dependencies.go are the single typed
// application boundary used by both Biz and the transport. They are distinct
// protocol contracts, not aliases of owner DTOs or compatibility facades.

// ApplicationPorts groups the narrow callbacks consumed by the HTTP adapter.
// Each group is independently replaceable in tests and in the production Biz
// wiring; no group is allowed to carry EventHub or repository handles.
type ApplicationPorts struct {
	Workspace     WorkspacePorts
	Definition    DefinitionPorts
	Run           RunPorts
	Governance    GovernancePorts
	Registry      RegistryPorts
	Memory        MemoryPorts
	Configuration ConfigurationPorts
}

type WorkspacePorts struct {
	Validate                    ValidateFunc
	CreateWorkspace             CreateWorkspaceFunc
	DeleteWorkspace             DeleteWorkspaceFunc
	ReadWorkspace               ReadWorkspaceFunc
	ReadWorkspaceSnapshot       ReadWorkspaceSnapshotFunc
	ReadWorkspaceDetailSnapshot ReadWorkspaceSnapshotFunc
	ReadProjectionSnapshot      ReadProjectionSnapshotFunc
	ReadWorkspaceSoulBindings   ReadWorkspaceSoulBindingsFunc
	UpdateWorkspaceSoulBindings UpdateWorkspaceSoulBindingsFunc
	UpdateWorkspaceLLMSelection UpdateWorkspaceLLMSelectionFunc
	ListWorkspaces              ListWorkspacesFunc
}

type DefinitionPorts struct {
	DefineGoal                  DefineGoalFunc
	UpdateGoal                  UpdateGoalFunc
	ReadGoal                    ReadGoalFunc
	ListGoals                   ListGoalsFunc
	DefineDoD                   DefineDoDFunc
	UpdateDoD                   UpdateDoDFunc
	ReadDoD                     ReadDoDFunc
	ListDoDs                    ListDoDsFunc
	CheckGoalDoDQuality         CheckGoalDoDQualityFunc
	GenerateWorkspaceSetupDraft GenerateWorkspaceSetupDraftFunc
}

type RunPorts struct {
	ReadRun                   ReadRunInstanceFunc
	ReadRunSnapshot           ReadRunSnapshotFunc
	ListRunSteps              ListRunStepsFunc
	ListRunToolCalls          ListRunToolCallsFunc
	ListRunRuntimeCheckpoints ListRunRuntimeCheckpointsFunc
	ListRunResults            ListRunResultsFunc
	ListRunPatches            ListRunPatchesFunc
	ListRunValidationResults  ListRunValidationResultsFunc
	ListRunArtifacts          ListRunArtifactsFunc
	ReadRunStatus             ReadRunStatusFunc
	ReadRunObservability      ReadRunObservabilityFunc
	ReadArtifactContent       ReadArtifactContentFunc
	ReadRuntimeRawContent     ReadRuntimeRawContentFunc
	ReadRunTaskProgress       ReadRunTaskProgressFunc
	ReadRunContextBundle      ReadRunContextBundleFunc
	ReadRunTaskDecompositions ReadRunTaskDecompositionsFunc
	ListEventRecords          ListEventRecordsFunc
	ListRuns                  ListRunInstancesFunc
}

type GovernancePorts struct {
	ListDecisions         ListDecisionsFunc
	ListDecisionPage      ListDecisionPageFunc
	ReadDecision          ReadDecisionFunc
	ResolveDecision       ResolveDecisionFunc
	BatchResolveDecisions BatchResolveDecisionsFunc
	ListApprovals         ListApprovalsFunc
	ListApprovalPage      ListApprovalPageFunc
	ReadApproval          ReadApprovalFunc
	ApproveApproval       ApproveApprovalFunc
	RejectApproval        RejectApprovalFunc
	BatchResolveApprovals BatchResolveApprovalsFunc
}

type RegistryPorts struct {
	CreateSoul              CreateSoulFunc
	UpdateSoul              UpdateSoulFunc
	DeleteSoul              DeleteSoulFunc
	ReadSoul                ReadSoulFunc
	ListSouls               ListSoulsFunc
	CreateWorkspaceTemplate CreateWorkspaceTemplateFunc
	UpdateWorkspaceTemplate UpdateWorkspaceTemplateFunc
	DeleteWorkspaceTemplate DeleteWorkspaceTemplateFunc
	ListWorkspaceTemplates  ListWorkspaceTemplatesFunc
	CreateSkill             CreateSkillFunc
	ReadSkill               ReadSkillFunc
	ListSkills              ListSkillsFunc
	CreateMCPServer         CreateMCPServerFunc
	UpdateMCPServer         UpdateMCPServerFunc
	ReadMCPServer           ReadMCPServerFunc
	ListMCPServers          ListMCPServersFunc
	ListMCPServerTools      ListMCPServerToolsFunc
	ListMCPServerResources  ListMCPServerResourcesFunc
	ListMCPServerPrompts    ListMCPServerPromptsFunc
	CreateMCPCapability     CreateMCPCapabilityFunc
	UpdateMCPCapability     UpdateMCPCapabilityFunc
	ReadMCPCapability       ReadMCPCapabilityFunc
	ListMCPCapabilities     ListMCPCapabilitiesFunc
}

type MemoryPorts struct {
	CreateMemoryItem       CreateMemoryItemFunc
	ReadMemoryItem         ReadMemoryItemFunc
	UpdateMemoryItem       UpdateMemoryItemFunc
	DeleteMemoryItem       DeleteMemoryItemFunc
	ListMemoryByWorkspace  ListMemoryByWorkspaceFunc
	SearchMemory           SearchMemoryFunc
	ListMemoryRecallTraces ListMemoryRecallTracesFunc
}

type ConfigurationPorts struct {
	ReadConfiguration            ReadConfigurationFunc
	UpdateServerConfiguration    UpdateServerConfigurationFunc
	UpdateRuntimeConfiguration   UpdateRuntimeConfigurationFunc
	UpdateLLMConfiguration       UpdateLLMConfigurationFunc
	GetLLMModelCatalog           GetLLMModelCatalogFunc
	RefreshLLMModelCatalog       RefreshLLMModelCatalogFunc
	ReadConfigurationWithCatalog ReadConfigurationWithCatalogFunc
	UpdateWebSearchConfiguration UpdateWebSearchConfigurationFunc
}

// Validate fails closed before the HTTP listener starts when a Biz adapter
// forgets one typed operation. This keeps nil callback panics out of requests.
func (ports ApplicationPorts) Validate() error {
	if ports.Workspace.Validate == nil {
		return fmt.Errorf("missing Workspace.Validate application port")
	}
	if ports.Workspace.CreateWorkspace == nil {
		return fmt.Errorf("missing Workspace.CreateWorkspace application port")
	}
	if ports.Workspace.DeleteWorkspace == nil {
		return fmt.Errorf("missing Workspace.DeleteWorkspace application port")
	}
	if ports.Workspace.ReadWorkspace == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspace application port")
	}
	if ports.Workspace.ReadWorkspaceSnapshot == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspaceSnapshot application port")
	}
	if ports.Workspace.ReadWorkspaceDetailSnapshot == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspaceDetailSnapshot application port")
	}
	if ports.Workspace.ReadProjectionSnapshot == nil {
		return fmt.Errorf("missing Workspace.ReadProjectionSnapshot application port")
	}
	if ports.Workspace.ReadWorkspaceSoulBindings == nil {
		return fmt.Errorf("missing Workspace.ReadWorkspaceSoulBindings application port")
	}
	if ports.Workspace.UpdateWorkspaceSoulBindings == nil {
		return fmt.Errorf("missing Workspace.UpdateWorkspaceSoulBindings application port")
	}
	if ports.Workspace.UpdateWorkspaceLLMSelection == nil {
		return fmt.Errorf("missing Workspace.UpdateWorkspaceLLMSelection application port")
	}
	if ports.Workspace.ListWorkspaces == nil {
		return fmt.Errorf("missing Workspace.ListWorkspaces application port")
	}
	if ports.Definition.DefineGoal == nil {
		return fmt.Errorf("missing Definition.DefineGoal application port")
	}
	if ports.Definition.UpdateGoal == nil {
		return fmt.Errorf("missing Definition.UpdateGoal application port")
	}
	if ports.Definition.ReadGoal == nil {
		return fmt.Errorf("missing Definition.ReadGoal application port")
	}
	if ports.Definition.ListGoals == nil {
		return fmt.Errorf("missing Definition.ListGoals application port")
	}
	if ports.Definition.DefineDoD == nil {
		return fmt.Errorf("missing Definition.DefineDoD application port")
	}
	if ports.Definition.UpdateDoD == nil {
		return fmt.Errorf("missing Definition.UpdateDoD application port")
	}
	if ports.Definition.ReadDoD == nil {
		return fmt.Errorf("missing Definition.ReadDoD application port")
	}
	if ports.Definition.ListDoDs == nil {
		return fmt.Errorf("missing Definition.ListDoDs application port")
	}
	if ports.Definition.CheckGoalDoDQuality == nil {
		return fmt.Errorf("missing Definition.CheckGoalDoDQuality application port")
	}
	if ports.Definition.GenerateWorkspaceSetupDraft == nil {
		return fmt.Errorf("missing Definition.GenerateWorkspaceSetupDraft application port")
	}
	if ports.Run.ReadRun == nil {
		return fmt.Errorf("missing Run.ReadRun application port")
	}
	if ports.Run.ReadRunSnapshot == nil {
		return fmt.Errorf("missing Run.ReadRunSnapshot application port")
	}
	if ports.Run.ListRunSteps == nil {
		return fmt.Errorf("missing Run.ListRunSteps application port")
	}
	if ports.Run.ListRunToolCalls == nil {
		return fmt.Errorf("missing Run.ListRunToolCalls application port")
	}
	if ports.Run.ListRunRuntimeCheckpoints == nil {
		return fmt.Errorf("missing Run.ListRunRuntimeCheckpoints application port")
	}
	if ports.Run.ListRunResults == nil {
		return fmt.Errorf("missing Run.ListRunResults application port")
	}
	if ports.Run.ListRunPatches == nil {
		return fmt.Errorf("missing Run.ListRunPatches application port")
	}
	if ports.Run.ListRunValidationResults == nil {
		return fmt.Errorf("missing Run.ListRunValidationResults application port")
	}
	if ports.Run.ListRunArtifacts == nil {
		return fmt.Errorf("missing Run.ListRunArtifacts application port")
	}
	if ports.Run.ReadRunStatus == nil {
		return fmt.Errorf("missing Run.ReadRunStatus application port")
	}
	if ports.Run.ReadRunObservability == nil {
		return fmt.Errorf("missing Run.ReadRunObservability application port")
	}
	if ports.Run.ReadArtifactContent == nil {
		return fmt.Errorf("missing Run.ReadArtifactContent application port")
	}
	if ports.Run.ReadRuntimeRawContent == nil {
		return fmt.Errorf("missing Run.ReadRuntimeRawContent application port")
	}
	if ports.Run.ReadRunTaskProgress == nil {
		return fmt.Errorf("missing Run.ReadRunTaskProgress application port")
	}
	if ports.Run.ReadRunContextBundle == nil {
		return fmt.Errorf("missing Run.ReadRunContextBundle application port")
	}
	if ports.Run.ReadRunTaskDecompositions == nil {
		return fmt.Errorf("missing Run.ReadRunTaskDecompositions application port")
	}
	if ports.Run.ListEventRecords == nil {
		return fmt.Errorf("missing Run.ListEventRecords application port")
	}
	if ports.Run.ListRuns == nil {
		return fmt.Errorf("missing Run.ListRuns application port")
	}
	if ports.Governance.ListDecisions == nil {
		return fmt.Errorf("missing Governance.ListDecisions application port")
	}
	if ports.Governance.ListDecisionPage == nil {
		return fmt.Errorf("missing Governance.ListDecisionPage application port")
	}
	if ports.Governance.ReadDecision == nil {
		return fmt.Errorf("missing Governance.ReadDecision application port")
	}
	if ports.Governance.ResolveDecision == nil {
		return fmt.Errorf("missing Governance.ResolveDecision application port")
	}
	if ports.Governance.BatchResolveDecisions == nil {
		return fmt.Errorf("missing Governance.BatchResolveDecisions application port")
	}
	if ports.Governance.ListApprovals == nil {
		return fmt.Errorf("missing Governance.ListApprovals application port")
	}
	if ports.Governance.ListApprovalPage == nil {
		return fmt.Errorf("missing Governance.ListApprovalPage application port")
	}
	if ports.Governance.ReadApproval == nil {
		return fmt.Errorf("missing Governance.ReadApproval application port")
	}
	if ports.Governance.ApproveApproval == nil {
		return fmt.Errorf("missing Governance.ApproveApproval application port")
	}
	if ports.Governance.RejectApproval == nil {
		return fmt.Errorf("missing Governance.RejectApproval application port")
	}
	if ports.Governance.BatchResolveApprovals == nil {
		return fmt.Errorf("missing Governance.BatchResolveApprovals application port")
	}
	if ports.Registry.CreateSoul == nil {
		return fmt.Errorf("missing Registry.CreateSoul application port")
	}
	if ports.Registry.UpdateSoul == nil {
		return fmt.Errorf("missing Registry.UpdateSoul application port")
	}
	if ports.Registry.DeleteSoul == nil {
		return fmt.Errorf("missing Registry.DeleteSoul application port")
	}
	if ports.Registry.ReadSoul == nil {
		return fmt.Errorf("missing Registry.ReadSoul application port")
	}
	if ports.Registry.ListSouls == nil {
		return fmt.Errorf("missing Registry.ListSouls application port")
	}
	if ports.Registry.CreateWorkspaceTemplate == nil {
		return fmt.Errorf("missing Registry.CreateWorkspaceTemplate application port")
	}
	if ports.Registry.UpdateWorkspaceTemplate == nil {
		return fmt.Errorf("missing Registry.UpdateWorkspaceTemplate application port")
	}
	if ports.Registry.DeleteWorkspaceTemplate == nil {
		return fmt.Errorf("missing Registry.DeleteWorkspaceTemplate application port")
	}
	if ports.Registry.ListWorkspaceTemplates == nil {
		return fmt.Errorf("missing Registry.ListWorkspaceTemplates application port")
	}
	if ports.Registry.CreateSkill == nil {
		return fmt.Errorf("missing Registry.CreateSkill application port")
	}
	if ports.Registry.ReadSkill == nil {
		return fmt.Errorf("missing Registry.ReadSkill application port")
	}
	if ports.Registry.ListSkills == nil {
		return fmt.Errorf("missing Registry.ListSkills application port")
	}
	if ports.Registry.CreateMCPServer == nil {
		return fmt.Errorf("missing Registry.CreateMCPServer application port")
	}
	if ports.Registry.UpdateMCPServer == nil {
		return fmt.Errorf("missing Registry.UpdateMCPServer application port")
	}
	if ports.Registry.ReadMCPServer == nil {
		return fmt.Errorf("missing Registry.ReadMCPServer application port")
	}
	if ports.Registry.ListMCPServers == nil {
		return fmt.Errorf("missing Registry.ListMCPServers application port")
	}
	if ports.Registry.ListMCPServerTools == nil {
		return fmt.Errorf("missing Registry.ListMCPServerTools application port")
	}
	if ports.Registry.ListMCPServerResources == nil {
		return fmt.Errorf("missing Registry.ListMCPServerResources application port")
	}
	if ports.Registry.ListMCPServerPrompts == nil {
		return fmt.Errorf("missing Registry.ListMCPServerPrompts application port")
	}
	if ports.Registry.CreateMCPCapability == nil {
		return fmt.Errorf("missing Registry.CreateMCPCapability application port")
	}
	if ports.Registry.UpdateMCPCapability == nil {
		return fmt.Errorf("missing Registry.UpdateMCPCapability application port")
	}
	if ports.Registry.ReadMCPCapability == nil {
		return fmt.Errorf("missing Registry.ReadMCPCapability application port")
	}
	if ports.Registry.ListMCPCapabilities == nil {
		return fmt.Errorf("missing Registry.ListMCPCapabilities application port")
	}
	if ports.Memory.CreateMemoryItem == nil {
		return fmt.Errorf("missing Memory.CreateMemoryItem application port")
	}
	if ports.Memory.ReadMemoryItem == nil {
		return fmt.Errorf("missing Memory.ReadMemoryItem application port")
	}
	if ports.Memory.UpdateMemoryItem == nil {
		return fmt.Errorf("missing Memory.UpdateMemoryItem application port")
	}
	if ports.Memory.DeleteMemoryItem == nil {
		return fmt.Errorf("missing Memory.DeleteMemoryItem application port")
	}
	if ports.Memory.ListMemoryByWorkspace == nil {
		return fmt.Errorf("missing Memory.ListMemoryByWorkspace application port")
	}
	if ports.Memory.SearchMemory == nil {
		return fmt.Errorf("missing Memory.SearchMemory application port")
	}
	if ports.Memory.ListMemoryRecallTraces == nil {
		return fmt.Errorf("missing Memory.ListMemoryRecallTraces application port")
	}
	if ports.Configuration.ReadConfiguration == nil {
		return fmt.Errorf("missing Configuration.ReadConfiguration application port")
	}
	if ports.Configuration.UpdateServerConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateServerConfiguration application port")
	}
	if ports.Configuration.UpdateRuntimeConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateRuntimeConfiguration application port")
	}
	if ports.Configuration.UpdateLLMConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateLLMConfiguration application port")
	}
	if ports.Configuration.GetLLMModelCatalog == nil {
		return fmt.Errorf("missing Configuration.GetLLMModelCatalog application port")
	}
	if ports.Configuration.RefreshLLMModelCatalog == nil {
		return fmt.Errorf("missing Configuration.RefreshLLMModelCatalog application port")
	}
	if ports.Configuration.ReadConfigurationWithCatalog == nil {
		return fmt.Errorf("missing Configuration.ReadConfigurationWithCatalog application port")
	}
	if ports.Configuration.UpdateWebSearchConfiguration == nil {
		return fmt.Errorf("missing Configuration.UpdateWebSearchConfiguration application port")
	}
	return nil
}
