package events

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const TopicObservePreparationDirectories = "/file/startup-preparation/directories/query"
const PreparationDirectoriesVersion = "workorch-file-preparation-directories/v1"

// Workspace alone resolves these paths from an accepted exact seal. No public
// caller can supply paths or a PASS; File reports observations, not readiness.
type ObservePreparationDirectoriesCommand struct {
	WorkspaceID                             common.WorkspaceID
	RequestID, PayloadHash                  string
	WorkspaceRevision                       uint64
	InternalDirectory, DeliverableDirectory string
}

func (c ObservePreparationDirectoriesCommand) Validate() error {
	if c.WorkspaceID == "" || c.WorkspaceRevision == 0 || !fileobservation.ValidPath(c.InternalDirectory) || !fileobservation.ValidPath(c.DeliverableDirectory) {
		return common.ErrInvalidArgument
	}
	for _, value := range []string{string(c.WorkspaceID), c.RequestID, c.PayloadHash} {
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return common.ErrInvalidArgument
		}
	}
	return nil
}

type ObservePreparationDirectoriesResult struct {
	Version                string
	Command                ObservePreparationDirectoriesCommand
	StartedAt, CompletedAt time.Time
	Internal, Deliverable  fileobservation.Directory
}

func (r ObservePreparationDirectoriesResult) ValidateFor(c ObservePreparationDirectoriesCommand) error {
	if c.Validate() != nil || r.Version != PreparationDirectoriesVersion || r.Command != c || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || r.Internal.Path != c.InternalDirectory || r.Deliverable.Path != c.DeliverableDirectory || r.Internal.Validate() != nil || r.Deliverable.Validate() != nil {
		return common.ErrConflict
	}
	return nil
}
