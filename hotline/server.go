package hotline

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/time/rate"
)

type contextKey string

var contextKeyReq = contextKey("req")

type requestCtx struct {
	remoteAddr string
}

type Server struct {
	NetInterface string
	Port         int

	rateLimiters   map[string]*rateLimiterEntry
	rateLimitersMu sync.Mutex

	handlers map[TranType]HandlerFunc

	Config Config
	Logger *slog.Logger

	TrackerPassID [4]byte

	Stats Counter

	FS FileStore // Storage backend to use for File storage

	Agreement io.ReadSeeker

	banner   []byte // server banner image; guarded by bannerMu as it is replaced on config reload
	bannerMu sync.RWMutex

	FileTransferMgr FileTransferMgr
	ChatMgr         ChatManager
	ClientMgr       ClientManager
	AccountManager  AccountManager
	ThreadedNewsMgr ThreadedNewsMgr
	BanList         BanMgr

	MessageBoard io.ReadWriteSeeker

	// Presence optionally records user session lifecycle events for an external online-user
	// list. When nil, online presence is derived from ClientMgr.
	Presence PresenceTracker

	// TrackerRegistrar handles tracker registration (injectable for testing)
	TrackerRegistrar TrackerRegistrar

	TextDecoder *encoding.Decoder
	TextEncoder *encoding.Encoder

	TLSConfig *tls.Config
	TLSPort   int

	shutdownInit sync.Once     // lazily creates shutdownCh so Shutdown works on test-constructed Servers
	shutdownOnce sync.Once     // guards close(shutdownCh)
	shutdownCh   chan struct{} // closed by Shutdown to stop ListenAndServe
}

func (s *Server) initShutdownCh() {
	s.shutdownInit.Do(func() { s.shutdownCh = make(chan struct{}) })
}

// Banner returns the server banner image.  Callers must not modify the returned slice.
func (s *Server) Banner() []byte {
	s.bannerMu.RLock()
	defer s.bannerMu.RUnlock()

	return s.banner
}

// SetBanner replaces the server banner image.
func (s *Server) SetBanner(banner []byte) {
	s.bannerMu.Lock()
	defer s.bannerMu.Unlock()

	s.banner = banner
}

type Option = func(s *Server)

func WithConfig(config Config) func(s *Server) {
	return func(s *Server) {
		s.Config = config
	}
}

func WithLogger(logger *slog.Logger) func(s *Server) {
	return func(s *Server) {
		s.Logger = logger
	}
}

// WithPort optionally overrides the default TCP port.
func WithPort(port int) func(s *Server) {
	return func(s *Server) {
		s.Port = port
	}
}

// WithInterface optionally sets a specific interface to listen on.
func WithInterface(netInterface string) func(s *Server) {
	return func(s *Server) {
		s.NetInterface = netInterface
	}
}

// WithTrackerRegistrar optionally sets a custom tracker registrar (useful for testing).
func WithTrackerRegistrar(registrar TrackerRegistrar) func(s *Server) {
	return func(s *Server) {
		s.TrackerRegistrar = registrar
	}
}

// WithPresenceTracker optionally sets a PresenceTracker to record user session lifecycle events.
func WithPresenceTracker(p PresenceTracker) func(s *Server) {
	return func(s *Server) {
		s.Presence = p
	}
}

// WithFileStore sets the storage backend for the file library. Defaults to OSFileStore.
func WithFileStore(fs FileStore) func(s *Server) {
	return func(s *Server) {
		s.FS = fs
	}
}

// WithTLS optionally enables TLS support on the specified port.
func WithTLS(tlsConfig *tls.Config, port int) func(s *Server) {
	return func(s *Server) {
		s.TLSConfig = tlsConfig
		s.TLSPort = port
	}
}

type ServerConfig struct {
}

func NewServer(options ...Option) (*Server, error) {
	server := Server{
		handlers:         make(map[TranType]HandlerFunc),
		rateLimiters:     make(map[string]*rateLimiterEntry),
		FS:               &OSFileStore{},
		ChatMgr:          NewMemChatManager(),
		ClientMgr:        NewMemClientMgr(),
		FileTransferMgr:  NewMemFileTransferMgr(),
		Stats:            NewStats(),
		TrackerRegistrar: NewRealTrackerRegistrar(),
	}

	for _, opt := range options {
		opt(&server)
	}

	// Initialize text encoding based on config.
	switch server.Config.Encoding {
	case "utf8":
		server.TextDecoder = encoding.Nop.NewDecoder()
		server.TextEncoder = encoding.Nop.NewEncoder()
	default:
		server.TextDecoder = charmap.Macintosh.NewDecoder()
		server.TextEncoder = charmap.Macintosh.NewEncoder()
	}

	// generate a new random passID for tracker registration
	_, err := rand.Read(server.TrackerPassID[:])
	if err != nil {
		return nil, err
	}

	return &server, nil
}

