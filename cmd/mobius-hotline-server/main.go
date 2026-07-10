package main

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/internal/mobius"
	"github.com/oleksandr/bonjour"
	"github.com/redis/go-redis/v9"
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
	fileStoreBackend := flag.String("file-store", "os", "File library storage backend: os (default), memory, or r2 (Cloudflare R2, configured via R2_* env vars)")

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
			slogger.Info("Config dir initialized", "config", *configDir)
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
	}
	if tlsConfig != nil {
		opts = append(opts, hotline.WithTLS(tlsConfig, *tlsPort))
	}

	// Select the file library storage backend. This is the seam a future object-store backend
	// (e.g. Cloudflare R2 / S3) slots into, mirroring the Redis-vs-file selection below: build
	// the concrete FileStore and pass it via hotline.WithFileStore.
	switch *fileStoreBackend {
	case "os", "":
		// Default OSFileStore is set by NewServer; nothing to do beyond resolving FileRoot,
		// which is a host filesystem path for this backend only.  Object-store backends treat
		// FileRoot as a path within the store's own namespace and keep it as configured, so a
		// relative FileRoot yields host-independent object keys.
		if !filepath.IsAbs(config.FileRoot) {
			config.FileRoot = filepath.Join(*configDir, config.FileRoot)
		}
	case "memory":
		opts = append(opts, hotline.WithFileStore(hotline.NewMemFileStore()))
		slogger.Warn("Using in-memory file store; uploaded files are not persisted")
		warnAbsoluteFileRoot(slogger, config.FileRoot)
	case "r2":
		r2Store, err := newR2FileStore(ctx)
		if err != nil {
			slogger.Error("Error configuring Cloudflare R2 file store", "err", err)
			os.Exit(1)
		}
		opts = append(opts, hotline.WithFileStore(r2Store))
		slogger.Info("Using Cloudflare R2 file store", "bucket", os.Getenv("R2_BUCKET"))
		warnAbsoluteFileRoot(slogger, config.FileRoot)
	default:
		slogger.Error("Unknown file-store backend", "backend", *fileStoreBackend)
		os.Exit(1)
	}

	// The config is passed by value, so this must come after the backend selection above, which
	// resolves config.FileRoot for the chosen backend.
	opts = append(opts, hotline.WithConfig(*config))

	srv, err := hotline.NewServer(opts...)
	if err != nil {
		slogger.Error("Error starting server", "err", err)
		os.Exit(1)
	}

	// reloaders collects the storage backends whose state is reloaded on SIGHUP or via the
	// reload API endpoint.
	var reloaders []namedReloader

	messageBoard, err := mobius.NewFlatNews(path.Join(*configDir, "MessageBoard.txt"))
	if err != nil {
		slogger.Error("Error loading message board", "err", err)
		os.Exit(1)
	}
	srv.MessageBoard = messageBoard
	reloaders = append(reloaders, namedReloader{"message board", messageBoard})

	// Initialize ban list - use Redis if configured, otherwise use file-based storage
	var onlineLister mobius.OnlineLister
	if *redisAddr != "" {
		redisClient := redis.NewClient(&redis.Options{
			Addr:     *redisAddr,
			Password: *redisPassword,
			DB:       *redisDB,
		})

		// Verify Redis connection
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slogger.Error("Error connecting to Redis", "err", err)
			os.Exit(1)
		}

		presence := mobius.NewRedisPresenceTracker(redisClient, slogger)
		// Discard stale online state from a previous run.
		if err := presence.Clear(ctx); err != nil {
			slogger.Warn("Failed to clear online users in Redis", "err", err)
		}
		srv.Presence = presence
		onlineLister = presence

		srv.BanList = mobius.NewRedisBanMgr(redisClient, slogger)
		slogger.Debug("Using Redis for ban management", "addr", *redisAddr)
	} else {
		banFile, err := mobius.NewBanFile(path.Join(*configDir, "Banlist.yaml"))
		if err != nil {
			slogger.Error("Error loading ban list", "err", err)
			os.Exit(1)
		}
		srv.BanList = banFile
		// The Redis-backed ban list needs no reload, so only the file-backed one registers.
		reloaders = append(reloaders, namedReloader{"ban list", banFile})
	}

	threadedNews, err := mobius.NewThreadedNewsYAML(path.Join(*configDir, "ThreadedNews.yaml"))
	if err != nil {
		slogger.Error("Error loading news", "err", err)
		os.Exit(1)
	}
	srv.ThreadedNewsMgr = threadedNews
	reloaders = append(reloaders, namedReloader{"threaded news", threadedNews})

	srv.AccountManager, err = mobius.NewYAMLAccountManager(path.Join(*configDir, "Users/"))
	if err != nil {
		slogger.Error("Error loading accounts", "err", err)
		os.Exit(1)
	}

	agreement, err := mobius.NewAgreement(*configDir, "\r")
	if err != nil {
		slogger.Error("Error loading agreement", "err", err)
		os.Exit(1)
	}
	srv.Agreement = agreement
	reloaders = append(reloaders, namedReloader{"agreement", agreement})

	// On reload failure, the previous banner is kept because SetBanner is only called on success.
	reloadBanner := mobius.ReloaderFunc(func() error {
		banner, err := os.ReadFile(path.Join(*configDir, config.BannerFile))
		if err != nil {
			return err
		}
		srv.SetBanner(banner)
		return nil
	})
	if err := reloadBanner.Reload(); err != nil {
		slogger.Error("Error loading banner", "err", err)
		os.Exit(1)
	}
	reloaders = append(reloaders, namedReloader{"banner", reloadBanner})

	reloadFunc := func() {
		for _, item := range reloaders {
			if err := item.reloader.Reload(); err != nil {
				slogger.Error("Error reloading "+item.name, "err", err)
			}
		}
	}

	if *apiAddr != "" {
		sh := mobius.NewAPIServer(srv, onlineLister, reloadFunc, slogger, *apiKey)
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
				// Canceling the context stops ListenAndServe, which unblocks main for a clean exit.
				signal.Stop(sigChan)
				cancel()
			}

		}
	}()

	boundInterface := *netInterface
	if boundInterface == "" {
		boundInterface = "0.0.0.0"
	}
	slogger.Info("Hotline server started", "version", version, "config", *configDir, "interface", boundInterface, "port", *basePort, "fileTransferPort", *basePort+1)
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

	// Serve Hotline requests until shutdown is requested via signal, the shutdown API, or a
	// server error.
	if err := srv.ListenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slogger.Error("Server error", "err", err)
		os.Exit(1)
	}

	slogger.Info("Server shut down")
}

