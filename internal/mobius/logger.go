package mobius

import (
	"io"
	"log/slog"
	"os"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logMaxSize    = 100 // MB
	logMaxBackups = 3
	logMaxAge     = 365 // days
)

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"error": slog.LevelError,
}

func NewLogger(logLevel, logFile *string) *slog.Logger {
	return slog.New(
		slog.NewTextHandler(
			io.MultiWriter(os.Stdout, &lumberjack.Logger{
				Filename:   *logFile,
				MaxSize:    logMaxSize,
				MaxBackups: logMaxBackups,
				MaxAge:     logMaxAge,
			}),
			&slog.HandlerOptions{
				Level: logLevels[*logLevel],
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						// Remove the milliseconds from the time field to save a few columns.
						a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339))
					}
					return a
				},
			},
		),
	)
}
