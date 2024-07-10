package hotline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"
)

type ClientPrefs struct {
	Username   string `yaml:"Username"`
	IconID     int    `yaml:"IconID"`
	Tracker    string `yaml:"Tracker"`
	EnableBell bool   `yaml:"EnableBell"`
}

func (cp *ClientPrefs) IconBytes() []byte {
	iconBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(iconBytes, uint16(cp.IconID))
	return iconBytes
}

type Client struct {
	Connection  net.Conn
	Logger      *slog.Logger
	Pref        *ClientPrefs
	Handlers    map[[2]byte]ClientHandler
	activeTasks map[[4]byte]*Transaction
}

type ClientHandler func(context.Context, *Client, *Transaction) ([]Transaction, error)

func (c *Client) HandleFunc(tranType [2]byte, handler ClientHandler) {
	c.Handlers[tranType] = handler
}

func NewClient(username string, logger *slog.Logger) *Client {
	c := &Client{
		Logger:      logger,
		activeTasks: make(map[[4]byte]*Transaction),
		Handlers:    make(map[[2]byte]ClientHandler),
		Pref:        &ClientPrefs{Username: username},
	}

	return c
}

type ClientTransaction struct {
	Name    string
	Handler func(*Client, *Transaction) ([]Transaction, error)
}

func (ch ClientTransaction) Handle(cc *Client, t *Transaction) ([]Transaction, error) {
	return ch.Handler(cc, t)
}

type ClientTHandler interface {
	Handle(*Client, *Transaction) ([]Transaction, error)
}

// JoinServer connects to a Hotline server and completes the login flow
func (c *Client) Connect(address, login, passwd string) (err error) {
	// Establish TCP connection to server
	c.Connection, err = net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return err
	}

	// Send handshake sequence
	if err := c.Handshake(); err != nil {
		return err
	}

	// Authenticate (send TranLogin 107)

	err = c.Send(
		NewTransaction(
			TranLogin, [2]byte{0, 0},
			NewField(FieldUserName, []byte(c.Pref.Username)),
			NewField(FieldUserIconID, c.Pref.IconBytes()),
			NewField(FieldUserLogin, encodeString([]byte(login))),
			NewField(FieldUserPassword, encodeString([]byte(passwd))),
		),
	)
	if err != nil {
		return fmt.Errorf("error sending login transaction: %w", err)
	}

	// start keepalive go routine
	go func() { _ = c.keepalive() }()

	return nil
}

const keepaliveInterval = 300 * time.Second

func (c *Client) keepalive() error {
	for {
		time.Sleep(keepaliveInterval)
		_ = c.Send(NewTransaction(TranKeepAlive, [2]byte{}))
	}
}

var ClientHandshake = []byte{
	0x54, 0x52, 0x54, 0x50, // TRTP
	0x48, 0x4f, 0x54, 0x4c, // HOTL
	0x00, 0x01,
	0x00, 0x02,
}

var ServerHandshake = []byte{
	0x54, 0x52, 0x54, 0x50, // TRTP
	0x00, 0x00, 0x00, 0x00, // ErrorCode
}

func (c *Client) Handshake() error {
	// Protocol Type	4	‘TRTP’	0x54 52 54 50
	// Sub-protocol Type	4		User defined
	// Version	2	1	Currently 1
	// Sub-version	2		User defined
	if _, err := c.Connection.Write(ClientHandshake); err != nil {
		return fmt.Errorf("handshake write err: %s", err)
	}

	replyBuf := make([]byte, 8)
	_, err := c.Connection.Read(replyBuf)
	if err != nil {
		return err
	}

	if bytes.Equal(replyBuf, ServerHandshake) {
		return nil
	}

	// In the case of an error, client and server close the connection.
	return fmt.Errorf("handshake response err: %s", err)
}

func (c *Client) Send(t Transaction) error {
	requestNum := binary.BigEndian.Uint16(t.Type[:])

	// if transaction is NOT reply, add it to the list to transactions we're expecting a response for
	if t.IsReply == 0 {
		c.activeTasks[t.ID] = &t
	}

	n, err := io.Copy(c.Connection, &t)
	if err != nil {
		return fmt.Errorf("error sending transaction: %w", err)
	}

	c.Logger.Debug("Sent Transaction",
		"IsReply", t.IsReply,
		"type", requestNum,
		"sentBytes", n,
	)
	return nil
}

func (c *Client) HandleTransaction(ctx context.Context, t *Transaction) error {
	var origT Transaction
	if t.IsReply == 1 {
		origT = *c.activeTasks[t.ID]
		t.Type = origT.Type
	}

	if handler, ok := c.Handlers[t.Type]; ok {
		c.Logger.Debug(
			"Received transaction",
			"IsReply", t.IsReply,
			"type", t.Type[:],
		)
		outT, err := handler(ctx, c, t)
		if err != nil {
			c.Logger.Error("error handling transaction", "err", err)
		}
		for _, t := range outT {
			if err := c.Send(t); err != nil {
				return err
			}
		}
	} else {
		c.Logger.Debug(
			"Unimplemented transaction type",
			"IsReply", t.IsReply,
			"type", t.Type[:],
		)
	}

	return nil
}

func (c *Client) Disconnect() error {
	return c.Connection.Close()
}

func (c *Client) HandleTransactions(ctx context.Context) error {
	// Create a new scanner for parsing incoming bytes into transaction tokens
	scanner := bufio.NewScanner(c.Connection)
	scanner.Split(transactionScanner)

	// Scan for new transactions and handle them as they come in.
	for scanner.Scan() {
		// Make a new []byte slice and copy the scanner bytes to it.  This is critical to avoid a data race as the
		// scanner re-uses the buffer for subsequent scans.
		buf := make([]byte, len(scanner.Bytes()))
		copy(buf, scanner.Bytes())

		var t Transaction
		_, err := t.Write(buf)
		if err != nil {
			break
		}

		if err := c.HandleTransaction(ctx, &t); err != nil {
			c.Logger.Error("Error handling transaction", "err", err)
		}
	}

	if scanner.Err() == nil {
		return scanner.Err()
	}
	return nil
}
