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
	ValidationRecipeID              string
	Command                         string
	Args                            []string
	TimeoutMillis                   int64
	StateChanging                   bool
	TargetRef                       string
	TargetRefs                      []string
	ValidationScopeTargetRefs       []string
	VerificationScope               string
	BoundedReason                   string
	ValidationScratchDirectory      string
	ValidationScratchAutoCleanup    bool
	ValidationScratchAutoCleanupSet bool
	ValidationEvidence              runtimecontract.RuntimeValidationEvidence
	ManagedValidation               runtimecontract.RuntimeManagedValidation
	EvidencePolicies                []runtimecontract.RuntimeEvidencePolicy
	// ValidationRecipes is the Shell-owner-confirmed admissibility recipe set
	// carried with a sealed managed-validation dispatch.
	ValidationRecipes           []runtimecontract.RuntimeValidationRecipe
	ValidationCheckAttestations []runtimecontract.RuntimeValidationCheckAttestationBinding
}

type RuntimeCommandResult struct {
	Status                      string
	Stage                       string
	Command                     string
	ValidationRecipeID          string
	Args                        []string
	ActualCommand               string
	WorkingDirectory            string
	Stdout                      string
	Stderr                      string
	ExitCode                    int64
	DurationMillis              int64
	TimeoutMillis               int64
	OutputTruncated             bool
	StdoutTruncated             bool
	StderrTruncated             bool
	StdoutBytes                 int64
	StderrBytes                 int64
	FailureCode                 string
	FailureMessage              string
	Summary                     string
	Reason                      string
	FailureReason               string
	UnavailableCommand          string
	CapabilityMissing           bool
	ZeroWork                    bool
	EvidenceQuality             string
	EvidenceReason              string
	EvidenceTags                []string
	TargetRefs                  []string
	CommandNormalization        string
	MaxOutputBytes              int64
	ValidationEvidence          runtimecontract.RuntimeValidationEvidence
	ManagedValidation           runtimecontract.RuntimeManagedValidation
	ValidationResources         []runtimecontract.RuntimeValidationResource
	RuntimeDiagnostics          runtimecontract.RuntimeDiagnostics
	ValidationFailureDiagnostic *runtimecontract.ValidationFailureDiagnostic
	// goTestJSONFacts is the single owner-private decode of a bounded
	// `go test -json` transcript. Observation, classification and target
	// extraction share this immutable parse so they cannot disagree about the
	// same tool output. It never crosses the Shell owner boundary.
	goTestJSONFacts *shellGoTestJSONFacts
	// v2 behavior-observation candidates are independent of assertion locations.
	// They never authorize repair without the existing File-owner confirmation.
	workspaceBehaviorTargetRefs        []string
	workspaceProcessBehaviorTargetRefs []string
	workspaceProcessBehaviorError      error
	// validationFailureTargetCandidatesInvalid records that the owner-private
	// structured-output parser observed more candidates than the bounded
	// validation-failure protocol permits. An empty TargetRefs slice alone cannot
	// distinguish that fail-closed case from output which identified no target.
	validationFailureTargetCandidatesInvalid bool
	WorkspaceStateChangeFingerprint          runtimecontract.RuntimeWorkspaceStateChangeFingerprint
	ValidationObservation                    *runtimecontract.RuntimeValidationObservation
	// WorkspaceTargetChanges is the bounded before/after observation for the
	// explicitly sealed workspace targets of a state-changing Shell operation.
	// It remains owner-local until the Shell owner converts it to the typed
	// receipt ResourceChanges contract.
	WorkspaceTargetChanges []RuntimeWorkspaceTargetChange
	// WorkspaceObservedTargetChanges is the bounded authority-relevant workspace
	// view for a state-changing command, including Git control files and symlink
	// identity while excluding generated directories omitted by the File owner.
	// Unlike WorkspaceTargetChanges (kept for declared-target diagnostics), it
	// lets the Shell owner reject observed changes outside the sealed target set.
	WorkspaceObservedTargetChanges []RuntimeWorkspaceTargetChange
	Evidence                       runtimecontract.RuntimeOperationEvidence
}

// RuntimeWorkspaceTargetChange is a content-derived observation of one
// workspace-relative declared target. "missing" is a valid revision: a
// state-changing Shell command may create a previously absent target.
type RuntimeWorkspaceTargetChange struct {
	TargetRef      string
	BeforeRevision string
	AfterRevision  string
}
