package hotline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"math/big"
	"math/rand"
	"net"
	"os"
	"path"
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
	NetInterface  string
	Port          int
	Accounts      map[string]*Account
	Agreement     []byte
	Clients       map[uint16]*ClientConn
	fileTransfers map[[4]byte]*FileTransfer

	Config    *Config
	ConfigDir string
	Logger    *slog.Logger
	banner    []byte

	PrivateChatsMu sync.Mutex
	PrivateChats   map[uint32]*PrivateChat

	NextGuestID   *uint16
	TrackerPassID [4]byte

	StatsMu sync.Mutex
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
	s.StatsMu.Lock()
	defer s.StatsMu.Unlock()

	stats := s.Stats
	stats.CurrentlyConnected = len(s.Clients)

	return *stats
}

type PrivateChat struct {
	Subject    string
	ClientConn map[uint16]*ClientConn
}

func (s *Server) ListenAndServe(ctx context.Context, cancelRoot context.CancelFunc) error {
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
				context.WithValue(ctx, contextKeyReq, requestCtx{
					remoteAddr: conn.RemoteAddr().String(),
				}),
				conn,
			)

			if err != nil {
				s.Logger.Error("file transfer error", "reason", err)
			}
		}()
	}
}

func (s *Server) sendTransaction(t Transaction) error {
	clientID, err := byteToInt(*t.clientID)
	if err != nil {
		return fmt.Errorf("invalid client ID: %v", err)
	}

	s.mux.Lock()
	client, ok := s.Clients[uint16(clientID)]
	s.mux.Unlock()
	if !ok || client == nil {
		return fmt.Errorf("invalid client id %v", *t.clientID)
	}

	_, err = io.Copy(client.Connection, &t)
	if err != nil {
		return fmt.Errorf("failed to send transaction to client %v: %v", clientID, err)
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
func NewServer(configDir, netInterface string, netPort int, logger *slog.Logger, fs FileStore) (*Server, error) {
	server := Server{
		NetInterface:  netInterface,
		Port:          netPort,
		Accounts:      make(map[string]*Account),
		Config:        new(Config),
		Clients:       make(map[uint16]*ClientConn),
		fileTransfers: make(map[[4]byte]*FileTransfer),
		PrivateChats:  make(map[uint32]*PrivateChat),
		ConfigDir:     configDir,
		Logger:        logger,
		NextGuestID:   new(uint16),
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
	_ = server.loadBanList(filepath.Join(configDir, "Banlist.yaml"))

	if err := server.loadThreadedNews(filepath.Join(configDir, "ThreadedNews.yaml")); err != nil {
		return nil, err
	}

	if err := server.loadConfig(filepath.Join(configDir, "config.yaml")); err != nil {
		return nil, err
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

	*server.NextGuestID = 1

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
					if err := register(t, tr); err != nil {
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

				flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(c.Flags)))
				flagBitmap.SetBit(flagBitmap, UserFlagAway, 1)
				binary.BigEndian.PutUint16(c.Flags, uint16(flagBitmap.Int64()))

				c.sendAll(
					TranNotifyChangeUser,
					NewField(FieldUserID, *c.ID),
					NewField(FieldUserFlags, c.Flags),
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
		ID:         &[]byte{0, 0},
		Icon:       []byte{0, 0},
		Flags:      []byte{0, 0},
		UserName:   []byte{},
		Connection: conn,
		Server:     s,
		Version:    []byte{},
		AutoReply:  []byte{},
		RemoteAddr: remoteAddr,
		transfers: map[int]map[[4]byte]*FileTransfer{
			FileDownload:   {},
			FileUpload:     {},
			FolderDownload: {},
			FolderUpload:   {},
			bannerDownload: {},
		},
	}

	*s.NextGuestID++
	ID := *s.NextGuestID

	binary.BigEndian.PutUint16(*clientConn.ID, ID)
	s.Clients[ID] = clientConn

	return clientConn
}

// NewUser creates a new user account entry in the server map and config file
func (s *Server) NewUser(login, name, password string, access accessBitmap) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	account := Account{
		Login:    login,
		Name:     name,
		Password: hashAndSalt([]byte(password)),
		Access:   access,
	}
	out, err := yaml.Marshal(&account)
	if err != nil {
		return err
	}

	// Create account file, returning an error if one already exists.
	file, err := os.OpenFile(
		filepath.Join(s.ConfigDir, "Users", path.Join("/", login)+".yaml"),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(out)
	if err != nil {
		return fmt.Errorf("error writing account file: %w", err)
	}

	s.Accounts[login] = &account

	return nil
}

func (s *Server) UpdateUser(login, newLogin, name, password string, access accessBitmap) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	// update renames the user login
	if login != newLogin {
		err := os.Rename(filepath.Join(s.ConfigDir, "Users", path.Join("/", login)+".yaml"), filepath.Join(s.ConfigDir, "Users", path.Join("/", newLogin)+".yaml"))
		if err != nil {
			return fmt.Errorf("unable to rename account: %w", err)
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
		return err
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
	s.mux.Lock()
	defer s.mux.Unlock()

	var connectedUsers []Field
	for _, c := range sortedClients(s.Clients) {
		b, err := io.ReadAll(&User{
			ID:    *c.ID,
			Icon:  c.Icon,
			Flags: c.Flags,
			Name:  string(c.UserName),
		})
		if err != nil {
			return nil
		}
		connectedUsers = append(connectedUsers, NewField(FieldUsernameWithInfo, b))
	}
	return connectedUsers
}

func (s *Server) loadBanList(path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(fh)

	return decoder.Decode(s.banList)
}

// loadThreadedNews loads the threaded news data from disk
func (s *Server) loadThreadedNews(threadedNewsPath string) error {
	fh, err := os.Open(threadedNewsPath)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(fh)

	return decoder.Decode(s.ThreadedNews)
}

// loadAccounts loads account data from disk
func (s *Server) loadAccounts(userDir string) error {
	matches, err := filepath.Glob(filepath.Join(userDir, "*.yaml"))
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return errors.New("no user accounts found in " + userDir)
	}

	for _, file := range matches {
		fh, err := s.FS.Open(file)
		if err != nil {
			return err
		}

		account := Account{}
		decoder := yaml.NewDecoder(fh)
		if err = decoder.Decode(&account); err != nil {
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

// handleNewConnection takes a new net.Conn and performs the initial login sequence
func (s *Server) handleNewConnection(ctx context.Context, rwc io.ReadWriteCloser, remoteAddr string) error {
	defer dontPanic(s.Logger)

	if err := Handshake(rwc); err != nil {
		return err
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
		return err
	}

	// check if remoteAddr is present in the ban list
	if banUntil, ok := s.banList[strings.Split(remoteAddr, ":")[0]]; ok {
		// permaban
		if banUntil == nil {
			t := NewTransaction(
				TranServerMsg,
				&[]byte{0, 0},
				NewField(FieldData, []byte("You are permanently banned on this server")),
				NewField(FieldChatOptions, []byte{0, 0}),
			)

			_, err := io.Copy(rwc, t)
			if err != nil {
				return err
			}

			time.Sleep(1 * time.Second)
			return nil
		}

		// temporary ban
		if time.Now().Before(*banUntil) {
			t := NewTransaction(
				TranServerMsg,
				&[]byte{0, 0},
				NewField(FieldData, []byte("You are temporarily banned on this server")),
				NewField(FieldChatOptions, []byte{0, 0}),
			)

			_, err := io.Copy(rwc, t)
			if err != nil {
				return err
			}

			time.Sleep(1 * time.Second)
			return nil
		}
	}

	c := s.NewClientConn(rwc, remoteAddr)
	defer c.Disconnect()

	encodedLogin := clientLogin.GetField(FieldUserLogin).Data
	encodedPassword := clientLogin.GetField(FieldUserPassword).Data
	c.Version = clientLogin.GetField(FieldVersion).Data

	var login string
	for _, char := range encodedLogin {
		login += string(rune(255 - uint(char)))
	}
	if login == "" {
		login = GuestAccount
	}

	c.logger = s.Logger.With("remoteAddr", remoteAddr, "login", login)

	// If authentication fails, send error reply and close connection
	if !c.Authenticate(login, encodedPassword) {
		t := c.NewErrReply(&clientLogin, "Incorrect login.")

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

	c.Account = c.Server.Accounts[login]

	if clientLogin.GetField(FieldUserName).Data != nil {
		if c.Authorize(accessAnyName) {
			c.UserName = clientLogin.GetField(FieldUserName).Data
		} else {
			c.UserName = []byte(c.Account.Name)
		}
	}

	if c.Authorize(accessDisconUser) {
		c.Flags = []byte{0, 2}
	}

	s.outbox <- c.NewReply(&clientLogin,
		NewField(FieldVersion, []byte{0x00, 0xbe}),
		NewField(FieldCommunityBannerID, []byte{0, 0}),
		NewField(FieldServerName, []byte(s.Config.Name)),
	)

	// Send user access privs so client UI knows how to behave
	c.Server.outbox <- *NewTransaction(TranUserAccess, c.ID, NewField(FieldUserAccess, c.Account.Access[:]))

	// Accounts with accessNoAgreement do not receive the server agreement on login.  The behavior is different between
	// client versions.  For 1.2.3 client, we do not send TranShowAgreement.  For other client versions, we send
	// TranShowAgreement but with the NoServerAgreement field set to 1.
	if c.Authorize(accessNoAgreement) {
		// If client version is nil, then the client uses the 1.2.3 login behavior
		if c.Version != nil {
			c.Server.outbox <- *NewTransaction(TranShowAgreement, c.ID, NewField(FieldNoServerAgreement, []byte{1}))
		}
	} else {
		c.Server.outbox <- *NewTransaction(TranShowAgreement, c.ID, NewField(FieldData, s.Agreement))
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
			*NewTransaction(
				TranNotifyChangeUser, nil,
				NewField(FieldUserName, c.UserName),
				NewField(FieldUserID, *c.ID),
				NewField(FieldUserIconID, c.Icon),
				NewField(FieldUserFlags, c.Flags),
			),
		) {
			c.Server.outbox <- t
		}
	}

	c.Server.Stats.ConnectionCounter += 1
	if len(s.Clients) > c.Server.Stats.ConnectionPeak {
		c.Server.Stats.ConnectionPeak = len(s.Clients)
	}

	// Scan for new transactions and handle them as they come in.
	for scanner.Scan() {
		// Make a new []byte slice and copy the scanner bytes to it.  This is critical to avoid a data race as the
		// scanner re-uses the buffer for subsequent scans.
		buf := make([]byte, len(scanner.Bytes()))
		copy(buf, scanner.Bytes())

		var t Transaction
		if _, err := t.Write(buf); err != nil {
			return err
		}

		if err := c.handleTransaction(t); err != nil {
			c.logger.Error("Error handling transaction", "err", err)
		}
	}
	return nil
}

func (s *Server) NewPrivateChat(cc *ClientConn) []byte {
	s.PrivateChatsMu.Lock()
	defer s.PrivateChatsMu.Unlock()

	randID := make([]byte, 4)
	rand.Read(randID)
	data := binary.BigEndian.Uint32(randID)

	s.PrivateChats[data] = &PrivateChat{
		ClientConn: make(map[uint16]*ClientConn),
	}
	s.PrivateChats[data].ClientConn[cc.uint16ID()] = cc

	return randID
}

const dlFldrActionSendFile = 1
const dlFldrActionResumeFile = 2
const dlFldrActionNextFile = 3

// handleFileTransfer receives a client net.Conn from the file transfer server, performs the requested transfer type, then closes the connection
func (s *Server) handleFileTransfer(ctx context.Context, rwc io.ReadWriter) error {
	defer dontPanic(s.Logger)

	txBuf := make([]byte, 16)
	if _, err := io.ReadFull(rwc, txBuf); err != nil {
		return err
	}

	var t transfer
	if _, err := t.Write(txBuf); err != nil {
		return err
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

		var dataOffset int64
		if fileTransfer.fileResumeData != nil {
			dataOffset = int64(binary.BigEndian.Uint32(fileTransfer.fileResumeData.ForkInfoList[0].DataSize[:]))
		}

		fw, err := newFileWrapper(s.FS, fullPath, 0)
		if err != nil {
			return err
		}

		rLogger.Info("File download started", "filePath", fullPath)

		// if file transfer options are included, that means this is a "quick preview" request from a 1.5+ client
		if fileTransfer.options == nil {
			_, err = io.Copy(rwc, fw.ffo)
			if err != nil {
				return err
			}
		}

		file, err := fw.dataForkReader()
		if err != nil {
			return err
		}

		br := bufio.NewReader(file)
		if _, err := br.Discard(int(dataOffset)); err != nil {
			return err
		}

		if _, err = io.Copy(rwc, io.TeeReader(br, fileTransfer.bytesSentCounter)); err != nil {
			return err
		}

		// if the client requested to resume transfer, do not send the resource fork header, or it will be appended into the fileWrapper data
		if fileTransfer.fileResumeData == nil {
			err = binary.Write(rwc, binary.BigEndian, fw.rsrcForkHeader())
			if err != nil {
				return err
			}
		}

		rFile, err := fw.rsrcForkFile()
		if err != nil {
			return nil
		}

		if _, err = io.Copy(rwc, io.TeeReader(rFile, fileTransfer.bytesSentCounter)); err != nil {
			return err
		}

	case FileUpload:
		s.Stats.UploadCounter += 1
		s.Stats.UploadsInProgress += 1
		defer func() { s.Stats.UploadsInProgress -= 1 }()

		var file *os.File

		// A file upload has three possible cases:
		// 1) Upload a new file
		// 2) Resume a partially transferred file
		// 3) Replace a fully uploaded file
		//  We have to infer which case applies by inspecting what is already on the filesystem

		// 1) Check for existing file:
		_, err = os.Stat(fullPath)
		if err == nil {
			return errors.New("existing file found at " + fullPath)
		}
		if errors.Is(err, fs.ErrNotExist) {
			// If not found, open or create a new .incomplete file
			file, err = os.OpenFile(fullPath+incompleteFileSuffix, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
		}

		f, err := newFileWrapper(s.FS, fullPath, 0)
		if err != nil {
			return err
		}

		rLogger.Info("File upload started", "dstFile", fullPath)

		rForkWriter := io.Discard
		iForkWriter := io.Discard
		if s.Config.PreserveResourceForks {
			rForkWriter, err = f.rsrcForkWriter()
			if err != nil {
				return err
			}

			iForkWriter, err = f.infoForkWriter()
			if err != nil {
				return err
			}
		}

		if err := receiveFile(rwc, file, rForkWriter, iForkWriter, fileTransfer.bytesSentCounter); err != nil {
			s.Logger.Error(err.Error())
		}

		if err := file.Close(); err != nil {
			return err
		}

		if err := s.FS.Rename(fullPath+".incomplete", fullPath); err != nil {
			return err
		}

		rLogger.Info("File upload complete", "dstFile", fullPath)

	case FolderDownload:
		s.Stats.DownloadCounter += 1
		s.Stats.DownloadsInProgress += 1
		defer func() { s.Stats.DownloadsInProgress -= 1 }()

		// Folder Download flow:
		// 1. Get filePath from the transfer
		// 2. Iterate over files
		// 3. For each fileWrapper:
		// 	 Send fileWrapper header to client
		// The client can reply in 3 ways:
		//
		// 1. If type is an odd number (unknown type?), or fileWrapper download for the current fileWrapper is completed:
		//		client sends []byte{0x00, 0x03} to tell the server to continue to the next fileWrapper
		//
		// 2. If download of a fileWrapper is to be resumed:
		//		client sends:
		//			[]byte{0x00, 0x02} // download folder action
		//			[2]byte // Resume data size
		//			[]byte fileWrapper resume data (see myField_FileResumeData)
		//
		// 3. Otherwise, download of the fileWrapper is requested and client sends []byte{0x00, 0x01}
		//
		// When download is requested (case 2 or 3), server replies with:
		// 			[4]byte - fileWrapper size
		//			[]byte  - Flattened File Object
		//
		// After every fileWrapper download, client could request next fileWrapper with:
		// 			[]byte{0x00, 0x03}
		//
		// This notifies the server to send the next item header

		basePathLen := len(fullPath)

		rLogger.Info("Start folder download", "path", fullPath)

		nextAction := make([]byte, 2)
		if _, err := io.ReadFull(rwc, nextAction); err != nil {
			return err
		}

		i := 0
		err = filepath.Walk(fullPath+"/", func(path string, info os.FileInfo, err error) error {
			s.Stats.DownloadCounter += 1
			i += 1

			if err != nil {
				return err
			}

			// skip dot files
			if strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			hlFile, err := newFileWrapper(s.FS, path, 0)
			if err != nil {
				return err
			}

			subPath := path[basePathLen+1:]
			rLogger.Debug("Sending fileheader", "i", i, "path", path, "fullFilePath", fullPath, "subPath", subPath, "IsDir", info.IsDir())

			if i == 1 {
				return nil
			}

			fileHeader := NewFileHeader(subPath, info.IsDir())
			if _, err := io.Copy(rwc, &fileHeader); err != nil {
				return fmt.Errorf("error sending file header: %w", err)
			}

			// Read the client's Next Action request
			if _, err := io.ReadFull(rwc, nextAction); err != nil {
				return err
			}

			rLogger.Debug("Client folder download action", "action", fmt.Sprintf("%X", nextAction[0:2]))

			var dataOffset int64

			switch nextAction[1] {
			case dlFldrActionResumeFile:
				// get size of resumeData
				resumeDataByteLen := make([]byte, 2)
				if _, err := io.ReadFull(rwc, resumeDataByteLen); err != nil {
					return err
				}

				resumeDataLen := binary.BigEndian.Uint16(resumeDataByteLen)
				resumeDataBytes := make([]byte, resumeDataLen)
				if _, err := io.ReadFull(rwc, resumeDataBytes); err != nil {
					return err
				}

				var frd FileResumeData
				if err := frd.UnmarshalBinary(resumeDataBytes); err != nil {
					return err
				}
				dataOffset = int64(binary.BigEndian.Uint32(frd.ForkInfoList[0].DataSize[:]))
			case dlFldrActionNextFile:
				// client asked to skip this file
				return nil
			}

			if info.IsDir() {
				return nil
			}

			rLogger.Info("File download started",
				"fileName", info.Name(),
				"TransferSize", fmt.Sprintf("%x", hlFile.ffo.TransferSize(dataOffset)),
			)

			// Send file size to client
			if _, err := rwc.Write(hlFile.ffo.TransferSize(dataOffset)); err != nil {
				s.Logger.Error(err.Error())
				return err
			}

			// Send ffo bytes to client
			_, err = io.Copy(rwc, hlFile.ffo)
			if err != nil {
				return err
			}

			file, err := s.FS.Open(path)
			if err != nil {
				return err
			}

			// wr := bufio.NewWriterSize(rwc, 1460)
			if _, err = io.Copy(rwc, io.TeeReader(file, fileTransfer.bytesSentCounter)); err != nil {
				return err
			}

			if nextAction[1] != 2 && hlFile.ffo.FlatFileHeader.ForkCount[1] == 3 {
				err = binary.Write(rwc, binary.BigEndian, hlFile.rsrcForkHeader())
				if err != nil {
					return err
				}

				rFile, err := hlFile.rsrcForkFile()
				if err != nil {
					return err
				}

				if _, err = io.Copy(rwc, io.TeeReader(rFile, fileTransfer.bytesSentCounter)); err != nil {
					return err
				}
			}

			// Read the client's Next Action request.  This is always 3, I think?
			if _, err := io.ReadFull(rwc, nextAction); err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			return err
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

		// Check if the target folder exists.  If not, create it.
		if _, err := s.FS.Stat(fullPath); os.IsNotExist(err) {
			if err := s.FS.Mkdir(fullPath, 0777); err != nil {
				return err
			}
		}

		// Begin the folder upload flow by sending the "next file action" to client
		if _, err := rwc.Write([]byte{0, dlFldrActionNextFile}); err != nil {
			return err
		}

		fileSize := make([]byte, 4)

		for i := 0; i < fileTransfer.ItemCount(); i++ {
			s.Stats.UploadCounter += 1

			var fu folderUpload
			if _, err := io.ReadFull(rwc, fu.DataSize[:]); err != nil {
				return err
			}
			if _, err := io.ReadFull(rwc, fu.IsFolder[:]); err != nil {
				return err
			}
			if _, err := io.ReadFull(rwc, fu.PathItemCount[:]); err != nil {
				return err
			}

			fu.FileNamePath = make([]byte, binary.BigEndian.Uint16(fu.DataSize[:])-4) // -4 to subtract the path separator bytes

			if _, err := io.ReadFull(rwc, fu.FileNamePath); err != nil {
				return err
			}

			rLogger.Info(
				"Folder upload continued",
				"FormattedPath", fu.FormattedPath(),
				"IsFolder", fmt.Sprintf("%x", fu.IsFolder),
				"PathItemCount", binary.BigEndian.Uint16(fu.PathItemCount[:]),
			)

			if fu.IsFolder == [2]byte{0, 1} {
				if _, err := os.Stat(filepath.Join(fullPath, fu.FormattedPath())); os.IsNotExist(err) {
					if err := os.Mkdir(filepath.Join(fullPath, fu.FormattedPath()), 0777); err != nil {
						return err
					}
				}

				// Tell client to send next file
				if _, err := rwc.Write([]byte{0, dlFldrActionNextFile}); err != nil {
					return err
				}
			} else {
				nextAction := dlFldrActionSendFile

				// Check if we have the full file already.  If so, send dlFldrAction_NextFile to client to skip.
				_, err = os.Stat(filepath.Join(fullPath, fu.FormattedPath()))
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				if err == nil {
					nextAction = dlFldrActionNextFile
				}

				//  Check if we have a partial file already.  If so, send dlFldrAction_ResumeFile to client to resume upload.
				incompleteFile, err := os.Stat(filepath.Join(fullPath, fu.FormattedPath()+incompleteFileSuffix))
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				if err == nil {
					nextAction = dlFldrActionResumeFile
				}

				if _, err := rwc.Write([]byte{0, uint8(nextAction)}); err != nil {
					return err
				}

				switch nextAction {
				case dlFldrActionNextFile:
					continue
				case dlFldrActionResumeFile:
					offset := make([]byte, 4)
					binary.BigEndian.PutUint32(offset, uint32(incompleteFile.Size()))

					file, err := os.OpenFile(fullPath+"/"+fu.FormattedPath()+incompleteFileSuffix, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err != nil {
						return err
					}

					fileResumeData := NewFileResumeData([]ForkInfoList{*NewForkInfoList(offset)})

					b, _ := fileResumeData.BinaryMarshal()

					bs := make([]byte, 2)
					binary.BigEndian.PutUint16(bs, uint16(len(b)))

					if _, err := rwc.Write(append(bs, b...)); err != nil {
						return err
					}

					if _, err := io.ReadFull(rwc, fileSize); err != nil {
						return err
					}

					if err := receiveFile(rwc, file, io.Discard, io.Discard, fileTransfer.bytesSentCounter); err != nil {
						s.Logger.Error(err.Error())
					}

					err = os.Rename(fullPath+"/"+fu.FormattedPath()+".incomplete", fullPath+"/"+fu.FormattedPath())
					if err != nil {
						return err
					}

				case dlFldrActionSendFile:
					if _, err := io.ReadFull(rwc, fileSize); err != nil {
						return err
					}

					filePath := filepath.Join(fullPath, fu.FormattedPath())

					hlFile, err := newFileWrapper(s.FS, filePath, 0)
					if err != nil {
						return err
					}

					rLogger.Info("Starting file transfer", "path", filePath, "fileNum", i+1, "fileSize", binary.BigEndian.Uint32(fileSize))

					incWriter, err := hlFile.incFileWriter()
					if err != nil {
						return err
					}

					rForkWriter := io.Discard
					iForkWriter := io.Discard
					if s.Config.PreserveResourceForks {
						iForkWriter, err = hlFile.infoForkWriter()
						if err != nil {
							return err
						}

						rForkWriter, err = hlFile.rsrcForkWriter()
						if err != nil {
							return err
						}
					}
					if err := receiveFile(rwc, incWriter, rForkWriter, iForkWriter, fileTransfer.bytesSentCounter); err != nil {
						return err
					}

					if err := os.Rename(filePath+".incomplete", filePath); err != nil {
						return err
					}
				}

				// Tell client to send next fileWrapper
				if _, err := rwc.Write([]byte{0, dlFldrActionNextFile}); err != nil {
					return err
				}
			}
		}
		rLogger.Info("Folder upload complete")
	}

	return nil
}
