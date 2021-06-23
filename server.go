package hotline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"math/rand"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"
)

const userIdleSeconds = 300
const idleCheckInterval = 10
const trackerUpdateInterval = 300

type Server struct {
	Addr          int
	Accounts      map[string]*Account
	Agreement     []byte
	Clients       map[uint16]*ClientConn
	FlatNews      []byte
	ThreadedNews  *ThreadedNews
	FileTransfers map[uint32]*FileTransfer
	Config        *Config
	ConfigDir     string
	Logger        *zap.SugaredLogger
	PrivateChats  map[uint32]*PrivateChat
	NextGuestID   *uint16
	outbox        chan Transaction

	mux sync.Mutex
}

type PrivateChat struct {
	Subject    string
	ClientConn map[uint16]*ClientConn
}

func ListenAndServe(addr int, configDir string) error {
	srv, _ := NewServer(configDir)
	srv.Addr = addr

	return srv.ListenAndServe()
}

func (s *Server) ListenAndServe() error {
	var wg sync.WaitGroup

	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", s.Addr))
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() { s.Logger.Fatal(s.Serve(ln)) }()
	s.Logger.Infow("Hotline server started", "Addr", fmt.Sprintf(":%v", s.Addr))

	ln2, err := net.Listen("tcp", fmt.Sprintf(":%v", s.Addr+1))
	if err != nil {
		return err
	}
	wg.Add(1)

	go func() { s.Logger.Fatal(s.ServeFileTransfers(ln2)) }()
	s.Logger.Infow("Hotline file transfer server started", "Addr", fmt.Sprintf(":%v", s.Addr+1))

	wg.Wait()

	return nil
}

func (s *Server) ServeFileTransfers(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go func() {
			if err := s.TransferFile(conn); err != nil {
				s.Logger.Errorw("file transfer error", "reason", err)
			}
		}()
	}
}

func (s *Server) SendTransactions() error {
	for {
		t := <-s.outbox
		requestNum := binary.BigEndian.Uint16(t.Type)
		clientID := binary.BigEndian.Uint16(*t.clientID)

		s.mux.Lock()
		client := s.Clients[clientID]
		s.mux.Unlock()

		handler := TransactionHandlers[requestNum]

		var err error
		var n int
		if n, err = client.Connection.Write(t.Payload()); err != nil {
			logger.Error("ohno")
		}
		logger.Debugw("Sent Transaction",
			"name", string(*client.UserName),
			"login", client.Account.Login,
			"IsReply", t.IsReply,
			"type", handler.Name,
			"bytes", n,
			"remoteAddr", client.Connection.RemoteAddr(),
		)
	}
}

func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go s.SendTransactions()
		go func() {
			if err := s.HandleConnection(conn); err != nil {
				if err == io.EOF {
					s.Logger.Infow("Client disconnected", "RemoteAddr", conn.RemoteAddr())

				} else {
					s.Logger.Errorw("error serving request", "err", err)
				}
			}
		}()
	}
}

