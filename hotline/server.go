package hotline

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/time/rate"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type contextKey string

var contextKeyReq = contextKey("req")

type requestCtx struct {
	remoteAddr string
}

// Converts bytes from Mac Roman encoding to UTF-8
var txtDecoder = charmap.Macintosh.NewDecoder()

// Converts bytes from UTF-8 to Mac Roman encoding
var txtEncoder = charmap.Macintosh.NewEncoder()

type Server struct {
	NetInterface string
	Port         int

	rateLimiters map[string]*rate.Limiter

	handlers map[TranType]HandlerFunc

	Config Config
	Logger *slog.Logger

	TrackerPassID [4]byte

	Stats Counter

	FS FileStore // Storage backend to use for File storage

	outbox chan Transaction

	Agreement io.ReadSeeker
	Banner    []byte

	FileTransferMgr FileTransferMgr
	ChatMgr         ChatManager
	ClientMgr       ClientManager
	AccountManager  AccountManager
	ThreadedNewsMgr ThreadedNewsMgr
	BanList         BanMgr

	MessageBoard io.ReadWriteSeeker
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

type ServerConfig struct {
}

func NewServer(options ...Option) (*Server, error) {
	server := Server{
		handlers:        make(map[TranType]HandlerFunc),
		outbox:          make(chan Transaction),
		rateLimiters:    make(map[string]*rate.Limiter),
		FS:              &OSFileStore{},
		ChatMgr:         NewMemChatManager(),
		ClientMgr:       NewMemClientMgr(),
		FileTransferMgr: NewMemFileTransferMgr(),
		Stats:           NewStats(),
	}

	for _, opt := range options {
		opt(&server)
	}

	// generate a new random passID for tracker registration
	_, err := rand.Read(server.TrackerPassID[:])
	if err != nil {
		return nil, err
	}

	return &server, nil
}

func (s *Server) CurrentStats() map[string]interface{} {
	return s.Stats.Values()
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	go s.registerWithTrackers(ctx)
	go s.keepaliveHandler(ctx)
	go s.processOutbox()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%v", s.NetInterface, s.Port))
		if err != nil {
			log.Fatal(err)
		}

		log.Fatal(s.Serve(ctx, ln))
	}()

	wg.Add(1)
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%v", s.NetInterface, s.Port+1))
		if err != nil {
			log.Fatal(err)
		}

		log.Fatal(s.ServeFileTransfers(ctx, ln))
	}()

	wg.Wait()

	return nil
}

func (s *Server) ServeFileTransfers(ctx context.Context, ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
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

func (s *Server) sendTransaction(t Transaction) error {
	client := s.ClientMgr.Get(t.ClientID)

	if client == nil {
		return nil
	}

	_, err := io.Copy(client.Connection, &t)
	if err != nil {
		return fmt.Errorf("failed to send transaction to client %v: %v", t.ClientID, err)
	}

	return nil
}

func (s *Server) processOutbox() {
	for {
		t := <-s.outbox
		go func() {
			if err := s.sendTransaction(t); err != nil {
				s.Logger.Error("error sending transaction", "err", err)
			}
		}()
	}
}

// perIPRateLimit controls how frequently an IP address can connect before being throttled.
// 0.5 = 1 connection every 2 seconds
const perIPRateLimit = rate.Limit(0.5)

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	for {
		select {
		case <-ctx.Done():
			s.Logger.Info("Server shutting down")
			return ctx.Err()
		default:
			conn, err := ln.Accept()
			if err != nil {
				s.Logger.Error("Error accepting connection", "err", err)
				continue
			}

			go func() {
				ipAddr := strings.Split(conn.RemoteAddr().(*net.TCPAddr).String(), ":")[0]

				connCtx := context.WithValue(ctx, contextKeyReq, requestCtx{
					remoteAddr: conn.RemoteAddr().String(),
				})

				s.Logger.Info("Connection established", "ip", ipAddr)
				defer conn.Close()

				// Check if we have an existing rate limit for the IP and create one if we do not.
				rl, ok := s.rateLimiters[ipAddr]
				if !ok {
					rl = rate.NewLimiter(perIPRateLimit, 1)
					s.rateLimiters[ipAddr] = rl
				}

				// Check if the rate limit is exceeded and close the connection if so.
				if !rl.Allow() {
					s.Logger.Info("Rate limit exceeded", "RemoteAddr", conn.RemoteAddr())
					conn.Close()
					return
				}

				if err := s.handleNewConnection(connCtx, conn, conn.RemoteAddr().String()); err != nil {
					if err == io.EOF {
						s.Logger.Info("Client disconnected", "RemoteAddr", conn.RemoteAddr())
					} else {
						s.Logger.Error("Error serving request", "RemoteAddr", conn.RemoteAddr(), "err", err)
					}
				}
			}()
		}
	}
}

