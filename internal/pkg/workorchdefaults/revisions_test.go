package workorchdefaults

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestReviewedBuiltinRevisionsAreExactBoundedContentAllowlist(t *testing.T) {
	if len(reviewedBuiltinRevisions) == 0 || len(reviewedBuiltinRevisions) > 200 {
		t.Fatal("unbounded/empty reviewed defaults")
	}
	seen := map[string]bool{}
	for _, item := range reviewedBuiltinRevisions {
		key := item.kind + "/" + item.id + "/" + item.contentHash
		if seen[key] {
			t.Fatalf("duplicate reviewed content %s", key)
		}
		seen[key] = true
		prefix := "soul-content-sha256:"
		if item.kind == "workspace_template" {
			prefix = "workspace-template-content-sha256:"
		} else if item.kind != "soul" {
			t.Fatalf("unknown kind %q", item.kind)
		}
		digest, err := hex.DecodeString(strings.TrimPrefix(item.contentHash, prefix))
		if err != nil || len(digest) != 32 || len(item.revision) != 8 {
			t.Fatalf("invalid fingerprint metadata: %+v", item)
		}
		if KnownBuiltinContentRevision(item.kind, item.id, item.contentHash) != item.revision {
			t.Fatalf("reviewed content not recognized: %+v", item)
		}
		if KnownBuiltinContentRevision(item.kind, "other-"+item.id, item.contentHash) != "" || KnownBuiltinContentRevision(item.kind, item.id, item.contentHash+" ") != "" {
			t.Fatal("non-exact content or identity accepted")
		}
	}
}
