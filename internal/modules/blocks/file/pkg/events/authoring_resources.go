package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const AuthoringResourceMaxBytes = 32 * 1024

// AcquireAuthoringResourceCommand reads one workspace-relative regular file
// only after the File owner asks Workspace for the current draft scope. This
// creation-time observation has no Run or Runtime authority identity.
type AcquireAuthoringResourceCommand struct {
	WorkspaceID        common.WorkspaceID
	AuthoringInputHash string
	ResourceRef        string
}

func (c AcquireAuthoringResourceCommand) Validate() error {
	if c.WorkspaceID == "" || len(c.WorkspaceID) > 256 || c.AuthoringInputHash == "" || len(c.AuthoringInputHash) > 256 || c.ResourceRef == "" || len(c.ResourceRef) > 1024 || !utf8.ValidString(c.ResourceRef) || strings.ContainsRune(c.ResourceRef, 0) {
		return fmt.Errorf("authoring resource query requires bounded workspace, input and ref")
	}
	return nil
}

type AcquireAuthoringResourceResult struct {
	WorkspaceID        common.WorkspaceID
	WorkspaceRevision  uint64
	AuthoringInputHash string
	ResourceRef        string
	Content            string
	ContentHash        string
	Size               int64
}

func (r AcquireAuthoringResourceResult) ValidateFor(c AcquireAuthoringResourceCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.AuthoringInputHash != c.AuthoringInputHash || r.ResourceRef != c.ResourceRef || len(r.Content) > AuthoringResourceMaxBytes || int64(len(r.Content)) != r.Size || !utf8.ValidString(r.Content) || strings.ContainsRune(r.Content, 0) {
		return fmt.Errorf("authoring resource result scope or content is invalid")
	}
	checksum := sha256.Sum256([]byte(r.Content))
	if r.ContentHash != "sha256:"+hex.EncodeToString(checksum[:]) {
		return fmt.Errorf("authoring resource result hash is invalid")
	}
	return nil
}
