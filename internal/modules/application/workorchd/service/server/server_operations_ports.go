package server

// applicationOperations is the single transport-to-application dependency
// boundary. The embedded domain slices keep HTTP handlers from owning a flat
// bag of unrelated callbacks while preserving private, typed contracts.
type applicationOperations struct {
	workspaceOperations
	definitionOperations
	runOperations
	governanceOperations
	registryOperations
	memoryOperations
	configurationOperations
}

type workspaceOperations struct {
	validate                        ValidateFunc
	createWorkspace                 CreateWorkspaceFunc
	deleteWorkspace                 DeleteWorkspaceFunc
	readWorkspace                   ReadWorkspaceFunc
	readWorkspaceSnapshot           ReadWorkspaceSnapshotFunc
	readWorkspaceDetailSnapshot     ReadWorkspaceSnapshotFunc
	readProjectionSnapshot          ReadProjectionSnapshotFunc
	readWorkspaceSoulBindings       ReadWorkspaceSoulBindingsFunc
	updateWorkspaceSoulBindings     UpdateWorkspaceSoulBindingsFunc
	updateWorkspaceLLMSelectionPort UpdateWorkspaceLLMSelectionFunc
	listWorkspaces                  ListWorkspacesFunc
}

type definitionOperations struct {
	defineGoal                  DefineGoalFunc
	updateGoal                  UpdateGoalFunc
	readGoal                    ReadGoalFunc
	listGoals                   ListGoalsFunc
	defineDoD                   DefineDoDFunc
	updateDoD                   UpdateDoDFunc
	readDoD                     ReadDoDFunc
	listDoDs                    ListDoDsFunc
	checkGoalDoDQuality         CheckGoalDoDQualityFunc
	generateWorkspaceSetupDraft GenerateWorkspaceSetupDraftFunc
}

type runOperations struct {
	readRun                   ReadRunInstanceFunc
	readRunSnapshot           ReadRunSnapshotFunc
	listRunSteps              ListRunStepsFunc
	listRunToolCalls          ListRunToolCallsFunc
	listRunRuntimeCheckpoints ListRunRuntimeCheckpointsFunc
	listRunResults            ListRunResultsFunc
	listRunPatches            ListRunPatchesFunc
	listRunValidationResults  ListRunValidationResultsFunc
	listRunArtifacts          ListRunArtifactsFunc
	readRunStatus             ReadRunStatusFunc
	readRunObservability      ReadRunObservabilityFunc
	readArtifactContent       ReadArtifactContentFunc
	readRuntimeRawContent     ReadRuntimeRawContentFunc
	readRunTaskProgress       ReadRunTaskProgressFunc
	readRunContextBundle      ReadRunContextBundleFunc
	readRunTaskDecompositions ReadRunTaskDecompositionsFunc
	listEventRecords          ListEventRecordsFunc
	listRuns                  ListRunInstancesFunc
}

type governanceOperations struct {
	listDecisions         ListDecisionsFunc
	listDecisionPage      ListDecisionPageFunc
	readDecision          ReadDecisionFunc
	resolveDecision       ResolveDecisionFunc
	batchResolveDecisions BatchResolveDecisionsFunc
	listApprovals         ListApprovalsFunc
	listApprovalPage      ListApprovalPageFunc
	readApproval          ReadApprovalFunc
	approveApproval       ApproveApprovalFunc
	rejectApproval        RejectApprovalFunc
	batchResolveApprovals BatchResolveApprovalsFunc
}

type registryOperations struct {
	listBuiltinDefaults     ListBuiltinDefaultsFunc
	upgradeBuiltinDefault   UpgradeBuiltinDefaultFunc
	createSoul              CreateSoulFunc
	updateSoul              UpdateSoulFunc
	deleteSoul              DeleteSoulFunc
	readSoul                ReadSoulFunc
	listSouls               ListSoulsFunc
	createWorkspaceTemplate CreateWorkspaceTemplateFunc
	updateWorkspaceTemplate UpdateWorkspaceTemplateFunc
	deleteWorkspaceTemplate DeleteWorkspaceTemplateFunc
	listWorkspaceTemplates  ListWorkspaceTemplatesFunc
	createSkill             CreateSkillFunc
	readSkill               ReadSkillFunc
	listSkills              ListSkillsFunc
	createMCPServer         CreateMCPServerFunc
	updateMCPServer         UpdateMCPServerFunc
	readMCPServer           ReadMCPServerFunc
	listMCPServers          ListMCPServersFunc
	listMCPServerTools      ListMCPServerToolsFunc
	listMCPServerResources  ListMCPServerResourcesFunc
	listMCPServerPrompts    ListMCPServerPromptsFunc
	createMCPCapability     CreateMCPCapabilityFunc
	updateMCPCapability     UpdateMCPCapabilityFunc
	readMCPCapability       ReadMCPCapabilityFunc
	listMCPCapabilities     ListMCPCapabilitiesFunc
}

type memoryOperations struct {
	createMemoryItem       CreateMemoryItemFunc
	readMemoryItem         ReadMemoryItemFunc
	updateMemoryItem       UpdateMemoryItemFunc
	deleteMemoryItem       DeleteMemoryItemFunc
	listMemoryByWorkspace  ListMemoryByWorkspaceFunc
	searchMemory           SearchMemoryFunc
	listMemoryRecallTraces ListMemoryRecallTracesFunc
}

type configurationOperations struct {
	readConfiguration            ReadConfigurationFunc
	updateServerConfiguration    UpdateServerConfigurationFunc
	updateRuntimeConfiguration   UpdateRuntimeConfigurationFunc
	updateLLMConfiguration       UpdateLLMConfigurationFunc
	getLLMModelCatalog           GetLLMModelCatalogFunc
	refreshLLMModelCatalog       RefreshLLMModelCatalogFunc
	readConfigurationWithCatalog ReadConfigurationWithCatalogFunc
	updateWebSearchConfiguration UpdateWebSearchConfigurationFunc
}
