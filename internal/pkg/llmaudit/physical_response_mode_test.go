package llmaudit

import "testing"

func TestPhysicalResponseModeIsBoundedAndStreamingIsDerived(t *testing.T) {
	for _, testCase := range []struct {
		mode      PhysicalResponseMode
		streaming bool
	}{
		{mode: PhysicalResponseModeNonStreaming},
		{mode: PhysicalResponseModeStreaming, streaming: true},
	} {
		if err := testCase.mode.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", testCase.mode, err)
		}
		if got := testCase.mode.Streaming(); got != testCase.streaming {
			t.Fatalf("Streaming(%q) = %v, want %v", testCase.mode, got, testCase.streaming)
		}
	}
	for _, invalid := range []PhysicalResponseMode{"", "stream", "true"} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("Validate(%q) unexpectedly succeeded", invalid)
		}
	}
}
