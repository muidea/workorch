package events

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const RuntimeValidationRawOutputMaxBytes = runtimecontract.RuntimeValidationOutputChunkMaxBytes

// caller 仅选择原回执中的流；目录、ref 与实际保存身份必须由 owner 反查。
type QueryRuntimeValidationRawOutputCommand struct {
	Offset            int64 `json:"offset,omitempty"`
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
	Offset            int64
	ChunkSHA256       string
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
	if c.Offset < 0 || c.Offset > runtimecontract.RuntimeValidationOutputSavedMaxBytes || c.Fence == 0 || c.Stream != "stdout" && c.Stream != "stderr" {
		return common.ErrInvalidArgument
	}
	origin, err := runtimecontract.RuntimeOperationExecutionKeyOrigin(c.WorkspaceID, c.RunID, c.ExecutionKey, c.OperationID)
	if err != nil || origin != c.DispatchID {
		return common.ErrInvalidArgument
	}
	return nil
}

func (r QueryRuntimeValidationRawOutputResult) ValidateFor(c QueryRuntimeValidationRawOutputCommand) error {
	if c.Validate() != nil || r.SourceReceiptHash != c.SourceReceiptHash || r.StageID != c.StageID || r.Stream != c.Stream {
		return fmt.Errorf("%w: validation output source mismatch", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeValidationOutputChunk(r.Identity, r.SavedSHA256, r.Offset, r.ChunkSHA256, r.Raw, c.Offset); err != nil {
		return fmt.Errorf("%w: %v", common.ErrConflict, err)
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
