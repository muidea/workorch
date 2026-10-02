package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const DefinitionAuthoringSaveReceiptLimit = 128

// 保存回执与草稿在同一 Workspace CAS 内落盘，不依赖 Application 内存。
type DefinitionAuthoringSaveReceipt struct {
	RequestID            string
	RequestHash          string
	InputHash            string
	TemplateSnapshotHash string
	WorkspaceRevision    uint64
}

func DefinitionAuthoringSaveRequestHash(requestID string, input DefinitionAuthoringInput, expectedRevision uint64, expectedHash string, directory *string) string {
	input = input.Normalized()
	// 输入 revision 由 owner 分配，不能随重放时的现行 revision 改变请求身份。
	input.Revision = 1
	h := sha256.New()
	specHashFields(h, requestID, input.InputHash(), fmt.Sprint(expectedRevision), expectedHash)
	if directory == nil {
		specHashFields(h, "directory:unchanged")
	} else {
		specHashFields(h, "directory:explicit", *directory)
	}
	return "authoring-save-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (r DefinitionAuthoringSaveReceipt) Validate() error {
	if strings.TrimSpace(r.RequestID) != r.RequestID || r.RequestID == "" || len(r.RequestID) > 256 || strings.ContainsRune(r.RequestID, 0) || !validAuthoringSaveHash(r.RequestHash, "authoring-save-sha256:") || !validAuthoringSaveHash(r.InputHash, "definition-authoring-sha256:") || !validAuthoringSaveHash(r.TemplateSnapshotHash, "template-snapshot-sha256:") || r.WorkspaceRevision == 0 {
		return fmt.Errorf("invalid authoring save receipt")
	}
	return nil
}

func validAuthoringSaveHash(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && hex.EncodeToString(digest) == strings.TrimPrefix(value, prefix)
}
