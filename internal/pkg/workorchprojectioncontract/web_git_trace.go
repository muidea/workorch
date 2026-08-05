package workorchprojectioncontract

type WebSourceTrace struct {
	Query          string             `json:"query,omitempty"`
	ProviderID     string             `json:"providerId,omitempty"`
	SearchedAt     string             `json:"searchedAt,omitempty"`
	Sources        []WebSource        `json:"sources,omitempty"`
	FetchedSources []FetchedWebSource `json:"fetchedSources,omitempty"`
	EvidenceRefs   []string           `json:"evidenceRefs,omitempty"`
	ApprovalID     string             `json:"approvalId,omitempty"`
	PolicyDecision string             `json:"policyDecision,omitempty"`
}

type WebSource struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet,omitempty"`
	Source      string `json:"source,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
	Rank        int    `json:"rank,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

type FetchedWebSource struct {
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl,omitempty"`
	Title       string `json:"title,omitempty"`
	Source      string `json:"source,omitempty"`
	RetrievedAt string `json:"retrievedAt,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

func CloneWebSourceTraces(items []WebSourceTrace) []WebSourceTrace {
	if items == nil {
		return nil
	}
	ret := make([]WebSourceTrace, 0, len(items))
	for _, item := range items {
		item.Sources = append([]WebSource(nil), item.Sources...)
		item.FetchedSources = append([]FetchedWebSource(nil), item.FetchedSources...)
		item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
		ret = append(ret, item)
	}
	return ret
}

type GitOperationTrace struct {
	StepID           string           `json:"stepId"`
	Kind             string           `json:"kind"`
	Operation        string           `json:"operation,omitempty"`
	Status           string           `json:"status,omitempty"`
	RepoRoot         string           `json:"repoRoot,omitempty"`
	WorkingDir       string           `json:"workingDir,omitempty"`
	Branch           string           `json:"branch,omitempty"`
	HeadCommit       string           `json:"headCommit,omitempty"`
	HeadCommitBefore string           `json:"headCommitBefore,omitempty"`
	HeadCommitAfter  string           `json:"headCommitAfter,omitempty"`
	CommitHash       string           `json:"commitHash,omitempty"`
	CommitSubject    string           `json:"commitSubject,omitempty"`
	ChangedFiles     []GitChangedFile `json:"changedFiles,omitempty"`
	StagedFiles      []GitChangedFile `json:"stagedFiles,omitempty"`
	Truncated        bool             `json:"truncated,omitempty"`
	ZeroWork         bool             `json:"zeroWork,omitempty"`
	ErrorCode        string           `json:"errorCode,omitempty"`
	Summary          string           `json:"summary,omitempty"`
	EvidenceRefs     []string         `json:"evidenceRefs,omitempty"`
}

type GitChangedFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath,omitempty"`
	Status    string `json:"status,omitempty"`
	Staged    bool   `json:"staged,omitempty"`
	Unstaged  bool   `json:"unstaged,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
}
