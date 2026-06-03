package appservice

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type WorkspaceTemplate struct {
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

func ListWorkspaceTemplatesWithBackend(ctx context.Context, backend string, dataRoot string) ([]WorkspaceTemplate, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListWorkspaceTemplatesWithSession(ctx, session)
}

func ListWorkspaceTemplatesWithSession(ctx context.Context, session *RuntimeSession) (ret []WorkspaceTemplate, err error) {
	repo, err := workspaceTemplateRepositoryFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		ret = append(ret, workspaceTemplateFromRepository(item))
	}
	return ret, nil
}

func CreateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	return CreateWorkspaceTemplateWithSession(ctx, session, template)
}

func CreateWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, template WorkspaceTemplate) (ret WorkspaceTemplate, err error) {
	repo, err := workspaceTemplateRepositoryFromSession(session)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := normalizeWorkspaceTemplate(template)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	if err := validateWorkspaceTemplateSouls(ctx, session, item); err != nil {
		return WorkspaceTemplate{}, err
	}
	if err := repo.Create(ctx, workspaceTemplateToRepository(item)); err != nil {
		return WorkspaceTemplate{}, err
	}
	return item, nil
}

func UpdateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	return UpdateWorkspaceTemplateWithSession(ctx, session, templateID, template)
}

func UpdateWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, templateID string, template WorkspaceTemplate) (ret WorkspaceTemplate, err error) {
	repo, err := workspaceTemplateRepositoryFromSession(session)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	template.ID = strings.TrimSpace(templateID)
	item, err := normalizeWorkspaceTemplate(template)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	if err := validateWorkspaceTemplateSouls(ctx, session, item); err != nil {
		return WorkspaceTemplate{}, err
	}
	if err := repo.Update(ctx, workspaceTemplateToRepository(item)); err != nil {
		return WorkspaceTemplate{}, err
	}
	return item, nil
}

func DeleteWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string) error {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return err
	}
	return DeleteWorkspaceTemplateWithSession(ctx, session, templateID)
}

func DeleteWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, templateID string) (err error) {
	repo, err := workspaceTemplateRepositoryFromSession(session)
	if err != nil {
		return err
	}
	defer session.CloseOnReturn(ctx, &err)

	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return common.ErrInvalidArgument
	}
	return repo.Delete(ctx, templateID)
}

func workspaceTemplateRepositoryFromSession(session *RuntimeSession) (repository.WorkspaceTemplateRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Repositories.WorkspaceTemplates == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Repositories.WorkspaceTemplates, nil
}

func normalizeWorkspaceTemplate(template WorkspaceTemplate) (WorkspaceTemplate, error) {
	item := WorkspaceTemplate{
		ID:               strings.TrimSpace(template.ID),
		Name:             strings.TrimSpace(template.Name),
		Description:      strings.TrimSpace(template.Description),
		GoalText:         strings.TrimSpace(template.GoalText),
		DoDItems:         normalizeDoDItems(template.DoDItems),
		DefaultSoulID:    strings.TrimSpace(template.DefaultSoulID),
		ContextHints:     cloneContextMap(template.ContextHints),
		EvidencePolicies: cloneContextMapSlice(template.EvidencePolicies),
	}
	if item.ID == "" || item.Name == "" {
		return WorkspaceTemplate{}, common.ErrInvalidArgument
	}
	bindings, err := normalizeSoulBindings(template.SoulBindings)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	item.SoulBindings = mapSoulBindingIDs(bindings)
	return item, nil
}

func validateWorkspaceTemplateSouls(ctx context.Context, session *RuntimeSession, template WorkspaceTemplate) error {
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	if exports.Infrastructure.EventHub == nil {
		return common.ErrInvalidArgument
	}
	if template.DefaultSoulID != "" {
		if _, err := readSoulByOwnerCommand(ctx, exports.Infrastructure.EventHub, common.SoulID(template.DefaultSoulID)); err != nil {
			return err
		}
	}
	for _, soulID := range template.SoulBindings {
		if strings.TrimSpace(soulID) == "" {
			continue
		}
		if _, err := readSoulByOwnerCommand(ctx, exports.Infrastructure.EventHub, common.SoulID(soulID)); err != nil {
			return err
		}
	}
	return nil
}

func workspaceTemplateToRepository(item WorkspaceTemplate) *repository.WorkspaceTemplate {
	return &repository.WorkspaceTemplate{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     cloneContextMap(item.ContextHints),
		EvidencePolicies: cloneContextMapSlice(item.EvidencePolicies),
	}
}

func workspaceTemplateFromRepository(item *repository.WorkspaceTemplate) WorkspaceTemplate {
	if item == nil {
		return WorkspaceTemplate{}
	}
	return WorkspaceTemplate{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     cloneContextMap(item.ContextHints),
		EvidencePolicies: cloneContextMapSlice(item.EvidencePolicies),
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]string{}
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneContextMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneContextMapDeep(item))
	}
	return ret
}

func cloneContextMapDeep(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func cloneContextValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneContextMapDeep(typed)
	case []map[string]any:
		return cloneContextMapSlice(typed)
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneContextValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
