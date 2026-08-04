package workorchclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func (c *Client) ListSouls(ctx context.Context) ([]workorchmanagementcontract.Soul, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/souls", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.Soul](resp, "souls")
}

func (c *Client) GetSoul(ctx context.Context, id string) (*workorchmanagementcontract.Soul, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/souls/%s", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Soul](resp, "soul")
}

func (c *Client) CreateSoul(ctx context.Context, s *workorchmanagementcontract.Soul) (*workorchmanagementcontract.Soul, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/souls", s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Soul](resp, "soul")
}

func (c *Client) UpdateSoul(ctx context.Context, id string, s *workorchmanagementcontract.Soul) (*workorchmanagementcontract.Soul, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/souls/%s", escapePath(id)), s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Soul](resp, "soul")
}

func (c *Client) DeleteSoul(ctx context.Context, id string) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/souls/%s", escapePath(id)), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ListSkills(ctx context.Context, workspaceID string) ([]workorchmanagementcontract.Skill, error) {
	path := "/api/v1/workorch/skills"
	if workspaceID != "" {
		values := url.Values{}
		values.Set("workspaceId", workspaceID)
		path = joinQuery(path, values)
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.Skill](resp, "skills")
}

func (c *Client) GetSkill(ctx context.Context, id, workspaceID string) (*workorchmanagementcontract.Skill, error) {
	path := fmt.Sprintf("/api/v1/workorch/skills/%s", escapePath(id))
	if workspaceID != "" {
		values := url.Values{}
		values.Set("workspaceId", workspaceID)
		path = joinQuery(path, values)
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Skill](resp, "skill")
}

func (c *Client) CreateSkill(ctx context.Context, s *workorchmanagementcontract.Skill) (*workorchmanagementcontract.Skill, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/skills", s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Skill](resp, "skill")
}

func (c *Client) ListMCPServers(ctx context.Context) ([]workorchmanagementcontract.MCPServer, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/mcp/servers", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.MCPServer](resp, "servers")
}

func (c *Client) GetMCPServer(ctx context.Context, id string) (*workorchmanagementcontract.MCPServer, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/mcp/servers/%s", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MCPServer](resp, "server")
}

func (c *Client) CreateMCPServer(ctx context.Context, s *workorchmanagementcontract.MCPServer) (*workorchmanagementcontract.MCPServer, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/mcp/servers", s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MCPServer](resp, "server")
}

func (c *Client) UpdateMCPServer(ctx context.Context, id string, s *workorchmanagementcontract.MCPServer) (*workorchmanagementcontract.MCPServer, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/mcp/servers/%s", escapePath(id)), s)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MCPServer](resp, "server")
}

func (c *Client) ListMCPServerTools(ctx context.Context, serverID string) ([]workorchmanagementcontract.MCPTool, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/mcp/servers/%s/tools", escapePath(serverID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.MCPTool](resp, "tools")
}

func (c *Client) ListMCPCapabilities(ctx context.Context) ([]workorchmanagementcontract.MCPCapability, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/mcp/capabilities", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.MCPCapability](resp, "capabilities")
}

func (c *Client) GetMCPCapability(ctx context.Context, id string) (*workorchmanagementcontract.MCPCapability, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/mcp/capabilities/%s", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MCPCapability](resp, "capability")
}

func (c *Client) CreateMCPCapability(ctx context.Context, cap *workorchmanagementcontract.MCPCapability) (*workorchmanagementcontract.MCPCapability, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/mcp/capabilities", cap)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MCPCapability](resp, "capability")
}

func (c *Client) ListMemory(ctx context.Context, workspaceID string) ([]workorchmanagementcontract.MemoryItem, error) {
	return c.SearchMemory(ctx, workorchmanagementcontract.MemorySearchQuery{WorkspaceID: workspaceID})
}

func (c *Client) SearchMemory(ctx context.Context, query workorchmanagementcontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
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
	return decodeEnvelopeSlice[workorchmanagementcontract.MemoryItem](resp, "memory")
}

func (c *Client) GetMemory(ctx context.Context, workspaceID string, memoryID string) (*workorchmanagementcontract.MemoryItem, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/memory/%s", escapePath(workspaceID), escapePath(memoryID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MemoryItem](resp, "memory")
}

func (c *Client) CreateMemory(ctx context.Context, workspaceID string, item *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/memory", escapePath(workspaceID)), item)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MemoryItem](resp, "memory")
}

