package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/internal/mobius"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"syscall"
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
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, os.Interrupt)

	netInterface := flag.String("interface", "", "IP addr of interface to listen on.  Defaults to all interfaces.")
	basePort := flag.Int("bind", defaultPort, "Base Hotline server port.  File transfer port is base port + 1.")
	statsPort := flag.String("stats-port", "", "Enable stats HTTP endpoint on address and port")
	configDir := flag.String("config", defaultConfigPath(), "Path to config root")
	printVersion := flag.Bool("version", false, "Print version and exit")
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

	// It's important for Windows compatibility to use path.Join and not filepath.Join for the config dir initialization.
	// https://github.com/golang/go/issues/44305
	if *init {
		if _, err := os.Stat(path.Join(*configDir, "/config.yaml")); os.IsNotExist(err) {
			if err := os.MkdirAll(*configDir, 0750); err != nil {
				slogger.Error(fmt.Sprintf("error creating config dir: %s", err))
				os.Exit(1)
			}
			if err := copyDir(path.Join("mobius", "config"), *configDir); err != nil {
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

	config, err := mobius.LoadConfig(path.Join(*configDir, "config.yaml"))
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading config: %v", err))
		os.Exit(1)
	}

	srv, err := hotline.NewServer(*config, *configDir, *netInterface, *basePort, slogger, &hotline.OSFileStore{})
	if err != nil {
		slogger.Error(fmt.Sprintf("Error starting server: %s", err))
		os.Exit(1)
	}

	srv.MessageBoard, err = mobius.NewFlatNews(path.Join(*configDir, "MessageBoard.txt"))
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading message board: %v", err))
		os.Exit(1)
	}

	srv.BanList, err = mobius.NewBanFile(path.Join(*configDir, "Banlist.yaml"))
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading ban list: %v", err))
		os.Exit(1)
	}

	srv.ThreadedNewsMgr, err = mobius.NewThreadedNewsYAML(path.Join(*configDir, "ThreadedNews.yaml"))
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading news: %v", err))
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

	go func() {
		for {
			sig := <-sigChan
			switch sig {
			case syscall.SIGHUP:
				slogger.Info("SIGHUP received.  Reloading configuration.")

				if err := srv.MessageBoard.(*mobius.FlatNews).Reload(); err != nil {
					slogger.Error("Error reloading news", "err", err)
				}

				if err := srv.BanList.(*mobius.BanFile).Load(); err != nil {
					slogger.Error("Error reloading ban list", "err", err)
				}

				if err := srv.ThreadedNewsMgr.(*mobius.ThreadedNewsYAML).Load(); err != nil {
					slogger.Error("Error reloading threaded news list", "err", err)
				}
			default:
				signal.Stop(sigChan)
				cancel()
				os.Exit(0)
			}

		}
	}()

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

// copyDir recursively copies a directory tree, attempting to preserve permissions.
func copyDir(src, dst string) error {
	entries, err := cfgTemplate.ReadDir(src)
	if err != nil {
		return err
	}
	for _, dirEntry := range entries {
		if dirEntry.IsDir() {
			if err := os.MkdirAll(path.Join(dst, dirEntry.Name()), 0777); err != nil {
				panic(err)
			}
			subdirEntries, _ := cfgTemplate.ReadDir(path.Join(src, dirEntry.Name()))
			for _, subDirEntry := range subdirEntries {
				f, err := os.Create(path.Join(dst, dirEntry.Name(), subDirEntry.Name()))
				if err != nil {
					return err
				}

				srcFile, err := cfgTemplate.Open(path.Join(src, dirEntry.Name(), subDirEntry.Name()))
				if err != nil {
					return fmt.Errorf("error copying srcFile: %w", err)
				}
				_, err = io.Copy(f, srcFile)
				if err != nil {
					return err
				}
				f.Close()
			}
		} else {
			f, err := os.Create(path.Join(dst, dirEntry.Name()))
			if err != nil {
				return err
			}

			srcFile, err := cfgTemplate.Open(path.Join(src, dirEntry.Name()))
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
