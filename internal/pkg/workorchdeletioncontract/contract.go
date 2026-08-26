package workorchdeletioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type Mode string

const (
	ModeRetain Mode = "retain"
	ModePurge  Mode = "purge"
)

type Scope struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	Mode               Mode
	QuiescenceID       string
	RequestHash        string
}

func (s Scope) Normalize() Scope {
	s.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(s.WorkspaceID)))
	s.DeletionID = strings.TrimSpace(s.DeletionID)
	s.Mode = Mode(strings.TrimSpace(string(s.Mode)))
	s.QuiescenceID = strings.TrimSpace(s.QuiescenceID)
	s.RequestHash = strings.TrimSpace(s.RequestHash)
	return s
}

func (s Scope) Validate() error {
	s = s.Normalize()
	if s.WorkspaceID == "" || s.DeletionID == "" || s.DeletionGeneration != 1 || s.QuiescenceID == "" || s.RequestHash == "" {
		return common.ErrInvalidArgument
	}
	switch s.Mode {
	case ModeRetain, ModePurge:
	default:
		return common.ErrInvalidArgument
	}
	if s.RequestHash != StableRequestHash(s) {
		return fmt.Errorf("%w: workspace deletion request hash mismatch", common.ErrConflict)
	}
	return nil
}

func StableRequestHash(s Scope) string {
	s = s.Normalize()
	material := fmt.Sprintf("workspace-deletion-request\x00%s\x00%s\x00%d\x00%s\x00%s", s.WorkspaceID, s.DeletionID, s.DeletionGeneration, s.Mode, s.QuiescenceID)
	digest := sha256.Sum256([]byte(material))
	return "workspace-deletion-request-sha256:" + hex.EncodeToString(digest[:])
}
