package appmodelcore

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Workspace struct {
	Revision             uint64                                      `json:"revision"`
	ID                   string                                      `json:"id"`
	Title                string                                      `json:"title"`
	Description          string                                      `json:"description"`
	DeliverableDirectory string                                      `json:"deliverableDirectory"`
	InternalDirectory    string                                      `json:"-"`
	State                string                                      `json:"state"`
	ActiveRunID          string                                      `json:"activeRunId,omitempty"`
	TemplateID           string                                      `json:"templateId,omitempty"`
	ProfileID            string                                      `json:"profileId,omitempty"`
	DefaultLLMSelection  *runtimecontract.ProviderInferenceSelection `json:"defaultLLMSelection,omitempty"`
	DefaultSoulID        string                                      `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string                           `json:"soulBindings,omitempty"`
	ContextHints         map[string]any                              `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any                            `json:"evidencePolicies,omitempty"`
	ViewModel            projectioncontract.WorkspaceViewModel       `json:"viewModel,omitempty"`
}

type DeleteWorkspaceCommand struct {
	Purge bool `json:"purge,omitempty"`
}