// NewServer constructs a new Server from a config dir
func NewServer(configDir string) (*Server, error) {
	cores := []zapcore.Core{newStdoutCore()}
	l := zap.New(zapcore.NewTee(cores...))
	defer l.Sync()
	logger = l.Sugar()

	server := Server{
		Accounts:      make(map[string]*Account),
		Config:        new(Config),
		Clients:       make(map[uint16]*ClientConn),
		FileTransfers: make(map[uint32]*FileTransfer),
		PrivateChats:  make(map[uint32]*PrivateChat),
		ConfigDir:     configDir,
		Logger:        logger,
		NextGuestID:   new(uint16),
		outbox:        make(chan Transaction),
	}

	server.loadAgreement(configDir + "Agreement.txt")
	server.loadFlatNews(configDir + "MessageBoard.txt")
	server.loadThreadedNews(configDir + "ThreadedNews.yaml")
	server.loadConfig(configDir + "config.yaml")
	server.loadAccounts(configDir + "Users/")
	server.Config.FileRoot = configDir + "Files/"

	*server.NextGuestID = 1

	if server.Config.EnableTrackerRegistration == true {
		go func() {
			for {
				for _, t := range server.Config.Trackers {
					server.Logger.Infof("Registering with tracker %v", t)
					if err := server.register(t); err != nil {
						server.Logger.Errorw("unable to register with tracker %v", "error", err)
					}
				}

				time.Sleep(trackerUpdateInterval * time.Second)
			}
		}()
	}

	// Start Client Keepalive go routine
	go func() {
		for {
			time.Sleep(idleCheckInterval * time.Second)
			server.mux.Lock()

			for _, c := range server.Clients {
				*c.IdleTime += 10
				if *c.IdleTime > userIdleSeconds && c.Idle != true {
					c.Idle = true

					flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*c.Flags)))
					flagBitmap.SetBit(flagBitmap, userFlagAway, 1)
					binary.BigEndian.PutUint16(*c.Flags, uint16(flagBitmap.Int64()))

					err := c.Server.NotifyAll(
						NewTransaction(
							tranNotifyChangeUser, 0,
							[]Field{
								NewField(fieldUserID, *c.ID),
								NewField(fieldUserFlags, *c.Flags),
								NewField(fieldUserName, *c.UserName),
								NewField(fieldUserIconID, *c.Icon),
							},
						),
					)
					if err != nil {
						server.Logger.Errorw("err", err)
					}
				}
			}
			server.mux.Unlock()
		}
	}()

	return &server, nil
}

// NotifyAll sends a transaction to all connected clients.  For example, to notify clients of a new chat message.
func (s *Server) NotifyAll(t Transaction) error {
	for _, c := range sortedClients(s.Clients) {
		t.clientID = c.ID
		s.outbox <- t
	}
	return nil
}

func (s *Server) writeThreadedNews() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	out, err := yaml.Marshal(s.ThreadedNews)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(
		s.ConfigDir+"ThreadedNews.yaml",
		out,
		0666,
	)
	return err
}

func (s *Server) NewClientConn(conn net.Conn) *ClientConn {
	s.mux.Lock()
	defer s.mux.Unlock()

	clientConn := &ClientConn{
		ID:         &[]byte{},
		Icon:       &[]byte{},
		Flags:      &[]byte{0, 0},
		UserName:   &[]byte{},
		Connection: conn,
		Server:     s,
		Version:    &[]byte{},
		IdleTime:   new(int),
		AutoReply:  &[]byte{},
	}
	*s.NextGuestID++
	ID := *s.NextGuestID

	*clientConn.IdleTime = 0

	*clientConn.ID = []byte{0, 0}
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

	return ioutil.WriteFile(s.ConfigDir+"Users/"+login+".yaml", out, 0666)
}

// DeleteUser deletes the user account for login
func (s *Server) DeleteUser(login string) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	delete(s.Accounts, login)

	return os.Remove(s.ConfigDir + "Users/" + login + ".yaml")
}

func (s *Server) connectedUsers() []Field {
	s.mux.Lock()
	defer s.mux.Unlock()

	var connectedUsers []Field
	for _, c := range s.Clients {
		user := User{
			ID:    *c.ID,
			Icon:  *c.Icon,
			Flags: *c.Flags,
			Name:  string(*c.UserName),
		}
		connectedUsers = append(connectedUsers, NewField(fieldUsernameWithInfo, user.Payload()))
	}
	return connectedUsers
}

func (s *Server) loadThreadedNews(threadedNewsPath string) {
	fh, err := os.Open(threadedNewsPath)
	if err != nil {
		panic(err)
	}
	news := &ThreadedNews{}
	decoder := yaml.NewDecoder(fh)
	decoder.SetStrict(true)
	err = decoder.Decode(news)
	if err != nil {
		panic(err)
	}
	s.ThreadedNews = news
}

func (s *Server) loadAccounts(userDir string) {
	matches, err := filepath.Glob(path.Join(userDir, "*.yaml"))
	if err != nil {
		panic(err)
	}

	for _, file := range matches {
		fh, _ := os.Open(file)

		account := Account{}
		decoder := yaml.NewDecoder(fh)
		decoder.SetStrict(true)
		err = decoder.Decode(&account)

		s.Accounts[account.Login] = &account
	}
}

