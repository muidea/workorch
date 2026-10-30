package readmodelprojection

import (
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

// Identity equality helpers compare ordered typed projection facts by stable
// display/contract fields. Same-length slices with content mutations must still
// mark ChangedSections (RR05/CR02). Map-bearing fields are projected to sorted
// key/value comparison facts — no JSON marshal, reflect, or map authority.

func decisionIdentitiesEqual(a, b []projectioncontract.Decision) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !decisionEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func decisionEqual(a, b projectioncontract.Decision) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		strings.TrimSpace(a.DecisionKind) == strings.TrimSpace(b.DecisionKind) &&
		strings.TrimSpace(a.Question) == strings.TrimSpace(b.Question) &&
		a.Context.Equal(b.Context) &&
		strings.TrimSpace(a.FailureClass) == strings.TrimSpace(b.FailureClass) &&
		strings.TrimSpace(a.FailureSignature) == strings.TrimSpace(b.FailureSignature) &&
		stringSliceEqual(a.AllowedRecoveryActions, b.AllowedRecoveryActions) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.ResolutionID) == strings.TrimSpace(b.ResolutionID) &&
		strings.TrimSpace(a.ResolutionType) == strings.TrimSpace(b.ResolutionType) &&
		a.ResolutionPayload.Equal(b.ResolutionPayload) &&
		strings.TrimSpace(a.ContinuationAction) == strings.TrimSpace(b.ContinuationAction) &&
		strings.TrimSpace(a.ResumeRunID) == strings.TrimSpace(b.ResumeRunID) &&
		strings.TrimSpace(a.SuccessorRunID) == strings.TrimSpace(b.SuccessorRunID) &&
		autoGovernanceEqual(a.AutoGovernance, b.AutoGovernance) &&
		strings.TrimSpace(a.ResolvedAt) == strings.TrimSpace(b.ResolvedAt) &&
		strings.TrimSpace(a.ResolvedBy) == strings.TrimSpace(b.ResolvedBy) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func approvalIdentitiesEqual(a, b []projectioncontract.Approval) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !approvalEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func approvalEqual(a, b projectioncontract.Approval) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		strings.TrimSpace(a.Action) == strings.TrimSpace(b.Action) &&
		a.Context.Equal(b.Context) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		autoGovernanceEqual(a.AutoGovernance, b.AutoGovernance) &&
		strings.TrimSpace(a.ResolvedAt) == strings.TrimSpace(b.ResolvedAt) &&
		strings.TrimSpace(a.ResolvedBy) == strings.TrimSpace(b.ResolvedBy) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func resultIdentitiesEqual(a, b []projectioncontract.Result) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !resultEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func resultEqual(a, b projectioncontract.Result) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.GoalID) == strings.TrimSpace(b.GoalID) &&
		strings.TrimSpace(a.DoDID) == strings.TrimSpace(b.DoDID) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.Title) == strings.TrimSpace(b.Title) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.SourceType) == strings.TrimSpace(b.SourceType) &&
		strings.TrimSpace(a.SourceRef) == strings.TrimSpace(b.SourceRef) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		stringSliceEqual(a.ArtifactRefs, b.ArtifactRefs) &&
		doDAssessmentEqual(a.DoDAssessment, b.DoDAssessment) &&
		resultSummaryEqual(a.SummaryDetail, b.SummaryDetail) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func artifactIdentitiesEqual(a, b []projectioncontract.Artifact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !artifactEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func artifactEqual(a, b projectioncontract.Artifact) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.ResultID) == strings.TrimSpace(b.ResultID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.ToolCallID) == strings.TrimSpace(b.ToolCallID) &&
		strings.TrimSpace(a.PatchID) == strings.TrimSpace(b.PatchID) &&
		strings.TrimSpace(a.ValidationResultID) == strings.TrimSpace(b.ValidationResultID) &&
		strings.TrimSpace(a.ArtifactType) == strings.TrimSpace(b.ArtifactType) &&
		strings.TrimSpace(a.Category) == strings.TrimSpace(b.Category) &&
		strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.URI) == strings.TrimSpace(b.URI) &&
		strings.TrimSpace(a.ContentType) == strings.TrimSpace(b.ContentType) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		stableMapFactsEqual(a.Metadata, b.Metadata) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func patchIdentitiesEqual(a, b []projectioncontract.Patch) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !patchEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func patchEqual(a, b projectioncontract.Patch) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.ToolCallID) == strings.TrimSpace(b.ToolCallID) &&
		strings.TrimSpace(a.ApprovalID) == strings.TrimSpace(b.ApprovalID) &&
		strings.TrimSpace(a.RevertApprovalID) == strings.TrimSpace(b.RevertApprovalID) &&
		strings.TrimSpace(a.PatchType) == strings.TrimSpace(b.PatchType) &&
		strings.TrimSpace(a.TargetRef) == strings.TrimSpace(b.TargetRef) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		a.AddedLines == b.AddedLines &&
		a.DeletedLines == b.DeletedLines &&
		strings.TrimSpace(a.BeforeHash) == strings.TrimSpace(b.BeforeHash) &&
		strings.TrimSpace(a.AfterHash) == strings.TrimSpace(b.AfterHash) &&
		stringSliceEqual(a.AffectedFiles, b.AffectedFiles) &&
		strings.TrimSpace(a.RiskLevel) == strings.TrimSpace(b.RiskLevel) &&
		stringSliceEqual(a.RiskReasons, b.RiskReasons) &&
		stableMapSliceFactsEqual(a.Diagnostics, b.Diagnostics) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.AppliedAt) == strings.TrimSpace(b.AppliedAt) &&
		strings.TrimSpace(a.RevertedAt) == strings.TrimSpace(b.RevertedAt)
}

