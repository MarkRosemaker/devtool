package main

import (
	"log/slog"
	"os"
)

// Logs go to stderr because stdout belongs to the run's output. A consumer
// parsing the event stream reads a log line there as an event, and one whose
// keys match a field — repo, coverage, commits — passes for a valid one.

// jsonLogs is whether logging is for a process reading it, set by -json.
var jsonLogs bool

// For a person by default: only what needs their attention, as plain text.
// What a run did is the console's to say, not the log's.
func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return a
		},
	})))
}

// useJSONLogs switches to everything, as JSON, for patchpal: it keeps the
// log in its journal, where a run nobody watched is read afterwards.
func useJSONLogs() {
	jsonLogs = true

	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.SetDefault(slog.New(slog.NewJSONHandler(
		os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug},
	)))
}