func (s *Server) loadConfig(path string) {
	fh, err := os.Open(path)
	if err != nil {
		panic(err)
	}

	decoder := yaml.NewDecoder(fh)
	decoder.SetStrict(true)
	err = decoder.Decode(s.Config)
	if err != nil {
		panic(err)
	}
}

func (s *Server) loadAgreement(agreementPath string) {
	var err error
	s.Agreement, err = ioutil.ReadFile(agreementPath)
	if err != nil {
		panic(err)
	}
}

func (s *Server) loadFlatNews(flatNewsPath string) {
	var err error
	s.FlatNews, err = ioutil.ReadFile(flatNewsPath)
	if err != nil {
		panic(err)
	}
}

const minTransactionLen = 22

func (s *Server) HandleConnection(conn net.Conn) error {
	c := s.NewClientConn(conn)
	defer c.Disconnect()

	if err := c.Handshake(); err != nil {
		return err
	}

	buf := make([]byte, 1024)
	readLen, err := conn.Read(buf)
	if readLen < minTransactionLen {
		return err
	}
	if err != nil {
		return err
	}

	clientLogin, err := ReadTransaction(buf)
	if err != nil {
		return err
	}
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

	// If authentication fails, send error reply and close connection
	if c.Authenticate(login, encodedPassword) == false {
		reply := clientLogin.ReplyTransaction(
			[]Field{
				NewField(fieldError, []byte("Incorrect login.")),
			},
		)
		reply.Type = []byte{0, 0} // TODO: Why is this hardcoded to zero?  Is this observed behavior in the real client?
		reply.ErrorCode = []byte{0, 0, 0, 1}

		if _, err := conn.Write(reply.Payload()); err != nil {
			return err
		}

		return fmt.Errorf("incorrect login")
	}

	if string(*c.Version) == "" {
		*c.UserName = clientLogin.GetField(fieldUserName).Data
		*c.Icon = clientLogin.GetField(fieldUserIconID).Data
	}

	c.Account = c.Server.Accounts[login]

	if c.Authorize(accessDisconUser) == true {
		*c.Flags = []byte{0, 2}
	}

	s.Logger.Infow("Client connection received", "login", login, "version", *c.Version, "RemoteAddr", conn.RemoteAddr().String())

	reply := clientLogin.ReplyTransaction(
		[]Field{
			NewField(fieldVersion, []byte{0x00, 0xbe}),
			NewField(fieldCommunityBannerID, []byte{0x00, 0x01}),
			NewField(fieldServerName, []byte(s.Config.Name)),
		},
	)
	reply.clientID = c.ID
	s.outbox <- reply

	// Send user access privs so client UI knows how to behave
	c.send(tranUserAccess, NewField(fieldUserAccess, *c.Account.Access))

	// Show agreement to client
	c.send(tranShowAgreement, NewField(fieldData, s.Agreement))

	// The Hotline ClientConn v1.2.3 has a different login sequence than 1.9.2
	if string(*c.Version) == "" {
		if _, err := c.notifyNewUserHasJoined(); err != nil {
			return err
		}

		_, err = c.Connection.Write(
			Transaction{
				Flags:     0x00,
				IsReply:   0x01,
				Type:      make([]byte, 2),
				ID:        []byte{0, 0, 0, 3},
				ErrorCode: []byte{0, 0, 0, 0},
				Fields: []Field{
					NewField(fieldData, c.Server.FlatNews),
				},
			}.Payload(),
		)
		if err != nil {
			return err
		}
	}

	// Main loop where we wait for and take action on client requests
	for {
		buf = make([]byte, 102400)
		readLen, err := c.Connection.Read(buf)
		if err != nil {
			return err
		}
		transactions, err := ReadTransactions(buf[:readLen])
		if err != nil {
			c.Server.Logger.Errorw(
				"Error handling transaction", "err", err,
			)
		}

		for _, t := range transactions {
			err := c.handleTransaction(&t)
			if err != nil {
				c.Server.Logger.Errorw(
					"Error handling transaction", "err", err,
				)
			}
		}
	}
}

