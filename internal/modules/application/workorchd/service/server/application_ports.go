package server

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
	Validate                              ValidateFunc
	DeleteWorkspace                       DeleteWorkspaceFunc
	ReadWorkspace                         ReadWorkspaceFunc
	ReadWorkspaceSnapshot                 ReadWorkspaceSnapshotFunc
	ReadWorkspaceDetailSnapshot           ReadWorkspaceSnapshotFunc
	ReadProjectionSnapshot                ReadProjectionSnapshotFunc
	ReadWorkspaceSoulBindings             ReadWorkspaceSoulBindingsFunc
	UpdateWorkspaceSoulBindings           UpdateWorkspaceSoulBindingsFunc
	UpdateWorkspaceLLMSelection           UpdateWorkspaceLLMSelectionFunc
	ListWorkspaces                        ListWorkspacesFunc
	ReadStartupPreparation                ReadStartupPreparationFunc
	CheckStartupPreparation               CheckStartupPreparationFunc
	InitializeStartupPreparationResources InitializeStartupPreparationResourcesFunc
	RecoverStartupPreparationResources    InitializeStartupPreparationResourcesFunc
	RecoverStartupPreparation             CheckStartupPreparationFunc
}

type DefinitionPorts struct {
	CreateDefinitionAuthoring                    CreateDefinitionAuthoringFunc
	ReadDefinitionAuthoring                      ReadDefinitionAuthoringFunc
	SaveDefinitionAuthoring                      SaveDefinitionAuthoringFunc
	CompileDefinitionAuthoring                   CompileDefinitionAuthoringFunc
	ConfirmDefinitionAuthoring                   ConfirmDefinitionAuthoringFunc
	ReadDefinitionAuthoringSuccessAudit          ReadDefinitionAuthoringSuccessAuditFunc
	ReadDefinitionAuthoringDiagnostic            ReadDefinitionAuthoringDiagnosticFunc
	ReadDefinitionAuthoringPreparationDiagnostic ReadDefinitionAuthoringPreparationDiagnosticFunc
	ReadGoal                                     ReadGoalFunc
	ListGoals                                    ListGoalsFunc
	ReadDoD                                      ReadDoDFunc
	ListDoDs                                     ListDoDsFunc
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
	ListBuiltinDefaults     ListBuiltinDefaultsFunc
	UpgradeBuiltinDefault   UpgradeBuiltinDefaultFunc
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
// forgets one typed operation. Group order preserves the historical first
// missing-port diagnostic exactly.
func (ports ApplicationPorts) Validate() error {
	if err := validateWorkspaceApplicationPorts(ports.Workspace); err != nil {
		return err
	}
	if err := validateDefinitionApplicationPorts(ports.Definition); err != nil {
		return err
	}
	if err := validateRunApplicationPorts(ports.Run); err != nil {
		return err
	}
	if err := validateGovernanceApplicationPorts(ports.Governance); err != nil {
		return err
	}
	if err := validateRegistryApplicationPorts(ports.Registry); err != nil {
		return err
	}
	if err := validateMemoryApplicationPorts(ports.Memory); err != nil {
		return err
	}
	return validateConfigurationApplicationPorts(ports.Configuration)
}
