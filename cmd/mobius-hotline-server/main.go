package main

import (
	"context"
	"crypto/tls"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/internal/mobius"
	"github.com/oleksandr/bonjour"
)

//go:embed mobius/config
var cfgTemplate embed.FS

// Values swapped in by go-releaser at build time
var (
	version = "dev"
	commit  = "none"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, os.Interrupt)

	netInterface := flag.String("interface", "", "IP addr of interface to listen on.  Defaults to all interfaces.")
	basePort := flag.Int("bind", 5500, "Base Hotline server port.  File transfer port is base port + 1.")
	apiAddr := flag.String("api-addr", "", "Enable HTTP API endpoint on address and port")
	apiKey := flag.String("api-key", "", "API key required for HTTP API authentication")
	redisAddr := flag.String("redis-addr", "", "Redis server address for API features")
	redisPassword := flag.String("redis-password", "", "Redis password, if required")
	redisDB := flag.Int("redis-db", 0, "Redis DB number, defaults to 0")
	configDir := flag.String("config", findConfigPath(), "Path to config root")
	printVersion := flag.Bool("version", false, "Print version and exit")
	logLevel := flag.String("log-level", "info", "Log level")
	logFile := flag.String("log-file", "", "Path to log file")
	init := flag.Bool("init", false, "Populate the config dir with default configuration")
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate file")
	tlsKey := flag.String("tls-key", "", "Path to TLS key file")
	tlsPort := flag.Int("tls-port", 5600, "Base TLS port. TLS file transfer port is base + 1.")

	flag.Parse()

	if *printVersion {
		fmt.Printf("mobius-hotline-server version %s, commit %s\n", version, commit)
		os.Exit(0)
	}

	slogger := mobius.NewLogger(logLevel, logFile)

	// It's important for Windows compatibility to use path.Join and not filepath.Join for the config dir initialization.
	// https://github.com/golang/go/issues/44305
	if *init {
		if _, err := os.Stat(path.Join(*configDir, "/config.yaml")); os.IsNotExist(err) {
			if err := os.MkdirAll(*configDir, 0750); err != nil {
				slogger.Error("Error creating config dir", "err", err)
				os.Exit(1)
			}
			if err := copyDir(path.Join("mobius", "config"), *configDir); err != nil {
				slogger.Error("Error copying config dir", "err", err)
				os.Exit(1)
			}
			slogger.Info("Config dir initialized at " + *configDir)
		} else {
			slogger.Info("Existing config dir found.  Skipping initialization.")
		}
	}

	config, err := mobius.LoadConfig(path.Join(*configDir, "config.yaml"))
	if err != nil {
		slogger.Error("Error loading config", "err", err)
		os.Exit(1)
	}

	var tlsConfig *tls.Config
	if *tlsCert != "" && *tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			slogger.Error("Error loading TLS certificate", "err", err)
			os.Exit(1)
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	opts := []hotline.Option{
		hotline.WithInterface(*netInterface),
		hotline.WithLogger(slogger),
		hotline.WithPort(*basePort),
		hotline.WithConfig(*config),
	}
	if tlsConfig != nil {
		opts = append(opts, hotline.WithTLS(tlsConfig, *tlsPort))
	}

	srv, err := hotline.NewServer(opts...)
	if err != nil {
		slogger.Error("Error starting server", "err", err)
		os.Exit(1)
	}

	srv.MessageBoard, err = mobius.NewFlatNews(path.Join(*configDir, "MessageBoard.txt"))
	if err != nil {
		slogger.Error("Error loading message board", "err", err)
		os.Exit(1)
	}

	srv.BanList, err = mobius.NewBanFile(path.Join(*configDir, "Banlist.yaml"))
	if err != nil {
		slogger.Error("Error loading ban list", "err", err)
		os.Exit(1)
	}

	srv.ThreadedNewsMgr, err = mobius.NewThreadedNewsYAML(path.Join(*configDir, "ThreadedNews.yaml"))
	if err != nil {
		slogger.Error("Error loading news", "err", err)
		os.Exit(1)
	}

	srv.AccountManager, err = mobius.NewYAMLAccountManager(path.Join(*configDir, "Users/"))
	if err != nil {
		slogger.Error("Error loading accounts", "err", err)
		os.Exit(1)
	}

	srv.Agreement, err = mobius.NewAgreement(*configDir, "\r")
	if err != nil {
		slogger.Error("Error loading agreement", "err", err)
		os.Exit(1)
	}

	bannerPath := path.Join(*configDir, config.BannerFile)
	srv.Banner, err = os.ReadFile(bannerPath)
	if err != nil {
		slogger.Error("Error loading banner", "err", err)
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
			slogger.Error("Error reloading agreement", "err", err)
		}

		// Let's try to reload the banner
		bannerPath := path.Join(*configDir, config.BannerFile)
		srv.Banner, err = os.ReadFile(bannerPath)
		if err != nil {
			slogger.Error("Error reloading banner", "err", err)
		}
	}

	if *apiAddr != "" {
		sh := mobius.NewAPIServer(srv, reloadFunc, slogger, *apiKey, *redisAddr, *redisPassword, *redisDB)
		go sh.Serve(*apiAddr)
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

	slogger.Info("Hotline server started", "version", version, "config", *configDir)
	if tlsConfig != nil {
		slogger.Info("TLS enabled", "port", *tlsPort, "fileTransferPort", *tlsPort+1)
	}

	// Assign functions to handle specific Hotline transaction types
	mobius.RegisterHandlers(srv)

	if srv.Config.EnableBonjour {
		s, err := bonjour.Register(srv.Config.Name, "_hotline._tcp", "", *basePort, []string{"txtv=1", "app=hotline"}, nil)
		if err != nil {
			slogger.Error("Error registering Hotline server with Bonjour", "err", err)
		}
		defer s.Shutdown()
	}

	// Serve Hotline requests until program exit
	log.Fatal(srv.ListenAndServe(ctx))
}

// findConfigPath searches for an existing config directory from the predefined search order.
// Returns the first directory that exists, or falls back to "config" as the default.
func findConfigPath() string {
	for _, cfgPath := range mobius.ConfigSearchOrder {
		if info, err := os.Stat(cfgPath); err == nil && info.IsDir() {
			return cfgPath
		}
	}

	// Default fallback - will be created by --init flag if needed
	return "config"
}

// copyDir recursively copies a directory tree from embedded filesystem to local filesystem.
func copyDir(src, dst string) error {
	return copyDirRecursive(src, dst)
}

// copyDirRecursive handles the recursive copying logic.
func copyDirRecursive(src, dst string) error {
	entries, err := cfgTemplate.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := path.Join(src, entry.Name())
		dstPath := path.Join(dst, entry.Name())

		if entry.IsDir() {
			// Create directory with proper permissions
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
			
			// Recursively copy subdirectory
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy subdirectory %s: %w", srcPath, err)
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy file %s to %s: %w", srcPath, dstPath, err)
			}
		}
	}

	return nil
}

// copyFile copies a single file from embedded filesystem to local filesystem.
func copyFile(src, dst string) error {
	srcFile, err := cfgTemplate.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	return nil
}
