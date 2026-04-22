package appservice

import (
	"context"
	"testing"
)

func TestRuntimeSessionCloseReleasesManagedAndBootstrapRuntime(t *testing.T) {
	session := NewRuntimeSessionWithBackend("memory", "")
	if session.Bootstrap == nil {
		t.Fatal("expected bootstrap runtime")
	}
	if session.Runtime == nil {
		t.Fatal("expected managed runtime")
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("close runtime session: %v", err)
	}
	if session.Bootstrap != nil {
		t.Fatal("expected bootstrap runtime to be released")
	}
	if session.Runtime != nil {
		t.Fatal("expected managed runtime to be released")
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("second close runtime session: %v", err)
	}
}
