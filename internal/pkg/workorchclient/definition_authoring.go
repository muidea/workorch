package workorchclient

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// CreateDefinitionAuthoring 创建持久草稿，不物化正式定义或启动 Run。
func (c *Client) CreateDefinitionAuthoring(ctx context.Context, request workorchmanagementcontract.DefinitionAuthoringCreate) (*workorchmanagementcontract.DefinitionAuthoringView, error) {
	return c.definitionAuthoringRequest(ctx, "POST", "/api/v1/workorch/workspace-authoring", request)
}

func (c *Client) ReadDefinitionAuthoring(ctx context.Context, workspaceID string) (*workorchmanagementcontract.DefinitionAuthoringView, error) {
	return c.definitionAuthoringRequest(ctx, "GET", authoringPath(workspaceID), nil)
}

func (c *Client) SaveDefinitionAuthoring(ctx context.Context, workspaceID string, request workorchmanagementcontract.DefinitionAuthoringSave) (*workorchmanagementcontract.DefinitionAuthoringView, error) {
	return c.definitionAuthoringRequest(ctx, "PUT", authoringPath(workspaceID), request)
}

// CompileDefinitionAuthoring 的接受回执不是持久编译成功状态。
func (c *Client) CompileDefinitionAuthoring(ctx context.Context, workspaceID string) (*workorchmanagementcontract.DefinitionAuthoringView, error) {
	return c.definitionAuthoringRequest(ctx, "POST", authoringPath(workspaceID)+"/compile", nil)
}

func (c *Client) ConfirmDefinitionAuthoring(ctx context.Context, workspaceID string, request workorchmanagementcontract.DefinitionAuthoringConfirm) (*workorchmanagementcontract.DefinitionAuthoringView, error) {
	return c.definitionAuthoringRequest(ctx, "POST", authoringPath(workspaceID)+"/confirm", request)
}

func authoringPath(workspaceID string) string {
	return "/api/v1/workorch/workspaces/" + escapePath(workspaceID) + "/authoring"
}

func (c *Client) definitionAuthoringRequest(ctx context.Context, method, path string, request any) (*workorchmanagementcontract.DefinitionAuthoringView, error) {
	response, err := c.do(ctx, method, path, request)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.DefinitionAuthoringView](response, "authoring")
}
