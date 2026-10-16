package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The enclosing query has verified the complete archive graph and object IDs.
// Walk exact raw tree names, never the working tree, Git display output, pathspec
// expansion or a symlink. Every requested file must be a regular tracked blob
// with identical full bytes. A different path with identical content is not proof.
func gitCommitSnapshotInputMemberships(ctx context.Context, record gitCommitSnapshotRecord, files []runtimecontract.RuntimeValidationInputReviewFile) ([]gitevents.RuntimeCommitSnapshotInputMembership, error) {
	return gitCommitSnapshotMemberships(ctx, record, files, true)
}

func gitCommitSnapshotValidationMemberships(ctx context.Context, record gitCommitSnapshotRecord, dependencies runtimecontract.RuntimeOwnerRevisionSet) ([]gitevents.RuntimeCommitSnapshotInputMembership, error) {
	files := make([]runtimecontract.RuntimeValidationInputReviewFile, 0, len(dependencies.Entries))
	for _, entry := range dependencies.Entries {
		files = append(files, runtimecontract.RuntimeValidationInputReviewFile{TargetRef: entry.ResourceID, ContentHash: entry.Revision})
	}
	return gitCommitSnapshotMemberships(ctx, record, files, false)
}

func gitCommitSnapshotMemberships(ctx context.Context, record gitCommitSnapshotRecord, files []runtimecontract.RuntimeValidationInputReviewFile, compareContent bool) ([]gitevents.RuntimeCommitSnapshotInputMembership, error) {
	objects := make(map[string]gitSnapshotObject, len(record.Objects))
	for _, object := range record.Objects {
		objects[object.ID] = object
	}
	result := make([]gitevents.RuntimeCommitSnapshotInputMembership, 0, len(files))
	for _, file := range files {
		id := record.RootTree
		components := strings.Split(file.TargetRef, "/")
		if len(components) > gitSnapshotMaxDepth {
			return nil, fmt.Errorf("Git input membership exceeds complete path depth")
		}
		for index, component := range components {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			tree, found := objects[id]
			if !found || tree.Kind != "tree" {
				return nil, fmt.Errorf("Git input membership has no original parent tree")
			}
			members, err := parseGitSnapshotTree(tree.Body, len(record.Head)/2)
			if err != nil {
				return nil, err
			}
			var matched gitSnapshotTreeMember
			matches := 0
			for _, member := range members {
				if member.Name == component {
					matched, matches = member, matches+1
				}
			}
			if matches != 1 {
				return nil, fmt.Errorf("Git input membership requires exact tracked path %q", file.TargetRef)
			}
			id = matched.ID
			if index < len(components)-1 {
				if matched.Kind != "tree" || matched.Mode != "40000" {
					return nil, fmt.Errorf("Git input membership cannot traverse non-tree path")
				}
				continue
			}
			blob, found := objects[id]
			if !found || blob.Kind != "blob" || (matched.Mode != "100644" && matched.Mode != "100755") ||
				(compareContent && !bytes.Equal(blob.Body, []byte(file.Content))) || runtimecontract.RuntimeValidationInputContentHash(string(blob.Body)) != file.ContentHash {
				return nil, fmt.Errorf("Git input membership requires exact complete regular blob bytes for %q", file.TargetRef)
			}
			result = append(result, gitevents.RuntimeCommitSnapshotInputMembership{TargetRef: file.TargetRef, ContentHash: file.ContentHash, BlobID: id})
		}
	}
	return result, nil
}
