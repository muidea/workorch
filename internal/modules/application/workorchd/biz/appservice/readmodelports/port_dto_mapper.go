package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchpayloadmapper"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func PortWorkspaceSnapshotInfo(snapshot readmodelcore.WorkspaceSnapshot) port.WorkspaceSnapshotInfo {
	return port.WorkspaceSnapshotInfo{
		Workspace:               PortWorkspaceInfo(snapshot.Workspace),
		ViewModel:               PortWorkspaceViewModelInfo(snapshot.ViewModel),
		Projection:              PortProjectionInfo(snapshot.ViewModel),
		Outcome:                 portWorkspaceOutcomeInfo(snapshot.Outcome),
		TaskProgress:            portTaskProgressInfo(snapshot.TaskProgress),
		RunHeartbeat:            portRunHeartbeatInfo(snapshot.RunHeartbeat),
		ConvergenceStatus:       portConvergenceStatusInfo(snapshot.ConvergenceStatus),
		RunCommand:              portRunCommandStatusInfo(snapshot.RunCommand),
		SchedulerDiagnostics:    PortSchedulerDiagnosticsInfo(snapshot.SchedulerDiagnostics),
		ContextBundle:           portPayload(portContextBundle(snapshot.ContextBundle)),
		CoverageSummary:         portPayload(snapshot.CoverageSummary),
		GapBaseline:             PortGapBaselineInfo(snapshot.GapBaseline),
		Acceptance:              portAcceptanceProjectionInfo(snapshot.Acceptance),
		TaskDecompositions:      portTaskDecompositionInfos(snapshot.TaskDecompositions),
		TaskPlans:               portTaskPlanInfos(snapshot.TaskPlans),
		Goals:                   PortGoalInfos(snapshot.Goals),
		DoDs:                    PortDoDInfos(snapshot.DoDs),
		Runs:                    PortRunInfos(snapshot.Runs),
		Results:                 portResultInfos(snapshot.Results),
		Artifacts:               portArtifactInfos(snapshot.Artifacts),
		ArtifactContinuations:   portArtifactContinuationInfos(snapshot.ArtifactContinuations),
		Patches:                 portPatchInfos(snapshot.Patches),
		MemoryRecallTraces:      portMemoryRecallTraceInfos(snapshot.MemoryRecallTraces),
		WebSources:              portWebSourceTraceInfos(snapshot.WebSources),
		LLMContextBudgets:       portLLMContextBudgetTraceInfos(snapshot.LLMContextBudgets),
		LLMContextBudgetSummary: portLLMContextBudgetSummaryInfo(snapshot.LLMContextBudgetSummary),
		LLMUsageTraces:          portLLMUsageTraceInfos(snapshot.LLMUsageTraces),
		LLMUsageSummary:         portLLMUsageSummaryInfo(snapshot.LLMUsageSummary),
		EventRecords:            portEventRecordInfos(snapshot.EventRecords),
		Decisions:               PortDecisionInfos(snapshot.Decisions),
		Approvals:               PortApprovalInfos(snapshot.Approvals),
	}
}

func PortWorkspaceInfo(item appmodelcore.Workspace) port.WorkspaceInfo {
	return port.WorkspaceInfo{
		ID:                   common.WorkspaceID(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		State:                common.WorkspaceState(item.State),
		ActiveRunID:          common.RunID(item.ActiveRunID),
		BlockingTicketID:     common.TicketID(item.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(item.BlockingApprovalID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMTarget:     cloneRuntimeProviderTarget(item.DefaultLLMTarget),
		DefaultSoulID:        common.SoulID(item.DefaultSoulID),
		SoulBindings:         portSoulBindings(item.SoulBindings),
		Projection:           portProjectionFromAppViewModel(item.ViewModel),
		ViewModel:            portAppWorkspaceViewModelInfo(item.ViewModel),
	}
}

func cloneRuntimeProviderTarget(input *runtimecontract.ProviderModelTarget) *runtimecontract.ProviderModelTarget {
	if input == nil {
		return nil
	}
	value := input.Clone()
	return &value
}

func PortWorkspaceInfos(items []appmodelcore.Workspace) []port.WorkspaceInfo {
	ret := make([]port.WorkspaceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortWorkspaceInfo(item))
	}
	return ret
}

func PortGoalInfo(item appmodelcore.Goal) port.GoalInfo {
	return port.GoalInfo{
		ID:          common.GoalID(item.ID),
		WorkspaceID: common.WorkspaceID(item.WorkspaceID),
		Text:        item.Text,
	}
}

func PortGoalInfos(items []appmodelcore.Goal) []port.GoalInfo {
	ret := make([]port.GoalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortGoalInfo(item))
	}
	return ret
}

func PortDoDInfo(item appmodelcore.DoD) port.DoDInfo {
	return port.DoDInfo{
		ID:          common.DoDID(item.ID),
		WorkspaceID: common.WorkspaceID(item.WorkspaceID),
		Items:       cloneStrings(item.Items),
		State:       item.State,
	}
}

func PortDoDInfos(items []appmodelcore.DoD) []port.DoDInfo {
	ret := make([]port.DoDInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortDoDInfo(item))
	}
	return ret
}

func PortRunInfo(item appmodelcore.Run) port.RunInfo {
	return port.RunInfo{
		ID:                 common.RunID(item.ID),
		WorkspaceID:        common.WorkspaceID(item.WorkspaceID),
		GoalID:             common.GoalID(item.GoalID),
		DoDID:              common.DoDID(item.DoDID),
		DefinitionRevision: item.DefinitionRevision,
		State:              common.RunState(item.State),
		Priority:           item.Priority,
		BlockingTicketID:   common.TicketID(item.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
}

func PortRunInfos(items []appmodelcore.Run) []port.RunInfo {
	ret := make([]port.RunInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortRunInfo(item))
	}
	return ret
}

func PortDecisionInfo(item appmodelcore.Decision) port.DecisionInfo {
	return port.DecisionInfo{
		ID:                 common.TicketID(item.ID),
		WorkspaceID:        common.WorkspaceID(item.WorkspaceID),
		RunID:              common.RunID(item.RunID),
		Reason:             item.Reason,
		Question:           item.Question,
		Context:            item.Context.Clone(),
		State:              item.State,
		ResolutionType:     item.ResolutionType,
		ContinuationAction: item.ContinuationAction,
		ResumeRunID:        common.RunID(item.ResumeRunID),
		SuccessorRunID:     common.RunID(item.SuccessorRunID),
		AutoGovernance:     portAutoGovernanceSummaryInfo(item.AutoGovernance),
	}
}

func PortDecisionInfos(items []appmodelcore.Decision) []port.DecisionInfo {
	ret := make([]port.DecisionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortDecisionInfo(item))
	}
	return ret
}

func PortApprovalInfo(item appmodelcore.Approval) port.ApprovalInfo {
	return port.ApprovalInfo{
		ID:             common.ApprovalID(item.ID),
		WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
		RunID:          common.RunID(item.RunID),
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        item.Context.Clone(),
		State:          item.State,
		AutoGovernance: portAutoGovernanceSummaryInfo(item.AutoGovernance),
	}
}

