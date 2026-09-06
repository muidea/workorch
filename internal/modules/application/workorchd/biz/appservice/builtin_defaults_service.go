package appservice

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ListBuiltinDefaultsWithSession(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.BuiltinDefault, error) {
	return withCommandHubSession(ctx, session, queryBuiltinDefaults)
}

func UpgradeBuiltinDefaultWithSession(ctx context.Context, session *runtimecore.Session, request workorchmanagementcontract.BuiltinDefaultUpgrade) (workorchmanagementcontract.BuiltinDefaultUpgradeResult, error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.BuiltinDefaultUpgradeResult, error) {
		return upgradeBuiltinDefault(ctx, hub, request)
	})
}

func queryBuiltinDefaults(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.BuiltinDefault, error) {
	soulEvent := magicevent.NewEventWithContext(soulevents.TopicSoulCommandQueryBuiltinDefaults, "application/workorchd/context-registry", soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.QueryBuiltinSoulDefaultsCommand{})
	ownerevent.BindLane(soulEvent, soulevents.Contracts(), soulEvent.ID(), "soul", "builtin-defaults", "", "", "")
	soulResult := hub.Send(soulEvent)
	if soulResult == nil {
		return nil, fmt.Errorf("Soul builtin query returned no result")
	}
	soulData, err := soulResult.Get()
	if err != nil {
		return nil, err
	}
	souls, ok := soulData.(soulevents.QueryBuiltinSoulDefaultsResult)
	if !ok || len(souls.Defaults) == 0 || len(souls.Defaults) > 200 {
		return nil, fmt.Errorf("Soul builtin query returned invalid result")
	}
	items := make([]workorchmanagementcontract.BuiltinDefault, 0, len(souls.Defaults))
	for _, item := range souls.Defaults {
		items = append(items, builtinSoulView(item))
	}
	templateEvent := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandQueryBuiltinDefaults, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.QueryBuiltinWorkspaceTemplateDefaultsCommand{})
	ownerevent.BindLane(templateEvent, workspaceevents.Contracts(), templateEvent.ID(), "workspace_template", "builtin-defaults", "", "", "")
	templateResult := hub.Send(templateEvent)
	if templateResult == nil {
		return nil, fmt.Errorf("Workspace builtin query returned no result")
	}
	templateData, err := templateResult.Get()
	if err != nil {
		return nil, err
	}
	templates, ok := templateData.(workspaceevents.QueryBuiltinWorkspaceTemplateDefaultsResult)
	if !ok || len(templates.Defaults) == 0 || len(templates.Defaults) > 200 {
		return nil, fmt.Errorf("Workspace builtin query returned invalid result")
	}
	for _, item := range templates.Defaults {
		items = append(items, builtinTemplateView(item))
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		key := item.Kind + "/" + item.ID
		if err := validateBuiltinDefaultView(item); err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate builtin resource %q", key)
		}
		seen[key] = true
	}
	return items, nil
}

func upgradeBuiltinDefault(ctx context.Context, hub magicevent.Hub, request workorchmanagementcontract.BuiltinDefaultUpgrade) (workorchmanagementcontract.BuiltinDefaultUpgradeResult, error) {
	ret := workorchmanagementcontract.BuiltinDefaultUpgradeResult{}
	for _, value := range []string{request.ID, request.ExpectedContentHash, request.TargetContentHash} {
		if value == "" || value != strings.TrimSpace(value) {
			return ret, common.ErrInvalidArgument
		}
	}
	switch request.Kind {
	case "soul":
		soulEvent := magicevent.NewEventWithContext(soulevents.TopicSoulCommandUpgradeBuiltinDefault, "application/workorchd/context-registry", soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.UpgradeBuiltinSoulDefaultCommand{SoulID: common.SoulID(request.ID), ExpectedContentHash: request.ExpectedContentHash, TargetContentHash: request.TargetContentHash})
		ownerevent.BindLane(soulEvent, soulevents.Contracts(), soulEvent.ID(), "soul", request.ID, "", "", "")
		result := hub.Send(soulEvent)
		if result == nil {
			return ret, fmt.Errorf("Soul builtin upgrade returned no result")
		}
		data, err := result.Get()
		if err != nil {
			return ret, err
		}
		value, ok := data.(soulevents.UpgradeBuiltinSoulDefaultResult)
		if !ok {
			return ret, fmt.Errorf("Soul builtin upgrade returned invalid result")
		}
		ret.Default, ret.Upgraded = builtinSoulView(value.Default), value.Upgraded
	case "workspace_template":
		templateEvent := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultCommand{TemplateID: request.ID, ExpectedContentHash: request.ExpectedContentHash, TargetContentHash: request.TargetContentHash})
		ownerevent.BindLane(templateEvent, workspaceevents.Contracts(), templateEvent.ID(), "workspace_template", request.ID, "", "", "")
		result := hub.Send(templateEvent)
		if result == nil {
			return ret, fmt.Errorf("Workspace builtin upgrade returned no result")
		}
		data, err := result.Get()
		if err != nil {
			return ret, err
		}
		value, ok := data.(workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult)
		if !ok {
			return ret, fmt.Errorf("Workspace builtin upgrade returned invalid result")
		}
		ret.Default, ret.Upgraded = builtinTemplateView(value.Default), value.Upgraded
	default:
		return ret, common.ErrInvalidArgument
	}
	if err := validateBuiltinDefaultView(ret.Default); err != nil {
		return workorchmanagementcontract.BuiltinDefaultUpgradeResult{}, err
	}
	if ret.Default.ID != request.ID || !ret.Default.Present || ret.Default.CurrentContentHash != request.TargetContentHash || ret.Default.BuiltinContentHash != request.TargetContentHash || ret.Default.UpgradeAvailable {
		return workorchmanagementcontract.BuiltinDefaultUpgradeResult{}, fmt.Errorf("builtin upgrade result does not match requested resource and target")
	}
	return ret, nil
}

func validateBuiltinDefaultView(value workorchmanagementcontract.BuiltinDefault) error {
	if value.ID == "" || value.ID != strings.TrimSpace(value.ID) || value.BuiltinContentHash == "" || value.BuiltinContentHash != strings.TrimSpace(value.BuiltinContentHash) || value.CurrentContentHash != strings.TrimSpace(value.CurrentContentHash) ||
		value.Present != (value.CurrentContentHash != "") ||
		(!value.Present && value.CurrentBuiltinRevision != "") ||
		(value.UpgradeAvailable && (!value.Present || value.CurrentBuiltinRevision == "" || value.CurrentContentHash == value.BuiltinContentHash)) {
		return fmt.Errorf("invalid builtin resource observation")
	}
	return nil
}

func builtinSoulView(item soulevents.BuiltinSoulDefault) workorchmanagementcontract.BuiltinDefault {
	return workorchmanagementcontract.BuiltinDefault{Kind: "soul", ID: string(item.SoulID), Present: item.Present, CurrentContentHash: item.CurrentContentHash, CurrentBuiltinRevision: item.CurrentBuiltinRevision, BuiltinContentHash: item.BuiltinContentHash, UpgradeAvailable: item.UpgradeAvailable}
}

func builtinTemplateView(item workspaceevents.BuiltinWorkspaceTemplateDefault) workorchmanagementcontract.BuiltinDefault {
	return workorchmanagementcontract.BuiltinDefault{Kind: "workspace_template", ID: item.TemplateID, Present: item.Present, CurrentContentHash: item.CurrentContentHash, CurrentBuiltinRevision: item.CurrentBuiltinRevision, BuiltinContentHash: item.BuiltinContentHash, UpgradeAvailable: item.UpgradeAvailable}
}
