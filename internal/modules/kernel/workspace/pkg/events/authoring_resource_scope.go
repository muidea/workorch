package events

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// QueryAuthoringResourceScopeCommand lets the File owner resolve a resource
// reference against the current Workspace-owned draft and directory. The
// response is a read fact, not a sealed resource snapshot or admission grant.
type QueryAuthoringResourceScopeCommand struct {
	WorkspaceID common.WorkspaceID
	InputHash   string
	ResourceRef string
}

func (c QueryAuthoringResourceScopeCommand) Validate() error {
	if c.WorkspaceID == "" || len(c.WorkspaceID) > 256 || c.InputHash == "" || len(c.InputHash) > 256 || c.ResourceRef == "" || len(c.ResourceRef) > 1024 || !utf8.ValidString(c.ResourceRef) || strings.ContainsRune(c.ResourceRef, 0) {
		return fmt.Errorf("authoring resource scope requires bounded workspace, input and ref")
	}
	return nil
}

type QueryAuthoringResourceScopeResult struct {
	WorkspaceID          common.WorkspaceID
	WorkspaceRevision    uint64
	InputHash            string
	ResourceRef          string
	DeliverableDirectory string
}

func (r QueryAuthoringResourceScopeResult) ValidateFor(c QueryAuthoringResourceScopeCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.InputHash != c.InputHash || r.ResourceRef != c.ResourceRef || r.DeliverableDirectory == "" {
		return fmt.Errorf("authoring resource scope response is inconsistent")
	}
	return nil
}

func AuthoringResourceScopeFor(view QueryDefinitionAuthoringResult, command QueryAuthoringResourceScopeCommand) (QueryAuthoringResourceScopeResult, error) {
	if command.Validate() != nil || view.ValidateFor(QueryDefinitionAuthoringCommand{WorkspaceID: command.WorkspaceID}) != nil || view.Record == nil || view.Record.InputHash != command.InputHash || !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != view.Record.TemplateSnapshotHash || view.DeliverableDirectory == "" || !slices.Contains(view.Record.Input.ResourceRefs, command.ResourceRef) {
		return QueryAuthoringResourceScopeResult{}, common.ErrConflict
	}
	return QueryAuthoringResourceScopeResult{WorkspaceID: command.WorkspaceID, WorkspaceRevision: view.WorkspaceRevision, InputHash: command.InputHash, ResourceRef: command.ResourceRef, DeliverableDirectory: view.DeliverableDirectory}, nil
}
