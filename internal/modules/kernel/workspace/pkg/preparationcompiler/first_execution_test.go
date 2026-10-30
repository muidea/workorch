package preparationcompiler

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func TestFirstExecutionInternalDirectoryPreservesBoundaryAcrossOwnerCAS(t *testing.T) {
	original := fileobservation.Directory{Path: "/internal/workspaces/ws", Complete: true,
		Device: 1, Inode: 2, Mode: 0040700, UID: 1001, GID: 1001,
		Readable: true, Writable: true, Searchable: true, DiagnosticSHA256: fileobservation.Digest(nil)}
	current := original.Clone()
	current.ModifiedSeconds, current.ChangedSeconds = 123, 124
	current.ModifiedNanos, current.ChangedNanos = 12, 13
	before := original.Clone()
	if !sameFirstExecutionInternalDirectory(original, current) || !reflect.DeepEqual(original, before) {
		t.Fatal("owner completion CAS timestamps changed directory boundary or mutated source")
	}
	for _, field := range []string{"path", "device", "inode", "mode", "uid", "gid", "read", "write", "search", "incomplete", "invalid_time"} {
		t.Run(field, func(t *testing.T) {
			bad := current.Clone()
			switch field {
			case "path":
				bad.Path += "-other"
			case "device":
				bad.Device++
			case "inode":
				bad.Inode++
			case "mode":
				bad.Mode = 0040755
			case "uid":
				bad.UID++
			case "gid":
				bad.GID++
			case "read":
				bad.Readable = false
			case "write":
				bad.Writable = false
			case "search":
				bad.Searchable = false
			case "incomplete":
				bad.Complete = false
			case "invalid_time":
				bad.ChangedNanos = 1e9
			}
			if sameFirstExecutionInternalDirectory(original, bad) {
				t.Fatal("changed/unobserved directory boundary accepted")
			}
		})
	}
}
