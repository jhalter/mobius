package hotline

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"math/big"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

const (
	trackerListPage = "trackerList"
	serverUIPage    = "serverUI"
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
	Username   string     `yaml:"Username"`
	IconID     int        `yaml:"IconID"`
	Bookmarks  []Bookmark `yaml:"Bookmarks"`
	Tracker    string     `yaml:"Tracker"`
	EnableBell bool       `yaml:"EnableBell"`
}

func (cp *ClientPrefs) IconBytes() []byte {
	iconBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(iconBytes, uint16(cp.IconID))
	return iconBytes
}

func (cp *ClientPrefs) AddBookmark(name, addr, login, pass string) error {
	cp.Bookmarks = append(cp.Bookmarks, Bookmark{Addr: addr, Login: login, Password: pass})

	return nil
}

func readConfig(cfgPath string) (*ClientPrefs, error) {
	fh, err := os.Open(cfgPath)
	if err != nil {
		return nil, err
	}

	prefs := ClientPrefs{}
	decoder := yaml.NewDecoder(fh)
	if err := decoder.Decode(&prefs); err != nil {
		return nil, err
	}
	return &prefs, nil
}

type Client struct {
	cfgPath     string
	DebugBuf    *DebugBuffer
	Connection  net.Conn
	UserAccess  []byte
	filePath    []string
	UserList    []User
	Logger      *zap.SugaredLogger
	activeTasks map[uint32]*Transaction
	serverName  string

	Pref *ClientPrefs

	Handlers map[uint16]ClientHandler

	UI *UI

	Inbox chan *Transaction
}

type ClientHandler func(*Client, *Transaction) ([]Transaction, error)

func (c *Client) HandleFunc(transactionID uint16, handler ClientHandler) {
	c.Handlers[transactionID] = handler
}

func NewClient(username string, logger *zap.SugaredLogger) *Client {
	c := &Client{
		Logger:      logger,
		activeTasks: make(map[uint32]*Transaction),
		Handlers:    make(map[uint16]ClientHandler),
	}
	c.Pref = &ClientPrefs{Username: username}

	return c
}

func NewUIClient(cfgPath string, logger *zap.SugaredLogger) *Client {
	c := &Client{
		cfgPath:     cfgPath,
		Logger:      logger,
		activeTasks: make(map[uint32]*Transaction),
		Handlers:    clientHandlers,
	}
	c.UI = NewUI(c)

	prefs, err := readConfig(cfgPath)
	if err != nil {
		logger.Fatal(fmt.Sprintf("unable to read config file %s\n", cfgPath))
	}
	c.Pref = prefs

	return c
}

// DebugBuffer wraps a *tview.TextView and adds a Sync() method to make it available as a Zap logger
type DebugBuffer struct {
	TextView *tview.TextView
}

func (db *DebugBuffer) Write(p []byte) (int, error) {
	return db.TextView.Write(p)
}

// Sync is a noop function that dataFile to satisfy the zapcore.WriteSyncer interface
func (db *DebugBuffer) Sync() error {
	return nil
}

