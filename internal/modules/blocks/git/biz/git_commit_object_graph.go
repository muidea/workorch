package git

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	gitSnapshotMaxObjects = 1024
	gitSnapshotMaxBytes   = 4 * 1024 * 1024
	gitSnapshotMaxDepth   = 64
)

// Body 保留完整 binary object bytes；不能使用 JSON string 保存 tree/blob，
// 因为无效 UTF-8 会被 JSON encoder 替换。此 DTO 仅属于 Git owner。
type gitSnapshotObject struct {
	Kind string
	ID   string
	Body []byte
}

type gitSnapshotTreeMember struct {
	Kind string
	ID   string
	Name string // Raw filename bytes; never normalized or interpreted as argv.
	Mode string
}

// parseGitSnapshotTree consumes the raw Git tree grammar, not ls-tree's
// display rendering. It never interprets filenames as refs, argv or paths.
func parseGitSnapshotTree(body []byte, objectIDBytes int) ([]gitSnapshotTreeMember, error) {
	if objectIDBytes != 20 && objectIDBytes != 32 {
		return nil, fmt.Errorf("Git snapshot tree requires a supported object format")
	}
	members := []gitSnapshotTreeMember{}
	names := map[string]bool{}
	for len(body) > 0 {
		space := bytes.IndexByte(body, ' ')
		if space < 1 {
			return nil, fmt.Errorf("Git snapshot tree has an invalid mode field")
		}
		mode := string(body[:space])
		body = body[space+1:]
		zero := bytes.IndexByte(body, 0)
		if zero < 1 || len(body)-zero-1 < objectIDBytes {
			return nil, fmt.Errorf("Git snapshot tree has an incomplete member")
		}
		name := string(body[:zero])
		if name == "." || name == ".." || strings.Contains(name, "/") || names[name] {
			return nil, fmt.Errorf("Git snapshot tree has an invalid or duplicated name")
		}
		names[name] = true
		kind := "blob"
		switch mode {
		case "40000":
			kind = "tree"
		case "100644", "100755", "120000":
		case "160000":
			// A gitlink names another repository, not a local blob. Never
			// claim the submodule's bytes have been retained or validated.
			return nil, fmt.Errorf("Git snapshot cannot prove submodule contents")
		default:
			return nil, fmt.Errorf("Git snapshot tree has an unsupported member mode")
		}
		members = append(members, gitSnapshotTreeMember{Kind: kind, ID: hex.EncodeToString(body[zero+1 : zero+1+objectIDBytes]), Name: name, Mode: mode})
		body = body[zero+1+objectIDBytes:]
	}
	return members, nil
}

func gitSnapshotRootTree(commit []byte, objectIDLength int) (string, error) {
	first, _, found := bytes.Cut(commit, []byte{'\n'})
	tree, foundTree := strings.CutPrefix(string(first), "tree ")
	if !found || !foundTree || !gitDependencyObjectID(tree) || len(tree) != objectIDLength {
		return "", fmt.Errorf("Git commit dependency has no exact tree identity")
	}
	return tree, nil
}

// readCommitObjectGraph retains exactly the named commit and its complete
// reachable tree/blob graph, not its parent history. Every body independently
// verifies its Git object ID; duplicate content is read once. Any missing,
// unsupported or over-budget member fails the entire capture, never a prefix.
func (a *RuntimeAdapter) readCommitObjectGraph(ctx context.Context, repo, commitID string) ([]gitSnapshotObject, error) {
	if !gitDependencyObjectID(commitID) {
		return nil, fmt.Errorf("Git snapshot requires an exact commit identity")
	}
	timeout := a.timeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	objects := []gitSnapshotObject{}
	seen := map[string]string{}
	heights := map[string]int{}
	byteCount := 0
	var read func(string, string, int) error
	read = func(kind, id string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > gitSnapshotMaxDepth {
			return fmt.Errorf("Git snapshot exceeds complete tree depth limit %d", gitSnapshotMaxDepth)
		}
		if prior, found := seen[id]; found {
			if prior != kind {
				return fmt.Errorf("Git snapshot object has conflicting member kinds")
			}
			if depth+heights[id] > gitSnapshotMaxDepth {
				return fmt.Errorf("Git snapshot exceeds complete tree depth limit %d", gitSnapshotMaxDepth)
			}
			return nil
		}
		if len(objects) >= gitSnapshotMaxObjects {
			return fmt.Errorf("Git snapshot exceeds complete object count limit %d", gitSnapshotMaxObjects)
		}
		body, err := a.readDependencyObject(ctx, repo, kind, id)
		if err != nil {
			return err
		}
		byteCount += len(body)
		if byteCount > gitSnapshotMaxBytes {
			return fmt.Errorf("Git snapshot exceeds complete object bytes limit %d", gitSnapshotMaxBytes)
		}
		seen[id] = kind
		objects = append(objects, gitSnapshotObject{Kind: kind, ID: id, Body: []byte(body)})
		switch kind {
		case "commit":
			tree, err := gitSnapshotRootTree([]byte(body), len(commitID))
			if err != nil {
				return err
			}
			if err := read("tree", tree, depth+1); err != nil {
				return err
			}
			heights[id] = heights[tree] + 1
		case "tree":
			members, err := parseGitSnapshotTree([]byte(body), len(commitID)/2)
			if err != nil {
				return err
			}
			for _, member := range members {
				if err := read(member.Kind, member.ID, depth+1); err != nil {
					return err
				}
				if height := heights[member.ID] + 1; height > heights[id] {
					heights[id] = height
				}
			}
		}
		return nil
	}
	if err := read("commit", commitID, 0); err != nil {
		return nil, err
	}
	return objects, nil
}
