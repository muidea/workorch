package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (p *runtimeApplicationPorts) configureRegistryPorts() {
	ports := &p.ports
	ports.Registry.ListBuiltinDefaults = func(ctx context.Context, _, _ string) ([]workorchmanagementcontract.BuiltinDefault, error) {
		var ret []workorchmanagementcontract.BuiltinDefault
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListBuiltinDefaultsWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Registry.UpgradeBuiltinDefault = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.BuiltinDefaultUpgrade) (workorchmanagementcontract.BuiltinDefaultUpgradeResult, error) {
		var ret workorchmanagementcontract.BuiltinDefaultUpgradeResult
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpgradeBuiltinDefaultWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.CreateSoul = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error) {
		var ret workorchmanagementcontract.Soul
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateSoulWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.UpdateSoul = func(ctx context.Context, _, _ string, soulID string, cmd workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error) {
		var ret workorchmanagementcontract.Soul
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateSoulWithSession(ctx, session, soulID, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.DeleteSoul = func(ctx context.Context, _, _, soulID string) error {
		return p.withSession(ctx, func(session *runtimecore.Session) error {
			return appservice.DeleteSoulWithSession(ctx, session, soulID)
		})
	}
	ports.Registry.DeleteWorkspaceTemplate = func(ctx context.Context, _, _, templateID string) error {
		return p.withSession(ctx, func(session *runtimecore.Session) error {
			return appservice.DeleteWorkspaceTemplateWithSession(ctx, session, templateID)
		})
	}
	ports.Registry.ReadSoul = func(ctx context.Context, _, _, soulID string) (workorchmanagementcontract.Soul, error) {
		var ret workorchmanagementcontract.Soul
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadSoulWithSession(ctx, session, soulID)
			return err
		})
		return ret, err
	}
	ports.Registry.ListSouls = func(ctx context.Context, _, _ string) ([]workorchmanagementcontract.Soul, error) {
		var ret []workorchmanagementcontract.Soul
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListSoulsWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Registry.CreateWorkspaceTemplate = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
		var ret workorchmanagementcontract.WorkspaceTemplate
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateWorkspaceTemplateWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.UpdateWorkspaceTemplate = func(ctx context.Context, _, _ string, templateID string, cmd workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
		var ret workorchmanagementcontract.WorkspaceTemplate
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateWorkspaceTemplateWithSession(ctx, session, templateID, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.ListWorkspaceTemplates = func(ctx context.Context, _, _ string) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
		var ret []workorchmanagementcontract.WorkspaceTemplate
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListWorkspaceTemplatesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Registry.CreateSkill = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.SkillMutation) (workorchmanagementcontract.Skill, error) {
		var ret workorchmanagementcontract.Skill
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateSkillWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.ReadSkill = func(ctx context.Context, _, _, skillID string, query contextregistrycore.SkillQuery) (workorchmanagementcontract.Skill, error) {
		var ret workorchmanagementcontract.Skill
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadSkillWithSession(ctx, session, skillID, query)
			return err
		})
		return ret, err
	}
	ports.Registry.ListSkills = func(ctx context.Context, _, _ string, query contextregistrycore.SkillQuery) ([]workorchmanagementcontract.Skill, error) {
		var ret []workorchmanagementcontract.Skill
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListSkillsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Registry.CreateMCPServer = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error) {
		var ret workorchmanagementcontract.MCPServer
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateMCPServerWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.UpdateMCPServer = func(ctx context.Context, _, _ string, serverID string, cmd workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error) {
		var ret workorchmanagementcontract.MCPServer
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateMCPServerWithSession(ctx, session, serverID, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.ReadMCPServer = func(ctx context.Context, _, _, serverID string) (workorchmanagementcontract.MCPServer, error) {
		var ret workorchmanagementcontract.MCPServer
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadMCPServerWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	ports.Registry.ListMCPServers = func(ctx context.Context, _, _ string) ([]workorchmanagementcontract.MCPServer, error) {
		var ret []workorchmanagementcontract.MCPServer
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMCPServersWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Registry.ListMCPServerTools = func(ctx context.Context, _, _, serverID string) ([]workorchmanagementcontract.MCPTool, error) {
		var ret []workorchmanagementcontract.MCPTool
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMCPServerToolsWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	ports.Registry.ListMCPServerResources = func(ctx context.Context, _, _, serverID string) ([]workorchmanagementcontract.MCPResource, error) {
		var ret []workorchmanagementcontract.MCPResource
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMCPServerResourcesWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	ports.Registry.ListMCPServerPrompts = func(ctx context.Context, _, _, serverID string) ([]workorchmanagementcontract.MCPPrompt, error) {
		var ret []workorchmanagementcontract.MCPPrompt
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMCPServerPromptsWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	ports.Registry.CreateMCPCapability = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
		var ret workorchmanagementcontract.MCPCapability
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateMCPCapabilityWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.UpdateMCPCapability = func(ctx context.Context, _, _, capabilityID string, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
		var ret workorchmanagementcontract.MCPCapability
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateMCPCapabilityWithSession(ctx, session, capabilityID, cmd)
			return err
		})
		return ret, err
	}
	ports.Registry.ReadMCPCapability = func(ctx context.Context, _, _, capabilityID string) (workorchmanagementcontract.MCPCapability, error) {
		var ret workorchmanagementcontract.MCPCapability
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadMCPCapabilityWithSession(ctx, session, capabilityID)
			return err
		})
		return ret, err
	}
	ports.Registry.ListMCPCapabilities = func(ctx context.Context, _, _ string, query contextregistrycore.MCPCapabilityQuery) ([]workorchmanagementcontract.MCPCapability, error) {
		var ret []workorchmanagementcontract.MCPCapability
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMCPCapabilitiesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
}
