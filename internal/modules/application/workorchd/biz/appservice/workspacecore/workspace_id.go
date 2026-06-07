package workspacecore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func GenerateWorkspaceID() common.WorkspaceID {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return common.WorkspaceID("ws-" + hex.EncodeToString(raw[:]))
	}
	return common.WorkspaceID(fmt.Sprintf("ws-%d", time.Now().UTC().UnixNano()))
}
