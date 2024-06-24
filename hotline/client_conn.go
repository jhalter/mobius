package hotline

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
)

// ClientConn represents a client connected to a Server
type ClientConn struct {
	Connection io.ReadWriteCloser
	RemoteAddr string
	ID         [2]byte
	Icon       []byte
	flagsMU    sync.Mutex
	Flags      UserFlags
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

	sync.Mutex
}

func (cc *ClientConn) sendAll(t [2]byte, fields ...Field) {
	for _, c := range cc.Server.Clients {
		cc.Server.outbox <- NewTransaction(t, c.ID, fields...)
	}
}

func (cc *ClientConn) handleTransaction(transaction Transaction) {
	if handler, ok := TransactionHandlers[transaction.Type]; ok {
		cc.logger.Debug("Received Transaction", "RequestType", transaction.Type)

		for _, t := range handler(cc, &transaction) {
			cc.Server.outbox <- t
		}
	}

	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()

	if transaction.Type != TranKeepAlive {
		// reset the user idle timer
		cc.IdleTime = 0

		// if user was previously idle, mark as not idle and notify other connected clients that
		// the user is no longer away
		if cc.Idle {
			cc.Flags.Set(UserFlagAway, 0)
			cc.Idle = false

			cc.sendAll(
				TranNotifyChangeUser,
				NewField(FieldUserID, cc.ID[:]),
				NewField(FieldUserFlags, cc.Flags[:]),
				NewField(FieldUserName, cc.UserName),
				NewField(FieldUserIconID, cc.Icon),
			)
		}
	}
}

func (cc *ClientConn) Authenticate(login string, password []byte) bool {
	if account, ok := cc.Server.Accounts[login]; ok {
		return bcrypt.CompareHashAndPassword([]byte(account.Password), password) == nil
	}

	return false
}

// Authorize checks if the user account has the specified permission
func (cc *ClientConn) Authorize(access int) bool {
	cc.Lock()
	defer cc.Unlock()
	if cc.Account == nil {
		return false
	}
	return cc.Account.Access.IsSet(access)
}

// Disconnect notifies other clients that a client has disconnected
func (cc *ClientConn) Disconnect() {
	cc.Server.mux.Lock()
	delete(cc.Server.Clients, cc.ID)
	cc.Server.mux.Unlock()

	for _, t := range cc.notifyOthers(NewTransaction(TranNotifyDeleteUser, [2]byte{}, NewField(FieldUserID, cc.ID[:]))) {
		cc.Server.outbox <- t
	}

	if err := cc.Connection.Close(); err != nil {
		cc.Server.Logger.Error("error closing client connection", "RemoteAddr", cc.RemoteAddr)
	}
}

// notifyOthers sends transaction t to other clients connected to the server
func (cc *ClientConn) notifyOthers(t Transaction) (trans []Transaction) {
	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()
	for _, c := range cc.Server.Clients {
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
		IsReply:  1,
		ID:       t.ID,
		clientID: cc.ID,
		Fields:   fields,
	}
}

// NewErrReply returns an error reply Transaction with errMsg
func (cc *ClientConn) NewErrReply(t *Transaction, errMsg string) []Transaction {
	return []Transaction{
		{
			clientID:  cc.ID,
			IsReply:   1,
			ID:        t.ID,
			ErrorCode: [4]byte{0, 0, 0, 1},
			Fields: []Field{
				NewField(FieldError, []byte(errMsg)),
			},
		},
	}
}

var clientSortFunc = func(a, b *ClientConn) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.ID[:]),
		binary.BigEndian.Uint16(b.ID[:]),
	)
}

// sortedClients is a utility function that takes a map of *ClientConn and returns a sorted slice of the values.
// The purpose of this is to ensure that the ordering of client connections is deterministic so that test assertions work.
func sortedClients(unsortedClients map[[2]byte]*ClientConn) (clients []*ClientConn) {
	for _, c := range unsortedClients {
		clients = append(clients, c)
	}

	slices.SortFunc(clients, clientSortFunc)

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
