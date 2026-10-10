package workspace

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDefinitionAuthoringCreationClonePreservesPendingAfterImage(t *testing.T) {
	original := DefinitionAuthoringCreation{
		RequestID: "pending-create", InputHash: "creation-hash",
		PendingPersistence: &DefinitionAuthoringCreationPersistence{WorkspaceRevision: 1, DefaultSoulID: "soul", SoulBindings: map[string]common.SoulID{"stage": "worker"}},
	}
	copy := original.Clone()
	copy.PendingPersistence.WorkspaceRevision = 9
	copy.PendingPersistence.SoulBindings["stage"] = "changed"
	if original.PendingPersistence.WorkspaceRevision != 1 || original.PendingPersistence.SoulBindings["stage"] != "worker" {
		t.Fatal("pending creation after-image aliases clone")
	}
	if err := original.Validate(DefinitionAuthoringWorkspaceID(original.RequestID)); err != nil {
		t.Fatal(err)
	}
}
