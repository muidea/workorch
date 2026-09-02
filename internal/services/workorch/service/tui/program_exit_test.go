package tui

import (
	"context"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestProgramExitCommandQuitsFromHelpOverlay(t *testing.T) {
	model := NewAppModel(&noEventsBE{fakeBE: newFake(t)}, "ws-1", "")
	model.helpOpen = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithoutRenderer(),
		tea.WithoutSignals(),
	)
	type programResult struct {
		model tea.Model
		err   error
	}
	resultCh := make(chan programResult, 1)
	go func() {
		finalModel, err := program.Run()
		resultCh <- programResult{model: finalModel, err: err}
	}()

	if _, err := writer.Write([]byte("/exit\r")); err != nil {
		t.Fatalf("write exit command: %v", err)
	}
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("run Bubble Tea program: %v", result.err)
		}
		if got := asModel(result.model); got.mode != modeQuit {
			t.Fatalf("final mode = %v, want quit", got.mode)
		}
	case <-ctx.Done():
		t.Fatal("Bubble Tea program did not terminate after /exit from help")
	}
}
