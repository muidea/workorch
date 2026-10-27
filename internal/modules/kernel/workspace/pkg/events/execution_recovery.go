package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
	"unicode/utf8"
)

// RecoverWorkspaceExecutionIntentCommand is the application POST preflight.
// It has no bootstrap and cannot reserve or forward. The public restart input
// contains only Workspace/request/kind; the owner retrieves the original full
// typed payload identities from its immutable reservation, never today's input.
type RecoverWorkspaceExecutionIntentCommand struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	Kind        runtimecontract.RuntimeExecutionIntentKind
}

func (c RecoverWorkspaceExecutionIntentCommand) Validate() error {
	workspaceID := string(c.WorkspaceID)
	validIdentity := func(v string) bool {
		return v != "" && len(v) <= 256 && strings.TrimSpace(v) == v && utf8.ValidString(v) && !strings.ContainsAny(v, "\x00\r\n")
	}
	if !validIdentity(workspaceID) || workspaceID == "." || workspaceID == ".." || strings.ContainsAny(workspaceID, "/\\") || !validIdentity(c.RequestID) || c.Kind != runtimecontract.RuntimeExecutionIntentRestart {
		return common.ErrInvalidArgument
	}
	return nil
}

// Nil Admission means no original and no blocking pending request was found.
// That absence is not a forward permit: the full ingress must still reserve.
type RecoverWorkspaceExecutionIntentResult struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	Kind        runtimecontract.RuntimeExecutionIntentKind
	Admission   *WorkspaceExecutionIntentResult
}

func (r RecoverWorkspaceExecutionIntentResult) ValidateFor(c RecoverWorkspaceExecutionIntentCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.RequestID != c.RequestID || r.Kind != c.Kind {
		return common.ErrConflict
	}
	if r.Admission != nil && (r.Admission.Validate() != nil || r.Admission.WorkspaceID != c.WorkspaceID || r.Admission.RequestID != c.RequestID || r.Admission.Kind != c.Kind) {
		return common.ErrConflict
	}
	return nil
}