func (s *Server) CurrentStats() StatValues {
	return s.Stats.Values()
}

// ListenAndServe starts the Hotline and file transfer listeners and blocks until the context is
// canceled, Shutdown is called, or a serve loop fails.  Canceling the context closes all
// listeners, which unblocks their accept loops.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Cancel the context when Shutdown is called.
	s.initShutdownCh()
	go func() {
		select {
		case <-s.shutdownCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	go s.registerWithTrackers(ctx)
	go s.keepaliveHandler(ctx)

	errCh := make(chan error, 4)

	listen := func(port int, serve func(context.Context, net.Listener) error) error {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%v", s.NetInterface, port))
		if err != nil {
			return err
		}

		// Close the listener when the context is canceled to unblock Accept.
		go func() {
			<-ctx.Done()
			_ = ln.Close()
		}()

		go func() { errCh <- serve(ctx, ln) }()

		return nil
	}

	if err := listen(s.Port, s.Serve); err != nil {
		return err
	}
	if err := listen(s.Port+1, s.ServeFileTransfers); err != nil {
		return err
	}

	if s.TLSConfig != nil {
		if err := listen(s.TLSPort, s.ServeWithTLS); err != nil {
			return err
		}
		if err := listen(s.TLSPort+1, s.ServeFileTransfersWithTLS); err != nil {
			return err
		}
	}

	// Block until the first serve loop returns.  The deferred cancel closes the remaining
	// listeners and stops their serve loops.
	err := <-errCh
	if ctx.Err() != nil {
		s.Logger.Info("Server shutting down")
	}
	return err
}

func (s *Server) ServeFileTransfers(ctx context.Context, ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		go func() {
			defer func() { _ = conn.Close() }()

			err = s.handleFileTransfer(
				context.WithValue(ctx, contextKeyReq, requestCtx{remoteAddr: conn.RemoteAddr().String()}),
				conn,
			)

			if err != nil {
				s.Logger.Error("file transfer error", "err", err)
			}
		}()
	}
}

func (s *Server) ServeWithTLS(ctx context.Context, ln net.Listener) error {
	return s.Serve(ctx, tls.NewListener(ln, s.TLSConfig))
}

func (s *Server) ServeFileTransfersWithTLS(ctx context.Context, ln net.Listener) error {
	return s.ServeFileTransfers(ctx, tls.NewListener(ln, s.TLSConfig))
}

// Send routes t to the send queue of the client identified by t.ClientID.  Transactions for
// clients that are no longer connected are dropped.
func (s *Server) Send(t Transaction) {
	if c := s.ClientMgr.Get(t.ClientID); c != nil {
		c.Send(t)
	}
}

// perIPRateLimit controls how frequently an IP address can connect before being throttled.
// 0.5 = 1 connection every 2 seconds
const perIPRateLimit = rate.Limit(0.5)

