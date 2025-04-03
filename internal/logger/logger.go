package logger

import (
	"log/slog"
	"os"
)

func SetupLogger(component string) {
	logLevel := slog.LevelInfo
	if debugEnv := os.Getenv("DEBUG"); debugEnv != "" {
		logLevel = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})

	logger := slog.New(handler)
	logger = logger.With("component", component)
	slog.SetDefault(logger)

}