func PortApprovalInfos(items []appmodelcore.Approval) []port.ApprovalInfo {
	ret := make([]port.ApprovalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortApprovalInfo(item))
	}
	return ret
}

func PortRunStatusInfo(item readmodelcore.RunStatus) port.RunStatusInfo {
	return port.RunStatusInfo{
		ID:                   common.RunID(item.ID),
		WorkspaceID:          common.WorkspaceID(item.WorkspaceID),
		State:                common.RunState(item.State),
		CurrentStepID:        common.StepID(item.CurrentStepID),
		CurrentStepKind:      item.CurrentStepKind,
		CurrentStepState:     common.StepState(item.CurrentStepState),
		CurrentActionLabel:   item.CurrentActionLabel,
		DiagnosticLineage:    common.StepID(item.DiagnosticLineage),
		StepCount:            item.StepCount,
		DecisionCount:        item.DecisionCount,
		ApprovalCount:        item.ApprovalCount,
		BlockingTicketID:     common.TicketID(item.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(item.BlockingApprovalID),
		FailureReason:        item.FailureReason,
		ConvergenceStatus:    portConvergenceStatusInfo(item.ConvergenceStatus),
		SchedulerDiagnostics: PortSchedulerDiagnosticsInfo(item.SchedulerDiagnostics),
	}
}

func PortWorkspaceViewModelInfo(item readmodelcore.WorkspaceViewModel) port.WorkspaceViewModelInfo {
	return port.WorkspaceViewModelInfo{
		WorkspaceID:             common.WorkspaceID(item.WorkspaceID),
		State:                   item.State,
		PromptMode:              item.PromptMode,
		Revision:                item.Revision,
		GlobalRevision:          item.GlobalRevision,
		WorkspaceRevision:       item.WorkspaceRevision,
		SourceEventRevision:     item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision,
		UpdatedAt:               item.UpdatedAt,
		RebuiltAt:               item.RebuiltAt,
		Degraded:                item.Degraded,
		StaleReason:             item.StaleReason,
		Error:                   item.Error,
		Sections:                portReadModelViewModelSections(item.Sections),
	}
}

func PortProjectionInfo(item readmodelcore.WorkspaceViewModel) port.ProjectionInfo {
	return port.ProjectionInfo{
		State:                   item.State,
		GlobalRevision:          item.GlobalRevision,
		WorkspaceRevision:       item.WorkspaceRevision,
		SourceEventRevision:     item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision,
		UpdatedAt:               item.UpdatedAt,
		RebuiltAt:               item.RebuiltAt,
		Degraded:                item.Degraded,
		StaleReason:             item.StaleReason,
		Error:                   item.Error,
	}
}

func PortGapBaselineInfo(item *readmodelcore.GapBaseline) *port.GapBaselineInfo {
	if item == nil {
		return nil
	}
	ret := &port.GapBaselineInfo{
		WorkspaceID:           common.WorkspaceID(item.WorkspaceID),
		RunID:                 common.RunID(item.RunID),
		DefinitionRevision:    item.DefinitionRevision,
		BaselineRevision:      item.BaselineRevision,
		CurrentFocus:          portPayload(item.CurrentFocus),
		OpenCoverageWorkItems: portPayloadSlice(item.OpenCoverageWorkItems),
		AcceptedEvidenceRefs:  cloneStrings(item.AcceptedEvidenceRefs),
		RejectedEvidenceRefs:  cloneStrings(item.RejectedEvidenceRefs),
		NextAction: port.GapNextActionInfo{
			Kind:   item.NextAction.Kind,
			Reason: item.NextAction.Reason,
		},
	}
	for _, todo := range item.TodoItems {
		ret.TodoItems = append(ret.TodoItems, port.GapTodoItemInfo{
			ID:                    todo.ID,
			Source:                todo.Source,
			SourceRef:             todo.SourceRef,
			CoverageWorkItemIDs:   cloneStrings(todo.CoverageWorkItemIDs),
			RequiredEvidenceKinds: cloneStrings(todo.RequiredEvidenceKinds),
			Status:                todo.Status,
			Priority:              todo.Priority,
			Reason:                todo.Reason,
			Metadata:              portPayload(todo.Metadata),
		})
	}
	for _, diagnostic := range item.Diagnostics {
		ret.Diagnostics = append(ret.Diagnostics, port.GapDiagnosticInfo{
			Code:     diagnostic.Code,
			Severity: diagnostic.Severity,
			Message:  diagnostic.Message,
		})
	}
	return ret
}

func PortSoulInfo(item contextregistrycore.Soul) port.SoulInfo {
	return port.SoulInfo{
		ID:              common.SoulID(item.ID),
		Name:            item.Name,
		Description:     item.Description,
		BusinessDomains: cloneStrings(item.BusinessDomains),
		SoulMD:          item.SoulMD,
	}
}

func PortSoulInfos(items []contextregistrycore.Soul) []port.SoulInfo {
	ret := make([]port.SoulInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortSoulInfo(item))
	}
	return ret
}

func PortSkillInfo(item contextregistrycore.Skill) port.SkillInfo {
	return port.SkillInfo{
		ID:          common.SkillID(item.ID),
		Name:        item.Name,
		Version:     item.Version,
		Description: item.Description,
		Instruction: item.Instruction,
	}
}

func PortSkillInfos(items []contextregistrycore.Skill) []port.SkillInfo {
	ret := make([]port.SkillInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortSkillInfo(item))
	}
	return ret
}

func PortMCPServerInfo(item contextregistrycore.MCPServer) port.MCPServerInfo {
	return port.MCPServerInfo{
		ID:        common.MCPServerID(item.ID),
		Name:      item.Name,
		Transport: item.Transport,
		Endpoint:  item.Endpoint,
		Enabled:   item.Enabled,
	}
}

func PortMCPServerInfos(items []contextregistrycore.MCPServer) []port.MCPServerInfo {
	ret := make([]port.MCPServerInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortMCPServerInfo(item))
	}
	return ret
}

func PortMCPCapabilityInfo(item contextregistrycore.MCPCapability) port.MCPCapabilityInfo {
	return port.MCPCapabilityInfo{
		ID:          common.MCPCapabilityID(item.ID),
		ServerID:    common.MCPServerID(item.ServerID),
		Kind:        item.Kind,
		Name:        item.Name,
		DisplayName: item.DisplayName,
		Description: item.Description,
	}
}

func PortMCPCapabilityInfos(items []contextregistrycore.MCPCapability) []port.MCPCapabilityInfo {
	ret := make([]port.MCPCapabilityInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortMCPCapabilityInfo(item))
	}
	return ret
}

func PortMemoryItemInfo(item contextregistrycore.MemoryItem) port.MemoryItemInfo {
	return port.MemoryItemInfo{
		ID:                  common.MemoryID(item.ID),
		WorkspaceID:         common.WorkspaceID(item.WorkspaceID),
		RunID:               common.RunID(item.RunID),
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                cloneStrings(item.Tags),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		VectorReady:         item.VectorReady,
		VectorDimension:     item.VectorDimension,
		VectorScore:         item.VectorScore,
	}
}