// time in seconds between tracker re-registration
const trackerUpdateFrequency = 300

// registerWithTrackers runs every trackerUpdateFrequency seconds to update the server's tracker entry on all configured
// trackers.
func (s *Server) registerWithTrackers(ctx context.Context) {
	if s.Config.EnableTrackerRegistration {
		s.Logger.Info("Tracker registration enabled", "trackers", s.Config.Trackers)
	}

	for {
		if s.Config.EnableTrackerRegistration {
			for _, t := range s.Config.Trackers {
				tr := &TrackerRegistration{
					UserCount:   len(s.ClientMgr.List()),
					PassID:      s.TrackerPassID,
					Name:        s.Config.Name,
					Description: s.Config.Description,
				}
				binary.BigEndian.PutUint16(tr.Port[:], uint16(s.Port))

				// Check the tracker string for a password.  This is janky but avoids a breaking change to the Config
				// Trackers field.
				splitAddr := strings.Split(":", t)
				if len(splitAddr) == 3 {
					tr.Password = splitAddr[2]
				}

				if err := register(&RealDialer{}, t, tr); err != nil {
					s.Logger.Error(fmt.Sprintf("Unable to register with tracker %v", t), "error", err)
				}
			}
		}
		// Using time.Ticker with for/select would be more idiomatic, but it's super annoying that it doesn't tick on
		// first pass.  Revist, maybe.
		// https://github.com/golang/go/issues/17601
		time.Sleep(trackerUpdateFrequency * time.Second)
	}
}

const (
	userIdleSeconds   = 300 // time in seconds before an inactive user is marked idle
	idleCheckInterval = 10  // time in seconds to check for idle users
)

