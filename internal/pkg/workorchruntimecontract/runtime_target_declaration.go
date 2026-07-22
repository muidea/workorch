package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

// RuntimeTargetDeclaration is an explicit, Workspace-owner supplied binding
// for a direct-evidence focus. It is declarative input sealed at activation;
// it never selects an action, performs I/O, or changes Runtime state.
type RuntimeTargetDeclaration struct {
	TargetRef  string                       `json:"targetRef"`
	TargetRole string                       `json:"targetRole"`
	Mode       RuntimeTargetDeclarationMode `json:"mode"`
	Revision   string                       `json:"revision,omitempty"`
}

type RuntimeTargetDeclarationMode string

const (
	RuntimeTargetDeclarationExistingFile RuntimeTargetDeclarationMode = "existing_file"
	RuntimeTargetDeclarationNewFile      RuntimeTargetDeclarationMode = "new_file"
)

func (d RuntimeTargetDeclaration) Normalized() RuntimeTargetDeclaration {
	d.TargetRef = strings.TrimSpace(strings.ReplaceAll(d.TargetRef, "\\", "/"))
	d.TargetRole = strings.TrimSpace(d.TargetRole)
	d.Mode = RuntimeTargetDeclarationMode(strings.TrimSpace(string(d.Mode)))
	if d.TargetRef == "" || d.TargetRole == "" || d.Mode == "" {
		d.Revision = ""
		return d
	}
	d.Revision = StableRuntimeTargetDeclarationRevision(d)
	return d
}

func (d RuntimeTargetDeclaration) Validate() error {
	d = d.Normalized()
	if d.TargetRef == "" || d.TargetRole == "" || d.Revision == "" {
		return fmt.Errorf("runtime target declaration requires target, role and revision")
	}
	clean := path.Clean(d.TargetRef)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || clean != d.TargetRef {
		return fmt.Errorf("runtime target declaration requires a canonical workspace-relative target")
	}
	switch d.Mode {
	case RuntimeTargetDeclarationExistingFile, RuntimeTargetDeclarationNewFile:
	default:
		return fmt.Errorf("runtime target declaration has unsupported mode %q", d.Mode)
	}
	return nil
}

func (d RuntimeTargetDeclaration) Clone() RuntimeTargetDeclaration { return d.Normalized() }

func SameRuntimeTargetDeclaration(left, right *RuntimeTargetDeclaration) bool {
	if left == nil || right == nil {
		return left == right
	}
	l, r := left.Normalized(), right.Normalized()
	return l.TargetRef == r.TargetRef && l.TargetRole == r.TargetRole && l.Mode == r.Mode && l.Revision == r.Revision
}

func StableRuntimeTargetDeclarationRevision(value RuntimeTargetDeclaration) string {
	value.TargetRef = strings.TrimSpace(strings.ReplaceAll(value.TargetRef, "\\", "/"))
	value.TargetRole = strings.TrimSpace(value.TargetRole)
	value.Mode = RuntimeTargetDeclarationMode(strings.TrimSpace(string(value.Mode)))
	hash := sha256.Sum256([]byte(strings.Join([]string{value.TargetRef, value.TargetRole, string(value.Mode)}, "\x00")))
	return "target-declaration-" + hex.EncodeToString(hash[:])
}

// RuntimeTargetSnapshotSatisfiesDeclaration reports whether a File-owner
// snapshot can satisfy the activation-sealed declaration. A missing snapshot
// is never generally usable: it is accepted only for the exact declared
// new-file target with an owner observation and workspace-tree revision.
func RuntimeTargetSnapshotSatisfiesDeclaration(snapshot TargetSnapshot, declaration *RuntimeTargetDeclaration) bool {
	snapshot.TargetRef = strings.TrimSpace(snapshot.TargetRef)
	snapshot.ContentHash = strings.TrimSpace(snapshot.ContentHash)
	snapshot.Revision = strings.TrimSpace(snapshot.Revision)
	snapshot.WorkspaceTreeRevision = strings.TrimSpace(snapshot.WorkspaceTreeRevision)
	if snapshot.Complete() {
		return true
	}
	if declaration == nil {
		return false
	}
	value := declaration.Normalized()
	if value.Validate() != nil || value.Mode != RuntimeTargetDeclarationNewFile || value.TargetRef != snapshot.TargetRef {
		return false
	}
	return snapshot.ContentState == SnapshotContentStateMissing && snapshot.Content == "" && snapshot.ContentHash == "" &&
		snapshot.Revision != "" && snapshot.WorkspaceTreeRevision != ""
}
