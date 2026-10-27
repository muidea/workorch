package fileobservation

import "testing"

func TestDirectoryDiagnosticByteFidelityAndBounds(t *testing.T) {
	raw := []byte{0xff, 0, '\r', '\n'}
	d := Directory{Path: "/workspace", Diagnostic: raw, DiagnosticBytes: len(raw), DiagnosticSHA256: Digest(raw)}
	if d.Validate() != nil {
		t.Fatal("arbitrary original bytes rejected")
	}
	clone := d.Clone()
	clone.Diagnostic[0] = 1
	if d.Diagnostic[0] != 0xff || clone.Validate() == nil {
		t.Fatal("bytes alias or hash not verified")
	}
	d.Diagnostic = make([]byte, DiagnosticLimit+1)
	d.DiagnosticBytes = len(d.Diagnostic)
	d.DiagnosticSHA256 = Digest(d.Diagnostic)
	if d.Validate() == nil {
		t.Fatal("oversize diagnostic silently accepted")
	}
	for _, path := range []string{"/", "relative", "/a/../b", "/a\x00b", string([]byte{'/', 0xff})} {
		if ValidPath(path) {
			t.Fatalf("unsafe/non-representable scope: %q", path)
		}
	}
}
