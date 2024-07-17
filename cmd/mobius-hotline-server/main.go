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
	"path/filepath"
	"syscall"
)

//go:embed mobius/config
var cfgTemplate embed.FS

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
	basePort := flag.Int("bind", 5500, "Base Hotline server port.  File transfer port is base port + 1.")
	statsPort := flag.String("stats-port", "", "Enable stats HTTP endpoint on address and port")
	configDir := flag.String("config", configSearchPaths(), "Path to config root")
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

	config, err := mobius.LoadConfig(path.Join(*configDir, "config.yaml"))
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading config: %v", err))
		os.Exit(1)
	}

	srv, err := hotline.NewServer(
		hotline.WithInterface(*netInterface),
		hotline.WithLogger(slogger),
		hotline.WithPort(*basePort),
		hotline.WithConfig(*config),
	)
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

	srv.AccountManager, err = mobius.NewYAMLAccountManager(filepath.Join(*configDir, "Users/"))
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading accounts: %v", err))
		os.Exit(1)
	}

	srv.Agreement, err = mobius.NewAgreement(*configDir, "\r")
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading agreement: %v", err))
		os.Exit(1)
	}

	bannerPath := filepath.Join(*configDir, config.BannerFile)
	srv.Banner, err = os.ReadFile(bannerPath)
	if err != nil {
		slogger.Error(fmt.Sprintf("Error loading accounts: %v", err))
		os.Exit(1)
	}

	reloadFunc := func() {
		if err := srv.MessageBoard.(*mobius.FlatNews).Reload(); err != nil {
			slogger.Error("Error reloading news", "err", err)
		}

		if err := srv.BanList.(*mobius.BanFile).Load(); err != nil {
			slogger.Error("Error reloading ban list", "err", err)
		}

		if err := srv.ThreadedNewsMgr.(*mobius.ThreadedNewsYAML).Load(); err != nil {
			slogger.Error("Error reloading threaded news list", "err", err)
		}

		if err := srv.Agreement.(*mobius.Agreement).Reload(); err != nil {
			slogger.Error(fmt.Sprintf("Error reloading agreement: %v", err))
			os.Exit(1)
		}
	}

	reloadHandler := func(reloadFunc func()) func(w http.ResponseWriter, _ *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			reloadFunc()

			_, _ = io.WriteString(w, `{ "msg": "config reloaded" }`)
		}
	}

	sh := APIHandler{hlServer: srv}
	if *statsPort != "" {
		http.HandleFunc("/", sh.RenderStats)
		http.HandleFunc("/api/v1/stats", sh.RenderStats)
		http.HandleFunc("/api/v1/reload", reloadHandler(reloadFunc))

		go func(srv *hotline.Server) {
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

				reloadFunc()
			default:
				signal.Stop(sigChan)
				cancel()
				os.Exit(0)
			}

		}
	}()

	slogger.Info("Hotline server started",
		"version", version,
		"config", *configDir,
		"API port", fmt.Sprintf("%s:%v", *netInterface, *basePort),
		"Transfer port", fmt.Sprintf("%s:%v", *netInterface, *basePort+1),
	)

	// Assign functions to handle specific Hotline transaction types
	mobius.RegisterHandlers(srv)

	// Serve Hotline requests until program exit
	log.Fatal(srv.ListenAndServe(ctx))
}

type APIHandler struct {
	hlServer *hotline.Server
}

func (sh *APIHandler) RenderStats(w http.ResponseWriter, _ *http.Request) {
	u, err := json.Marshal(sh.hlServer.CurrentStats())
	if err != nil {
		panic(err)
	}

	_, _ = io.WriteString(w, string(u))
}

func configSearchPaths() string {
	for _, cfgPath := range mobius.ConfigSearchOrder {
		if _, err := os.Stat(cfgPath); err == nil {
			return cfgPath
		}
	}

	return "config"
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
				_ = f.Close()
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
			_ = f.Close()
		}
	}

	return nil
}
