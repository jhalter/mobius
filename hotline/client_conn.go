package hotline

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"sync"
)

type byClientID []*ClientConn

func (s byClientID) Len() int {
	return len(s)
}

func (s byClientID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byClientID) Less(i, j int) bool {
	return s[i].uint16ID() < s[j].uint16ID()
}

// ClientConn represents a client connected to a Server
type ClientConn struct {
	Connection io.ReadWriteCloser
	RemoteAddr string
	ID         *[]byte
	Icon       []byte
	Flags      []byte
	UserName   []byte
	Account    *Account
	IdleTime   int
	Server     *Server
	Version    []byte
	Idle       bool
	AutoReply  []byte

	transfersMU sync.Mutex
	transfers   map[int]map[[4]byte]*FileTransfer

	logger *slog.Logger
}

func (cc *ClientConn) sendAll(t int, fields ...Field) {
	for _, c := range sortedClients(cc.Server.Clients) {
		cc.Server.outbox <- *NewTransaction(t, c.ID, fields...)
	}
}

func (cc *ClientConn) handleTransaction(transaction Transaction) error {
	requestNum := binary.BigEndian.Uint16(transaction.Type[:])
	if handler, ok := TransactionHandlers[requestNum]; ok {
		for _, reqField := range handler.RequiredFields {
			field := transaction.GetField(reqField.ID)

			// Validate that required field is present
			if field.ID == [2]byte{0, 0} {
				cc.logger.Error(
					"Missing required field",
					"RequestType", handler.Name, "FieldID", reqField.ID,
				)
				return nil
			}

			if len(field.Data) < reqField.minLen {
				cc.logger.Info(
					"Field does not meet minLen",
					"RequestType", handler.Name, "FieldID", reqField.ID,
				)
				return nil
			}
		}

		cc.logger.Debug("Received Transaction", "RequestType", handler.Name)

		transactions, err := handler.Handler(cc, &transaction)
		if err != nil {
			return fmt.Errorf("error handling transaction: %w", err)
		}
		for _, t := range transactions {
			cc.Server.outbox <- t
		}
	} else {
		cc.logger.Error(
			"Unimplemented transaction type received", "RequestID", requestNum)
	}

	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()

	if requestNum != TranKeepAlive {
		// reset the user idle timer
		cc.IdleTime = 0

		// if user was previously idle, mark as not idle and notify other connected clients that
		// the user is no longer away
		if cc.Idle {
			flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(cc.Flags)))
			flagBitmap.SetBit(flagBitmap, UserFlagAway, 0)
			binary.BigEndian.PutUint16(cc.Flags, uint16(flagBitmap.Int64()))
			cc.Idle = false

			cc.sendAll(
				TranNotifyChangeUser,
				NewField(FieldUserID, *cc.ID),
				NewField(FieldUserFlags, cc.Flags),
				NewField(FieldUserName, cc.UserName),
				NewField(FieldUserIconID, cc.Icon),
			)
		}
	}

	return nil
}

func (cc *ClientConn) Authenticate(login string, password []byte) bool {
	if account, ok := cc.Server.Accounts[login]; ok {
		return bcrypt.CompareHashAndPassword([]byte(account.Password), password) == nil
	}

	return false
}

func (cc *ClientConn) uint16ID() uint16 {
	id, _ := byteToInt(*cc.ID)
	return uint16(id)
}

// Authorize checks if the user account has the specified permission
func (cc *ClientConn) Authorize(access int) bool {
	return cc.Account.Access.IsSet(access)
}

// Disconnect notifies other clients that a client has disconnected
func (cc *ClientConn) Disconnect() {
	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()

	delete(cc.Server.Clients, binary.BigEndian.Uint16(*cc.ID))

	for _, t := range cc.notifyOthers(*NewTransaction(TranNotifyDeleteUser, nil, NewField(FieldUserID, *cc.ID))) {
		cc.Server.outbox <- t
	}

	if err := cc.Connection.Close(); err != nil {
		cc.Server.Logger.Error("error closing client connection", "RemoteAddr", cc.RemoteAddr)
	}
}

// notifyOthers sends transaction t to other clients connected to the server
func (cc *ClientConn) notifyOthers(t Transaction) (trans []Transaction) {
	for _, c := range sortedClients(cc.Server.Clients) {
		if c.ID != cc.ID {
			t.clientID = c.ID
			trans = append(trans, t)
		}
	}
	return trans
}

// NewReply returns a reply Transaction with fields for the ClientConn
func (cc *ClientConn) NewReply(t *Transaction, fields ...Field) Transaction {
	return Transaction{
		IsReply:   0x01,
		Type:      [2]byte{0x00, 0x00},
		ID:        t.ID,
		clientID:  cc.ID,
		ErrorCode: [4]byte{0, 0, 0, 0},
		Fields:    fields,
	}
}

// NewErrReply returns an error reply Transaction with errMsg
func (cc *ClientConn) NewErrReply(t *Transaction, errMsg string) Transaction {
	return Transaction{
		clientID:  cc.ID,
		IsReply:   0x01,
		Type:      [2]byte{0, 0},
		ID:        t.ID,
		ErrorCode: [4]byte{0, 0, 0, 1},
		Fields: []Field{
			NewField(FieldError, []byte(errMsg)),
		},
	}
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

const userInfoTemplate = `Nickname:   %s
Name:       %s
Account:    %s
Address:    %s

-------- File Downloads ---------

%s
------- Folder Downloads --------

%s
--------- File Uploads ----------

%s
-------- Folder Uploads ---------

%s
------- Waiting Downloads -------

%s
`

func formatDownloadList(fts map[[4]byte]*FileTransfer) (s string) {
	if len(fts) == 0 {
		return "None.\n"
	}

	for _, dl := range fts {
		s += dl.String()
	}

	return s
}

func (cc *ClientConn) String() string {
	cc.transfersMU.Lock()
	defer cc.transfersMU.Unlock()
	template := fmt.Sprintf(
		userInfoTemplate,
		cc.UserName,
		cc.Account.Name,
		cc.Account.Login,
		cc.RemoteAddr,
		formatDownloadList(cc.transfers[FileDownload]),
		formatDownloadList(cc.transfers[FolderDownload]),
		formatDownloadList(cc.transfers[FileUpload]),
		formatDownloadList(cc.transfers[FolderUpload]),
		"None.\n",
	)

	return strings.ReplaceAll(template, "\n", "\r")
}
