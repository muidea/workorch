package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DefinitionAuthoringCreation 是初始创建身份，不随后续草稿修订改变。
// 同一请求的重放只能恢复未完成的持久化，不能重新应用初始输入。
type DefinitionAuthoringCreation struct {
	RequestID string
	InputHash string
	// DirectoryPrepared records completion of application-owned directory preparation.
	// It is a historical creation fact, not current resource readiness or admission.
	DirectoryPrepared bool
	// PendingPersistence 在 metadata 可见前封存，全部绑定／索引提交后清除。
	// 仅由 Workspace 自有持久化层使用，不作为运行或准入权威。
	PendingPersistence *DefinitionAuthoringCreationPersistence
}

type DefinitionAuthoringCreationPersistence struct {
	WorkspaceRevision uint64
	DefaultSoulID     common.SoulID
	SoulBindings      map[string]common.SoulID
}

func (c DefinitionAuthoringCreation) Clone() DefinitionAuthoringCreation {
	if c.PendingPersistence != nil {
		pending := *c.PendingPersistence
		if pending.SoulBindings != nil {
			pending.SoulBindings = make(map[string]common.SoulID, len(c.PendingPersistence.SoulBindings))
			for key, value := range c.PendingPersistence.SoulBindings {
				pending.SoulBindings[key] = value
			}
		}
		c.PendingPersistence = &pending
	}
	return c
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
