package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateDefinitionAuthoringCommand struct {
	RequestID            string
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	Input                workspace.DefinitionAuthoringInput
}

func (c CreateDefinitionAuthoringCommand) Validate() error {
	if err := c.Input.Validate(); err != nil {
		return err
	}
	if (workspace.DefinitionAuthoringCreation{RequestID: c.RequestID, InputHash: c.CreationInputHash()}).Validate(c.WorkspaceID) != nil || c.Input.WorkspaceID != c.WorkspaceID || c.Input.Revision != 1 {
		return fmt.Errorf("invalid authoring creation request identity")
	}
	if strings.TrimSpace(c.Title) == "" || len(c.Title) > 1024 || len(c.Description) > 32*1024 || c.DeliverableDirectory == "" || c.InternalDirectory == "" {
		return fmt.Errorf("invalid authoring creation command")
	}
	for _, value := range []string{c.Title, c.Description, c.DeliverableDirectory, c.InternalDirectory} {
		if strings.ContainsRune(value, 0) || len(value) > 32*1024 {
			return common.ErrInvalidArgument
		}
	}
	return nil
}

func (c CreateDefinitionAuthoringCommand) CreationInputHash() string {
	h := sha256.New()
	for _, value := range []string{"workspace-authoring-create/v1", c.RequestID, string(c.WorkspaceID), c.Title, c.Description, c.DeliverableDirectory, c.InternalDirectory, c.Input.InputHash()} {
		_, _ = fmt.Fprintf(h, "%08x%s", len(value), value)
	}
	return "authoring-creation-sha256:" + hex.EncodeToString(h.Sum(nil))
}

type CreateDefinitionAuthoringResult struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	InputHash   string
}

// RecordAuthoringDirectoryPreparedCommand records only the original creation's
// directory preparation. It cannot change input, seal a contract or activate a Run.
type RecordAuthoringDirectoryPreparedCommand struct {
	Creation CreateDefinitionAuthoringCommand
}

type RecordAuthoringDirectoryPreparedResult struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	InputHash   string
}

func (r RecordAuthoringDirectoryPreparedResult) ValidateFor(c RecordAuthoringDirectoryPreparedCommand) error {
	return (CreateDefinitionAuthoringResult{WorkspaceID: r.WorkspaceID, RequestID: r.RequestID, InputHash: r.InputHash}).ValidateFor(c.Creation)
}

type DefinitionAuthoringCreationStatus struct {
	RequestID         string
	InputHash         string
	DirectoryPrepared bool
	// Replay is present only when the owner can reproduce the exact creation hash.
	Replay *CreateDefinitionAuthoringCommand
}

func (r CreateDefinitionAuthoringResult) ValidateFor(c CreateDefinitionAuthoringCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.RequestID != c.RequestID || r.InputHash != c.CreationInputHash() {
		return common.ErrConflict
	}
	return nil
}
