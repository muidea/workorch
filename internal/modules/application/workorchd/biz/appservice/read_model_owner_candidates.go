package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"

func ownerReaderCandidatesFromReadModelDependencies(deps readModelDependencies) readmodelports.OwnerReaderCandidates {
	return readmodelports.OwnerReaderCandidates{
		Workspaces:            deps.workspaces,
		Goals:                 deps.goals,
		DoDs:                  deps.dods,
		Runs:                  deps.runs,
		Steps:                 deps.steps,
		Results:               deps.results,
		Artifacts:             deps.artifacts,
		ArtifactContinuations: deps.artifactContinuations,
		CoverageWorkItems:     deps.coverageWorkItems,
		EventRecords:          deps.eventRecords,
		MemoryRecalls:         deps.memoryRecalls,
		ToolCalls:             deps.toolCalls,
		MCPCapabilities:       deps.mcpCapabilities,
		Patches:               deps.patches,
		Validations:           deps.validations,
		Checkpoints:           deps.checkpoints,
		AutoGovernanceTraces:  deps.autoGovernanceTraces,
		Tickets:               deps.tickets,
		Resolutions:           deps.resolutions,
		Approvals:             deps.approvals,
	}
}
