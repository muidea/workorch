package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type readModelDependencies struct {
	ownerReaders          readmodelports.OwnerReaders
	workspaces            readmodelcore.WorkspaceReader
	goals                 readmodelcore.GoalReader
	dods                  readmodelcore.DoDReader
	runs                  readmodelcore.RunReader
	steps                 readmodelcore.StepReader
	results               readmodelcore.ResultReader
	artifacts             readmodelcore.ArtifactReader
	artifactContinuations readmodelcore.ArtifactContinuationReader
	coverageWorkItems     readmodelcore.CoverageWorkItemReader
	eventRecords          readmodelcore.EventRecordReader
	memoryRecalls         readmodelcore.MemoryRecallTraceReader
	toolCalls             readmodelcore.ToolCallReader
	mcpCapabilities       readmodelcore.MCPCapabilityReader
	patches               readmodelcore.PatchReader
	validations           readmodelcore.ValidationResultReader
	checkpoints           readmodelcore.RuntimeCheckpointReader
	autoGovernanceTraces  readmodelcore.AutoGovernanceTraceReader
	tickets               readmodelcore.DecisionTicketReader
	resolutions           readmodelcore.DecisionResolutionReader
	approvals             readmodelcore.ApprovalReader
	commandHub            magicevent.Hub
	published             []eventstreamevents.EventRecordData
	loadConfig            func() (workorchconfig.Config, error)
}

func readModelDependenciesFromSession(session *RuntimeSession) (readModelDependencies, error) {
	commandHub, err := session.RequiredCommandHub()
	if err != nil {
		return readModelDependencies{}, err
	}
	readers, err := readmodelports.NewOwnerReaders(commandHub)
	if err != nil {
		return readModelDependencies{}, err
	}

	var published []eventstreamevents.EventRecordData
	if session.Runtime != nil {
		published = session.Runtime.PublishedEventRecords()
	}

	return readModelDependencies{
		ownerReaders:          readers,
		workspaces:            readers.Workspaces,
		goals:                 readers.Goals,
		dods:                  readers.DoDs,
		runs:                  readers.Runs,
		steps:                 readers.Steps,
		results:               readers.Results,
		artifacts:             readers.Artifacts,
		artifactContinuations: readers.ArtifactContinuations,
		coverageWorkItems:     readers.CoverageWorkItems,
		eventRecords:          readers.EventRecords,
		memoryRecalls:         readers.MemoryRecalls,
		toolCalls:             readers.ToolCalls,
		mcpCapabilities:       readers.MCPCapabilities,
		patches:               readers.Patches,
		validations:           readers.Validations,
		checkpoints:           readers.Checkpoints,
		autoGovernanceTraces:  readers.AutoGovernanceTraces,
		tickets:               readers.Tickets,
		resolutions:           readers.Resolutions,
		approvals:             readers.Approvals,
		commandHub:            commandHub,
		published:             published,
		loadConfig:            session.ConfigLoader(),
	}, nil
}
