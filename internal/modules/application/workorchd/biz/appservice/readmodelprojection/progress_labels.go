package readmodelprojection

type WorkspaceTaskProgressLabels struct {
	NoActiveRun      string
	ActiveRun        string
	WorkspaceDefined string
	FormalResult     string
	Runs             string
	Results          string
	Artifacts        string
}

type RunTaskProgressLabels struct {
	RecoveryNextAction string
	RunCreated         string
	ExecutionSteps     string
	Governance         string
	Validation         string
	FormalResult       string
}
