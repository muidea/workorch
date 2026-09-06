package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const (
	TopicSoulCommandQueryBuiltinDefaults  = "/soul/command/query-builtin-defaults"
	TopicSoulCommandUpgradeBuiltinDefault = "/soul/command/upgrade-builtin-default"
)

// BuiltinSoulDefault is a detached owner observation, never upgrade authority.
type BuiltinSoulDefault struct {
	SoulID                 common.SoulID
	Present                bool
	CurrentContentHash     string
	CurrentBuiltinRevision string
	BuiltinContentHash     string
	UpgradeAvailable       bool
}

type QueryBuiltinSoulDefaultsCommand struct{}
type QueryBuiltinSoulDefaultsResult struct{ Defaults []BuiltinSoulDefault }

type UpgradeBuiltinSoulDefaultCommand struct {
	SoulID              common.SoulID
	ExpectedContentHash string
	TargetContentHash   string
}
type UpgradeBuiltinSoulDefaultResult struct {
	Default  BuiltinSoulDefault
	Upgraded bool
}

func builtinDefaultMessageContracts() []eventcontract.MessageContract {
	return []eventcontract.MessageContract{
		eventcontract.CommandTopic[QueryBuiltinSoulDefaultsCommand, QueryBuiltinSoulDefaultsResult]("soul", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicSoulCommandQueryBuiltinDefaults).MessageContract(),
		eventcontract.CommandTopic[UpgradeBuiltinSoulDefaultCommand, UpgradeBuiltinSoulDefaultResult]("soul", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicSoulCommandUpgradeBuiltinDefault).MessageContract(),
	}
}
