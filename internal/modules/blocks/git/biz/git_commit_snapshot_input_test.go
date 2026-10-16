package git

import (
	"bytes"
	"context"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGitSnapshotInputMembershipUsesRawNamesAndExactPathNotSharedBlob(t *testing.T) {
	for _, size := range []int{20, 32} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			id := func(value byte) string { return strings.Repeat(string(value), size*2) }
			entry := func(mode, name, objectID string) []byte {
				raw, err := hex.DecodeString(objectID)
				if err != nil {
					t.Fatal(err)
				}
				return append(append([]byte(mode+" "+name), 0), raw...)
			}
			blob := gitSnapshotObject{Kind: "blob", ID: id('a'), Body: []byte("exact input\x00\n")}
			root := gitSnapshotObject{Kind: "tree", ID: id('b'), Body: bytes.Join([][]byte{
				entry("100644", "unrelated\xff\t\n", blob.ID),
				entry("100755", "-reviewed input.go", blob.ID),
				entry("120000", "link", blob.ID),
			}, nil)}
			record := gitCommitSnapshotRecord{Head: id('c'), RootTree: root.ID, Objects: []gitSnapshotObject{root, blob}}
			files := []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "-reviewed input.go", Content: string(blob.Body), ContentHash: runtimecontract.RuntimeValidationInputContentHash(string(blob.Body))}}
			members, err := gitCommitSnapshotInputMemberships(context.Background(), record, files)
			if err != nil || len(members) != 1 || members[0].BlobID != blob.ID || members[0].TargetRef != files[0].TargetRef {
				t.Fatalf("raw tree membership lost exact name/bytes: %+v %v", members, err)
			}
			parsed, err := parseGitSnapshotTree(root.Body, size)
			if err != nil || parsed[0].Name != "unrelated\xff\t\n" || parsed[0].Mode != "100644" {
				t.Fatal("raw tree parser rewrote non-UTF8/special filename")
			}
			for _, path := range []string{"absent.go", "link", "link/child.go", "-reviewed input.go/child.go"} {
				files[0].TargetRef = path
				if _, err := gitCommitSnapshotInputMemberships(context.Background(), record, files); err == nil {
					t.Fatal("shared blob/nonregular path incorrectly proved requested member")
				}
			}
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := gitCommitSnapshotInputMemberships(cancelled, record, files); err == nil {
				t.Fatal("cancelled input read reported complete membership")
			}
		})
	}
}
