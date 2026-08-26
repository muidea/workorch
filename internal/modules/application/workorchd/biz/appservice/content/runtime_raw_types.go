package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

const MaxRuntimePayloadBytes = 1 << 20

type RuntimeRawContent struct {
	Ref           string
	FileName      string
	ContentType   string
	Content       []byte
	ContentSHA256 string
}

func newRuntimeRawContent(ref RuntimeRawRef, body []byte) (RuntimeRawContent, error) {
	if len(body) == 0 || len(body) > MaxRuntimePayloadBytes {
		return RuntimeRawContent{}, fmt.Errorf("%w: runtime payload must contain between 1 and %d bytes", common.ErrConflict, MaxRuntimePayloadBytes)
	}
	content := append([]byte(nil), body...)
	hash := sha256.Sum256(content)
	return RuntimeRawContent{
		Ref: ref.Ref, FileName: ref.FileName, ContentType: RuntimeRawContentType(ref.FileName),
		Content: content, ContentSHA256: hex.EncodeToString(hash[:]),
	}, nil
}

type RuntimeRawRef struct {
	Ref         string
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Collection  string
	ItemID      string
	FileName    string
}
