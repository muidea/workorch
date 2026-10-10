package workspace

import (
	"strings"
	"testing"
)

func TestDefinitionAuthoringSaveReceiptRejectsMalformedHashes(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := WorkspaceTemplate{ID: "t"}
	valid := DefinitionAuthoringSaveReceipt{RequestID: "save", RequestHash: DefinitionAuthoringSaveRequestHash("save", input, 1, "", nil), InputHash: input.InputHash(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template), WorkspaceRevision: 2}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for field, prefix := range map[string]string{"request": "authoring-save-sha256:", "input": "definition-authoring-sha256:", "template": "template-snapshot-sha256:"} {
		for _, malformed := range []string{"invented", prefix + strings.Repeat("0", 63), prefix + strings.Repeat("G", 64), "wrong-prefix:" + strings.Repeat("0", 64)} {
			receipt := valid
			switch field {
			case "request":
				receipt.RequestHash = malformed
			case "input":
				receipt.InputHash = malformed
			case "template":
				receipt.TemplateSnapshotHash = malformed
			}
			if receipt.Validate() == nil {
				t.Fatalf("损坏 %s hash 被接受", field)
			}
		}
	}
}