func memoryRecallIdentitiesEqual(a, b []projectioncontract.MemoryRecallTrace) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !memoryRecallEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func memoryRecallEqual(a, b projectioncontract.MemoryRecallTrace) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.GraphNodeID) == strings.TrimSpace(b.GraphNodeID) &&
		strings.TrimSpace(a.Stage) == strings.TrimSpace(b.Stage) &&
		strings.TrimSpace(a.Source) == strings.TrimSpace(b.Source) &&
		strings.TrimSpace(a.Policy) == strings.TrimSpace(b.Policy) &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.ProviderID) == strings.TrimSpace(b.ProviderID) &&
		strings.TrimSpace(a.ModelID) == strings.TrimSpace(b.ModelID) &&
		strings.TrimSpace(a.Operation) == strings.TrimSpace(b.Operation) &&
		strings.TrimSpace(a.QueryTextPreview) == strings.TrimSpace(b.QueryTextPreview) &&
		strings.TrimSpace(a.QueryTextSha256) == strings.TrimSpace(b.QueryTextSha256) &&
		a.MinScore == b.MinScore &&
		a.Limit == b.Limit &&
		strings.TrimSpace(a.EmbeddingModel) == strings.TrimSpace(b.EmbeddingModel) &&
		a.VectorDimension == b.VectorDimension &&
		strings.TrimSpace(a.RefreshSource) == strings.TrimSpace(b.RefreshSource) &&
		strings.TrimSpace(a.SelectionPolicyVersion) == strings.TrimSpace(b.SelectionPolicyVersion) &&
		stringSliceEqual(a.CandidateMemoryIDs, b.CandidateMemoryIDs) &&
		stringSliceEqual(a.SelectedMemoryIDs, b.SelectedMemoryIDs) &&
		stringSliceEqual(a.ExcludedMemoryIDs, b.ExcludedMemoryIDs) &&
		intMapFactsEqual(a.TypeQuota, b.TypeQuota) &&
		memorySelectionScoresEqual(a.SelectionScores, b.SelectionScores) &&
		stringSliceEqual(a.RetrievedMemoryIDs, b.RetrievedMemoryIDs) &&
		memoryHitsEqual(a.RetrievedMemoryHits, b.RetrievedMemoryHits) &&
		a.ContextBudget == b.ContextBudget &&
		recallSelectionEvidenceEqual(a.SemanticRecall, b.SemanticRecall) &&
		strings.TrimSpace(a.FallbackFrom) == strings.TrimSpace(b.FallbackFrom) &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt) &&
		strings.TrimSpace(a.EvidenceRef) == strings.TrimSpace(b.EvidenceRef)
}