func randomBanner() string {
	rand.Seed(time.Now().UnixNano())

	bannerFiles, _ := bannerDir.ReadDir("banners")
	file, _ := bannerDir.ReadFile("banners/" + bannerFiles[rand.Intn(len(bannerFiles))].Name())

	return fmt.Sprintf("\n\n\nWelcome to...\n\n[red::b]%s[-:-:-]\n\n", file)
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

type mockClientHandler struct {
	mock.Mock
}

func (mh *mockClientHandler) Handle(cc *Client, t *Transaction) ([]Transaction, error) {
	args := mh.Called(cc, t)
	return args.Get(0).([]Transaction), args.Error(1)
}

var clientHandlers = map[uint16]ClientHandler{
	TranChatMsg:          handleClientChatMsg,
	TranLogin:            handleClientTranLogin,
	TranShowAgreement:    handleClientTranShowAgreement,
	TranUserAccess:       handleClientTranUserAccess,
	TranGetUserNameList:  handleClientGetUserNameList,
	TranNotifyChangeUser: handleNotifyChangeUser,
	TranNotifyDeleteUser: handleNotifyDeleteUser,
	TranGetMsgs:          handleGetMsgs,
	TranGetFileNameList:  handleGetFileNameList,
	TranServerMsg:        handleTranServerMsg,
	TranKeepAlive: func(client *Client, transaction *Transaction) (t []Transaction, err error) {
		return t, err
	},
}

func handleTranServerMsg(c *Client, t *Transaction) (res []Transaction, err error) {
	time := time.Now().Format(time.RFC850)

	msg := strings.ReplaceAll(string(t.GetField(FieldData).Data), "\r", "\n")
	msg += "\n\nAt " + time
	title := fmt.Sprintf("| Private Message From: 	%s |", t.GetField(FieldUserName).Data)

	msgBox := tview.NewTextView().SetScrollable(true)
	msgBox.SetText(msg).SetBackgroundColor(tcell.ColorDarkSlateBlue)
	msgBox.SetTitle(title).SetBorder(true)
	msgBox.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			c.UI.Pages.RemovePage("serverMsgModal" + time)
		}
		return event
	})

	centeredFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(msgBox, 0, 2, true).
			AddItem(nil, 0, 1, false), 0, 2, true).
		AddItem(nil, 0, 1, false)

	c.UI.Pages.AddPage("serverMsgModal"+time, centeredFlex, true, true)
	c.UI.App.Draw() // TODO: errModal doesn't render without this.  wtf?

	return res, err
}

func (c *Client) showErrMsg(msg string) {
	time := time.Now().Format(time.RFC850)

	title := "| Error |"

	msgBox := tview.NewTextView().SetScrollable(true)
	msgBox.SetText(msg).SetBackgroundColor(tcell.ColorDarkRed)
	msgBox.SetTitle(title).SetBorder(true)
	msgBox.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			c.UI.Pages.RemovePage("serverMsgModal" + time)
		}
		return event
	})

	centeredFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(msgBox, 0, 2, true).
			AddItem(nil, 0, 1, false), 0, 2, true).
		AddItem(nil, 0, 1, false)

	c.UI.Pages.AddPage("serverMsgModal"+time, centeredFlex, true, true)
	c.UI.App.Draw() // TODO: errModal doesn't render without this.  wtf?
}

func handleGetFileNameList(c *Client, t *Transaction) (res []Transaction, err error) {
	if t.IsError() {
		c.showErrMsg(string(t.GetField(FieldError).Data))
		c.Logger.Infof("Error: %s", t.GetField(FieldError).Data)
		return res, err
	}

	fTree := tview.NewTreeView().SetTopLevel(1)
	root := tview.NewTreeNode("Root")
	fTree.SetRoot(root).SetCurrentNode(root)
	fTree.SetBorder(true).SetTitle("| Files |")
	fTree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			c.UI.Pages.RemovePage("files")
			c.filePath = []string{}
		case tcell.KeyEnter:
			selectedNode := fTree.GetCurrentNode()

			if selectedNode.GetText() == "<- Back" {
				c.filePath = c.filePath[:len(c.filePath)-1]
				f := NewField(FieldFilePath, EncodeFilePath(strings.Join(c.filePath, "/")))

				if err := c.UI.HLClient.Send(*NewTransaction(TranGetFileNameList, nil, f)); err != nil {
					c.UI.HLClient.Logger.Errorw("err", "err", err)
				}
				return event
			}

			entry := selectedNode.GetReference().(*FileNameWithInfo)

			if bytes.Equal(entry.Type[:], []byte("fldr")) {
				c.Logger.Infow("get new directory listing", "name", string(entry.name))

				c.filePath = append(c.filePath, string(entry.name))
				f := NewField(FieldFilePath, EncodeFilePath(strings.Join(c.filePath, "/")))

				if err := c.UI.HLClient.Send(*NewTransaction(TranGetFileNameList, nil, f)); err != nil {
					c.UI.HLClient.Logger.Errorw("err", "err", err)
				}
			} else {
				// TODO: initiate file download
				c.Logger.Infow("download file", "name", string(entry.name))
			}
		}

		return event
	})

	if len(c.filePath) > 0 {
		node := tview.NewTreeNode("<- Back")
		root.AddChild(node)
	}

	for _, f := range t.Fields {
		var fn FileNameWithInfo
		err = fn.UnmarshalBinary(f.Data)
		if err != nil {
			return nil, nil
		}

		if bytes.Equal(fn.Type[:], []byte("fldr")) {
			node := tview.NewTreeNode(fmt.Sprintf("[blue::]📁 %s[-:-:-]", fn.name))
			node.SetReference(&fn)
			root.AddChild(node)
		} else {
			size := binary.BigEndian.Uint32(fn.FileSize[:]) / 1024

			node := tview.NewTreeNode(fmt.Sprintf("   %-40s %10v KB", fn.name, size))
			node.SetReference(&fn)
			root.AddChild(node)
		}
	}

	centerFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(fTree, 20, 1, true).
			AddItem(nil, 0, 1, false), 60, 1, true).
		AddItem(nil, 0, 1, false)

	c.UI.Pages.AddPage("files", centerFlex, true, true)
	c.UI.App.Draw()

	return res, err
}