func hashAndSalt(pwd []byte) string {
	// Use GenerateFromPassword to hash & salt pwd.
	// MinCost is just an integer constant provided by the bcrypt
	// package along with DefaultCost & MaxCost.
	// The cost can be any value you want provided it isn't lower
	// than the MinCost (4)
	hash, err := bcrypt.GenerateFromPassword(pwd, bcrypt.MinCost)
	if err != nil {
		log.Println(err)
	}
	// GenerateFromPassword returns a byte slice so we need to
	// convert the bytes to a string and return it
	return string(hash)
}

func (s *Server) NewTransactionRef() []byte {
	// Generate a random ID for the file transfer.  The Hotline client includes this ID in the file transfer request
	// payload, and the file transfer server will use it to map the request to a transfer
	transactionRef := make([]byte, 4)

	rand.Read(transactionRef)
	return transactionRef
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

const dlFldrAction_SendFile = 1
const dlFldrAction_ResumeFile = 2
const dlFldrAction_NextFile = 3

func (s *Server) TransferFile(conn net.Conn) error {
	defer conn.Close()

	buf := make([]byte, 1024)
	if _, err := conn.Read(buf); err != nil {
		return err
	}

	transaction, err := NewReadTransfer(buf)
	if err != nil {
		return err
	}
	data := binary.BigEndian.Uint32(transaction.ReferenceNumber[:])
	fileTransfer := s.FileTransfers[data]

	switch fileTransfer.Type {
	case FileDownload:
		fullFilePath := fmt.Sprintf("./%v/%v", s.Config.FileRoot+string(fileTransfer.FilePath), string(fileTransfer.FileName))

		ffo, _ := NewFlattenedFileObject(
			s.Config.FileRoot+string(fileTransfer.FilePath),
			string(fileTransfer.FileName),
		)

		s.Logger.Infow("File download started", "transactionRef", fileTransfer.ReferenceNumber, "RemoteAddr", conn.RemoteAddr().String())

		// Start by sending flat file object to client
		if _, err := conn.Write(ffo.Payload()); err != nil {
			return err
		}

		file, err := os.Open(fullFilePath)
		if err != nil {
			return err
		}

		sendBuffer := make([]byte, 1024)

		totalBytesSent := len(ffo.Payload())

		for {
			bytesRead := 0
			bytesRead, err = file.Read(sendBuffer)
			if err == io.EOF {
				break
			}

			sentBytes, readErr := conn.Write(sendBuffer[:bytesRead])
			totalBytesSent += sentBytes
			if readErr != nil {
				return err
			}
		}
	case FileUpload:
		if _, err := conn.Read(buf); err != nil {
			return err
		}

		ffo := ReadFlattenedFileObject(buf)
		payloadLen := len(ffo.Payload())
		fileSize := int(binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize))

		destinationFile := s.Config.FileRoot + ReadFilePath(fileTransfer.FilePath) + "/" + string(fileTransfer.FileName)
		s.Logger.Infow(
			"File upload started",
			"transactionRef", fileTransfer.ReferenceNumber,
			"RemoteAddr", conn.RemoteAddr().String(),
			"size", fileSize,
			"dstFile", destinationFile,
		)

		newFile, err := os.Create(destinationFile)
		if err != nil {
			return err
		}

		defer func() { _ = newFile.Close() }()

		const buffSize = 1024

		if _, err := newFile.Write(buf[payloadLen:]); err != nil {
			return err
		}
		receivedBytes := buffSize - payloadLen

		for {
			if (fileSize - receivedBytes) < buffSize {
				s.Logger.Infow(
					"File upload complete",
					"transactionRef", fileTransfer.ReferenceNumber,
					"RemoteAddr", conn.RemoteAddr().String(),
					"size", fileSize,
					"dstFile", destinationFile,
				)

				_, err := io.CopyN(newFile, conn, int64(fileSize-receivedBytes))
				if err != nil {
					return fmt.Errorf("file transfer failed: %s", err)
				}
				return nil
			}

			// Copy N bytes from conn to upload file
			n, err := io.CopyN(newFile, conn, buffSize)
			if err != nil {
				return err
			}
			receivedBytes += int(n)
		}
	case FolderDownload:
		// Folder Download flow:
		// 1. Get filePath from the Transfer
		// 2. Iterate over files
		// 3. For each file:
		// 	 Send file header
		// After receiving this header client can reply in 3 ways:
		//
		// 1. If type is an odd number (unknown type?), or file download for the current file is completed:
		//		client sends []byte{0x00, 0x03} to tell the server to continue to the next file header
		//
		// 2. If download of a file is to be resumed:
		//		client sends:
		//			[]byte{0x00, 0x02} // download folder action
		//			[2]byte // Resume data size
		//			[]byte file resume data (see myField_FileResumeData)
		// 3. Otherwise download of the file is requested and client sends []byte{0x00, 0x01}
		//
		// When download is requested (case 2 or 3), server replies with:
		// 			[4]byte - file size
		//			[]byte  - Flattened File Object
		//
		// After every file download, client could request next file with:
		// 			[]byte{0x00, 0x03}
		//
		// This notifies the server to send the next item header

		fh := NewFilePath(fileTransfer.FilePath)
		fullFilePath := fmt.Sprintf("./%v/%v", s.Config.FileRoot+fh.String(), string(fileTransfer.FileName))

		basePathLen := len(fullFilePath)

		fmt.Printf("FileTransferBasePath")

		readBuffer := make([]byte, 1024)

		logger.Infow("Start folder download", "path", fullFilePath, "ReferenceNumber", fileTransfer.ReferenceNumber, "RemoteAddr", conn.RemoteAddr())

		i := 0

		filepath.Walk(fullFilePath+"/", func(path string, info os.FileInfo, err error) error {
			i += 1
			subPath := path[basePathLen-2:]
			logger.Infow("Sending fileheader", "i", i, "path", path, "fullFilePath", fullFilePath, "subPath", subPath, "IsDir", info.IsDir())

			fileHeader := NewFileHeader("", subPath, info.IsDir())
			//
			//if info.IsDir() {
			//	// TODO: How to handle subdirs?
			//	fmt.Printf("isDir: %v\n", path)
			//	return nil
			//}

			if i == 1 {
				return nil
			}

			// Send the file header to client
			if _, err := conn.Write(fileHeader.Payload()); err != nil {
				logger.Errorf("error sending file header: %v", err)
				return err
			}

			// Read the client's Next Action request
			//TODO: Remove hardcoded behavior and switch behaviors based on the next action send
			if _, err := conn.Read(readBuffer); err != nil {
				logger.Errorf("error reading next action: %v", err)
				return err
			}

			logger.Infow("Client folder download action", "action", fmt.Sprintf("%X", readBuffer[0:2]))

			if info.IsDir() {
				return nil
			}

			splitPath := strings.Split(path, "/")
			//strings.Join(splitPath[:len(splitPath)-1], "/")

			ffo, err := NewFlattenedFileObject(strings.Join(splitPath[:len(splitPath)-1], "/"), info.Name())
			if err != nil {
				return err
			}
			s.Logger.Infow("File download started",
				"fileName", info.Name(),
				"transactionRef", fileTransfer.ReferenceNumber,
				"RemoteAddr", conn.RemoteAddr().String(),
				"TransferSize", fmt.Sprintf("%x", ffo.TransferSize()),
			)

			spew.Dump(len(ffo.Payload()))

			// Send fileSize to client
			if _, err := conn.Write(ffo.TransferSize()); err != nil {
				logger.Error(err)
				return err
			}

			// Send FFO to client
			if _, err := conn.Write(ffo.Payload()); err != nil {
				logger.Error(err)
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}

			sendBuffer := make([]byte, 1024)
			totalBytesSent := len(ffo.Payload())

			for {
				bytesRead, err := file.Read(sendBuffer)
				if err == io.EOF {
					fmt.Printf("Finished sending file data\n")
					// Read the client's Next Action request
					//TODO: Remove hardcoded behavior and switch behaviors based on the next action send
					if _, err := conn.Read(readBuffer); err != nil {
						logger.Errorf("error reading next action: %v", err)
						return err
					}
					break
				}

				sentBytes, readErr := conn.Write(sendBuffer[:bytesRead])
				totalBytesSent += sentBytes
				if readErr != nil {
					return err
				}
			}
			return nil
		})

	case FolderUpload:
		dstPath := s.Config.FileRoot + ReadFilePath(fileTransfer.FilePath) + "/" + string(fileTransfer.FileName)
		logger.Infow(
			"Folder upload started",
			"transactionRef", fileTransfer.ReferenceNumber,
			"RemoteAddr", conn.RemoteAddr().String(),
			"dstPath", dstPath,
			"TransferSize", fileTransfer.TransferSize,
			"FolderItemCount", fileTransfer.FolderItemCount,
		)

		// Check if the target folder exists.  If not, create it.
		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			logger.Infow("Target path does not exist; Creating...", "dstPath", dstPath)
			if err := os.Mkdir(dstPath, 0777); err != nil {
				logger.Error(err)
			}
		}

		readBuffer := make([]byte, 1024)

		// Begin the folder upload flow by sending the "next file action" to client
		if _, err := conn.Write([]byte{0, dlFldrAction_NextFile}); err != nil {
			return err
		}

		fileSize := make([]byte, 4)
		itemCount := binary.BigEndian.Uint16(fileTransfer.FolderItemCount)

		for i := uint16(0); i < itemCount; i++ {
			if _, err := conn.Read(readBuffer); err != nil {
				return err
			}
			fu := readFolderUpload(readBuffer)

			logger.Infow(
				"Folder upload continued",
				"transactionRef", fmt.Sprintf("%x", fileTransfer.ReferenceNumber),
				"RemoteAddr", conn.RemoteAddr().String(),
				"FormattedPath", string(fu.FormattedPath()),
				"IsFolder", fmt.Sprintf("%x", fu.IsFolder),
				"PathItemCount", binary.BigEndian.Uint16(fu.PathItemCount),
			)

			if bytes.Compare(fu.IsFolder, []byte{0, 1}) == 0 {
				if _, err := os.Stat(dstPath + "/" + fu.FormattedPath()); os.IsNotExist(err) {
					logger.Infow("Target path does not exist; Creating...", "dstPath", dstPath)
					if err := os.Mkdir(dstPath+"/"+fu.FormattedPath(), 0777); err != nil {
						logger.Error(err)
					}
				}
				logger.Infof("Send NextFile to client")
				// Tell client to send next file
				if _, err := conn.Write([]byte{0, dlFldrAction_NextFile}); err != nil {
					logger.Error(err)
					return err
				}
			} else {
				// TODO: Check if we have the full file already.  If so, send dlFldrAction_NextFile to client to skip.
				// TODO: Check if we have a partial file already.  If so, send dlFldrAction_ResumeFile to client to resume upload.
				// Send dlFldrAction_SendFile to client to begin transfer
				if _, err := conn.Write([]byte{0, dlFldrAction_SendFile}); err != nil {
					return err
				}

				if _, err := conn.Read(fileSize); err != nil {
					fmt.Println("Error reading:", err.Error()) // TODO: handle
				}
				logger.Infow("Size of next file", "fileSize", fmt.Sprintf("%x", fileSize))
				logger.Infof("Starting transfer of file %v/%v", i+1, itemCount)
				if err := transferFile(conn, dstPath+"/"+fu.FormattedPath()); err != nil {
					logger.Error(err)
				}

				// Tell client to send next file
				if _, err := conn.Write([]byte{0, dlFldrAction_NextFile}); err != nil {
					logger.Error(err)
					return err
				}

				// Client sends "MACR" after the file.  Read and discard.
				// TODO: This doesn't seem to be documented.  What is this?  Maybe resource fork?
				if _, err := conn.Read(readBuffer); err != nil {
					return err
				}
			}
		}
		logger.Infof("Folder upload complete")
	}

	return nil
}

