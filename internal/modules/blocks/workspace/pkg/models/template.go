package workspace

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// WorkspaceTemplate is the workspace owner's durable template DTO. Repository
// representations are intentionally kept inside the workspace owner.
type WorkspaceTemplate struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     map[string]string
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}
