package hotline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type contextKey string

var contextKeyReq = contextKey("req")

type requestCtx struct {
	remoteAddr string
	login      string
	name       string
}

const (
	userIdleSeconds        = 300 // time in seconds before an inactive user is marked idle
	idleCheckInterval      = 10  // time in seconds to check for idle users
	trackerUpdateFrequency = 300 // time in seconds between tracker re-registration
)

var nostalgiaVersion = []byte{0, 0, 2, 0x2c} // version ID used by the Nostalgia client

type Server struct {
	Port         int
	Accounts     map[string]*Account
	Agreement    []byte
	Clients      map[uint16]*ClientConn
	ThreadedNews *ThreadedNews

	fileTransfers map[[4]byte]*FileTransfer

	Config        *Config
	ConfigDir     string
	Logger        *zap.SugaredLogger
	PrivateChats  map[uint32]*PrivateChat
	NextGuestID   *uint16
	TrackerPassID [4]byte
	Stats         *Stats

	FS FileStore // Storage backend to use for File storage

	outbox chan Transaction
	mux    sync.Mutex

	flatNewsMux sync.Mutex
	FlatNews    []byte

	banListMU sync.Mutex
	banList   map[string]*time.Time
}

type PrivateChat struct {
	Subject    string
	ClientConn map[uint16]*ClientConn
}

func (s *Server) ListenAndServe(ctx context.Context, cancelRoot context.CancelFunc) error {
	s.Logger.Infow("Hotline server started",
		"version", VERSION,
		"API port", fmt.Sprintf(":%v", s.Port),
		"Transfer port", fmt.Sprintf(":%v", s.Port+1),
	)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%v", "", s.Port))
		if err != nil {
			s.Logger.Fatal(err)
		}

		s.Logger.Fatal(s.Serve(ctx, ln))
	}()

	wg.Add(1)
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%v", "", s.Port+1))
		if err != nil {
			s.Logger.Fatal(err)

		}

		s.Logger.Fatal(s.ServeFileTransfers(ctx, ln))
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
				s.Logger.Errorw("file transfer error", "reason", err)
			}
		}()
	}
}

func (s *Server) sendTransaction(t Transaction) error {
	clientID, err := byteToInt(*t.clientID)
	if err != nil {
		return err
	}

	s.mux.Lock()
	client := s.Clients[uint16(clientID)]
	if client == nil {
		return fmt.Errorf("invalid client id %v", *t.clientID)
	}

	s.mux.Unlock()

	b, err := t.MarshalBinary()
	if err != nil {
		return err
	}

	if _, err := client.Connection.Write(b); err != nil {
		return err
	}

	return nil
}

func (s *Server) processOutbox() {
	for {
		t := <-s.outbox
		go func() {
			if err := s.sendTransaction(t); err != nil {
				s.Logger.Errorw("error sending transaction", "err", err)
			}
		}()
	}
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go s.processOutbox()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.Logger.Errorw("error accepting connection", "err", err)
		}
		connCtx := context.WithValue(ctx, contextKeyReq, requestCtx{
			remoteAddr: conn.RemoteAddr().String(),
		})

		go func() {
			s.Logger.Infow("Connection established", "RemoteAddr", conn.RemoteAddr())

			defer conn.Close()
			if err := s.handleNewConnection(connCtx, conn, conn.RemoteAddr().String()); err != nil {
				if err == io.EOF {
					s.Logger.Infow("Client disconnected", "RemoteAddr", conn.RemoteAddr())
				} else {
					s.Logger.Errorw("error serving request", "RemoteAddr", conn.RemoteAddr(), "err", err)
				}
			}
		}()
	}
}

const (
	agreementFile = "Agreement.txt"
)

