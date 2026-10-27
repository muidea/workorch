package events

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestExecutionRecoveryRequiresExactPublicIdentityAndCannotSupplyBootstrap(t *testing.T) {
	c := RecoverWorkspaceExecutionIntentCommand{WorkspaceID: "ws", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart}
	if c.Validate() != nil {
		t.Fatal("valid identity rejected")
	}
	for _, scenario := range []string{"scope", "path", "request", "oversize", "utf8", "resume"} {
		v := c
		switch scenario {
		case "scope":
			v.WorkspaceID = " ws "
		case "path":
			v.WorkspaceID = "../ws"
		case "request":
			v.RequestID = " original "
		case "oversize":
			v.RequestID = strings.Repeat("x", 257)
		case "utf8":
			v.RequestID = string([]byte{0xff})
		case "resume":
			v.Kind = runtimecontract.RuntimeExecutionIntentResume
		}
		if v.Validate() == nil {
			t.Fatal("invalid original identity", scenario)
		}
	}
	r := RecoverWorkspaceExecutionIntentResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind}
	if r.ValidateFor(c) != nil {
		t.Fatal("absence rejected")
	}
	r.RequestID = "other"
	if r.ValidateFor(c) == nil {
		t.Fatal("mismatched response accepted")
	}
}