func recallSelectionEvidenceEqual(a, b *workorchmemorycontract.RecallSelectionEvidence) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return workorchmemorycontract.SameRecallSelectionEvidence(*a, *b)
}

func llmUsageIdentitiesEqual(a, b []projectioncontract.LLMUsageTrace) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !llmUsageEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func llmUsageEqual(a, b projectioncontract.LLMUsageTrace) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.LogicalRequestID) == strings.TrimSpace(b.LogicalRequestID) &&
		strings.TrimSpace(a.PhysicalCallID) == strings.TrimSpace(b.PhysicalCallID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.ProviderID) == strings.TrimSpace(b.ProviderID) &&
		strings.TrimSpace(a.Provider) == strings.TrimSpace(b.Provider) &&
		strings.TrimSpace(a.APISurface) == strings.TrimSpace(b.APISurface) &&
		strings.TrimSpace(a.Model) == strings.TrimSpace(b.Model) &&
		strings.TrimSpace(a.Operation) == strings.TrimSpace(b.Operation) &&
		a.InputTokens == b.InputTokens &&
		a.OutputTokens == b.OutputTokens &&
		a.TotalTokens == b.TotalTokens &&
		a.Estimated == b.Estimated &&
		strings.TrimSpace(a.UsageSource) == strings.TrimSpace(b.UsageSource) &&
		a.RequestBytes == b.RequestBytes &&
		a.ResponseBytes == b.ResponseBytes &&
		a.PhysicalResponseMode == b.PhysicalResponseMode &&
		a.ProviderSubmitted == b.ProviderSubmitted &&
		a.ContextBudget.Equal(b.ContextBudget) &&
		a.CacheUsage.Equal(b.CacheUsage) &&
		a.CacheMarkers.Equal(b.CacheMarkers) &&
		strings.TrimSpace(a.StartedAt) == strings.TrimSpace(b.StartedAt) &&
		strings.TrimSpace(a.CompletedAt) == strings.TrimSpace(b.CompletedAt) &&
		a.DurationMs == b.DurationMs &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.FailureClass) == strings.TrimSpace(b.FailureClass) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt) &&
		strings.TrimSpace(a.EvidenceRef) == strings.TrimSpace(b.EvidenceRef)
}

func llmUsageSummaryEqual(a, b projectioncontract.LLMUsageSummary) bool {
	if !llmUsageSummaryTotalsEqual(a, b) {
		return false
	}
	return llmUsageProviderModelBreakdownsEqual(a.ProviderModelBreakdown, b.ProviderModelBreakdown)
}

