package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

func (c *Client) ListSouls(ctx context.Context) ([]Soul, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/souls", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Soul](resp, "souls")
}

func (c *Client) GetSoul(ctx context.Context, id string) (*Soul, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/souls/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Soul](resp, "soul")
}

func (c *Client) CreateSoul(ctx context.Context, s *Soul) (*Soul, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/souls", s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Soul](resp, "soul")
}

func (c *Client) UpdateSoul(ctx context.Context, id string, s *Soul) (*Soul, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/souls/%s", id), s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Soul](resp, "soul")
}

func (c *Client) DeleteSoul(ctx context.Context, id string) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/souls/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ListSkills(ctx context.Context, workspaceID string) ([]Skill, error) {
	path := "/api/v1/workorch/skills"
	if workspaceID != "" {
		path += "?workspaceId=" + workspaceID
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Skill](resp, "skills")
}

func (c *Client) GetSkill(ctx context.Context, id, workspaceID string) (*Skill, error) {
	path := fmt.Sprintf("/api/v1/workorch/skills/%s", id)
	if workspaceID != "" {
		path += "?workspaceId=" + workspaceID
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Skill](resp, "skill")
}

func (c *Client) CreateSkill(ctx context.Context, s *Skill) (*Skill, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/skills", s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Skill](resp, "skill")
}

func (c *Client) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/mcp/servers", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[MCPServer](resp, "servers")
}

func (c *Client) GetMCPServer(ctx context.Context, id string) (*MCPServer, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/mcp/servers/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MCPServer](resp, "server")
}

func (c *Client) CreateMCPServer(ctx context.Context, s *MCPServer) (*MCPServer, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/mcp/servers", s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MCPServer](resp, "server")
}

func (c *Client) UpdateMCPServer(ctx context.Context, id string, s *MCPServer) (*MCPServer, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/mcp/servers/%s", id), s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MCPServer](resp, "server")
}

func (c *Client) ListMCPServerTools(ctx context.Context, serverID string) ([]MCPTool, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/mcp/servers/%s/tools", serverID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[MCPTool](resp, "tools")
}

func (c *Client) ListMCPCapabilities(ctx context.Context) ([]MCPCapability, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/mcp/capabilities", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[MCPCapability](resp, "capabilities")
}

func (c *Client) GetMCPCapability(ctx context.Context, id string) (*MCPCapability, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/mcp/capabilities/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MCPCapability](resp, "capability")
}

func (c *Client) CreateMCPCapability(ctx context.Context, cap *MCPCapability) (*MCPCapability, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/mcp/capabilities", cap)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MCPCapability](resp, "capability")
}

func (c *Client) ListMemory(ctx context.Context, workspaceID string) ([]MemoryItem, error) {
	return c.SearchMemory(ctx, MemorySearchQuery{WorkspaceID: workspaceID})
}

func (c *Client) SearchMemory(ctx context.Context, query MemorySearchQuery) ([]MemoryItem, error) {
	values := url.Values{}
	values.Set("workspaceId", query.WorkspaceID)
	if query.Query != "" {
		values.Set("q", query.Query)
	}
	for _, tag := range query.Tags {
		values.Add("tag", tag)
	}
	for _, typ := range query.Types {
		values.Add("type", typ)
	}
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/memory?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[MemoryItem](resp, "memory")
}

func (c *Client) GetMemory(ctx context.Context, memoryID string) (*MemoryItem, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/memory/%s", url.PathEscape(memoryID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MemoryItem](resp, "memory")
}

func (c *Client) CreateMemory(ctx context.Context, workspaceID string, item *MemoryItem) (*MemoryItem, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/memory", url.PathEscape(workspaceID)), item)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MemoryItem](resp, "memory")
}

func (c *Client) UpdateMemory(ctx context.Context, memoryID string, item *MemoryItem) (*MemoryItem, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/memory/%s", url.PathEscape(memoryID)), item)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[MemoryItem](resp, "memory")
}

func (c *Client) DeleteMemory(ctx context.Context, memoryID string) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/memory/%s", url.PathEscape(memoryID)), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ListWorkspaceTemplates(ctx context.Context) ([]WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/workspace-templates", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[WorkspaceTemplate](resp, "workspaceTemplates")
}

func (c *Client) GetWorkspaceTemplate(ctx context.Context, id string) (*WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspace-templates/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[WorkspaceTemplate](resp, "workspaceTemplate")
}

func (c *Client) CreateWorkspaceTemplate(ctx context.Context, t *WorkspaceTemplate) (*WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/workspace-templates", t)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[WorkspaceTemplate](resp, "workspaceTemplate")
}

func (c *Client) UpdateWorkspaceTemplate(ctx context.Context, id string, t *WorkspaceTemplate) (*WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspace-templates/%s", id), t)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[WorkspaceTemplate](resp, "workspaceTemplate")
}

func (c *Client) DeleteWorkspaceTemplate(ctx context.Context, id string) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/workspace-templates/%s", id), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) GetConfiguration(ctx context.Context) (*ServiceConfiguration, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/configuration", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[ServiceConfiguration](resp, "configuration")
}

func (c *Client) ListEvents(ctx context.Context, workspaceID, runID string) ([]EventRecord, error) {
	path := "/api/v1/workorch/events"
	parts := []string{}
	if workspaceID != "" {
		parts = append(parts, "workspaceId="+workspaceID)
	}
	if runID != "" {
		parts = append(parts, "runId="+runID)
	}
	if len(parts) > 0 {
		path += "?"
		for i, p := range parts {
			if i > 0 {
				path += "&"
			}
			path += p
		}
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[EventRecord](resp, "eventRecords")
}

// GetWorkspaceSoulBindings returns the soul bindings for a workspace.
func (c *Client) GetWorkspaceSoulBindings(ctx context.Context, workspaceID string) (*WorkspaceSoulBindings, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/soul-bindings", workspaceID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[WorkspaceSoulBindings](resp, "soulBindings")
}

// UpdateWorkspaceSoulBindings updates the soul bindings for a workspace.
func (c *Client) UpdateWorkspaceSoulBindings(ctx context.Context, workspaceID string, b *WorkspaceSoulBindings) (*WorkspaceSoulBindings, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/soul-bindings", workspaceID), b)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[WorkspaceSoulBindings](resp, "soulBindings")
}
