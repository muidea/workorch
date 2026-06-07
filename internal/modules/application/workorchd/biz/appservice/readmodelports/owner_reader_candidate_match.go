package readmodelports

func (c OwnerReaderCandidates) Matches(owner OwnerReaders) bool {
	if !ownerReaderHubsReady(owner) {
		return false
	}
	return c.matchesDefinitionReaders(owner) &&
		c.matchesRuntimeReaders(owner) &&
		c.matchesGovernanceReaders(owner)
}

func ownerReaderHubsReady(owner OwnerReaders) bool {
	return owner.Workspaces.Hub != nil &&
		owner.Runs.Hub != nil &&
		owner.EventRecords.Hub != nil
}

func (c OwnerReaderCandidates) matchesDefinitionReaders(owner OwnerReaders) bool {
	workspaces, ok := c.Workspaces.(WorkspaceReader)
	if !ok || workspaces != owner.Workspaces {
		return false
	}
	goals, ok := c.Goals.(GoalReader)
	if !ok || goals != owner.Goals {
		return false
	}
	dods, ok := c.DoDs.(DoDReader)
	if !ok || dods != owner.DoDs {
		return false
	}
	capabilities, ok := c.MCPCapabilities.(MCPCapabilityReader)
	return ok && capabilities == owner.MCPCapabilities
}

func (c OwnerReaderCandidates) matchesRuntimeReaders(owner OwnerReaders) bool {
	runs, ok := c.Runs.(RunReader)
	if !ok || runs != owner.Runs {
		return false
	}
	steps, ok := c.Steps.(StepReader)
	if !ok || steps != owner.Steps {
		return false
	}
	results, ok := c.Results.(ResultReader)
	if !ok || results != owner.Results {
		return false
	}
	artifacts, ok := c.Artifacts.(ArtifactReader)
	if !ok || artifacts != owner.Artifacts {
		return false
	}
	continuations, ok := c.ArtifactContinuations.(ArtifactContinuationReader)
	if !ok || continuations != owner.ArtifactContinuations {
		return false
	}
	eventRecords, ok := c.EventRecords.(EventRecordReader)
	if !ok || eventRecords != owner.EventRecords {
		return false
	}
	memoryRecalls, ok := c.MemoryRecalls.(MemoryRecallTraceReader)
	if !ok || memoryRecalls != owner.MemoryRecalls {
		return false
	}
	toolCalls, ok := c.ToolCalls.(ToolCallReader)
	if !ok || toolCalls != owner.ToolCalls {
		return false
	}
	patches, ok := c.Patches.(PatchReader)
	if !ok || patches != owner.Patches {
		return false
	}
	validations, ok := c.Validations.(ValidationResultReader)
	if !ok || validations != owner.Validations {
		return false
	}
	checkpoints, ok := c.Checkpoints.(RuntimeCheckpointReader)
	if !ok || checkpoints != owner.Checkpoints {
		return false
	}
	traces, ok := c.AutoGovernanceTraces.(AutoGovernanceTraceReader)
	if !ok || traces != owner.AutoGovernanceTraces {
		return false
	}
	return true
}
