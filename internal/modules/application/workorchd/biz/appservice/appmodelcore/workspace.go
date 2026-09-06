package appmodelcore

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Workspace struct {
	Revision                     uint64                                      `json:"revision"`
	ID                           string                                      `json:"id"`
	Title                        string                                      `json:"title"`
	Description                  string                                      `json:"description"`
	DeliverableDirectory         string                                      `json:"deliverableDirectory"`
	InternalDirectory            string                                      `json:"-"`
	State                        string                                      `json:"state"`
	ActiveRunID                  string                                      `json:"activeRunId,omitempty"`
	TemplateID                   string                                      `json:"templateId,omitempty"`
	AdmittedStartupInputHash     string                                      `json:"admittedStartupInputHash,omitempty"`
	AdmittedTemplateRevision     string                                      `json:"admittedTemplateRevision,omitempty"`
	AdmittedTemplateSnapshotHash string                                      `json:"admittedTemplateSnapshotHash,omitempty"`
	ProfileID                    string                                      `json:"profileId,omitempty"`
	DefaultLLMSelection          *runtimecontract.ProviderInferenceSelection `json:"defaultLLMSelection,omitempty"`
	DefaultSoulID                string                                      `json:"defaultSoulId,omitempty"`
	SoulBindings                 map[string]string                           `json:"soulBindings,omitempty"`
	ContextHints                 map[string]any                              `json:"contextHints,omitempty"`
	EvidencePolicies             []map[string]any                            `json:"evidencePolicies,omitempty"`
	ViewModel                    projectioncontract.WorkspaceViewModel       `json:"viewModel,omitempty"`
}

type DeleteWorkspaceCommand struct {
	RequestID        string `json:"requestId,omitempty"`
	Purge            bool   `json:"purge,omitempty"`
	ExpectedRevision uint64 `json:"expectedRevision,omitempty"`
}

func (c DeleteWorkspaceCommand) Normalized(workspaceID string) DeleteWorkspaceCommand {
	c.RequestID = strings.TrimSpace(c.RequestID)
	if c.RequestID == "" && strings.TrimSpace(workspaceID) != "" {
		// Workspace IDs are never reused. Keep the ingress request identity mode
		// independent so a retry with changed intent cannot create a second
		// deletion journal; the Workspace owner rejects the payload mismatch.
		digest := sha256.Sum256([]byte("workorchd.workspace-deletion-request\x00" + strings.TrimSpace(workspaceID)))
		c.RequestID = "workspace-deletion-request-sha256:" + hex.EncodeToString(digest[:])
	}
	return c
}
