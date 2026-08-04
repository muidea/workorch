package workspace

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// WorkspaceTemplate is the workspace owner's durable template DTO. Repository
// representations are intentionally kept inside the workspace owner.
type WorkspaceTemplate struct {
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	Description      string                         `json:"description,omitempty"`
	GoalText         string                         `json:"goalText,omitempty"`
	DoDItems         []string                       `json:"doDItems,omitempty"`
	DefaultSoulID    string                         `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string              `json:"soulBindings,omitempty"`
	ContextHints     workorcheventpayload.Payload   `json:"contextHints,omitempty"`
	EvidencePolicies []workorcheventpayload.Payload `json:"evidencePolicies,omitempty"`
}
