package main

import (
	"context"
	"flag"
	"fmt"
	hotline "github.com/jhalter/mobius"
	"github.com/rivo/tview"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	_, cancelRoot := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	version := flag.Bool("version", false, "print version and exit")
	logLevel := flag.String("log-level", "info", "Log level")
	logFile := flag.String("log-file", "", "output logs to file")

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

	// init DebugBuffer
	db := &hotline.DebugBuffer{
		TextView: tview.NewTextView(),
	}

	cores := []zapcore.Core{newZapCore(zapLvl, db)}

	// Add file logger if optional log-file flag was passed
	if *logFile != "" {
		f, err := os.OpenFile(*logFile,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		defer f.Close()
		if err != nil {
			panic(err)
		}
		cores = append(cores, newZapCore(zapLvl, f))
	}

	l := zap.New(zapcore.NewTee(cores...))
	defer func() { _ = l.Sync() }()
	logger := l.Sugar()
	logger.Infow("Started Mobius client", "Version", hotline.VERSION)

	go func() {
		sig := <-sigChan
		logger.Infow("Stopping client", "signal", sig.String())
		cancelRoot()
	}()

	client := hotline.NewClient("", logger)
	client.DebugBuf = db
	client.UI.Start()

}

func newZapCore(level zapcore.Level, syncer zapcore.WriteSyncer) zapcore.Core {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(syncer),
		level,
	)
}

var zapLogLevel = map[string]zapcore.Level{
	"debug": zap.DebugLevel,
	"info":  zap.InfoLevel,
	"warn":  zap.WarnLevel,
	"error": zap.ErrorLevel,
}
