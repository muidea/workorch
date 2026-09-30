package events

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// VerifyAuthoringResourceCommand checks one exact caller snapshot against a
// fresh File-owner observation. A content hash supplied by the caller alone
// never proves provenance; Workspace must fence the revision again on CAS.
type VerifyAuthoringResourceCommand struct {
	WorkspaceID        common.WorkspaceID
	WorkspaceRevision  uint64
	AuthoringInputHash string
	ResourceRef        string
	Content            string
	ContentHash        string
}

func (c VerifyAuthoringResourceCommand) Validate() error {
	query := AcquireAuthoringResourceCommand{WorkspaceID: c.WorkspaceID, AuthoringInputHash: c.AuthoringInputHash, ResourceRef: c.ResourceRef}
	result := AcquireAuthoringResourceResult{WorkspaceID: c.WorkspaceID, WorkspaceRevision: c.WorkspaceRevision, AuthoringInputHash: c.AuthoringInputHash, ResourceRef: c.ResourceRef, Content: c.Content, ContentHash: c.ContentHash, Size: int64(len(c.Content))}
	return result.ValidateFor(query)
}

type VerifyAuthoringResourceResult struct {
	WorkspaceID        common.WorkspaceID
	WorkspaceRevision  uint64
	AuthoringInputHash string
	ResourceRef        string
	ContentHash        string
}

func (r VerifyAuthoringResourceResult) ValidateFor(c VerifyAuthoringResourceCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision != c.WorkspaceRevision || r.AuthoringInputHash != c.AuthoringInputHash || r.ResourceRef != c.ResourceRef || r.ContentHash != c.ContentHash {
		return fmt.Errorf("authoring resource verification identity mismatch")
	}
	return nil
}
