package hotline

import (
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"math/big"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

const (
	trackerListPage = "trackerList"
)

//go:embed banners/*.txt
var bannerDir embed.FS

type Bookmark struct {
	Name     string `yaml:"Name"`
	Addr     string `yaml:"Addr"`
	Login    string `yaml:"Login"`
	Password string `yaml:"Password"`
}

type ClientPrefs struct {
	Username  string     `yaml:"Username"`
	IconID    int        `yaml:"IconID"`
	Bookmarks []Bookmark `yaml:"Bookmarks"`
	Tracker   string     `yaml:"Tracker"`
}

func (cp *ClientPrefs) IconBytes() []byte {
	iconBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(iconBytes, uint16(cp.IconID))
	return iconBytes
}

func readConfig(cfgPath string) (*ClientPrefs, error) {
	fh, err := os.Open(cfgPath)
	if err != nil {
		return nil, err
	}

	prefs := ClientPrefs{}
	decoder := yaml.NewDecoder(fh)
	decoder.SetStrict(true)
	if err := decoder.Decode(&prefs); err != nil {
		return nil, err
	}
	return &prefs, nil
}

type Client struct {
	cfgPath     string
	DebugBuf    *DebugBuffer
	Connection  net.Conn
	Login       *[]byte
	Password    *[]byte
	Flags       *[]byte
	ID          *[]byte
	Version     []byte
	UserAccess  []byte
	Agreed      bool
	UserList    []User
	Logger      *zap.SugaredLogger
	activeTasks map[uint32]*Transaction

	pref *ClientPrefs

	Handlers map[uint16]clientTHandler

	UI *UI

	outbox chan *Transaction
	Inbox  chan *Transaction
}

func NewClient(cfgPath string, logger *zap.SugaredLogger) *Client {
	c := &Client{
		cfgPath:     cfgPath,
		Logger:      logger,
		activeTasks: make(map[uint32]*Transaction),
		Handlers:    clientHandlers,
	}
	c.UI = NewUI(c)

	prefs, err := readConfig(cfgPath)
	if err != nil {
		fmt.Printf("unable to read config file")
		logger.Fatal("unable to read config file", "path", cfgPath)
	}
	c.pref = prefs

	return c
}


// DebugBuffer wraps a *tview.TextView and adds a Sync() method to make it available as a Zap logger
type DebugBuffer struct {
	TextView *tview.TextView
}

func (db *DebugBuffer) Write(p []byte) (int, error) {
	return db.TextView.Write(p)
}

// Sync is a noop function that exists to satisfy the zapcore.WriteSyncer interface
func (db *DebugBuffer) Sync() error {
	return nil
}

func randomBanner() string {
	rand.Seed(time.Now().UnixNano())

	bannerFiles, _ := bannerDir.ReadDir("banners")
	file, _ := bannerDir.ReadFile("banners/" + bannerFiles[rand.Intn(len(bannerFiles))].Name())

	return fmt.Sprintf("\n\n\nWelcome to...\n\n[red::b]%s[-:-:-]\n\n", file)
}





type clientTransaction struct {
	Name    string
	Handler func(*Client, *Transaction) ([]Transaction, error)
}

func (ch clientTransaction) Handle(cc *Client, t *Transaction) ([]Transaction, error) {
	return ch.Handler(cc, t)
}

type clientTHandler interface {
	Handle(*Client, *Transaction) ([]Transaction, error)
}

type mockClientHandler struct {
	mock.Mock
}

func (mh *mockClientHandler) Handle(cc *Client, t *Transaction) ([]Transaction, error) {
	args := mh.Called(cc, t)
	return args.Get(0).([]Transaction), args.Error(1)
}

var clientHandlers = map[uint16]clientTHandler{
	// Server initiated
	tranChatMsg: clientTransaction{
		Name:    "tranChatMsg",
		Handler: handleClientChatMsg,
	},
	tranLogin: clientTransaction{
		Name:    "tranLogin",
		Handler: handleClientTranLogin,
	},
	tranShowAgreement: clientTransaction{
		Name:    "tranShowAgreement",
		Handler: handleClientTranShowAgreement,
	},
	tranUserAccess: clientTransaction{
		Name:    "tranUserAccess",
		Handler: handleClientTranUserAccess,
	},
	tranGetUserNameList: clientTransaction{
		Name:    "tranGetUserNameList",
		Handler: handleClientGetUserNameList,
	},
	tranNotifyChangeUser: clientTransaction{
		Name:    "tranNotifyChangeUser",
		Handler: handleNotifyChangeUser,
	},
	tranNotifyDeleteUser: clientTransaction{
		Name:    "tranNotifyDeleteUser",
		Handler: handleNotifyDeleteUser,
	},
	tranGetMsgs: clientTransaction{
		Name:    "tranNotifyDeleteUser",
		Handler: handleGetMsgs,
	},
}

func handleGetMsgs(c *Client, t *Transaction) (res []Transaction, err error) {
	newsText := string(t.GetField(fieldData).Data)
	newsText = strings.ReplaceAll(newsText, "\r", "\n")

	newsTextView := tview.NewTextView().
		SetText(newsText).
		SetDoneFunc(func(key tcell.Key) {
			c.UI.Pages.SwitchToPage("serverUI")
			c.UI.App.SetFocus(c.UI.chatInput)
		})
	newsTextView.SetBorder(true).SetTitle("News")

	c.UI.Pages.AddPage("news", newsTextView, true, true)
	c.UI.Pages.SwitchToPage("news")
	c.UI.App.SetFocus(newsTextView)
	c.UI.App.Draw()

	return res, err
}

func handleNotifyChangeUser(c *Client, t *Transaction) (res []Transaction, err error) {
	newUser := User{
		ID:    t.GetField(fieldUserID).Data,
		Name:  string(t.GetField(fieldUserName).Data),
		Icon:  t.GetField(fieldUserIconID).Data,
		Flags: t.GetField(fieldUserFlags).Data,
	}

	// Possible cases:
	// user is new to the server
	// user is already on the server but has a new name

	var oldName string
	var newUserList []User
	updatedUser := false
	for _, u := range c.UserList {
		c.Logger.Debugw("Comparing Users", "userToUpdate", newUser.ID, "myID", u.ID, "userToUpdateName", newUser.Name, "myname", u.Name)
		if bytes.Equal(newUser.ID, u.ID) {
			oldName = u.Name
			u.Name = newUser.Name
			if u.Name != newUser.Name {
				_, _ = fmt.Fprintf(c.UI.chatBox, " <<< "+oldName+" is now known as "+newUser.Name+" >>>\n")
			}
			updatedUser = true
		}
		newUserList = append(newUserList, u)
	}

	if !updatedUser {
		newUserList = append(newUserList, newUser)
	}

	c.UserList = newUserList

	c.renderUserList()

	return res, err
}

func handleNotifyDeleteUser(c *Client, t *Transaction) (res []Transaction, err error) {
	exitUser := t.GetField(fieldUserID).Data

	var newUserList []User
	for _, u := range c.UserList {
		if !bytes.Equal(exitUser, u.ID) {
			newUserList = append(newUserList, u)
		}
	}

	c.UserList = newUserList

	c.renderUserList()

	return res, err
}

const readBuffSize = 1024000 // 1KB - TODO: what should this be?

func (c *Client) ReadLoop() error {
	tranBuff := make([]byte, 0)
	tReadlen := 0
	// Infinite loop where take action on incoming client requests until the connection is closed
	for {
		buf := make([]byte, readBuffSize)
		tranBuff = tranBuff[tReadlen:]

		readLen, err := c.Connection.Read(buf)
		if err != nil {
			return err
		}
		tranBuff = append(tranBuff, buf[:readLen]...)

		// We may have read multiple requests worth of bytes from Connection.Read.  readTransactions splits them
		// into a slice of transactions
		var transactions []Transaction
		if transactions, tReadlen, err = readTransactions(tranBuff); err != nil {
			c.Logger.Errorw("Error handling transaction", "err", err)
		}

		// iterate over all of the transactions that were parsed from the byte slice and handle them
		for _, t := range transactions {
			if err := c.HandleTransaction(&t); err != nil {
				c.Logger.Errorw("Error handling transaction", "err", err)
			}
		}
	}
}

func (c *Client) GetTransactions() error {
	tranBuff := make([]byte, 0)
	tReadlen := 0

	buf := make([]byte, readBuffSize)
	tranBuff = tranBuff[tReadlen:]

	readLen, err := c.Connection.Read(buf)
	if err != nil {
		return err
	}
	tranBuff = append(tranBuff, buf[:readLen]...)

	return nil
}

func handleClientGetUserNameList(c *Client, t *Transaction) (res []Transaction, err error) {
	var users []User
	for _, field := range t.Fields {
		// The Hotline protocol docs say that ClientGetUserNameList should only return fieldUsernameWithInfo (300)
		// fields, but shxd sneaks in fieldChatSubject (115) so it's important to filter explicitly for the expected
		// field type.  Probably a good idea to do everywhere.
		if bytes.Equal(field.ID, []byte{0x01, 0x2c}) {
			u, err := ReadUser(field.Data)
			if err != nil {
				return res, err
			}
			users = append(users, *u)
		}
	}
	c.UserList = users

	c.renderUserList()

	return res, err
}

func (c *Client) renderUserList() {
	c.UI.userList.Clear()
	for _, u := range c.UserList {
		flagBitmap := big.NewInt(int64(binary.BigEndian.Uint16(u.Flags)))
		if flagBitmap.Bit(userFlagAdmin) == 1 {
			_, _ = fmt.Fprintf(c.UI.userList, "[red::b]%s[-:-:-]\n", u.Name)
		} else {
			_, _ = fmt.Fprintf(c.UI.userList, "%s\n", u.Name)
		}
		// TODO: fade if user is away
	}
}

func handleClientChatMsg(c *Client, t *Transaction) (res []Transaction, err error) {
	_, _ = fmt.Fprintf(c.UI.chatBox, "%s \n", t.GetField(fieldData).Data)

	return res, err
}

func handleClientTranUserAccess(c *Client, t *Transaction) (res []Transaction, err error) {
	c.UserAccess = t.GetField(fieldUserAccess).Data

	return res, err
}

func handleClientTranShowAgreement(c *Client, t *Transaction) (res []Transaction, err error) {
	agreement := string(t.GetField(fieldData).Data)
	agreement = strings.ReplaceAll(agreement, "\r", "\n")

	c.UI.agreeModal = tview.NewModal().
		SetText(agreement).
		AddButtons([]string{"Agree", "Disagree"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				res = append(res,
					*NewTransaction(
						tranAgreed, nil,
						NewField(fieldUserName, []byte(c.pref.Username)),
						NewField(fieldUserIconID, c.pref.IconBytes()),
						NewField(fieldUserFlags, []byte{0x00, 0x00}),
						NewField(fieldOptions, []byte{0x00, 0x00}),
					),
				)
				c.Agreed = true
				c.UI.Pages.HidePage("agreement")
				c.UI.App.SetFocus(c.UI.chatInput)
			} else {
				_ = c.Disconnect()
				c.UI.Pages.SwitchToPage("home")
			}
		},
		)

	c.Logger.Debug("show agreement page")
	c.UI.Pages.AddPage("agreement", c.UI.agreeModal, false, true)
	c.UI.Pages.ShowPage("agreement ")
	c.UI.App.Draw()

	return res, err
}

