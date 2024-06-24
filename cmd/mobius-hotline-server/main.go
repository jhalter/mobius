package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed mobius/config
var cfgTemplate embed.FS

const defaultPort = 5500

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"error": slog.LevelError,
}

// Values swapped in by go-releaser at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, _ := context.WithCancel(context.Background())

	// TODO: implement graceful shutdown by closing context
	//c := make(chan os.Signal, 1)
	//signal.Notify(c, os.Interrupt)
	//defer func() {
	//	signal.Stop(c)
	//	cancel()
	//}()
	//go func() {
	//	select {
	//	case <-c:
	//		cancel()
	//	case <-ctx.Done():
	//	}
	//}()

	netInterface := flag.String("interface", "", "IP addr of interface to listen on.  Defaults to all interfaces.")
	basePort := flag.Int("bind", defaultPort, "Base Hotline server port.  File transfer port is base port + 1.")
	statsPort := flag.String("stats-port", "", "Enable stats HTTP endpoint on address and port")
	configDir := flag.String("config", defaultConfigPath(), "Path to config root")
	printVersion := flag.Bool("version", false, "print version and exit")
	logLevel := flag.String("log-level", "info", "Log level")
	logFile := flag.String("log-file", "", "Path to log file")

	init := flag.Bool("init", false, "Populate the config dir with default configuration")

	flag.Parse()

	if *printVersion {
		fmt.Printf("mobius-hotline-server %s, commit %s, built at %s", version, commit, date)
		os.Exit(0)
	}

	slogger := slog.New(
		slog.NewTextHandler(
			io.MultiWriter(os.Stdout, &lumberjack.Logger{
				Filename:   *logFile,
				MaxSize:    100, // MB
				MaxBackups: 3,
				MaxAge:     365, // days
			}),
			&slog.HandlerOptions{Level: logLevels[*logLevel]},
		),
	)

	if *init {
		if _, err := os.Stat(filepath.Join(*configDir, "/config.yaml")); os.IsNotExist(err) {
			if err := os.MkdirAll(*configDir, 0750); err != nil {
				slogger.Error(fmt.Sprintf("error creating config dir: %s", err))
				os.Exit(1)
			}

			if err := copyDir("mobius/config", *configDir); err != nil {
				slogger.Error(fmt.Sprintf("error copying config dir: %s", err))
				os.Exit(1)
			}
			slogger.Info("Config dir initialized at " + *configDir)
		} else {
			slogger.Info("Existing config dir found.  Skipping initialization.")
		}
	}

	if _, err := os.Stat(*configDir); os.IsNotExist(err) {
		slogger.Error("Configuration directory not found.  Correct the path or re-run with -init to generate initial config.")
		os.Exit(1)
	}

	srv, err := hotline.NewServer(*configDir, *netInterface, *basePort, slogger, &hotline.OSFileStore{})
	if err != nil {
		slogger.Error(fmt.Sprintf("Error starting server: %s", err))
		os.Exit(1)
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

	slogger.Info("Hotline server started",
		"version", version,
		"API port", fmt.Sprintf("%s:%v", *netInterface, *basePort),
		"Transfer port", fmt.Sprintf("%s:%v", *netInterface, *basePort+1),
	)

	// Serve Hotline requests until program exit
	log.Fatal(srv.ListenAndServe(ctx))
}

type statHandler struct {
	hlServer *hotline.Server
}

func (sh *statHandler) RenderStats(w http.ResponseWriter, _ *http.Request) {
	u, err := json.Marshal(sh.hlServer.CurrentStats())
	if err != nil {
		panic(err)
	}

	_, _ = io.WriteString(w, string(u))
}

func defaultConfigPath() string {
	var cfgPath string

	switch runtime.GOOS {
	case "windows":
		cfgPath = "config/"
	case "darwin":
		if _, err := os.Stat("/usr/local/var/mobius/config/"); err == nil {
			cfgPath = "/usr/local/var/mobius/config/"
		} else if _, err := os.Stat("/opt/homebrew/var/mobius/config"); err == nil {
			cfgPath = "/opt/homebrew/var/mobius/config/"
		}
	case "linux":
		cfgPath = "/usr/local/var/mobius/config/"
	default:
		cfgPath = "./config/"
	}

	return cfgPath
}

// copyFile copies a file from src to dst. If dst does not exist, it is created.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

// copyDir recursively copies a directory tree, attempting to preserve permissions.
func copyDir(src, dst string) error {
	entries, err := cfgTemplate.ReadDir(src)
	if err != nil {
		return err
	}
	for _, dirEntry := range entries {
		if dirEntry.IsDir() {
			if err := os.MkdirAll(filepath.Join(dst, dirEntry.Name()), 0777); err != nil {
				panic(err)
			}
			subdirEntries, _ := cfgTemplate.ReadDir(filepath.Join(src, dirEntry.Name()))
			for _, subDirEntry := range subdirEntries {
				f, err := os.Create(filepath.Join(dst, dirEntry.Name(), subDirEntry.Name()))
				if err != nil {
					return err
				}

				srcFile, err := cfgTemplate.Open(filepath.Join(src, dirEntry.Name(), subDirEntry.Name()))
				if err != nil {
					return err
				}
				_, err = io.Copy(f, srcFile)
				if err != nil {
					return err
				}
				f.Close()
			}
		} else {
			f, err := os.Create(filepath.Join(dst, dirEntry.Name()))
			if err != nil {
				return err
			}

			srcFile, err := cfgTemplate.Open(filepath.Join(src, dirEntry.Name()))
			if err != nil {
				return err
			}
			_, err = io.Copy(f, srcFile)
			if err != nil {
				return err
			}
			f.Close()
		}
	}

	return nil
}
