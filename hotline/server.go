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
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
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

	Config    Config
	ConfigDir string
	Logger    *slog.Logger

	TrackerPassID [4]byte

	Stats Counter

	FS FileStore // Storage backend to use for File storage

	outbox chan Transaction

	// TODO
	Agreement []byte
	banner    []byte
	// END TODO

	FileTransferMgr FileTransferMgr
	ChatMgr         ChatManager
	ClientMgr       ClientManager
	AccountManager  AccountManager
	ThreadedNewsMgr ThreadedNewsMgr
	BanList         BanMgr

	MessageBoard io.ReadWriteSeeker
}

// NewServer constructs a new Server from a config dir
func NewServer(config Config, configDir, netInterface string, netPort int, logger *slog.Logger, fs FileStore) (*Server, error) {
	server := Server{
		NetInterface:    netInterface,
		Port:            netPort,
		Config:          config,
		ConfigDir:       configDir,
		Logger:          logger,
		outbox:          make(chan Transaction),
		Stats:           NewStats(),
		FS:              fs,
		ChatMgr:         NewMemChatManager(),
		ClientMgr:       NewMemClientMgr(),
		FileTransferMgr: NewMemFileTransferMgr(),
	}

	// generate a new random passID for tracker registration
	_, err := rand.Read(server.TrackerPassID[:])
	if err != nil {
		return nil, err
	}

	server.Agreement, err = os.ReadFile(filepath.Join(configDir, agreementFile))
	if err != nil {
		return nil, err
	}

	server.AccountManager, err = NewYAMLAccountManager(filepath.Join(configDir, "Users/"))
	if err != nil {
		return nil, fmt.Errorf("error loading accounts: %w", err)
	}

	// If the FileRoot is an absolute path, use it, otherwise treat as a relative path to the config dir.
	if !filepath.IsAbs(server.Config.FileRoot) {
		server.Config.FileRoot = filepath.Join(configDir, server.Config.FileRoot)
	}

	server.banner, err = os.ReadFile(filepath.Join(server.ConfigDir, server.Config.BannerFile))
	if err != nil {
		return nil, fmt.Errorf("error opening banner: %w", err)
	}

	if server.Config.EnableTrackerRegistration {
		server.Logger.Info(
			"Tracker registration enabled",
			"frequency", fmt.Sprintf("%vs", trackerUpdateFrequency),
			"trackers", server.Config.Trackers,
		)

		go server.registerWithTrackers()
	}

	// Start Client Keepalive go routine
	go server.keepaliveHandler()

	return &server, nil
}

func (s *Server) CurrentStats() map[string]interface{} {
	return s.Stats.Values()
}

func (s *Server) ListenAndServe(ctx context.Context) error {
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
				s.Logger.Error("file transfer error", "reason", err)
			}
		}()
	}
}

func (s *Server) sendTransaction(t Transaction) error {
	client := s.ClientMgr.Get(t.clientID)

	if client == nil {
		return nil
	}

	_, err := io.Copy(client.Connection, &t)
	if err != nil {
		return fmt.Errorf("failed to send transaction to client %v: %v", t.clientID, err)
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

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go s.processOutbox()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.Logger.Error("error accepting connection", "err", err)
		}
		connCtx := context.WithValue(ctx, contextKeyReq, requestCtx{
			remoteAddr: conn.RemoteAddr().String(),
		})

		go func() {
			s.Logger.Info("Connection established", "RemoteAddr", conn.RemoteAddr())

			defer conn.Close()
			if err := s.handleNewConnection(connCtx, conn, conn.RemoteAddr().String()); err != nil {
				if err == io.EOF {
					s.Logger.Info("Client disconnected", "RemoteAddr", conn.RemoteAddr())
				} else {
					s.Logger.Error("error serving request", "RemoteAddr", conn.RemoteAddr(), "err", err)
				}
			}
		}()
	}
}

const (
	agreementFile = "Agreement.txt"
)

func (s *Server) registerWithTrackers() {
	for {
		tr := &TrackerRegistration{
			UserCount:   len(s.ClientMgr.List()),
			PassID:      s.TrackerPassID,
			Name:        s.Config.Name,
			Description: s.Config.Description,
		}
		binary.BigEndian.PutUint16(tr.Port[:], uint16(s.Port))
		for _, t := range s.Config.Trackers {
			if err := register(&RealDialer{}, t, tr); err != nil {
				s.Logger.Error(fmt.Sprintf("unable to register with tracker %v", t), "error", err)
			}
		}

		time.Sleep(trackerUpdateFrequency * time.Second)
	}
}

// keepaliveHandler
func (s *Server) keepaliveHandler() {
	for {
		time.Sleep(idleCheckInterval * time.Second)

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

// loadFromYAMLFile loads data from a YAML file into the provided data structure.
func loadFromYAMLFile(path string, data interface{}) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	decoder := yaml.NewDecoder(fh)
	return decoder.Decode(data)
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

	if err := performHandshake(rwc); err != nil {
		return fmt.Errorf("error performing handshake: %w", err)
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

	c.logger = s.Logger.With("ip", ipAddr, "login", login)

	// If authentication fails, send error reply and close connection
	if !c.Authenticate(login, encodedPassword) {
		t := c.NewErrReply(&clientLogin, "Incorrect login.")[0]

		_, err := io.Copy(rwc, &t)
		if err != nil {
			return err
		}

		c.logger.Info("Login failed", "clientVersion", fmt.Sprintf("%x", c.Version))

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
		c.Server.outbox <- NewTransaction(TranShowAgreement, c.ID, NewField(FieldData, s.Agreement))
	}

	// If the client has provided a username as part of the login, we can infer that it is using the 1.2.3 login
	// flow and not the 1.5+ flow.
	if len(c.UserName) != 0 {
		// Add the client username to the logger.  For 1.5+ clients, we don't have this information yet as it comes as
		// part of TranAgreed
		c.logger = c.logger.With("Name", string(c.UserName))
		c.logger.Info("Login successful", "clientVersion", "Not sent (probably 1.2.3)")

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
		buf := make([]byte, len(scanner.Bytes()))
		copy(buf, scanner.Bytes())

		var t Transaction
		if _, err := t.Write(buf); err != nil {
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

	fullPath, err := readPath(s.Config.FileRoot, fileTransfer.FilePath, fileTransfer.FileName)
	if err != nil {
		return err
	}

	switch fileTransfer.Type {
	case BannerDownload:
		if _, err := io.Copy(rwc, bytes.NewBuffer(s.banner)); err != nil {
			return fmt.Errorf("error sending banner: %w", err)
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
			return fmt.Errorf("file upload error: %w", err)
		}

	case FolderDownload:
		s.Stats.Increment(StatDownloadCounter, StatDownloadsInProgress)
		defer func() {
			s.Stats.Decrement(StatDownloadsInProgress)
		}()

		err = DownloadFolderHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, s.Config.PreserveResourceForks)
		if err != nil {
			return fmt.Errorf("file upload error: %w", err)
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
			return fmt.Errorf("file upload error: %w", err)
		}
	}
	return nil
}