func handleClientTranLogin(c *Client, t *Transaction) (res []Transaction, err error) {
	if !bytes.Equal(t.ErrorCode, []byte{0, 0, 0, 0}) {
		errMsg := string(t.GetField(fieldError).Data)
		errModal := tview.NewModal()
		errModal.SetText(errMsg)
		errModal.AddButtons([]string{"Oh no"})
		errModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			c.UI.Pages.RemovePage("errModal")
		})
		c.UI.Pages.RemovePage("joinServer")
		c.UI.Pages.AddPage("errModal", errModal, false, true)

		c.UI.App.Draw() // TODO: errModal doesn't render without this.  wtf?

		c.Logger.Error(string(t.GetField(fieldError).Data))
		return nil, errors.New("login error: " + string(t.GetField(fieldError).Data))
	}
	c.UI.Pages.AddAndSwitchToPage("serverUI", c.UI.renderServerUI(), true)
	c.UI.App.SetFocus(c.UI.chatInput)

	if err := c.Send(*NewTransaction(tranGetUserNameList, nil)); err != nil {
		c.Logger.Errorw("err", "err", err)
	}
	return res, err
}

// JoinServer connects to a Hotline server and completes the login flow
func (c *Client) JoinServer(address, login, passwd string) error {
	// Establish TCP connection to server
	if err := c.connect(address); err != nil {
		return err
	}

	// Send handshake sequence
	if err := c.Handshake(); err != nil {
		return err
	}

	// Authenticate (send tranLogin 107)
	if err := c.LogIn(login, passwd); err != nil {
		return err
	}

	return nil
}