func PortMemoryItemInfos(items []contextregistrycore.MemoryItem) []port.MemoryItemInfo {
	ret := make([]port.MemoryItemInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortMemoryItemInfo(item))
	}
	return ret
}

func PortTemplateInfo(item workspacecore.WorkspaceTemplate) port.TemplateInfo {
	return port.TemplateInfo{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         cloneStrings(item.DoDItems),
		DefaultSoulID:    common.SoulID(item.DefaultSoulID),
		SoulBindings:     cloneStringMap(item.SoulBindings),
		EvidencePolicies: portPayloadSlice(item.EvidencePolicies),
	}
}

func PortTemplateInfos(items []workspacecore.WorkspaceTemplate) []port.TemplateInfo {
	ret := make([]port.TemplateInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PortTemplateInfo(item))
	}
	return ret
}

func PortServiceConfig(item configcore.ServiceConfiguration) port.ServiceConfig {
	ret := port.ServiceConfig{
		ServerAddr: item.Server.Addr,
		Backend:    item.Repository.Backend,
		DataDir:    item.Repository.DataDir,
		DefaultChat: port.ProviderModelTarget{
			ProviderID: item.LLM.DefaultChat.ProviderID,
			ModelID:    item.LLM.DefaultChat.ModelID,
		},
		MaxConcurrentRuns: item.Server.MaxConcurrentRuns,
		AIProxyBaseURL:    item.LLM.AIProxy.BaseURL,
		LLMOutput: port.LLMOutputConfig{
			MaxPatchContentBytes:               item.Runtime.LLMOutput.MaxPatchContentBytes,
			MaxGraphBytes:                      item.Runtime.LLMOutput.MaxGraphBytes,
			MaxGraphFileWrites:                 item.Runtime.LLMOutput.MaxGraphFileWrites,
			SchemaInvalidLargePreviewThreshold: item.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			MaxArtifactChunkBytes:              item.Runtime.LLMOutput.MaxArtifactChunkBytes,
			PreviousChunkTailBytes:             item.Runtime.LLMOutput.PreviousChunkTailBytes,
			MaxChunkRetryAttempts:              item.Runtime.LLMOutput.MaxChunkRetryAttempts,
			MaxArtifactPlanAttempts:            item.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			MaxArtifactReplanAttempts:          item.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}
	for _, model := range item.LLM.Catalog.Models {
		if !model.CapabilityReady || !containsString(model.Operations, "chat_completions") {
			continue
		}
		ret.ChatModelTargets = append(ret.ChatModelTargets, port.ChatModelTargetOption{
			ProviderID: model.ProviderID, ModelID: model.ModelID, DisplayName: model.ModelID,
			ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
			CapabilityReady: true, CapabilityRevision: model.CapabilityRevision, Source: item.LLM.Catalog.Source,
		})
	}
	for _, provider := range item.LLM.CustomProviders {
		for _, model := range provider.Models {
			if !containsString(model.Operations, "chat_completions") {
				continue
			}
			ret.ChatModelTargets = append(ret.ChatModelTargets, port.ChatModelTargetOption{
				ProviderID: provider.ID, ModelID: model.ID, DisplayName: model.ID,
				ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
				CapabilityReady: true, Source: "configuration",
			})
		}
	}
	return ret
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func portWorkspaceOutcomeInfo(item readmodelcore.WorkspaceOutcome) port.WorkspaceOutcomeInfo {
	return port.WorkspaceOutcomeInfo{
		WorkspaceID:  common.WorkspaceID(item.WorkspaceID),
		State:        item.State,
		RunID:        common.RunID(item.RunID),
		ResultID:     common.ResultID(item.ResultID),
		Accepted:     item.Accepted,
		Terminal:     item.Terminal,
		Summary:      item.Summary,
		Reason:       item.Reason,
		EvidenceRefs: cloneStrings(item.EvidenceRefs),
		ArtifactRefs: cloneStrings(item.ArtifactRefs),
	}
}

func portTaskProgressInfo(item readmodelcore.TaskProgress) port.TaskProgressInfo {
	return port.TaskProgressInfo{
		WorkspaceID:        common.WorkspaceID(item.WorkspaceID),
		RunID:              common.RunID(item.RunID),
		State:              item.State,
		Summary:            item.Summary,
		CurrentStepID:      common.StepID(item.CurrentStepID),
		CurrentStepKind:    item.CurrentStepKind,
		CurrentStepState:   item.CurrentStepState,
		CurrentActionLabel: item.CurrentActionLabel,
		DiagnosticLineage:  common.StepID(item.DiagnosticLineage),
		CurrentMilestone:   item.CurrentMilestone,
		Milestones:         portProgressMilestones(item.Milestones),
		Recovery:           portRecoveryPlanInfo(item.Recovery),
		RepairLoop:         portRepairLoopInfo(item.RepairLoop),
		AssessmentOutcome: port.AssessmentOutcomeInfo{
			State:                       item.AssessmentOutcome.State,
			AcceptedSlices:              item.AssessmentOutcome.AcceptedSlices,
			AcceptedResultCount:         item.AssessmentOutcome.AcceptedResultCount,
			RejectedGlobalAssessments:   item.AssessmentOutcome.RejectedGlobalAssessments,
			SatisfiedCoverageCount:      item.AssessmentOutcome.SatisfiedCoverageCount,
			RemainingCoverageCount:      item.AssessmentOutcome.RemainingCoverageCount,
			NewlySatisfiedCoverageCount: item.AssessmentOutcome.NewlySatisfiedCoverageCount,
			LatestAcceptedResultID:      common.ResultID(item.AssessmentOutcome.LatestAcceptedResultID),
			LatestRejectedResultID:      common.ResultID(item.AssessmentOutcome.LatestRejectedResultID),
			LatestRejectedReason:        item.AssessmentOutcome.LatestRejectedReason,
		},
		RuntimeV2: portRuntimeV2ProgressInfo(item.RuntimeV2),
	}
}

func portProgressMilestones(items []readmodelcore.ProgressMilestone) []port.ProgressMilestoneInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]port.ProgressMilestoneInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ProgressMilestoneInfo{
			ID: item.ID, Title: item.Title, State: item.State, EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret
}

func portRuntimeV2ProgressInfo(item *readmodelcore.RuntimeV2Progress) *port.RuntimeV2ProgressInfo {
	if item == nil {
		return nil
	}
	return &port.RuntimeV2ProgressInfo{
		StateRevision: item.StateRevision, EvidenceSequence: item.EvidenceSequence,
		CompletedDispatchCount: item.CompletedDispatchCount, ActiveDispatchID: item.ActiveDispatchID,
		CandidateAttemptCount: item.CandidateAttemptCount, AcceptedEvidenceCount: item.AcceptedEvidenceCount,
		RejectedEvidenceCount: item.RejectedEvidenceCount, MissingEvidenceKinds: append([]string(nil), item.MissingEvidenceKinds...),
		CurrentFocusID: item.CurrentFocusID, CurrentAction: item.CurrentAction, FailureClass: item.FailureClass,
		FailureCode: item.FailureCode, FailureSignature: item.FailureSignature, FailureMessage: item.FailureMessage,
		RepairHintCodes: append([]string(nil), item.RepairHintCodes...),
	}
}

func portRecoveryPlanInfo(item readmodelcore.RecoveryPlan) port.RecoveryPlanInfo {
	return port.RecoveryPlanInfo{
		Mode:               item.Mode,
		NextAction:         item.NextAction,
		RunID:              common.RunID(item.RunID),
		StepID:             common.StepID(item.StepID),
		BlockingTicketID:   common.TicketID(item.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(item.BlockingApprovalID),
		CheckpointID:       common.CheckpointID(item.CheckpointID),
		FailureClass:       item.FailureClass,
		FailureSignature:   item.FailureSignature,
		Reason:             item.Reason,
	}
}

func portRepairLoopInfo(item readmodelcore.RepairLoop) port.RepairLoopInfo {
	return port.RepairLoopInfo{
		State:                    item.State,
		AttemptCount:             item.AttemptCount,
		LastAttempt:              item.LastAttempt,
		LastRepairStepID:         common.StepID(item.LastRepairStepID),
		LastFailureStepID:        common.StepID(item.LastFailureStepID),
		LastFailureClass:         item.LastFailureClass,
		LastFailureSignature:     item.LastFailureSignature,
		LastFailureTargetScope:   item.LastFailureTargetScope,
		LastFailureAt:            item.LastFailureAt,
		StructuredFailureSignals: portPayload(item.StructuredFailureSignals),
		RepairInputCompaction:    portPayload(item.RepairInputCompaction),
		LastRepairInputBytes:     item.LastRepairInputBytes,
		RepeatedFailureReason:    item.RepeatedFailureReason,
		NextAction:               item.NextAction,
		Escalated:                item.Escalated,
		EscalationReason:         item.EscalationReason,
		EscalationTicketID:       common.TicketID(item.EscalationTicketID),
		EvidenceRefs:             cloneStrings(item.EvidenceRefs),
	}
}

func portRunHeartbeatInfo(item readmodelcore.RunHeartbeat) port.RunHeartbeatInfo {
	return port.RunHeartbeatInfo{
		WorkspaceID:               common.WorkspaceID(item.WorkspaceID),
		RunID:                     common.RunID(item.RunID),
		State:                     item.State,
		ActivityState:             item.ActivityState,
		LatestStepID:              common.StepID(item.LatestStepID),
		LatestStepKind:            item.LatestStepKind,
		LatestStepState:           item.LatestStepState,
		LatestEvidenceRef:         item.LatestEvidenceRef,
		LatestEventTopic:          item.LatestEventTopic,
		LatestEventRevision:       item.LatestEventRevision,
		LatestEventAt:             item.LatestEventAt,
		LatestLLMUsageAt:          item.LatestLLMUsageAt,
		LatestResultAt:            item.LatestResultAt,
		LatestArtifactAt:          item.LatestArtifactAt,
		LatestPatchAt:             item.LatestPatchAt,
		LatestEffectiveProgressAt: item.LatestEffectiveProgressAt,
		IneffectiveRetry:          item.IneffectiveRetry,
		Stalled:                   item.Stalled,
		Summary:                   item.Summary,
	}
}

func portConvergenceStatusInfo(item *readmodelcore.ConvergenceStatus) *port.ConvergenceStatusInfo {
	if item == nil {
		return nil
	}
	return &port.ConvergenceStatusInfo{
		WorkspaceID:               common.WorkspaceID(item.WorkspaceID),
		RunID:                     common.RunID(item.RunID),
		RunState:                  common.RunState(item.RunState),
		CurrentFocusID:            item.CurrentFocusID,
		NextActionKind:            item.NextActionKind,
		CurrentGap:                item.CurrentGap,
		MissingEvidenceKinds:      cloneStrings(item.MissingEvidenceKinds),
		AcceptedEvidenceCount:     item.AcceptedEvidenceCount,
		RejectedEvidenceCount:     item.RejectedEvidenceCount,
		LatestEffectiveProgressAt: item.LatestEffectiveProgressAt,
		DecisionCause:             item.DecisionCause,
		Stalled:                   item.Stalled,
		IneffectiveRetry:          item.IneffectiveRetry,
	}
}

func portRunCommandStatusInfo(item *readmodelcore.RunCommandStatus) *port.RunCommandStatusInfo {
	if item == nil {
		return nil
	}
	return &port.RunCommandStatusInfo{
		WorkspaceID:        common.WorkspaceID(item.WorkspaceID),
		RunID:              common.RunID(item.RunID),
		State:              item.State,
		Intent:             item.Intent,
		Reason:             item.Reason,
		RequestID:          item.RequestID,
		IdempotencyKey:     item.IdempotencyKey,
		Source:             item.Source,
		DefinitionRevision: item.DefinitionRevision,
		DecisionID:         common.TicketID(item.DecisionID),
		AcceptedAt:         item.AcceptedAt,
		RejectedAt:         item.RejectedAt,
		Error:              item.Error,
		Revision:           item.Revision,
	}
}

func PortSchedulerDiagnosticsInfo(item *readmodelcore.SchedulerDiagnostics) *port.SchedulerDiagnosticsInfo {
	if item == nil {
		return nil
	}
	return &port.SchedulerDiagnosticsInfo{
		Watchdog:                    portWatchdogDiagnosticsInfo(item.Watchdog),
		LatestContinuation:          portContinuationDiagnosticInfo(item.LatestContinuation),
		RecentContinuations:         portContinuationDiagnosticInfos(item.RecentContinuations),
		ContinuationFailureCounts:   portContinuationDiagnosticCountInfos(item.ContinuationFailureCounts),
		ContinuationNoActionCounts:  portContinuationDiagnosticCountInfos(item.ContinuationNoActionCounts),
		ContinuationCompletedCounts: portContinuationDiagnosticCountInfos(item.ContinuationCompletedCounts),
	}
}

func portWatchdogDiagnosticsInfo(item readmodelcore.WatchdogDiagnostics) port.WatchdogDiagnosticsInfo {
	return port.WatchdogDiagnosticsInfo{
		TimerRegistrations: item.TimerRegistrations,
		LatestTickAt:       item.LatestTickAt,
		LatestDurationMs:   item.LatestDurationMs,
		LatestError:        item.LatestError,
		StaleRecoveryRuns:  item.StaleRecoveryRuns,
	}
}

func portContinuationDiagnosticInfo(item *readmodelcore.ContinuationDiagnostic) *port.ContinuationDiagnosticInfo {
	if item == nil {
		return nil
	}
	return &port.ContinuationDiagnosticInfo{
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		Kind:               item.Kind,
		State:              item.State,
		Reason:             item.Reason,
		Detail:             item.Detail,
		Readback:           portContinuationReadbackDiagnosticInfo(item.Readback),
		Recoverability:     item.Recoverability,
		RecommendedActions: cloneStrings(item.RecommendedActions),
		DedupeKey:          item.DedupeKey,
		Attempt:            item.Attempt,
		OccurredAt:         item.OccurredAt,
		Source:             item.Source,
	}
}

func portContinuationDiagnosticInfos(items []readmodelcore.ContinuationDiagnostic) []port.ContinuationDiagnosticInfo {
	ret := make([]port.ContinuationDiagnosticInfo, 0, len(items))
	for _, item := range items {
		if mapped := portContinuationDiagnosticInfo(&item); mapped != nil {
			ret = append(ret, *mapped)
		}
	}
	return ret
}

func portContinuationReadbackDiagnosticInfo(item *readmodelcore.ContinuationReadbackDiagnostic) *port.ContinuationReadbackDiagnosticInfo {
	if item == nil {
		return nil
	}
	return &port.ContinuationReadbackDiagnosticInfo{
		WorkspaceID:             item.WorkspaceID,
		RunID:                   item.RunID,
		Action:                  item.Action,
		Found:                   item.Found,
		OldRunState:             item.OldRunState,
		NewRunState:             item.NewRunState,
		OldBlockingTicketID:     item.OldBlockingTicketID,
		NewBlockingTicketID:     item.NewBlockingTicketID,
		OldBlockingApprovalID:   item.OldBlockingApprovalID,
		NewBlockingApprovalID:   item.NewBlockingApprovalID,
		SuccessorRunID:          item.SuccessorRunID,
		LatestStepID:            item.LatestStepID,
		Terminal:                item.Terminal,
		GovernedWait:            item.GovernedWait,
		Advanced:                item.Advanced,
		Reason:                  item.Reason,
		ProjectionRevision:      item.ProjectionRevision,
		SourceEventRevision:     item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision,
	}
}

func portContinuationDiagnosticCountInfos(items []readmodelcore.ContinuationDiagnosticCount) []port.ContinuationDiagnosticCountInfo {
	ret := make([]port.ContinuationDiagnosticCountInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ContinuationDiagnosticCountInfo{
			Kind:   item.Kind,
			Reason: item.Reason,
			Count:  item.Count,
		})
	}
	return ret
}

func portAcceptanceProjectionInfo(item readmodelcore.AcceptanceProjection) port.AcceptanceProjectionInfo {
	ret := port.AcceptanceProjectionInfo{
		ActiveSliceID: item.ActiveSliceID,
		Summary: port.AcceptanceSummaryInfo{
			AcceptedSlices: item.Summary.AcceptedSlices,
			ActiveSlices:   item.Summary.ActiveSlices,
			OpenSlices:     item.Summary.OpenSlices,
			FailedSlices:   item.Summary.FailedSlices,
		},
	}
	for _, slice := range item.Slices {
		ret.Slices = append(ret.Slices, port.AcceptanceSliceInfo{
			ID:                    slice.ID,
			Title:                 slice.Title,
			State:                 slice.State,
			Priority:              slice.Priority,
			Scope:                 slice.Scope,
			CoverageWorkItemIDs:   cloneStrings(slice.CoverageWorkItemIDs),
			RequiredEvidenceKinds: cloneStrings(slice.RequiredEvidenceKinds),
			CompletionPolicy:      slice.CompletionPolicy,
			ResultPolicy:          slice.ResultPolicy,
			CreatedAt:             slice.CreatedAt,
			ActivatedAt:           slice.ActivatedAt,
			ClosedAt:              slice.ClosedAt,
		})
	}
	if item.CurrentFocus != nil {
		ret.CurrentFocus = &port.CoverageFocusInfo{
			ID:                    item.CurrentFocus.ID,
			SliceID:               item.CurrentFocus.SliceID,
			CoverageWorkItemIDs:   cloneStrings(item.CurrentFocus.CoverageWorkItemIDs),
			FailureClass:          item.CurrentFocus.FailureClass,
			FailureSignature:      item.CurrentFocus.FailureSignature,
			RequiredEvidenceKinds: cloneStrings(item.CurrentFocus.RequiredEvidenceKinds),
			AcceptedEvidenceRefs:  cloneStrings(item.CurrentFocus.AcceptedEvidenceRefs),
			RejectedEvidenceRefs:  cloneStrings(item.CurrentFocus.RejectedEvidenceRefs),
			TargetRefs:            cloneStrings(item.CurrentFocus.TargetRefs),
			State:                 item.CurrentFocus.State,
			Attempt:               item.CurrentFocus.Attempt,
		}
	}
	if item.Budget != nil {
		ret.Budget = &port.ConvergenceBudgetInfo{
			State:                       item.Budget.State,
			Decision:                    item.Budget.Decision,
			MaxRepairAttemptsPerFocus:   item.Budget.MaxRepairAttemptsPerFocus,
			MaxLLMRoundsPerSlice:        item.Budget.MaxLLMRoundsPerSlice,
			MaxRepeatedFailureSignature: item.Budget.MaxRepeatedFailureSignature,
			MaxValidationOnlyRetries:    item.Budget.MaxValidationOnlyRetries,
			MaxNoCoverageDeltaRounds:    item.Budget.MaxNoCoverageDeltaRounds,
			MaxContextBytesPerRepair:    item.Budget.MaxContextBytesPerRepair,
			RepairAttempts:              item.Budget.RepairAttempts,
			LLMRounds:                   item.Budget.LLMRounds,
			RepeatedFailureSignatures:   item.Budget.RepeatedFailureSignatures,
			ValidationOnlyRetries:       item.Budget.ValidationOnlyRetries,
			NoCoverageDeltaRounds:       item.Budget.NoCoverageDeltaRounds,
			LastDecisionReason:          item.Budget.LastDecisionReason,
		}
	}
	return ret
}

func portTaskDecompositionInfos(items []readmodelcore.TaskDecomposition) []port.TaskDecompositionInfo {
	ret := make([]port.TaskDecompositionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.TaskDecompositionInfo{
			ID:            item.ID,
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			RootStepID:    common.StepID(item.RootStepID),
			State:         item.State,
			Summary:       item.Summary,
			GraphBranch:   item.GraphBranch,
			NodeTotal:     item.NodeTotal,
			NodeCompleted: item.NodeCompleted,
			NodeFailed:    item.NodeFailed,
			NodeRunning:   item.NodeRunning,
			Nodes:         portTaskDecompositionNodeInfos(item.Nodes),
			EvidenceRefs:  cloneStrings(item.EvidenceRefs),
		})
	}
	return ret
}

