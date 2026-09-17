package main

import (
	"log/slog"
	"os"
)

const minLevel = slog.LevelDebug

// Logs go to stderr because stdout belongs to the event stream.
//
// They used to share stdout, and the sharing was invisible for months: a
// consumer parsing the stream read every log line as an event, and the ones
// whose keys happened to match a field — repo, coverage, commits — became
// valid-looking events with no kind that it silently dropped. The reader now
// refuses a line that is not an event, so the two would fail loudly rather
// than quietly, but the reason they must not share a stream is the same.
func init() {
	slog.SetLogLoggerLevel(minLevel)

	slog.SetDefault(slog.New(slog.NewJSONHandler(
		os.Stderr, &slog.HandlerOptions{Level: minLevel},
	)))
}
