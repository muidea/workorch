package web

import "testing"

func TestBoundWebContentPreservesProviderFlagAndByteCeiling(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		maxBytes          int64
		providerTruncated bool
		want              string
		wantTruncated     bool
	}{
		{name: "within bound", content: "ready", maxBytes: 5, want: "ready"},
		{name: "provider truncated", content: "ready", maxBytes: 5, providerTruncated: true, want: "ready", wantTruncated: true},
		{name: "owner bound", content: "abcdef", maxBytes: 4, want: "abcd", wantTruncated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, truncated := boundWebContent(test.content, test.maxBytes, test.providerTruncated)
			if got != test.want || truncated != test.wantTruncated {
				t.Fatalf("bound content = %q/%t, want %q/%t", got, truncated, test.want, test.wantTruncated)
			}
		})
	}
}
