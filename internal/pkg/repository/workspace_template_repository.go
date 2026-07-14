package repository

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

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

type WorkspaceTemplateRepository interface {
	Create(context.Context, *WorkspaceTemplate) error
	Get(context.Context, string) (*WorkspaceTemplate, error)
	List(context.Context) ([]*WorkspaceTemplate, error)
	Update(context.Context, *WorkspaceTemplate) error
	Delete(context.Context, string) error
}
