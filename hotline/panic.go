package hotline

import (
	"fmt"
	"go.uber.org/zap"
	"runtime/debug"
)

// dontPanic logs panics instead of crashing
func dontPanic(logger *zap.SugaredLogger) {
	if r := recover(); r != nil {
		fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
		logger.Errorw("PANIC", "err", r, "trace", string(debug.Stack()))
	}
}