// NewServer constructs a new Server from a config dir
func NewServer(configDir string, netPort int, logger *zap.SugaredLogger, FS FileStore) (*Server, error) {
	server := Server{
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
		Stats:         &Stats{StartTime: time.Now()},
		ThreadedNews:  &ThreadedNews{},
		FS:            FS,
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

	server.Config.FileRoot = filepath.Join(configDir, "Files")

	*server.NextGuestID = 1

	if server.Config.EnableTrackerRegistration {
		server.Logger.Infow(
			"Tracker registration enabled",
			"frequency", fmt.Sprintf("%vs", trackerUpdateFrequency),
			"trackers", server.Config.Trackers,
		)

		go func() {
			for {
				tr := &TrackerRegistration{
					UserCount:   server.userCount(),
					PassID:      server.TrackerPassID[:],
					Name:        server.Config.Name,
					Description: server.Config.Description,
				}
				binary.BigEndian.PutUint16(tr.Port[:], uint16(server.Port))
				for _, t := range server.Config.Trackers {
					if err := register(t, tr); err != nil {
						server.Logger.Errorw("unable to register with tracker %v", "error", err)
					}
					server.Logger.Infow("Sent Tracker registration", "data", tr)
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

				flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*c.Flags)))
				flagBitmap.SetBit(flagBitmap, userFlagAway, 1)
				binary.BigEndian.PutUint16(*c.Flags, uint16(flagBitmap.Int64()))

				c.sendAll(
					tranNotifyChangeUser,
					NewField(fieldUserID, *c.ID),
					NewField(fieldUserFlags, *c.Flags),
					NewField(fieldUserName, c.UserName),
					NewField(fieldUserIconID, *c.Icon),
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
	err = ioutil.WriteFile(
		filepath.Join(s.ConfigDir, "Banlist.yaml"),
		out,
		0666,
	)
	return err
}

func (s *Server) writeThreadedNews() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	out, err := yaml.Marshal(s.ThreadedNews)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(
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
		Icon:       &[]byte{0, 0},
		Flags:      &[]byte{0, 0},
		UserName:   []byte{},
		Connection: conn,
		Server:     s,
		Version:    &[]byte{},
		AutoReply:  []byte{},
		transfers:  map[int]map[[4]byte]*FileTransfer{},
		Agreed:     false,
		RemoteAddr: remoteAddr,
	}
	clientConn.transfers = map[int]map[[4]byte]*FileTransfer{
		FileDownload:   {},
		FileUpload:     {},
		FolderDownload: {},
		FolderUpload:   {},
		bannerDownload: {},
	}

	*s.NextGuestID++
	ID := *s.NextGuestID

	binary.BigEndian.PutUint16(*clientConn.ID, ID)
	s.Clients[ID] = clientConn

	return clientConn
}

// NewUser creates a new user account entry in the server map and config file
func (s *Server) NewUser(login, name, password string, access []byte) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	account := Account{
		Login:    login,
		Name:     name,
		Password: hashAndSalt([]byte(password)),
		Access:   &access,
	}
	out, err := yaml.Marshal(&account)
	if err != nil {
		return err
	}
	s.Accounts[login] = &account

	return s.FS.WriteFile(filepath.Join(s.ConfigDir, "Users", login+".yaml"), out, 0666)
}

func (s *Server) UpdateUser(login, newLogin, name, password string, access []byte) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	// update renames the user login
	if login != newLogin {
		err := os.Rename(filepath.Join(s.ConfigDir, "Users", login+".yaml"), filepath.Join(s.ConfigDir, "Users", newLogin+".yaml"))
		if err != nil {
			return err
		}
		s.Accounts[newLogin] = s.Accounts[login]
		delete(s.Accounts, login)
	}

	account := s.Accounts[newLogin]
	account.Access = &access
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

	delete(s.Accounts, login)

	return s.FS.Remove(filepath.Join(s.ConfigDir, "Users", login+".yaml"))
}