// rateLimiterTTL is how long the rate limiter for an idle IP address is retained before eviction.
const rateLimiterTTL = 7 * 24 * time.Hour

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// sweepRateLimiters evicts rate limiters for IP addresses that have not connected recently, so
// the rate limiter map does not grow unbounded over the lifetime of the server.
func (s *Server) sweepRateLimiters() {
	cutoff := time.Now().Add(-rateLimiterTTL)

	s.rateLimitersMu.Lock()
	defer s.rateLimitersMu.Unlock()

	for ip, entry := range s.rateLimiters {
		if entry.lastSeen.Before(cutoff) {
			delete(s.rateLimiters, ip)
		}
	}
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Context cancellation closes the listener, unblocking Accept.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}

			s.Logger.Error("Error accepting connection", "err", err)
			continue
		}

		go func() {
			ipAddr, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

			connCtx := context.WithValue(ctx, contextKeyReq, requestCtx{
				remoteAddr: conn.RemoteAddr().String(),
			})

			s.Logger.Info("Connection established", "ip", ipAddr)
			defer func() { _ = conn.Close() }()

			// Check if we have an existing rate limit for the IP and create one if we do not.
			s.rateLimitersMu.Lock()
			entry, ok := s.rateLimiters[ipAddr]
			if !ok {
				entry = &rateLimiterEntry{limiter: rate.NewLimiter(perIPRateLimit, 1)}
				s.rateLimiters[ipAddr] = entry
			}
			entry.lastSeen = time.Now()
			rl := entry.limiter
			s.rateLimitersMu.Unlock()

			// Check if the rate limit is exceeded and close the connection if so.
			if !rl.Allow() {
				s.Logger.Info("Rate limit exceeded", "remoteAddr", conn.RemoteAddr())
				_ = conn.Close()
				return
			}

			if err := s.handleNewConnection(connCtx, conn, conn.RemoteAddr().String()); err != nil {
				if err == io.EOF {
					s.Logger.Info("Client disconnected", "remoteAddr", conn.RemoteAddr())
				} else {
					s.Logger.Error("Error serving request", "remoteAddr", conn.RemoteAddr(), "err", err)
				}
			}
		}()
	}
}

// time in seconds between tracker re-registration
const trackerUpdateFrequency = 300

// TrackerRegistrar interface for tracker registration operations
type TrackerRegistrar interface {
	Register(tracker string, registration *TrackerRegistration) error
}

// RealTrackerRegistrar implements TrackerRegistrar using the real network operations
type RealTrackerRegistrar struct {
	dialer Dialer
}

func NewRealTrackerRegistrar() *RealTrackerRegistrar {
	return &RealTrackerRegistrar{
		dialer: &RealDialer{},
	}
}

func (r *RealTrackerRegistrar) Register(tracker string, registration *TrackerRegistration) error {
	return register(r.dialer, tracker, registration)
}

// parseTrackerPassword extracts the password from a tracker address in format "host:port:password"
// Returns empty string if no password is present or if the format is invalid
// For addresses with more than 3 parts (like passwords containing colons), everything after the second colon is treated as the password
func parseTrackerPassword(trackerAddr string) string {
	splitAddr := strings.Split(trackerAddr, ":")
	if len(splitAddr) >= 3 {
		// Join everything from the third part onwards (index 2+) to handle passwords with colons
		return strings.Join(splitAddr[2:], ":")
	}
	return ""
}

// registerWithAllTrackers performs tracker registration for all configured trackers
func (s *Server) registerWithAllTrackers() {
	if !s.Config.EnableTrackerRegistration {
		return
	}

	for _, t := range s.Config.Trackers {
		tr := &TrackerRegistration{
			UserCount:   len(s.ClientMgr.List()),
			PassID:      s.TrackerPassID,
			Name:        s.Config.Name,
			Description: s.Config.Description,
		}
		binary.BigEndian.PutUint16(tr.Port[:], uint16(s.Port))
		binary.BigEndian.PutUint16(tr.TLSPort[:], uint16(s.TLSPort))

		tr.Password = parseTrackerPassword(t)

		if err := s.TrackerRegistrar.Register(t, tr); err != nil {
			s.Logger.Error("Unable to register with tracker", "tracker", t, "err", err)
		}
	}
}

// registerWithTrackers runs every trackerUpdateFrequency seconds to update the server's tracker entry on all configured
// trackers.
func (s *Server) registerWithTrackers(ctx context.Context) {
	if s.Config.EnableTrackerRegistration {
		s.Logger.Info("Tracker registration enabled", "trackers", s.Config.Trackers)
	}

	// Do the first registration immediately
	s.registerWithAllTrackers()

	ticker := time.NewTicker(trackerUpdateFrequency * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.registerWithAllTrackers()
		}
	}
}

const (
	userIdleSeconds   = 300 // time in seconds before an inactive user is marked idle
	idleCheckInterval = 10  // time in seconds to check for idle users
)

