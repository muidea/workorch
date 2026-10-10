package biz

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringSuccessAuditSenderRejectsInvalidRepliesAndClonesRaw(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery)
	raw := []byte("原始成功文档\r\n\x00\xff")
	sum := sha256.Sum256(raw)
	observation := workspace.DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	stored := workspaceevents.QueryAuthoringSuccessAuditResult{WorkspaceID: "ws", PermitID: "permit", CanonicalHash: "canonical", Audit: workspace.DefinitionAuthoringSuccessByteAudit{Document: observation, Wire: observation.Clone()}}
	mode := "valid"
	calls := 0
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery, func(ev magicevent.Event, result magicevent.Result) {
		calls++
		command := ev.Data().(workspaceevents.QueryAuthoringSuccessAuditCommand)
		if command.WorkspaceID != "ws" || command.PermitID != "permit" {
			t.Fatal("发送端改变审计 scope")
		}
		reply := stored
		switch mode {
		case "wrong_type":
			result.Set("untyped", nil)
			return
		case "missing":
			result.Set(nil, nil)
			return
		case "workspace":
			reply.WorkspaceID = "other"
		case "permit":
			reply.PermitID = "other"
		case "canonical":
			reply.CanonicalHash = ""
		case "raw_hash":
			reply.Audit = reply.Audit.Clone()
			reply.Audit.Document.Content[0] ^= 1
		case "incomplete":
			reply.Audit.Document.Complete = false
		case "metadata_only":
			reply.Audit = reply.Audit.Metadata()
		}
		result.Set(reply, nil)
	}); err != nil {
		t.Fatal(err)
	}
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	command := workspaceevents.QueryAuthoringSuccessAuditCommand{WorkspaceID: "ws", PermitID: "permit"}
	for _, value := range []string{"wrong_type", "missing", "workspace", "permit", "canonical", "raw_hash", "incomplete", "metadata_only"} {
		mode = value
		if _, err := b.QueryAuthoringSuccessAudit(ctx, command); err == nil {
			t.Fatalf("非法审计响应被接受：%s", mode)
		}
	}
	mode = "valid"
	result, err := b.QueryAuthoringSuccessAudit(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	result.Audit.Document.Content[0] ^= 1
	view, err := b.ReadDefinitionAuthoringSuccessAuditView(ctx, common.WorkspaceID("ws"), "permit")
	if err != nil || view.Document.ContentBase64 != base64.StdEncoding.EncodeToString(raw) || view.Wire.ContentBase64 != base64.StdEncoding.EncodeToString(raw) {
		t.Fatal("调用方改变了 owner 原文或读取编码")
	}
	before := calls
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := b.QueryAuthoringSuccessAudit(cancelled, command); err == nil {
		t.Fatal("已取消读取仍发送")
	}
	if _, err := b.QueryAuthoringSuccessAudit(ctx, workspaceevents.QueryAuthoringSuccessAuditCommand{}); err == nil || calls != before {
		t.Fatal("非法或已取消读取到达 owner")
	}
}