func portTaskPlanInfos(items []readmodelcore.TaskPlan) []port.TaskPlanInfo {
	ret := make([]port.TaskPlanInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.TaskPlanInfo{
			ID:                item.ID,
			WorkspaceID:       common.WorkspaceID(item.WorkspaceID),
			RunID:             common.RunID(item.RunID),
			AnchorStepID:      common.StepID(item.AnchorStepID),
			RevisionStepID:    common.StepID(item.RevisionStepID),
			Revision:          item.Revision,
			State:             item.State,
			Stage:             item.Stage,
			SourceOutputState: item.SourceOutputState,
			Summary:           item.Summary,
			GraphBranch:       item.GraphBranch,
			NodeTotal:         item.NodeTotal,
			NodeCompleted:     item.NodeCompleted,
			NodeFailed:        item.NodeFailed,
			NodeRunning:       item.NodeRunning,
			Nodes:             portTaskDecompositionNodeInfos(item.Nodes),
			RetryOf:           item.RetryOf,
			RetryAttempt:      item.RetryAttempt,
			FailureClass:      item.FailureClass,
			FailureSignature:  item.FailureSignature,
			TargetScope:       item.TargetScope,
			BlockingTicketID:  common.TicketID(item.BlockingTicketID),
			EvidenceRefs:      cloneStrings(item.EvidenceRefs),
			OccurredAt:        item.OccurredAt,
			SourceProjection:  item.SourceProjection,
		})
	}
	return ret
}

