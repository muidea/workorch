package workorchport

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNormalizeRunCommandIntent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   RunCommandIntent
		want    RunCommandIntent
		wantErr bool
	}{
		{name: "default resume", input: "", want: RunCommandIntentResume},
		{name: "resume", input: RunCommandIntentResume, want: RunCommandIntentResume},
		{name: "restart", input: RunCommandIntentRestart, want: RunCommandIntentRestart},
		{name: "replan", input: RunCommandIntentReplan, want: RunCommandIntentReplan},
		{name: "recover is not canonical", input: RunCommandIntent("recover"), wantErr: true},
		{name: "unsupported intent", input: RunCommandIntent("unsupported"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeRunCommandIntent(tc.input)
			if tc.wantErr {
				if !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("err = %v, want invalid argument", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeRunCommandIntent: %v", err)
			}
			if got != tc.want {
				t.Fatalf("intent = %q, want %q", got, tc.want)
			}
		})
	}
}