func handleGetMsgs(c *Client, t *Transaction) (res []Transaction, err error) {
	newsText := string(t.GetField(FieldData).Data)
	newsText = strings.ReplaceAll(newsText, "\r", "\n")

	newsTextView := tview.NewTextView().
		SetText(newsText).
		SetDoneFunc(func(key tcell.Key) {
			c.UI.Pages.SwitchToPage(serverUIPage)
			c.UI.App.SetFocus(c.UI.chatInput)
		})
	newsTextView.SetBorder(true).SetTitle("News")

	c.UI.Pages.AddPage("news", newsTextView, true, true)
	// c.UI.Pages.SwitchToPage("news")
	// c.UI.App.SetFocus(newsTextView)
	c.UI.App.Draw()

	return res, err
}

func handleNotifyChangeUser(c *Client, t *Transaction) (res []Transaction, err error) {
	newUser := User{
		ID:    t.GetField(FieldUserID).Data,
		Name:  string(t.GetField(FieldUserName).Data),
		Icon:  t.GetField(FieldUserIconID).Data,
		Flags: t.GetField(FieldUserFlags).Data,
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
	exitUser := t.GetField(FieldUserID).Data

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

func handleClientGetUserNameList(c *Client, t *Transaction) (res []Transaction, err error) {
	var users []User
	for _, field := range t.Fields {
		// The Hotline protocol docs say that ClientGetUserNameList should only return FieldUsernameWithInfo (300)
		// fields, but shxd sneaks in FieldChatSubject (115) so it's important to filter explicitly for the expected
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
	if c.Pref.EnableBell {
		fmt.Println("\a")
	}

	_, _ = fmt.Fprintf(c.UI.chatBox, "%s \n", t.GetField(FieldData).Data)

	return res, err
}

func handleClientTranUserAccess(c *Client, t *Transaction) (res []Transaction, err error) {
	c.UserAccess = t.GetField(FieldUserAccess).Data

	return res, err
}

func handleClientTranShowAgreement(c *Client, t *Transaction) (res []Transaction, err error) {
	agreement := string(t.GetField(FieldData).Data)
	agreement = strings.ReplaceAll(agreement, "\r", "\n")

	agreeModal := tview.NewModal().
		SetText(agreement).
		AddButtons([]string{"Agree", "Disagree"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				res = append(res,
					*NewTransaction(
						TranAgreed, nil,
						NewField(FieldUserName, []byte(c.Pref.Username)),
						NewField(FieldUserIconID, c.Pref.IconBytes()),
						NewField(FieldUserFlags, []byte{0x00, 0x00}),
						NewField(FieldOptions, []byte{0x00, 0x00}),
					),
				)
				c.UI.Pages.HidePage("agreement")
				c.UI.App.SetFocus(c.UI.chatInput)
			} else {
				_ = c.Disconnect()
				c.UI.Pages.SwitchToPage("home")
			}
		},
		)

	c.UI.Pages.AddPage("agreement", agreeModal, false, true)

	return res, err
}

func handleClientTranLogin(c *Client, t *Transaction) (res []Transaction, err error) {
	if !bytes.Equal(t.ErrorCode, []byte{0, 0, 0, 0}) {
		errMsg := string(t.GetField(FieldError).Data)
		errModal := tview.NewModal()
		errModal.SetText(errMsg)
		errModal.AddButtons([]string{"Oh no"})
		errModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			c.UI.Pages.RemovePage("errModal")
		})
		c.UI.Pages.RemovePage("joinServer")
		c.UI.Pages.AddPage("errModal", errModal, false, true)

		c.UI.App.Draw() // TODO: errModal doesn't render without this.  wtf?

		c.Logger.Error(string(t.GetField(FieldError).Data))
		return nil, errors.New("login error: " + string(t.GetField(FieldError).Data))
	}
	c.UI.Pages.AddAndSwitchToPage(serverUIPage, c.UI.renderServerUI(), true)
	c.UI.App.SetFocus(c.UI.chatInput)

	if err := c.Send(*NewTransaction(TranGetUserNameList, nil)); err != nil {
		c.Logger.Errorw("err", "err", err)
	}
	return res, err
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
	if err := c.LogIn(login, passwd); err != nil {
		return err
	}

	// start keepalive go routine
	go func() { _ = c.keepalive() }()

	return nil
}

