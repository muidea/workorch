package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type ContextBundle struct {
	WorkspaceID           string                                `json:"workspaceId"`
	RunID                 string                                `json:"runId,omitempty"`
	Mode                  string                                `json:"mode"`
	Summary               string                                `json:"summary,omitempty"`
	CompressedContext     CompressedContext                     `json:"compressedContext"`
	TaskProgress          projectioncontract.TaskProgress       `json:"taskProgress"`
	Recovery              projectioncontract.RecoveryPlan       `json:"recovery"`
	LatestCheckpoint      *projectioncontract.RuntimeCheckpoint `json:"latestCheckpoint,omitempty"`
	Sections              []ContextSection                      `json:"sections,omitempty"`
	RecentEvidence        []ContextEvidence                     `json:"recentEvidence,omitempty"`
	CompressionCandidates []ContextCompressionCandidate         `json:"compressionCandidates,omitempty"`
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
	Version            string                                 `json:"version"`
	Mode               string                                 `json:"mode"`
	WorkspaceID        string                                 `json:"workspaceId"`
	RunID              string                                 `json:"runId,omitempty"`
	Summary            string                                 `json:"summary,omitempty"`
	CurrentMilestone   string                                 `json:"currentMilestone,omitempty"`
	Milestones         []projectioncontract.ProgressMilestone `json:"milestones,omitempty"`
	Blockers           []projectioncontract.ProgressBlocker   `json:"blockers,omitempty"`
	Recovery           projectioncontract.RecoveryPlan        `json:"recovery"`
	RecentEvidence     []ContextEvidence                      `json:"recentEvidence,omitempty"`
	ResultRefs         []string                               `json:"resultRefs,omitempty"`
	SourceSectionRefs  []string                               `json:"sourceSectionRefs,omitempty"`
	SourceEvidenceRefs []string                               `json:"sourceEvidenceRefs,omitempty"`
	RetentionPolicy    string                                 `json:"retentionPolicy,omitempty"`
}
