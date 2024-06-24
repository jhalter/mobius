package hotline

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	Accounts     map[string]*Account
	Agreement    []byte

	Clients       map[[2]byte]*ClientConn
	fileTransfers map[[4]byte]*FileTransfer

	Config    *Config
	ConfigDir string
	Logger    *slog.Logger
	banner    []byte

	PrivateChatsMu sync.Mutex
	PrivateChats   map[[4]byte]*PrivateChat

	nextClientID  atomic.Uint32
	TrackerPassID [4]byte

	statsMu sync.Mutex
	Stats   *Stats

	FS FileStore // Storage backend to use for File storage

	outbox chan Transaction
	mux    sync.Mutex

	threadedNewsMux sync.Mutex
	ThreadedNews    *ThreadedNews

	flatNewsMux sync.Mutex
	FlatNews    []byte

	banListMU sync.Mutex
	banList   map[string]*time.Time
}

func (s *Server) CurrentStats() Stats {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	stats := s.Stats
	stats.CurrentlyConnected = len(s.Clients)

	return *stats
}

type PrivateChat struct {
	Subject    string
	ClientConn map[[2]byte]*ClientConn
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
	s.mux.Lock()
	client, ok := s.Clients[t.clientID]
	s.mux.Unlock()

	if !ok || client == nil {
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

// NewServer constructs a new Server from a config dir
// TODO: move config file reads out of this function
func NewServer(configDir, netInterface string, netPort int, logger *slog.Logger, fs FileStore) (*Server, error) {
	server := Server{
		NetInterface:  netInterface,
		Port:          netPort,
		Accounts:      make(map[string]*Account),
		Config:        new(Config),
		Clients:       make(map[[2]byte]*ClientConn),
		fileTransfers: make(map[[4]byte]*FileTransfer),
		PrivateChats:  make(map[[4]byte]*PrivateChat),
		ConfigDir:     configDir,
		Logger:        logger,
		outbox:        make(chan Transaction),
		Stats:         &Stats{Since: time.Now()},
		ThreadedNews:  &ThreadedNews{},
		FS:            fs,
		banList:       make(map[string]*time.Time),
	}

	var err error

	// generate a new random passID for tracker registration
	if _, err := rand.Read(server.TrackerPassID[:]); err != nil {
		return nil, err
	}

	server.Agreement, err = os.ReadFile(filepath.Join(configDir, agreementFile))
	if err != nil {
		return nil, err
	}

	if server.FlatNews, err = os.ReadFile(filepath.Join(configDir, "MessageBoard.txt")); err != nil {
		return nil, err
	}

	// try to load the ban list, but ignore errors as this file may not be present or may be empty
	//_ = server.loadBanList(filepath.Join(configDir, "Banlist.yaml"))

	_ = loadFromYAMLFile(filepath.Join(configDir, "Banlist.yaml"), &server.banList)

	err = loadFromYAMLFile(filepath.Join(configDir, "ThreadedNews.yaml"), &server.ThreadedNews)
	if err != nil {
		return nil, fmt.Errorf("error loading threaded news: %w", err)
	}

	err = server.loadConfig(filepath.Join(configDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	if err := server.loadAccounts(filepath.Join(configDir, "Users/")); err != nil {
		return nil, err
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

		go func() {
			for {
				tr := &TrackerRegistration{
					UserCount:   server.userCount(),
					PassID:      server.TrackerPassID,
					Name:        server.Config.Name,
					Description: server.Config.Description,
				}
				binary.BigEndian.PutUint16(tr.Port[:], uint16(server.Port))
				for _, t := range server.Config.Trackers {
					if err := register(&RealDialer{}, t, tr); err != nil {
						server.Logger.Error("unable to register with tracker %v", "error", err)
					}
					server.Logger.Debug("Sent Tracker registration", "addr", t)
				}

				time.Sleep(trackerUpdateFrequency * time.Second)
			}
		}()
	}

	// Start Client Keepalive go routine
	go server.keepaliveHandler()

	return &server, nil
}

func (s *Server) userCount() int {
	s.mux.Lock()
	defer s.mux.Unlock()

	return len(s.Clients)
}

func (s *Server) keepaliveHandler() {
	for {
		time.Sleep(idleCheckInterval * time.Second)
		s.mux.Lock()

		for _, c := range s.Clients {
			c.IdleTime += idleCheckInterval
			if c.IdleTime > userIdleSeconds && !c.Idle {
				c.Idle = true

				c.flagsMU.Lock()
				c.Flags.Set(UserFlagAway, 1)
				c.flagsMU.Unlock()
				c.sendAll(
					TranNotifyChangeUser,
					NewField(FieldUserID, c.ID[:]),
					NewField(FieldUserFlags, c.Flags[:]),
					NewField(FieldUserName, c.UserName),
					NewField(FieldUserIconID, c.Icon),
				)
			}
		}
		s.mux.Unlock()
	}
}

func (s *Server) writeBanList() error {
	s.banListMU.Lock()
	defer s.banListMU.Unlock()

	out, err := yaml.Marshal(s.banList)
	if err != nil {
		return err
	}
	err = os.WriteFile(
		filepath.Join(s.ConfigDir, "Banlist.yaml"),
		out,
		0666,
	)
	return err
}

func (s *Server) writeThreadedNews() error {
	s.threadedNewsMux.Lock()
	defer s.threadedNewsMux.Unlock()

	out, err := yaml.Marshal(s.ThreadedNews)
	if err != nil {
		return err
	}
	err = s.FS.WriteFile(
		filepath.Join(s.ConfigDir, "ThreadedNews.yaml"),
		out,
		0666,
	)
	return err
}

func (s *Server) NewClientConn(conn io.ReadWriteCloser, remoteAddr string) *ClientConn {
	s.mux.Lock()
	defer s.mux.Unlock()

	clientConn := &ClientConn{
		Icon:       []byte{0, 0}, // TODO: make array type
		Connection: conn,
		Server:     s,
		RemoteAddr: remoteAddr,
		transfers: map[int]map[[4]byte]*FileTransfer{
			FileDownload:   {},
			FileUpload:     {},
			FolderDownload: {},
			FolderUpload:   {},
			bannerDownload: {},
		},
	}

	s.nextClientID.Add(1)

	binary.BigEndian.PutUint16(clientConn.ID[:], uint16(s.nextClientID.Load()))
	s.Clients[clientConn.ID] = clientConn

	return clientConn
}

// NewUser creates a new user account entry in the server map and config file
func (s *Server) NewUser(login, name, password string, access accessBitmap) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	account := NewAccount(login, name, password, access)

	// Create account file, returning an error if one already exists.
	file, err := os.OpenFile(
		filepath.Join(s.ConfigDir, "Users", path.Join("/", login)+".yaml"),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644,
	)
	if err != nil {
		return fmt.Errorf("error creating account file: %w", err)
	}
	defer file.Close()

	b, err := yaml.Marshal(account)
	if err != nil {
		return err
	}

	_, err = file.Write(b)
	if err != nil {
		return fmt.Errorf("error writing account file: %w", err)
	}

	s.Accounts[login] = account

	return nil
}

func (s *Server) UpdateUser(login, newLogin, name, password string, access accessBitmap) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	// If the login has changed, rename the account file.
	if login != newLogin {
		err := os.Rename(
			filepath.Join(s.ConfigDir, "Users", path.Join("/", login)+".yaml"),
			filepath.Join(s.ConfigDir, "Users", path.Join("/", newLogin)+".yaml"),
		)
		if err != nil {
			return fmt.Errorf("error renaming account file: %w", err)
		}
		s.Accounts[newLogin] = s.Accounts[login]
		s.Accounts[newLogin].Login = newLogin
		delete(s.Accounts, login)
	}

	account := s.Accounts[newLogin]
	account.Access = access
	account.Name = name
	account.Password = password

	out, err := yaml.Marshal(&account)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(s.ConfigDir, "Users", newLogin+".yaml"), out, 0666); err != nil {
		return fmt.Errorf("error writing account file: %w", err)
	}

	return nil
}

// DeleteUser deletes the user account
func (s *Server) DeleteUser(login string) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	err := s.FS.Remove(filepath.Join(s.ConfigDir, "Users", path.Join("/", login)+".yaml"))
	if err != nil {
		return err
	}

	delete(s.Accounts, login)

	return nil
}

