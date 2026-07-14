package workorchruntimecontract

// RuntimeValidationEvidence carries validation metadata across the typed
// runtime command contract. It is intentionally a concrete structure; JSON
// projection is owned by the external boundary rather than the block.
type RuntimeValidationEvidence struct {
	Mode                          string
	ObservationType               string
	Observation                   string
	DiagnosticClass               string
	FailureClass                  string
	ExpectedFailureClass          string
	RecommendedEvidenceTargetType string
	TargetRefs                    []string
	CoverageTargets               []string
	ObservedClaims                []string
}

type RuntimeManagedValidation struct {
	RuntimeManaged    bool
	AutoCleanup       bool
	AutoCleanupSet    bool
	Kind              string
	OriginalNodeKind  string
	CleanupPolicy     string
	ResourceCount     int
	ResourcesReleased bool
}

type RuntimeValidationResource struct {
	ID            string
	WorkspaceID   string
	RunID         string
	StepID        string
	Kind          string
	PID           int
	Path          string
	Status        string
	CleanupPolicy string
	CleanupError  string
	CreatedAt     string
	ReleasedAt    string
}

type RuntimeValidationResourceLifecycle struct {
	ResourceCount int
	Released      bool
}

type RuntimeDiagnostics struct {
	Kind                        string
	Operation                   string
	TargetRef                   string
	FailureReason               string
	UnavailableCommand          string
	CommandPolicyViolation      bool
	ValidationResourceLifecycle RuntimeValidationResourceLifecycle
	ValidationFailureDiagnostic *ValidationFailureDiagnostic
}

type RuntimeWorkspaceStateChangeFingerprint struct {
	Before string
	After  string
}
