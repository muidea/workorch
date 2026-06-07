package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"

type Soul = contextregistrycore.Soul
type Skill = contextregistrycore.Skill
type MCPServer = contextregistrycore.MCPServer
type MCPTool = contextregistrycore.MCPTool
type MCPResource = contextregistrycore.MCPResource
type MCPPrompt = contextregistrycore.MCPPrompt
type MCPCapability = contextregistrycore.MCPCapability
type MCPCapabilityQuery = contextregistrycore.MCPCapabilityQuery

type CreateSoulCommand = Soul
type UpdateSoulCommand = Soul
type CreateSkillCommand = Skill
type CreateMCPServerCommand = MCPServer
type UpdateMCPServerCommand = MCPServer
type CreateMCPCapabilityCommand = MCPCapability
type UpdateMCPCapabilityCommand = MCPCapability