// keepaliveHandler runs every idleCheckInterval seconds and increments a user's idle time by idleCheckInterval seconds.
// If the updated idle time exceeds userIdleSeconds and the user was not previously idle, we notify all connected clients
// that the user has gone idle.  For most clients, this turns the user grey in the user list.
func (s *Server) keepaliveHandler(ctx context.Context) {
	ticker := time.NewTicker(idleCheckInterval * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, c := range s.ClientMgr.List() {
				c.mu.Lock()
				c.IdleTime += idleCheckInterval

				// Check if the user
				if c.IdleTime > userIdleSeconds && !c.Flags.IsSet(UserFlagAway) {
					c.Flags.Set(UserFlagAway, 1)

					c.SendAll(
						TranNotifyChangeUser,
						NewField(FieldUserID, c.ID[:]),
						NewField(FieldUserFlags, c.Flags[:]),
						NewField(FieldUserName, c.UserName),
						NewField(FieldUserIconID, c.Icon),
					)
				}
				c.mu.Unlock()
			}
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

	s.ClientMgr.Add(clientConn)

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

	// Check if remoteAddr is present in the ban list
	ipAddr := strings.Split(remoteAddr, ":")[0]
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

	c := s.NewClientConn(rwc, remoteAddr)
	defer c.Disconnect()

	encodedPassword := clientLogin.GetField(FieldUserPassword).Data
	c.Version = clientLogin.GetField(FieldVersion).Data

	login := clientLogin.GetField(FieldUserLogin).DecodeObfuscatedString()
	if login == "" {
		login = GuestAccount
	}

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
		c.Icon = clientLogin.GetField(FieldUserIconID).Data
	}

	c.Account = c.Server.AccountManager.Get(login)
	if c.Account == nil {
		return nil
	}

	if clientLogin.GetField(FieldUserName).Data != nil {
		if c.Authorize(AccessAnyName) {
			c.UserName = clientLogin.GetField(FieldUserName).Data
		} else {
			c.UserName = []byte(c.Account.Name)
		}
	}

	if c.Authorize(AccessDisconUser) {
		c.Flags.Set(UserFlagAdmin, 1)
	}

	s.outbox <- c.NewReply(&clientLogin,
		NewField(FieldVersion, []byte{0x00, 0xbe}),
		NewField(FieldCommunityBannerID, []byte{0, 0}),
		NewField(FieldServerName, []byte(s.Config.Name)),
	)

	// Send user access privs so client UI knows how to behave
	c.Server.outbox <- NewTransaction(TranUserAccess, c.ID, NewField(FieldUserAccess, c.Account.Access[:]))

	// Accounts with AccessNoAgreement do not receive the server agreement on login.  The behavior is different between
	// client versions.  For 1.2.3 client, we do not send TranShowAgreement.  For other client versions, we send
	// TranShowAgreement but with the NoServerAgreement field set to 1.
	if c.Authorize(AccessNoAgreement) {
		// If client version is nil, then the client uses the 1.2.3 login behavior
		if c.Version != nil {
			c.Server.outbox <- NewTransaction(TranShowAgreement, c.ID, NewField(FieldNoServerAgreement, []byte{1}))
		}
	} else {
		_, _ = c.Server.Agreement.Seek(0, 0)
		data, _ := io.ReadAll(c.Server.Agreement)

		c.Server.outbox <- NewTransaction(TranShowAgreement, c.ID, NewField(FieldData, data))
	}

	// If the client has provided a username as part of the login, we can infer that it is using the 1.2.3 login
	// flow and not the 1.5+ flow.
	if len(c.UserName) != 0 {
		// Add the client username to the logger.  For 1.5+ clients, we don't have this information yet as it comes as
		// part of TranAgreed
		c.Logger = c.Logger.With("name", string(c.UserName))
		c.Logger.Info("Login successful")

		// Notify other clients on the server that the new user has logged in.  For 1.5+ clients we don't have this
		// information yet, so we do it in TranAgreed instead
		for _, t := range c.NotifyOthers(
			NewTransaction(
				TranNotifyChangeUser, [2]byte{0, 0},
				NewField(FieldUserName, c.UserName),
				NewField(FieldUserID, c.ID[:]),
				NewField(FieldUserIconID, c.Icon),
				NewField(FieldUserFlags, c.Flags[:]),
			),
		) {
			c.Server.outbox <- t
		}
	}

	c.Server.Stats.Increment(StatConnectionCounter, StatCurrentlyConnected)
	defer c.Server.Stats.Decrement(StatCurrentlyConnected)

	if len(s.ClientMgr.List()) > c.Server.Stats.Get(StatConnectionPeak) {
		c.Server.Stats.Set(StatConnectionPeak, len(s.ClientMgr.List()))
	}

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
		return errors.New("invalid transaction ID")
	}

	defer func() {
		s.FileTransferMgr.Delete(t.ReferenceNumber)

		// Wait a few seconds before closing the connection: this is a workaround for problems
		// observed with Windows clients where the client must initiate close of the TCP connection before
		// the server does.  This is gross and seems unnecessary.  TODO: Revisit?
		time.Sleep(3 * time.Second)
	}()

	rLogger := s.Logger.With(
		"remoteAddr", ctx.Value(contextKeyReq).(requestCtx).remoteAddr,
		"login", fileTransfer.ClientConn.Account.Login,
		"Name", string(fileTransfer.ClientConn.UserName),
	)

	fullPath, err := ReadPath(fileTransfer.FileRoot, fileTransfer.FilePath, fileTransfer.FileName)
	if err != nil {
		return err
	}

	switch fileTransfer.Type {
	case BannerDownload:
		if _, err := io.Copy(rwc, bytes.NewBuffer(s.Banner)); err != nil {
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

		rLogger.Info(
			"Folder upload started",
			"dstPath", fullPath,
			"TransferSize", binary.BigEndian.Uint32(fileTransfer.TransferSize),
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
		s.outbox <- NewTransaction(t, c.ID, fields...)
	}
}

func (s *Server) Shutdown(msg []byte) {
	s.Logger.Info("Shutdown signal received")
	s.SendAll(TranDisconnectMsg, NewField(FieldData, msg))

	time.Sleep(3 * time.Second)

	os.Exit(0)
}