func portTaskDecompositionNodeInfos(items []readmodelcore.TaskDecompositionNode) []port.TaskDecompositionNodeInfo {
	ret := make([]port.TaskDecompositionNodeInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.TaskDecompositionNodeInfo{
			ID:               item.ID,
			Kind:             item.Kind,
			Title:            item.Title,
			State:            item.State,
			DependsOn:        cloneStrings(item.DependsOn),
			Branch:           item.Branch,
			StepID:           common.StepID(item.StepID),
			StepState:        item.StepState,
			RetryOf:          item.RetryOf,
			RetryAttempt:     item.RetryAttempt,
			FailureClass:     item.FailureClass,
			FailureSignature: item.FailureSignature,
			TargetScope:      item.TargetScope,
			BlockingTicketID: common.TicketID(item.BlockingTicketID),
			EvidenceRefs:     cloneStrings(item.EvidenceRefs),
			CoverageTargets:  cloneStrings(item.CoverageTargets),
			Input:            portPayload(item.Input),
		})
	}
	return ret
}

func portResultInfos(items []readmodelcore.Result) []port.ResultInfo {
	ret := make([]port.ResultInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ResultInfo{
			ID:            common.ResultID(item.ID),
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			GoalID:        common.GoalID(item.GoalID),
			DoDID:         common.DoDID(item.DoDID),
			State:         common.ResultState(item.State),
			Title:         item.Title,
			Summary:       item.Summary,
			SourceType:    item.SourceType,
			SourceRef:     item.SourceRef,
			EvidenceRefs:  cloneStrings(item.EvidenceRefs),
			ArtifactRefs:  cloneStrings(item.ArtifactRefs),
			SummaryDetail: portResultSummaryInfo(item.SummaryDetail),
		})
	}
	return ret
}

