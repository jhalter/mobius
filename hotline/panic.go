package hotline

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// dontPanic logs panics instead of crashing
func dontPanic(logger *slog.Logger) {
	if r := recover(); r != nil {
		fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
		logger.Error("PANIC", "err", r, "trace", string(debug.Stack()))
	}
}