// connect establishes a connection with a Server by sending handshake sequence
func (c *Client) connect(address string) error {
	var err error
	c.Connection, err = net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return err
	}
	return nil
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
	//Protocol ID	4	‘TRTP’	0x54 52 54 50
	//Sub-protocol ID	4		User defined
	//Version	2	1	Currently 1
	//Sub-version	2		User defined
	if _, err := c.Connection.Write(ClientHandshake); err != nil {
		return fmt.Errorf("handshake write err: %s", err)
	}

	replyBuf := make([]byte, 8)
	_, err := c.Connection.Read(replyBuf)
	if err != nil {
		return err
	}

	if bytes.Compare(replyBuf, ServerHandshake) == 0 {
		return nil
	}

	// In the case of an error, client and server close the connection.
	return fmt.Errorf("handshake response err: %s", err)
}

func (c *Client) LogIn(login string, password string) error {
	return c.Send(
		*NewTransaction(
			tranLogin, nil,
			NewField(fieldUserName, []byte(c.pref.Username)),
			NewField(fieldUserIconID, c.pref.IconBytes()),
			NewField(fieldUserLogin, []byte(NegatedUserString([]byte(login)))),
			NewField(fieldUserPassword, []byte(NegatedUserString([]byte(password)))),
			NewField(fieldVersion, []byte{0, 2}),
		),
	)
}

