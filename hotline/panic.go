package hotline

import (
	"log/slog"
	"runtime/debug"
)

// dontPanic recovers from a panic and logs it (with a stack trace) instead of
// letting it crash the goroutine. The trace is recorded via the structured
// logger only; it is intentionally not written to stdout, so a client that can
// repeatedly trigger a panic cannot flood stdout.
func dontPanic(logger *slog.Logger) {
	if r := recover(); r != nil {
		logger.Error("PANIC", "err", r, "trace", string(debug.Stack()))
	}
}
