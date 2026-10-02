package backend

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestRetiredLocalDefinitionWritesNeverSendEvent(t *testing.T) {
	// nil parent 确保拒绝发生在任何 EventHub 依赖检查或发送之前。
	ctx := context.Background()
	calls := []func() error{
		func() error {
			_, err := (&eventHubWorkspaceBackend{}).Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{})
			return err
		},
		func() error { _, err := (&eventHubGoalBackend{}).Define(ctx, "ws", "目标"); return err },
		func() error { _, err := (&eventHubGoalBackend{}).Update(ctx, "ws", "goal", "目标"); return err },
		func() error { _, err := (&eventHubDoDBackend{}).Define(ctx, "ws", []string{"验收"}); return err },
		func() error {
			_, err := (&eventHubDoDBackend{}).Update(ctx, "ws", "dod", []string{"验收"})
			return err
		},
	}
	for index, call := range calls {
		if err := call(); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("退休本地写操作 %d 未在发送前拒绝: %v", index, err)
		}
	}
}

func TestRetiredRemoteDefinitionWritesNeverUseHTTPClient(t *testing.T) {
	// nil client 在误发 HTTP 时必然失败，确保拒绝发生在任何网络调用之前。
	ctx := context.Background()
	if _, err := (&remoteWorkspaceBackend{}).Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{}); err == nil {
		t.Fatal("退休创建未拒绝")
	}
	goal := &remoteGoalBackend{}
	if _, err := goal.Define(ctx, "ws", "目标"); err == nil {
		t.Fatal("退休目标创建未拒绝")
	}
	if _, err := goal.Update(ctx, "ws", "goal", "目标"); err == nil {
		t.Fatal("退休目标修改未拒绝")
	}
	dod := &remoteDoDBackend{}
	if _, err := dod.Define(ctx, "ws", []string{"验收"}); err == nil {
		t.Fatal("退休 DoD 创建未拒绝")
	}
	if _, err := dod.Update(ctx, "ws", "dod", []string{"验收"}); err == nil {
		t.Fatal("退休 DoD 修改未拒绝")
	}
}
