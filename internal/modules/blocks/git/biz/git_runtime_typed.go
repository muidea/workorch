package git

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	StepID                        common.StepID
	Kind                          string
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Operation                     string
	TimeoutMillis                 int64
	Pathspecs                     []string
	Ref                           string
	BaseRef                       string
	Limit                         int64
	Message                       string
	ExpectedHead                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeOperationResult struct {
	Status             string
	Stage              string
	Operation          string
	RepoRoot           string
	WorkingDirectory   string
	Branch             string
	HeadCommit         string
	HeadCommitBefore   string
	HeadCommitAfter    string
	BeforeHash         string
	AfterHash          string
	DetachedHead       bool
	UnbornHead         bool
	Stdout             string
	Stderr             string
	ExitCode           int64
	DurationMillis     int64
	TimeoutMillis      int64
	Truncated          bool
	OutputBytes        int64
	ErrorOutputBytes   int64
	ErrorCode          string
	ErrorMessage       string
	Summary            string
	Message            string
	ZeroWork           bool
	Initialized        bool
	ExistingRepository bool
	Pathspecs          []string
	Ref                string
	Limit              int64
	LimitClamped       bool
	CommitHash         string
	CommitSubject      string
	ChangedFiles       []GitFileChange
	StagedFiles        []GitFileChange
	CommittedFiles     []GitFileChange
	Commits            []GitCommit
	Branches           []GitBranch
	AheadBehind        GitAheadBehind
	Evidence           runtimecontract.RuntimeOperationEvidence
}

type GitFileChange struct {
	Path      string
	OldPath   string
	Status    string
	Staged    bool
	Unstaged  bool
	Untracked bool
}

type GitAheadBehind struct {
	Ahead  int64
	Behind int64
}

type GitCommit struct {
	Hash       string
	ShortHash  string
	Author     string
	AuthorTime string
	Subject    string
}

type GitBranch struct {
	Name     string
	Commit   string
	Current  bool
	Upstream string
}
