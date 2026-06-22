package workorchport

import "testing"

func TestCanonicalRuntimeBlockKind(t *testing.T) {
	cases := []struct {
		name string
		kind string
		want string
	}{
		{name: "workspace plan", kind: StepKindWorkspacePlan, want: StepKindWorkspaceExecution},
		{name: "legacy patch request", kind: "patchRequests", want: RuntimeBlockKindPatchApply},
		{name: "patch request", kind: "patch/request", want: RuntimeBlockKindPatchApply},
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
