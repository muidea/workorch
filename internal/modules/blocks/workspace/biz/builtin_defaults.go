package workspace

import (
	"context"
	"errors"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
		current, err := m.templates.Get(ctx, target.ID)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		value, err := builtinTemplateDefaultObservation(current, target)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, nil
}

func builtinTemplateDefaultObservation(current, target *workspacemodel.WorkspaceTemplate) (workspaceevents.BuiltinWorkspaceTemplateDefault, error) {
	value := workspaceevents.BuiltinWorkspaceTemplateDefault{TemplateID: target.ID, BuiltinContentHash: workspacemodel.StableWorkspaceTemplateContentHash(target)}
	if value.BuiltinContentHash == "" {
		return value, common.ErrInvalidArgument
	}
	if current == nil {
		return value, nil
	}
	if current.ID != target.ID {
		return value, common.ErrConflict
	}
	value.Present = true
	value.CurrentContentHash = workspacemodel.StableWorkspaceTemplateContentHash(current)
	if value.CurrentContentHash == "" {
		return value, common.ErrInvalidArgument
	}
	if value.CurrentContentHash == value.BuiltinContentHash {
		value.CurrentBuiltinRevision = value.BuiltinContentHash
		return value, nil
	}
	value.CurrentBuiltinRevision = workorchdefaults.KnownBuiltinContentRevision("workspace_template", current.ID, value.CurrentContentHash)
	value.UpgradeAvailable = value.CurrentBuiltinRevision != ""
	return value, nil
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
		current, err := m.templates.Get(ctx, event.TemplateID)
		if err != nil {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, err
		}
		value, err := builtinTemplateDefaultObservation(current, target)
		if err != nil {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, err
		}
		if event.TargetContentHash != value.BuiltinContentHash {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, fmt.Errorf("%w: builtin template target changed", common.ErrConflict)
		}
		knownExpected := event.ExpectedContentHash == value.BuiltinContentHash || workorchdefaults.KnownBuiltinContentRevision("workspace_template", event.TemplateID, event.ExpectedContentHash) != ""
		if !knownExpected || (value.CurrentContentHash != value.BuiltinContentHash && (!value.UpgradeAvailable || event.ExpectedContentHash != value.CurrentContentHash)) {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, fmt.Errorf("%w: template is customized, unknown, or changed since preview", common.ErrConflict)
		}
		if value.CurrentContentHash == value.BuiltinContentHash {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{Default: value}, nil
		}
		if err := m.templates.Update(ctx, target); err != nil {
			return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, err
		}
		updated, err := builtinTemplateDefaultObservation(target, target)
		return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{Default: updated, Upgraded: true}, err
	}
	return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, common.ErrNotFound
}
