package server

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

// newDefaultApplicationOperations is retained for standalone/test server
// construction. Production Biz wiring can replace this typed boundary before
// starting the listener.
func newDefaultApplicationOperations() applicationOperations {
	var operations applicationOperations
	operations.validate = defaultValidate
	operations.createWorkspace = defaultCreateWorkspace
	operations.deleteWorkspace = defaultDeleteWorkspace
	operations.readWorkspace = defaultReadWorkspace
	operations.readWorkspaceSnapshot = defaultReadWorkspaceSnapshot
	operations.readWorkspaceDetailSnapshot = defaultReadWorkspaceDetailSnapshot
	operations.readProjectionSnapshot = defaultReadWorkspaceProjectionSnapshot
	operations.readWorkspaceSoulBindings = defaultReadWorkspaceSoulBindings
	operations.updateWorkspaceSoulBindings = defaultUpdateWorkspaceSoulBindings
	operations.listWorkspaces = defaultListWorkspaces
	operations.defineGoal = defaultDefineGoal
	operations.updateGoal = defaultUpdateGoal
	operations.readGoal = defaultReadGoal
	operations.listGoals = defaultListGoals
	operations.defineDoD = defaultDefineDoD
	operations.updateDoD = defaultUpdateDoD
	operations.readDoD = defaultReadDoD
	operations.listDoDs = defaultListDoDs
	operations.checkGoalDoDQuality = defaultCheckGoalDoDQuality
	operations.readRun = defaultReadRun
	operations.readRunSnapshot = defaultReadRunSnapshot
	operations.listRunSteps = defaultListRunSteps
	operations.listRunToolCalls = defaultListRunToolCalls
	operations.listRunRuntimeCheckpoints = defaultListRunRuntimeCheckpoints
	operations.listRunResults = defaultListRunResults
	operations.listRunPatches = defaultListRunPatches
	operations.listRunValidationResults = defaultListRunValidationResults
	operations.listRunArtifacts = defaultListRunArtifacts
	operations.readRunStatus = defaultReadRunStatus
	operations.readRunObservability = defaultReadRunObservability
	operations.readArtifactContent = defaultReadArtifactContent
	operations.readRuntimeRawContent = defaultReadRuntimeRawContent
	operations.readRunTaskProgress = defaultReadRunTaskProgress
	operations.readRunContextBundle = defaultReadRunContextBundle
	operations.readRunTaskDecompositions = defaultReadRunTaskDecompositions
	operations.listMemoryRecallTraces = defaultListMemoryRecallTraces
	operations.listEventRecords = defaultListEventRecords
	operations.listRuns = defaultListRuns
	operations.listDecisions = defaultListDecisions
	operations.listDecisionPage = defaultListDecisionPage
	operations.readDecision = defaultReadDecision
	operations.resolveDecision = defaultResolveDecision
	operations.batchResolveDecisions = defaultBatchResolveDecisions
	operations.listApprovals = defaultListApprovals
	operations.listApprovalPage = defaultListApprovalPage
	operations.readApproval = defaultReadApproval
	operations.approveApproval = defaultApproveApproval
	operations.rejectApproval = defaultRejectApproval
	operations.batchResolveApprovals = defaultBatchResolveApprovals
	operations.createSoul = defaultCreateSoul
	operations.updateSoul = defaultUpdateSoul
	operations.deleteSoul = defaultDeleteSoul
	operations.readSoul = defaultReadSoul
	operations.listSouls = defaultListSouls
	operations.createWorkspaceTemplate = defaultCreateWorkspaceTemplate
	operations.updateWorkspaceTemplate = defaultUpdateWorkspaceTemplate
	operations.deleteWorkspaceTemplate = defaultDeleteWorkspaceTemplate
	operations.listWorkspaceTemplates = defaultListWorkspaceTemplates
	operations.createSkill = defaultCreateSkill
	operations.readSkill = defaultReadSkill
	operations.listSkills = defaultListSkills
	operations.createMCPServer = defaultCreateMCPServer
	operations.updateMCPServer = defaultUpdateMCPServer
	operations.readMCPServer = defaultReadMCPServer
	operations.listMCPServers = defaultListMCPServers
	operations.listMCPServerTools = defaultListMCPServerTools
	operations.listMCPServerResources = defaultListMCPServerResources
	operations.listMCPServerPrompts = defaultListMCPServerPrompts
	operations.createMCPCapability = defaultCreateMCPCapability
	operations.updateMCPCapability = defaultUpdateMCPCapability
	operations.readMCPCapability = defaultReadMCPCapability
	operations.listMCPCapabilities = defaultListMCPCapabilities
	operations.createMemoryItem = defaultCreateMemoryItem
	operations.readMemoryItem = defaultReadMemoryItem
	operations.updateMemoryItem = defaultUpdateMemoryItem
	operations.deleteMemoryItem = defaultDeleteMemoryItem
	operations.listMemoryByWorkspace = defaultListMemoryByWorkspace
	operations.searchMemory = defaultSearchMemory
	operations.readConfiguration = defaultReadConfiguration
	operations.updateServerConfiguration = appservice.UpdateServerConfiguration
	operations.updateRuntimeConfiguration = appservice.UpdateRuntimeConfiguration
	operations.updateLLMConfiguration = appservice.UpdateLLMConfiguration
	operations.updateWebSearchConfiguration = appservice.UpdateWebSearchConfiguration
	return operations
}
