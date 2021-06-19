package hotline

// TODO: decide whether to keep or remove nascent client work

//
//import (
//	"bytes"
//	"encoding/binary"
//	"fmt"
//	"net"
//	"time"
//)
//
//type Client struct {
//	Connection net.Conn
//	UserName   []byte
//	Login      *[]byte
//	Password   *[]byte
//	Icon       *[]byte
//	Flags      *[]byte
//	ID         *[]byte
//	Version    []byte
//	UserAccess []byte
//	Agreed     bool
//}
//
//func NewClient(username string) *Client {
//	if username == "" {
//		username = "unnamed"
//	}
//	return &Client{
//		UserName: []byte(username),
//		Icon:     &[]byte{0, 2},
//	}
//}
//
//func (c *Client) JoinServer(address, login, passwd string) error {
//	// Establish TCP connection to server
//	if err := c.Connect(address); err != nil {
//		return err
//	}
//
//	// Send handshake sequence
//	if err := c.Handshake() ; err != nil {
//		return err
//	}
//
//	// Authenticate
//	if err := c.LogIn(login, passwd); err != nil {
//		return err
//	}
//
//
//	// Main loop where we wait for and take action on client requests
//	for {
//		if c.Connected() == true {
//			fmt.Printf("Connected: %v\n", c.Connected())
//			return nil
//		}
//
//		buf := make([]byte, 1400)
//		readLen, err := c.Connection.Read(buf)
//		if err != nil {
//			return err
//		}
//		transactions := ReadTransactions(buf[:readLen])
//
//		for _, t := range transactions {
//			c.HandleTransaction(&t)
//		}
//	}
//
//	return nil
//}
//
//// Connect establishes a connection with a Server by sending handshake sequence
//func (c *Client) Connect(address string) error {
//	var err error
//	c.Connection, err = net.DialTimeout("tcp", address, 5*time.Second)
//	if err != nil {
//		return err
//	}
//	return nil
//}
//
//var ClientHandshake = []byte{
//	0x54, 0x52, 0x54, 0x50,
//	0x00, 0x00, 0x00, 0x00,
//	0x00, 0x01,
//	0x00, 0x00,
//}
//
//var ServerHandshake = []byte{
//	0x54, 0x52, 0x54, 0x50, // TRTP
//	0x00, 0x00, 0x00, 0x00, // ErrorCode
//}
//
//func (c *Client) Handshake() error {
//	//Protocol ID	4	‘TRTP’	0x54 52 54 50
//	//Sub-protocol ID	4		User defined
//	//Version	2	1	Currently 1
//	//Sub-version	2		User defined
//	c.Connection.Write(ClientHandshake)
//
//	replyBuf := make([]byte, 8)
//	_, err := c.Connection.Read(replyBuf)
//
//	for {
//		if bytes.Compare(replyBuf, ServerHandshake) == 0 {
//			return nil
//		}
//	}
//	// In the case of an error, client and server close the connection.
//
//	return err
//}
//
//func (c *Client) LogIn(login string, password string) error {
//	encodedLogin := NegatedUserString([]byte(login))
//	encodedPassword := NegatedUserString([]byte(password))
//
//	err := c.Send(
//		NewTransaction(
//			tranLogin, 1,
//			[]Field{
//				NewField(fieldUserLogin, []byte(encodedLogin)),
//				NewField(fieldUserPassword, []byte(encodedPassword)),
//				NewField(fieldVersion, []byte{0, 2}),
//			},
//		),
//	)
//	if err != nil {
//		return err
//	}
//
//	return nil
//}
//
//// Agree agrees to the server agreement and sends user info, completing the login sequence
//func (c *Client) Agree() {
//	c.Send(
//		NewTransaction(
//			tranAgreed, 3,
//			[]Field{
//				NewField(fieldUserName, []byte("test")),
//				NewField(fieldUserIconID, *c.Icon),
//				NewField(fieldUserFlags, []byte{0x00, 0x00}),
//			},
//		),
//	)
//	//
//	//// Block until we receive the agreement reply from the server
//	//_ = c.WaitForTransaction(tranAgreed)
//}
//
////func (c *Client) WaitForTransaction(id uint16) Transaction {
////	var trans Transaction
////	for {
////		buf := make([]byte, 1400)
////		readLen, err := c.Connection.Read(buf)
////		if err != nil {
////			panic(err)
////		}
////
////		transactions := ReadTransactions(buf[:readLen])
////		tran, err := FindTransactions(id, transactions)
////		if err == nil {
////			fmt.Println("returning")
////			return tran
////		}
////	}
////
////	return trans
////}
//
//func (c *Client) Read() error {
//	// Main loop where we wait for and take action on client requests
//	for {
//		buf := make([]byte, 1400)
//		readLen, err := c.Connection.Read(buf)
//		if err != nil {
//			panic(err)
//		}
//		transactions := ReadTransactions(buf[:readLen])
//
//		for _, t := range transactions {
//			c.HandleTransaction(&t)
//		}
//	}
//
//	return nil
//}
//
//func (c *Client) ReadN(nLimit int) error {
//	n := 0
//	// Main loop where we wait for and take action on client requests
//	for {
//		fmt.Printf("n %v, nLimit %v\n", n, nLimit)
//
//		if n >= nLimit {
//			fmt.Printf("nLimit reached\n")
//			return nil
//		}
//
//		buf := make([]byte, 1400)
//		readLen, err := c.Connection.Read(buf)
//		if err != nil {
//			panic(err)
//		}
//		fmt.Printf("Wait on Read")
//		transactions := ReadTransactions(buf[:readLen])
//		fmt.Printf("n %v, nLimit %v \n", n, nLimit)
//		n += len(transactions)
//
//		for _, t := range transactions {
//			c.HandleTransaction(&t)
//		}
//	}
//
//	return nil
//}
//
//func (c *Client) HandleTransaction(t *Transaction) error {
//	requestNum := binary.BigEndian.Uint16(t.Type)
//	fmt.Printf("Client received request type %v\n", requestNum)
//
//	switch requestType := requestNum; requestType {
//	case tranShowAgreement:
//		c.Agree()
//	case tranLogin:
//		//fmt.Printf("Server name: %s\n", t.GetField(fieldServerName).Data)
//	case tranChatMsg:
//		fmt.Printf(" Chat: %s \n", t.GetField(fieldData).Data)
//	case tranUserAccess:
//		//fmt.Printf("Client received UserAccess from server %#v\n", t.GetField(fieldUserAccess).Data)
//		c.UserAccess = t.GetField(fieldUserAccess).Data
//	case tranNotifyChangeUser:
//		t.ReplyTransaction([]Field{})
//	case tranAgreed:
//		//errField := t.GetField(fieldError)
//		//if errField.Data != nil {
//		//	return fmt.Errorf("login error: %v", string(errField.Data))
//		//}
//		//fmt.Printf("Server acked our agreement\n")
//		c.Agreed = true
//	default:
//		//spew.Dump(t)
//		fmt.Printf("Unimplemented transaction type %v \n", requestNum)
//	}
//
//	return nil
//}
//
//
//func (c *Client) Connected() bool {
//	if c.Agreed == true && c.UserAccess != nil {
//		return true
//	}
//	return false
//}
//
//func (c *Client) Disconnect() {
//	c.Connection.Close()
//}
//
//func (c *Client) ReadTransactions() []Transaction {
//	buf := make([]byte, 4096)
//	n, err := c.Connection.Read(buf)
//
//	if err != nil {
//		panic(err)
//	}
//
//	return ReadTransactions(buf[:n])
//}
//
//func (c *Client) Send(t Transaction) error {
//	_, err := c.Connection.Write(t.Payload())
//	return err
//}
