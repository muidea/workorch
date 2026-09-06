package workorchprojectioncontract

// RunHeartbeat is the owner-neutral, read-only projection of recent effective
// Run activity. It cannot schedule work or transition the Run lifecycle.
type RunHeartbeat struct {
	WorkspaceID               string `json:"workspaceId,omitempty"`
	RunID                     string `json:"runId,omitempty"`
	State                     string `json:"state,omitempty"`
	ActivityState             string `json:"activityState,omitempty"`
	LatestStepID              string `json:"latestStepId,omitempty"`
	LatestStepKind            string `json:"latestStepKind,omitempty"`
	LatestStepState           string `json:"latestStepState,omitempty"`
	LatestEvidenceRef         string `json:"latestEvidenceRef,omitempty"`
	LatestEventTopic          string `json:"latestEventTopic,omitempty"`
	LatestEventRevision       int64  `json:"latestEventRevision,omitempty"`
	LatestEventAt             string `json:"latestEventAt,omitempty"`
	LatestLLMUsageAt          string `json:"latestLLMUsageAt,omitempty"`
	LatestResultAt            string `json:"latestResultAt,omitempty"`
	LatestArtifactAt          string `json:"latestArtifactAt,omitempty"`
	LatestPatchAt             string `json:"latestPatchAt,omitempty"`
	LatestActivityAt          string `json:"latestActivityAt,omitempty"`
	LatestEffectiveProgressAt string `json:"latestEffectiveProgressAt,omitempty"`
	IneffectiveRetry          bool   `json:"ineffectiveRetry,omitempty"`
	Stalled                   bool   `json:"stalled,omitempty"`
	Summary                   string `json:"summary,omitempty"`
}

func CloneRunCommandStatus(item *RunCommandStatus) *RunCommandStatus {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}
