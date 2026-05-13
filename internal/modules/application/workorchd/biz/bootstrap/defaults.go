package bootstrap

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"strings"

	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

//go:embed defaults/templates.json defaults/souls.json
var embeddedDefaults embed.FS

type embeddedTemplate struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	GoalText         string            `json:"goalText,omitempty"`
	DoDItems         []string          `json:"doDItems,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
	ContextHints     map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies []map[string]any  `json:"evidencePolicies,omitempty"`
}

type embeddedSoul struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description"`
	BusinessDomains             []string `json:"businessDomains"`
	AllowedSkillRefs            []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs        []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedCapabilityRefs       []string `json:"allowedCapabilityRefs,omitempty"`
	AllowedCapabilityCategories []string `json:"allowedCapabilityCategories,omitempty"`
	AllowedToolKinds            []string `json:"allowedToolKinds,omitempty"`
	SoulMD                      string   `json:"soulMD"`
}

func loadEmbeddedTemplates() ([]embeddedTemplate, error) {
	data, err := embeddedDefaults.ReadFile("defaults/templates.json")
	if err != nil {
		return nil, err
	}
	var items []embeddedTemplate
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func loadEmbeddedSouls() ([]embeddedSoul, error) {
	data, err := embeddedDefaults.ReadFile("defaults/souls.json")
	if err != nil {
		return nil, err
	}
	var items []embeddedSoul
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func EnsureBuiltinDefaults(ctx context.Context, soulRepo repository.SoulRepository, templateRepo repository.WorkspaceTemplateRepository) error {
	if soulRepo == nil || templateRepo == nil {
		return nil
	}

	souls, err := loadEmbeddedSouls()
	if err != nil {
		return err
	}
	for _, s := range souls {
		if s.ID == "" {
			continue
		}
		item := &soulmodel.Soul{
			ID:                          common.SoulID(s.ID),
			Name:                        s.Name,
			Description:                 s.Description,
			BusinessDomains:             s.BusinessDomains,
			SoulMD:                      s.SoulMD,
			AllowedSkillRefs:            s.AllowedSkillRefs,
			AllowedMCPServerRefs:        s.AllowedMCPServerRefs,
			AllowedCapabilityRefs:       s.AllowedCapabilityRefs,
			AllowedCapabilityCategories: s.AllowedCapabilityCategories,
			AllowedToolKinds:            s.AllowedToolKinds,
		}
		if err := upsertBuiltinSoul(ctx, soulRepo, item); err != nil {
			return err
		}
	}

	templates, err := loadEmbeddedTemplates()
	if err != nil {
		return err
	}
	for _, t := range templates {
		if t.ID == "" {
			continue
		}
		if err := upsertBuiltinWorkspaceTemplate(ctx, templateRepo, workspaceTemplateFromEmbedded(t)); err != nil {
			return err
		}
	}
	return nil
}

func upsertBuiltinSoul(ctx context.Context, repo repository.SoulRepository, item *soulmodel.Soul) error {
	if item == nil || item.ID == "" {
		return nil
	}
	if err := repo.Update(ctx, item); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return repo.Create(ctx, item)
		}
		return err
	}
	return nil
}

func upsertBuiltinWorkspaceTemplate(ctx context.Context, repo repository.WorkspaceTemplateRepository, item *repository.WorkspaceTemplate) error {
	if item == nil || strings.TrimSpace(item.ID) == "" {
		return nil
	}
	if err := repo.Update(ctx, item); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return repo.Create(ctx, item)
		}
		return err
	}
	return nil
}

func workspaceTemplateFromEmbedded(t embeddedTemplate) *repository.WorkspaceTemplate {
	return &repository.WorkspaceTemplate{
		ID:               t.ID,
		Name:             t.Name,
		Description:      t.Description,
		GoalText:         t.GoalText,
		DoDItems:         filterEmptyStrings(t.DoDItems),
		DefaultSoulID:    t.DefaultSoulID,
		SoulBindings:     t.SoulBindings,
		ContextHints:     cloneContextMap(t.ContextHints),
		EvidencePolicies: cloneEmbeddedEvidencePolicies(t.EvidencePolicies),
	}
}

func cloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneEmbeddedEvidencePolicyValue(value)
	}
	return ret
}

func cloneEmbeddedEvidencePolicies(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = cloneEmbeddedEvidencePolicyValue(value)
		}
		ret = append(ret, cloned)
	}
	return ret
}

func cloneEmbeddedEvidencePolicyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, value := range typed {
			cloned[key] = cloneEmbeddedEvidencePolicyValue(value)
		}
		return cloned
	case []map[string]any:
		return cloneEmbeddedEvidencePolicies(typed)
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneEmbeddedEvidencePolicyValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func filterEmptyStrings(items []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
