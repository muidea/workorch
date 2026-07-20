package shell

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeCommandRequest struct {
	WorkspaceID                     common.WorkspaceID
	RunID                           common.RunID
	StepID                          common.StepID
	WorkspaceDeliverableDirectory   string
	WorkspaceInternalDirectory      string
	Command                         string
	Args                            []string
	TimeoutMillis                   int64
	StateChanging                   bool
	TargetRef                       string
	TargetRefs                      []string
	RelatedFailureTargets           []string
	VerificationScope               string
	BoundedReason                   string
	ValidationScratchDirectory      string
	ValidationScratchAutoCleanup    bool
	ValidationScratchAutoCleanupSet bool
	ValidationEvidence              runtimecontract.RuntimeValidationEvidence
	ManagedValidation               runtimecontract.RuntimeManagedValidation
	EvidencePolicies                []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeCommandResult struct {
	Status                          string
	Stage                           string
	Command                         string
	Args                            []string
	ActualCommand                   string
	WorkingDirectory                string
	Stdout                          string
	Stderr                          string
	ExitCode                        int64
	DurationMillis                  int64
	TimeoutMillis                   int64
	OutputTruncated                 bool
	StdoutTruncated                 bool
	StderrTruncated                 bool
	StdoutBytes                     int64
	StderrBytes                     int64
	FailureCode                     string
	FailureMessage                  string
	Summary                         string
	Reason                          string
	FailureReason                   string
	UnavailableCommand              string
	CapabilityMissing               bool
	ZeroWork                        bool
	EvidenceQuality                 string
	EvidenceReason                  string
	EvidenceTags                    []string
	TargetRefs                      []string
	CommandNormalization            string
	MaxOutputBytes                  int64
	ValidationEvidence              runtimecontract.RuntimeValidationEvidence
	ManagedValidation               runtimecontract.RuntimeManagedValidation
	ValidationResources             []runtimecontract.RuntimeValidationResource
	RuntimeDiagnostics              runtimecontract.RuntimeDiagnostics
	ValidationFailureDiagnostic     *runtimecontract.ValidationFailureDiagnostic
	WorkspaceStateChangeFingerprint runtimecontract.RuntimeWorkspaceStateChangeFingerprint
	// WorkspaceTargetChanges is the bounded before/after observation for the
	// explicitly sealed workspace targets of a state-changing Shell operation.
	// It remains owner-local until the Shell owner converts it to the typed
	// receipt ResourceChanges contract.
	WorkspaceTargetChanges []RuntimeWorkspaceTargetChange
	Evidence               runtimecontract.RuntimeOperationEvidence
}

// RuntimeWorkspaceTargetChange is a content-derived observation of one
// workspace-relative declared target. "missing" is a valid revision: a
// state-changing Shell command may create a previously absent target.
type RuntimeWorkspaceTargetChange struct {
	TargetRef      string
	BeforeRevision string
	AfterRevision  string
}
