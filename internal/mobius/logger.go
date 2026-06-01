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
	// Always log to stdout. Only add the rotating file writer when a log file path
	// is configured; an empty Filename makes lumberjack write to a temp file.
	var out io.Writer = os.Stdout
	if *logFile != "" {
		out = io.MultiWriter(os.Stdout, &lumberjack.Logger{
			Filename:   *logFile,
			MaxSize:    logMaxSize,
			MaxBackups: logMaxBackups,
			MaxAge:     logMaxAge,
		})
	}

	return slog.New(
		slog.NewTextHandler(
			out,
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