func (c *Client) UpdateMemory(ctx context.Context, workspaceID string, memoryID string, item *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/memory/%s", escapePath(workspaceID), escapePath(memoryID)), item)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.MemoryItem](resp, "memory")
}

func (c *Client) DeleteMemory(ctx context.Context, workspaceID string, memoryID string) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/workspaces/%s/memory/%s", escapePath(workspaceID), escapePath(memoryID)), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ListWorkspaceTemplates(ctx context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/workspace-templates", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.WorkspaceTemplate](resp, "workspaceTemplates")
}

func (c *Client) GetWorkspaceTemplate(ctx context.Context, id string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspace-templates/%s", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.WorkspaceTemplate](resp, "workspaceTemplate")
}

func (c *Client) CreateWorkspaceTemplate(ctx context.Context, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/workspace-templates", t)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.WorkspaceTemplate](resp, "workspaceTemplate")
}

func (c *Client) UpdateWorkspaceTemplate(ctx context.Context, id string, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspace-templates/%s", escapePath(id)), t)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.WorkspaceTemplate](resp, "workspaceTemplate")
}

func (c *Client) DeleteWorkspaceTemplate(ctx context.Context, id string) error {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/workspace-templates/%s", escapePath(id)), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) GetConfiguration(ctx context.Context) (*workorchconfigurationcontract.ServiceConfiguration, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/configuration", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchconfigurationcontract.ServiceConfiguration](resp, "configuration")
}

func (c *Client) GetLLMModelCatalog(ctx context.Context, providerID string) (*workorchmanagementcontract.ProviderCatalogSummary, error) {
	values := url.Values{}
	if strings.TrimSpace(providerID) != "" {
		values.Set("providerId", strings.TrimSpace(providerID))
	}
	resp, err := c.do(ctx, "GET", joinQuery("/api/v1/workorch/configuration/llm/models", values), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.ProviderCatalogSummary](resp, "catalog")
}

func (c *Client) RefreshLLMModelCatalog(ctx context.Context, providerID string) (*workorchmanagementcontract.ProviderCatalogSummary, error) {
	values := url.Values{}
	if strings.TrimSpace(providerID) != "" {
		values.Set("providerId", strings.TrimSpace(providerID))
	}
	resp, err := c.do(ctx, "POST", joinQuery("/api/v1/workorch/configuration/llm/models/refresh", values), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.ProviderCatalogSummary](resp, "catalog")
}

func (c *Client) ListEvents(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
	path := "/api/v1/workorch/events"
	values := url.Values{}
	if query.WorkspaceID != "" {
		values.Set("workspaceId", string(query.WorkspaceID))
	}
	if query.RunID != "" {
		values.Set("runId", string(query.RunID))
	}
	if query.Topic != "" {
		values.Set("topic", query.Topic)
	}
	for _, topic := range query.ExcludeTopics {
		if topic = strings.TrimSpace(topic); topic != "" {
			values.Add("excludeTopic", topic)
		}
	}
	if query.SubjectType != "" {
		values.Set("subjectType", query.SubjectType)
	}
	if query.SubjectID != "" {
		values.Set("subjectId", query.SubjectID)
	}
	if query.CursorAfterID != "" {
		values.Set("cursorAfterId", string(query.CursorAfterID))
	}
	if query.OccurredAfter != "" {
		values.Set("occurredAfter", query.OccurredAfter)
	}
	if query.OccurredBefore != "" {
		values.Set("occurredBefore", query.OccurredBefore)
	}
	if query.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", query.Limit))
	}
	if query.IncludePayload {
		values.Set("includePayload", "true")
	}
	path = joinQuery(path, values)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[projectioncontract.EventRecord](resp, "eventRecords")
}

// GetWorkspaceSoulBindings returns the soul bindings for a workspace.
func (c *Client) GetWorkspaceSoulBindings(ctx context.Context, workspaceID string) (*workorchmanagementcontract.WorkspaceSoulBindings, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/soul-bindings", escapePath(workspaceID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.WorkspaceSoulBindings](resp, "soulBindings")
}

// UpdateWorkspaceSoulBindings updates the soul bindings for a workspace.
func (c *Client) UpdateWorkspaceSoulBindings(ctx context.Context, workspaceID string, b *workorchmanagementcontract.WorkspaceSoulBindings) (*workorchmanagementcontract.WorkspaceSoulBindings, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/soul-bindings", escapePath(workspaceID)), b)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.WorkspaceSoulBindings](resp, "soulBindings")
}
