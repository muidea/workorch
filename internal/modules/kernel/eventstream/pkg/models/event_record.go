package eventstream

import "github.com/muidea/workorch/internal/pkg/common"

type EventRecord struct {
	ID             common.EventRecordID
	Version        string
	Topic          string
	OccurredAt     string
	SourceModule   string
	SubjectType    string
	SubjectID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Revision       int64
	Payload        map[string]any
	PayloadSummary map[string]any
	PayloadRef     string
}