// keepaliveHandler runs every idleCheckInterval seconds and increments a user's idle time by idleCheckInterval seconds.
// If the updated idle time exceeds userIdleSeconds and the user was not previously idle, we notify all connected clients
// that the user has gone idle.  For most clients, this turns the user grey in the user list.
// It also sweeps stale per-IP rate limiters on each tick.
func (s *Server) keepaliveHandler(ctx context.Context) {
	ticker := time.NewTicker(idleCheckInterval * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, c := range s.ClientMgr.List() {
				if c.incrementIdleTime(idleCheckInterval) {
					c.SendAll(
						TranNotifyChangeUser,
						NewField(FieldUserID, c.ID[:]),
						NewField(FieldUserFlags, c.FlagBytes()),
						NewField(FieldUserName, c.GetUserName()),
						NewField(FieldUserIconID, c.GetIcon()),
					)
				}
			}

			s.sweepRateLimiters()
		}
	}
}

func (s *Server) NewClientConn(conn io.ReadWriteCloser, remoteAddr string) *ClientConn {
	clientConn := &ClientConn{
		Icon:       []byte{0, 0}, // TODO: make array type
		Connection: conn,
		Server:     s,
		RemoteAddr: remoteAddr,

		ClientFileTransferMgr: NewClientFileTransferMgr(),
	}

	return clientConn
}

func sendBanMessage(rwc io.Writer, message string) {
	t := NewTransaction(
		TranServerMsg,
		[2]byte{0, 0},
		NewField(FieldData, []byte(message)),
		NewField(FieldChatOptions, []byte{0, 0}),
	)
	_, _ = io.Copy(rwc, &t)
	time.Sleep(1 * time.Second)
}