// namedReloader pairs a Reloader with a human-readable name for reload error logging.
type namedReloader struct {
	name     string
	reloader mobius.Reloader
}

// warnAbsoluteFileRoot flags an absolute FileRoot when an object-store backend is selected: the
// path is used verbatim as the key namespace, so host filesystem layout would leak into every key.
func warnAbsoluteFileRoot(logger *slog.Logger, fileRoot string) {
	if filepath.IsAbs(fileRoot) {
		logger.Warn("FileRoot is an absolute path; object keys will embed it verbatim. Use a relative FileRoot for host-independent keys.", "FileRoot", fileRoot)
	}
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

// newR2FileStore builds a Cloudflare R2-backed file store from R2_* environment variables.
//
// Required: R2_BUCKET and credentials (R2_ACCESS_KEY_ID + R2_SECRET_ACCESS_KEY), plus the endpoint,
// given either as R2_ACCOUNT_ID (from which the standard R2 endpoint is derived) or as an explicit
// R2_ENDPOINT. Optional: R2_PREFIX (a key prefix within the bucket) and R2_STAGING_DIR (local temp
// dir for in-progress .incomplete uploads; defaults to <os.TempDir>/mobius-uploads).
func newR2FileStore(ctx context.Context) (hotline.FileStore, error) {
	bucket := os.Getenv("R2_BUCKET")
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY must be set")
	}

	endpoint := os.Getenv("R2_ENDPOINT")
	if endpoint == "" {
		accountID := os.Getenv("R2_ACCOUNT_ID")
		if accountID == "" {
			return nil, errors.New("either R2_ENDPOINT or R2_ACCOUNT_ID must be set")
		}
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	}

	stagingDir := os.Getenv("R2_STAGING_DIR")
	if stagingDir == "" {
		stagingDir = filepath.Join(os.TempDir(), "mobius-uploads")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
	})

	return hotline.NewR2FileStore(client, bucket, os.Getenv("R2_PREFIX"), stagingDir), nil
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
