package events

import (
	"strings"
	"time"
	"unicode/utf8"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicObserveWorkspacePreparationHistory = "/run/startup-preparation/history/query"

// This read is separate from ordinary visibility-filtered Run lists. RequestID
// identifies only this observation, never a fresh execution request or permit.
type ObserveWorkspacePreparationHistoryCommand struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
}

func (c ObserveWorkspacePreparationHistoryCommand) Validate() error {
	if runmodel.ValidatePreparationHistoryWorkspaceID(c.WorkspaceID) != nil {
		return common.ErrInvalidArgument
	}
	for _, value := range []string{string(c.WorkspaceID), c.RequestID} {
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
			return common.ErrInvalidArgument
		}
	}
	return nil
}

type ObserveWorkspacePreparationHistoryResult struct {
	RequestID  string
	ObservedAt time.Time
	History    runmodel.WorkspacePreparationHistory
}

func (r ObserveWorkspacePreparationHistoryResult) Clone() ObserveWorkspacePreparationHistoryResult {
	r.History = r.History.Clone()
	return r
}

func (r ObserveWorkspacePreparationHistoryResult) ValidateFor(c ObserveWorkspacePreparationHistoryCommand) error {
	if c.Validate() != nil || r.RequestID != c.RequestID || r.ObservedAt.IsZero() || r.History.WorkspaceID != c.WorkspaceID || r.History.Validate() != nil {
		return common.ErrConflict
	}
	return nil
}
