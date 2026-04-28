package repository

import (
	"context"

	eventstream "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type EventRecordQuery struct {
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Topic          string
	SubjectType    string
	SubjectID      string
	CursorAfterID  common.EventRecordID
	OccurredAfter  string
	OccurredBefore string
	Limit          int
}

type EventRecordRepository interface {
	Append(context.Context, *eventstream.EventRecord) error
	Get(context.Context, common.EventRecordID) (*eventstream.EventRecord, error)
	List(context.Context, EventRecordQuery) ([]*eventstream.EventRecord, error)
}