// handleNewConnection takes a new net.Conn and performs the initial login sequence
func (s *Server) handleNewConnection(ctx context.Context, rwc io.ReadWriteCloser, remoteAddr string) error {
	defer dontPanic(s.Logger)

	if err := performHandshake(rwc); err != nil {
		return fmt.Errorf("perform handshake: %w", err)
	}

	// Create a new scanner for parsing incoming bytes into transaction tokens
	scanner := bufio.NewScanner(rwc)
	scanner.Split(transactionScanner)

	scanner.Scan()

	// Make a new []byte slice and copy the scanner bytes to it.  This is critical to avoid a data race as the
	// scanner re-uses the buffer for subsequent scans.
	buf := make([]byte, len(scanner.Bytes()))
	copy(buf, scanner.Bytes())

	var clientLogin Transaction
	if _, err := clientLogin.Write(buf); err != nil {
		return fmt.Errorf("error writing login transaction: %w", err)
	}

	login := clientLogin.GetField(FieldUserLogin).DecodeObfuscatedString()
	if login == "" {
		login = GuestAccount
	}

	// Check if remoteAddr is present in the ban list, we do this after we have the login name
	ipAddr, _, _ := net.SplitHostPort(remoteAddr)

	// Check if user is banned
	if s.BanList != nil && s.BanList.IsUsernameBanned(login) {
		_ = s.BanList.Add(ipAddr, nil)
		sendBanMessage(rwc, "You are banned on this server")
		s.Logger.Debug("Disconnecting banned user", "login", login, "ip", ipAddr)
		return nil
	}

	// Check if IP is banned
	if s.BanList != nil {
		if isBanned, banUntil := s.BanList.IsBanned(ipAddr); isBanned {
			// permaban
			if banUntil == nil {
				sendBanMessage(rwc, "You are permanently banned on this server")
				s.Logger.Debug("Disconnecting permanently banned IP", "remoteAddr", ipAddr)
				return nil
			}
			// temporary ban
			if time.Now().Before(*banUntil) {
				sendBanMessage(rwc, "You are temporarily banned on this server")
				s.Logger.Debug("Disconnecting temporarily banned IP", "remoteAddr", ipAddr)
				return nil
			}
		}
	}

	c := s.NewClientConn(rwc, remoteAddr)

	// Start the client's writer goroutine: the single writer to the connection, which preserves
	// transaction ordering and prevents interleaved writes.
	go c.writeLoop()

	if s.Presence != nil {
		s.Presence.UserConnected(login, ipAddr)
	}

	// Remove the client from the list of connected clients when they disconnect
	defer func() {
		if s.Presence != nil {
			s.Presence.UserDisconnected(login, string(c.GetUserName()), ipAddr)
		}
		c.Disconnect()
	}()

	encodedPassword := clientLogin.GetField(FieldUserPassword).Data
	c.Version = clientLogin.GetField(FieldVersion).Data

	c.Logger = s.Logger.With("ip", ipAddr, "login", login)

	// If authentication fails, send error reply and close connection
	if !c.Authenticate(login, encodedPassword) {
		t := c.NewErrReply(&clientLogin, "Incorrect login.")[0]

		_, err := io.Copy(rwc, &t)
		if err != nil {
			return err
		}

		c.Logger.Info("Incorrect login")

		return nil
	}

	if clientLogin.GetField(FieldUserIconID).Data != nil {
		c.SetIcon(clientLogin.GetField(FieldUserIconID).Data)
	}

	account := c.Server.AccountManager.Get(login)
	if account == nil {
		return nil
	}
	c.SetAccount(account)

	if clientLogin.GetField(FieldUserName).Data != nil {
		if c.Authorize(AccessAnyName) {
			c.SetUserName(clientLogin.GetField(FieldUserName).Data)
		} else {
			c.SetUserName([]byte(account.Name))
		}
	}

	if c.Authorize(AccessDisconUser) {
		c.SetFlag(UserFlagAdmin, 1)
	}

	// Publish the client to the manager only now that its session state (Account, Version,
	// UserName, Flags) is fully initialized.  Other goroutines iterate ClientMgr.List() and
	// dereference Account, so a client must never be visible before login completes.
	s.ClientMgr.Add(c)

	c.Send(c.NewReply(&clientLogin,
		NewField(FieldVersion, []byte{0x00, 0xbe}),
		NewField(FieldCommunityBannerID, []byte{0, 0}),
		NewField(FieldServerName, []byte(s.Config.Name)),
	))

	// Send user access privs so client UI knows how to behave
	c.Send(NewTransaction(TranUserAccess, c.ID, NewField(FieldUserAccess, c.AccessBytes())))

	// Accounts with AccessNoAgreement do not receive the server agreement on login.  The behavior is different between
	// client versions.  For 1.2.3 client, we do not send TranShowAgreement.  For other client versions, we send
	// TranShowAgreement but with the NoServerAgreement field set to 1.
	if c.Authorize(AccessNoAgreement) {
		// If client version is nil, then the client uses the 1.2.3 login behavior
		if c.Version != nil {
			c.Send(NewTransaction(TranShowAgreement, c.ID, NewField(FieldNoServerAgreement, []byte{1})))
		}
	} else {
		_, _ = c.Server.Agreement.Seek(0, 0)
		data, _ := io.ReadAll(c.Server.Agreement)

		c.Send(NewTransaction(TranShowAgreement, c.ID, NewField(FieldData, data)))
	}

	// If the client has provided a username as part of the login, we can infer that it is using the 1.2.3 login
	// flow and not the 1.5+ flow.
	if userName := c.GetUserName(); len(userName) != 0 {
		// Add the client username to the logger.  For 1.5+ clients, we don't have this information yet as it comes as
		// part of TranAgreed
		c.Logger = c.Logger.With("name", string(userName))
		c.Logger.Info("Login successful")

		if s.Presence != nil {
			s.Presence.UserRenamed(login, "", string(userName), ipAddr)
		}

		// Notify other clients on the server that the new user has logged in.  For 1.5+ clients we don't have this
		// information yet, so we do it in TranAgreed instead
		for _, t := range c.NotifyOthers(
			NewTransaction(
				TranNotifyChangeUser, [2]byte{0, 0},
				NewField(FieldUserName, userName),
				NewField(FieldUserID, c.ID[:]),
				NewField(FieldUserIconID, c.GetIcon()),
				NewField(FieldUserFlags, c.FlagBytes()),
			),
		) {
			c.Server.Send(t)
		}
	}

	c.Server.Stats.Increment(StatConnectionCounter, StatCurrentlyConnected)
	defer c.Server.Stats.Decrement(StatCurrentlyConnected)

	c.Server.Stats.Max(StatConnectionPeak, len(s.ClientMgr.List()))

	// Scan for new transactions and handle them as they come in.
	for scanner.Scan() {
		// Copy the scanner bytes to a new slice to it to avoid a data race when the scanner re-uses the buffer.
		tmpBuf := make([]byte, len(scanner.Bytes()))
		copy(tmpBuf, scanner.Bytes())

		var t Transaction
		if _, err := t.Write(tmpBuf); err != nil {
			return err
		}

		c.handleTransaction(t)
	}
	return nil
}

