package file

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func TestInitializationDiagnosticsPreserveKnownFailureAtByteBoundary(t *testing.T) {
	for _, raw := range [][]byte{[]byte{0xff, 0, '\r', '\n'}, []byte(strings.Repeat("x", fileobservation.DiagnosticLimit)), []byte(strings.Repeat("y", fileobservation.DiagnosticLimit+1))} {
		d := initializationDiagnostic(errors.New(string(raw)))
		if d.Validate() != nil || !d.Present() || d.Bytes != len(raw) || d.SHA256 != fileobservation.Digest(raw) {
			t.Fatal("lost original error identity")
		}
		if len(raw) <= fileobservation.DiagnosticLimit {
			if !bytes.Equal(d.Raw, raw) || d.NotSavedReason != "" {
				t.Fatal("rewrote diagnostic")
			}
		} else if len(d.Raw) != 0 || d.NotSavedReason != "diagnostic_exceeds_byte_boundary" {
			t.Fatal("truncated or concealed oversized error")
		}
	}
}