func transferFile(conn net.Conn, dst string) error {
	logger.Infof("Starting file transfer: %v\n", dst)
	const buffSize = 1024
	buf := make([]byte, buffSize)

	// Read first chunk of bytes from conn; this will be the Flat File Object and initial chunk of file bytes
	if _, err := conn.Read(buf); err != nil {
		return err
	}
	ffo := ReadFlattenedFileObject(buf)
	payloadLen := len(ffo.Payload())
	fileSize := int(binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize))

	newFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = newFile.Close() }()
	if _, err := newFile.Write(buf[payloadLen:]); err != nil {
		return err
	}
	receivedBytes := buffSize - payloadLen

	for {
		if (fileSize - receivedBytes) < buffSize {
			logger.Infow(
				"File transfer complete",
				"RemoteAddr", conn.RemoteAddr().String(),
				"size", fileSize,
				"dstFile", dst,
			)

			_, err := io.CopyN(newFile, conn, int64(fileSize-receivedBytes))
			return err
		}

		// Copy N bytes from conn to upload file
		n, err := io.CopyN(newFile, conn, buffSize)
		if err != nil {
			return err
		}
		receivedBytes += int(n)
	}
}

// 00 28 // DataSize
// 00 00 // IsFolder
// 00 02 // PathItemCount
//
// 00 00
// 09
// 73 75 62 66 6f 6c 64 65 72 // "subfolder"
//
// 00 00
// 15
// 73 75 62 66 6f 6c 64 65 72 2d 74 65 73 74 66 69 6c 65 2d 35 6b // "subfolder-testfile-5k"
func readFolderUpload(buf []byte) folderUpload {
	dataLen := binary.BigEndian.Uint16(buf[0:2])

	fu := folderUpload{
		DataSize:      buf[0:2], // Size of this structure (not including data size element itself)
		IsFolder:      buf[2:4],
		PathItemCount: buf[4:6],
		FileNamePath:  buf[6 : dataLen+2],
	}

	return fu
}

