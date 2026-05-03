package app

import (
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
)

const interactiveLogFileName = "workorch.log"

func installInteractiveLogging(configDir string) func() {
	previousSlog := slog.Default()
	previousLogWriter := stdlog.Writer()
	previousLogFlags := stdlog.Flags()
	previousLogPrefix := stdlog.Prefix()

	writer, closer := interactiveLogWriter(configDir)
	slog.SetDefault(slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})))
	stdlog.SetOutput(writer)

	return func() {
		slog.SetDefault(previousSlog)
		stdlog.SetOutput(previousLogWriter)
		stdlog.SetFlags(previousLogFlags)
		stdlog.SetPrefix(previousLogPrefix)
		if closer != nil {
			_ = closer.Close()
		}
	}
}

func interactiveLogWriter(configDir string) (io.Writer, io.Closer) {
	if configDir == "" {
		return io.Discard, nil
	}
	logDir := filepath.Join(configDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return io.Discard, nil
	}
	logPath := filepath.Join(logDir, interactiveLogFileName)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return io.Discard, nil
	}
	return file, file
}
