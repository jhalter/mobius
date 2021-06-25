package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/bcrypt"
	"math/big"
	"net"
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
	Connection net.Conn
	ID         *[]byte
	Icon       *[]byte
	Flags      *[]byte
	UserName   *[]byte
	Account    *Account
	IdleTime   *int
	Server     *Server
	Version    *[]byte
	Idle       bool
	AutoReply  *[]byte
}

func (cc *ClientConn) send(t int, fields ...Field) {
	cc.Server.outbox <- *NewNewTransaction(t, cc.ID, fields...)
}

func (cc *ClientConn) handleTransaction(transaction *Transaction) error {
	requestNum := binary.BigEndian.Uint16(transaction.Type)

	if handler, ok := TransactionHandlers[requestNum]; ok {
		for _, field := range handler.RequiredFields {
			spew.Dump(transaction.GetField(field.ID))
		}
		if !authorize(cc.Account.Access, handler.Access) {
			logger.Infow(
				"Unauthorized Action",
				"Account", cc.Account.Login, "UserName", string(*cc.UserName), "RequestType", handler.Name,
			)
			cc.Server.outbox <- cc.NewErrReply(transaction, handler.DenyMsg)

			return nil
		}

		cc.Server.Logger.Infow(
			"Received Transaction",
			"login", cc.Account.Login,
			"name", string(*cc.UserName),
			"RequestType", handler.Name,
		)

		transactions, err := handler.Handler(cc, transaction)
		if err != nil {
			return err
		}
		for _, t := range transactions {
			cc.Server.outbox <- t
		}
	} else {
		cc.Server.Logger.Errorw(
			"Unimplemented transaction type received",
			"UserName", string(*cc.UserName), "RequestID", requestNum,
		)
	}

	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()

	// Check if user was away before sending this transaction; if so, this transaction
	// indicates they are no longer idle, so notify all clients to clear the away flag
	if *cc.IdleTime > userIdleSeconds && requestNum != tranKeepAlive {
		flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(*cc.Flags)))
		flagBitmap.SetBit(flagBitmap, userFlagAway, 0)
		binary.BigEndian.PutUint16(*cc.Flags, uint16(flagBitmap.Int64()))
		cc.Idle = false
		*cc.IdleTime = 0

		err := cc.Server.NotifyAll(
			NewTransaction(
				tranNotifyChangeUser,
				0,
				[]Field{
					NewField(fieldUserID, *cc.ID),
					NewField(fieldUserFlags, *cc.Flags),
					NewField(fieldUserName, *cc.UserName),
					NewField(fieldUserIconID, *cc.Icon),
				},
			),
		)
		if err != nil {
			return err
		}

		return nil
	}

	*cc.IdleTime = 0

	return nil
}

func (cc *ClientConn) Authenticate(login string, password []byte) bool {
	if account, ok := cc.Server.Accounts[login]; ok {
		result := bcrypt.CompareHashAndPassword([]byte(account.Password), password)
		return result == nil
	}

	return false
}

func (cc *ClientConn) uint16ID() uint16 {
	id, _ := byteToInt(*cc.ID)
	return uint16(id)
}

// Authorize checks if the user account has the specified permission
func (cc *ClientConn) Authorize(access int) bool {
	if access == 0 {
		return true
	}

	accessBitmap := big.NewInt(int64(binary.BigEndian.Uint64(*cc.Account.Access)))

	return accessBitmap.Bit(63-access) == 1
}

// Disconnect notifies other clients that a client has disconnected
func (cc ClientConn) Disconnect() {
	cc.Server.mux.Lock()
	defer cc.Server.mux.Unlock()

	delete(cc.Server.Clients, binary.BigEndian.Uint16(*cc.ID))

	cc.NotifyOthers(
		NewTransaction(
			tranNotifyDeleteUser, 0,
			[]Field{NewField(fieldUserID, *cc.ID)},
		),
	)

	//TODO: Do we really need to send a newline to all connected clients?
	cc.Server.NotifyAll(
		NewTransaction(
			tranChatMsg, 3,
			[]Field{
				NewField(fieldData, []byte("\r")),
			},
		),
	)

	err := cc.Connection.Close()
	if err != nil {
		cc.Server.Logger.Errorw("error closing client connection", "RemoteAddr", cc.Connection.RemoteAddr())
	}
}

// NotifyOthers sends transaction t to other clients connected to the server
func (cc ClientConn) NotifyOthers(t Transaction) {
	for _, c := range sortedClients(cc.Server.Clients) {
		if c.ID != cc.ID {
			t.clientID = c.ID
			cc.Server.outbox <- t
		}
	}
}

type handshake struct {
	Protocol   [4]byte
	SubProtol  [4]byte
	Version    [2]byte
	SubVersion [2]byte
}

// Handshake
// After establishing TCP connection, both client and server start the handshake process
// in order to confirm that each of them comply with requirements of the other.
// The information provided in this initial data exchange identifies protocols,
// and their versions, used in the communication. In the case where, after inspection,
// the capabilities of one of the subjects do not comply with the requirements of the other,
// the connection is dropped.
//
// The following information is sent to the server:
// Description		Size 	Data	Note
// Protocol ID		4		TRTP	0x54525450
// Sub-protocol ID	4		HOTL	User defined
// Version			2		1		Currently 1
// Sub-version		2		2		User defined
//
// The server replies with the following:
// Description		Size 	Data	Note
// Protocol ID		4		TRTP
//Error code		4				Error code returned by the server (0 = no error)
func (cc *ClientConn) Handshake(buf []byte) error {
	var handshake handshake
	r := bytes.NewReader(buf)
	if err := binary.Read(r, binary.BigEndian, &handshake); err != nil {
		return err
	}

	if handshake.Protocol != [4]byte{0x54, 0x52, 0x54, 0x50} {
		return errors.New("invalid handshake")
	}

	_, err := cc.Connection.Write([]byte{84, 82, 84, 80, 0, 0, 0, 0})
	return err
}

// NewReply returns a reply Transaction with fields for the ClientConn
func (cc *ClientConn) NewReply(t *Transaction, fields ...Field) Transaction {
	reply := t.ReplyTransaction(fields)
	reply.clientID = cc.ID

	return reply
}

// NewErrReply returns an error reply Transaction with errMsg
func (cc *ClientConn) NewErrReply(t *Transaction, errMsg string) Transaction {
	return Transaction{
		clientID:  cc.ID,
		Flags:     0x00,
		IsReply:   0x01,
		Type:      []byte{0, 0},
		ID:        t.ID,
		ErrorCode: []byte{0, 0, 0, 1},
		Fields: []Field{
			NewField(fieldError, []byte(errMsg)),
		},
	}
}
