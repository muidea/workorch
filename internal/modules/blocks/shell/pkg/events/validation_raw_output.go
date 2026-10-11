package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const RuntimeValidationRawOutputMaxBytes = 64 * 1024

// caller 仅选择原回执中的流；目录、ref 与实际保存身份必须由 owner 反查。
type QueryRuntimeValidationRawOutputCommand struct {
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	DispatchID        string
	OperationID       string
	ExecutionKey      string
	Fence             uint64
	SourceReceiptHash string
	StageID           string
	Stream            string
}

type QueryRuntimeValidationRawOutputResult struct {
	SourceReceiptHash string
	StageID           string
	Stream            string
	Identity          runtimecontract.RuntimeValidationOutputIdentity
	SavedSHA256       string
	Raw               []byte
}

func (c QueryRuntimeValidationRawOutputCommand) Validate() error {
	for _, value := range []string{string(c.WorkspaceID), string(c.RunID), c.DispatchID, c.OperationID, c.ExecutionKey, c.SourceReceiptHash, c.StageID} {
		if value == "" || len(value) > 1024 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return common.ErrInvalidArgument
		}
	}
	if c.Fence == 0 || c.Stream != "stdout" && c.Stream != "stderr" {
		return common.ErrInvalidArgument
	}
	origin, err := runtimecontract.RuntimeOperationExecutionKeyOrigin(c.WorkspaceID, c.RunID, c.ExecutionKey, c.OperationID)
	if err != nil || origin != c.DispatchID {
		return common.ErrInvalidArgument
	}
	return nil
}

func (r QueryRuntimeValidationRawOutputResult) ValidateFor(c QueryRuntimeValidationRawOutputCommand) error {
	if c.Validate() != nil || r.SourceReceiptHash != c.SourceReceiptHash || r.StageID != c.StageID || r.Stream != c.Stream ||
		r.Identity.Validate() != nil || r.Identity.Storage == nil || r.Identity.Storage.Reference == "" ||
		len(r.Raw) > RuntimeValidationRawOutputMaxBytes || r.Identity.SavedBytes != int64(len(r.Raw)) {
		return fmt.Errorf("%w: validation raw output response identity or bounds mismatch", common.ErrConflict)
	}
	digest := sha256.Sum256(r.Raw)
	if r.SavedSHA256 != hex.EncodeToString(digest[:]) || r.Identity.Bytes == r.Identity.SavedBytes && r.Identity.SHA256 != r.SavedSHA256 {
		return fmt.Errorf("%w: validation raw output response bytes mismatch", common.ErrConflict)
	}
	return nil
}

func (r QueryRuntimeValidationRawOutputResult) Clone() QueryRuntimeValidationRawOutputResult {
	r.Identity = r.Identity.Clone()
	if r.Raw != nil {
		r.Raw = append(make([]byte, 0, len(r.Raw)), r.Raw...)
	}
	return r
}
