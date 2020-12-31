package hotline

import (
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
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"
)

const userIdleSeconds = 20
const idleCheckInterval = 10

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
	NextTranID    *uint32

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

func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go func() {
			if err := s.HandleConnection(conn); err != nil {
				s.Logger.Errorw("error serving request", "err", err)
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
		NextTranID:    new(uint32),
	}

	server.loadAgreement(configDir + "Agreement.txt")
	server.loadFlatNews(configDir + "MessageBoard.txt")
	server.loadThreadedNews(configDir + "ThreadedNews.yaml")
	server.loadConfig(configDir + "config.yaml")
	server.loadAccounts(configDir + "Users/")
	server.Config.FileRoot = configDir + "Files/"

	*server.NextGuestID = 1
	*server.NextTranID = 1

	if server.Config.EnableTrackerRegistration == true {
		for _, t := range server.Config.Trackers {
			server.Logger.Infof("Registering with tracker %v", t)
			if err := server.RegisterWithTracker(t); err != nil {
				server.Logger.Errorw("unable to register with tracker %v", "error", err)
			}
		}
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

					c.Server.NotifyAll(
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

				}
			}
			server.mux.Unlock()
		}
	}()

	return &server, nil
}

// NotifyAll sends a transaction to all connected clients.  For example, to notify clients of a new chat message.
func (s *Server) NotifyAll(t Transaction) error {
	for _, c := range s.Clients {
		_, err := c.Connection.Write(t.Payload())
		if err != nil {
			return fmt.Errorf("error sending notify transaction: %s", err)
		}
	}
	return nil
}

func (s *Server) WriteThreadedNews() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	out, _ := yaml.Marshal(s.ThreadedNews)
	return ioutil.WriteFile(
		s.ConfigDir+"ThreadedNews.yaml",
		out,
		0666,
	)
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
		Access:   access,
	}
	out, err := yaml.Marshal(&account)
	if err != nil {
		return err
	}
	s.Accounts[login] = &account

	return ioutil.WriteFile(s.ConfigDir+"Users/"+login+".yaml", out, 0666)
}

// Delete user deletes the user account for login
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

func (s *Server) GetNextTransactionID() uint32 {
	s.mux.Lock()
	defer s.mux.Unlock()

	*s.NextTranID++

	return *s.NextTranID
}

