package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"math/rand"
	"os"
	"time"
)

const (
	defaultConfigPath = "/usr/local/var/mobius/config/" // matches Homebrew default config location
	defaultPort       = 5500
)

func main() {
	rand.Seed(time.Now().UnixNano())

	ctx, cancelRoot := context.WithCancel(context.Background())

	basePort := flag.Int("bind", defaultPort, "Bind address and port")
	configDir := flag.String("config", defaultConfigPath, "Path to config root")
	version := flag.Bool("version", false, "print version and exit")
	logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	if *version {
		fmt.Printf("v%s\n", hotline.VERSION)
		os.Exit(0)
	}

	zapLvl, ok := zapLogLevel[*logLevel]
	if !ok {
		fmt.Printf("Invalid log level %s.  Must be debug, info, warn, or error.\n", *logLevel)
		os.Exit(0)
	}

	cores := []zapcore.Core{newStdoutCore(zapLvl)}
	l := zap.New(zapcore.NewTee(cores...))
	defer func() { _ = l.Sync() }()
	logger := l.Sugar()

	if _, err := os.Stat(*configDir); os.IsNotExist(err) {
		logger.Fatalw("Configuration directory not found", "path", configDir)
	}

	hotline.FS = &hotline.OSFileStore{}

	srv, err := hotline.NewServer(*configDir, "", *basePort, logger)
	if err != nil {
		logger.Fatal(err)
	}

	// Serve Hotline requests until program exit
	logger.Fatal(srv.ListenAndServe(ctx, cancelRoot))
}

func newStdoutCore(level zapcore.Level) zapcore.Core {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		level,
	)
}

var zapLogLevel = map[string]zapcore.Level{
	"debug": zap.DebugLevel,
	"info":  zap.InfoLevel,
	"warn":  zap.WarnLevel,
	"error": zap.ErrorLevel,
}
