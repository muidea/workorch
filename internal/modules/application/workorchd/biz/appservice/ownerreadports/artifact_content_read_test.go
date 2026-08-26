package ownerreadports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadArtifactContentByResultOwnerCommandValidatesScopeHashAndCopy(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-artifact-content-port")
	runID := common.RunID("run-artifact-content-port")
	uri := "workorch://workspaces/ws-artifact-content-port/artifacts/run-artifact-content-port/result.txt"
	body := []byte("owner content\n")
	hash := sha256.Sum256(body)
	ownerResult := resultevents.ReadArtifactContentResult{
		ArtifactID: "artifact-content-port", WorkspaceID: workspaceID, RunID: runID,
		URI: uri, FileName: "result.txt", ContentType: "text/plain; charset=utf-8",
		Content: body, ContentSHA256: hex.EncodeToString(hash[:]),
	}
	responseValue := ownerResult

	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	observer.Subscribe(resultevents.TopicArtifactCommandReadContent, func(ev magicevent.Event, response magicevent.Result) {
		cmd, ok := ev.Data().(resultevents.ReadArtifactContentCommand)
		if !ok || cmd.WorkspaceID != workspaceID || cmd.RunID != runID || cmd.URI != uri || common.RuntimeWorkspaceIDFromContext(ev.Context()) != workspaceID {
			t.Fatalf("read content command = %#v scope=%q", ev.Data(), common.RuntimeWorkspaceIDFromContext(ev.Context()))
		}
		response.Set(responseValue, nil)
	})

	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	got, err := ReadArtifactContentByResultOwnerCommand(ctx, hub, workspaceID, runID, uri)
	if err != nil {
		t.Fatal(err)
	}
	got.Content[0] = 'X'
	if string(ownerResult.Content) != "owner content\n" {
		t.Fatalf("caller content aliases owner response: %q", ownerResult.Content)
	}

	drifted := ownerResult.Clone()
	drifted.Content[0] = 'X'
	responseValue = drifted
	if _, err := ReadArtifactContentByResultOwnerCommand(ctx, hub, workspaceID, runID, uri); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("hash drift err = %v, want invalid argument", err)
	}

	responseValue = ownerResult.Clone()
	responseValue.RunID = "run-other"
	if _, err := ReadArtifactContentByResultOwnerCommand(ctx, hub, workspaceID, runID, uri); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("scope mismatch err = %v, want conflict", err)
	}
}
