package contextregistryports

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPreparationSkillHubInstructionFIFOIsRejectedWithoutWaiting(t *testing.T) {
	directory, root, runner := preparationInstructionFixture(t, false, "s", []byte("raw"))
	path := filepath.Join(root, "s", "SKILL.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", directory, false)
	if err == nil || value.InstructionsReobserved || len(value.Instructions) != 1 || value.Instructions[0].Complete || time.Since(start) > time.Second {
		t.Fatal("FIFO blocked or credited")
	}
}
