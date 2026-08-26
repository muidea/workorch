package content

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestParseArtifactURI(t *testing.T) {
	ref, err := ParseArtifactURI("workorch://workspaces/ws-001/artifacts/run-001/result-summary.txt")
	if err != nil {
		t.Fatalf("ParseArtifactURI err = %v", err)
	}
	if ref.WorkspaceID != "ws-001" || ref.RunID != "run-001" || ref.RelativePath != "result-summary.txt" {
		t.Fatalf("artifact ref = %+v", ref)
	}
	for _, uri := range []string{
		"workorch://workspaces/ws-001/artifacts/run-001/../secret.txt",
		"workorch://workspaces/../artifacts/run-001/secret.txt",
		"workorch://workspaces/ws-001/artifacts/../secret.txt",
	} {
		if _, err := ParseArtifactURI(uri); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("unsafe artifact uri %q err = %v, want invalid argument", uri, err)
		}
	}
}

func TestParseRuntimeRawRefAndContentType(t *testing.T) {
	itemID := "run-001/step-1"
	itemToken := base64.RawURLEncoding.EncodeToString([]byte(itemID))
	ref, err := ParseRuntimeRawRef("workorch://workspaces/ws-001/runs/run-001/steps/" + itemToken + "/output.json")
	if err != nil {
		t.Fatalf("ParseRuntimeRawRef err = %v", err)
	}
	if ref.WorkspaceID != "ws-001" || ref.RunID != "run-001" || ref.Collection != "steps" || ref.ItemID != itemID || ref.FileName != "output.json" {
		t.Fatalf("runtime raw ref = %+v", ref)
	}
	if RuntimeRawContentType(ref.FileName) != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", RuntimeRawContentType(ref.FileName))
	}
	llmToken := base64.RawURLEncoding.EncodeToString([]byte("run-001/llm-1"))
	for _, uri := range []string{
		"workorch://workspaces/ws-001/runs/run-001/llm-calls/" + llmToken + "/request.json",
		"workorch://workspaces/ws-001/runs/run-001/steps/run-001/step-1/secret.txt",
		"workorch://workspaces/ws-001/runs/run-001/steps/not-base64/output.json",
	} {
		if _, err := ParseRuntimeRawRef(uri); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("unsupported runtime ref %q err = %v, want invalid argument", uri, err)
		}
	}
}

func TestRuntimeRawContentIsBoundedCopiedAndHashed(t *testing.T) {
	itemToken := base64.RawURLEncoding.EncodeToString([]byte("run-001/step-1"))
	ref, err := ParseRuntimeRawRef("workorch://workspaces/ws-001/runs/run-001/steps/" + itemToken + "/output.json")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"ok":true}`)
	content, err := newRuntimeRawContent(ref, body)
	if err != nil || len(content.ContentSHA256) != 64 {
		t.Fatalf("content = %+v err=%v", content, err)
	}
	body[0] = 'X'
	if string(content.Content) != `{"ok":true}` {
		t.Fatalf("content aliases source: %q", content.Content)
	}
	if _, err := newRuntimeRawContent(ref, []byte(strings.Repeat("x", MaxRuntimePayloadBytes+1))); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("oversized content err = %v, want conflict", err)
	}
}

func TestContentReadersRejectContextAndURIWorkspaceMismatch(t *testing.T) {
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	reader := NewReader(hub)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-context")

	if _, err := reader.ReadArtifact(ctx, "workorch://workspaces/ws-uri/artifacts/run-1/result.txt"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("artifact workspace mismatch err = %v, want conflict", err)
	}
	itemToken := base64.RawURLEncoding.EncodeToString([]byte("run-1/step-1"))
	runtimeRef := "workorch://workspaces/ws-uri/runs/run-1/steps/" + itemToken + "/output.json"
	if _, err := reader.ReadRuntimeRaw(ctx, runtimeRef); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("runtime workspace mismatch err = %v, want conflict", err)
	}
}