func (s *Server) connectedUsers() []Field {
	//s.mux.Lock()
	//defer s.mux.Unlock()

	var connectedUsers []Field
	for _, c := range sortedClients(s.Clients) {
		b, err := io.ReadAll(&User{
			ID:    c.ID,
			Icon:  c.Icon,
			Flags: c.Flags[:],
			Name:  string(c.UserName),
		})
		if err != nil {
			return nil
		}
		connectedUsers = append(connectedUsers, NewField(FieldUsernameWithInfo, b))
	}
	return connectedUsers
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

// loadAccounts loads account data from disk
func (s *Server) loadAccounts(userDir string) error {
	matches, err := filepath.Glob(filepath.Join(userDir, "*.yaml"))
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return fmt.Errorf("no accounts found in directory: %s", userDir)
	}

	for _, file := range matches {
		var account Account
		if err = loadFromYAMLFile(file, &account); err != nil {
			return fmt.Errorf("error loading account %s: %w", file, err)
		}

		s.Accounts[account.Login] = &account
	}
	return nil
}

func (s *Server) loadConfig(path string) error {
	fh, err := s.FS.Open(path)
	if err != nil {
		return err
	}

	decoder := yaml.NewDecoder(fh)
	err = decoder.Decode(s.Config)
	if err != nil {
		return err
	}

	validate := validator.New()
	err = validate.Struct(s.Config)
	if err != nil {
		return err
	}
	return nil
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
	if banUntil, ok := s.banList[ipAddr]; ok {
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

	login := string(encodeString(clientLogin.GetField(FieldUserLogin).Data))
	if login == "" {
		login = GuestAccount
	}

	c.logger = s.Logger.With("remoteAddr", remoteAddr, "login", login)

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

	c.Lock()
	c.Account = c.Server.Accounts[login]
	c.Unlock()

	if clientLogin.GetField(FieldUserName).Data != nil {
		if c.Authorize(accessAnyName) {
			c.UserName = clientLogin.GetField(FieldUserName).Data
		} else {
			c.UserName = []byte(c.Account.Name)
		}
	}

	if c.Authorize(accessDisconUser) {
		c.Flags.Set(UserFlagAdmin, 1)
	}

	s.outbox <- c.NewReply(&clientLogin,
		NewField(FieldVersion, []byte{0x00, 0xbe}),
		NewField(FieldCommunityBannerID, []byte{0, 0}),
		NewField(FieldServerName, []byte(s.Config.Name)),
	)

	// Send user access privs so client UI knows how to behave
	c.Server.outbox <- NewTransaction(TranUserAccess, c.ID, NewField(FieldUserAccess, c.Account.Access[:]))

	// Accounts with accessNoAgreement do not receive the server agreement on login.  The behavior is different between
	// client versions.  For 1.2.3 client, we do not send TranShowAgreement.  For other client versions, we send
	// TranShowAgreement but with the NoServerAgreement field set to 1.
	if c.Authorize(accessNoAgreement) {
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
		for _, t := range c.notifyOthers(
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

	c.Server.mux.Lock()
	c.Server.Stats.ConnectionCounter += 1
	if len(s.Clients) > c.Server.Stats.ConnectionPeak {
		c.Server.Stats.ConnectionPeak = len(s.Clients)
	}
	c.Server.mux.Unlock()

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

func (s *Server) NewPrivateChat(cc *ClientConn) [4]byte {
	s.PrivateChatsMu.Lock()
	defer s.PrivateChatsMu.Unlock()

	var randID [4]byte
	_, _ = rand.Read(randID[:])

	s.PrivateChats[randID] = &PrivateChat{
		ClientConn: make(map[[2]byte]*ClientConn),
	}
	s.PrivateChats[randID].ClientConn[cc.ID] = cc

	return randID
}

const dlFldrActionSendFile = 1
const dlFldrActionResumeFile = 2
const dlFldrActionNextFile = 3

// handleFileTransfer receives a client net.Conn from the file transfer server, performs the requested transfer type, then closes the connection
func (s *Server) handleFileTransfer(ctx context.Context, rwc io.ReadWriter) error {
	defer dontPanic(s.Logger)

	// The first 16 bytes contain the file transfer.
	var t transfer
	if _, err := io.CopyN(&t, rwc, 16); err != nil {
		return fmt.Errorf("error reading file transfer: %w", err)
	}

	defer func() {
		s.mux.Lock()
		delete(s.fileTransfers, t.ReferenceNumber)
		s.mux.Unlock()

		// Wait a few seconds before closing the connection: this is a workaround for problems
		// observed with Windows clients where the client must initiate close of the TCP connection before
		// the server does.  This is gross and seems unnecessary.  TODO: Revisit?
		time.Sleep(3 * time.Second)
	}()

	s.mux.Lock()
	fileTransfer, ok := s.fileTransfers[t.ReferenceNumber]
	s.mux.Unlock()
	if !ok {
		return errors.New("invalid transaction ID")
	}

	defer func() {
		fileTransfer.ClientConn.transfersMU.Lock()
		delete(fileTransfer.ClientConn.transfers[fileTransfer.Type], t.ReferenceNumber)
		fileTransfer.ClientConn.transfersMU.Unlock()
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
	case bannerDownload:
		if _, err := io.Copy(rwc, bytes.NewBuffer(s.banner)); err != nil {
			return fmt.Errorf("error sending banner: %w", err)
		}
	case FileDownload:
		s.Stats.DownloadCounter += 1
		s.Stats.DownloadsInProgress += 1
		defer func() {
			s.Stats.DownloadsInProgress -= 1
		}()

		err = DownloadHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, true)
		if err != nil {
			return fmt.Errorf("file download error: %w", err)
		}

	case FileUpload:
		s.Stats.UploadCounter += 1
		s.Stats.UploadsInProgress += 1
		defer func() { s.Stats.UploadsInProgress -= 1 }()

		err = UploadHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, s.Config.PreserveResourceForks)
		if err != nil {
			return fmt.Errorf("file upload error: %w", err)
		}

	case FolderDownload:
		s.Stats.DownloadCounter += 1
		s.Stats.DownloadsInProgress += 1
		defer func() { s.Stats.DownloadsInProgress -= 1 }()

		err = DownloadFolderHandler(rwc, fullPath, fileTransfer, s.FS, rLogger, s.Config.PreserveResourceForks)
		if err != nil {
			return fmt.Errorf("file upload error: %w", err)
		}

	case FolderUpload:
		s.Stats.UploadCounter += 1
		s.Stats.UploadsInProgress += 1
		defer func() { s.Stats.UploadsInProgress -= 1 }()
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
