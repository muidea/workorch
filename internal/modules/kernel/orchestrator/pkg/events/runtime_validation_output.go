package events

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeValidationOutputReadSource is the application diagnostic read ingress.
// The relay must not let arbitrary capability callers impersonate Orchestrator
// at the Shell owner's restricted raw-output boundary.
const RuntimeValidationOutputReadSource = "application/workorchd/appservice"

// QueryRuntimeValidationOutputCommand selects a recorded diagnostic stream.
// It carries no directory or storage reference and grants no execution credit.
type QueryRuntimeValidationOutputCommand struct {
	Offset            int64              `json:"offset,omitempty"`
	WorkspaceID       common.WorkspaceID `json:"workspaceId"`
	RunID             common.RunID       `json:"runId"`
	DispatchID        string             `json:"dispatchId"`
	OperationID       string             `json:"operationId"`
	ExecutionKey      string             `json:"executionKey"`
	Fence             uint64             `json:"fence"`
	SourceReceiptHash string             `json:"sourceReceiptHash"`
	StageID           string             `json:"stageId"`
	Stream            string             `json:"stream"`
}

// ContentRef encodes only the immutable read selector, never a storage path.
// JSON/base64url is the reversible HTTP reference encoding, not EventHub I/O.
func (c QueryRuntimeValidationOutputCommand) ContentRef() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	body, err := json.Marshal(c)
	if err != nil || len(body) > 8192 || strings.ContainsAny(string(c.WorkspaceID)+string(c.RunID), "/\\") {
		return "", common.ErrInvalidArgument
	}
	return fmt.Sprintf("workorch://workspaces/%s/runs/%s/validation-output/%s/%s.bin", c.WorkspaceID, c.RunID, base64.RawURLEncoding.EncodeToString(body), c.Stream), nil
}

func (c QueryRuntimeValidationOutputCommand) Validate() error {
	for _, value := range []string{string(c.WorkspaceID), string(c.RunID), c.DispatchID, c.OperationID, c.ExecutionKey, c.SourceReceiptHash, c.StageID} {
		if value == "" || len(value) > 1024 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return common.ErrInvalidArgument
		}
	}
	if c.Offset < 0 || c.Offset > runtimecontract.RuntimeValidationOutputSavedMaxBytes || c.Fence == 0 || (c.Stream != "stdout" && c.Stream != "stderr") {
		return common.ErrInvalidArgument
	}
	origin, err := runtimecontract.RuntimeOperationExecutionKeyOrigin(c.WorkspaceID, c.RunID, c.ExecutionKey, c.OperationID)
	if err != nil || origin != c.DispatchID {
		return common.ErrInvalidArgument
	}
	return nil
}

type QueryRuntimeValidationOutputResult struct {
	Offset            int64
	ChunkSHA256       string
	SourceReceiptHash string
	StageID           string
	Stream            string
	Identity          runtimecontract.RuntimeValidationOutputIdentity
	SavedSHA256       string
	Raw               []byte
}

func (r QueryRuntimeValidationOutputResult) ValidateFor(c QueryRuntimeValidationOutputCommand) error {
	if c.Validate() != nil || r.SourceReceiptHash != c.SourceReceiptHash || r.StageID != c.StageID || r.Stream != c.Stream {
		return fmt.Errorf("%w: validation output source mismatch", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeValidationOutputChunk(r.Identity, r.SavedSHA256, r.Offset, r.ChunkSHA256, r.Raw, c.Offset); err != nil {
		return fmt.Errorf("%w: %v", common.ErrConflict, err)
	}
	return nil
}
