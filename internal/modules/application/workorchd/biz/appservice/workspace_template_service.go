package appservice

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceTemplate struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	GoalText      string            `json:"goalText,omitempty"`
	DoDItems      []string          `json:"doDItems,omitempty"`
	DefaultSoulID string            `json:"defaultSoulId,omitempty"`
	SoulBindings  map[string]string `json:"soulBindings,omitempty"`
}

var workspaceTemplateMemoryStore = struct {
	sync.RWMutex
	items map[string]map[string]WorkspaceTemplate
}{
	items: map[string]map[string]WorkspaceTemplate{},
}

func ListWorkspaceTemplatesWithBackend(ctx context.Context, backend string, dataRoot string) ([]WorkspaceTemplate, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListWorkspaceTemplatesWithSession(ctx, session)
}

func ListWorkspaceTemplatesWithSession(_ context.Context, session *RuntimeSession) (ret []WorkspaceTemplate, err error) {
	if isFileRepositoryBackend(session.RepositoryBackend()) {
		items, err := readWorkspaceTemplateFile(workspaceTemplatePath(session.RepositoryBackend(), session.RepositoryDataDir()))
		if err != nil {
			return nil, err
		}
		return sortedWorkspaceTemplates(items), nil
	}
	workspaceTemplateMemoryStore.RLock()
	defer workspaceTemplateMemoryStore.RUnlock()
	return sortedWorkspaceTemplates(workspaceTemplateMemoryStore.items[workspaceTemplateMemoryKey(session)]), nil
}

func CreateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	return CreateWorkspaceTemplateWithSession(ctx, session, template)
}

func CreateWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	item, err := normalizeWorkspaceTemplate(template)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	if err := validateWorkspaceTemplateSouls(ctx, session, item); err != nil {
		return WorkspaceTemplate{}, err
	}
	if isFileRepositoryBackend(session.RepositoryBackend()) {
		path := workspaceTemplatePath(session.RepositoryBackend(), session.RepositoryDataDir())
		items, err := readWorkspaceTemplateFile(path)
		if err != nil {
			return WorkspaceTemplate{}, err
		}
		if _, ok := items[item.ID]; ok {
			return WorkspaceTemplate{}, common.ErrConflict
		}
		items[item.ID] = item
		return item, writeWorkspaceTemplateFile(path, items)
	}
	workspaceTemplateMemoryStore.Lock()
	defer workspaceTemplateMemoryStore.Unlock()
	key := workspaceTemplateMemoryKey(session)
	if workspaceTemplateMemoryStore.items[key] == nil {
		workspaceTemplateMemoryStore.items[key] = map[string]WorkspaceTemplate{}
	}
	if _, ok := workspaceTemplateMemoryStore.items[key][item.ID]; ok {
		return WorkspaceTemplate{}, common.ErrConflict
	}
	workspaceTemplateMemoryStore.items[key][item.ID] = item
	return item, nil
}

func UpdateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	return UpdateWorkspaceTemplateWithSession(ctx, session, templateID, template)
}

func UpdateWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, templateID string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	template.ID = strings.TrimSpace(templateID)
	item, err := normalizeWorkspaceTemplate(template)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	if err := validateWorkspaceTemplateSouls(ctx, session, item); err != nil {
		return WorkspaceTemplate{}, err
	}
	if isFileRepositoryBackend(session.RepositoryBackend()) {
		path := workspaceTemplatePath(session.RepositoryBackend(), session.RepositoryDataDir())
		items, err := readWorkspaceTemplateFile(path)
		if err != nil {
			return WorkspaceTemplate{}, err
		}
		if _, ok := items[item.ID]; !ok {
			return WorkspaceTemplate{}, common.ErrNotFound
		}
		items[item.ID] = item
		return item, writeWorkspaceTemplateFile(path, items)
	}
	workspaceTemplateMemoryStore.Lock()
	defer workspaceTemplateMemoryStore.Unlock()
	key := workspaceTemplateMemoryKey(session)
	if _, ok := workspaceTemplateMemoryStore.items[key][item.ID]; !ok {
		return WorkspaceTemplate{}, common.ErrNotFound
	}
	workspaceTemplateMemoryStore.items[key][item.ID] = item
	return item, nil
}

func normalizeWorkspaceTemplate(template WorkspaceTemplate) (WorkspaceTemplate, error) {
	item := WorkspaceTemplate{
		ID:            strings.TrimSpace(template.ID),
		Name:          strings.TrimSpace(template.Name),
		Description:   strings.TrimSpace(template.Description),
		GoalText:      strings.TrimSpace(template.GoalText),
		DoDItems:      normalizeDoDItems(template.DoDItems),
		DefaultSoulID: strings.TrimSpace(template.DefaultSoulID),
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
	if exports.Repositories.Souls == nil {
		return common.ErrInvalidArgument
	}
	if template.DefaultSoulID != "" {
		if _, err := exports.Repositories.Souls.Get(ctx, common.SoulID(template.DefaultSoulID)); err != nil {
			return err
		}
	}
	for _, soulID := range template.SoulBindings {
		if strings.TrimSpace(soulID) == "" {
			continue
		}
		if _, err := exports.Repositories.Souls.Get(ctx, common.SoulID(soulID)); err != nil {
			return err
		}
	}
	return nil
}

func workspaceTemplatePath(backend string, dataRoot string) string {
	root := bootstrap.ResolveRepositoryDataRoot(strings.TrimSpace(backend), strings.TrimSpace(dataRoot))
	return filepath.Join(root, "system", "workspace_templates.json")
}

func workspaceTemplateMemoryKey(session *RuntimeSession) string {
	return session.RepositoryBackend() + ":" + session.RepositoryDataDir()
}

func isFileRepositoryBackend(backend string) bool {
	return strings.EqualFold(strings.TrimSpace(backend), "file")
}

func readWorkspaceTemplateFile(path string) (map[string]WorkspaceTemplate, error) {
	content, err := os.ReadFile(path)
	if err != nil && os.IsNotExist(err) {
		return map[string]WorkspaceTemplate{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return map[string]WorkspaceTemplate{}, nil
	}
	var items map[string]WorkspaceTemplate
	if err := json.Unmarshal(content, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = map[string]WorkspaceTemplate{}
	}
	return items, nil
}

func writeWorkspaceTemplateFile(path string, items map[string]WorkspaceTemplate) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o644)
}

func sortedWorkspaceTemplates(items map[string]WorkspaceTemplate) []WorkspaceTemplate {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]WorkspaceTemplate, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, items[id])
	}
	return ret
}