const keepaliveInterval = 300 * time.Second

func (c *Client) keepalive() error {
	for {
		time.Sleep(keepaliveInterval)
		_ = c.Send(*NewTransaction(TranKeepAlive, nil))
		c.Logger.Debugw("Sent keepalive ping")
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
	// Protocol ID	4	‘TRTP’	0x54 52 54 50
	// Sub-protocol ID	4		User defined
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

func (c *Client) LogIn(login string, password string) error {
	return c.Send(
		*NewTransaction(
			TranLogin, nil,
			NewField(FieldUserName, []byte(c.Pref.Username)),
			NewField(FieldUserIconID, c.Pref.IconBytes()),
			NewField(FieldUserLogin, negateString([]byte(login))),
			NewField(FieldUserPassword, negateString([]byte(password))),
		),
	)
}

func (c *Client) Send(t Transaction) error {
	requestNum := binary.BigEndian.Uint16(t.Type)
	tID := binary.BigEndian.Uint32(t.ID)

	// handler := TransactionHandlers[requestNum]

	// if transaction is NOT reply, add it to the list to transactions we're expecting a response for
	if t.IsReply == 0 {
		c.activeTasks[tID] = &t
	}

	var n int
	var err error
	b, err := t.MarshalBinary()
	if err != nil {
		return err
	}
	if n, err = c.Connection.Write(b); err != nil {
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
	c.Logger.Debugw("Received Transaction", "RequestType", requestNum)

	if handler, ok := c.Handlers[requestNum]; ok {
		outT, _ := handler(c, t)
		for _, t := range outT {
			c.Send(t)
		}
	} else {
		c.Logger.Debugw(
			"Unimplemented transaction type received",
			"RequestID", requestNum,
			"TransactionID", t.ID,
		)
	}

	return nil
}

func (c *Client) Disconnect() error {
	return c.Connection.Close()
}

func (c *Client) HandleTransactions() error {
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
		if err := c.HandleTransaction(&t); err != nil {
			c.Logger.Errorw("Error handling transaction", "err", err)
		}
	}

	if scanner.Err() == nil {
		return scanner.Err()
	}
	return nil
}
