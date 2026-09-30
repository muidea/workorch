package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DefinitionAuthoringCreation 是初始创建身份，不随后续草稿修订改变。
// 同一请求的重放只能读取原 Workspace，不能重新应用初始输入。
type DefinitionAuthoringCreation struct {
	RequestID string
	InputHash string
}

func DefinitionAuthoringWorkspaceID(requestID string) common.WorkspaceID {
	sum := sha256.Sum256([]byte("workspace-authoring-create/v1\x00" + requestID))
	return common.WorkspaceID("ws-" + hex.EncodeToString(sum[:]))
}

func (c DefinitionAuthoringCreation) Validate(id common.WorkspaceID) error {
	if c.RequestID == "" || strings.TrimSpace(c.RequestID) != c.RequestID || len(c.RequestID) > 128 || strings.ContainsRune(c.RequestID, 0) || c.InputHash == "" || id != DefinitionAuthoringWorkspaceID(c.RequestID) {
		return fmt.Errorf("invalid authoring creation identity")
	}
	return nil
}