func (s *Server) connectedUsers() []Field {
	s.mux.Lock()
	defer s.mux.Unlock()

	var connectedUsers []Field
	for _, c := range sortedClients(s.Clients) {
		if !c.Agreed {
			continue
		}
		user := User{
			ID:    *c.ID,
			Icon:  *c.Icon,
			Flags: *c.Flags,
			Name:  string(c.UserName),
		}
		connectedUsers = append(connectedUsers, NewField(fieldUsernameWithInfo, user.Payload()))
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
		if err := decoder.Decode(&account); err != nil {
			return err
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

// dontPanic logs panics instead of crashing
func dontPanic(logger *zap.SugaredLogger) {
	if r := recover(); r != nil {
		fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
		logger.Errorw("PANIC", "err", r, "trace", string(debug.Stack()))
	}
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

	clientLogin, _, err := ReadTransaction(scanner.Bytes())
	if err != nil {
		panic(err)
	}

	c := s.NewClientConn(rwc, remoteAddr)

	// check if remoteAddr is present in the ban list
	if banUntil, ok := s.banList[strings.Split(remoteAddr, ":")[0]]; ok {
		// permaban
		if banUntil == nil {
			s.outbox <- *NewTransaction(
				tranServerMsg,
				c.ID,
				NewField(fieldData, []byte("You are permanently banned on this server")),
				NewField(fieldChatOptions, []byte{0, 0}),
			)
			time.Sleep(1 * time.Second)
			return nil
		} else if time.Now().Before(*banUntil) {
			s.outbox <- *NewTransaction(
				tranServerMsg,
				c.ID,
				NewField(fieldData, []byte("You are temporarily banned on this server")),
				NewField(fieldChatOptions, []byte{0, 0}),
			)
			time.Sleep(1 * time.Second)
			return nil
		}

	}
	defer c.Disconnect()

	encodedLogin := clientLogin.GetField(fieldUserLogin).Data
	encodedPassword := clientLogin.GetField(fieldUserPassword).Data
	*c.Version = clientLogin.GetField(fieldVersion).Data

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
		t := c.NewErrReply(clientLogin, "Incorrect login.")
		b, err := t.MarshalBinary()
		if err != nil {
			return err
		}
		if _, err := rwc.Write(b); err != nil {
			return err
		}

		c.logger.Infow("Login failed", "clientVersion", fmt.Sprintf("%x", *c.Version))

		return nil
	}

	if clientLogin.GetField(fieldUserName).Data != nil {
		c.UserName = clientLogin.GetField(fieldUserName).Data
	}

	if clientLogin.GetField(fieldUserIconID).Data != nil {
		*c.Icon = clientLogin.GetField(fieldUserIconID).Data
	}

	c.Account = c.Server.Accounts[login]

	if c.Authorize(accessDisconUser) {
		*c.Flags = []byte{0, 2}
	}

	s.outbox <- c.NewReply(clientLogin,
		NewField(fieldVersion, []byte{0x00, 0xbe}),
		NewField(fieldCommunityBannerID, []byte{0, 0}),
		NewField(fieldServerName, []byte(s.Config.Name)),
	)

	// Send user access privs so client UI knows how to behave
	c.Server.outbox <- *NewTransaction(tranUserAccess, c.ID, NewField(fieldUserAccess, *c.Account.Access))

	// Show agreement to client
	c.Server.outbox <- *NewTransaction(tranShowAgreement, c.ID, NewField(fieldData, s.Agreement))

	// Used simplified hotline v1.2.3 login flow for clients that do not send login info in tranAgreed
	if *c.Version == nil || bytes.Equal(*c.Version, nostalgiaVersion) {
		c.Agreed = true
		c.logger = c.logger.With("name", string(c.UserName))
		c.logger.Infow("Login successful", "clientVersion", fmt.Sprintf("%x", *c.Version))

		for _, t := range c.notifyOthers(
			*NewTransaction(
				tranNotifyChangeUser, nil,
				NewField(fieldUserName, c.UserName),
				NewField(fieldUserID, *c.ID),
				NewField(fieldUserIconID, *c.Icon),
				NewField(fieldUserFlags, *c.Flags),
			),
		) {
			c.Server.outbox <- t
		}
	}

	c.Server.Stats.LoginCount += 1

	// Scan for new transactions and handle them as they come in.
	for scanner.Scan() {
		// Make a new []byte slice and copy the scanner bytes to it.  This is critical to avoid a data race as the
		// scanner re-uses the buffer for subsequent scans.
		buf := make([]byte, len(scanner.Bytes()))
		copy(buf, scanner.Bytes())

		t, _, err := ReadTransaction(buf)
		if err != nil {
			panic(err)
		}
		if err := c.handleTransaction(*t); err != nil {
			c.logger.Errorw("Error handling transaction", "err", err)
		}
	}
	return nil
}

func (s *Server) NewPrivateChat(cc *ClientConn) []byte {
	s.mux.Lock()
	defer s.mux.Unlock()

	randID := make([]byte, 4)
	rand.Read(randID)
	data := binary.BigEndian.Uint32(randID[:])

	s.PrivateChats[data] = &PrivateChat{
		Subject:    "",
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
		"name", string(fileTransfer.ClientConn.UserName),
	)

	fullPath, err := readPath(s.Config.FileRoot, fileTransfer.FilePath, fileTransfer.FileName)
	if err != nil {
		return err
	}

	switch fileTransfer.Type {
	case bannerDownload:
		if err := s.bannerDownload(rwc); err != nil {
			panic(err)
			return err
		}
	case FileDownload:
		s.Stats.DownloadCounter += 1

		var dataOffset int64
		if fileTransfer.fileResumeData != nil {
			dataOffset = int64(binary.BigEndian.Uint32(fileTransfer.fileResumeData.ForkInfoList[0].DataSize[:]))
		}

		fw, err := newFileWrapper(s.FS, fullPath, 0)
		if err != nil {
			return err
		}

		rLogger.Infow("File download started", "filePath", fullPath)

		// if file transfer options are included, that means this is a "quick preview" request from a 1.5+ client
		if fileTransfer.options == nil {
			// Start by sending flat file object to client
			if _, err := rwc.Write(fw.ffo.BinaryMarshal()); err != nil {
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

		rLogger.Infow("File upload started", "dstFile", fullPath)

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
			s.Logger.Error(err)
		}

		if err := file.Close(); err != nil {
			return err
		}

		if err := s.FS.Rename(fullPath+".incomplete", fullPath); err != nil {
			return err
		}

		rLogger.Infow("File upload complete", "dstFile", fullPath)
	case FolderDownload:
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

		rLogger.Infow("Start folder download", "path", fullPath)

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
			rLogger.Debugw("Sending fileheader", "i", i, "path", path, "fullFilePath", fullPath, "subPath", subPath, "IsDir", info.IsDir())

			if i == 1 {
				return nil
			}

			fileHeader := NewFileHeader(subPath, info.IsDir())

			// Send the fileWrapper header to client
			if _, err := rwc.Write(fileHeader.Payload()); err != nil {
				s.Logger.Errorf("error sending file header: %v", err)
				return err
			}

			// Read the client's Next Action request
			if _, err := io.ReadFull(rwc, nextAction); err != nil {
				return err
			}

			rLogger.Debugw("Client folder download action", "action", fmt.Sprintf("%X", nextAction[0:2]))

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

			rLogger.Infow("File download started",
				"fileName", info.Name(),
				"TransferSize", fmt.Sprintf("%x", hlFile.ffo.TransferSize(dataOffset)),
			)

			// Send file size to client
			if _, err := rwc.Write(hlFile.ffo.TransferSize(dataOffset)); err != nil {
				s.Logger.Error(err)
				return err
			}

			// Send ffo bytes to client
			if _, err := rwc.Write(hlFile.ffo.BinaryMarshal()); err != nil {
				s.Logger.Error(err)
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
		rLogger.Infow(
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

			rLogger.Infow(
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

					if err := receiveFile(rwc, file, ioutil.Discard, ioutil.Discard, fileTransfer.bytesSentCounter); err != nil {
						s.Logger.Error(err)
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

					rLogger.Infow("Starting file transfer", "path", filePath, "fileNum", i+1, "fileSize", binary.BigEndian.Uint32(fileSize))

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
		rLogger.Infof("Folder upload complete")
	}

	return nil
}
