package main

import (
	"log/slog"
	"os"
)

const minLevel = slog.LevelDebug

func init() {
	slog.SetLogLoggerLevel(minLevel)

	// Set the default logger to use a combined handler that writes to both the file and stdout
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: minLevel})))
}
