package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"time"
)

const (
	defaultPort = 5500
)

func main() {
	rand.Seed(time.Now().UnixNano())

	ctx, cancelRoot := context.WithCancel(context.Background())

	basePort := flag.Int("bind", defaultPort, "Bind address and port")
	statsPort := flag.String("stats-port", "", "Enable stats HTTP endpoint on address and port")
	configDir := flag.String("config", defaultConfigPath(), "Path to config root")
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

	srv, err := hotline.NewServer(*configDir, "", *basePort, logger, &hotline.OSFileStore{})
	if err != nil {
		logger.Fatal(err)
	}

	sh := statHandler{hlServer: srv}
	if *statsPort != "" {
		http.HandleFunc("/", sh.RenderStats)

		go func(srv *hotline.Server) {
			// Use the default DefaultServeMux.
			err = http.ListenAndServe(":"+*statsPort, nil)
			if err != nil {
				log.Fatal(err)
			}
		}(srv)
	}

	// Serve Hotline requests until program exit
	logger.Fatal(srv.ListenAndServe(ctx, cancelRoot))
}

type statHandler struct {
	hlServer *hotline.Server
}

func (sh *statHandler) RenderStats(w http.ResponseWriter, _ *http.Request) {
	u, err := json.Marshal(sh.hlServer.Stats)
	if err != nil {
		panic(err)
	}

	_, _ = io.WriteString(w, string(u))
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

func defaultConfigPath() (cfgPath string) {
	switch runtime.GOOS {
	case "windows":
		cfgPath = "config"
	case "darwin":
		if _, err := os.Stat("/usr/local/var/mobius/config/"); err == nil {
			cfgPath = "/usr/local/var/mobius/config/"
		} else if _, err := os.Stat("/opt/homebrew/var/mobius/config"); err == nil {
			cfgPath = "/opt/homebrew/var/mobius/config/"
		}
	case "linux":
		cfgPath = "/usr/local/var/mobius/config/"
	default:
		fmt.Printf("unsupported OS")
	}

	return cfgPath
}