func llmUsageSummaryTotalsEqual(a, b projectioncontract.LLMUsageSummary) bool {
	return a.NativeCache == b.NativeCache && a.Availability == b.Availability &&
		a.Scope == b.Scope &&
		a.RecordLimit == b.RecordLimit &&
		a.ObservedRecords == b.ObservedRecords &&
		a.EntryTotal == b.EntryTotal &&
		a.InputTokens == b.InputTokens &&
		a.OutputTokens == b.OutputTokens &&
		a.TotalTokens == b.TotalTokens &&
		a.EstimatedEntries == b.EstimatedEntries &&
		a.ProviderNativeEntries == b.ProviderNativeEntries &&
		a.FailedEntries == b.FailedEntries &&
		a.CacheReadKnownEntries == b.CacheReadKnownEntries &&
		a.CacheWriteKnownEntries == b.CacheWriteKnownEntries &&
		a.CacheReadTokens == b.CacheReadTokens &&
		a.CacheWriteTokens == b.CacheWriteTokens &&
		a.CacheCreationTokens == b.CacheCreationTokens &&
		a.CacheableInputTokens == b.CacheableInputTokens &&
		a.NonCacheableInputTokens == b.NonCacheableInputTokens &&
		a.CacheReadInputRatio == b.CacheReadInputRatio &&
		a.CacheableInputRatio == b.CacheableInputRatio &&
		a.NonCacheableInputRatio == b.NonCacheableInputRatio &&
		a.CacheEstimatedEntries == b.CacheEstimatedEntries &&
		a.CacheProviderEntries == b.CacheProviderEntries
}

func llmUsageProviderModelBreakdownsEqual(a, b []projectioncontract.LLMUsageProviderModelBreakdown) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if !llmUsageProviderModelBreakdownEqual(a[index], b[index]) {
			return false
		}
	}
	return true
}

func llmUsageProviderModelBreakdownEqual(a, b projectioncontract.LLMUsageProviderModelBreakdown) bool {
	return strings.TrimSpace(a.ProviderID) == strings.TrimSpace(b.ProviderID) &&
		strings.TrimSpace(a.Provider) == strings.TrimSpace(b.Provider) &&
		strings.TrimSpace(a.APISurface) == strings.TrimSpace(b.APISurface) &&
		strings.TrimSpace(a.Model) == strings.TrimSpace(b.Model) &&
		strings.TrimSpace(a.ProviderMode) == strings.TrimSpace(b.ProviderMode) &&
		a.EntryTotal == b.EntryTotal &&
		a.InputTokens == b.InputTokens &&
		a.OutputTokens == b.OutputTokens &&
		a.TotalTokens == b.TotalTokens &&
		a.CacheReadKnownEntries == b.CacheReadKnownEntries &&
		a.CacheWriteKnownEntries == b.CacheWriteKnownEntries &&
		a.CacheReadTokens == b.CacheReadTokens &&
		a.CacheWriteTokens == b.CacheWriteTokens &&
		a.CacheCreationTokens == b.CacheCreationTokens &&
		a.CacheablePromptCount == b.CacheablePromptCount &&
		a.ActualCacheHitCount == b.ActualCacheHitCount &&
		a.CacheCreationCount == b.CacheCreationCount &&
		a.PromptCacheKeySentCount == b.PromptCacheKeySentCount &&
		a.CacheControlSentCount == b.CacheControlSentCount &&
		a.AutoPrefixExpectedCount == b.AutoPrefixExpectedCount &&
		a.CacheReadInputRatio == b.CacheReadInputRatio &&
		a.EstimatedEntries == b.EstimatedEntries &&
		a.FailedEntries == b.FailedEntries
}

func autoGovernanceEqual(a, b *projectioncontract.AutoGovernanceSummary) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.TraceID) == strings.TrimSpace(b.TraceID) &&
		strings.TrimSpace(a.Action) == strings.TrimSpace(b.Action) &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.RiskLevel) == strings.TrimSpace(b.RiskLevel) &&
		strings.TrimSpace(a.PolicyMatched) == strings.TrimSpace(b.PolicyMatched) &&
		strings.TrimSpace(a.PolicyBlocked) == strings.TrimSpace(b.PolicyBlocked) &&
		a.Confidence == b.Confidence &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		strings.TrimSpace(a.PromptFingerprint) == strings.TrimSpace(b.PromptFingerprint) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt)
}

