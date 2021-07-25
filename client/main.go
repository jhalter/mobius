package main

import (
	"context"
	"flag"
	"fmt"
	hotline "github.com/jhalter/mobius"
	"github.com/rivo/tview"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//var defaultTrackerList = []string{
//	"hltracker.com:5498",
//}

const connectTimeout = 3 * time.Second

func main() {
	_, cancelRoot := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	version := flag.Bool("version", false, "print version and exit")
	logLevel := flag.String("log-level", "info", "Log level")
	userName := flag.String("name", "unnamed", "User name")
	//srvAddr := flag.String("server", "localhost:5500", "Hostname/Port of server")
	//login := flag.String("login", "guest", "Login Name")
	//pass := flag.String("password", "", "Login Password")
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

	cores := []zapcore.Core{
		newDebugCore(zapLvl, db),
		//newStderrCore(zapLvl),
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

	client := hotline.NewClient(*userName, logger)
	client.DebugBuf = db
	client.UI.Start()

}

func newDebugCore(level zapcore.Level, db *hotline.DebugBuffer) zapcore.Core {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(db),
		level,
	)
}

func newStderrCore(level zapcore.Level) zapcore.Core {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stderr),
		level,
	)
}

var zapLogLevel = map[string]zapcore.Level{
	"debug": zap.DebugLevel,
	"info":  zap.InfoLevel,
	"warn":  zap.WarnLevel,
	"error": zap.ErrorLevel,
}
