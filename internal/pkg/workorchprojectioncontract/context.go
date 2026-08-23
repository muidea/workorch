package workorchprojectioncontract

import "maps"

type ContextBundle struct {
	WorkspaceID           string                        `json:"workspaceId"`
	RunID                 string                        `json:"runId,omitempty"`
	Mode                  string                        `json:"mode"`
	Summary               string                        `json:"summary,omitempty"`
	CompressedContext     CompressedContext             `json:"compressedContext"`
	TaskProgress          TaskProgress                  `json:"taskProgress"`
	Recovery              RecoveryPlan                  `json:"recovery"`
	LatestCheckpoint      *RuntimeCheckpoint            `json:"latestCheckpoint,omitempty"`
	Sections              []ContextSection              `json:"sections,omitempty"`
	RecentEvidence        []ContextEvidence             `json:"recentEvidence,omitempty"`
	CompressionCandidates []ContextCompressionCandidate `json:"compressionCandidates,omitempty"`
}

type ContextSection struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	Priority     int            `json:"priority"`
	State        string         `json:"state,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	EvidenceRefs []string       `json:"evidenceRefs,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

type ContextEvidence struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	State        string         `json:"state,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	OccurredAt   string         `json:"occurredAt,omitempty"`
	EvidenceRefs []string       `json:"evidenceRefs,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

type ContextCompressionCandidate struct {
	SectionID     string   `json:"sectionId"`
	Reason        string   `json:"reason"`
	Retention     string   `json:"retention"`
	SourceRefs    []string `json:"sourceRefs,omitempty"`
	SuggestedMode string   `json:"suggestedMode,omitempty"`
}

type CompressedContext struct {
	Version            string              `json:"version"`
	Mode               string              `json:"mode"`
	WorkspaceID        string              `json:"workspaceId"`
	RunID              string              `json:"runId,omitempty"`
	Summary            string              `json:"summary,omitempty"`
	CurrentMilestone   string              `json:"currentMilestone,omitempty"`
	Milestones         []ProgressMilestone `json:"milestones,omitempty"`
	Blockers           []ProgressBlocker   `json:"blockers,omitempty"`
	Recovery           RecoveryPlan        `json:"recovery"`
	RecentEvidence     []ContextEvidence   `json:"recentEvidence,omitempty"`
	ResultRefs         []string            `json:"resultRefs,omitempty"`
	SourceSectionRefs  []string            `json:"sourceSectionRefs,omitempty"`
	SourceEvidenceRefs []string            `json:"sourceEvidenceRefs,omitempty"`
	RetentionPolicy    string              `json:"retentionPolicy,omitempty"`
}

func CloneContextBundle(input *ContextBundle) *ContextBundle {
	if input == nil {
		return nil
	}
	ret := *input
	ret.Recovery = CloneRecoveryPlan(input.Recovery)
	ret.TaskProgress = CloneTaskProgress(input.TaskProgress)
	ret.Sections = append([]ContextSection(nil), input.Sections...)
	for index := range ret.Sections {
		ret.Sections[index].EvidenceRefs = append([]string(nil), input.Sections[index].EvidenceRefs...)
		ret.Sections[index].Data = maps.Clone(input.Sections[index].Data)
	}
	ret.RecentEvidence = append([]ContextEvidence(nil), input.RecentEvidence...)
	cloneContextEvidence(ret.RecentEvidence, input.RecentEvidence)
	ret.CompressionCandidates = append([]ContextCompressionCandidate(nil), input.CompressionCandidates...)
	for index := range ret.CompressionCandidates {
		ret.CompressionCandidates[index].SourceRefs = append([]string(nil), input.CompressionCandidates[index].SourceRefs...)
	}
	ret.CompressedContext.Milestones = append([]ProgressMilestone(nil), input.CompressedContext.Milestones...)
	ret.CompressedContext.Blockers = append([]ProgressBlocker(nil), input.CompressedContext.Blockers...)
	ret.CompressedContext.Recovery = CloneRecoveryPlan(input.CompressedContext.Recovery)
	ret.CompressedContext.RecentEvidence = append([]ContextEvidence(nil), input.CompressedContext.RecentEvidence...)
	cloneContextEvidence(ret.CompressedContext.RecentEvidence, input.CompressedContext.RecentEvidence)
	ret.CompressedContext.ResultRefs = append([]string(nil), input.CompressedContext.ResultRefs...)
	ret.CompressedContext.SourceSectionRefs = append([]string(nil), input.CompressedContext.SourceSectionRefs...)
	ret.CompressedContext.SourceEvidenceRefs = append([]string(nil), input.CompressedContext.SourceEvidenceRefs...)
	return &ret
}

func cloneContextEvidence(output []ContextEvidence, input []ContextEvidence) {
	for index := range output {
		output[index].EvidenceRefs = append([]string(nil), input[index].EvidenceRefs...)
		output[index].Data = maps.Clone(input[index].Data)
	}
}