func doDAssessmentEqual(a, b *workorchresultcontract.DoDAssessment) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Satisfied != b.Satisfied || strings.TrimSpace(a.Summary) != strings.TrimSpace(b.Summary) {
		return false
	}
	if !doDAssessmentItemsEqual(a.Items, b.Items) {
		return false
	}
	return doDCoverageEqual(a.GoalCoverage, b.GoalCoverage) &&
		doDCoverageEqual(a.AcceptanceDimensions, b.AcceptanceDimensions) &&
		doDCoverageEqual(a.DeliverableCoverage, b.DeliverableCoverage)
}

func doDAssessmentItemsEqual(a, b []workorchresultcontract.DoDAssessmentItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].Item) != strings.TrimSpace(b[i].Item) ||
			strings.TrimSpace(a[i].State) != strings.TrimSpace(b[i].State) ||
			!stringSliceEqual(a[i].EvidenceRefs, b[i].EvidenceRefs) ||
			strings.TrimSpace(a[i].FailureReason) != strings.TrimSpace(b[i].FailureReason) ||
			a[i].Confidence != b[i].Confidence ||
			strings.TrimSpace(a[i].SourceNote) != strings.TrimSpace(b[i].SourceNote) {
			return false
		}
	}
	return true
}

func doDCoverageEqual(a, b *workorchresultcontract.DoDCoverage) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Satisfied != b.Satisfied || strings.TrimSpace(a.Summary) != strings.TrimSpace(b.Summary) {
		return false
	}
	if len(a.Items) != len(b.Items) {
		return false
	}
	for i := range a.Items {
		if strings.TrimSpace(a.Items[i].ID) != strings.TrimSpace(b.Items[i].ID) ||
			strings.TrimSpace(a.Items[i].Deliverable) != strings.TrimSpace(b.Items[i].Deliverable) ||
			strings.TrimSpace(a.Items[i].Name) != strings.TrimSpace(b.Items[i].Name) ||
			strings.TrimSpace(a.Items[i].Path) != strings.TrimSpace(b.Items[i].Path) ||
			strings.TrimSpace(a.Items[i].Goal) != strings.TrimSpace(b.Items[i].Goal) ||
			strings.TrimSpace(a.Items[i].GoalItem) != strings.TrimSpace(b.Items[i].GoalItem) ||
			strings.TrimSpace(a.Items[i].Item) != strings.TrimSpace(b.Items[i].Item) ||
			strings.TrimSpace(a.Items[i].Requirement) != strings.TrimSpace(b.Items[i].Requirement) ||
			strings.TrimSpace(a.Items[i].State) != strings.TrimSpace(b.Items[i].State) ||
			!stringSliceEqual(a.Items[i].EvidenceRefs, b.Items[i].EvidenceRefs) ||
			!artifactIDSliceEqual(a.Items[i].ArtifactRefs, b.Items[i].ArtifactRefs) {
			return false
		}
	}
	return true
}

func resultSummaryEqual(a, b *workorchresultcontract.ResultSummary) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.GoalOutcome) == strings.TrimSpace(b.GoalOutcome) &&
		doDAssessmentItemsEqual(a.DoDItems, b.DoDItems) &&
		strings.TrimSpace(a.ImplementationSummary) == strings.TrimSpace(b.ImplementationSummary) &&
		strings.TrimSpace(a.VerificationSummary) == strings.TrimSpace(b.VerificationSummary) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		artifactIDSliceEqual(a.ArtifactRefs, b.ArtifactRefs) &&
		stringSliceEqual(a.KnownLimitations, b.KnownLimitations) &&
		stringSliceEqual(a.ResidualRisks, b.ResidualRisks) &&
		stringSliceEqual(a.NextActions, b.NextActions) &&
		stableMapFactsEqual(a.Diagnostics.Map(), b.Diagnostics.Map())
}

func artifactIDSliceEqual(a, b []common.ArtifactID) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func memorySelectionScoresEqual(a, b []workorchmemorycontract.RecallSelectionScore) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(string(a[i].ID)) != strings.TrimSpace(string(b[i].ID)) ||
			a[i].Score != b[i].Score ||
			strings.TrimSpace(a[i].Reason) != strings.TrimSpace(b[i].Reason) {
			return false
		}
	}
	return true
}

