package readmodelcore

type MemoryRecallTraceQuery struct {
	WorkspaceID string
	RunID       string
	StepID      string
	Stage       string
	Source      string
	Status      string
	Limit       int
}

type EventRecordQuery struct {
	WorkspaceID    string
	RunID          string
	Topic          string
	SubjectType    string
	SubjectID      string
	CursorAfterID  string
	OccurredAfter  string
	OccurredBefore string
	Limit          int
	IncludePayload bool
}
