package workorchport

import "testing"

func TestCanonicalRuntimeBlockKind(t *testing.T) {
	cases := []struct {
		name string
		kind string
		want string
	}{
		{name: "workspace execution", kind: StepKindWorkspaceExecution, want: StepKindWorkspaceExecution},
		{name: "unknown kind", kind: "custom/unknown", want: "custom/unknown"},
		{name: "trimmed custom", kind: " shell/command ", want: "shell/command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanonicalRuntimeBlockKind(tc.kind); got != tc.want {
				t.Fatalf("CanonicalRuntimeBlockKind(%q) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}