func memoryHitsEqual(a, b []workorchmemorycontract.RecallHit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(string(a[i].ID)) != strings.TrimSpace(string(b[i].ID)) ||
			strings.TrimSpace(a[i].Summary) != strings.TrimSpace(b[i].Summary) ||
			a[i].VectorScore != b[i].VectorScore ||
			a[i].VectorDimension != b[i].VectorDimension {
			return false
		}
	}
	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func intMapFactsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, ok := b[key]
		if !ok || left != right {
			return false
		}
	}
	return true
}

// stableMapFactsEqual projects map[string]any into sorted key/value comparison
// facts without JSON/reflect authority. Unsupported owner payload values are
// deliberately treated as unequal: a conservative changed section is safe,
// whereas a generic string fallback could silently hide a real change.
func stableMapFactsEqual(a, b map[string]any) bool {
	left, leftSupported := stableMapFactPairsWithSupport(a)
	right, rightSupported := stableMapFactPairsWithSupport(b)
	if !leftSupported || !rightSupported {
		return false
	}
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stableMapSliceFactsEqual(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !stableMapFactsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func stableMapFactPairs(input map[string]any) []string {
	pairs, _ := stableMapFactPairsWithSupport(input)
	return pairs
}

func stableMapFactPairsWithSupport(input map[string]any) ([]string, bool) {
	if len(input) == 0 {
		return nil, true
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		// Length-prefix keys/values so embedded '=' or separators cannot collide (FR06).
		value, supported := stableAnyFactWithSupport(input[key])
		if !supported {
			return nil, false
		}
		pairs = append(pairs, lengthPrefixed(key)+"="+value)
	}
	return pairs, true
}

// lengthPrefixed encodes s as "<len>:<s>" so delimiter-bearing content cannot
// collide with multi-element joins (FR06).
func lengthPrefixed(s string) string {
	return strconv.Itoa(len(s)) + ":" + s
}

func stableAnyFact(value any) string {
	fact, _ := stableAnyFactWithSupport(value)
	return fact
}

// stableAnyFactWithSupport accepts only the JSON-shaped owner payload values
// that the projection explicitly understands. It never falls back to
// fmt.Sprint: a fallback can collapse different Go values into the same fact.
func stableAnyFactWithSupport(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "null", true
	case string:
		return "s:" + lengthPrefixed(typed), true
	case bool:
		if typed {
			return "b:true", true
		}
		return "b:false", true
	case int:
		return "n:" + strconv.FormatInt(int64(typed), 10), true
	case int64:
		return "n:" + strconv.FormatInt(typed, 10), true
	case int32:
		return "n:" + strconv.FormatInt(int64(typed), 10), true
	case float64:
		return "n:" + strconv.FormatFloat(typed, 'g', -1, 64), true
	case float32:
		return "n:" + strconv.FormatFloat(float64(typed), 'g', -1, 64), true
	case []string:
		// Length-prefix each element and the count so ["a,b"] != ["a","b"].
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, lengthPrefixed(item))
		}
		return "as:" + strconv.Itoa(len(typed)) + ":[" + strings.Join(parts, ",") + "]", true
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			fact, supported := stableAnyFactWithSupport(item)
			if !supported {
				return "", false
			}
			parts = append(parts, lengthPrefixed(fact))
		}
		return "a:" + strconv.Itoa(len(typed)) + ":[" + strings.Join(parts, ",") + "]", true
	case map[string]any:
		pairs, supported := stableMapFactPairsWithSupport(typed)
		if !supported {
			return "", false
		}
		for idx := range pairs {
			pairs[idx] = lengthPrefixed(pairs[idx])
		}
		return "o:" + strconv.Itoa(len(pairs)) + ":{" + strings.Join(pairs, ",") + "}", true
	default:
		return "", false
	}
}