func portResultSummaryInfo(item *readmodelcore.ResultSummary) *port.ResultSummaryInfo {
	if item == nil {
		return nil
	}
	return &port.ResultSummaryInfo{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneStrings(item.EvidenceRefs),
		ArtifactRefs:          cloneStrings(item.ArtifactRefs),
		KnownLimitations:      cloneStrings(item.KnownLimitations),
		ResidualRisks:         cloneStrings(item.ResidualRisks),
		NextActions:           cloneStrings(item.NextActions),
		Diagnostics:           portPayload(item.Diagnostics),
	}
}

func portArtifactInfos(items []readmodelcore.Artifact) []port.ArtifactInfo {
	ret := make([]port.ArtifactInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ArtifactInfo{
			ID:           common.ArtifactID(item.ID),
			WorkspaceID:  common.WorkspaceID(item.WorkspaceID),
			RunID:        common.RunID(item.RunID),
			ResultID:     common.ResultID(item.ResultID),
			ArtifactType: item.ArtifactType,
			Category:     item.Category,
			Name:         item.Name,
			Summary:      item.Summary,
			URI:          item.URI,
			ContentType:  item.ContentType,
			State:        common.ArtifactState(item.State),
			EvidenceRefs: cloneStrings(item.EvidenceRefs),
		})
	}
	return ret
}

func portArtifactContinuationInfos(items []readmodelcore.ArtifactContinuation) []port.ArtifactContinuationInfo {
	ret := make([]port.ArtifactContinuationInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ArtifactContinuationInfo{
			ID:                common.ArtifactContinuationID(item.ID),
			WorkspaceID:       common.WorkspaceID(item.WorkspaceID),
			RunID:             common.RunID(item.RunID),
			RootStepID:        common.StepID(item.RootStepID),
			ArtifactID:        item.ArtifactID,
			TargetRef:         item.TargetRef,
			Operation:         item.Operation,
			State:             common.ArtifactContinuationState(item.State),
			ExpectedSequence:  item.ExpectedSequence,
			ReceivedSequences: cloneInts(item.ReceivedSequences),
			FinalReceived:     item.FinalReceived,
			BytesWritten:      item.BytesWritten,
			RollingHash:       item.RollingHash,
			LastChunkHash:     item.LastChunkHash,
			CoverageTargets:   cloneStrings(item.CoverageTargets),
			EvidenceRefs:      cloneStrings(item.EvidenceRefs),
			FailureReason:     item.FailureReason,
			CreatedAt:         item.CreatedAt,
			UpdatedAt:         item.UpdatedAt,
			Progress:          portPayload(item.Progress),
		})
	}
	return ret
}

func portPatchInfos(items []readmodelcore.Patch) []port.PatchInfo {
	ret := make([]port.PatchInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.PatchInfo{
			ID:               common.PatchID(item.ID),
			WorkspaceID:      common.WorkspaceID(item.WorkspaceID),
			RunID:            common.RunID(item.RunID),
			StepID:           common.StepID(item.StepID),
			ToolCallID:       common.ToolCallID(item.ToolCallID),
			ApprovalID:       common.ApprovalID(item.ApprovalID),
			RevertApprovalID: common.ApprovalID(item.RevertApprovalID),
			PatchType:        item.PatchType,
			TargetRef:        item.TargetRef,
			Summary:          item.Summary,
			State:            common.PatchState(item.State),
			AddedLines:       item.AddedLines,
			DeletedLines:     item.DeletedLines,
			BeforeHash:       item.BeforeHash,
			AfterHash:        item.AfterHash,
			AffectedFiles:    cloneStrings(item.AffectedFiles),
			RiskLevel:        item.RiskLevel,
			RiskReasons:      cloneStrings(item.RiskReasons),
			Diagnostics:      portPayloadSlice(item.Diagnostics),
			CreatedAt:        item.CreatedAt,
			AppliedAt:        item.AppliedAt,
			RevertedAt:       item.RevertedAt,
		})
	}
	return ret
}

func portMemoryRecallTraceInfos(items []readmodelcore.MemoryRecallTrace) []port.MemoryRecallTraceInfo {
	ret := make([]port.MemoryRecallTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.MemoryRecallTraceInfo{
			ID:                  common.MemoryRecallTraceID(item.ID),
			WorkspaceID:         common.WorkspaceID(item.WorkspaceID),
			RunID:               common.RunID(item.RunID),
			StepID:              common.StepID(item.StepID),
			GraphNodeID:         item.GraphNodeID,
			Stage:               item.Stage,
			Source:              item.Source,
			Policy:              item.Policy,
			Status:              item.Status,
			ProviderID:          item.ProviderID,
			Operation:           item.Operation,
			QueryTextPreview:    item.QueryTextPreview,
			QueryTextSha256:     item.QueryTextSha256,
			MinScore:            item.MinScore,
			Limit:               item.Limit,
			EmbeddingModel:      item.EmbeddingModel,
			VectorDimension:     item.VectorDimension,
			RetrievedMemoryIDs:  memoryIDs(item.RetrievedMemoryIDs),
			RetrievedMemoryHits: portMemoryRecallHitInfos(item.RetrievedMemoryHits),
			Reason:              item.Reason,
			OccurredAt:          item.OccurredAt,
			EvidenceRef:         item.EvidenceRef,
		})
	}
	return ret
}

