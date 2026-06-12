package hotline

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/encoding"
)

var clientConnSortFunc = func(a, b *ClientConn) int {
	return cmp.Compare(
		binary.BigEndian.Uint16(a.ID[:]),
		binary.BigEndian.Uint16(b.ID[:]),
	)
}

// sendQueueDepth is the number of transactions that can be queued for delivery to a client before
// the client is considered too slow and is disconnected.
const sendQueueDepth = 64

// ClientConn represents a client connected to a Server
type ClientConn struct {
	Connection io.ReadWriteCloser
	RemoteAddr string
	ID         ClientID
	Version    []byte // TODO: make fixed size of 2

	Account *Account
	Server  *Server // TODO: consider adding methods to interact with server

	// The following fields hold mutable session state guarded by mu.  They are read and written
	// by multiple goroutines (the client's own transaction loop, other clients' handlers, and the
	// server keepalive loop), so production code must use the accessor methods below.  Direct
	// field access is only safe before the connection is shared, e.g. in tests.
	Flags     UserFlags
	UserName  []byte
	Icon      []byte // TODO: make fixed size of 2
	IdleTime  int
	AutoReply []byte

	ClientFileTransferMgr ClientFileTransferMgr

	Logger *slog.Logger

	mu sync.RWMutex // guards the mutable session state fields above

	sendCh     chan Transaction
	sendInit   sync.Once
	sendMu     sync.Mutex // guards sendClosed and close(sendCh)
	sendClosed bool
}

func (cc *ClientConn) initSendQueue() {
	cc.sendInit.Do(func() { cc.sendCh = make(chan Transaction, sendQueueDepth) })
}

// Send enqueues t for delivery to this client by its writer goroutine, preserving enqueue order.
// It never blocks: if the queue is full, the client is considered too slow and its connection is
// closed, which unblocks the client's read loop and triggers the usual Disconnect cleanup.
func (cc *ClientConn) Send(t Transaction) {
	cc.initSendQueue()

	cc.sendMu.Lock()
	defer cc.sendMu.Unlock()

	if cc.sendClosed {
		return
	}

	select {
	case cc.sendCh <- t:
	default:
		cc.sendClosed = true
		close(cc.sendCh)

		if cc.Logger != nil {
			cc.Logger.Warn("Send queue full; disconnecting slow client")
		}
		if cc.Connection != nil {
			_ = cc.Connection.Close()
		}
	}
}

// closeSendQueue idempotently closes the send queue, stopping the client's writer goroutine after
// it drains any remaining queued transactions.
func (cc *ClientConn) closeSendQueue() {
	cc.initSendQueue()

	cc.sendMu.Lock()
	defer cc.sendMu.Unlock()

	if !cc.sendClosed {
		cc.sendClosed = true
		close(cc.sendCh)
	}
}

// writeLoop is the single writer to cc.Connection.  Serializing all writes through one goroutine
// prevents concurrent sends from interleaving bytes within the connection's transaction framing.
// It runs until the send queue is closed or a write fails.
func (cc *ClientConn) writeLoop() {
	cc.initSendQueue()

	for t := range cc.sendCh {
		if _, err := io.Copy(cc.Connection, &t); err != nil {
			if cc.Logger != nil {
				cc.Logger.Debug("error writing transaction to client", "err", err)
			}
			_ = cc.Connection.Close()
			return
		}
	}
}

func (cc *ClientConn) TextDecoder() *encoding.Decoder { return cc.Server.TextDecoder }
func (cc *ClientConn) TextEncoder() *encoding.Encoder { return cc.Server.TextEncoder }

// SetFlag sets the user flag at position flag to v.
func (cc *ClientConn) SetFlag(flag int, v uint) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.Flags.Set(flag, v)
}

// IsFlagSet reports whether the user flag at position flag is set.
func (cc *ClientConn) IsFlagSet(flag int) bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.Flags.IsSet(flag)
}