func (s *Server) HandleConnection(conn net.Conn) error {
	hotlineClientConn := s.NewClientConn(conn)
	defer hotlineClientConn.Disconnect()

	if err := hotlineClientConn.Handshake(); err != nil {
		return err
	}

	buf := make([]byte, 1024)
	_, err := conn.Read(buf)
	if err != nil {
		return err
	}

	clientLogin := ReadTransaction(buf)
	encodedLogin := clientLogin.GetField(fieldUserLogin).Data
	encodedPassword := clientLogin.GetField(fieldUserPassword).Data
	*hotlineClientConn.Version = clientLogin.GetField(fieldVersion).Data

	var login string
	for _, char := range encodedLogin {
		login += string(255 - uint(char))
	}
	if login == "" {
		login = GuestAccount
	}

	// If authentication fails, send error reply and close connection
	if hotlineClientConn.Authenticate(login, encodedPassword) == false {
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

	if string(*hotlineClientConn.Version) == "" {
		*hotlineClientConn.UserName = clientLogin.GetField(fieldUserName).Data
		*hotlineClientConn.Icon = clientLogin.GetField(fieldUserIconID).Data
	}

	hotlineClientConn.Account = hotlineClientConn.Server.Accounts[login]

	if hotlineClientConn.Authorize(accessDisconUser) == true {
		*hotlineClientConn.Flags = []byte{0, 2}
	}

	s.Logger.Infow("Client connection received", "login", login, "version", *hotlineClientConn.Version, "RemoteAddr", conn.RemoteAddr().String())

	_, err = hotlineClientConn.Connection.Write(
		clientLogin.ReplyTransaction(
			[]Field{
				NewField(fieldVersion, []byte{0x00, 0xbe}),
				NewField(fieldCommunityBannerID, []byte{0x00, 0x01}),
				NewField(fieldServerName, []byte(s.Config.Name)),
			},
		).Payload(),
	)
	if err != nil {
		return err
	}

	// Send user access privs so client UI knows how to behave
	err = hotlineClientConn.SendTransaction(
		tranUserAccess,
		NewField(fieldUserAccess, hotlineClientConn.Account.Access),
	)
	if err != nil {
		return err
	}

	// Show agreement to client
	err = hotlineClientConn.SendTransaction(
		tranShowAgreement,
		NewField(fieldData, s.Agreement),
	)
	if err != nil {
		panic(err)
	}

	// The Hotline ClientConn v1.2.3 has a different login sequence than 1.9.2
	if string(*hotlineClientConn.Version) == "" {
		if err := hotlineClientConn.notifyNewUserHasJoined(); err != nil {
			return err
		}

		_, err = hotlineClientConn.Connection.Write(
			Transaction{
				Flags:     0x00,
				IsReply:   0x01,
				Type:      make([]byte, 2),
				ID:        []byte{0, 0, 0, 3},
				ErrorCode: []byte{0, 0, 0, 0},
				Fields: []Field{
					NewField(fieldData, hotlineClientConn.Server.FlatNews),
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
		readLen, err := hotlineClientConn.Connection.Read(buf)
		if err != nil {
			return err
		}
		transactions := ReadTransactions(buf[:readLen])

		for _, t := range transactions {
			err := hotlineClientConn.HandleTransaction(&t)
			if err != nil {
				return err
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
		_, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Error reading:", err.Error())
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

		s.Logger.Debugw(
			"File info",
			"CommentSize", ffo.FlatFileInformationFork.CommentSize,
			"Comment", string(ffo.FlatFileInformationFork.Comment),
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
		fmt.Println("Folder Download")

		// The FileName field is a misnomer here; in the case of a folder download, the FileName is really the folder name.
		fullFilePath := fmt.Sprintf("./%v/%v", s.Config.FileRoot+string(fileTransfer.FilePath), string(fileTransfer.FileName))
		totalSize, _ := CalcTotalSize(fullFilePath)
		fmt.Printf("fullFilePath: %v, totalSize: %#v\n", fullFilePath, totalSize)

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

		// re-read buf

		// 00 12 // Header Size (18 )
		// 00 00
		//
		// 00 01 // len
		// 00 		// path section
		// 00 0b // len
		// 6b 69 74 74  65 6e 31 2e  6a 70 67 // kitten1.jpg

		folderTransfer, _ := ReadFolderTransfer(buf)
		spew.Dump(folderTransfer)
		spew.Dump(fileTransfer)

		//decodedFilePath := ReadFilePath(fileTransfer.FilePath)
		//fmt.Printf("folder download filePath: %v\n", decodedFilePath)


		readBuffer := make([]byte, 1024)

		fmt.Printf("walking fullFilePath: %v\n", fullFilePath)
		_ = filepath.Walk(fullFilePath +"/", func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
qq

			fileHeader := NewFileHeader(s.Config.FileRoot+string(fileTransfer.FilePath)+string(fileTransfer.FileName)+"/", info.Name())


			// Send the file header to client
			if _, err := conn.Write(fileHeader.Payload()); err != nil {
				return err
			}

			// Read the client's Next Action request
			if _, err := conn.Read(readBuffer); err != nil {
				return err
			}

			ffo, _ := NewFlattenedFileObject(s.Config.FileRoot+string(fileTransfer.FilePath)+string(fileTransfer.FileName), info.Name())
			s.Logger.Infow("File download started", "fileName", info.Name(), "transactionRef", fileTransfer.ReferenceNumber, "RemoteAddr", conn.RemoteAddr().String())

			// Send fileSize to client
			if _, err := conn.Write(ffo.FlatFileDataForkHeader.DataSize); err != nil {
				return err
			}

			// Send FFO to client
			if _, err := conn.Write(ffo.Payload()); err != nil {
				return err
			}

			fooz := s.Config.FileRoot+string(fileTransfer.FilePath)+string(fileTransfer.FileName) + "/" + info.Name()
			fmt.Printf("Reading file content %v\n", fooz)
			file, err := os.Open(fooz)
			if err != nil {
				return err
			}

			sendBuffer := make([]byte, 512)

			totalBytesSent := len(ffo.Payload())

			for {
				fmt.Printf("Sending file data\n")
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
				fmt.Printf("sentBytes: %v, totalBytesSent: %v\n", sentBytes, totalBytesSent)
			}
			return nil
		})


	case FolderUpload:
		fmt.Println("Folder Upload")

	}

	return nil
}
