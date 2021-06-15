package hotline

import (
	"encoding/binary"
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
	tran := NewNewTransaction(t, nil, fields...)
	tran.clientID = cc.ID
	cc.Server.outbox <- *tran
}

func (cc *ClientConn) HandleTransaction(transaction *Transaction) error {
	requestNum := binary.BigEndian.Uint16(transaction.Type)

	if handler, ok := TransactionHandlers[requestNum]; ok {
		if !cc.Authorize(handler.Access) {
			logger.Infow(
				"Unauthorized Action",
				"UserName", string(*cc.UserName), "RequestType", handler.Name,
			)
			cc.Server.outbox <- *transaction.NewErrorReply(cc.ID, handler.DenyMsg)
			return nil
		}

		cc.Server.Logger.Infow(
			"Received Transaction",
			"Account", cc.Account.Login, "UserName", string(*cc.UserName), "RequestType", handler.Name,
		)

		var transactions []Transaction
		var err error
		if transactions, err = handler.Handler(cc, transaction); err != nil {
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
		logger.Infow("User is no longer away")
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
			panic(err)
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
	return binary.BigEndian.Uint16(*cc.ID)
}

// Authorize checks if the user account has the specified permission
func (cc *ClientConn) Authorize(access int) bool {
	if access == 0 {
		return true
	}

	accessBitmap := big.NewInt(int64(binary.BigEndian.Uint64(*cc.Account.Access)))

	return accessBitmap.Bit(63-access) == 1
}

func (cc *ClientConn) notifyOtherClientConn(ID []byte, t Transaction) error {
	clientConn := cc.Server.Clients[binary.BigEndian.Uint16(ID)]
	_, err := clientConn.Connection.Write(t.Payload())
	return err
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
	//cc.Server.NotifyAll(
	//	NewTransaction(
	//		tranChatMsg, 3,
	//		[]Field{
	//			NewField(fieldData, []byte("\r")),
	//		},
	//	),
	//)

	_ = cc.Connection.Close()
}

// NotifyOthers sends transaction t to other clients connected to the server
func (cc ClientConn) NotifyOthers(t Transaction) {
	for _, c := range cc.Server.Clients {
		if c.ID != cc.ID {
			t.clientID = c.ID
			cc.Server.outbox <- t
		}
	}
}

func (cc *ClientConn) Handshake() error {
	buf := make([]byte, 1024)
	_, err := cc.Connection.Read(buf)
	if err != nil {
		return err
	}
	_, err = cc.Connection.Write([]byte{84, 82, 84, 80, 0, 0, 0, 0})
	return err
}

func (cc *ClientConn) SendTransaction(id int, fields ...Field) error {
	cc.Connection.Write(
		NewTransaction(
			id,
			0,
			fields,
		).Payload(),
	)

	return nil
}

func (cc *ClientConn) Reply(t *Transaction, fields ...Field) error {
	if _, err := cc.Connection.Write(t.ReplyTransaction(fields).Payload()); err != nil {
		return err
	}

	return nil
}

// NewReply returns a reply Transaction with fields for the ClientConn
func (cc *ClientConn) NewReply(t *Transaction, fields ...Field) Transaction {
	reply := t.ReplyTransaction(fields)
	reply.clientID = cc.ID

	return reply
}

