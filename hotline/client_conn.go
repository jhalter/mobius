package hotline

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

var clientConnSortFunc = func(a, b *ClientConn) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.ID[:]),
		binary.BigEndian.Uint16(b.ID[:]),
	)
}

// ClientConn represents a client connected to a Server
type ClientConn struct {
	Connection io.ReadWriteCloser
	RemoteAddr string
	ID         ClientID
	Icon       []byte // TODO: make fixed size of 2
	Version    []byte // TODO: make fixed size of 2

	FlagsMU sync.Mutex // TODO: move into UserFlags struct
	Flags   UserFlags

	UserName  []byte
	Account   *Account
	IdleTime  int
	Server    *Server // TODO: consider adding methods to interact with server
	AutoReply []byte

	ClientFileTransferMgr ClientFileTransferMgr

	Logger *slog.Logger

	mu sync.RWMutex
}

func (cc *ClientConn) FileRoot() string {
	if cc.Account.FileRoot != "" {
		return cc.Account.FileRoot
	}
	return cc.Server.Config.FileRoot
}

type ClientFileTransferMgr struct {
	transfers map[FileTransferType]map[FileTransferID]*FileTransfer

	mu sync.RWMutex
}

func NewClientFileTransferMgr() ClientFileTransferMgr {
	return ClientFileTransferMgr{
		transfers: map[FileTransferType]map[FileTransferID]*FileTransfer{
			FileDownload:   {},
			FileUpload:     {},
			FolderDownload: {},
			FolderUpload:   {},
			BannerDownload: {},
		},
	}
}

func (cftm *ClientFileTransferMgr) Add(ftType FileTransferType, ft *FileTransfer) {
	cftm.mu.Lock()
	defer cftm.mu.Unlock()

	cftm.transfers[ftType][ft.RefNum] = ft
}

func (cftm *ClientFileTransferMgr) Get(ftType FileTransferType) []FileTransfer {
	cftm.mu.Lock()
	defer cftm.mu.Unlock()

	fts := cftm.transfers[ftType]

	var transfers []FileTransfer
	for _, ft := range fts {
		transfers = append(transfers, *ft)
	}

	return transfers
}

func (cftm *ClientFileTransferMgr) Delete(ftType FileTransferType, id FileTransferID) {
	cftm.mu.Lock()
	defer cftm.mu.Unlock()

	delete(cftm.transfers[ftType], id)
}

func (cc *ClientConn) SendAll(t [2]byte, fields ...Field) {
	for _, c := range cc.Server.ClientMgr.List() {
		cc.Server.outbox <- NewTransaction(t, c.ID, fields...)
	}
}

func (cc *ClientConn) handleTransaction(transaction Transaction) {
	if handler, ok := cc.Server.handlers[transaction.Type]; ok {
		if transaction.Type != TranKeepAlive {
			cc.Logger.Info(tranTypeNames[transaction.Type])
		}

		for _, t := range handler(cc, &transaction) {
			cc.Server.outbox <- t
		}
	}

	if transaction.Type != TranKeepAlive {
		cc.mu.Lock()
		defer cc.mu.Unlock()

		// reset the user idle timer
		cc.IdleTime = 0

		// if user was previously idle, mark as not idle and notify other connected clients that
		// the user is no longer away
		if cc.Flags.IsSet(UserFlagAway) {
			cc.Flags.Set(UserFlagAway, 0)

			cc.SendAll(
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
	if account := cc.Server.AccountManager.Get(login); account != nil {
		return bcrypt.CompareHashAndPassword([]byte(account.Password), password) == nil
	}

	return false
}

// Authorize checks if the user account has the specified permission
func (cc *ClientConn) Authorize(access int) bool {
	if cc.Account == nil {
		return false
	}
	return cc.Account.Access.IsSet(access)
}

// Disconnect notifies other clients that a client has disconnected and closes the connection.
func (cc *ClientConn) Disconnect() {
	cc.Server.ClientMgr.Delete(cc.ID)

	for _, t := range cc.NotifyOthers(NewTransaction(TranNotifyDeleteUser, [2]byte{}, NewField(FieldUserID, cc.ID[:]))) {
		cc.Server.outbox <- t
	}

	if err := cc.Connection.Close(); err != nil {
		cc.Server.Logger.Debug("error closing client connection", "RemoteAddr", cc.RemoteAddr)
	}
}

// NotifyOthers sends transaction t to other clients connected to the server
func (cc *ClientConn) NotifyOthers(t Transaction) (trans []Transaction) {
	for _, c := range cc.Server.ClientMgr.List() {
		if c.ID != cc.ID {
			t.ClientID = c.ID
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
		ClientID: cc.ID,
		Fields:   fields,
	}
}

// NewErrReply returns an error reply Transaction with errMsg
func (cc *ClientConn) NewErrReply(t *Transaction, errMsg string) []Transaction {
	return []Transaction{
		{
			ClientID:  cc.ID,
			IsReply:   1,
			ID:        t.ID,
			ErrorCode: [4]byte{0, 0, 0, 1},
			Fields: []Field{
				NewField(FieldError, []byte(errMsg)),
			},
		},
	}
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

func formatDownloadList(fts []FileTransfer) (s string) {
	if len(fts) == 0 {
		return "None.\n"
	}

	for _, dl := range fts {
		s += dl.String()
	}

	return s
}

func (cc *ClientConn) String() string {
	template := fmt.Sprintf(
		userInfoTemplate,
		cc.UserName,
		cc.Account.Name,
		cc.Account.Login,
		cc.RemoteAddr,
		formatDownloadList(cc.ClientFileTransferMgr.Get(FileDownload)),
		formatDownloadList(cc.ClientFileTransferMgr.Get(FolderDownload)),
		formatDownloadList(cc.ClientFileTransferMgr.Get(FileUpload)),
		formatDownloadList(cc.ClientFileTransferMgr.Get(FolderUpload)),
		"None.\n",
	)

	return strings.ReplaceAll(template, "\n", "\r")
}