func portMemoryRecallHitInfos(items []readmodelcore.MemoryRecallHit) []port.MemoryRecallHitInfo {
	ret := make([]port.MemoryRecallHitInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.MemoryRecallHitInfo{
			ID:              common.MemoryID(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func portWebSourceTraceInfos(items []readmodelcore.WebSourceTrace) []port.WebSourceTraceInfo {
	ret := make([]port.WebSourceTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.WebSourceTraceInfo{
			Query:          item.Query,
			ProviderID:     item.ProviderID,
			SearchedAt:     item.SearchedAt,
			Sources:        portWebSourceInfos(item.Sources),
			FetchedSources: portFetchedWebSourceInfos(item.FetchedSources),
			EvidenceRefs:   cloneStrings(item.EvidenceRefs),
			ApprovalID:     common.ApprovalID(item.ApprovalID),
			PolicyDecision: item.PolicyDecision,
		})
	}
	return ret
}

func portWebSourceInfos(items []readmodelcore.WebSource) []port.WebSourceInfo {
	ret := make([]port.WebSourceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.WebSourceInfo{
			Title:       item.Title,
			URL:         item.URL,
			Snippet:     item.Snippet,
			Source:      item.Source,
			PublishedAt: item.PublishedAt,
			Rank:        item.Rank,
			EvidenceRef: item.EvidenceRef,
		})
	}
	return ret
}

func portFetchedWebSourceInfos(items []readmodelcore.FetchedWebSource) []port.FetchedWebSourceInfo {
	ret := make([]port.FetchedWebSourceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.FetchedWebSourceInfo{
			URL:         item.URL,
			FinalURL:    item.FinalURL,
			Title:       item.Title,
			Source:      item.Source,
			RetrievedAt: item.RetrievedAt,
			ContentHash: item.ContentHash,
			Truncated:   item.Truncated,
			EvidenceRef: item.EvidenceRef,
		})
	}
	return ret
}

func portLLMContextBudgetTraceInfos(items []readmodelcore.LLMContextBudgetTrace) []port.LLMContextBudgetTraceInfo {
	ret := make([]port.LLMContextBudgetTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.LLMContextBudgetTraceInfo{
			StepID:                common.StepID(item.StepID),
			ProviderID:            item.ProviderID,
			Model:                 item.Model,
			MaxBytes:              item.MaxBytes,
			ActualBytes:           item.ActualBytes,
			MaxTokens:             item.MaxTokens,
			EstimatedInputTokens:  item.EstimatedInputTokens,
			EstimatedOutputTokens: item.EstimatedOutputTokens,
			ActualTokens:          item.ActualTokens,
			ShrinkLevel:           item.ShrinkLevel,
			Truncated:             item.Truncated,
			Rules:                 cloneStrings(item.Rules),
			SourceRefs:            cloneStrings(item.SourceRefs),
			IncludedSections:      cloneStrings(item.IncludedSections),
			NormalizedSections:    cloneStrings(item.NormalizedSections),
			CompressedSections:    cloneStrings(item.CompressedSections),
			OmittedSections:       cloneStrings(item.OmittedSections),
			GoalDoDCompressed:     item.GoalDoDCompressed,
			OverBudget:            item.OverBudget,
			P0Overflow:            item.P0Overflow,
			ProfileMode:           item.ProfileMode,
			CapabilitySource:      item.CapabilitySource,
			ContextUtilization:    item.ContextUtilization,
			MaxRequestBytes:       item.MaxRequestBytes,
			StageBudgetRatio:      item.StageBudgetRatio,
			StageBudgetSource:     item.StageBudgetSource,
			TracePolicy:           item.TracePolicy,
			PromptCaptured:        item.PromptCaptured,
			Estimator:             item.Estimator,
			Profile:               item.Profile.Clone(),
			CachePlan:             item.CachePlan.Clone(),
		})
	}
	return ret
}

func portLLMContextBudgetSummaryInfo(item readmodelcore.LLMContextBudgetSummary) port.LLMContextBudgetSummaryInfo {
	return port.LLMContextBudgetSummaryInfo{
		TraceTotal:             item.TraceTotal,
		TruncatedCount:         item.TruncatedCount,
		OverBudgetCount:        item.OverBudgetCount,
		P0OverflowCount:        item.P0OverflowCount,
		GoalDoDCompressedCount: item.GoalDoDCompressedCount,
		PromptCapturedCount:    item.PromptCapturedCount,
		MaxShrinkLevel:         item.MaxShrinkLevel,
		MaxContextUtilization:  item.MaxContextUtilization,
		AvgContextUtilization:  item.AvgContextUtilization,
		MaxInputTokens:         item.MaxInputTokens,
		MaxRequestBytes:        item.MaxRequestBytes,
		RecentWindowSize:       item.RecentWindowSize,
		RecentTraceTotal:       item.RecentTraceTotal,
		RecentTruncatedCount:   item.RecentTruncatedCount,
		RecentOverBudgetCount:  item.RecentOverBudgetCount,
		RecentMaxUtilization:   item.RecentMaxUtilization,
		RecentAvgUtilization:   item.RecentAvgUtilization,
		RecentMaxInputTokens:   item.RecentMaxInputTokens,
		UtilizationTrend:       item.UtilizationTrend,
	}
}

func portLLMUsageTraceInfos(items []readmodelcore.LLMUsageTrace) []port.LLMUsageTraceInfo {
	ret := make([]port.LLMUsageTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.LLMUsageTraceInfo{
			ID:            item.ID,
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			StepID:        common.StepID(item.StepID),
			ProviderID:    item.ProviderID,
			Provider:      item.Provider,
			Model:         item.Model,
			Operation:     item.Operation,
			InputTokens:   item.InputTokens,
			OutputTokens:  item.OutputTokens,
			TotalTokens:   item.TotalTokens,
			Estimated:     item.Estimated,
			UsageSource:   item.UsageSource,
			RequestBytes:  item.RequestBytes,
			ResponseBytes: item.ResponseBytes,
			Streaming:     item.Streaming,
			ContextBudget: item.ContextBudget.Clone(),
			CacheUsage:    item.CacheUsage.Clone(),
			Status:        item.Status,
			FailureClass:  item.FailureClass,
			OccurredAt:    item.OccurredAt,
			EvidenceRef:   item.EvidenceRef,
		})
	}
	return ret
}

