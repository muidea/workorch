package events

import (
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const (
	TopicWorkspaceTemplateCommandQueryBuiltinDefaults  = "/workspace-template/command/query-builtin-defaults"
	TopicWorkspaceTemplateCommandUpgradeBuiltinDefault = "/workspace-template/command/upgrade-builtin-default"
)

// BuiltinWorkspaceTemplateDefault is a detached owner observation, never upgrade authority.
type BuiltinWorkspaceTemplateDefault struct {
	TemplateID             string
	Present                bool
	CurrentContentHash     string
	CurrentBuiltinRevision string
	BuiltinContentHash     string
	UpgradeAvailable       bool
}

type QueryBuiltinWorkspaceTemplateDefaultsCommand struct{}
type QueryBuiltinWorkspaceTemplateDefaultsResult struct {
	Defaults []BuiltinWorkspaceTemplateDefault
}

type UpgradeBuiltinWorkspaceTemplateDefaultCommand struct {
	TemplateID          string
	ExpectedContentHash string
	TargetContentHash   string
}
type UpgradeBuiltinWorkspaceTemplateDefaultResult struct {
	Default  BuiltinWorkspaceTemplateDefault
	Upgraded bool
}

func builtinDefaultMessageContracts() []eventcontract.MessageContract {
	return []eventcontract.MessageContract{
		eventcontract.CommandTopic[QueryBuiltinWorkspaceTemplateDefaultsCommand, QueryBuiltinWorkspaceTemplateDefaultsResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceTemplateCommandQueryBuiltinDefaults).MessageContract(),
		eventcontract.CommandTopic[UpgradeBuiltinWorkspaceTemplateDefaultCommand, UpgradeBuiltinWorkspaceTemplateDefaultResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandUpgradeBuiltinDefault).MessageContract(),
	}
}
