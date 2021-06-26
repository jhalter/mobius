package main

import (
	"bitbucket.org/jhalter/hotline"
	"flag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

func main() {
	basePort := flag.Int("bind", 5500, "Bind address and port")
	configDir := flag.String("config", "config/", "Path to config root")
	//logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	cores := []zapcore.Core{newStdoutCore()}
	l := zap.New(zapcore.NewTee(cores...))
	defer func() { _ = l.Sync() }()
	logger := l.Sugar()

	srv, err := hotline.NewServer(*configDir, *basePort, logger)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Fatal(srv.ListenAndServe())
}

func newStdoutCore() zapcore.Core {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		zap.DebugLevel,
	)
}
