package workspace

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (m *Biz) handleBuiltinTemplateDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	fail := func(err error) { result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err}) }
	if m == nil || m.templates == nil || ev == nil || ev.Source() != "application/workorchd/appservice" {
		fail(common.ErrInvalidArgument)
		return
	}
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceTemplateCommandQueryBuiltinDefaults:
		if _, ok := ev.Data().(workspaceevents.QueryBuiltinWorkspaceTemplateDefaultsCommand); !ok {
			fail(common.ErrInvalidArgument)
			return
		}
		items, err := m.queryBuiltinTemplateDefaults(ev.Context())
		if err != nil {
			fail(err)
			return
		}
		result.Set(workspaceevents.QueryBuiltinWorkspaceTemplateDefaultsResult{Defaults: items}, nil)
	case workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault:
		cmd, ok := ev.Data().(workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultCommand)
		if !ok {
			fail(common.ErrInvalidArgument)
			return
		}
		value, err := m.submitBuiltinTemplateUpgrade(ev.Context(), workspaceTemplateUpgradeEvent{TemplateID: cmd.TemplateID, ExpectedContentHash: cmd.ExpectedContentHash, TargetContentHash: cmd.TargetContentHash})
		if err != nil {
			fail(err)
			return
		}
		result.Set(value, nil)
	default:
		fail(common.ErrInvalidArgument)
	}
}

func builtinWorkspaceTemplate(raw workorchmanagementcontract.WorkspaceTemplate) (*workspacemodel.WorkspaceTemplate, error) {
	contextHints, err := workorcheventpayload.FromMap(raw.ContextHints)
	if err != nil {
		return nil, err
	}
	policies, err := workorcheventpayload.FromMapSlice(raw.EvidencePolicies)
	if err != nil {
		return nil, err
	}
	return workspaceTemplateFromCommand(workspaceevents.CreateWorkspaceTemplateCommand{
		TemplateID: raw.ID, Name: raw.Name, Description: raw.Description, GoalText: raw.GoalText,
		DoDItems: raw.DoDItems, DefaultSoulID: raw.DefaultSoulID, SoulBindings: soulbinding.FromStringMap(raw.SoulBindings),
		ContextHints: contextHints, EvidencePolicies: policies,
	})
}

func (m *Biz) queryBuiltinTemplateDefaults(ctx context.Context) ([]workspaceevents.BuiltinWorkspaceTemplateDefault, error) {
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		return nil, err
	}
	items := make([]workspaceevents.BuiltinWorkspaceTemplateDefault, 0, len(defaults))
	for _, raw := range defaults {
		target, err := builtinWorkspaceTemplate(raw)
		if err != nil {
			return nil, err
		}
		value, err := builtinTemplateDefaultObservation(target)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, nil
}

// The effective builtin is always the declaration embedded in this program.
func builtinTemplateDefaultObservation(target *workspacemodel.WorkspaceTemplate) (workspaceevents.BuiltinWorkspaceTemplateDefault, error) {
	hash := workspacemodel.StableWorkspaceTemplateContentHash(target)
	if hash == "" {
		return workspaceevents.BuiltinWorkspaceTemplateDefault{}, common.ErrInvalidArgument
	}
	return workspaceevents.BuiltinWorkspaceTemplateDefault{TemplateID: target.ID, Present: true, CurrentContentHash: hash, CurrentBuiltinRevision: hash, BuiltinContentHash: hash}, nil
}

func (m *Biz) upgradeBuiltinTemplate(ctx context.Context, event workspaceTemplateUpgradeEvent) (workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult, error) {
	defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, err
	}
	for _, raw := range defaults {
		if raw.ID != event.TemplateID {
			continue
		}
		target, err := builtinWorkspaceTemplate(raw)
		if err != nil {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, err
		}
		value, err := builtinTemplateDefaultObservation(target)
		if err != nil {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, err
		}
		if event.TargetContentHash != value.BuiltinContentHash || event.ExpectedContentHash != value.CurrentContentHash {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, fmt.Errorf("%w: builtin template is supplied by the running program", common.ErrConflict)
		}
		// Retained ingress is an exact, read-only no-op for current clients.
		return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{Default: value}, nil
	}
	return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, common.ErrNotFound
}
