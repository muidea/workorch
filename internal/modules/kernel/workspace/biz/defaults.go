package workspace

import (
	"context"
	"sort"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

// Builtins belong to the running program. Startup validates their declarations;
// it never seeds, upgrades or deletes a durable catalog copy.
func (m *Biz) ensureCurrentBuiltinWorkspaceTemplateDefaults(ctx context.Context) error {
	if m == nil || m.templates == nil {
		return common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	items, err := builtinWorkspaceTemplates()
	if err != nil {
		return err
	}
	for _, item := range items {
		if _, err := m.resolvedTemplate(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func builtinWorkspaceTemplates() ([]*workspacemodel.WorkspaceTemplate, error) {
	defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		return nil, err
	}
	items := make([]*workspacemodel.WorkspaceTemplate, 0, len(defaults))
	seen := make(map[string]bool, len(defaults))
	for _, raw := range defaults {
		if seen[raw.ID] {
			return nil, common.ErrConflict
		}
		seen[raw.ID] = true
		item, err := builtinWorkspaceTemplate(raw)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func builtinWorkspaceTemplateByID(id string) (*workspacemodel.WorkspaceTemplate, error) {
	items, err := builtinWorkspaceTemplates()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

func (m *Biz) currentWorkspaceTemplate(ctx context.Context, id string) (*workspacemodel.WorkspaceTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, err := builtinWorkspaceTemplateByID(id)
	if err != nil || item != nil {
		return item, err
	}
	return m.templates.Get(ctx, id)
}

func (m *Biz) currentWorkspaceTemplates(ctx context.Context) ([]*workspacemodel.WorkspaceTemplate, error) {
	items, err := builtinWorkspaceTemplates()
	if err != nil {
		return nil, err
	}
	reserved := make(map[string]bool, len(items))
	for _, item := range items {
		reserved[item.ID] = true
	}
	custom, err := m.templates.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range custom {
		// Historical catalog rows cannot override program-owned declarations.
		// Leave their bytes untouched; they are not current template authority.
		if !reserved[item.ID] {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