type folderUpload struct {
	DataSize      []byte
	IsFolder      []byte
	PathItemCount []byte
	FileNamePath  []byte
}

func (fu *folderUpload) FormattedPath() string {
	pathItemLen := binary.BigEndian.Uint16(fu.PathItemCount)

	var pathSegments []string
	pathData := fu.FileNamePath

	for i := uint16(0); i < pathItemLen; i++ {
		segLen := pathData[2]
		pathSegments = append(pathSegments, string(pathData[3:3+segLen]))
		pathData = pathData[3+segLen:]
	}

	return strings.Join(pathSegments, "/")
}

// 00 00
// 09
// 73 75 62 66 6f 6c 64 65 72 // "subfolder"
type FilePathItem struct {
	Len  byte
	Name []byte
}

func NewFilePathItem(b []byte) FilePathItem {
	return FilePathItem{
		Len:  b[2],
		Name: b[3:],
	}
}

type FilePath struct {
	PathItemCount []byte
	PathItems     []FilePathItem
}

func NewFilePath(b []byte) FilePath {
	if b == nil {
		return FilePath{}
	}

	fp := FilePath{PathItemCount: b[0:2]}

	// number of items in the path
	pathItemLen := binary.BigEndian.Uint16(b[0:2])
	pathData := b[2:]
	for i := uint16(0); i < pathItemLen; i++ {
		segLen := pathData[2]
		fp.PathItems = append(fp.PathItems, NewFilePathItem(pathData[:segLen+3]))
		pathData = pathData[3+segLen:]
	}

	return fp
}

func (fp *FilePath) String() string {
	var out []string
	for _, i := range fp.PathItems {
		out = append(out, string(i.Name))
	}
	return strings.Join(out, "/")
}

// sortedClients is a utility function that takes a map of *ClientConn and returns a sorted slice of the values.
// The purpose of this is to ensure that the ordering of client connections is deterministic so that test assertions work.
func sortedClients(unsortedClients map[uint16]*ClientConn) (clients []*ClientConn) {
	for _, c := range unsortedClients {
		clients = append(clients, c)
	}
	sort.Sort(byClientID(clients))
	return clients
}