func (c *Client) Send(t Transaction) error {
	requestNum := binary.BigEndian.Uint16(t.Type)
	tID := binary.BigEndian.Uint32(t.ID)

	//handler := TransactionHandlers[requestNum]

	// if transaction is NOT reply, add it to the list to transactions we're expecting a response for
	if t.IsReply == 0 {
		c.activeTasks[tID] = &t
	}

	var n int
	var err error
	if n, err = c.Connection.Write(t.Payload()); err != nil {
		return err
	}
	c.Logger.Debugw("Sent Transaction",
		"IsReply", t.IsReply,
		"type", requestNum,
		"sentBytes", n,
	)
	return nil
}

func (c *Client) HandleTransaction(t *Transaction) error {
	var origT Transaction
	if t.IsReply == 1 {
		requestID := binary.BigEndian.Uint32(t.ID)
		origT = *c.activeTasks[requestID]
		t.Type = origT.Type
	}

	requestNum := binary.BigEndian.Uint16(t.Type)
	c.Logger.Infow(
		"Received Transaction",
		"RequestType", requestNum,
	)

	if handler, ok := c.Handlers[requestNum]; ok {
		outT, _ := handler.Handle(c, t)
		for _, t := range outT {
			c.Send(t)
		}
	} else {
		c.Logger.Errorw(
			"Unimplemented transaction type received",
			"RequestID", requestNum,
			"TransactionID", t.ID,
		)
	}

	return nil
}

func (c *Client) Connected() bool {
	fmt.Printf("Agreed: %v UserAccess: %v\n", c.Agreed, c.UserAccess)
	// c.Agreed == true &&
	if c.UserAccess != nil {
		return true
	}
	return false
}

func (c *Client) Disconnect() error {
	err := c.Connection.Close()
	if err != nil {
		return err
	}
	return nil
}