// FlagBytes returns a copy of the user flags bitmap, suitable for use as a transaction field.
func (cc *ClientConn) FlagBytes() []byte {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	flags := cc.Flags
	return flags[:]
}

// SetUserName sets the client's display name.
func (cc *ClientConn) SetUserName(name []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.UserName = name
}

// GetUserName returns the client's display name.  Callers must not modify the returned slice.
func (cc *ClientConn) GetUserName() []byte {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.UserName
}

// SetIcon sets the client's icon ID bytes.
func (cc *ClientConn) SetIcon(icon []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.Icon = icon
}

// GetIcon returns the client's icon ID bytes.  Callers must not modify the returned slice.
func (cc *ClientConn) GetIcon() []byte {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.Icon
}

// SetAutoReply sets the client's away auto-reply message.
func (cc *ClientConn) SetAutoReply(msg []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.AutoReply = msg
}

// GetAutoReply returns the client's away auto-reply message.  Callers must not modify the
// returned slice.
func (cc *ClientConn) GetAutoReply() []byte {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.AutoReply
}

// incrementIdleTime adds interval seconds to the client's idle time.  It returns true if this
// crossed the idle threshold and marked the client as away, in which case the caller should
// notify other clients of the change.
func (cc *ClientConn) incrementIdleTime(interval int) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.IdleTime += interval
	if cc.IdleTime > userIdleSeconds && !cc.Flags.IsSet(UserFlagAway) {
		cc.Flags.Set(UserFlagAway, 1)
		return true
	}
	return false
}

// clearIdleAndAway resets the client's idle timer.  It returns true if the client was marked as
// away and is no longer, in which case the caller should notify other clients of the change.
func (cc *ClientConn) clearIdleAndAway() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.IdleTime = 0
	if cc.Flags.IsSet(UserFlagAway) {
		cc.Flags.Set(UserFlagAway, 0)
		return true
	}
	return false
}

func (cc *ClientConn) FileRoot() string {
	if cc.Account.FileRoot != "" {
		return cc.Account.FileRoot
	}
	return cc.Server.Config.FileRoot
}

// IP returns the IP address portion of the client's remote address.
func (cc *ClientConn) IP() string {
	ip, _, _ := net.SplitHostPort(cc.RemoteAddr)
	return ip
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
		c.Send(NewTransaction(t, c.ID, fields...))
	}
}

func (cc *ClientConn) handleTransaction(transaction Transaction) {
	// Contain panics to the individual transaction so a single malformed request
	// cannot tear down the whole client connection.
	defer dontPanic(cc.Logger)

	if handler, ok := cc.Server.handlers[transaction.Type]; ok {
		if transaction.Type != TranKeepAlive {
			cc.Logger.Info(tranTypeNames[transaction.Type])
		}

		for _, t := range handler(cc, &transaction) {
			cc.Server.Send(t)
		}
	}

	if transaction.Type != TranKeepAlive {
		// Reset the user idle timer.  If the user was previously marked as away, notify other
		// connected clients that the user is no longer away.
		if cc.clearIdleAndAway() {
			cc.SendAll(
				TranNotifyChangeUser,
				NewField(FieldUserID, cc.ID[:]),
				NewField(FieldUserFlags, cc.FlagBytes()),
				NewField(FieldUserName, cc.GetUserName()),
				NewField(FieldUserIconID, cc.GetIcon()),
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
	// Remove the client from the manager first so no new transactions are routed to it.
	cc.Server.ClientMgr.Delete(cc.ID)

	for _, t := range cc.NotifyOthers(NewTransaction(TranNotifyDeleteUser, [2]byte{}, NewField(FieldUserID, cc.ID[:]))) {
		cc.Server.Send(t)
	}

	cc.closeSendQueue()

	if err := cc.Connection.Close(); err != nil {
		cc.Server.Logger.Debug("error closing client connection", "remoteAddr", cc.RemoteAddr)
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
		cc.GetUserName(),
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