func portLLMUsageSummaryInfo(item readmodelcore.LLMUsageSummary) port.LLMUsageSummaryInfo {
	return port.LLMUsageSummaryInfo{
		EntryTotal:              item.EntryTotal,
		InputTokens:             item.InputTokens,
		OutputTokens:            item.OutputTokens,
		TotalTokens:             item.TotalTokens,
		EstimatedEntries:        item.EstimatedEntries,
		ProviderNativeEntries:   item.ProviderNativeEntries,
		FailedEntries:           item.FailedEntries,
		CacheReadTokens:         item.CacheReadTokens,
		CacheWriteTokens:        item.CacheWriteTokens,
		CacheCreationTokens:     item.CacheCreationTokens,
		CacheableInputTokens:    item.CacheableInputTokens,
		NonCacheableInputTokens: item.NonCacheableInputTokens,
		CacheEstimatedEntries:   item.CacheEstimatedEntries,
		CacheProviderEntries:    item.CacheProviderEntries,
		ProviderModelBreakdown:  portLLMUsageProviderModelBreakdownInfos(item.ProviderModelBreakdown),
	}
}

func portLLMUsageProviderModelBreakdownInfos(items []readmodelcore.LLMUsageProviderModelBreakdown) []port.LLMUsageProviderModelBreakdownInfo {
	ret := make([]port.LLMUsageProviderModelBreakdownInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.LLMUsageProviderModelBreakdownInfo{
			ProviderID:       item.ProviderID,
			Provider:         item.Provider,
			Model:            item.Model,
			EntryTotal:       item.EntryTotal,
			InputTokens:      item.InputTokens,
			OutputTokens:     item.OutputTokens,
			TotalTokens:      item.TotalTokens,
			CacheReadTokens:  item.CacheReadTokens,
			CacheWriteTokens: item.CacheWriteTokens,
			EstimatedEntries: item.EstimatedEntries,
			FailedEntries:    item.FailedEntries,
		})
	}
	return ret
}

func portEventRecordInfos(items []readmodelcore.EventRecord) []port.EventRecordInfo {
	ret := make([]port.EventRecordInfo, 0, len(items))
	for _, item := range items {
		if len(item.Payload.Fields) == 0 {
			continue
		}
		typedPayload := workorchpayloadmapper.FromEventPayload(item.Payload)
		ret = append(ret, port.EventRecordInfo{
			ID:           common.EventRecordID(item.ID),
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  common.WorkspaceID(item.WorkspaceID),
			RunID:        common.RunID(item.RunID),
			StepID:       common.StepID(item.StepID),
			Payload:      typedPayload,
			Summary:      typedPayload.CompactSummary(),
			PayloadRef:   item.PayloadRef,
		})
	}
	return ret
}

func portContextBundle(item *readmodelcore.ContextBundle) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"workspaceId": string(common.WorkspaceID(item.WorkspaceID)),
		"runId":       string(common.RunID(item.RunID)),
		"mode":        item.Mode,
		"summary":     item.Summary,
	}
}

func portAutoGovernanceSummaryInfo(item *governancecore.AutoGovernanceSummary) *port.AutoGovernanceSummaryInfo {
	if item == nil {
		return nil
	}
	return &port.AutoGovernanceSummaryInfo{
		TraceID:           item.TraceID,
		Action:            item.Action,
		Status:            item.Status,
		RiskLevel:         item.RiskLevel,
		PolicyMatched:     item.PolicyMatched,
		PolicyBlocked:     item.PolicyBlocked,
		Confidence:        item.Confidence,
		Reason:            item.Reason,
		EvidenceRefs:      cloneStrings(item.EvidenceRefs),
		PromptFingerprint: item.PromptFingerprint,
		OccurredAt:        item.OccurredAt,
	}
}

func portAppWorkspaceViewModelInfo(item appmodelcore.WorkspaceViewModel) port.WorkspaceViewModelInfo {
	return port.WorkspaceViewModelInfo{
		WorkspaceID:             common.WorkspaceID(item.WorkspaceID),
		State:                   item.State,
		PromptMode:              item.PromptMode,
		Revision:                item.Revision,
		GlobalRevision:          item.GlobalRevision,
		WorkspaceRevision:       item.WorkspaceRevision,
		SourceEventRevision:     item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision,
		UpdatedAt:               item.UpdatedAt,
		RebuiltAt:               item.RebuiltAt,
		Degraded:                item.Degraded,
		StaleReason:             item.StaleReason,
		Error:                   item.Error,
		Sections:                portAppViewModelSections(item.Sections),
	}
}

func portProjectionFromAppViewModel(item appmodelcore.WorkspaceViewModel) port.ProjectionInfo {
	return port.ProjectionInfo{
		State:                   item.State,
		GlobalRevision:          item.GlobalRevision,
		WorkspaceRevision:       item.WorkspaceRevision,
		SourceEventRevision:     item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision,
		UpdatedAt:               item.UpdatedAt,
		RebuiltAt:               item.RebuiltAt,
		Degraded:                item.Degraded,
		StaleReason:             item.StaleReason,
		Error:                   item.Error,
	}
}

func portReadModelViewModelSections(items map[string]readmodelcore.WorkspaceViewModelSection) map[string]port.WorkspaceViewModelSectionInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make(map[string]port.WorkspaceViewModelSectionInfo, len(items))
	for key, item := range items {
		ret[key] = port.WorkspaceViewModelSectionInfo{
			Name:                    item.Name,
			Revision:                item.Revision,
			WorkspaceRevision:       item.WorkspaceRevision,
			SourceEventRevision:     item.SourceEventRevision,
			ProjectedSourceRevision: item.ProjectedSourceRevision,
			State:                   item.State,
		}
	}
	return ret
}

func portAppViewModelSections(items map[string]appmodelcore.WorkspaceViewModelSection) map[string]port.WorkspaceViewModelSectionInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make(map[string]port.WorkspaceViewModelSectionInfo, len(items))
	for key, item := range items {
		ret[key] = port.WorkspaceViewModelSectionInfo{
			Name:                    item.Name,
			Revision:                item.Revision,
			WorkspaceRevision:       item.WorkspaceRevision,
			SourceEventRevision:     item.SourceEventRevision,
			ProjectedSourceRevision: item.ProjectedSourceRevision,
			State:                   item.State,
		}
	}
	return ret
}

func portSoulBindings(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		ret[key] = common.SoulID(value)
	}
	return ret
}

func memoryIDs(values []string) []common.MemoryID {
	ret := make([]common.MemoryID, 0, len(values))
	for _, value := range values {
		ret = append(ret, common.MemoryID(value))
	}
	return ret
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	return append([]int(nil), values...)
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneDTOMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneDTOValue(value, 0)
	}
	return ret
}

func cloneDTOMapSlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneDTOMap(item))
	}
	return ret
}

func portPayload(input map[string]any) workorcheventpayload.Payload {
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		return workorcheventpayload.Payload{}
	}
	return payload
}

func portPayloadSlice(input []map[string]any) []workorcheventpayload.Payload {
	payloads, err := workorcheventpayload.FromMapSlice(input)
	if err != nil {
		return nil
	}
	return payloads
}

func cloneDTOValue(value any, depth int) any {
	if depth >= 8 {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, value := range typed {
			ret[key] = cloneDTOValue(value, depth+1)
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneDTOMap(item))
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneDTOValue(item, depth+1))
		}
		return ret
	case []string:
		return cloneStrings(typed)
	case []int:
		return cloneInts(typed)
	default:
		return value
	}
}
