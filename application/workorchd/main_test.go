package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWorkorchdDemoOutputContainsAllPhase1Slices(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run workorchd: %v\noutput:\n%s", err, string(out))
	}

	text := string(out)
	checks := []string{
		"== workorch phase1 observable demo ==",
		"phase1 runnable slice",
		"phase1 decision-blocked-resume slice",
		"phase1 approval-blocked-resume slice",
		"/workspace/snapshot",
		"/run/snapshot",
		"/step/snapshot",
		"/decision/ticket/snapshot",
		"/approval/snapshot",
	}
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("demo output missing %q\noutput:\n%s", want, text)
		}
	}
}
