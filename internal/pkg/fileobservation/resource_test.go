package fileobservation

import "testing"

func TestResourceOriginalBytesCloneAndCompleteValidation(t *testing.T) {
	raw := []byte{0xff, 0, '\r', '\n'}
	r := Resource{Ref: "input", Complete: true, Content: raw, ContentBytes: len(raw), ContentSHA256: Digest(raw), Inode: 1, Mode: 0100600, DiagnosticSHA256: Digest(nil)}
	if r.Validate() != nil {
		t.Fatal("original arbitrary bytes not representable")
	}
	clone := r.Clone()
	clone.Content[0] = 0
	if r.Content[0] != 0xff || clone.Validate() == nil {
		t.Fatal("content aliased or checksum ignored")
	}
	for _, mutate := range []func(*Resource){
		func(r *Resource) { r.ContentBytes-- },
		func(r *Resource) { r.Mode = 0040700 },
		func(r *Resource) { r.Inode = 0 },
		func(r *Resource) { r.ChangedNanos = 1e9 },
		func(r *Resource) { r.Ref = string([]byte{0xff}) },
		func(r *Resource) {
			r.Content = make([]byte, ResourceMaxBytes+1)
			r.ContentBytes = len(r.Content)
			r.ContentSHA256 = Digest(r.Content)
		},
		func(r *Resource) {
			r.Diagnostic = []byte("failure")
			r.DiagnosticBytes = len(r.Diagnostic)
			r.DiagnosticSHA256 = Digest(r.Diagnostic)
		},
	} {
		bad := r.Clone()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("malformed complete resource accepted")
		}
	}
	failure := Resource{Ref: "input", ContentSHA256: Digest(nil), Diagnostic: raw, DiagnosticBytes: len(raw), DiagnosticSHA256: Digest(raw)}
	if failure.Validate() != nil {
		t.Fatal("raw failure diagnostic not preserved")
	}
	for _, mutate := range []func(*Resource){
		func(r *Resource) { r.Content = raw; r.ContentBytes = len(raw); r.ContentSHA256 = Digest(raw) },
		func(r *Resource) { r.Inode = 1 },
		func(r *Resource) {
			r.Diagnostic = make([]byte, DiagnosticLimit+1)
			r.DiagnosticBytes = len(r.Diagnostic)
			r.DiagnosticSHA256 = Digest(r.Diagnostic)
		},
	} {
		bad := failure.Clone()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("partial failure content/version accepted")
		}
	}
	long := make([]byte, DiagnosticLimit+1)
	omitted := failure.Clone()
	omitted.Diagnostic, omitted.DiagnosticBytes, omitted.DiagnosticSHA256, omitted.DiagnosticNotSavedReason = nil, len(long), Digest(long), "diagnostic_exceeds_byte_boundary"
	if omitted.Validate() != nil {
		t.Fatal("explicit oversized diagnostic omission rejected")
	}
	omitted.Complete = true
	if omitted.Validate() == nil {
		t.Fatal("missing diagnostic promoted to complete")
	}
}