// handleFileTransfer receives a client net.Conn from the file transfer server, performs the requested transfer type, then closes the connection
func (s *Server) handleFileTransfer(ctx context.Context, rwc io.ReadWriter) error {
	defer dontPanic(s.Logger)

	// The first 16 bytes contain the file transfer.
	var t transfer
	if _, err := io.CopyN(&t, rwc, 16); err != nil {
		return fmt.Errorf("error reading file transfer: %w", err)
	}

	fileTransfer := s.FileTransferMgr.Get(t.ReferenceNumber)
	if fileTransfer == nil {
		return fmt.Errorf("invalid transaction ID: %v", t.ReferenceNumber)
	}

	defer func() {
		s.FileTransferMgr.Delete(t.ReferenceNumber)

		// Wait a few seconds before closing the connection: this is a workaround for problems
		// observed with Windows clients where the client must initiate close of the TCP connection before
		// the server does.  This is gross and seems unnecessary.  TODO: Revisit?
		time.Sleep(3 * time.Second)
	}()

	var remoteAddr string
	if rc, ok := ctx.Value(contextKeyReq).(requestCtx); ok {
		remoteAddr = rc.remoteAddr
	}
	rLogger := s.Logger.With(
		"remoteAddr", remoteAddr,
		"login", fileTransfer.ClientConn.GetAccount().Login,
		"Name", string(fileTransfer.ClientConn.GetUserName()),
	)

	fullPath, err := ReadPath(fileTransfer.FileRoot, fileTransfer.FilePath, fileTransfer.FileName, s.TextDecoder)
	if err != nil {
		return err
	}

	switch fileTransfer.Type {
	case BannerDownload:
		if _, err := io.Copy(rwc, bytes.NewReader(s.Banner())); err != nil {
			return fmt.Errorf("banner download: %w", err)
		}
	case FileDownload:
		s.Stats.Increment(StatDownloadCounter, StatDownloadsInProgress)
		defer func() {
			s.Stats.Decrement(StatDownloadsInProgress)
		}()

		err = DownloadHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, true)
		if err != nil {
			return fmt.Errorf("file download: %w", err)
		}

	case FileUpload:
		s.Stats.Increment(StatUploadCounter, StatUploadsInProgress)
		defer func() {
			s.Stats.Decrement(StatUploadsInProgress)
		}()

		err = UploadHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, s.Config.PreserveResourceForks)
		if err != nil {
			return fmt.Errorf("file upload: %w", err)
		}

	case FolderDownload:
		s.Stats.Increment(StatDownloadCounter, StatDownloadsInProgress)
		defer func() {
			s.Stats.Decrement(StatDownloadsInProgress)
		}()

		err = DownloadFolderHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, s.Config.PreserveResourceForks)
		if err != nil {
			return fmt.Errorf("folder download: %w", err)
		}

	case FolderUpload:
		s.Stats.Increment(StatUploadCounter, StatUploadsInProgress)
		defer func() {
			s.Stats.Decrement(StatUploadsInProgress)
		}()

		var transferSizeValue uint32
		switch len(fileTransfer.TransferSize) {
		case 2: // 16-bit
			transferSizeValue = uint32(binary.BigEndian.Uint16(fileTransfer.TransferSize))
		case 4: // 32-bit
			transferSizeValue = binary.BigEndian.Uint32(fileTransfer.TransferSize)
		default:
			rLogger.Warn("Unexpected TransferSize length", "bytes", len(fileTransfer.TransferSize))
		}

		rLogger.Info(
			"Folder upload started",
			"dstPath", fullPath,
			"TransferSize", transferSizeValue,
			"FolderItemCount", fileTransfer.FolderItemCount,
		)

		err = UploadFolderHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, s.Config.PreserveResourceForks)
		if err != nil {
			return fmt.Errorf("folder upload: %w", err)
		}
	}
	return nil
}

func (s *Server) SendAll(t TranType, fields ...Field) {
	for _, c := range s.ClientMgr.List() {
		c.Send(NewTransaction(t, c.ID, fields...))
	}
}

// Shutdown sends msg to all connected clients and stops ListenAndServe.
func (s *Server) Shutdown(msg []byte) {
	s.Logger.Info("Shutdown signal received")
	s.SendAll(TranDisconnectMsg, NewField(FieldData, msg))

	// Give the client writer goroutines a moment to flush the disconnect message.
	time.Sleep(3 * time.Second)

	s.initShutdownCh()
	s.shutdownOnce.Do(func() { close(s.shutdownCh) })
}
