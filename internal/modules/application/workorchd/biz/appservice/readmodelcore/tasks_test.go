package readmodelcore

import (
	"errors"
	"testing"
)

func TestRunTasksReturnsFirstErrorAndSkipsNilTasks(t *testing.T) {
	first := errors.New("first")
	second := errors.New("second")

	got := RunTasks(nil, func() error { return first }, func() error { return second })

	if !errors.Is(got, first) && !errors.Is(got, second) {
		t.Fatalf("RunTasks error = %v, want one task error", got)
	}
}

func TestRunTasksReturnsNilWhenAllTasksPass(t *testing.T) {
	if err := RunTasks(nil, func() error { return nil }); err != nil {
		t.Fatalf("RunTasks err = %v, want nil", err)
	}
}
